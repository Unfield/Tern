package oidc_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"

	"golang.org/x/oauth2"

	"github.com/Unfield/Tern/kit"
	"github.com/Unfield/Tern/plugins/oauth"
	"github.com/Unfield/Tern/plugins/oidc"
	"github.com/Unfield/Tern/plugins/sessions"
)

const secret = "test-secret-test-secret-test-secret!!"

type env struct {
	issuer, clientURL string
	provider          *oidc.Plugin
	mu                sync.Mutex
	identity          *oauth.Identity
	signedIn          bool // whether the provider's login page signs users in
}

// newEnv starts a provider app and a client app that signs in with it.
func newEnv(t *testing.T) *env {
	t.Helper()
	e := &env{signedIn: true}
	pl, _ := net.Listen("tcp", "127.0.0.1:0")
	cl, _ := net.Listen("tcp", "127.0.0.1:0")
	e.issuer = "http://" + pl.Addr().String()
	e.clientURL = "http://" + cl.Addr().String()

	e.provider = oidc.New(oidc.Options{
		Issuer: e.issuer,
		Clients: []oidc.Client{
			{ID: "rp", Secret: "rp-secret-0123456789", Name: "Team wiki", RedirectURIs: []string{e.clientURL + "/auth/tern/callback"}},
			{ID: "spa", Public: true, Trusted: true, RedirectURIs: []string{"http://localhost/cb"}},
			{ID: "svc", Secret: "svc-secret-0123456789", GrantTypes: []string{"client_credentials"}, Scopes: []string{"api:read"}},
		},
		Scopes:   []string{"api:read", "api:write"},
		LoginURL: "/login",
		Authenticate: func(ctx *kit.Ctx) (string, error) {
			return sessions.Get(ctx).GetString("user"), nil
		},
		Claims: func(_ context.Context, subject string, _ []string) (map[string]any, error) {
			return map[string]any{"email": subject + "@example.com", "email_verified": true, "name": "Alice", "role": "admin"}, nil
		},
	})
	providerApp, err := kit.New(&kit.Manifest{
		AppHTML: `<html><head>%tern.head%</head><body>%tern.body%</body></html>`,
		Routes: []kit.RouteDef{
			{ID: "/login", Endpoint: map[string]kit.HandlerFunc{"GET": func(c *kit.Ctx) error {
				e.mu.Lock()
				ok := e.signedIn
				e.mu.Unlock()
				if !ok {
					return c.Text(200, "login form")
				}
				if err := sessions.Get(c).Set("user", "alice"); err != nil {
					return err
				}
				return kit.Redirect(http.StatusSeeOther, c.Request.URL.Query().Get("next"))
			}}},
			{ID: "/api/projects", Endpoint: map[string]kit.HandlerFunc{"GET": func(c *kit.Ctx) error {
				tok, err := e.provider.Bearer(c, "api:read")
				if err != nil {
					return err
				}
				return c.Text(200, "projects of "+tok.Subject)
			}}},
		},
	}, kit.WithPlugins(sessions.New(secret), e.provider))
	if err != nil {
		t.Fatal(err)
	}

	clientApp, err := kit.New(&kit.Manifest{
		AppHTML: `<html><head>%tern.head%</head><body>%tern.body%</body></html>`,
		Routes:  []kit.RouteDef{{ID: "/done", Endpoint: map[string]kit.HandlerFunc{"GET": func(c *kit.Ctx) error { return c.Text(200, "signed in") }}}},
	}, kit.WithPlugins(sessions.New(secret), oauth.New(oauth.Options{
		Secret:    secret,
		Providers: []*oauth.Provider{oauth.OIDC("tern", "Tern", e.issuer, "rp", "rp-secret-0123456789")},
		OnLogin: func(_ *kit.Ctx, id oauth.Identity) error {
			e.mu.Lock()
			e.identity = &id
			e.mu.Unlock()
			return nil
		},
	})))
	if err != nil {
		t.Fatal(err)
	}
	ps := &httptest.Server{Listener: pl, Config: &http.Server{Handler: providerApp}}
	cs := &httptest.Server{Listener: cl, Config: &http.Server{Handler: clientApp}}
	ps.Start()
	cs.Start()
	t.Cleanup(ps.Close)
	t.Cleanup(cs.Close)
	return e
}

func browser() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar}
}

var hidden = regexp.MustCompile(`<input type="hidden" name="([^"]+)" value="([^"]*)">`)

func TestSignInWithTheProvider(t *testing.T) {
	e := newEnv(t)
	b := browser()
	res, err := b.Get(e.clientURL + "/auth/tern/login?next=/done")
	if err != nil {
		t.Fatal(err)
	}
	page := read(t, res)
	if !strings.Contains(page, "Team wiki wants to access your account") || !strings.Contains(page, "See your email address") {
		t.Fatalf("expected the consent page, got %d:\n%s", res.StatusCode, page)
	}
	if res.Header.Get("X-Frame-Options") != "DENY" {
		t.Error("the consent page can be framed")
	}

	form := url.Values{"decision": {"allow"}}
	for _, m := range hidden.FindAllStringSubmatch(page, -1) {
		form.Set(m[1], htmlUnescape(m[2]))
	}
	res, err = b.PostForm(res.Request.URL.String(), form)
	if err != nil {
		t.Fatal(err)
	}
	if body := read(t, res); body != "signed in" || res.Request.URL.Path != "/done" {
		t.Fatalf("after consent: %s %q", res.Request.URL, body)
	}
	id := e.identity
	if id == nil || id.Provider != "tern" || id.Subject != "alice" || id.Email != "alice@example.com" || !id.EmailVerified || id.Name != "Alice" || id.Claims["role"] != "admin" {
		t.Fatalf("identity = %+v", id)
	}

	// The consent is remembered: signing in again goes straight through.
	e.identity = nil
	res, err = b.Get(e.clientURL + "/auth/tern/login?next=/done")
	if err != nil {
		t.Fatal(err)
	}
	if body := read(t, res); body != "signed in" || e.identity == nil {
		t.Fatalf("second sign-in: %q", body)
	}

	// Its access token works for userinfo.
	req, _ := http.NewRequest(http.MethodGet, e.issuer+"/oauth/userinfo", nil)
	req.Header.Set("Authorization", "Bearer "+e.identity.Token.AccessToken)
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var info map[string]any
	_ = json.NewDecoder(res.Body).Decode(&info)
	_ = res.Body.Close()
	if info["sub"] != "alice" || info["email"] != "alice@example.com" {
		t.Errorf("userinfo = %v", info)
	}
}

func TestDeniedConsent(t *testing.T) {
	e := newEnv(t)
	b := browser()
	b.CheckRedirect = func(*http.Request, []*http.Request) error { return nil }
	res, _ := b.Get(e.clientURL + "/auth/tern/login?next=/done")
	page := read(t, res)
	form := url.Values{"decision": {"deny"}}
	for _, m := range hidden.FindAllStringSubmatch(page, -1) {
		form.Set(m[1], htmlUnescape(m[2]))
	}
	b.CheckRedirect = func(req *http.Request, _ []*http.Request) error {
		if req.URL.Path == "/login" && req.URL.Host == strings.TrimPrefix(e.clientURL, "http://") {
			return http.ErrUseLastResponse
		}
		return nil
	}
	res, _ = b.PostForm(res.Request.URL.String(), form)
	_ = res.Body.Close()
	if loc := res.Header.Get("Location"); loc != "/login?error=denied" {
		t.Errorf("a denied sign-in goes to %q", loc)
	}
	if e.identity != nil {
		t.Error("OnLogin ran for a denied sign-in")
	}
}

// pkce runs the authorization code flow for the public "spa" client and
// returns the token response.
func pkce(t *testing.T, e *env, scope string) *oauth2.Token {
	t.Helper()
	conf := &oauth2.Config{ClientID: "spa", RedirectURL: "http://localhost/cb", Scopes: strings.Fields(scope),
		Endpoint: oauth2.Endpoint{AuthURL: e.issuer + "/oauth/authorize", TokenURL: e.issuer + "/oauth/token", AuthStyle: oauth2.AuthStyleInParams}}
	verifier := oauth2.GenerateVerifier()
	code := authorize(t, e, conf.AuthCodeURL("st", oauth2.S256ChallengeOption(verifier)))
	tok, err := conf.Exchange(context.Background(), code, oauth2.VerifierOption(verifier))
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

// authorize follows an authorization URL as a signed-in user and returns
// the code from the final redirect.
func authorize(t *testing.T, e *env, u string) string {
	t.Helper()
	q := redirectParams(t, browser(), u)
	if q.Get("error") != "" {
		t.Fatalf("authorize: %s: %s", q.Get("error"), q.Get("error_description"))
	}
	if q.Get("iss") != e.issuer || q.Get("state") != "st" {
		t.Fatalf("redirect params = %v", q)
	}
	return q.Get("code")
}

// redirectParams follows u until it redirects to http://localhost/cb.
func redirectParams(t *testing.T, b *http.Client, u string) url.Values {
	t.Helper()
	var last *url.URL
	b.CheckRedirect = func(req *http.Request, _ []*http.Request) error {
		if req.URL.Host == "localhost" {
			last = req.URL
			return http.ErrUseLastResponse
		}
		return nil
	}
	res, err := b.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	body := read(t, res)
	if last == nil {
		t.Fatalf("no redirect to the client; got %d: %s", res.StatusCode, body)
	}
	return last.Query()
}

func tokenRequest(t *testing.T, e *env, form url.Values) (int, map[string]any) {
	t.Helper()
	res, err := http.PostForm(e.issuer+"/oauth/token", form)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	_ = json.NewDecoder(res.Body).Decode(&body)
	_ = res.Body.Close()
	return res.StatusCode, body
}

func TestPKCE(t *testing.T) {
	e := newEnv(t)
	base := e.issuer + "/oauth/authorize?client_id=spa&redirect_uri=" + url.QueryEscape("http://localhost/cb") + "&response_type=code&scope=openid&state=st"

	// Public clients must use PKCE.
	if q := redirectParams(t, browser(), base); q.Get("error") != "invalid_request" {
		t.Errorf("no challenge: %v", q)
	}
	if q := redirectParams(t, browser(), base+"&code_challenge=abcabcabcabcabcabcabcabcabcabcabcabcabcabca&code_challenge_method=plain"); q.Get("error") != "invalid_request" {
		t.Errorf("plain challenge: %v", q)
	}

	verifier := oauth2.GenerateVerifier()
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	code := authorize(t, e, base+"&code_challenge="+challenge+"&code_challenge_method=S256")
	form := url.Values{"grant_type": {"authorization_code"}, "client_id": {"spa"}, "redirect_uri": {"http://localhost/cb"}, "code": {code}, "code_verifier": {"wrong-" + verifier}}
	if status, body := tokenRequest(t, e, form); status != 400 || body["error"] != "invalid_grant" {
		t.Errorf("wrong verifier: %d %v", status, body)
	}
	// The failed attempt used the code up.
	form.Set("code_verifier", verifier)
	if status, body := tokenRequest(t, e, form); status != 400 || body["error"] != "invalid_grant" {
		t.Errorf("reused code: %d %v", status, body)
	}

	code = authorize(t, e, base+"&code_challenge="+challenge+"&code_challenge_method=S256")
	form.Set("code", code)
	status, body := tokenRequest(t, e, form)
	if status != 200 || body["id_token"] == nil || body["access_token"] == nil || body["refresh_token"] != nil {
		t.Errorf("token: %d %v", status, body)
	}
}

func TestRefreshRotationAndReuse(t *testing.T) {
	e := newEnv(t)
	tok := pkce(t, e, "openid offline_access")
	first := tok.RefreshToken
	if first == "" {
		t.Fatal("offline_access gives a refresh token")
	}
	refresh := func(rt string) (int, map[string]any) {
		return tokenRequest(t, e, url.Values{"grant_type": {"refresh_token"}, "client_id": {"spa"}, "refresh_token": {rt}})
	}
	status, body := refresh(first)
	second, _ := body["refresh_token"].(string)
	if status != 200 || second == "" || second == first || body["access_token"] == nil {
		t.Fatalf("refresh: %d %v", status, body)
	}
	// Using the first token again revokes the whole family, including the
	// token issued from it.
	if status, body := refresh(first); status != 400 || body["error"] != "invalid_grant" {
		t.Errorf("reuse: %d %v", status, body)
	}
	if status, _ := refresh(second); status != 400 {
		t.Errorf("after reuse, the newer token still works: %d", status)
	}
}

func TestBearerScopesAndRevocation(t *testing.T) {
	e := newEnv(t)
	// A service account gets a token for its allowed scopes only.
	status, body := tokenRequest(t, e, url.Values{"grant_type": {"client_credentials"}, "client_id": {"svc"}, "client_secret": {"svc-secret-0123456789"}, "scope": {"api:read api:write"}})
	access, _ := body["access_token"].(string)
	if status != 200 || access == "" || body["scope"] != "api:read" || body["id_token"] != nil {
		t.Fatalf("client_credentials: %d %v", status, body)
	}
	if status, body := tokenRequest(t, e, url.Values{"grant_type": {"client_credentials"}, "client_id": {"svc"}, "client_secret": {"wrong-secret-0123456"}}); status != 401 || body["error"] != "invalid_client" {
		t.Errorf("wrong secret: %d %v", status, body)
	}
	if status, body := tokenRequest(t, e, url.Values{"grant_type": {"client_credentials"}, "client_id": {"spa"}}); status != 400 || body["error"] != "unauthorized_client" {
		t.Errorf("public client_credentials: %d %v", status, body)
	}

	api := func(token string) (int, string) {
		req, _ := http.NewRequest(http.MethodGet, e.issuer+"/api/projects", nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = res.Body.Close() }()
		return res.StatusCode, res.Header.Get("WWW-Authenticate")
	}
	if status, _ := api(access); status != 200 {
		t.Errorf("api with token: %d", status)
	}
	if status, hdr := api(""); status != 401 || !strings.HasPrefix(hdr, "Bearer") {
		t.Errorf("api without token: %d %q", status, hdr)
	}
	if status, hdr := api(access[:len(access)-4] + "AAAA"); status != 401 || !strings.Contains(hdr, "invalid_token") {
		t.Errorf("api with a forged token: %d %q", status, hdr)
	}
	// A user token without api:read is refused with insufficient_scope.
	user := pkce(t, e, "openid")
	if status, hdr := api(user.AccessToken); status != 403 || !strings.Contains(hdr, "insufficient_scope") {
		t.Errorf("api without the scope: %d %q", status, hdr)
	}

	introspect := func(token string) map[string]any {
		req, _ := http.NewRequest(http.MethodPost, e.issuer+"/oauth/introspect", strings.NewReader(url.Values{"token": {token}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.SetBasicAuth("svc", "svc-secret-0123456789")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		var out map[string]any
		_ = json.NewDecoder(res.Body).Decode(&out)
		_ = res.Body.Close()
		return out
	}
	if got := introspect(access); got["active"] != true || got["client_id"] != "svc" {
		t.Errorf("introspect = %v", got)
	}
	res, _ := http.PostForm(e.issuer+"/oauth/revoke", url.Values{"token": {access}, "client_id": {"svc"}, "client_secret": {"svc-secret-0123456789"}})
	_ = res.Body.Close()
	if status, _ := api(access); status != 401 {
		t.Errorf("api after revoking: %d", status)
	}
	if got := introspect(access); got["active"] != false {
		t.Errorf("introspect after revoking = %v", got)
	}
}

func TestAuthorizeErrors(t *testing.T) {
	e := newEnv(t)
	b := browser()
	// An unregistered redirect URI is shown as an error, never redirected to.
	res, _ := b.Get(e.issuer + "/oauth/authorize?client_id=spa&response_type=code&redirect_uri=" + url.QueryEscape("https://evil.example/cb"))
	if body := read(t, res); res.StatusCode != http.StatusBadRequest || res.Request.URL.Host != strings.TrimPrefix(e.issuer, "http://") || !strings.Contains(body, "not registered") {
		t.Errorf("bad redirect URI: %d %s", res.StatusCode, res.Request.URL)
	}
	res, _ = b.Get(e.issuer + "/oauth/authorize?client_id=nobody&response_type=code&redirect_uri=x")
	if read(t, res); res.StatusCode != http.StatusBadRequest {
		t.Errorf("unknown client: %d", res.StatusCode)
	}

	// prompt=none for a signed-out user answers login_required.
	e.signedIn = false
	q := redirectParams(t, browser(), e.issuer+"/oauth/authorize?client_id=spa&response_type=code&state=st&prompt=none&code_challenge=abcabcabcabcabcabcabcabcabcabcabcabcabcabca&code_challenge_method=S256&redirect_uri="+url.QueryEscape("http://localhost/cb"))
	if q.Get("error") != "login_required" || q.Get("state") != "st" {
		t.Errorf("prompt=none: %v", q)
	}
	// Otherwise the user is sent to log in, with a way back.
	b2 := browser()
	res, _ = b2.Get(e.issuer + "/oauth/authorize?client_id=spa&response_type=code&state=st&code_challenge=abcabcabcabcabcabcabcabcabcabcabcabcabcabca&code_challenge_method=S256&redirect_uri=" + url.QueryEscape("http://localhost/cb"))
	if body := read(t, res); body != "login form" || !strings.HasPrefix(res.Request.URL.Query().Get("next"), "/oauth/authorize?") {
		t.Errorf("signed out: %s %q", res.Request.URL, body)
	}
}

func TestDiscovery(t *testing.T) {
	e := newEnv(t)
	res, err := http.Get(e.issuer + "/.well-known/openid-configuration")
	if err != nil {
		t.Fatal(err)
	}
	var d map[string]any
	_ = json.NewDecoder(res.Body).Decode(&d)
	_ = res.Body.Close()
	if d["issuer"] != e.issuer || d["token_endpoint"] != e.issuer+"/oauth/token" || d["code_challenge_methods_supported"].([]any)[0] != "S256" {
		t.Errorf("discovery = %v", d)
	}
	res, _ = http.Get(e.issuer + "/oauth/jwks")
	var jwks struct {
		Keys []map[string]any `json:"keys"`
	}
	_ = json.NewDecoder(res.Body).Decode(&jwks)
	_ = res.Body.Close()
	if len(jwks.Keys) != 1 || jwks.Keys[0]["kty"] != "RSA" || jwks.Keys[0]["d"] != nil {
		t.Errorf("jwks = %v (the private part must not be published)", jwks)
	}
	// Browser apps on other sites can call the token endpoint.
	req, _ := http.NewRequest(http.MethodOptions, e.issuer+"/oauth/token", nil)
	req.Header.Set("Origin", "https://spa.example")
	req.Header.Set("Access-Control-Request-Method", "POST")
	res, _ = http.DefaultClient.Do(req)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusNoContent || res.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("preflight: %d %v", res.StatusCode, res.Header)
	}
}

func read(t *testing.T, res *http.Response) string {
	t.Helper()
	b, err := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func htmlUnescape(s string) string {
	return strings.NewReplacer("&amp;", "&", "&lt;", "<", "&gt;", ">", "&#34;", `"`, "&#39;", "'", "&#43;", "+", "&quot;", `"`).Replace(s)
}

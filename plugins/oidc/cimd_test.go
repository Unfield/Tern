package oidc_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"golang.org/x/oauth2"

	"github.com/Unfield/Tern/plugins/oidc"
)

// metadataServer serves a client metadata document over TLS and returns
// its URL, the client ID.
func metadataServer(t *testing.T, doc func(id string) map[string]any, fetches *atomic.Int32) (*httptest.Server, string) {
	t.Helper()
	var id string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/client.json" {
			http.NotFound(w, r)
			return
		}
		if fetches != nil {
			fetches.Add(1)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "max-age=600")
		_ = json.NewEncoder(w).Encode(doc(id))
	}))
	t.Cleanup(srv.Close)
	id = srv.URL + "/client.json"
	return srv, id
}

func assistant(id string) map[string]any {
	return map[string]any{
		"client_id": id, "client_name": "Assistant", "redirect_uris": []string{"http://localhost/cb"},
		"grant_types": []string{"authorization_code", "refresh_token"}, "response_types": []string{"code"}, "token_endpoint_auth_method": "none",
	}
}

func withMetadata(srv *httptest.Server) func(*oidc.Options) {
	return func(o *oidc.Options) {
		o.ClientMetadataDocuments = true
		// The test server is on 127.0.0.1, which the default client refuses.
		o.MetadataClient = srv.Client()
	}
}

// consentAndCode approves the consent page that u leads to, and returns
// the page and the code from the redirect.
func consentAndCode(t *testing.T, u string) (string, url.Values) {
	t.Helper()
	b := browser()
	res, err := b.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	page := read(t, res)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("authorize: %d %s", res.StatusCode, page)
	}
	form := url.Values{"decision": {"allow"}}
	for _, m := range hidden.FindAllStringSubmatch(page, -1) {
		form.Set(m[1], htmlUnescape(m[2]))
	}
	var last *url.URL
	b.CheckRedirect = func(req *http.Request, _ []*http.Request) error {
		if req.URL.Host == "localhost" {
			last = req.URL
			return http.ErrUseLastResponse
		}
		return nil
	}
	res, err = b.PostForm(res.Request.URL.String(), form)
	if err != nil {
		t.Fatal(err)
	}
	_ = read(t, res)
	if last == nil {
		t.Fatal("no redirect after consent")
	}
	return page, last.Query()
}

func TestClientMetadataDocument(t *testing.T) {
	var fetches atomic.Int32
	srv, id := metadataServer(t, assistant, &fetches)
	e := newEnv(t, withMetadata(srv))
	resource := e.issuer + "/mcp"

	res, _ := http.Get(e.issuer + "/.well-known/oauth-authorization-server")
	var meta map[string]any
	_ = json.NewDecoder(res.Body).Decode(&meta)
	_ = res.Body.Close()
	if meta["issuer"] != e.issuer || meta["client_id_metadata_document_supported"] != true {
		t.Fatalf("authorization server metadata: %v", meta)
	}

	verifier := oauth2.GenerateVerifier()
	sum := sha256.Sum256([]byte(verifier))
	q := url.Values{
		"client_id": {id}, "redirect_uri": {"http://localhost/cb"}, "response_type": {"code"}, "state": {"st"},
		"code_challenge": {base64.RawURLEncoding.EncodeToString(sum[:])}, "code_challenge_method": {"S256"}, "resource": {resource},
	}
	page, params := consentAndCode(t, e.issuer+"/oauth/authorize?"+q.Encode())
	// Never trusted: the user approves it, and sees where it leads.
	if !strings.Contains(page, "Assistant wants to access your account") || !strings.Contains(page, "<strong>localhost</strong>") {
		t.Fatalf("consent page:\n%s", page)
	}
	form := url.Values{"grant_type": {"authorization_code"}, "client_id": {id}, "redirect_uri": {"http://localhost/cb"},
		"code": {params.Get("code")}, "code_verifier": {verifier}, "resource": {resource}}
	status, body := tokenRequest(t, e, form)
	if status != 200 {
		t.Fatalf("token: %d %v", status, body)
	}
	tok, err := e.provider.Verify(context.Background(), body["access_token"].(string))
	if err != nil {
		t.Fatal(err)
	}
	if !tok.For(resource) || tok.For(e.issuer+"/api") || tok.ClientID != id || tok.Subject != "alice" {
		t.Fatalf("token = %+v", tok)
	}

	// The refreshed token is for the same resource.
	status, body = tokenRequest(t, e, url.Values{"grant_type": {"refresh_token"}, "client_id": {id}, "refresh_token": {body["refresh_token"].(string)}})
	if status != 200 {
		t.Fatalf("refresh: %d %v", status, body)
	}
	if tok, _ := e.provider.Verify(context.Background(), body["access_token"].(string)); tok == nil || !tok.For(resource) {
		t.Fatalf("refreshed token = %+v", tok)
	}
	if n := fetches.Load(); n != 1 {
		t.Errorf("the document was fetched %d times; it should be cached", n)
	}
}

func TestResourceIndicators(t *testing.T) {
	e := newEnv(t)
	base := url.Values{"client_id": {"spa"}, "redirect_uri": {"http://localhost/cb"}, "response_type": {"code"}, "state": {"st"},
		"code_challenge": {strings.Repeat("a", 43)}, "code_challenge_method": {"S256"}}
	q := url.Values{}
	for k, v := range base {
		q[k] = v
	}
	q.Set("resource", "https://elsewhere.example/mcp")
	if p := redirectParams(t, browser(), e.issuer+"/oauth/authorize?"+q.Encode()); p.Get("error") != "invalid_target" {
		t.Errorf("a resource on another site: %v", p)
	}

	// A code for one resource can't be exchanged for another.
	conf := &oauth2.Config{ClientID: "spa", RedirectURL: "http://localhost/cb",
		Endpoint: oauth2.Endpoint{AuthURL: e.issuer + "/oauth/authorize", TokenURL: e.issuer + "/oauth/token", AuthStyle: oauth2.AuthStyleInParams}}
	verifier := oauth2.GenerateVerifier()
	code := authorize(t, e, conf.AuthCodeURL("st", oauth2.S256ChallengeOption(verifier), oauth2.SetAuthURLParam("resource", e.issuer+"/mcp")))
	status, body := tokenRequest(t, e, url.Values{"grant_type": {"authorization_code"}, "client_id": {"spa"}, "redirect_uri": {"http://localhost/cb"},
		"code": {code}, "code_verifier": {verifier}, "resource": {e.issuer + "/other"}})
	if status != 400 || body["error"] != "invalid_target" {
		t.Errorf("another resource at the token endpoint: %d %v", status, body)
	}

	// Without a resource, the token is for the client, as before.
	tok := pkce(t, e, "api:read")
	at, err := e.provider.Verify(context.Background(), tok.AccessToken)
	if err != nil || at.For(e.issuer+"/mcp") || !at.For("spa") {
		t.Fatalf("token without a resource: %+v %v", at, err)
	}
}

func TestBadMetadataDocuments(t *testing.T) {
	cases := map[string]func(id string) map[string]any{
		"another client_id": func(string) map[string]any { return assistant("https://elsewhere.example/client.json") },
		"no name": func(id string) map[string]any {
			d := assistant(id)
			delete(d, "client_name")
			return d
		},
		"an http redirect elsewhere": func(id string) map[string]any {
			d := assistant(id)
			d["redirect_uris"] = []string{"http://attacker.example/cb"}
			return d
		},
		"a custom scheme": func(id string) map[string]any {
			d := assistant(id)
			d["redirect_uris"] = []string{"javascript:alert(1)"}
			return d
		},
		"a client secret": func(id string) map[string]any {
			d := assistant(id)
			d["token_endpoint_auth_method"] = "client_secret_basic"
			return d
		},
	}
	for name, doc := range cases {
		t.Run(name, func(t *testing.T) {
			srv, id := metadataServer(t, doc, nil)
			e := newEnv(t, withMetadata(srv))
			q := url.Values{"client_id": {id}, "redirect_uri": {"http://localhost/cb"}, "response_type": {"code"},
				"code_challenge": {strings.Repeat("a", 43)}, "code_challenge_method": {"S256"}}
			res, err := http.Get(e.issuer + "/oauth/authorize?" + q.Encode())
			if err != nil {
				t.Fatal(err)
			}
			if page := read(t, res); res.StatusCode != http.StatusBadRequest || !strings.Contains(page, "not registered") {
				t.Errorf("%d %s", res.StatusCode, page)
			}
		})
	}

	// Off unless enabled.
	srv, id := metadataServer(t, assistant, nil)
	e := newEnv(t, func(o *oidc.Options) { o.MetadataClient = srv.Client() })
	res, _ := http.Get(e.issuer + "/oauth/authorize?" + url.Values{"client_id": {id}, "redirect_uri": {"http://localhost/cb"}, "response_type": {"code"}}.Encode())
	if page := read(t, res); res.StatusCode != http.StatusBadRequest {
		t.Errorf("metadata documents are on by default: %d %s", res.StatusCode, page)
	}
}

// A client that asks for no scopes still needs the user's consent: with
// none, "every scope was already approved" would be true for any app.
func TestConsentWithoutScopes(t *testing.T) {
	e := newEnv(t)
	q := url.Values{"client_id": {"rp"}, "redirect_uri": {e.clientURL + "/auth/tern/callback"}, "response_type": {"code"}, "state": {"st"}}
	b := browser()
	b.CheckRedirect = func(req *http.Request, _ []*http.Request) error {
		if req.URL.Host == strings.TrimPrefix(e.clientURL, "http://") {
			return http.ErrUseLastResponse
		}
		return nil
	}
	res, err := b.Get(e.issuer + "/oauth/authorize?" + q.Encode())
	if err != nil {
		t.Fatal(err)
	}
	if page := read(t, res); res.StatusCode != http.StatusOK || !strings.Contains(page, "Team wiki wants to access your account") {
		t.Fatalf("no consent asked: %d %s %s", res.StatusCode, res.Header.Get("Location"), page)
	}
}

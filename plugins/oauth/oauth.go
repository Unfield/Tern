// Package oauth adds "Sign in with GitHub, Google or any OpenID Connect
// provider" to a Tern app:
//
//	kit.WithPlugins(
//		sessions.New(secret),
//		oauth.New(oauth.Options{
//			Secret: secret,
//			Providers: []oauth.Provider{
//				oauth.GitHub(os.Getenv("GITHUB_CLIENT_ID"), os.Getenv("GITHUB_CLIENT_SECRET")),
//				oauth.Google(os.Getenv("GOOGLE_CLIENT_ID"), os.Getenv("GOOGLE_CLIENT_SECRET")),
//			},
//			OnLogin: func(ctx *kit.Ctx, id oauth.Identity) error {
//				user, err := users.FindOrCreate(ctx, id.Provider, id.Subject, id.Email)
//				if err != nil {
//					return err
//				}
//				return sessions.Get(ctx).Set("user", user.ID)
//			},
//		}),
//	)
//
// Link to /auth/github/login (optionally with ?next=/somewhere) to sign in.
// The provider sends the user back to /auth/github/callback, which must be
// registered as the redirect URL with the provider. The flow uses PKCE, a
// state parameter and, for OpenID Connect, a nonce, kept in a short-lived
// encrypted cookie; ID tokens are verified against the provider's keys.
package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/Unfield/Tern/internal/seal"
	"github.com/Unfield/Tern/kit"
)

// Identity is who signed in.
type Identity struct {
	// Provider is the provider's Name, such as "github".
	Provider string
	// Subject is the user's stable ID at the provider. Use Provider and
	// Subject together to find the user; emails can change.
	Subject       string
	Email         string
	EmailVerified bool
	Name          string
	Username      string // GitHub login, or preferred_username
	Picture       string
	// Claims holds the ID token's claims, or the user API's response.
	Claims map[string]any
	// Token is the provider's token, to call its APIs for the user.
	Token *oauth2.Token
}

// Provider is an OAuth 2.0 or OpenID Connect provider.
type Provider struct {
	// Name is the provider's path segment, as in /auth/<name>/login.
	Name string
	// Label is a display name, such as "GitHub".
	Label string
	// Config holds the client ID and secret, endpoints and scopes. The
	// redirect URL is set by the plugin.
	Config oauth2.Config
	// Issuer is the OpenID Connect issuer URL. With it, the endpoints are
	// discovered and the ID token is verified.
	Issuer string
	// UserInfo fetches the identity for providers without OpenID Connect.
	UserInfo func(ctx context.Context, client *http.Client, token *oauth2.Token) (Identity, error)
	// AuthParams are added to the authorization URL, such as
	// prompt=select_account.
	AuthParams map[string]string

	mu       sync.Mutex
	verifier *oidc.IDTokenVerifier
}

// OIDC returns a provider that supports OpenID Connect discovery, such as
// Keycloak, Auth0, Okta, Microsoft Entra ID, or a Tern app using the oidc
// plugin. The openid, email and profile scopes are requested unless scopes
// are given.
func OIDC(name, label, issuer, clientID, clientSecret string, scopes ...string) *Provider {
	if len(scopes) == 0 {
		scopes = []string{oidc.ScopeOpenID, "email", "profile"}
	}
	return &Provider{Name: name, Label: label, Issuer: issuer, Config: oauth2.Config{ClientID: clientID, ClientSecret: clientSecret, Scopes: scopes}}
}

// Google signs in with a Google account.
func Google(clientID, clientSecret string) *Provider {
	p := OIDC("google", "Google", "https://accounts.google.com", clientID, clientSecret)
	p.AuthParams = map[string]string{"prompt": "select_account"}
	return p
}

// GitHub signs in with a GitHub account. The identity's email is the
// user's primary verified email, if they have one.
func GitHub(clientID, clientSecret string) *Provider {
	return &Provider{
		Name:  "github",
		Label: "GitHub",
		Config: oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			Endpoint:     oauth2.Endpoint{AuthURL: "https://github.com/login/oauth/authorize", TokenURL: "https://github.com/login/oauth/access_token"},
			Scopes:       []string{"read:user", "user:email"},
		},
		UserInfo: githubUser("https://api.github.com"),
	}
}

func githubUser(api string) func(context.Context, *http.Client, *oauth2.Token) (Identity, error) {
	return func(ctx context.Context, client *http.Client, _ *oauth2.Token) (Identity, error) {
		var u struct {
			ID        int64  `json:"id"`
			Login     string `json:"login"`
			Name      string `json:"name"`
			Email     string `json:"email"`
			AvatarURL string `json:"avatar_url"`
		}
		claims := map[string]any{}
		if err := getJSON(ctx, client, api+"/user", &claims); err != nil {
			return Identity{}, err
		}
		b, _ := json.Marshal(claims)
		_ = json.Unmarshal(b, &u)
		id := Identity{Subject: fmt.Sprint(u.ID), Username: u.Login, Name: u.Name, Picture: u.AvatarURL, Claims: claims}
		var emails []struct {
			Email    string `json:"email"`
			Primary  bool   `json:"primary"`
			Verified bool   `json:"verified"`
		}
		if err := getJSON(ctx, client, api+"/user/emails", &emails); err == nil {
			for _, e := range emails {
				if e.Primary && e.Verified {
					id.Email, id.EmailVerified = e.Email, true
				}
			}
		}
		if id.Email == "" {
			id.Email = u.Email // public email, not known to be verified
		}
		if u.ID == 0 {
			return Identity{}, errors.New("oauth: GitHub returned no user")
		}
		return id, nil
	}
}

func getJSON(ctx context.Context, client *http.Client, u string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("oauth: %s answered HTTP %d", u, res.StatusCode)
	}
	return json.NewDecoder(res.Body).Decode(v)
}

// Options configure the plugin.
type Options struct {
	Providers []*Provider
	// Secret encrypts the cookie that carries the flow's state; at least
	// 32 bytes. The sessions plugin's secret can be reused.
	Secret string
	// OnLogin is called after a successful sign-in, to find or create the
	// user and sign them in. Returning an error, such as a kit.Redirect or
	// kit.Error(403, ...), stops the sign-in; otherwise the user is sent to
	// the next URL given to the login link, or "/".
	OnLogin func(ctx *kit.Ctx, id Identity) error
	// Path is where the plugin's routes are; the default is "/auth".
	Path string
	// URL is the app's public URL, such as "https://example.com", used to
	// build redirect URLs. The default is taken from the request, which is
	// right unless the app runs behind a proxy that changes the host.
	URL string
	// ErrorURL is where users go when a sign-in fails or is canceled, with
	// ?error=<code>. The default is "/login".
	ErrorURL string
	// HTTPClient is used for requests to providers.
	HTTPClient *http.Client
}

// Plugin is the oauth plugin.
type Plugin struct {
	log       *slog.Logger
	opts      Options
	box       *seal.Box
	providers map[string]*Provider
}

// New returns the plugin.
func New(opts Options) *Plugin {
	if opts.Path == "" {
		opts.Path = "/auth"
	}
	opts.Path = "/" + strings.Trim(opts.Path, "/")
	if opts.ErrorURL == "" {
		opts.ErrorURL = "/login"
	}
	if opts.HTTPClient == nil {
		opts.HTTPClient = &http.Client{Timeout: 15 * time.Second}
	}
	p := &Plugin{opts: opts, providers: map[string]*Provider{}}
	for _, pr := range opts.Providers {
		p.providers[pr.Name] = pr
	}
	return p
}

// Name implements kit.Plugin.
func (*Plugin) Name() string { return "oauth" }

// Setup implements kit.Plugin.
func (p *Plugin) Setup(s *kit.Setup) error {
	if p.opts.OnLogin == nil {
		return errors.New("oauth: Options.OnLogin is required")
	}
	box, err := seal.New(p.opts.Secret, "oauth-state")
	if err != nil {
		return fmt.Errorf("oauth: %w", err)
	}
	p.box = box
	p.log = s.Logger()
	for name, pr := range p.providers {
		if name == "" || strings.ContainsAny(name, "/?#") {
			return fmt.Errorf("oauth: invalid provider name %q", name)
		}
		if pr.Issuer == "" && (pr.UserInfo == nil || pr.Config.Endpoint.AuthURL == "") {
			return fmt.Errorf("oauth: provider %s needs an Issuer, or endpoints and UserInfo", name)
		}
	}
	s.Endpoint("GET "+p.opts.Path+"/{provider}/login", p.login)
	s.Endpoint("GET "+p.opts.Path+"/{provider}/callback", p.callback)
	return nil
}

// Providers lists the configured providers, for rendering sign-in buttons.
func (p *Plugin) Providers() []*Provider { return p.opts.Providers }

// flow is what the state cookie carries between login and callback.
type flow struct {
	State    string    `json:"s"`
	Verifier string    `json:"v"`
	Nonce    string    `json:"n,omitempty"`
	Next     string    `json:"x"`
	Expires  time.Time `json:"e"`
}

func (p *Plugin) cookieName(provider string) string { return "tern_oauth_" + provider }

func (p *Plugin) provider(ctx *kit.Ctx) (*Provider, error) {
	pr, ok := p.providers[ctx.Request.PathValue("provider")]
	if !ok {
		return nil, kit.NotFound()
	}
	if pr.Issuer != "" {
		pr.mu.Lock()
		defer pr.mu.Unlock()
		if pr.verifier == nil {
			// Discover on first use, and again after a failure.
			dctx, cancel := context.WithTimeout(oidc.ClientContext(context.WithoutCancel(ctx), p.opts.HTTPClient), 15*time.Second)
			defer cancel()
			op, err := oidc.NewProvider(dctx, pr.Issuer)
			if err != nil {
				return nil, fmt.Errorf("oauth: discovering %s: %w", pr.Issuer, err)
			}
			pr.Config.Endpoint = op.Endpoint()
			pr.verifier = op.Verifier(&oidc.Config{ClientID: pr.Config.ClientID})
		}
	}
	return pr, nil
}

func (p *Plugin) config(ctx *kit.Ctx, pr *Provider) *oauth2.Config {
	pr.mu.Lock()
	c := pr.Config
	pr.mu.Unlock()
	base := strings.TrimRight(p.opts.URL, "/")
	if base == "" {
		scheme := "https"
		if ctx.Request.TLS == nil && isLocal(ctx.Request.Host) {
			scheme = "http"
		}
		base = scheme + "://" + ctx.Request.Host
	}
	c.RedirectURL = base + ctx.Path(p.opts.Path+"/"+pr.Name+"/callback")
	return &c
}

func isLocal(host string) bool {
	h := host
	if i := strings.LastIndex(h, ":"); i >= 0 && !strings.HasSuffix(h, "]") {
		h = h[:i]
	}
	return h == "localhost" || h == "127.0.0.1" || h == "[::1]" || strings.HasSuffix(h, ".localhost")
}

// safeNext keeps redirects on this site.
func safeNext(next string) string {
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") || strings.HasPrefix(next, "/\\") {
		return "/"
	}
	return next
}

func (p *Plugin) login(ctx *kit.Ctx) error {
	pr, err := p.provider(ctx)
	if err != nil {
		return err
	}
	f := flow{
		State:    oauth2.GenerateVerifier(),
		Verifier: oauth2.GenerateVerifier(),
		Next:     safeNext(ctx.Request.URL.Query().Get("next")),
		Expires:  time.Now().Add(10 * time.Minute),
	}
	opts := []oauth2.AuthCodeOption{oauth2.S256ChallengeOption(f.Verifier)}
	if pr.Issuer != "" {
		f.Nonce = oauth2.GenerateVerifier()
		opts = append(opts, oidc.Nonce(f.Nonce))
	}
	for k, v := range pr.AuthParams {
		opts = append(opts, oauth2.SetAuthURLParam(k, v))
	}
	b, _ := json.Marshal(f)
	ctx.SetCookie(&http.Cookie{
		Name:     p.cookieName(pr.Name),
		Value:    p.box.Seal(b),
		Path:     ctx.Path(p.opts.Path + "/" + pr.Name),
		MaxAge:   600,
		HttpOnly: true,
		Secure:   ctx.Request.TLS != nil || !isLocal(ctx.Request.Host),
		// The provider redirects back with a top-level GET, which Lax allows.
		SameSite: http.SameSiteLaxMode,
	})
	return kit.Redirect(http.StatusSeeOther, p.config(ctx, pr).AuthCodeURL(f.State, opts...))
}

func (p *Plugin) fail(ctx *kit.Ctx, code string, err error) error {
	if err != nil {
		p.log.Warn("oauth: sign-in failed", "provider", ctx.Request.PathValue("provider"), "reason", code, "err", err)
	}
	u := p.opts.ErrorURL
	sep := "?"
	if strings.Contains(u, "?") {
		sep = "&"
	}
	return kit.Redirect(http.StatusSeeOther, u+sep+"error="+url.QueryEscape(code))
}

func (p *Plugin) callback(ctx *kit.Ctx) error {
	pr, err := p.provider(ctx)
	if err != nil {
		return err
	}
	q := ctx.Request.URL.Query()
	raw, ok := ctx.Cookie(p.cookieName(pr.Name))
	ctx.SetCookie(&http.Cookie{Name: p.cookieName(pr.Name), Path: ctx.Path(p.opts.Path + "/" + pr.Name), MaxAge: -1})
	if e := q.Get("error"); e != "" {
		return p.fail(ctx, "denied", fmt.Errorf("provider: %s %s", e, q.Get("error_description")))
	}
	if !ok {
		return p.fail(ctx, "expired", errors.New("no state cookie"))
	}
	plain, err := p.box.Open(raw)
	var f flow
	if err != nil || json.Unmarshal(plain, &f) != nil || time.Now().After(f.Expires) {
		return p.fail(ctx, "expired", errors.New("invalid or expired state cookie"))
	}
	if q.Get("state") == "" || q.Get("state") != f.State {
		return p.fail(ctx, "state", errors.New("state mismatch"))
	}
	octx := context.WithValue(context.WithoutCancel(ctx), oauth2.HTTPClient, p.opts.HTTPClient)
	tctx, cancel := context.WithTimeout(octx, 20*time.Second)
	defer cancel()
	conf := p.config(ctx, pr)
	token, err := conf.Exchange(tctx, q.Get("code"), oauth2.VerifierOption(f.Verifier))
	if err != nil {
		return p.fail(ctx, "exchange", err)
	}

	var id Identity
	if pr.Issuer != "" {
		rawID, _ := token.Extra("id_token").(string)
		if rawID == "" {
			return p.fail(ctx, "id_token", errors.New("no id_token in the token response"))
		}
		idToken, err := pr.verifier.Verify(oidc.ClientContext(tctx, p.opts.HTTPClient), rawID)
		if err != nil {
			return p.fail(ctx, "id_token", err)
		}
		if idToken.Nonce != f.Nonce {
			return p.fail(ctx, "id_token", errors.New("nonce mismatch"))
		}
		claims := map[string]any{}
		if err := idToken.Claims(&claims); err != nil {
			return p.fail(ctx, "id_token", err)
		}
		id = fromClaims(claims)
		id.Subject = idToken.Subject
	} else {
		if id, err = pr.UserInfo(tctx, conf.Client(tctx, token), token); err != nil {
			return p.fail(ctx, "userinfo", err)
		}
	}
	id.Provider, id.Token = pr.Name, token
	if err := p.opts.OnLogin(ctx, id); err != nil {
		return err
	}
	return kit.Redirect(http.StatusSeeOther, f.Next)
}

func fromClaims(c map[string]any) Identity {
	s := func(k string) string { v, _ := c[k].(string); return v }
	verified, _ := c["email_verified"].(bool)
	return Identity{Email: s("email"), EmailVerified: verified, Name: s("name"), Username: s("preferred_username"), Picture: s("picture"), Claims: c}
}

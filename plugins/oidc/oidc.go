// Package oidc makes a Tern app an OAuth 2.0 authorization server and
// OpenID Connect provider: other apps (or your own frontends, mobile apps
// and services) sign users in with it, and your APIs accept its tokens.
//
//	provider := oidc.New(oidc.Options{
//		Issuer: "https://auth.example.com",
//		Clients: []oidc.Client{{
//			ID: "wiki", Secret: os.Getenv("WIKI_CLIENT_SECRET"), Name: "Team wiki",
//			RedirectURIs: []string{"https://wiki.example.com/auth/example/callback"},
//		}},
//		Store: store, // oidc.NewSQLStore(db)
//		Authenticate: func(ctx *kit.Ctx) (string, error) {
//			return sessions.Get(ctx).GetString("user"), nil // "" when signed out
//		},
//		LoginURL: "/login",
//		Claims: func(ctx context.Context, subject string, scopes []string) (map[string]any, error) {
//			u, err := users.Get(ctx, subject)
//			return map[string]any{"email": u.Email, "email_verified": true, "name": u.Name}, err
//		},
//	})
//	app, err := ternApp(kit.WithPlugins(sessions.New(secret), provider))
//
// It serves discovery (/.well-known/openid-configuration), keys
// (/oauth/jwks), and the authorize, token, userinfo, revocation and
// introspection endpoints under /oauth. Supported: the authorization code
// flow with PKCE (required for public clients), refresh tokens with
// rotation and reuse detection, and the client credentials grant. Access
// and ID tokens are RS256-signed JWTs.
package oidc

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/go-jose/go-jose/v4"

	"github.com/Unfield/Tern/kit"
)

// Client is an application allowed to use the provider.
type Client struct {
	ID string
	// Secret authenticates confidential clients (server-side apps). Leave
	// it empty and set Public for apps that can't keep a secret, such as
	// single-page and mobile apps; they must use PKCE.
	Secret string
	Public bool
	// Name is shown on the consent screen.
	Name string
	// RedirectURIs are the exact URLs codes may be sent to.
	RedirectURIs []string
	// Trusted clients, such as your own apps, skip the consent screen.
	Trusted bool
	// GrantTypes the client may use; the default is authorization_code and
	// refresh_token. Add client_credentials for service accounts.
	GrantTypes []string
	// Scopes the client may request; the default is every supported scope.
	Scopes []string

	// fromMetadata marks clients described by a metadata document.
	fromMetadata bool
}

func (c *Client) allowsGrant(g string) bool {
	if len(c.GrantTypes) == 0 {
		return g == "authorization_code" || g == "refresh_token"
	}
	return slices.Contains(c.GrantTypes, g)
}

// Options configure the provider.
type Options struct {
	// Issuer is the provider's public URL, such as
	// "https://auth.example.com", without a trailing slash. Tokens carry
	// it, and clients check it.
	Issuer  string
	Clients []Client
	// Authenticate returns the signed-in user's subject (a stable ID), or
	// "" when nobody is signed in.
	Authenticate func(ctx *kit.Ctx) (string, error)
	// LoginURL is where signed-out users go, with ?next=<the authorize
	// URL> to come back to.
	LoginURL string
	// ConsentURL is a page of the app that asks users to approve a client,
	// such as one with Tern UI's ConsentScreen, instead of the plugin's
	// own page. It gets the authorization request as its query; its load
	// calls ConsentRequest for what to show.
	ConsentURL string
	// Claims returns the user's claims for ID tokens and userinfo, such as
	// email, email_verified, name and picture. scopes lets you leave out
	// what wasn't asked for. It may be nil.
	Claims func(ctx context.Context, subject string, scopes []string) (map[string]any, error)
	// Store keeps codes, refresh tokens, consents and the signing key. The
	// default is a MemoryStore, which forgets everything on restart.
	Store Store
	// Scopes the provider supports besides openid, profile, email and
	// offline_access, such as "api:read".
	Scopes []string
	// Path is where the endpoints are; the default is "/oauth".
	Path string
	// Lifetimes; the defaults are an hour for access and ID tokens, 30
	// days for refresh tokens and a minute for codes.
	AccessTokenTTL, IDTokenTTL, RefreshTokenTTL, CodeTTL time.Duration
	// SigningKey is an RSA key in PEM. Without it, a key is created and
	// kept in the Store.
	SigningKey []byte

	// ClientMetadataDocuments lets clients that aren't in Clients sign
	// users in with an https URL as their client_id, which the provider
	// fetches the client's metadata from (Client ID Metadata Documents).
	// That's how AI assistants register with an app's MCP server. Such
	// clients are public, must use PKCE, and always get the consent screen.
	ClientMetadataDocuments bool
	// MetadataClient fetches metadata documents. The default allows only
	// https to public addresses on port 443, without redirects; replace it
	// only in tests.
	MetadataClient *http.Client
	// Resources are the resource indicators (RFC 8707) clients may ask
	// tokens for, such as "https://example.com/mcp". Tokens for a resource
	// carry it as their audience. URLs on the issuer's own origin are always
	// allowed.
	Resources []string
}

// Plugin is the provider.
type Plugin struct {
	opts    Options
	clients map[string]*Client
	scopes  []string
	key     *rsa.PrivateKey
	kid     string
	log     *slog.Logger

	metadata       metadataClients
	metadataOnce   sync.Once
	metadataClient *http.Client
}

// New returns the provider.
func New(opts Options) *Plugin {
	if opts.Path == "" {
		opts.Path = "/oauth"
	}
	opts.Path = "/" + strings.Trim(opts.Path, "/")
	opts.Issuer = strings.TrimRight(opts.Issuer, "/")
	if opts.Store == nil {
		opts.Store = NewMemoryStore()
	}
	def := func(d *time.Duration, v time.Duration) {
		if *d <= 0 {
			*d = v
		}
	}
	def(&opts.AccessTokenTTL, time.Hour)
	def(&opts.IDTokenTTL, time.Hour)
	def(&opts.RefreshTokenTTL, 30*24*time.Hour)
	def(&opts.CodeTTL, time.Minute)
	p := &Plugin{opts: opts, clients: map[string]*Client{}}
	p.scopes = append([]string{"openid", "profile", "email", "offline_access"}, opts.Scopes...)
	for i := range opts.Clients {
		c := &opts.Clients[i]
		p.clients[c.ID] = c
	}
	return p
}

// Name implements kit.Plugin.
func (*Plugin) Name() string { return "oidc" }

// Setup implements kit.Plugin.
func (p *Plugin) Setup(s *kit.Setup) error {
	p.log = s.Logger()
	u, err := url.Parse(p.opts.Issuer)
	secure := u != nil && (u.Scheme == "https" || (u.Scheme == "http" && isLocalHost(u.Hostname())))
	if err != nil || !secure || u.Host == "" || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("oidc: the issuer must be an https URL (http only on localhost), got %q", p.opts.Issuer)
	}
	if p.opts.Authenticate == nil || p.opts.LoginURL == "" {
		return errors.New("oidc: Options.Authenticate and Options.LoginURL are required")
	}
	for _, c := range p.clients {
		if c.ID == "" {
			return errors.New("oidc: a client has no ID")
		}
		if !c.Public && len(c.Secret) < 16 {
			return fmt.Errorf("oidc: client %s needs a secret of at least 16 characters, or Public: true", c.ID)
		}
		if c.Public && c.Secret != "" {
			return fmt.Errorf("oidc: public client %s must not have a secret", c.ID)
		}
		if c.allowsGrant("authorization_code") && len(c.RedirectURIs) == 0 {
			return fmt.Errorf("oidc: client %s has no redirect URIs", c.ID)
		}
		for _, r := range c.RedirectURIs {
			ru, err := url.Parse(r)
			if err != nil || ru.Fragment != "" || ru.Scheme == "" {
				return fmt.Errorf("oidc: client %s: invalid redirect URI %q", c.ID, r)
			}
		}
	}
	if err := p.loadKey(context.Background()); err != nil {
		return err
	}

	s.Handle("GET /.well-known/openid-configuration", http.HandlerFunc(p.discovery))
	// The same document as OAuth authorization server metadata (RFC 8414),
	// which OAuth clients such as MCP's look for first.
	s.Handle("GET /.well-known/oauth-authorization-server", http.HandlerFunc(p.discovery))
	if u.Path != "" && u.Path != "/" {
		// An issuer with a path publishes discovery below it, and RFC 8414
		// metadata with the path after the well-known part.
		s.Handle("GET "+u.Path+"/.well-known/openid-configuration", http.HandlerFunc(p.discovery))
		s.Handle("GET /.well-known/openid-configuration"+u.Path, http.HandlerFunc(p.discovery))
		s.Handle("GET /.well-known/oauth-authorization-server"+u.Path, http.HandlerFunc(p.discovery))
	}
	s.Handle("GET "+p.opts.Path+"/jwks", http.HandlerFunc(p.jwks))
	s.Endpoint(p.opts.Path+"/authorize", p.authorize)
	s.Handle(p.opts.Path+"/token", postOnly(p.token))
	s.Handle(p.opts.Path+"/userinfo", http.HandlerFunc(p.userinfo))
	s.Handle(p.opts.Path+"/revoke", postOnly(p.revoke))
	s.Handle("POST "+p.opts.Path+"/introspect", http.HandlerFunc(p.introspect))
	// These authenticate with client credentials or bearer tokens, never
	// cookies, and browser apps on other sites call them; cross-origin
	// protection is for cookie-authenticated forms.
	for _, e := range []string{"token", "userinfo", "revoke", "introspect"} {
		s.AllowCrossOrigin(p.opts.Path + "/" + e)
	}
	return nil
}

// postOnly allows POST, and OPTIONS for CORS preflights.
func postOnly(h http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost && r.Method != http.MethodOptions {
			w.Header().Set("Allow", "POST, OPTIONS")
			tokenError(w, http.StatusMethodNotAllowed, "invalid_request", "use POST")
			return
		}
		h(w, r)
	})
}

func isLocalHost(h string) bool {
	return h == "localhost" || h == "127.0.0.1" || h == "::1" || strings.HasSuffix(h, ".localhost")
}

func (p *Plugin) loadKey(ctx context.Context) error {
	raw := p.opts.SigningKey
	if raw == nil {
		var err error
		raw, err = p.opts.Store.SigningKey(ctx)
		if errors.Is(err, ErrNotFound) {
			key, err := rsa.GenerateKey(rand.Reader, 2048)
			if err != nil {
				return err
			}
			raw = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: must(x509.MarshalPKCS8PrivateKey(key))})
			if err := p.opts.Store.SaveSigningKey(ctx, raw); err != nil {
				return fmt.Errorf("oidc: saving the signing key: %w", err)
			}
		} else if err != nil {
			return fmt.Errorf("oidc: loading the signing key: %w", err)
		}
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return errors.New("oidc: the signing key is not PEM")
	}
	var key any
	var err error
	if block.Type == "RSA PRIVATE KEY" {
		key, err = x509.ParsePKCS1PrivateKey(block.Bytes)
	} else {
		key, err = x509.ParsePKCS8PrivateKey(block.Bytes)
	}
	rsaKey, ok := key.(*rsa.PrivateKey)
	if err != nil || !ok {
		return errors.New("oidc: the signing key must be an RSA private key")
	}
	p.key = rsaKey
	thumb := sha256.Sum256(must(x509.MarshalPKIXPublicKey(&rsaKey.PublicKey)))
	p.kid = hex.EncodeToString(thumb[:8])
	return nil
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func (p *Plugin) endpoint(name string) string { return p.opts.Issuer + p.opts.Path + "/" + name }

func (p *Plugin) discovery(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	doc := map[string]any{
		"issuer":                                         p.opts.Issuer,
		"authorization_endpoint":                         p.endpoint("authorize"),
		"token_endpoint":                                 p.endpoint("token"),
		"userinfo_endpoint":                              p.endpoint("userinfo"),
		"jwks_uri":                                       p.endpoint("jwks"),
		"revocation_endpoint":                            p.endpoint("revoke"),
		"introspection_endpoint":                         p.endpoint("introspect"),
		"response_types_supported":                       []string{"code"},
		"response_modes_supported":                       []string{"query"},
		"grant_types_supported":                          []string{"authorization_code", "refresh_token", "client_credentials"},
		"subject_types_supported":                        []string{"public"},
		"id_token_signing_alg_values_supported":          []string{"RS256"},
		"scopes_supported":                               p.scopes,
		"token_endpoint_auth_methods_supported":          []string{"client_secret_basic", "client_secret_post", "none"},
		"code_challenge_methods_supported":               []string{"S256"},
		"claims_supported":                               []string{"sub", "iss", "aud", "exp", "iat", "auth_time", "nonce", "name", "email", "email_verified", "picture", "preferred_username"},
		"authorization_response_iss_parameter_supported": true,
	}
	if p.opts.ClientMetadataDocuments {
		doc["client_id_metadata_document_supported"] = true
	}
	writeJSON(w, http.StatusOK, doc)
}

// resourceAllowed reports whether tokens may be issued for resource: an
// absolute URL without a fragment, on the issuer's origin or listed in
// Options.Resources.
func (p *Plugin) resourceAllowed(resource string) bool {
	u, err := url.Parse(resource)
	if err != nil || !u.IsAbs() || u.Fragment != "" || u.Host == "" {
		return false
	}
	if slices.Contains(p.opts.Resources, resource) || slices.Contains(p.opts.Resources, strings.TrimRight(resource, "/")) {
		return true
	}
	iss, err := url.Parse(p.opts.Issuer)
	return err == nil && strings.EqualFold(iss.Scheme, u.Scheme) && strings.EqualFold(iss.Host, u.Host)
}

func (p *Plugin) jwks(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "public, max-age=300")
	writeJSON(w, http.StatusOK, jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &p.key.PublicKey, KeyID: p.kid, Algorithm: "RS256", Use: "sig"}}})
}

// randomToken returns 32 random bytes as URL-safe text.
func randomToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// hash is how codes and refresh tokens are stored.
func hash(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

func equal(a, b string) bool { return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1 }

// allowedScopes keeps the requested scopes the client may have.
func (p *Plugin) allowedScopes(c *Client, requested []string) []string {
	out := []string{}
	for _, s := range requested {
		if !slices.Contains(p.scopes, s) || (len(c.Scopes) > 0 && !slices.Contains(c.Scopes, s)) || slices.Contains(out, s) {
			continue
		}
		out = append(out, s)
	}
	return out
}

// Issuer is the provider's public URL, as Options.Issuer set it.
func (p *Plugin) Issuer() string { return p.opts.Issuer }

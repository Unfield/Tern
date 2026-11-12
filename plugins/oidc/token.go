package oidc

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"

	"github.com/Unfield/Tern/kit"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// tokenError writes an OAuth error response (RFC 6749 §5.2).
func tokenError(w http.ResponseWriter, status int, code, desc string) {
	w.Header().Set("Cache-Control", "no-store")
	if status == http.StatusUnauthorized {
		w.Header().Set("WWW-Authenticate", `Basic realm="token"`)
	}
	writeJSON(w, status, map[string]string{"error": code, "error_description": desc})
}

// cors lets browser apps on other origins call an endpoint without cookies.
func cors(w http.ResponseWriter, r *http.Request) bool {
	if r.Header.Get("Origin") == "" {
		return false
	}
	h := w.Header()
	h.Set("Access-Control-Allow-Origin", "*")
	h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
	h.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	h.Set("Access-Control-Max-Age", "600")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return true
	}
	return false
}

// client authenticates the client of a token request: with HTTP Basic or
// form credentials for confidential clients, or client_id alone for public
// ones.
func (p *Plugin) client(r *http.Request) (*Client, error) {
	id, secret, basic := r.BasicAuth()
	if !basic {
		id, secret = r.PostForm.Get("client_id"), r.PostForm.Get("client_secret")
	}
	c, err := p.lookupClient(r.Context(), id)
	if err != nil {
		return nil, errors.New("unknown client")
	}
	if c.Public {
		if secret != "" {
			return nil, errors.New("public clients have no secret")
		}
		return c, nil
	}
	if secret == "" || !equal(secret, c.Secret) {
		return nil, errors.New("wrong client secret")
	}
	return c, nil
}

func (p *Plugin) token(w http.ResponseWriter, r *http.Request) {
	if cors(w, r) {
		return
	}
	if err := r.ParseForm(); err != nil {
		tokenError(w, http.StatusBadRequest, "invalid_request", "malformed form body")
		return
	}
	c, err := p.client(r)
	if err != nil {
		tokenError(w, http.StatusUnauthorized, "invalid_client", err.Error())
		return
	}
	grant := r.PostForm.Get("grant_type")
	if !c.allowsGrant(grant) {
		tokenError(w, http.StatusBadRequest, "unauthorized_client", "the client may not use grant_type "+grant)
		return
	}
	ctx := r.Context()
	switch grant {
	case "authorization_code":
		p.codeGrant(ctx, w, r, c)
	case "refresh_token":
		p.refreshGrant(ctx, w, r, c)
	case "client_credentials":
		if c.Public {
			tokenError(w, http.StatusBadRequest, "unauthorized_client", "public clients can't use client_credentials")
			return
		}
		scopes := p.allowedScopes(c, strings.Fields(r.PostForm.Get("scope")))
		scopes = slices.DeleteFunc(scopes, func(s string) bool { return s == "openid" || s == "offline_access" })
		resource := r.PostForm.Get("resource")
		if resource != "" && !p.resourceAllowed(resource) {
			tokenError(w, http.StatusBadRequest, "invalid_target", "tokens can't be issued for that resource")
			return
		}
		p.issue(ctx, w, c, c.ID, scopes, time.Now(), "", "", resource)
	default:
		tokenError(w, http.StatusBadRequest, "unsupported_grant_type", "unsupported grant_type")
	}
}

func (p *Plugin) codeGrant(ctx context.Context, w http.ResponseWriter, r *http.Request, c *Client) {
	code, err := p.opts.Store.TakeCode(ctx, hash(r.PostForm.Get("code")))
	if err != nil {
		tokenError(w, http.StatusBadRequest, "invalid_grant", "the code is invalid, expired or used")
		return
	}
	if code.ClientID != c.ID || code.RedirectURI != r.PostForm.Get("redirect_uri") {
		tokenError(w, http.StatusBadRequest, "invalid_grant", "the code was issued to another client or redirect URI")
		return
	}
	// The token request names the same resource as the authorization
	// request, or none (RFC 8707 §2.2).
	if res := r.PostForm.Get("resource"); res != "" && res != code.Resource {
		tokenError(w, http.StatusBadRequest, "invalid_target", "the resource doesn't match the authorization request")
		return
	}
	verifier := r.PostForm.Get("code_verifier")
	switch {
	case code.CodeChallenge != "":
		sum := sha256.Sum256([]byte(verifier))
		if verifier == "" || !equal(base64.RawURLEncoding.EncodeToString(sum[:]), code.CodeChallenge) {
			tokenError(w, http.StatusBadRequest, "invalid_grant", "the code_verifier doesn't match")
			return
		}
	case verifier != "":
		tokenError(w, http.StatusBadRequest, "invalid_grant", "a code_verifier was sent but no challenge was")
		return
	}
	refresh := ""
	if slices.Contains(code.Scopes, "offline_access") || !slices.Contains(code.Scopes, "openid") {
		// OpenID Connect issues refresh tokens for offline_access; plain
		// OAuth clients get one with every code.
		refresh = randomToken()
		if err := p.opts.Store.SaveRefresh(ctx, hash(refresh), RefreshToken{
			ClientID: c.ID, Subject: code.Subject, Scopes: code.Scopes, Family: randomToken(), Resource: code.Resource,
			AuthTime: code.AuthTime, Expires: time.Now().Add(p.opts.RefreshTokenTTL),
		}); err != nil {
			p.serverError(w, err)
			return
		}
	}
	p.issue(ctx, w, c, code.Subject, code.Scopes, code.AuthTime, code.Nonce, refresh, code.Resource)
}

func (p *Plugin) refreshGrant(ctx context.Context, w http.ResponseWriter, r *http.Request, c *Client) {
	h := hash(r.PostForm.Get("refresh_token"))
	t, err := p.opts.Store.GetRefresh(ctx, h)
	if err != nil || t.ClientID != c.ID {
		tokenError(w, http.StatusBadRequest, "invalid_grant", "the refresh token is invalid or expired")
		return
	}
	fresh, err := p.opts.Store.UseRefresh(ctx, h)
	if err != nil {
		p.serverError(w, err)
		return
	}
	if !fresh {
		// A refresh token was used twice: it may have been stolen. Revoke
		// every token descended from the same sign-in (OAuth 2.0 Security
		// Best Current Practice §4.14).
		_ = p.opts.Store.RevokeFamily(ctx, t.Family)
		p.log.Warn("oidc: a refresh token was reused; revoked its family", "client", c.ID, "subject", t.Subject)
		tokenError(w, http.StatusBadRequest, "invalid_grant", "the refresh token was already used")
		return
	}
	if res := r.PostForm.Get("resource"); res != "" && res != t.Resource {
		tokenError(w, http.StatusBadRequest, "invalid_target", "the resource doesn't match the authorization request")
		return
	}
	scopes := t.Scopes
	if requested := strings.Fields(r.PostForm.Get("scope")); len(requested) > 0 {
		if !containsAll(t.Scopes, requested) {
			tokenError(w, http.StatusBadRequest, "invalid_scope", "a refresh can't add scopes")
			return
		}
		scopes = requested
	}
	next := randomToken()
	if err := p.opts.Store.SaveRefresh(ctx, hash(next), RefreshToken{
		ClientID: c.ID, Subject: t.Subject, Scopes: t.Scopes, Family: t.Family, Resource: t.Resource, AuthTime: t.AuthTime, Expires: t.Expires,
	}); err != nil {
		p.serverError(w, err)
		return
	}
	p.issue(ctx, w, c, t.Subject, scopes, t.AuthTime, "", next, t.Resource)
}

func (p *Plugin) serverError(w http.ResponseWriter, err error) {
	p.log.Error("oidc: token endpoint", "err", err)
	tokenError(w, http.StatusInternalServerError, "server_error", "internal error")
}

// accessClaims are an access token's claims (RFC 9068).
type accessClaims struct {
	jwt.Claims
	ClientID string `json:"client_id"`
	Scope    string `json:"scope,omitempty"`
	AuthTime int64  `json:"auth_time,omitempty"`
}

func (p *Plugin) sign(claims any, typ string) (string, error) {
	opts := (&jose.SignerOptions{}).WithType(jose.ContentType(typ))
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: jose.JSONWebKey{Key: p.key, KeyID: p.kid, Algorithm: "RS256", Use: "sig"}}, opts)
	if err != nil {
		return "", err
	}
	return jwt.Signed(signer).Claims(claims).Serialize()
}

// issue writes a token response. An access token for a resource has it as
// its audience; otherwise the audience is the client.
func (p *Plugin) issue(ctx context.Context, w http.ResponseWriter, c *Client, subject string, scopes []string, authTime time.Time, nonce, refresh, resource string) {
	now := time.Now()
	aud := jwt.Audience{c.ID}
	if resource != "" {
		aud = jwt.Audience{resource}
	}
	at := accessClaims{
		Claims: jwt.Claims{
			Issuer: p.opts.Issuer, Subject: subject, Audience: aud,
			IssuedAt: jwt.NewNumericDate(now), NotBefore: jwt.NewNumericDate(now),
			Expiry: jwt.NewNumericDate(now.Add(p.opts.AccessTokenTTL)), ID: randomToken()[:22],
		},
		ClientID: c.ID, Scope: strings.Join(scopes, " "), AuthTime: authTime.Unix(),
	}
	access, err := p.sign(at, "at+jwt")
	if err != nil {
		p.serverError(w, err)
		return
	}
	res := map[string]any{"access_token": access, "token_type": "Bearer", "expires_in": int(p.opts.AccessTokenTTL.Seconds()), "scope": strings.Join(scopes, " ")}
	if refresh != "" {
		res["refresh_token"] = refresh
	}
	if slices.Contains(scopes, "openid") {
		claims := map[string]any{}
		if p.opts.Claims != nil {
			if claims, err = p.opts.Claims(ctx, subject, scopes); err != nil {
				p.serverError(w, err)
				return
			}
			claims = filterClaims(claims, scopes)
		}
		sum := sha256.Sum256([]byte(access))
		for k, v := range map[string]any{
			"iss": p.opts.Issuer, "sub": subject, "aud": c.ID, "azp": c.ID,
			"iat": now.Unix(), "exp": now.Add(p.opts.IDTokenTTL).Unix(), "auth_time": authTime.Unix(),
			"at_hash": base64.RawURLEncoding.EncodeToString(sum[:16]),
		} {
			claims[k] = v
		}
		if nonce != "" {
			claims["nonce"] = nonce
		}
		id, err := p.sign(claims, "JWT")
		if err != nil {
			p.serverError(w, err)
			return
		}
		res["id_token"] = id
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	writeJSON(w, http.StatusOK, res)
}

// filterClaims keeps the standard claims the scopes cover, plus any
// non-standard claims the app returned.
func filterClaims(claims map[string]any, scopes []string) map[string]any {
	profile := []string{"name", "family_name", "given_name", "middle_name", "nickname", "preferred_username", "profile", "picture", "website", "gender", "birthdate", "zoneinfo", "locale", "updated_at"}
	email := []string{"email", "email_verified"}
	out := map[string]any{}
	for k, v := range claims {
		switch {
		case slices.Contains(profile, k) && !slices.Contains(scopes, "profile"):
		case slices.Contains(email, k) && !slices.Contains(scopes, "email"):
		case k == "iss" || k == "sub" || k == "aud" || k == "exp" || k == "iat" || k == "nonce":
		default:
			out[k] = v
		}
	}
	return out
}

// AccessToken is a verified access token.
type AccessToken struct {
	Subject  string
	ClientID string
	Scopes   []string
	Expires  time.Time
	ID       string
	// Audience is who the token is for: the resource it was issued for
	// (RFC 8707), or the client.
	Audience []string
}

// For reports whether the token was issued for resource, such as an MCP
// server's URL. Resource servers must check it, so a token issued to one
// of them can't be replayed against another.
func (t *AccessToken) For(resource string) bool {
	return slices.Contains(t.Audience, resource) || slices.Contains(t.Audience, strings.TrimRight(resource, "/"))
}

// HasScope reports whether the token was granted scope.
func (t *AccessToken) HasScope(scope string) bool { return slices.Contains(t.Scopes, scope) }

// ErrInvalidToken is returned for missing, malformed, expired or revoked
// tokens.
var ErrInvalidToken = errors.New("oidc: invalid access token")

// Verify checks an access token issued by this provider.
func (p *Plugin) Verify(ctx context.Context, raw string) (*AccessToken, error) {
	tok, err := jwt.ParseSigned(raw, []jose.SignatureAlgorithm{jose.RS256})
	if err != nil || len(tok.Headers) != 1 || tok.Headers[0].ExtraHeaders["typ"] != "at+jwt" {
		return nil, ErrInvalidToken
	}
	var c accessClaims
	if err := tok.Claims(&p.key.PublicKey, &c); err != nil {
		return nil, ErrInvalidToken
	}
	if err := c.ValidateWithLeeway(jwt.Expected{Issuer: p.opts.Issuer, Time: time.Now()}, 30*time.Second); err != nil {
		return nil, ErrInvalidToken
	}
	if revoked, err := p.opts.Store.AccessRevoked(ctx, c.ID); err != nil {
		return nil, err
	} else if revoked {
		return nil, ErrInvalidToken
	}
	return &AccessToken{Subject: c.Subject, ClientID: c.ClientID, Scopes: strings.Fields(c.Scope), Expires: c.Expiry.Time(), ID: c.ID, Audience: c.Audience}, nil
}

// Bearer authenticates an API request by its "Authorization: Bearer"
// access token, and checks it has every scope given. It answers 401 or 403
// errors with the WWW-Authenticate header clients expect (RFC 6750):
//
//	func GET(ctx *kit.Ctx) error {
//		token, err := provider.Bearer(ctx, "api:read")
//		if err != nil {
//			return err
//		}
//		return ctx.JSON(200, projectsOf(token.Subject))
//	}
func (p *Plugin) Bearer(ctx *kit.Ctx, scopes ...string) (*AccessToken, error) {
	raw, ok := strings.CutPrefix(ctx.Request.Header.Get("Authorization"), "Bearer ")
	if !ok || raw == "" {
		ctx.SetHeader("WWW-Authenticate", `Bearer realm="api"`)
		return nil, kit.Error(http.StatusUnauthorized, "an access token is required")
	}
	t, err := p.Verify(ctx, strings.TrimSpace(raw))
	if errors.Is(err, ErrInvalidToken) {
		ctx.SetHeader("WWW-Authenticate", `Bearer realm="api", error="invalid_token"`)
		return nil, kit.Error(http.StatusUnauthorized, "the access token is invalid or expired")
	}
	if err != nil {
		return nil, err
	}
	for _, s := range scopes {
		if !t.HasScope(s) {
			ctx.SetHeader("WWW-Authenticate", fmt.Sprintf(`Bearer realm="api", error="insufficient_scope", scope=%q`, strings.Join(scopes, " ")))
			return nil, kit.Error(http.StatusForbidden, "the access token lacks the scope "+s)
		}
	}
	return t, nil
}

func (p *Plugin) userinfo(w http.ResponseWriter, r *http.Request) {
	if cors(w, r) {
		return
	}
	raw, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok {
		w.Header().Set("WWW-Authenticate", `Bearer realm="userinfo"`)
		tokenError(w, http.StatusUnauthorized, "invalid_token", "an access token is required")
		return
	}
	t, err := p.Verify(r.Context(), strings.TrimSpace(raw))
	if err != nil || !t.HasScope("openid") {
		w.Header().Set("WWW-Authenticate", `Bearer realm="userinfo", error="invalid_token"`)
		tokenError(w, http.StatusUnauthorized, "invalid_token", "the access token is invalid or lacks the openid scope")
		return
	}
	claims := map[string]any{}
	if p.opts.Claims != nil {
		if claims, err = p.opts.Claims(r.Context(), t.Subject, t.Scopes); err != nil {
			p.serverError(w, err)
			return
		}
		claims = filterClaims(claims, t.Scopes)
	}
	claims["sub"] = t.Subject
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, claims)
}

// revoke implements RFC 7009: refresh tokens are deleted, access tokens
// are remembered as revoked until they expire.
func (p *Plugin) revoke(w http.ResponseWriter, r *http.Request) {
	if cors(w, r) {
		return
	}
	if err := r.ParseForm(); err != nil {
		tokenError(w, http.StatusBadRequest, "invalid_request", "malformed form body")
		return
	}
	c, err := p.client(r)
	if err != nil {
		tokenError(w, http.StatusUnauthorized, "invalid_client", err.Error())
		return
	}
	token := r.PostForm.Get("token")
	ctx := r.Context()
	if t, err := p.opts.Store.GetRefresh(ctx, hash(token)); err == nil && t.ClientID == c.ID {
		_ = p.opts.Store.RevokeFamily(ctx, t.Family)
	} else if at, err := p.Verify(ctx, token); err == nil && at.ClientID == c.ID {
		_ = p.opts.Store.RevokeAccess(ctx, at.ID, at.Expires)
	}
	// Unknown tokens are not an error (RFC 7009 §2.2).
	w.WriteHeader(http.StatusOK)
}

// introspect implements RFC 7662 for confidential clients.
func (p *Plugin) introspect(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		tokenError(w, http.StatusBadRequest, "invalid_request", "malformed form body")
		return
	}
	c, err := p.client(r)
	if err != nil || c.Public {
		tokenError(w, http.StatusUnauthorized, "invalid_client", "introspection needs a confidential client")
		return
	}
	token := r.PostForm.Get("token")
	ctx := r.Context()
	if at, err := p.Verify(ctx, token); err == nil {
		writeJSON(w, http.StatusOK, map[string]any{"active": true, "token_type": "access_token", "sub": at.Subject, "client_id": at.ClientID, "aud": at.Audience, "scope": strings.Join(at.Scopes, " "), "exp": at.Expires.Unix(), "iss": p.opts.Issuer})
		return
	}
	if t, err := p.opts.Store.GetRefresh(ctx, hash(token)); err == nil && !t.Used {
		writeJSON(w, http.StatusOK, map[string]any{"active": true, "token_type": "refresh_token", "sub": t.Subject, "client_id": t.ClientID, "scope": strings.Join(t.Scopes, " "), "exp": t.Expires.Unix(), "iss": p.opts.Issuer})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"active": false})
}

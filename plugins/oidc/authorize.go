package oidc

import (
	"html/template"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/Unfield/Tern/kit"
)

// authorize handles the authorization request (RFC 6749 §4.1, OpenID
// Connect Core §3.1.2): it checks the client, signs the user in through
// the app, asks for consent, and redirects back with a code.
func (p *Plugin) authorize(ctx *kit.Ctx) error {
	r := ctx.Request
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		return kit.Error(http.StatusMethodNotAllowed, "GET or POST")
	}
	var q url.Values
	if r.Method == http.MethodPost {
		form, err := ctx.Form()
		if err != nil {
			return err
		}
		q = form
	} else {
		q = r.URL.Query()
	}

	// Until the client and redirect URI are known to be valid, errors are
	// shown here rather than redirected, so the endpoint can't be used to
	// redirect users anywhere.
	client, err := p.lookupClient(ctx, q.Get("client_id"))
	if err != nil {
		return errorPage(ctx, "This app is not registered.")
	}
	redirect := q.Get("redirect_uri")
	if !slices.Contains(client.RedirectURIs, redirect) {
		return errorPage(ctx, "The app asked to return to an address it has not registered.")
	}
	state := q.Get("state")
	fail := func(code, desc string) error {
		return kit.Redirect(http.StatusSeeOther, p.redirectWith(redirect, url.Values{"error": {code}, "error_description": {desc}, "state": {state}}))
	}
	if !client.allowsGrant("authorization_code") {
		return fail("unauthorized_client", "the client may not use the authorization code flow")
	}
	if q.Get("response_type") != "code" {
		return fail("unsupported_response_type", "only response_type=code is supported")
	}
	challenge, method := q.Get("code_challenge"), q.Get("code_challenge_method")
	switch {
	case challenge == "" && client.Public:
		return fail("invalid_request", "public clients must use PKCE (code_challenge)")
	case challenge != "" && method != "S256":
		return fail("invalid_request", "code_challenge_method must be S256")
	case challenge != "" && (len(challenge) < 43 || len(challenge) > 128):
		return fail("invalid_request", "invalid code_challenge")
	}
	resource := q.Get("resource")
	if resource != "" && !p.resourceAllowed(resource) {
		return fail("invalid_target", "tokens can't be issued for that resource")
	}
	scopes := p.allowedScopes(client, strings.Fields(q.Get("scope")))
	prompt := strings.Fields(q.Get("prompt"))

	subject, err := p.opts.Authenticate(ctx)
	if err != nil {
		return err
	}
	if subject == "" {
		if slices.Contains(prompt, "none") {
			return fail("login_required", "the user is not signed in")
		}
		// Come back to this request after signing in.
		back := r.URL.Path + "?" + q.Encode()
		sep := "?"
		if strings.Contains(p.opts.LoginURL, "?") {
			sep = "&"
		}
		return kit.Redirect(http.StatusSeeOther, p.opts.LoginURL+sep+"next="+url.QueryEscape(back))
	}

	consented := client.Trusted
	if !consented {
		given, err := p.opts.Store.Consent(ctx, subject, client.ID)
		if err != nil {
			return err
		}
		// A consent must have been given, even for a request with no scopes:
		// with none, "every scope was approved" would be true for any app.
		consented = given != nil && !slices.Contains(prompt, "consent") && containsAll(given, scopes)
	}
	if !consented {
		switch {
		case slices.Contains(prompt, "none"):
			return fail("consent_required", "the user has not approved this app")
		case r.Method == http.MethodPost && q.Get("decision") == "deny":
			return fail("access_denied", "the user declined")
		case r.Method == http.MethodPost && q.Get("decision") == "allow":
			if err := p.opts.Store.SaveConsent(ctx, subject, client.ID, scopes); err != nil {
				return err
			}
		default:
			if p.opts.ConsentURL != "" {
				sep := "?"
				if strings.Contains(p.opts.ConsentURL, "?") {
					sep = "&"
				}
				return kit.Redirect(http.StatusSeeOther, p.opts.ConsentURL+sep+consentParams(q).Encode())
			}
			return p.consentPage(ctx, client, scopes, q)
		}
	}

	code := randomToken()
	err = p.opts.Store.SaveCode(ctx, hash(code), AuthCode{
		ClientID: client.ID, RedirectURI: redirect, Subject: subject, Scopes: scopes, Resource: resource,
		Nonce: q.Get("nonce"), CodeChallenge: challenge, AuthTime: time.Now(), Expires: time.Now().Add(p.opts.CodeTTL),
	})
	if err != nil {
		return err
	}
	return kit.Redirect(http.StatusSeeOther, p.redirectWith(redirect, url.Values{"code": {code}, "state": {state}}))
}

// redirectWith adds params (and iss, RFC 9207) to a redirect URI.
func (p *Plugin) redirectWith(redirect string, params url.Values) string {
	u, _ := url.Parse(redirect)
	q := u.Query()
	for k, v := range params {
		if len(v) > 0 && v[0] != "" {
			q.Set(k, v[0])
		}
	}
	q.Set("iss", p.opts.Issuer)
	u.RawQuery = q.Encode()
	return u.String()
}

func containsAll(have, want []string) bool {
	for _, w := range want {
		if !slices.Contains(have, w) {
			return false
		}
	}
	return true
}

var scopeText = map[string]string{
	"openid":         "Know who you are",
	"profile":        "See your name and picture",
	"email":          "See your email address",
	"offline_access": "Stay signed in when you're away",
}

var pages = template.Must(template.New("").Parse(`
{{define "head"}}<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>{{.Title}}</title>
<style nonce="{{.Nonce}}">body{font:16px/1.5 system-ui,sans-serif;background:#f4f4f5;color:#18181b;display:grid;place-items:center;min-height:100vh;margin:0}main{background:#fff;border-radius:12px;box-shadow:0 1px 3px #0002;padding:2rem;max-width:24rem;width:calc(100% - 2rem)}h1{font-size:1.25rem;margin:0 0 .5rem}ul{padding-left:1.2rem}.actions{display:flex;gap:.5rem;justify-content:flex-end;margin-top:1.5rem}button{font:inherit;border-radius:8px;padding:.5rem 1rem;border:1px solid #d4d4d8;background:#fff;cursor:pointer}button[value=allow]{background:#18181b;color:#fff;border-color:#18181b}@media (prefers-color-scheme:dark){body{background:#09090b;color:#fafafa}main{background:#18181b}button{background:#27272a;color:#fafafa;border-color:#3f3f46}button[value=allow]{background:#fafafa;color:#18181b}}</style></head><body><main>{{end}}
{{define "consent"}}{{template "head" .}}
<h1>{{.Client}} wants to access your account</h1>
{{if .Returns}}<p>You'll go back to <strong>{{.Returns}}</strong> afterwards. Allow it only if you started this there.</p>{{end}}
<p>It will be able to:</p>
<ul>{{range .Scopes}}<li>{{.}}</li>{{end}}</ul>
<form method="post">
{{range $k, $v := .Params}}<input type="hidden" name="{{$k}}" value="{{$v}}">{{end}}
<div class="actions"><button name="decision" value="deny">Deny</button><button name="decision" value="allow" autofocus>Allow</button></div>
</form></main></body></html>{{end}}
{{define "error"}}{{template "head" .}}<h1>Sign-in failed</h1><p>{{.Message}}</p></main></body></html>{{end}}
`))

// consentParams are the parts of an authorization request the consent
// form posts back.
func consentParams(q url.Values) url.Values {
	out := url.Values{}
	for _, k := range []string{"client_id", "redirect_uri", "response_type", "scope", "state", "nonce", "code_challenge", "code_challenge_method", "resource"} {
		if v := q.Get(k); v != "" {
			out.Set(k, v)
		}
	}
	return out
}

func describeScopes(scopes []string) []string {
	lines := make([]string, 0, len(scopes))
	for _, s := range scopes {
		if t, ok := scopeText[s]; ok {
			lines = append(lines, t)
		} else {
			lines = append(lines, s)
		}
	}
	return lines
}

func clientName(c *Client) string {
	if c.Name != "" {
		return c.Name
	}
	return c.ID
}

// Consent is what a consent page shows and posts: see ConsentRequest.
type Consent struct {
	// Client is the app asking, such as "Team wiki".
	Client string `json:"client"`
	// Scopes describe what it may do, such as "See your email address".
	Scopes []string `json:"scopes"`
	// Returns is the site the user goes back to, such as "claude.ai", for
	// clients that registered themselves with a metadata document: their
	// name is theirs to choose, so the page should show where they lead.
	Returns string `json:"returns,omitempty"`
	// Action is where the form posts, with the Params as hidden inputs and
	// a "decision" of "allow" or "deny" from the buttons.
	Action string            `json:"action"`
	Params map[string]string `json:"params"`
}

// ConsentRequest reads the authorization request on the page at
// Options.ConsentURL and returns what to show. It checks the request as the
// authorize endpoint does, which checks it again when the form is posted;
// a signed-out user is sent to sign in first. It sets X-Frame-Options, so
// other sites can't frame the page to trick users into approving.
//
//	func Load(ctx *kit.Ctx) (oidc.Consent, error) { return provider.ConsentRequest(ctx) }
func (p *Plugin) ConsentRequest(ctx *kit.Ctx) (Consent, error) {
	q := ctx.Request.URL.Query()
	client, err := p.lookupClient(ctx, q.Get("client_id"))
	if err != nil {
		return Consent{}, kit.Error(http.StatusBadRequest, "This app is not registered.")
	}
	if !slices.Contains(client.RedirectURIs, q.Get("redirect_uri")) {
		return Consent{}, kit.Error(http.StatusBadRequest, "The app asked to return to an address it has not registered.")
	}
	subject, err := p.opts.Authenticate(ctx)
	if err != nil {
		return Consent{}, err
	}
	if subject == "" {
		sep := "?"
		if strings.Contains(p.opts.LoginURL, "?") {
			sep = "&"
		}
		return Consent{}, kit.Redirect(http.StatusSeeOther, p.opts.LoginURL+sep+"next="+url.QueryEscape(ctx.Request.URL.RequestURI()))
	}
	ctx.SetHeader("X-Frame-Options", "DENY")
	ctx.SetHeader("Cache-Control", "no-store")
	params := map[string]string{}
	for k, v := range consentParams(q) {
		params[k] = v[0]
	}
	return Consent{
		Client:  clientName(client),
		Returns: returnsTo(client, q.Get("redirect_uri")),
		Scopes:  describeScopes(p.allowedScopes(client, strings.Fields(q.Get("scope")))),
		Action:  ctx.Path(p.opts.Path + "/authorize"),
		Params:  params,
	}, nil
}

func (p *Plugin) consentPage(ctx *kit.Ctx, client *Client, scopes []string, q url.Values) error {
	params := map[string]string{}
	for k, v := range consentParams(q) {
		params[k] = v[0]
	}
	lines := describeScopes(scopes)
	name := clientName(client)
	// After the form is posted, the browser is redirected to the client,
	// which form-action must allow.
	actions := []string{"'self'"}
	for _, ru := range client.RedirectURIs {
		if u, err := url.Parse(ru); err == nil && u.Host != "" {
			actions = append(actions, u.Scheme+"://"+u.Host)
		} else if err == nil {
			actions = append(actions, u.Scheme+":")
		}
	}
	return render(ctx, http.StatusOK, "consent", map[string]any{"Title": "Allow " + name + "?", "Client": name, "Returns": returnsTo(client, q.Get("redirect_uri")), "Scopes": lines, "Params": params, "FormAction": strings.Join(actions, " ")})
}

// returnsTo is the host a self-registered client sends the user back to.
func returnsTo(c *Client, redirect string) string {
	if !c.fromMetadata {
		return ""
	}
	if u, err := url.Parse(redirect); err == nil {
		return u.Hostname()
	}
	return ""
}

func errorPage(ctx *kit.Ctx, msg string) error {
	return render(ctx, http.StatusBadRequest, "error", map[string]any{"Title": "Sign-in failed", "Message": msg})
}

func render(ctx *kit.Ctx, status int, name string, data map[string]any) error {
	nonce := randomToken()[:22]
	data["Nonce"] = nonce
	w := ctx.Writer()
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	// The consent page must never be framed: clickjacking would let another
	// site trick users into approving.
	h.Set("X-Frame-Options", "DENY")
	formAction, _ := data["FormAction"].(string)
	if formAction == "" {
		formAction = "'none'"
	}
	h.Set("Content-Security-Policy", "default-src 'none'; style-src 'nonce-"+nonce+"'; form-action "+formAction+"; frame-ancestors 'none'")
	w.WriteHeader(status)
	return pages.ExecuteTemplate(w, name, data)
}

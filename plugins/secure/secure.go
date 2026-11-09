// Package secure sets security headers on every response of a Tern app:
//
//	ternApp(kit.WithPlugins(secure.New()))
//
// The defaults are safe for most apps: pages cannot be framed by other
// sites, referrers are trimmed for cross-origin requests, powerful browser
// features are off, and the page gets its own browsing context group.
// HTTPS-only (HSTS) is opt-in with [HSTS], because it is hard to undo.
// For a Content-Security-Policy, use kit.WithContentSecurityPolicy, which
// handles nonces.
package secure

import (
	"net/http"
	"strconv"
	"time"

	"github.com/Unfield/Tern/kit"
)

// Option configures the plugin.
type Option func(*Plugin)

// HSTS tells browsers to use HTTPS only for maxAge, including subdomains
// when includeSubdomains is set. Only enable it on sites served over HTTPS.
func HSTS(maxAge time.Duration, includeSubdomains bool) Option {
	return func(p *Plugin) {
		v := "max-age=" + strconv.Itoa(int(maxAge.Seconds()))
		if includeSubdomains {
			v += "; includeSubDomains"
		}
		p.headers["Strict-Transport-Security"] = v
	}
}

// AllowFraming lets same-origin pages frame the app (X-Frame-Options:
// SAMEORIGIN) instead of forbidding all framing.
func AllowFraming() Option {
	return func(p *Plugin) { p.headers["X-Frame-Options"] = "SAMEORIGIN" }
}

// Header sets any header, or removes one of the defaults when value is "".
func Header(name, value string) Option {
	return func(p *Plugin) {
		if value == "" {
			delete(p.headers, name)
			return
		}
		p.headers[name] = value
	}
}

// Plugin sets security headers. Create it with New.
type Plugin struct {
	headers map[string]string
}

// New returns the security headers plugin.
func New(opts ...Option) *Plugin {
	p := &Plugin{headers: map[string]string{
		"X-Frame-Options":                   "DENY",
		"Referrer-Policy":                   "strict-origin-when-cross-origin",
		"Permissions-Policy":                "camera=(), microphone=(), geolocation=(), interest-cohort=()",
		"Cross-Origin-Opener-Policy":        "same-origin",
		"X-Permitted-Cross-Domain-Policies": "none",
	}}
	for _, o := range opts {
		o(p)
	}
	return p
}

// Name implements kit.Plugin.
func (*Plugin) Name() string { return "secure" }

// Setup implements kit.Plugin.
func (p *Plugin) Setup(s *kit.Setup) error {
	s.Middleware(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			for k, v := range p.headers {
				h.Set(k, v)
			}
			next.ServeHTTP(w, r)
		})
	})
	return nil
}

// Package secure changes the security headers a Tern app sends. Since
// v1.16 kit sends them without the plugin (see ADR 0027): pages cannot be
// framed by other sites, referrers are trimmed for cross-origin requests,
// powerful browser features are off, and the page gets its own browsing
// context group. Add the plugin to change one, or for HTTPS-only (HSTS),
// which is opt-in because it is hard to undo:
//
//	ternApp(kit.WithPlugins(secure.New(secure.HSTS(365*24*time.Hour, false))))
//
// For the Content-Security-Policy, see kit.WithCSPSources and
// kit.WithContentSecurityPolicy, which handle nonces.
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
			p.removed = append(p.removed, name)
			return
		}
		p.headers[name] = value
	}
}

// Plugin sets security headers. Create it with New.
type Plugin struct {
	headers map[string]string
	removed []string
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
			for _, k := range p.removed {
				h.Del(k)
			}
			for k, v := range p.headers {
				h.Set(k, v)
			}
			next.ServeHTTP(w, r)
		})
	})
	return nil
}

// Package cors lets other origins call a Tern app's endpoints:
//
//	ternApp(kit.WithPlugins(cors.New(
//		cors.Origins("https://app.example.com"),
//		cors.Paths("/api/"),
//	)))
//
// It answers preflight requests, adds the CORS headers to responses, and
// trusts the listed origins for Tern's cross-site request protection.
package cors

import (
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Unfield/Tern/kit"
)

// Option configures the plugin.
type Option func(*Plugin)

// Origins lists the origins allowed to make requests, such as
// "https://app.example.com". "*" allows any origin, but not with credentials.
func Origins(origins ...string) Option {
	return func(p *Plugin) { p.origins = append(p.origins, origins...) }
}

// Methods sets the allowed methods. The default is GET, HEAD, POST, PUT,
// PATCH and DELETE.
func Methods(m ...string) Option { return func(p *Plugin) { p.methods = m } }

// Headers sets the request headers clients may send. The default is
// Content-Type and Authorization.
func Headers(h ...string) Option { return func(p *Plugin) { p.headers = h } }

// Credentials allows cookies and HTTP authentication on cross-origin requests.
func Credentials() Option { return func(p *Plugin) { p.credentials = true } }

// MaxAge sets how long browsers may cache a preflight. The default is 10 minutes.
func MaxAge(d time.Duration) Option { return func(p *Plugin) { p.maxAge = d } }

// Paths limits CORS to URL paths starting with one of the prefixes, such
// as "/api/". The default is every path.
func Paths(prefixes ...string) Option { return func(p *Plugin) { p.paths = prefixes } }

// Plugin handles CORS. Create it with New.
type Plugin struct {
	origins, methods, headers, paths []string
	credentials                      bool
	maxAge                           time.Duration
}

// New returns the CORS plugin.
func New(opts ...Option) *Plugin {
	p := &Plugin{
		methods: []string{http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete},
		headers: []string{"Content-Type", "Authorization"},
		maxAge:  10 * time.Minute,
	}
	for _, o := range opts {
		o(p)
	}
	return p
}

// Name implements kit.Plugin.
func (*Plugin) Name() string { return "cors" }

// Setup implements kit.Plugin.
func (p *Plugin) Setup(s *kit.Setup) error {
	anyOrigin := slices.Contains(p.origins, "*")
	for _, o := range p.origins {
		if o != "*" {
			s.TrustOrigin(o)
		}
	}
	s.Middleware(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin == "" || !p.applies(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			h := w.Header()
			h.Add("Vary", "Origin")
			allowed := slices.Contains(p.origins, origin) || (anyOrigin && !p.credentials)
			if !allowed {
				next.ServeHTTP(w, r)
				return
			}
			if anyOrigin && !p.credentials {
				h.Set("Access-Control-Allow-Origin", "*")
			} else {
				h.Set("Access-Control-Allow-Origin", origin)
			}
			if p.credentials {
				h.Set("Access-Control-Allow-Credentials", "true")
			}
			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				h.Set("Access-Control-Allow-Methods", strings.Join(p.methods, ", "))
				h.Set("Access-Control-Allow-Headers", strings.Join(p.headers, ", "))
				h.Set("Access-Control-Max-Age", strconv.Itoa(int(p.maxAge.Seconds())))
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	})
	return nil
}

func (p *Plugin) applies(path string) bool {
	if len(p.paths) == 0 {
		return true
	}
	for _, prefix := range p.paths {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

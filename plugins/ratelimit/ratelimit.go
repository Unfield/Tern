// Package ratelimit limits how often each client can call a Tern app:
//
//	ternApp(kit.WithPlugins(ratelimit.New(ratelimit.Rate(10, time.Minute))))
//
// Each client gets a token bucket: up to Burst requests at once, refilled
// at the given rate. By default only state-changing requests (POST, PUT,
// PATCH, DELETE), such as form actions and API writes, are limited. A
// client over the limit gets 429 Too Many Requests with Retry-After.
//
// Clients are identified by their IP address from kit.ClientIP. Behind a
// reverse proxy, configure the app with kit.WithTrustedProxies.
//
// Since v1.16 kit limits state-changing requests without the plugin, to
// 120 a minute in bursts of 60 (ADR 0027); adding the plugin replaces that
// limit with its own.
package ratelimit

import (
	"context"
	"math"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/Unfield/Tern/cache"
	"github.com/Unfield/Tern/internal/ratelimit"
	"github.com/Unfield/Tern/kit"
)

// Option configures the plugin.
type Option func(*Plugin)

// Rate allows n requests per period, per client. The default is 30 per minute.
func Rate(n int, per time.Duration) Option {
	return func(p *Plugin) { p.rate = float64(n) / per.Seconds(); p.burst = float64(n) }
}

// Burst sets how many requests a client may make at once. It defaults to n
// from Rate.
func Burst(n int) Option { return func(p *Plugin) { p.burst = float64(n) } }

// Methods sets which methods are limited. Pass none to limit every request.
func Methods(m ...string) Option { return func(p *Plugin) { p.methods = m } }

// KeyFunc identifies the client for a request. The default is
// kit.ClientIP. Return "" to exempt a request.
func KeyFunc(fn func(*http.Request) string) Option { return func(p *Plugin) { p.key = fn } }

// Store keeps the counts in store, such as Redis, instead of this
// process's memory. Every instance of the app then shares them, and they
// outlast a restart. A store counts in fixed windows of Burst requests over
// Burst/rate: with Rate(30, time.Minute), 30 in each minute.
func Store(store cache.Store) Option { return func(p *Plugin) { p.store = store } }

// Plugin limits request rates. Create it with New.
type Plugin struct {
	rate, burst float64
	methods     []string
	key         func(*http.Request) string
	store       cache.Store
	limiter     ratelimit.Taker
	memory      *ratelimit.Limiter
}

// New returns the rate limiting plugin.
func New(opts ...Option) *Plugin {
	p := &Plugin{
		methods: []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete},
		key:     kit.ClientIP,
	}
	Rate(30, time.Minute)(p)
	for _, o := range opts {
		o(p)
	}
	if p.store != nil {
		p.limiter = ratelimit.NewStore(p.store, "tern:ratelimit:plugin:", p.rate, p.burst)
	} else {
		p.memory = ratelimit.New(p.rate, p.burst)
		p.limiter = p.memory
	}
	return p
}

// Name implements kit.Plugin.
func (*Plugin) Name() string { return "ratelimit" }

// Setup implements kit.Plugin.
func (p *Plugin) Setup(s *kit.Setup) error {
	if st, ok := p.limiter.(*ratelimit.Store); ok {
		log := s.Logger()
		st.OnError = func(err error) { log.Warn("ratelimit: the store failed; allowing the request", "err", err) }
	}
	if p.memory != nil {
		stop := make(chan struct{})
		go p.memory.Sweep(stop)
		s.OnShutdown(func(context.Context) error { close(stop); return nil })
	}
	s.Middleware(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if len(p.methods) > 0 && !slices.Contains(p.methods, r.Method) {
				next.ServeHTTP(w, r)
				return
			}
			k := p.key(r)
			if k == "" {
				next.ServeHTTP(w, r)
				return
			}
			if wait, ok := p.limiter.Take(r.Context(), k); !ok {
				w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(wait.Seconds()))))
				http.Error(w, "Too many requests; try again later.", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	})
	return nil
}

// Package health adds liveness and readiness endpoints to a Tern app:
//
//	ternApp(kit.WithPlugins(health.New(
//		health.Check("database", db.PingContext),
//	)))
//
// GET /healthz reports that the process is up. GET /readyz runs every check
// and reports 503 if one fails, or while the app is shutting down, so a load
// balancer stops sending traffic before the server stops.
package health

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Unfield/Tern/kit"
)

// CheckFunc reports whether a dependency is usable.
type CheckFunc func(ctx context.Context) error

type check struct {
	name string
	fn   CheckFunc
}

// Option configures the plugin.
type Option func(*Plugin)

// Check adds a readiness check.
func Check(name string, fn CheckFunc) Option {
	return func(p *Plugin) { p.checks = append(p.checks, check{name, fn}) }
}

// Timeout bounds all checks together. The default is 5 seconds.
func Timeout(d time.Duration) Option {
	return func(p *Plugin) { p.timeout = d }
}

// Paths changes the endpoints from /healthz and /readyz.
func Paths(live, ready string) Option {
	return func(p *Plugin) { p.live, p.ready = live, ready }
}

// Plugin serves the health endpoints. Create it with New.
type Plugin struct {
	checks       []check
	timeout      time.Duration
	live, ready  string
	shuttingDown atomic.Bool
}

// New returns the health plugin.
func New(opts ...Option) *Plugin {
	p := &Plugin{timeout: 5 * time.Second, live: "/healthz", ready: "/readyz"}
	for _, o := range opts {
		o(p)
	}
	return p
}

// Name implements kit.Plugin.
func (*Plugin) Name() string { return "health" }

// Setup implements kit.Plugin.
func (p *Plugin) Setup(s *kit.Setup) error {
	s.Handle("GET "+p.live, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		write(w, http.StatusOK, map[string]any{"status": "ok"})
	}))
	s.Handle("GET "+p.ready, http.HandlerFunc(p.serveReady))
	s.OnShutdown(func(context.Context) error {
		p.shuttingDown.Store(true)
		return nil
	})
	return nil
}

type result struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
	Took   string `json:"took"`
}

func (p *Plugin) serveReady(w http.ResponseWriter, r *http.Request) {
	if p.shuttingDown.Load() {
		write(w, http.StatusServiceUnavailable, map[string]any{"status": "shutting down"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), p.timeout)
	defer cancel()

	results := make(map[string]result, len(p.checks))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, c := range p.checks {
		wg.Go(func() {
			start := time.Now()
			err := c.fn(ctx)
			res := result{Status: "ok", Took: time.Since(start).Round(time.Microsecond).String()}
			if err != nil {
				res.Status, res.Error = "failing", err.Error()
			}
			mu.Lock()
			results[c.name] = res
			mu.Unlock()
		})
	}
	wg.Wait()

	status, code := "ok", http.StatusOK
	for _, res := range results {
		if res.Status != "ok" {
			status, code = "failing", http.StatusServiceUnavailable
		}
	}
	write(w, code, map[string]any{"status": status, "checks": results})
}

func write(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

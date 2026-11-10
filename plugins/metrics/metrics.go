// Package metrics exposes Prometheus metrics for a Tern app, with no
// dependencies:
//
//	ternApp(kit.WithPlugins(metrics.New()))
//
// GET /metrics serves, in the Prometheus text format:
//
//	tern_http_requests_total{method,route,status}
//	tern_http_request_duration_seconds{method,route} (histogram)
//	tern_http_requests_in_flight
//	go_goroutines, go_memstats_heap_alloc_bytes, process_uptime_seconds
//
// The route label is the route's pattern, such as /projects/[id], never the
// raw URL, so the number of series stays bounded. Requests that match no
// route (assets, 404s) are labeled "other".
package metrics

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Unfield/Tern/internal/routes"
	"github.com/Unfield/Tern/kit"
)

// Option configures the plugin.
type Option func(*Plugin)

// Path changes the endpoint from /metrics.
func Path(p string) Option { return func(m *Plugin) { m.path = p } }

// Buckets sets the latency histogram's upper bounds, in seconds.
func Buckets(b ...float64) Option { return func(m *Plugin) { m.buckets = b } }

// Plugin collects and serves metrics. Create it with New.
type Plugin struct {
	path     string
	buckets  []float64
	started  time.Time
	inFlight atomic.Int64

	mu       sync.Mutex
	counts   map[key]uint64
	hists    map[histKey]*hist
	patterns sync.Map // route ID -> display pattern
}

type key struct{ method, route, status string }
type histKey struct{ method, route string }

type hist struct {
	counts []uint64 // per bucket, cumulative at export
	sum    float64
	n      uint64
}

// New returns the metrics plugin.
func New(opts ...Option) *Plugin {
	m := &Plugin{
		path:    "/metrics",
		buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		started: time.Now(),
		counts:  map[key]uint64{},
		hists:   map[histKey]*hist{},
	}
	for _, o := range opts {
		o(m)
	}
	sort.Float64s(m.buckets)
	return m
}

// Name implements kit.Plugin.
func (*Plugin) Name() string { return "metrics" }

type routeKey struct{}

// Setup implements kit.Plugin.
func (m *Plugin) Setup(s *kit.Setup) error {
	s.Handle("GET "+m.path, http.HandlerFunc(m.serve))
	// The hook runs after routing, so it can record which route matched.
	s.Hook(func(c *kit.Ctx) error {
		if slot, ok := c.Value(routeKey{}).(*string); ok {
			*slot = c.RouteID()
		}
		return nil
	})
	s.Middleware(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == m.path {
				next.ServeHTTP(w, r)
				return
			}
			m.inFlight.Add(1)
			defer m.inFlight.Add(-1)
			var route string
			r = r.WithContext(context.WithValue(r.Context(), routeKey{}, &route))
			rec := &recorder{ResponseWriter: w, status: http.StatusOK}
			start := time.Now()
			next.ServeHTTP(rec, r)
			m.observe(r.Method, m.pattern(route), rec.status, time.Since(start))
		})
	})
	return nil
}

func (m *Plugin) pattern(id string) string {
	if id == "" {
		return "other"
	}
	if p, ok := m.patterns.Load(id); ok {
		return p.(string)
	}
	p := id
	if segs, err := routes.ParseID(id); err == nil {
		p = (&routes.Route{Segments: segs}).Pattern()
	}
	m.patterns.Store(id, p)
	return p
}

func (m *Plugin) observe(method, route string, status int, d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.counts[key{method, route, strconv.Itoa(status)}]++
	hk := histKey{method, route}
	h := m.hists[hk]
	if h == nil {
		h = &hist{counts: make([]uint64, len(m.buckets))}
		m.hists[hk] = h
	}
	sec := d.Seconds()
	for i, b := range m.buckets {
		if sec <= b {
			h.counts[i]++
			break
		}
	}
	h.sum += sec
	h.n++
}

func (m *Plugin) serve(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	m.write(w)
}

func (m *Plugin) write(w io.Writer) {
	m.mu.Lock()
	keys := make([]key, 0, len(m.counts))
	for k := range m.counts {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		return a.route+a.method+a.status < b.route+b.method+b.status
	})
	fmt.Fprintln(w, "# HELP tern_http_requests_total Requests served, by method, route and status.")
	fmt.Fprintln(w, "# TYPE tern_http_requests_total counter")
	for _, k := range keys {
		fmt.Fprintf(w, "tern_http_requests_total{method=%q,route=%q,status=%q} %d\n", k.method, k.route, k.status, m.counts[k])
	}

	hkeys := make([]histKey, 0, len(m.hists))
	for k := range m.hists {
		hkeys = append(hkeys, k)
	}
	sort.Slice(hkeys, func(i, j int) bool { return hkeys[i].route+hkeys[i].method < hkeys[j].route+hkeys[j].method })
	fmt.Fprintln(w, "# HELP tern_http_request_duration_seconds Request latency.")
	fmt.Fprintln(w, "# TYPE tern_http_request_duration_seconds histogram")
	for _, k := range hkeys {
		h := m.hists[k]
		var cum uint64
		for i, b := range m.buckets {
			cum += h.counts[i]
			fmt.Fprintf(w, "tern_http_request_duration_seconds_bucket{method=%q,route=%q,le=%q} %d\n", k.method, k.route, strconv.FormatFloat(b, 'g', -1, 64), cum)
		}
		fmt.Fprintf(w, "tern_http_request_duration_seconds_bucket{method=%q,route=%q,le=\"+Inf\"} %d\n", k.method, k.route, h.n)
		fmt.Fprintf(w, "tern_http_request_duration_seconds_sum{method=%q,route=%q} %s\n", k.method, k.route, strconv.FormatFloat(h.sum, 'g', -1, 64))
		fmt.Fprintf(w, "tern_http_request_duration_seconds_count{method=%q,route=%q} %d\n", k.method, k.route, h.n)
	}
	m.mu.Unlock()

	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	gauge := func(name, help string, v float64) {
		fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s gauge\n%s %s\n", name, help, name, name, strings.TrimSuffix(strconv.FormatFloat(v, 'f', 3, 64), ".000"))
	}
	gauge("tern_http_requests_in_flight", "Requests being served.", float64(m.inFlight.Load()))
	gauge("go_goroutines", "Goroutines.", float64(runtime.NumGoroutine()))
	gauge("go_memstats_heap_alloc_bytes", "Heap bytes in use.", float64(ms.HeapAlloc))
	gauge("process_uptime_seconds", "Seconds since the app started.", time.Since(m.started).Seconds())
}

type recorder struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (r *recorder) WriteHeader(code int) {
	if !r.wrote {
		r.status, r.wrote = code, true
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *recorder) Write(b []byte) (int, error) {
	r.wrote = true
	return r.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (r *recorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

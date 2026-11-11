package kit

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Unfield/Tern/tracing"
)

type spanMemory struct {
	mu    sync.Mutex
	spans []tracing.SpanData
}

func (m *spanMemory) Export(_ context.Context, s []tracing.SpanData) error {
	m.mu.Lock()
	m.spans = append(m.spans, s...)
	m.mu.Unlock()
	return nil
}

func TestTracing(t *testing.T) {
	mem := &spanMemory{}
	tr := tracing.New(tracing.Options{Exporters: []tracing.Exporter{mem}, Batch: tracing.Batch{Interval: time.Millisecond}})
	var logs bytes.Buffer
	a, err := New(testManifest(), WithTracer(tr), WithTrustedProxies("10.0.0.0/8"), WithLogger(slog.New(slog.NewJSONHandler(&logs, nil))))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = a.Close() }()
	defer tracing.SetDefault(nil)

	get := func(path, traceparent string) {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		r.RemoteAddr = "10.0.0.2:1"
		if traceparent != "" {
			r.Header.Set("Traceparent", traceparent)
		}
		a.ServeHTTP(httptest.NewRecorder(), r)
	}
	get("/blog/hello", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	get("/blog/broken", "")
	if err := tr.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	mem.mu.Lock()
	defer mem.mu.Unlock()
	traces := map[tracing.TraceID][]tracing.SpanData{}
	for _, s := range mem.spans {
		traces[s.Context.TraceID] = append(traces[s.Context.TraceID], s)
	}
	remote, _ := tracing.ParseTraceparent("00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	hello := traces[remote.TraceID]
	names := map[string]tracing.SpanData{}
	for _, s := range hello {
		names[s.Name] = s
	}
	server, ok := names["GET /blog/slug.param"]
	if !ok || server.Parent != remote.SpanID || server.Kind != tracing.Server {
		t.Fatalf("server span: %+v", hello)
	}
	// The blog layout has no load, and the test app no server renderer.
	for _, n := range []string{"load layout /", "load page"} {
		s, ok := names[n]
		if !ok || s.Parent != server.Context.SpanID {
			t.Errorf("%s: %+v (%v)", n, s, ok)
		}
	}
	var status int64
	for _, a := range server.Attrs {
		if a.Key == "http.response.status_code" {
			status = a.Value.Int64()
		}
	}
	if status != 200 {
		t.Errorf("status %d", status)
	}

	// The failing load: its span and the request's are errors, and the log
	// line carries the trace.
	var broken []tracing.SpanData
	for id, spans := range traces {
		if id != remote.TraceID {
			broken = spans
		}
	}
	failed := map[string]bool{}
	for _, s := range broken {
		if s.Status.Code == tracing.Error {
			failed[s.Name] = true
		}
	}
	if !failed["load page"] || !failed["GET /blog/slug.param"] {
		t.Errorf("errors on %v", failed)
	}
	var line map[string]any
	for _, l := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		if strings.Contains(l, "unexpected error") {
			_ = json.Unmarshal([]byte(l), &line)
		}
	}
	if line == nil || line["trace_id"] != broken[0].Context.TraceID.String() {
		t.Errorf("log line %v", line)
	}
}

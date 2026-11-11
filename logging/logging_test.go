package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"testing/slogtest"
	"time"
)

type memSink struct {
	mu      sync.Mutex
	entries []Entry
	fail    int // fail this many calls first
	calls   int
	err     error
}

func (m *memSink) Export(_ context.Context, es []Entry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	if m.fail > 0 {
		m.fail--
		if m.err != nil {
			return m.err
		}
		return errors.New("down")
	}
	m.entries = append(m.entries, es...)
	return nil
}

func (m *memSink) got() []Entry {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Entry(nil), m.entries...)
}

func jsonLogger(buf *bytes.Buffer, o Options) *slog.Logger {
	o.Output, o.Format = buf, "json"
	if o.Level == nil {
		o.Level = slog.LevelDebug
	}
	return New(o)
}

func lines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, l := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if l == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatalf("%q: %v", l, err)
		}
		out = append(out, m)
	}
	return out
}

// The handler follows slog's rules for handlers.
func TestSlogtest(t *testing.T) {
	var buf bytes.Buffer
	h := NewHandler(Options{Output: &buf, Format: "json", Level: slog.LevelDebug})
	slogtest.Run(t, func(*testing.T) slog.Handler { buf.Reset(); return h }, func(t *testing.T) map[string]any {
		ls := lines(t, &buf)
		if len(ls) != 1 {
			t.Fatalf("%d lines", len(ls))
		}
		return ls[0]
	})
}

// So do the entries sinks get.
func TestSlogtestEntries(t *testing.T) {
	var sink *memSink
	slogtest.Run(t, func(*testing.T) slog.Handler {
		sink = &memSink{}
		return NewHandler(Options{Output: discard{}, Level: slog.LevelDebug, Sinks: []Sink{sink}, Batch: Batch{Interval: time.Millisecond}})
	}, func(t *testing.T) map[string]any {
		deadline := time.Now().Add(2 * time.Second)
		for len(sink.got()) == 0 && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		es := sink.got()
		if len(es) != 1 {
			t.Fatalf("%d entries", len(es))
		}
		e := es[0]
		m := AttrsMap(e.Attrs)
		m[slog.MessageKey] = e.Message
		m[slog.LevelKey] = e.Level
		if !e.Time.IsZero() {
			m[slog.TimeKey] = e.Time
		}
		// slogtest compares group members as map[string]any.
		return m
	})
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

func TestContextAttrs(t *testing.T) {
	var buf bytes.Buffer
	sink := &memSink{}
	l := jsonLogger(&buf, Options{Sinks: []Sink{sink}})
	ctx := With(context.Background(), slog.String(RequestIDKey, "r1"))
	ctx = With(ctx, slog.String("user", "u1"), slog.String(RequestIDKey, "r2"))
	l.With("app", "x").WithGroup("g").InfoContext(ctx, "hi", "k", 1)
	if err := Flush(context.Background(), l); err != nil {
		t.Fatal(err)
	}
	got := lines(t, &buf)[0]
	if got["request_id"] != "r2" || got["user"] != "u1" || got["app"] != "x" {
		t.Fatalf("output: %v", got)
	}
	if g, _ := got["g"].(map[string]any); g["k"] != float64(1) {
		t.Fatalf("group: %v", got)
	}
	if RequestID(ctx) != "r2" {
		t.Fatal(RequestID(ctx))
	}
	es := sink.got()
	if len(es) != 1 {
		t.Fatalf("entries: %v", es)
	}
	var keys []string
	es[0].Walk(func(k string, _ slog.Value) { keys = append(keys, k) })
	if want := "app user request_id g.k"; strings.Join(keys, " ") != want {
		t.Fatalf("keys %v, want %s", keys, want)
	}
	// The output has them in the same order.
	line := buf.String()
	if ordered := strings.Index(line, `"app"`) < strings.Index(line, `"user"`) && strings.Index(line, `"request_id"`) < strings.Index(line, `"g"`); !ordered {
		t.Fatalf("order: %s", line)
	}
}

func TestRedact(t *testing.T) {
	var buf bytes.Buffer
	l := jsonLogger(&buf, Options{Redact: []string{"ssn"}, Keep: []string{"session"}})
	u, _ := url.Parse("https://bob:hunter2@example.com/cb?code=1&access_token=abc&page=2")
	h := http.Header{"Authorization": {"Bearer x"}, "Accept": {"text/html"}, "Cookie": {"a=b"}}
	l.Info("m",
		"password", "hunter2",
		"Access-Token", "t",
		"stripe_secret_key", "sk",
		"cache_key", "ok",
		"session", "kept",
		"ssn", "123",
		"key", Secret("s"),
		"url", u,
		"headers", h,
		slog.Group("user", "name", "ann", "api_key", "k"),
	)
	got := lines(t, &buf)[0]
	for _, k := range []string{"password", "Access-Token", "stripe_secret_key", "ssn", "key"} {
		if got[k] != Redacted {
			t.Errorf("%s = %v", k, got[k])
		}
	}
	if got["cache_key"] != "ok" || got["session"] != "kept" {
		t.Errorf("kept keys redacted: %v", got)
	}
	if s := got["url"].(string); strings.Contains(s, "hunter2") || strings.Contains(s, "abc") || !strings.Contains(s, "page=2") {
		t.Errorf("url %s", s)
	}
	hs := got["headers"].(map[string]any)
	if hs["Authorization"] != Redacted || hs["Cookie"] != Redacted || hs["Accept"] != "text/html" {
		t.Errorf("headers %v", hs)
	}
	if us := got["user"].(map[string]any); us["api_key"] != Redacted || us["name"] != "ann" {
		t.Errorf("group %v", us)
	}
	// With attributes are redacted too.
	buf.Reset()
	l.With("token", "t").Info("m")
	if lines(t, &buf)[0]["token"] != Redacted {
		t.Error(buf.String())
	}
}

func TestSampling(t *testing.T) {
	var buf bytes.Buffer
	l := jsonLogger(&buf, Options{Sampling: &Sampling{First: 3, Thereafter: 5, Tick: time.Hour}})
	for range 20 {
		l.Info("flood")
		l.Warn("flood") // above MaxLevel: all kept
	}
	l.Info("other")
	var info, warn, other int
	for _, m := range lines(t, &buf) {
		switch {
		case m["msg"] == "other":
			other++
		case m["level"] == "INFO":
			info++
		default:
			warn++
		}
	}
	// 3 first, then the 8th, 13th and 18th.
	if info != 6 || warn != 20 || other != 1 {
		t.Fatalf("info %d warn %d other %d", info, warn, other)
	}
	if s := StatsOf(l).Sampled; s != 14 {
		t.Fatalf("sampled %d", s)
	}
}

func TestSamplingTick(t *testing.T) {
	s := newSampler(&Sampling{First: 1, Tick: time.Second})
	now := time.Unix(100, 0)
	if !s.keep(slog.LevelInfo, "m", now) || s.keep(slog.LevelInfo, "m", now) {
		t.Fatal("first tick")
	}
	if !s.keep(slog.LevelInfo, "m", now.Add(2*time.Second)) {
		t.Fatal("next tick")
	}
}

func TestShipperRetries(t *testing.T) {
	sink := &memSink{fail: 2}
	sh := newShipper(sink, Batch{Interval: time.Millisecond}, nil)
	sh.sleep = func(context.Context, time.Duration) bool { return true }
	for i := range 3 {
		sh.add(Entry{Message: string(rune('a' + i))})
	}
	if err := sh.flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if es := sink.got(); len(es) != 3 || es[0].Message != "a" || es[2].Message != "c" {
		t.Fatalf("%v", es)
	}
	if err := sh.close(context.Background()); err != nil {
		t.Fatal(err)
	}
	sh.add(Entry{})
	if sh.dropped.Load() != 1 {
		t.Fatal("add after close kept")
	}
}

func TestShipperPermanent(t *testing.T) {
	var report bytes.Buffer
	sink := &memSink{fail: 1, err: Permanent(errors.New("bad request"))}
	sh := newShipper(sink, Batch{Interval: time.Millisecond}, &report)
	sh.add(Entry{Message: "x"})
	_ = sh.flush(context.Background())
	if sink.calls != 1 || sh.failed.Load() != 1 || !strings.Contains(report.String(), "bad request") {
		t.Fatalf("calls %d failed %d report %q", sink.calls, sh.failed.Load(), report.String())
	}
	_ = sh.close(context.Background())
}

func TestShipperQueueFull(t *testing.T) {
	block := make(chan struct{})
	sink := SinkFunc(func(context.Context, []Entry) error { <-block; return nil })
	sh := newShipper(sink, Batch{Size: 1, Queue: 2, Interval: time.Millisecond}, nil)
	for range 10 {
		sh.add(Entry{})
	}
	close(block)
	_ = sh.close(context.Background())
	if d := sh.dropped.Load(); d < 7 {
		t.Fatalf("dropped %d", d)
	}
}

func TestMinLevel(t *testing.T) {
	all, errs := &memSink{}, &memSink{}
	l := New(Options{Output: discard{}, Sinks: []Sink{all, MinLevel(errs, slog.LevelError)}})
	l.Info("i")
	l.Error("e")
	_ = Close(context.Background(), l)
	if len(all.got()) != 2 || len(errs.got()) != 1 {
		t.Fatalf("all %d errs %d", len(all.got()), len(errs.got()))
	}
}

func TestWrap(t *testing.T) {
	var buf bytes.Buffer
	h := Wrap(slog.NewJSONHandler(&buf, nil))
	if Wrap(h) != h {
		t.Fatal("wrapped twice")
	}
	own := NewHandler(Options{Output: discard{}})
	if Wrap(own) != own {
		t.Fatal("wrapped New's handler")
	}
	slog.New(h).InfoContext(With(context.Background(), slog.String(RequestIDKey, "r")), "m")
	if lines(t, &buf)[0]["request_id"] != "r" {
		t.Fatal(buf.String())
	}
}

func TestFormatFromEnv(t *testing.T) {
	t.Setenv("TERN_LOG_FORMAT", "text")
	t.Setenv("TERN_LOG_LEVEL", "warn")
	var buf bytes.Buffer
	l := New(Options{Output: &buf})
	l.Info("hidden")
	l.Warn("shown")
	if s := buf.String(); strings.Contains(s, "hidden") || !strings.Contains(s, "level=WARN msg=shown") {
		t.Fatal(s)
	}
}

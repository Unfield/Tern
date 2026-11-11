package tracing

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// memory collects exported spans.
type memory struct {
	mu    sync.Mutex
	spans []SpanData
}

func (m *memory) Export(_ context.Context, s []SpanData) error {
	m.mu.Lock()
	m.spans = append(m.spans, s...)
	m.mu.Unlock()
	return nil
}

func (m *memory) got(t *testing.T, tr *Tracer) []SpanData {
	t.Helper()
	if err := tr.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]SpanData(nil), m.spans...)
}

func byName(spans []SpanData, name string) *SpanData {
	for i := range spans {
		if spans[i].Name == name {
			return &spans[i]
		}
	}
	return nil
}

func newTracer() (*Tracer, *memory) {
	m := &memory{}
	return New(Options{Exporters: []Exporter{m}, Batch: Batch{Interval: time.Millisecond}}), m
}

func TestTraceparent(t *testing.T) {
	sc, ok := ParseTraceparent("00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	if !ok || !sc.Sampled || sc.TraceID.String() != "4bf92f3577b34da6a3ce929d0e0e4736" || sc.SpanID.String() != "00f067aa0ba902b7" {
		t.Fatalf("%+v %v", sc, ok)
	}
	if sc.Traceparent() != "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01" {
		t.Error(sc.Traceparent())
	}
	for _, bad := range []string{"", "00-0000000000000000000000000000000-00f067aa0ba902b7-01", "00-00000000000000000000000000000000-00f067aa0ba902b7-01", "ff-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01", "00-4BF92F3577B34DA6A3CE929D0E0E4736-00f067aa0ba902b7-01"} {
		if _, ok := ParseTraceparent(bad); ok {
			t.Errorf("accepted %q", bad)
		}
	}
	if sc, ok := ParseTraceparent("00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-00"); !ok || sc.Sampled {
		t.Error("unsampled flag")
	}
}

func TestSpans(t *testing.T) {
	tr, m := newTracer()
	ctx, root := Start(context.Background(), "root", WithTracer(tr), WithKind(Server), WithAttrs(slog.String("a", "1")))
	_, child := Start(ctx, "child")
	child.SetAttrs(slog.Int("n", 1), slog.Int("n", 2))
	child.RecordError(errors.New("boom"))
	child.SetStatus(OK, "") // an error stays an error
	child.End()
	child.End() // twice: exported once
	root.SetName("renamed")
	root.End()

	spans := m.got(t, tr)
	if len(spans) != 2 {
		t.Fatalf("%d spans", len(spans))
	}
	c, r := byName(spans, "child"), byName(spans, "renamed")
	if c == nil || r == nil {
		t.Fatalf("%+v", spans)
	}
	if c.Context.TraceID != r.Context.TraceID || c.Parent != r.Context.SpanID || r.Parent.IsValid() {
		t.Error("parenting")
	}
	if c.Status.Code != Error || c.Status.Message != "boom" || len(c.Events) != 1 || c.Events[0].Name != "exception" {
		t.Errorf("child status %+v events %+v", c.Status, c.Events)
	}
	if len(c.Attrs) != 1 || c.Attrs[0].Value.Int64() != 2 {
		t.Errorf("attrs %v", c.Attrs)
	}
	if r.Kind != Server || c.Kind != Internal || r.Duration() < c.Duration() {
		t.Errorf("kinds or times: %v %v", r, c)
	}
}

func TestRemoteParentAndSampling(t *testing.T) {
	tr, m := newTracer()
	remote, _ := ParseTraceparent("00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	_, s := Start(context.Background(), "server", WithTracer(tr), WithRemoteParent(remote))
	s.End()
	if got := m.got(t, tr); len(got) != 1 || got[0].Context.TraceID != remote.TraceID || got[0].Parent != remote.SpanID {
		t.Fatalf("%+v", got)
	}

	// An unsampled parent: nothing recorded, but the IDs go on.
	off, _ := ParseTraceparent("00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-00")
	ctx, s := Start(context.Background(), "x", WithTracer(tr), WithRemoteParent(off))
	if s.IsRecording() || !s.Context().IsValid() {
		t.Error("an unsampled span")
	}
	s.SetAttrs(slog.String("ignored", "x"))
	s.End()
	if FromContext(ctx).Context().TraceID != off.TraceID {
		t.Error("trace lost")
	}

	none := New(Options{Sample: 0.000000001})
	_, s = Start(context.Background(), "x", WithTracer(none))
	if s.IsRecording() {
		t.Error("sampled at a rate near 0")
	}

	// No tracer at all: nil-safe, still a context.
	var nilSpan *Span
	nilSpan.End()
	nilSpan.SetAttrs(slog.String("a", "b"))
	if nilSpan.IsRecording() || nilSpan.Context().IsValid() {
		t.Error("nil span")
	}
}

func TestTransport(t *testing.T) {
	tr, m := newTracer()
	var seen string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("Traceparent")
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	ctx, root := Start(context.Background(), "root", WithTracer(tr))
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/x?token=secret&page=1", nil)
	resp, err := (&http.Client{Transport: Transport(nil)}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	root.End()
	spans := m.got(t, tr)
	client := byName(spans, "GET "+req.URL.Host)
	if client == nil || client.Kind != Client {
		t.Fatalf("%+v", spans)
	}
	sc, ok := ParseTraceparent(seen)
	if !ok || sc.TraceID != root.Context().TraceID || sc.SpanID != client.Context.SpanID {
		t.Errorf("traceparent %q", seen)
	}
	if client.Status.Code != Error {
		t.Error("a 502 isn't an error")
	}
	for _, a := range client.Attrs {
		if a.Key == "url.full" && strings.Contains(a.Value.String(), "secret") {
			t.Errorf("url not redacted: %s", a.Value)
		}
	}
}

func TestSQL(t *testing.T) {
	tr, m := newTracer()
	db, err := OpenDB("sqlite", "file:"+t.Name()+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(`CREATE TABLE t (n INTEGER)`); err != nil { // no span: no trace
		t.Fatal(err)
	}
	ctx, root := Start(context.Background(), "root", WithTracer(tr))
	if _, err := db.ExecContext(ctx, `INSERT INTO t (n) VALUES (?)`, 1); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRowContext(ctx, `/* hi */ SELECT n FROM t`).Scan(&n); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	st, err := db.PrepareContext(ctx, `SELECT count(*) FROM t WHERE n = ?`)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.QueryRowContext(ctx, 1).Scan(&n); err != nil {
		t.Fatal(err)
	}
	_ = st.Close()
	_, err = db.ExecContext(ctx, `INSERT INTO nonesuch VALUES (1)`)
	if err == nil {
		t.Fatal("no error")
	}
	if err := db.QueryRowContext(ctx, `SELECT n FROM t WHERE n = 99`).Scan(&n); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal(err)
	}
	root.End()
	spans := m.got(t, tr)
	var names []string
	failed := 0
	for _, s := range spans {
		if s.Name != "root" {
			names = append(names, s.Name)
			if s.Kind != Client || s.Parent != root.Context().SpanID {
				t.Errorf("%s: kind %v parent %v", s.Name, s.Kind, s.Parent)
			}
		}
		if s.Status.Code == Error {
			failed++
		}
	}
	if strings.Join(names, ",") != "INSERT,SELECT,SELECT,INSERT,SELECT" || failed != 1 {
		t.Errorf("spans %v, %d failed", names, failed)
	}
}

func TestQueryName(t *testing.T) {
	for q, want := range map[string]string{"select 1": "SELECT", "  -- c\nUPDATE x": "UPDATE", "/* a */ WITH x AS (": "WITH", "": "SQL"} {
		if got := queryName(q); got != want {
			t.Errorf("%q: %s", q, got)
		}
	}
}

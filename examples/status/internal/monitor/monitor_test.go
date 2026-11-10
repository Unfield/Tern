package monitor

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Unfield/Tern/migrate"

	"status/internal/store"
)

func TestChecks(t *testing.T) {
	var code atomic.Int32
	code.Store(200)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(int(code.Load()))
		_, _ = w.Write([]byte("hello tern"))
	}))
	defer srv.Close()
	ctx := context.Background()
	m := store.Monitor{Type: "http", Target: srv.URL, ExpectedStatus: "200-299", TimeoutSeconds: 2}
	if r := Check(ctx, m); !r.OK || r.Message != "HTTP 200" {
		t.Errorf("up: %+v", r)
	}
	m.Keyword = "missing"
	if r := Check(ctx, m); r.OK {
		t.Errorf("a missing keyword should fail: %+v", r)
	}
	m.Keyword = ""
	code.Store(503)
	if r := Check(ctx, m); r.OK || r.Message != "HTTP 503, expected 200-299" {
		t.Errorf("down: %+v", r)
	}

	host := srv.Listener.Addr().String()
	if r := Check(ctx, store.Monitor{Type: "tcp", Target: host, TimeoutSeconds: 2}); !r.OK {
		t.Errorf("tcp: %+v", r)
	}
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	closed := ln.Addr().String()
	_ = ln.Close()
	if r := Check(ctx, store.Monitor{Type: "tcp", Target: closed, TimeoutSeconds: 2}); r.OK || r.Message != "connection refused" {
		t.Errorf("closed port: %+v", r)
	}
	if r := Check(ctx, store.Monitor{Type: "dns", Target: "localhost", TimeoutSeconds: 2}); !r.OK {
		t.Errorf("dns: %+v", r)
	}
	if r := Check(ctx, store.Monitor{Type: "dns", Target: "does-not-exist.invalid", TimeoutSeconds: 2}); r.OK {
		t.Errorf("a missing host should fail: %+v", r)
	}
}

func TestStatuses(t *testing.T) {
	ok, err := parseStatuses("200-299, 301,404")
	if err != nil || !ok(204) || !ok(301) || !ok(404) || ok(302) || ok(500) {
		t.Errorf("parse: %v", err)
	}
	for _, bad := range []string{"abc", "299-200", "99", "200-700"} {
		if _, err := parseStatuses(bad); err == nil {
			t.Errorf("%q should be invalid", bad)
		}
	}
}

func TestValidate(t *testing.T) {
	good := store.Monitor{Name: "x", Type: "http", Target: "https://example.com", IntervalSeconds: 60, TimeoutSeconds: 10, Retries: 1}
	if errs := Validate(good); len(errs) != 0 {
		t.Errorf("valid monitor: %v", errs)
	}
	bad := store.Monitor{Type: "tcp", Target: "no-port", IntervalSeconds: 1, TimeoutSeconds: 0, Retries: 99}
	errs := Validate(bad)
	for _, k := range []string{"name", "target", "intervalSeconds", "timeoutSeconds", "retries"} {
		if errs[k] == "" {
			t.Errorf("no error for %s: %v", k, errs)
		}
	}
}

// TestEngineRetriesAndIncidents checks that a monitor tolerates its
// retries, then goes down, opens an incident and notifies, and that
// recovering resolves the incident.
func TestEngineRetriesAndIncidents(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := migrate.Up(ctx, st.DB, os.DirFS("../..")); err != nil {
		t.Fatal(err)
	}
	var code atomic.Int32
	code.Store(200)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(int(code.Load())) }))
	defer srv.Close()
	id, err := st.SaveMonitor(ctx, store.Monitor{Name: "API", Type: "http", Target: srv.URL, ExpectedStatus: "200-399", IntervalSeconds: 3600, TimeoutSeconds: 2, Retries: 1, Group: "x", Public: true})
	if err != nil {
		t.Fatal(err)
	}
	var changes []Change
	// Checks run one at a time here, without the engine's own loops.
	e := &Engine{Store: st, Notify: func(_ context.Context, c Change) { changes = append(changes, c) }}
	status := func() store.Status {
		m, _ := st.Monitor(ctx, id)
		return m.Status
	}
	e.CheckNow(ctx, id)
	if s := status(); s != store.Up {
		t.Fatalf("status = %s", s)
	}

	code.Store(500)
	e.CheckNow(ctx, id)
	if s := status(); s != store.Pending {
		t.Fatalf("after one failure (one retry allowed) status = %s, want pending", s)
	}
	e.CheckNow(ctx, id)
	if s := status(); s != store.Down {
		t.Fatalf("after two failures status = %s, want down", s)
	}
	if _, open, _ := st.OpenAutoIncident(ctx, id); !open {
		t.Error("going down opens an incident")
	}

	time.Sleep(10 * time.Millisecond)
	code.Store(200)
	e.CheckNow(ctx, id)
	if s := status(); s != store.Up {
		t.Fatalf("status = %s after recovering", s)
	}
	if _, open, _ := st.OpenAutoIncident(ctx, id); open {
		t.Error("recovering resolves the incident")
	}
	if len(changes) != 2 || changes[0].Status != store.Down || changes[1].Status != store.Up || changes[1].Downtime <= 0 {
		t.Errorf("notifications = %+v", changes)
	}
	u, _ := st.Uptime(ctx, id, time.Now().Add(-time.Hour))
	if u.Checks != 3 || u.Ratio < 0.66 || u.Ratio > 0.67 {
		t.Errorf("uptime = %+v (the pending check doesn't count)", u)
	}
}

package kit

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequestIDs(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	m := testManifest()
	m.Routes = append(m.Routes, RouteDef{ID: "/log", Endpoint: map[string]HandlerFunc{"GET": func(c *Ctx) error {
		c.Logger().Info("from ctx logger")
		slog.New(c.app.logger.Handler()).InfoContext(c, "from context")
		return c.Text(http.StatusOK, "ok")
	}}})
	a, err := New(m, WithLogger(logger), WithTrustedProxies("10.0.0.0/8"))
	if err != nil {
		t.Fatal(err)
	}

	get := func(peer, id, traceparent string) *httptest.ResponseRecorder {
		buf.Reset()
		r := httptest.NewRequest(http.MethodGet, "/log", nil)
		r.RemoteAddr = peer
		if id != "" {
			r.Header.Set(RequestIDHeader, id)
		}
		if traceparent != "" {
			r.Header.Set("Traceparent", traceparent)
		}
		w := httptest.NewRecorder()
		a.ServeHTTP(w, r)
		return w
	}
	records := func() []map[string]any {
		var out []map[string]any
		for _, l := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
			var m map[string]any
			if err := json.Unmarshal([]byte(l), &m); err != nil {
				t.Fatalf("%q: %v", l, err)
			}
			out = append(out, m)
		}
		return out
	}

	w := get("10.0.0.2:1", "abc-123", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	if got := w.Header().Get(RequestIDHeader); got != "abc-123" {
		t.Fatalf("response ID %q", got)
	}
	rs := records()
	if len(rs) != 2 {
		t.Fatalf("records %v", rs)
	}
	for _, r := range rs {
		if r["request_id"] != "abc-123" || r["trace_id"] != "4bf92f3577b34da6a3ce929d0e0e4736" || r["span_id"] != "00f067aa0ba902b7" {
			t.Errorf("record %v", r)
		}
	}
	if rs[0]["route"] != "/log" {
		t.Errorf("ctx logger has no route: %v", rs[0])
	}

	// An untrusted peer's ID and trace are ignored; a new ID is made.
	w = get("203.0.113.9:1", "abc-123", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	id := w.Header().Get(RequestIDHeader)
	if id == "" || id == "abc-123" {
		t.Fatalf("untrusted ID kept: %q", id)
	}
	if r := records()[0]; r["request_id"] != id || r["trace_id"] != nil {
		t.Errorf("record %v", r)
	}

	// Malformed IDs are replaced.
	if id := get("10.0.0.2:1", "bad id\n", "").Header().Get(RequestIDHeader); id == "bad id\n" || len(id) != 16 {
		t.Errorf("malformed ID: %q", id)
	}
}

func TestParseTraceparent(t *testing.T) {
	for in, ok := range map[string]bool{
		"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01":       true,
		"01-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01-extra": true,
		"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01-extra": false,
		"ff-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01":       false,
		"00-00000000000000000000000000000000-00f067aa0ba902b7-01":       false,
		"00-4BF92F3577B34DA6A3CE929D0E0E4736-00f067aa0ba902b7-01":       false,
		"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7":          false,
		"": false,
	} {
		if _, _, got := parseTraceparent(in); got != ok {
			t.Errorf("%q: %v", in, got)
		}
	}
}

func TestLogWith(t *testing.T) {
	var buf bytes.Buffer
	m := testManifest()
	m.Routes = append(m.Routes, RouteDef{ID: "/lw", Endpoint: map[string]HandlerFunc{"GET": func(c *Ctx) error {
		c.Logger().Info("one")
		slog.New(c.app.logger.Handler()).InfoContext(c, "two")
		return c.Text(http.StatusOK, "ok")
	}}})
	a, err := New(m, WithLogger(slog.New(slog.NewJSONHandler(&buf, nil))), WithHook(func(c *Ctx) error {
		c.LogWith(slog.String("user", "u1"))
		return nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	a.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/lw", nil))
	if n := strings.Count(buf.String(), `"user":"u1"`); n != 2 {
		t.Fatalf("%d records with the user: %s", n, buf.String())
	}
}

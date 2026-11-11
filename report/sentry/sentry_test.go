package sentry

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Unfield/Tern/report"
)

func TestParseDSN(t *testing.T) {
	ep, auth, err := parseDSN("https://abc123@o1.ingest.sentry.io/42")
	if err != nil || ep != "https://o1.ingest.sentry.io/api/42/envelope/" || !strings.Contains(auth, "sentry_key=abc123") {
		t.Fatalf("%s %s %v", ep, auth, err)
	}
	if ep, _, _ := parseDSN("http://k@localhost:8000/glitchtip/7"); ep != "http://localhost:8000/glitchtip/api/7/envelope/" {
		t.Error(ep)
	}
	for _, bad := range []string{"", "https://o1.ingest.sentry.io/42", "https://k@host/"} {
		if _, _, err := parseDSN(bad); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}

func TestSend(t *testing.T) {
	var body, auth, ctype string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body, auth, ctype = string(b), r.Header.Get("X-Sentry-Auth"), r.Header.Get("Content-Type")
	}))
	defer srv.Close()
	s := New(Options{DSN: strings.Replace(srv.URL, "://", "://key1@", 1) + "/9", ServerName: "web-1"})
	err := s.Send(context.Background(), []report.Report{{
		ID: "0123456789abcdef0123456789abcdef", Time: time.Unix(1794398400, 0), Source: "server",
		Type: "panic", Message: "boom", Panic: true, Group: "g1", Route: "/orders/id.param", Method: "POST", URL: "https://x/orders/1",
		Stack:     []report.Frame{{Function: "app.inner", File: "/a.go", Line: 3, InApp: true}, {Function: "app.outer", File: "/a.go", Line: 9, InApp: true}},
		RequestID: "r1", TraceID: "4bf92f3577b34da6a3ce929d0e0e4736", SpanID: "00f067aa0ba902b7", User: "u7", Release: "v1",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if ctype != "application/x-sentry-envelope" || !strings.Contains(auth, "sentry_key=key1") {
		t.Errorf("headers %s / %s", ctype, auth)
	}
	lines := strings.Split(strings.TrimSpace(body), "\n")
	if len(lines) != 3 || lines[1] != `{"type":"event"}` {
		t.Fatalf("envelope %q", body)
	}
	var ev map[string]any
	if err := json.Unmarshal([]byte(lines[2]), &ev); err != nil {
		t.Fatal(err)
	}
	exc := ev["exception"].(map[string]any)["values"].([]any)[0].(map[string]any)
	frames := exc["stacktrace"].(map[string]any)["frames"].([]any)
	if frames[0].(map[string]any)["function"] != "app.outer" || exc["mechanism"] == nil {
		t.Errorf("frames %v", frames) // oldest first
	}
	if ev["fingerprint"].([]any)[0] != "g1" || ev["transaction"] != "/orders/id.param" || ev["user"].(map[string]any)["id"] != "u7" {
		t.Errorf("event %v", ev)
	}
	if ev["contexts"].(map[string]any)["trace"].(map[string]any)["trace_id"] != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Error("trace")
	}
	if err := New(Options{}).Send(context.Background(), []report.Report{{}}); err == nil {
		t.Error("no DSN, no error")
	}
}

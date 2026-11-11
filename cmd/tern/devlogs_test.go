package main

import (
	"bufio"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Unfield/Tern/logging"
)

func TestDevLogs(t *testing.T) {
	l := newDevLogs()
	stop, err := l.listen()
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	// The app's side: a logger from logging, under tern dev.
	for _, kv := range l.env() {
		k, v, _ := strings.Cut(kv, "=")
		t.Setenv(k, v)
	}
	logger := logging.New(logging.Options{Output: discard{}})
	ctx := logging.With(context.Background(), slog.String(logging.RequestIDKey, "abc"))
	logger.InfoContext(ctx, "order placed", "items", 3, "password", "hunter2")
	if err := logging.Flush(context.Background(), logger); err != nil {
		t.Fatal(err)
	}

	// A request through the proxy.
	app := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Request-Id", "abc")
		w.WriteHeader(http.StatusCreated)
	})
	l.proxy(app, httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/orders?x=1&token=t", nil))

	srv := httptest.NewServer(http.HandlerFunc(l.serve))
	defer srv.Close()
	ctxT, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctxT, http.MethodGet, srv.URL+"/__tern/dev/logs/stream", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(nil, 1<<20)
	var items []devLogItem
	for sc.Scan() && len(items) < 2 {
		if data, ok := strings.CutPrefix(sc.Text(), "data: "); ok {
			var got []devLogItem
			if err := json.Unmarshal([]byte(data), &got); err != nil {
				t.Fatal(err)
			}
			items = append(items, got...)
		}
	}
	if len(items) != 2 {
		t.Fatalf("items %+v", items)
	}
	lg, rq := items[0], items[1]
	if lg.Kind != "log" || lg.Msg != "order placed" || lg.RequestID != "abc" || lg.Attrs["items"] != float64(3) || lg.Attrs["password"] != logging.Redacted {
		t.Errorf("log %+v", lg)
	}
	if rq.Kind != "request" || rq.Status != 201 || rq.Path != "/orders?token=[redacted]&x=1" || rq.RequestID != "abc" {
		t.Errorf("request %+v", rq)
	}

	// The ingest endpoint wants the token.
	bad, _ := http.Post(l.url, "application/json", strings.NewReader("[]"))
	if bad.StatusCode != http.StatusForbidden {
		t.Errorf("no token: %d", bad.StatusCode)
	}
	_ = bad.Body.Close()
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

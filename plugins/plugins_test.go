// Package plugins_test checks the built-in plugins against a real kit.App.
package plugins_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Unfield/Tern/kit"
	"github.com/Unfield/Tern/plugins/compress"
	"github.com/Unfield/Tern/plugins/health"
	"github.com/Unfield/Tern/plugins/requestlog"
	"github.com/Unfield/Tern/plugins/secure"
)

type slowData struct {
	Fast string               `json:"fast"`
	Slow kit.Deferred[string] `json:"slow"`
}

func app(t *testing.T, opts ...kit.Option) *kit.App {
	t.Helper()
	m := &kit.Manifest{
		AppHTML: `<html><head>%tern.head%</head><body>%tern.body%</body></html>`,
		Routes: []kit.RouteDef{
			{ID: "/", Page: &kit.PageDef{Load: kit.LoadOf(func(*kit.Ctx) (map[string]string, error) {
				return map[string]string{"text": strings.Repeat("compressible ", 200)}, nil
			})}},
			{ID: "/stream", Page: &kit.PageDef{Load: kit.LoadOf(func(c *kit.Ctx) (slowData, error) {
				return slowData{Fast: "now", Slow: kit.Defer(c, func() (string, error) {
					time.Sleep(150 * time.Millisecond)
					return "later", nil
				})}, nil
			})}},
			{ID: "/missing", Endpoint: map[string]kit.HandlerFunc{"GET": func(*kit.Ctx) error { return kit.NotFound() }}},
		},
	}
	opts = append([]kit.Option{kit.WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil)))}, opts...)
	a, err := kit.New(m, opts...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })
	return a
}

func get(t *testing.T, h http.Handler, path string, headers ...string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestHealth(t *testing.T) {
	dbErr := errors.New("connection refused")
	var failing bool
	a := app(t, kit.WithPlugins(health.New(
		health.Check("cache", func(context.Context) error { return nil }),
		health.Check("db", func(context.Context) error {
			if failing {
				return dbErr
			}
			return nil
		}),
	)))

	if rec := get(t, a, "/healthz"); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"ok"`) {
		t.Errorf("healthz = %d %s", rec.Code, rec.Body.String())
	}
	if rec := get(t, a, "/readyz"); rec.Code != 200 {
		t.Errorf("readyz = %d %s", rec.Code, rec.Body.String())
	}
	failing = true
	rec := get(t, a, "/readyz")
	var body struct {
		Status string `json:"status"`
		Checks map[string]struct {
			Status, Error string
		} `json:"checks"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if rec.Code != 503 || body.Checks["db"].Error != "connection refused" || body.Checks["cache"].Status != "ok" {
		t.Errorf("failing readyz = %d %s", rec.Code, rec.Body.String())
	}

	failing = false
	_ = a.Close()
	if rec := get(t, a, "/readyz"); rec.Code != 503 || !strings.Contains(rec.Body.String(), "shutting down") {
		t.Errorf("readyz during shutdown = %d %s", rec.Code, rec.Body.String())
	}
}

func TestRequestLog(t *testing.T) {
	var buf bytes.Buffer
	a := app(t, kit.WithPlugins(requestlog.New(requestlog.Logger(slog.New(slog.NewTextHandler(&buf, nil))), requestlog.SkipAssets())))
	get(t, a, "/")
	get(t, a, "/missing", "Accept", "application/json")
	get(t, a, "/_app/immutable/x.js")
	logs := buf.String()
	if !strings.Contains(logs, "level=INFO msg=request method=GET path=/ status=200") {
		t.Errorf("missing page log:\n%s", logs)
	}
	if !strings.Contains(logs, "level=WARN msg=request method=GET path=/missing status=404") {
		t.Errorf("missing 404 log:\n%s", logs)
	}
	if strings.Contains(logs, "/_app/") {
		t.Errorf("assets should be skipped:\n%s", logs)
	}
}

func TestCompress(t *testing.T) {
	a := app(t, kit.WithPlugins(compress.New()))

	rec := get(t, a, "/", "Accept-Encoding", "gzip")
	if rec.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("page not compressed: %v", rec.Header())
	}
	zr, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatal(err)
	}
	html, _ := io.ReadAll(zr)
	if !strings.Contains(string(html), "compressible compressible") {
		t.Error("decompressed body is wrong")
	}

	if rec := get(t, a, "/"); rec.Header().Get("Content-Encoding") != "" {
		t.Error("clients without gzip must get plain responses")
	}

	// Streaming still flushes: the first chunk arrives before the deferred value.
	srv := httptest.NewServer(a)
	defer srv.Close()
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/stream/__data.json", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	start := time.Now()
	resp, err := http.DefaultTransport.RoundTrip(req) // no transparent decompression
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	zr, err = gzip.NewReader(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	first := make([]byte, 64)
	if _, err := io.ReadAtLeast(zr, first, 10); err != nil {
		t.Fatal(err)
	}
	if took := time.Since(start); took > 120*time.Millisecond {
		t.Errorf("first chunk took %v; compression must not buffer streamed responses", took)
	}
	rest, _ := io.ReadAll(zr)
	if !strings.Contains(string(rest), `"data":"later"`) {
		t.Errorf("streamed value missing: %s", rest)
	}
}

func TestSecure(t *testing.T) {
	a := app(t, kit.WithPlugins(secure.New(secure.HSTS(365*24*time.Hour, true), secure.Header("Permissions-Policy", ""))))
	h := get(t, a, "/").Header()
	if h.Get("X-Frame-Options") != "DENY" || h.Get("Referrer-Policy") != "strict-origin-when-cross-origin" {
		t.Errorf("defaults missing: %v", h)
	}
	if h.Get("Strict-Transport-Security") != "max-age=31536000; includeSubDomains" {
		t.Errorf("HSTS = %q", h.Get("Strict-Transport-Security"))
	}
	if h.Get("Permissions-Policy") != "" {
		t.Error("Header(name, \"\") should remove a default")
	}
}

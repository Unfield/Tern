package kit_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Unfield/Tern/kit"
)

type cachedPage struct {
	N      int64  `json:"n"`
	Cookie string `json:"cookie"`
}

func cacheApp(t *testing.T) (*kit.App, *atomic.Int64) {
	t.Helper()
	var loads atomic.Int64
	load := func(c *kit.Ctx) (cachedPage, error) {
		c.Tag("posts")
		ck, _ := c.Cookie("session")
		c.SetCookie(&http.Cookie{Name: "seen", Value: "1"})
		return cachedPage{N: loads.Add(1), Cookie: ck}, nil
	}
	m := &kit.Manifest{AppHTML: `<html><head>%tern.head%</head><body>%tern.body%</body></html>`, Routes: []kit.RouteDef{
		{ID: "/", Page: &kit.PageDef{Load: kit.LoadOf(load)}, Revalidate: time.Hour, CacheControl: "public, max-age=60"},
		{ID: "/forever", Page: &kit.PageDef{Load: kit.LoadOf(load)}, Revalidate: kit.Forever},
		{ID: "/static", Page: &kit.PageDef{Load: kit.LoadOf(func(*kit.Ctx) (cachedPage, error) { return cachedPage{N: 7}, nil }), NoCSR: true}},
		{ID: "/missing", Page: &kit.PageDef{Load: kit.LoadOf(func(*kit.Ctx) (cachedPage, error) { loads.Add(1); return cachedPage{}, kit.NotFound() })}, Revalidate: time.Hour},
	}}
	app, err := kit.New(m, kit.WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))), kit.WithContentSecurityPolicy("script-src 'self'"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	return app, &loads
}

// answer is a response, its body read.
type answer struct {
	StatusCode int
	Header     http.Header
	Text       string
}

func fetch(t *testing.T, app http.Handler, path string, cookie string) answer {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Accept", "text/html")
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	return answer{StatusCode: rec.Code, Header: rec.Header(), Text: rec.Body.String()}
}

var nonceAttr = regexp.MustCompile(`nonce="([^"]+)"`)

func TestPageCache(t *testing.T) {
	app, loads := cacheApp(t)

	first := fetch(t, app, "/", "session=secret")
	b1 := first.Text
	if first.Header.Get("X-Tern-Cache") != "MISS" || !strings.Contains(b1, `"n":1`) {
		t.Fatalf("first: %s %s", first.Header.Get("X-Tern-Cache"), b1)
	}
	// Rendered as anyone sees it: no visitor's cookie in, none set out.
	if strings.Contains(b1, "secret") || first.Header.Get("Set-Cookie") != "" {
		t.Fatalf("the cached page has the visitor's cookie: %s / %q", b1, first.Header.Get("Set-Cookie"))
	}
	if first.Header.Get("Cache-Control") != "public, max-age=60" {
		t.Errorf("Cache-Control %q", first.Header.Get("Cache-Control"))
	}

	second := fetch(t, app, "/", "")
	b2 := second.Text
	if second.Header.Get("X-Tern-Cache") != "HIT" || !strings.Contains(b2, `"n":1`) || loads.Load() != 1 {
		t.Fatalf("second: %s, %d loads", second.Header.Get("X-Tern-Cache"), loads.Load())
	}
	// Each copy gets a nonce of its own, the same in the header and page.
	n1, n2 := nonceAttr.FindStringSubmatch(b1), nonceAttr.FindStringSubmatch(b2)
	if n1 == nil || n2 == nil || n1[1] == n2[1] || strings.Contains(b2, "tern-cache-nonce") {
		t.Fatalf("nonces %v %v", n1, n2)
	}
	if csp := second.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "'nonce-"+n2[1]+"'") {
		t.Fatalf("CSP %q, page nonce %s", csp, n2[1])
	}

	// The client's data is cached apart from the document.
	data := fetch(t, app, "/__data.json", "")
	if data.Header.Get("X-Tern-Cache") != "MISS" || data.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("data: %s %s", data.Header.Get("X-Tern-Cache"), data.Header.Get("Content-Type"))
	}

	// Revalidating a tag: the stale copy is served while a new one renders.
	if err := app.Revalidate(t.Context(), "posts"); err != nil {
		t.Fatal(err)
	}
	stale := fetch(t, app, "/", "")
	if stale.Header.Get("X-Tern-Cache") != "STALE" || !strings.Contains(stale.Text, `"n":1`) {
		t.Fatalf("after revalidating: %s", stale.Header.Get("X-Tern-Cache"))
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		r := fetch(t, app, "/", "")
		b := r.Text
		if r.Header.Get("X-Tern-Cache") == "HIT" && !strings.Contains(b, `"n":1`) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the page was never rendered again")
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Forever: cached until revalidated, then rendered at once.
	fetch(t, app, "/forever", "")
	if r := fetch(t, app, "/forever", ""); r.Header.Get("X-Tern-Cache") != "HIT" {
		t.Fatalf("forever: %s", r.Header.Get("X-Tern-Cache"))
	}
	before := loads.Load()
	_ = app.Revalidate(t.Context(), "posts")
	if r := fetch(t, app, "/forever", ""); r.Header.Get("X-Tern-Cache") != "MISS" || loads.Load() == before {
		t.Fatalf("forever after revalidating: %s", r.Header.Get("X-Tern-Cache"))
	}

	// Errors are served, never stored.
	for range 2 {
		if r := fetch(t, app, "/missing", ""); r.StatusCode != http.StatusNotFound || r.Header.Get("X-Tern-Cache") != "MISS" {
			t.Fatalf("missing: %d %s", r.StatusCode, r.Header.Get("X-Tern-Cache"))
		}
	}
}

func TestPagesWithoutJavaScript(t *testing.T) {
	app, _ := cacheApp(t)
	r := fetch(t, app, "/static", "")
	if b := r.Text; strings.Contains(b, "__tern_data") || strings.Contains(b, "<script") {
		t.Fatalf("a page without JavaScript has scripts: %s", b)
	}
	// The router asks for its data on a client navigation; an answer that
	// isn't JSON makes it load the page in full.
	if r := fetch(t, app, "/static/__data.json", ""); r.StatusCode != http.StatusNoContent {
		t.Fatalf("data: %d", r.StatusCode)
	}
	if r := fetch(t, app, "/forever", ""); !strings.Contains(r.Text, "__tern_data") {
		t.Error("other pages lost their data")
	}
}

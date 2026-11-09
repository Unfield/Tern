package kit

import (
	"net/http"
	"strings"
	"testing"
	"testing/fstest"
)

func TestBasePath(t *testing.T) {
	m := testManifest()
	m.BasePath = "/app"
	m.AppHTML = `<html><head><link rel="icon" href="%tern.assets%/favicon.svg">%tern.head%</head><body>%tern.body%</body></html>`
	m.ClientEntry = "entry.js"
	m.Client = fstest.MapFS{
		".vite/manifest.json":     {Data: []byte(`{"entry.js": {"file": "_app/immutable/entry.js", "isEntry": true}}`)},
		"_app/immutable/entry.js": {Data: []byte("entry")},
		"favicon.svg":             {Data: []byte("<svg/>")},
	}
	m.Routes[2].Page.Load = LoadOf(func(c *Ctx) (map[string]string, error) {
		return map[string]string{"link": c.Path("/blog/x")}, Redirect(http.StatusSeeOther, "/login")
	}) // "/scalar" now redirects
	a, _ := newTestApp(t, m)

	rec := do(t, a, "GET", "/app/blog/hello", nil)
	if rec.Code != 200 {
		t.Fatalf("page under base: %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{`href="/app/favicon.svg"`, `src="/app/_app/immutable/entry.js"`} {
		if !strings.Contains(body, want) {
			t.Errorf("page missing %s", want)
		}
	}
	if rec := do(t, a, "GET", "/app/_app/immutable/entry.js", nil); rec.Body.String() != "entry" {
		t.Errorf("asset under base: %d", rec.Code)
	}
	if rec := do(t, a, "GET", "/app", nil); rec.Code != 200 {
		t.Errorf("base itself is the home page: %d", rec.Code)
	}
	if rec := do(t, a, "GET", "/", nil); rec.Code != http.StatusFound || rec.Header().Get("Location") != "/app" {
		t.Errorf("root redirects to base: %d %q", rec.Code, rec.Header().Get("Location"))
	}
	if rec := do(t, a, "GET", "/blog/hello", nil); rec.Code != 404 {
		t.Errorf("paths outside base: %d", rec.Code)
	}
	if rec := do(t, a, "GET", "/app/scalar", nil); rec.Header().Get("Location") != "/app/login" {
		t.Errorf("redirects get the base: %q", rec.Header().Get("Location"))
	}
	if p := jsonPayload(t, do(t, a, "GET", "/app/scalar/__data.json", nil)); p.Location != "/app/login" {
		t.Errorf("data redirects get the base: %q", p.Location)
	}
	for in, want := range map[string]string{"/": "/app", "/?a=1": "/app?a=1", "/login": "/app/login", "/app/x": "/app/x", "https://x.example/": "https://x.example/", "//cdn": "//cdn"} {
		if got := a.withBase(in); got != want {
			t.Errorf("withBase(%q) = %q, want %q", in, got, want)
		}
	}
	if rec := do(t, a, "GET", "/app/blog/hello/", nil); rec.Header().Get("Location") != "/app/blog/hello" {
		t.Errorf("trailing slash redirect keeps the base: %q", rec.Header().Get("Location"))
	}
}

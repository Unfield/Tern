package kit

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
)

const testHTML = `<!doctype html><html><head>%tern.head%</head><body>%tern.body%</body></html>`

type user struct {
	Name string `json:"name"`
}

type layoutData struct {
	User *user `json:"user"`
}

type postData struct {
	Slug     string `json:"slug"`
	Greeting string `json:"greeting"`
}

func testManifest() *Manifest {
	return &Manifest{
		AppHTML: testHTML,
		Layouts: []LayoutDef{
			{Dir: "", Component: "src/routes/+layout.svelte", Load: LoadOf(func(c *Ctx) (layoutData, error) {
				u, _ := c.Locals["user"].(*user)
				return layoutData{User: u}, nil
			})},
			{Dir: "blog", Component: "src/routes/blog/+layout.svelte"},
		},
		Fallback: FallbackDef{Layouts: []int{0}},
		Routes: []RouteDef{
			{ID: "/", Layouts: []int{0}, Page: &PageDef{Component: "src/routes/+page.svelte"}},
			{ID: "/blog/slug.param", Layouts: []int{0, 1}, Page: &PageDef{
				Component: "src/routes/blog/slug.param/+page.svelte",
				Load: LoadOf(func(c *Ctx) (postData, error) {
					switch c.Param("slug") {
					case "missing":
						return postData{}, NotFound()
					case "moved":
						return postData{}, Redirect(http.StatusMovedPermanently, "/blog/new-home")
					case "broken":
						return postData{}, errors.New("database is down")
					case "panic":
						panic("boom")
					}
					p, err := Parent[layoutData](c)
					if err != nil {
						return postData{}, err
					}
					greeting := "hello stranger"
					if p.User != nil {
						greeting = "hello " + p.User.Name
					}
					c.SetCookie(&http.Cookie{Name: "seen", Value: c.Param("slug")})
					return postData{Slug: c.Param("slug"), Greeting: greeting}, nil
				}),
			}},
			{ID: "/scalar", Page: &PageDef{Component: "src/routes/scalar/+page.svelte", Load: LoadOf(func(*Ctx) (int, error) { return 42, nil })}},
			{ID: "/todos", Layouts: []int{0}, Page: &PageDef{
				Component: "src/routes/todos/+page.svelte",
				Load:      LoadOf(func(*Ctx) (map[string]int, error) { return map[string]int{"count": 1}, nil }),
				Actions: Actions{
					"default": Action(func(c *Ctx) (map[string]string, error) {
						form, err := c.Form()
						if err != nil {
							return nil, err
						}
						title := form.Get("title")
						if title == "" {
							return nil, Fail(http.StatusBadRequest, map[string]string{"error": "title is required"})
						}
						return map[string]string{"created": title}, nil
					}),
					"clear": func(*Ctx) (any, error) { return nil, Redirect(http.StatusSeeOther, "/") },
				},
			}},
			{ID: "/api/items/id.param.integer", Endpoint: map[string]HandlerFunc{
				"GET": func(c *Ctx) error { return c.JSON(http.StatusOK, map[string]string{"id": c.Param("id")}) },
				"DELETE": func(c *Ctx) error {
					if c.Param("id") == "0" {
						return Error(http.StatusConflict, "cannot delete item 0")
					}
					return nil
				},
			}},
			{ID: "/feed", Page: &PageDef{Component: "src/routes/feed/+page.svelte"}, Endpoint: map[string]HandlerFunc{
				"GET": func(c *Ctx) error { return c.Text(http.StatusOK, "feed endpoint") },
			}},
		},
	}
}

func newTestApp(t *testing.T, m *Manifest, opts ...Option) (*App, *bytes.Buffer) {
	t.Helper()
	logs := &bytes.Buffer{}
	opts = append([]Option{WithLogger(slog.New(slog.NewTextHandler(logs, nil)))}, opts...)
	a, err := New(m, opts...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return a, logs
}

func do(t *testing.T, h http.Handler, method, target string, body io.Reader, headers ...string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, body)
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

var dataScriptRe = regexp.MustCompile(`<script type="application/json" id="__tern_data" nonce="[^"]+">(.*?)</script>`)

// pagePayload extracts the inline payload from an HTML response.
func pagePayload(t *testing.T, rec *httptest.ResponseRecorder) payload {
	t.Helper()
	m := dataScriptRe.FindStringSubmatch(rec.Body.String())
	if m == nil {
		t.Fatalf("no data script in response:\n%s", rec.Body.String())
	}
	var p payload
	if err := json.Unmarshal([]byte(m[1]), &p); err != nil {
		t.Fatalf("decoding payload: %v", err)
	}
	return p
}

func jsonPayload(t *testing.T, rec *httptest.ResponseRecorder) payload {
	t.Helper()
	var p payload
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("decoding %q: %v", rec.Body.String(), err)
	}
	return p
}

func TestPage(t *testing.T) {
	a, _ := newTestApp(t, testManifest(), WithHook(func(c *Ctx) error {
		if c.Header("Authorization") == "alice" {
			c.Locals["user"] = &user{Name: "alice"}
		}
		return nil
	}))

	rec := do(t, a, "GET", "http://example.com/blog/hello", nil, "Authorization", "alice")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body:\n%s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q", ct)
	}
	if !strings.Contains(rec.Body.String(), `<div id="tern-root" style="display: contents"></div>`) {
		t.Error("missing mount point")
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("missing nosniff header")
	}

	p := pagePayload(t, rec)
	if p.Type != "data" || p.Route != "/blog/slug.param" || p.Params["slug"] != "hello" || p.Status != 200 {
		t.Errorf("payload = %+v", p)
	}
	if len(p.Nodes) != 3 {
		t.Fatalf("nodes = %d, want 3 (two layouts and the page)", len(p.Nodes))
	}
	if got := string(p.Nodes[0]); got != `{"user":{"name":"alice"}}` {
		t.Errorf("root layout node = %s", got)
	}
	if p.Nodes[1] != nil && string(p.Nodes[1]) != "null" {
		t.Errorf("layout without load should be null, got %s", p.Nodes[1])
	}
	if got := string(p.Nodes[2]); got != `{"slug":"hello","greeting":"hello alice"}` {
		t.Errorf("page node = %s", got)
	}

	cookie := rec.Header().Get("Set-Cookie")
	for _, want := range []string{"seen=hello", "Path=/", "Secure", "SameSite=Lax"} {
		if !strings.Contains(cookie, want) {
			t.Errorf("Set-Cookie %q missing %q", cookie, want)
		}
	}
}

func TestDataRequest(t *testing.T) {
	a, _ := newTestApp(t, testManifest())

	rec := do(t, a, "GET", "/blog/hello/__data.json", nil)
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("status = %d, type = %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	p := jsonPayload(t, rec)
	if p.Type != "data" || len(p.Nodes) != 3 || !strings.Contains(string(p.Nodes[2]), "hello stranger") {
		t.Errorf("payload = %+v", p)
	}

	root := jsonPayload(t, do(t, a, "GET", "/__data.json", nil))
	if root.Type != "data" || root.Route != "/" {
		t.Errorf("root data = %+v", root)
	}

	moved := jsonPayload(t, do(t, a, "GET", "/blog/moved/__data.json", nil))
	if moved.Type != "redirect" || moved.Location != "/blog/new-home" || moved.Status != 301 {
		t.Errorf("redirect data = %+v", moved)
	}

	missing := jsonPayload(t, do(t, a, "GET", "/nope/__data.json", nil))
	if missing.Type != "error" || missing.Status != 404 {
		t.Errorf("404 data = %+v", missing)
	}
}

func TestTrailingSlashRedirect(t *testing.T) {
	a, _ := newTestApp(t, testManifest())
	rec := do(t, a, "GET", "/blog/hello/?x=1", nil)
	if rec.Code != http.StatusPermanentRedirect || rec.Header().Get("Location") != "/blog/hello?x=1" {
		t.Errorf("status = %d, Location = %q", rec.Code, rec.Header().Get("Location"))
	}
}

func TestErrors(t *testing.T) {
	a, logs := newTestApp(t, testManifest())

	t.Run("no route", func(t *testing.T) {
		rec := do(t, a, "GET", "/does/not/exist", nil)
		p := pagePayload(t, rec)
		if rec.Code != 404 || p.Type != "error" || p.Error.Message != "Not Found" || p.Route != "" {
			t.Errorf("status = %d, payload = %+v", rec.Code, p)
		}
		if len(p.Nodes) != 1 || p.Failed == nil || *p.Failed != 1 {
			t.Errorf("fallback should render inside the loaded root layout: nodes=%v failed=%v", p.Nodes, p.Failed)
		}
	})

	t.Run("expected error", func(t *testing.T) {
		rec := do(t, a, "GET", "/blog/missing", nil)
		p := pagePayload(t, rec)
		if rec.Code != 404 || p.Route != "/blog/slug.param" || *p.Failed != 2 || len(p.Nodes) != 2 {
			t.Errorf("status = %d, payload = %+v", rec.Code, p)
		}
	})

	t.Run("redirect", func(t *testing.T) {
		rec := do(t, a, "GET", "/blog/moved", nil)
		if rec.Code != 301 || rec.Header().Get("Location") != "/blog/new-home" {
			t.Errorf("status = %d, Location = %q", rec.Code, rec.Header().Get("Location"))
		}
	})

	t.Run("unexpected error is hidden and logged", func(t *testing.T) {
		logs.Reset()
		rec := do(t, a, "GET", "/blog/broken", nil)
		p := pagePayload(t, rec)
		if rec.Code != 500 || p.Error.Message != "Internal Error" {
			t.Errorf("status = %d, payload = %+v", rec.Code, p)
		}
		if strings.Contains(rec.Body.String(), "database is down") {
			t.Error("internal error message leaked to the client")
		}
		if !strings.Contains(logs.String(), "database is down") {
			t.Errorf("error not logged: %s", logs.String())
		}
	})

	t.Run("panic is recovered", func(t *testing.T) {
		logs.Reset()
		rec := do(t, a, "GET", "/blog/panic", nil)
		if rec.Code != 500 || !strings.Contains(logs.String(), "boom") {
			t.Errorf("status = %d, logs = %s", rec.Code, logs.String())
		}
	})

	t.Run("load data must be an object", func(t *testing.T) {
		rec := do(t, a, "GET", "/scalar", nil)
		if rec.Code != 500 {
			t.Errorf("status = %d", rec.Code)
		}
	})
}

func postForm(t *testing.T, h http.Handler, target string, form url.Values, headers ...string) *httptest.ResponseRecorder {
	t.Helper()
	headers = append([]string{"Content-Type", "application/x-www-form-urlencoded"}, headers...)
	return do(t, h, "POST", target, strings.NewReader(form.Encode()), headers...)
}

func TestActions(t *testing.T) {
	a, _ := newTestApp(t, testManifest())

	t.Run("success without JavaScript re-renders with form", func(t *testing.T) {
		rec := postForm(t, a, "/todos", url.Values{"title": {"milk"}})
		p := pagePayload(t, rec)
		if rec.Code != 200 || !p.HasForm || p.Form.(map[string]any)["created"] != "milk" {
			t.Errorf("status = %d, payload = %+v", rec.Code, p)
		}
		if len(p.Nodes) != 2 {
			t.Errorf("page should re-run its loads, nodes = %v", p.Nodes)
		}
	})

	t.Run("failure without JavaScript", func(t *testing.T) {
		rec := postForm(t, a, "/todos", url.Values{})
		p := pagePayload(t, rec)
		if rec.Code != 400 || p.Form.(map[string]any)["error"] != "title is required" {
			t.Errorf("status = %d, payload = %+v", rec.Code, p)
		}
	})

	t.Run("enhanced success, failure and redirect", func(t *testing.T) {
		ok := jsonPayload(t, postForm(t, a, "/todos", url.Values{"title": {"eggs"}}, "X-Tern-Action", "true"))
		if ok.Type != "success" || ok.Data.(map[string]any)["created"] != "eggs" {
			t.Errorf("success = %+v", ok)
		}
		bad := jsonPayload(t, postForm(t, a, "/todos", url.Values{}, "X-Tern-Action", "true"))
		if bad.Type != "failure" || bad.Status != 400 {
			t.Errorf("failure = %+v", bad)
		}
		redir := jsonPayload(t, postForm(t, a, "/todos?/clear", url.Values{}, "X-Tern-Action", "true"))
		if redir.Type != "redirect" || redir.Location != "/" || redir.Status != 303 {
			t.Errorf("redirect = %+v", redir)
		}
	})

	t.Run("named action redirect without JavaScript", func(t *testing.T) {
		rec := postForm(t, a, "/todos?/clear", url.Values{})
		if rec.Code != 303 || rec.Header().Get("Location") != "/" {
			t.Errorf("status = %d, Location = %q", rec.Code, rec.Header().Get("Location"))
		}
	})

	t.Run("unknown action", func(t *testing.T) {
		if rec := postForm(t, a, "/todos?/nope", url.Values{}); rec.Code != 404 {
			t.Errorf("status = %d", rec.Code)
		}
	})

	t.Run("page without actions", func(t *testing.T) {
		rec := postForm(t, a, "/", url.Values{})
		if rec.Code != 405 || rec.Header().Get("Allow") != "GET, HEAD" {
			t.Errorf("status = %d, Allow = %q", rec.Code, rec.Header().Get("Allow"))
		}
	})

	t.Run("non-form body", func(t *testing.T) {
		rec := do(t, a, "POST", "/todos", strings.NewReader("{}"), "Content-Type", "application/json")
		if rec.Code != 415 {
			t.Errorf("status = %d", rec.Code)
		}
	})

	t.Run("body too large", func(t *testing.T) {
		small, _ := newTestApp(t, testManifest(), WithBodyLimit(10))
		rec := postForm(t, small, "/todos", url.Values{"title": {strings.Repeat("x", 100)}})
		if rec.Code != 413 {
			t.Errorf("status = %d", rec.Code)
		}
	})
}

func TestCrossOriginProtection(t *testing.T) {
	a, _ := newTestApp(t, testManifest())
	rec := postForm(t, a, "http://example.com/todos", url.Values{"title": {"x"}}, "Sec-Fetch-Site", "cross-site")
	if rec.Code != 403 {
		t.Errorf("cross-site POST status = %d, want 403", rec.Code)
	}
	rec = postForm(t, a, "http://example.com/todos", url.Values{"title": {"x"}}, "Sec-Fetch-Site", "same-origin")
	if rec.Code != 200 {
		t.Errorf("same-origin POST status = %d, want 200", rec.Code)
	}

	trusted, _ := newTestApp(t, testManifest(), WithTrustedOrigins("https://partner.example"))
	rec = postForm(t, trusted, "http://example.com/todos", url.Values{"title": {"x"}}, "Origin", "https://partner.example", "Sec-Fetch-Site", "cross-site")
	if rec.Code != 200 {
		t.Errorf("trusted origin POST status = %d, want 200", rec.Code)
	}
}

func TestEndpoints(t *testing.T) {
	a, _ := newTestApp(t, testManifest())

	rec := do(t, a, "GET", "/api/items/7", nil)
	if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != `{"id":"7"}` {
		t.Errorf("GET status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if rec := do(t, a, "GET", "/api/items/abc", nil); rec.Code != 404 {
		t.Errorf("matcher should reject non-integers: status = %d", rec.Code)
	}
	if rec := do(t, a, "HEAD", "/api/items/7", nil); rec.Code != 200 {
		t.Errorf("HEAD should fall back to GET: status = %d", rec.Code)
	}
	if rec := do(t, a, "DELETE", "/api/items/7", nil); rec.Code != 204 {
		t.Errorf("DELETE with no body should be 204: status = %d", rec.Code)
	}
	rec = do(t, a, "DELETE", "/api/items/0", nil)
	if rec.Code != 409 || !strings.Contains(rec.Body.String(), "cannot delete item 0") {
		t.Errorf("error status = %d, body = %s", rec.Code, rec.Body.String())
	}
	rec = do(t, a, "PUT", "/api/items/7", nil)
	if rec.Code != 405 || rec.Header().Get("Allow") != "DELETE, GET, HEAD" {
		t.Errorf("PUT status = %d, Allow = %q", rec.Code, rec.Header().Get("Allow"))
	}

	// A route with a page and a GET endpoint negotiates on Accept.
	if rec := do(t, a, "GET", "/feed", nil, "Accept", "text/html,*/*"); !strings.Contains(rec.Body.String(), "__tern_data") {
		t.Errorf("browser request should get the page: %s", rec.Body.String())
	}
	if rec := do(t, a, "GET", "/feed", nil, "Accept", "application/json"); rec.Body.String() != "feed endpoint" {
		t.Errorf("API request should get the endpoint: %s", rec.Body.String())
	}
}

func TestHookRedirect(t *testing.T) {
	a, _ := newTestApp(t, testManifest(), WithHook(func(c *Ctx) error {
		if strings.HasPrefix(c.URL().Path, "/todos") {
			return Redirect(http.StatusSeeOther, "/login")
		}
		return nil
	}))
	if rec := do(t, a, "GET", "/todos", nil); rec.Code != 303 || rec.Header().Get("Location") != "/login" {
		t.Errorf("status = %d, Location = %q", rec.Code, rec.Header().Get("Location"))
	}
	p := jsonPayload(t, do(t, a, "GET", "/todos/__data.json", nil))
	if p.Type != "redirect" || p.Location != "/login" {
		t.Errorf("data redirect = %+v", p)
	}
}

func TestMiddleware(t *testing.T) {
	var order []string
	mw := func(name string) func(http.Handler) http.Handler {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, name)
				next.ServeHTTP(w, r)
			})
		}
	}
	a, _ := newTestApp(t, testManifest(), WithMiddleware(mw("outer"), mw("inner")))
	do(t, a, "GET", "/", nil)
	if strings.Join(order, ",") != "outer,inner" {
		t.Errorf("order = %v", order)
	}
}

func TestNewErrors(t *testing.T) {
	m := testManifest()
	m.Routes = append(m.Routes, RouteDef{ID: "/x/id.param.nope", Page: &PageDef{}})
	if _, err := New(m); err == nil || !strings.Contains(err.Error(), `unknown matcher "nope"`) {
		t.Errorf("err = %v", err)
	}

	m = testManifest()
	m.Routes[0].Layouts = []int{9}
	if _, err := New(m); err == nil || !strings.Contains(err.Error(), "layout 9") {
		t.Errorf("err = %v", err)
	}

	m = testManifest()
	m.AppHTML = "<html></html>"
	if _, err := New(m); err == nil || !strings.Contains(err.Error(), "%tern.head%") {
		t.Errorf("err = %v", err)
	}
}

func TestAssets(t *testing.T) {
	m := testManifest()
	m.ClientEntry = ".tern/generated/client.js"
	m.Client = fstest.MapFS{
		".vite/manifest.json": {Data: []byte(`{
			".tern/generated/client.js": {"file": "_app/immutable/entry-abc.js", "isEntry": true, "imports": ["_shared"], "css": ["_app/immutable/entry.css"]},
			"_shared": {"file": "_app/immutable/shared-123.js"},
			"src/routes/+page.svelte": {"file": "_app/immutable/page-456.js", "imports": ["_shared"], "css": ["_app/immutable/page.css"]}
		}`)},
		"_app/immutable/entry-abc.js":  {Data: []byte("entry")},
		"_app/immutable/shared-123.js": {Data: []byte("shared")},
		"_app/immutable/page-456.js":   {Data: []byte("page")},
		"robots.txt":                   {Data: []byte("User-agent: *")},
		"robots.txt.gz":                {Data: []byte("gzipped")},
	}
	a, _ := newTestApp(t, m)

	rec := do(t, a, "GET", "/_app/immutable/entry-abc.js", nil)
	if rec.Body.String() != "entry" || rec.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Errorf("immutable asset: body=%q cache=%q", rec.Body.String(), rec.Header().Get("Cache-Control"))
	}
	if rec := do(t, a, "GET", "/_app/immutable/gone.js", nil); rec.Code != 404 {
		t.Errorf("missing immutable asset status = %d", rec.Code)
	}
	if rec := do(t, a, "GET", "/.vite/manifest.json", nil); rec.Code != 404 {
		t.Errorf("build manifest must not be served: status = %d", rec.Code)
	}

	rec = do(t, a, "GET", "/robots.txt", nil)
	etag := rec.Header().Get("ETag")
	if rec.Body.String() != "User-agent: *" || etag == "" || rec.Header().Get("Cache-Control") != "public, max-age=0, must-revalidate" {
		t.Errorf("static asset: body=%q etag=%q cache=%q", rec.Body.String(), etag, rec.Header().Get("Cache-Control"))
	}
	if rec := do(t, a, "GET", "/robots.txt", nil, "If-None-Match", etag); rec.Code != 304 {
		t.Errorf("conditional request status = %d, want 304", rec.Code)
	}
	rec = do(t, a, "GET", "/robots.txt", nil, "Accept-Encoding", "gzip, br")
	if rec.Body.String() != "gzipped" || rec.Header().Get("Content-Encoding") != "gzip" {
		t.Errorf("gzip: body=%q encoding=%q", rec.Body.String(), rec.Header().Get("Content-Encoding"))
	}

	body := do(t, a, "GET", "/", nil).Body.String()
	for _, want := range []string{
		`<link rel="stylesheet" href="/_app/immutable/entry.css">`,
		`<link rel="stylesheet" href="/_app/immutable/page.css">`,
		`<link rel="modulepreload" href="/_app/immutable/shared-123.js">`,
		`<link rel="modulepreload" href="/_app/immutable/page-456.js">`,
		`<script type="module" src="/_app/immutable/entry-abc.js"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page head missing %s\n%s", want, body)
		}
	}
	if strings.Count(body, "shared-123.js") != 1 {
		t.Error("shared chunk should be preloaded once")
	}
}

func TestShell(t *testing.T) {
	s, err := parseShell(`<html lang="en"><head>%tern.head%<meta nonce="%tern.nonce%"></head><body>%tern.body%</body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	got := s.render("<HEAD>", "<BODY>", "N", "", "en")
	want := `<html lang="en"><head><HEAD><meta nonce="N"></head><body><BODY></body></html>`
	if got != want {
		t.Errorf("render = %q, want %q", got, want)
	}
}

func TestCookieSecureOnLocalhost(t *testing.T) {
	a, _ := newTestApp(t, testManifest())
	rec := do(t, a, "GET", "http://localhost:5173/blog/hi", nil)
	if c := rec.Header().Get("Set-Cookie"); strings.Contains(c, "Secure") {
		t.Errorf("cookie on plain-HTTP localhost should not be Secure: %q", c)
	}
}

func TestPrerenderList(t *testing.T) {
	t.Setenv("TERN_PRERENDER", "1")
	m := testManifest()
	m.Routes[0].Prerender = true // "/"
	m.Routes[1].Prerender = true // "/blog/slug.param"
	m.Routes[1].Entries = func(*Ctx) ([]map[string]string, error) {
		return []map[string]string{{"slug": "first"}, {"slug": "second post"}}, nil
	}
	a, _ := newTestApp(t, m)
	var out struct {
		Paths  []string `json:"paths"`
		Errors []string `json:"errors"`
	}
	if err := json.Unmarshal(do(t, a, "GET", "/__tern/prerender", nil).Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	want := []string{"/", "/blog/first", "/blog/second%20post"}
	if strings.Join(out.Paths, ",") != strings.Join(want, ",") || len(out.Errors) != 0 {
		t.Errorf("got %+v, want paths %v", out, want)
	}

	m.Routes[1].Entries = nil
	a, _ = newTestApp(t, m)
	_ = json.Unmarshal(do(t, a, "GET", "/__tern/prerender", nil).Body.Bytes(), &out)
	if len(out.Errors) != 1 || !strings.Contains(out.Errors[0], "needs an Entries function") {
		t.Errorf("errors = %v", out.Errors)
	}
}

func TestPrerenderListOnlyDuringBuild(t *testing.T) {
	a, _ := newTestApp(t, testManifest())
	if rec := do(t, a, "GET", "/__tern/prerender", nil); strings.Contains(rec.Body.String(), `"paths"`) {
		t.Error("the prerender list must not be served outside tern build")
	}
}

func TestPrerenderedPagesAreServed(t *testing.T) {
	m := testManifest()
	m.ClientEntry = "entry.js"
	m.Client = fstest.MapFS{
		".vite/manifest.json":    {Data: []byte(`{"entry.js": {"file": "_app/immutable/entry.js", "isEntry": true}}`)},
		"index.html":             {Data: []byte("<p>prerendered home</p>")},
		"blog/first/index.html":  {Data: []byte("<p>prerendered post</p>")},
		"blog/first/__data.json": {Data: []byte(`{"type":"data"}`)},
	}
	a, _ := newTestApp(t, m)
	for path, want := range map[string]string{
		"/":                       "prerendered home",
		"/blog/first":             "prerendered post",
		"/blog/first/__data.json": `{"type":"data"}`,
	} {
		rec := do(t, a, "GET", path, nil)
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("GET %s = %q, want %q", path, rec.Body.String(), want)
		}
	}
	if body := do(t, a, "GET", "/blog/other", nil).Body.String(); strings.Contains(body, "prerendered") {
		t.Error("pages that were not prerendered must render dynamically")
	}
}

func TestContentSecurityPolicy(t *testing.T) {
	a, _ := newTestApp(t, testManifest(), WithContentSecurityPolicy("default-src 'self'; img-src *"))
	rec := do(t, a, "GET", "/", nil)
	csp := rec.Header().Get("Content-Security-Policy")
	m := regexp.MustCompile(`'nonce-([^']+)'`).FindStringSubmatch(csp)
	if m == nil || !strings.HasPrefix(csp, "default-src 'self'; img-src *; script-src 'self' 'nonce-") {
		t.Fatalf("CSP = %q", csp)
	}
	if !strings.Contains(rec.Body.String(), `nonce="`+m[1]+`"`) {
		t.Error("Tern's scripts must carry the policy's nonce")
	}
	if other := do(t, a, "GET", "/", nil).Header().Get("Content-Security-Policy"); other == csp {
		t.Error("the nonce must change on every request")
	}

	ro, _ := newTestApp(t, testManifest(), WithContentSecurityPolicyReportOnly("script-src 'self' https://cdn.example"))
	got := do(t, ro, "GET", "/", nil).Header().Get("Content-Security-Policy-Report-Only")
	if !strings.HasPrefix(got, "script-src 'self' https://cdn.example 'nonce-") {
		t.Errorf("report-only CSP = %q", got)
	}
}

func TestPrerenderListIncludesEndpoints(t *testing.T) {
	t.Setenv("TERN_PRERENDER", "1")
	m := testManifest()
	ok := func(c *Ctx) error { return c.Text(200, "x") }
	m.Routes = append(m.Routes,
		RouteDef{ID: "/sitemap.xml", Prerender: true, Endpoint: map[string]HandlerFunc{"GET": ok}},
		RouteDef{ID: "/feed", Prerender: true, Endpoint: map[string]HandlerFunc{"GET": ok}},
	)
	a, _ := newTestApp(t, m)
	var out struct {
		Endpoints []string `json:"endpoints"`
		Errors    []string `json:"errors"`
	}
	_ = json.Unmarshal(do(t, a, "GET", "/__tern/prerender", nil).Body.Bytes(), &out)
	if strings.Join(out.Endpoints, ",") != "/sitemap.xml" {
		t.Errorf("endpoints = %v", out.Endpoints)
	}
	if len(out.Errors) != 1 || !strings.Contains(out.Errors[0], "needs a file extension") {
		t.Errorf("errors = %v", out.Errors)
	}
}

func TestPublicEnv(t *testing.T) {
	t.Setenv("PUBLIC_API_URL", "https://api.example.com")
	t.Setenv("SECRET_TOKEN", "do-not-send")
	a, _ := newTestApp(t, testManifest())
	body := do(t, a, "GET", "/", nil).Body.String()
	if !strings.Contains(body, `"env":{"PUBLIC_API_URL":"https://api.example.com"}`) {
		t.Errorf("public env missing from the page payload")
	}
	if strings.Contains(body, "do-not-send") {
		t.Error("only PUBLIC_ variables may reach the browser")
	}
	if strings.Contains(do(t, a, "GET", "/__data.json", nil).Body.String(), "PUBLIC_API_URL") {
		t.Error("data responses should not repeat the environment")
	}
}

func TestSetCookieReplaces(t *testing.T) {
	m := testManifest()
	m.Routes = append(m.Routes, RouteDef{ID: "/c", Endpoint: map[string]HandlerFunc{"GET": func(c *Ctx) error {
		c.SetCookie(&http.Cookie{Name: "s", Value: "one"})
		c.SetCookie(&http.Cookie{Name: "other", Value: "x"})
		c.SetCookie(&http.Cookie{Name: "s", Value: "two"})
		return c.Text(200, "ok")
	}}})
	a, _ := newTestApp(t, m)
	got := do(t, a, "GET", "/c", nil).Header().Values("Set-Cookie")
	if len(got) != 2 || !strings.HasPrefix(got[0], "s=two") {
		t.Errorf("Set-Cookie = %v", got)
	}
}

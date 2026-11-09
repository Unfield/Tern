package kit

import (
	"strings"
	"testing"
)

const fakeServerBundle = `
globalThis.__tern_render = (payload, href) => {
	const p = JSON.parse(payload);
	if (p.route === '/todos') throw new Error('component exploded');
	const title = p.type === 'error' ? 'Error ' + p.status : 'Page ' + (p.route || '');
	return JSON.stringify({ head: '<title>' + title + '</title>', body: '<h1>' + title + '</h1><p>' + href + '</p>' });
};
`

func TestServerRendering(t *testing.T) {
	m := testManifest()
	m.ServerBundle = fakeServerBundle
	a, logs := newTestApp(t, m, WithSSRWorkers(2))
	t.Cleanup(func() { _ = a.Close() })

	body := do(t, a, "GET", "http://example.com/blog/hello?x=1", nil).Body.String()
	for _, want := range []string{
		`<div id="tern-root" data-tern-ssr style="display: contents"><h1>Page /blog/slug.param</h1><p>http://example.com/blog/hello?x=1</p></div>`,
		`<title>Page /blog/slug.param</title>`,
		`id="__tern_data"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("response missing %s\n%s", want, body)
		}
	}
	if strings.Index(body, "<title>") > strings.Index(body, "__tern_data") {
		t.Error("rendered head should come before Tern's scripts")
	}

	// Error pages render on the server too.
	rec := do(t, a, "GET", "/nope", nil)
	if rec.Code != 404 || !strings.Contains(rec.Body.String(), "<h1>Error 404</h1>") {
		t.Errorf("404 status = %d, body = %s", rec.Code, rec.Body.String())
	}

	// A failing render falls back to client rendering and is logged.
	rec = do(t, a, "GET", "/todos", nil)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `<div id="tern-root" style="display: contents"></div>`) {
		t.Errorf("fallback status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(logs.String(), "component exploded") {
		t.Errorf("render failure not logged: %s", logs.String())
	}
}

func TestServerRenderingOptOut(t *testing.T) {
	m := testManifest()
	m.ServerBundle = fakeServerBundle
	m.Routes[0].Page.NoSSR = true // "/"
	a, _ := newTestApp(t, m)
	t.Cleanup(func() { _ = a.Close() })
	if body := do(t, a, "GET", "/", nil).Body.String(); strings.Contains(body, "data-tern-ssr") {
		t.Error("page with NoSSR was server-rendered")
	}

	off, _ := newTestApp(t, m, WithoutSSR())
	if body := do(t, off, "GET", "/blog/x", nil).Body.String(); strings.Contains(body, "data-tern-ssr") {
		t.Error("WithoutSSR app server-rendered a page")
	}
}

func TestBrokenServerBundleFailsAtStartup(t *testing.T) {
	m := testManifest()
	m.ServerBundle = "syntax error ("
	if _, err := New(m); err == nil {
		t.Fatal("New should fail with a broken server bundle")
	}
}

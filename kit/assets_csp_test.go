package kit

import (
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestPrerenderedPageCSP(t *testing.T) {
	inline := `document.documentElement.dataset.theme = localStorage.theme`
	page := `<!doctype html><html><head>
<script>` + inline + `</script>
<script type="module" src="/_app/immutable/client.js"></script>
<script type="application/json" id="__tern_data">{"a":1}</script>
</head><body></body></html>`
	fsys := fstest.MapFS{
		".vite/manifest.json":      {Data: []byte(`{"src/client.ts":{"file":"_app/immutable/client.js","isEntry":true}}`)},
		"_app/immutable/client.js": {Data: []byte(`console.log(1)`)},
		"docs/index.html":          {Data: []byte(page)},
		"plain.html":               {Data: []byte(`<script>alert(1)</script>`)},
	}
	a, err := loadAssets(fsys, "src/client.ts")
	if err != nil {
		t.Fatal(err)
	}
	a.csp, a.cspName = "default-src 'self'; script-src 'self'", "Content-Security-Policy"

	sum := sha256.Sum256([]byte(inline))
	want := "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"

	w := httptest.NewRecorder()
	if !a.serve(w, httptest.NewRequest(http.MethodGet, "/docs/", nil)) {
		t.Fatal("not served")
	}
	csp := w.Header().Get("Content-Security-Policy")
	if csp != "default-src 'self'; script-src 'self' "+want {
		t.Errorf("CSP %q, want the inline script's hash %s", csp, want)
	}
	if strings.Count(csp, "sha256-") != 1 {
		t.Errorf("only the script that runs is hashed: %q", csp)
	}

	// A static HTML file Tern didn't render keeps its old headers.
	w = httptest.NewRecorder()
	a.serve(w, httptest.NewRequest(http.MethodGet, "/plain.html", nil))
	if got := w.Header().Get("Content-Security-Policy"); got != "" {
		t.Errorf("plain.html got a CSP: %q", got)
	}

	// Report-only and no policy carry over.
	a.cspName = "Content-Security-Policy-Report-Only"
	w = httptest.NewRecorder()
	a.serve(w, httptest.NewRequest(http.MethodGet, "/docs/", nil))
	if w.Header().Get("Content-Security-Policy-Report-Only") == "" {
		t.Error("report-only policy missing")
	}
	a.csp = ""
	w = httptest.NewRecorder()
	a.serve(w, httptest.NewRequest(http.MethodGet, "/docs/", nil))
	if len(w.Header().Values("Content-Security-Policy-Report-Only")) != 0 {
		t.Error("a policy was sent without one set")
	}
}

func TestBrotliFirst(t *testing.T) {
	fsys := fstest.MapFS{
		".vite/manifest.json":         {Data: []byte(`{"src/client.ts":{"file":"_app/immutable/client.js","isEntry":true}}`)},
		"_app/immutable/client.js":    {Data: []byte(`plain`)},
		"_app/immutable/client.js.gz": {Data: []byte(`gz`)},
		"_app/immutable/client.js.br": {Data: []byte(`br`)},
	}
	a, err := loadAssets(fsys, "src/client.ts")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := a.files["/_app/immutable/client.js.br"]; ok {
		t.Error("the .br sibling is served as a file of its own")
	}
	for accept, want := range map[string]string{"gzip, deflate, br": "br", "gzip": "gz", "br;q=0, gzip": "gz", "": "plain"} {
		r := httptest.NewRequest(http.MethodGet, "/_app/immutable/client.js", nil)
		r.Header.Set("Accept-Encoding", accept)
		w := httptest.NewRecorder()
		a.serve(w, r)
		if w.Body.String() != want || w.Header().Get("Vary") != "Accept-Encoding" {
			t.Errorf("Accept-Encoding %q: got %q (Vary %q), want %q", accept, w.Body.String(), w.Header().Get("Vary"), want)
		}
	}
}

package kit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDevStyles(t *testing.T) {
	var got []string
	vite := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/__tern/dev/styles" {
			http.NotFound(w, r)
			return
		}
		got = r.URL.Query()["c"]
		_, _ = w.Write([]byte(`.a{color:red}/* </style><script>x</script> */`))
	}))
	defer vite.Close()
	d, err := newDevServer(vite.URL, t.TempDir(), "entry.js", "")
	if err != nil {
		t.Fatal(err)
	}
	css := d.styles(context.Background(), []string{"src/routes/+layout.svelte", "src/routes/+page.svelte"})
	if strings.Join(got, ",") != "src/routes/+layout.svelte,src/routes/+page.svelte" {
		t.Errorf("components %q", got)
	}
	if !strings.Contains(css, ".a{color:red}") || strings.Contains(css, "</style") {
		t.Errorf("css %q", css)
	}
	// Vite gone: no styles, no error.
	vite.Close()
	if css := d.styles(context.Background(), []string{"x"}); css != "" {
		t.Errorf("without Vite: %q", css)
	}
}

// Under tern dev, payloads tell the dev overlay their request and files.
func TestDevInfo(t *testing.T) {
	a, _ := newTestApp(t, testManifest())
	rec := do(t, a, "GET", "/blog/hello/__data.json", nil)
	if strings.Contains(rec.Body.String(), `"dev"`) {
		t.Fatal("dev info outside tern dev")
	}
	d, err := newDevServer("http://127.0.0.1:1", t.TempDir(), "entry.js", "")
	if err != nil {
		t.Fatal(err)
	}
	a.dev = d
	rec = do(t, a, "GET", "/blog/hello/__data.json", nil)
	var p struct {
		Dev devInfo `json:"dev"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	if p.Dev.RequestID == "" || p.Dev.RequestID != rec.Header().Get(RequestIDHeader) ||
		strings.Join(p.Dev.Components, ",") != "src/routes/+layout.svelte,src/routes/blog/+layout.svelte,src/routes/blog/slug.param/+page.svelte" {
		t.Errorf("dev = %+v", p.Dev)
	}
}

package kit

import (
	"context"
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

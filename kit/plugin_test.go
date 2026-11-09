package kit

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

type testPlugin struct {
	name     string
	setupErr error
	stopped  *[]string
}

func (p testPlugin) Name() string { return p.name }

func (p testPlugin) Setup(s *Setup) error {
	if p.setupErr != nil {
		return p.setupErr
	}
	s.Handle("GET /plugin/"+p.name, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("from " + s.Name()))
	}))
	s.Head(`<meta name="plugin" content="` + p.name + `">`)
	s.HeadFunc(func(_ *Ctx, nonce string) string { return `<script nonce="` + nonce + `">/*` + p.name + `*/</script>` })
	s.Hook(func(c *Ctx) error {
		c.SetHeader("X-Plugin-"+p.name, "hooked")
		return nil
	})
	s.Matcher("even", func(v string) bool { return len(v)%2 == 0 })
	s.OnShutdown(func(context.Context) error {
		*p.stopped = append(*p.stopped, p.name)
		return nil
	})
	return nil
}

func TestPlugins(t *testing.T) {
	var stopped []string
	m := testManifest()
	m.Routes = append(m.Routes, RouteDef{ID: "/even/x.param.even", Endpoint: map[string]HandlerFunc{"GET": func(c *Ctx) error { return c.Text(200, "even") }}})
	a, _ := newTestApp(t, m, WithPlugins(testPlugin{name: "a", stopped: &stopped}, testPlugin{name: "b", stopped: &stopped}))

	if body := do(t, a, "GET", "/plugin/a", nil).Body.String(); body != "from a" {
		t.Errorf("plugin route = %q", body)
	}
	rec := do(t, a, "GET", "/", nil)
	body := rec.Body.String()
	for _, want := range []string{`<meta name="plugin" content="a">`, `<meta name="plugin" content="b">`, "/*b*/"} {
		if !strings.Contains(body, want) {
			t.Errorf("page head missing %s", want)
		}
	}
	if rec.Header().Get("X-Plugin-a") != "hooked" || rec.Header().Get("X-Plugin-b") != "hooked" {
		t.Errorf("plugin hooks did not run: %v", rec.Header())
	}
	if do(t, a, "GET", "/even/ab", nil).Code != 200 || do(t, a, "GET", "/even/abc", nil).Code != 404 {
		t.Error("plugin matcher not applied")
	}

	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	if strings.Join(stopped, ",") != "b,a" {
		t.Errorf("shutdown order = %v, want newest first", stopped)
	}
}

func TestPluginErrors(t *testing.T) {
	if _, err := New(testManifest(), WithPlugins(testPlugin{name: "x", setupErr: errors.New("no config")})); err == nil || !strings.Contains(err.Error(), `plugin "x": no config`) {
		t.Errorf("setup error = %v", err)
	}
	var s []string
	if _, err := New(testManifest(), WithPlugins(testPlugin{name: "x", stopped: &s}, testPlugin{name: "x", stopped: &s})); err == nil || !strings.Contains(err.Error(), "added twice") {
		t.Errorf("duplicate error = %v", err)
	}
}

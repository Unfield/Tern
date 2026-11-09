package kit

import (
	"encoding/json"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

type nested struct {
	Lang  string `json:"lang"`
	Count int64  `json:"count"`
}

func reuseManifest(rootRuns, langRuns *atomic.Int64) *Manifest {
	return &Manifest{
		AppHTML: testHTML,
		Layouts: []LayoutDef{
			{Dir: "", Load: LoadOf(func(c *Ctx) (map[string]int64, error) {
				c.Depends("app:session")
				return map[string]int64{"root": rootRuns.Add(1)}, nil
			})},
			{Dir: "lang.param", Load: LoadOf(func(c *Ctx) (nested, error) {
				return nested{Lang: c.Param("lang"), Count: langRuns.Add(1)}, nil
			})},
		},
		Routes: []RouteDef{
			{ID: "/lang.param/a", Layouts: []int{0, 1}, Page: &PageDef{Load: LoadOf(func(c *Ctx) (map[string]string, error) {
				q := c.URL().Query()
				return map[string]string{"page": "a", "q": q.Get("q"), "has": q.Get(reuseParam)}, nil
			})}},
			{ID: "/lang.param/b", Layouts: []int{0, 1}, Page: &PageDef{Load: LoadOf(func(c *Ctx) (map[string]any, error) {
				p, err := Parent[nested](c)
				return map[string]any{"page": "b", "parentLang": p.Lang}, err
			})}},
		},
	}
}

func dataWithReuse(t *testing.T, a *App, path string, entries []reuseEntry) payload {
	t.Helper()
	target := path + "/__data.json"
	if entries != nil {
		b, _ := json.Marshal(entries)
		target += "?" + reuseParam + "=" + url.QueryEscape(string(b))
	}
	return jsonPayload(t, do(t, a, "GET", target, nil))
}

func TestLayoutReuse(t *testing.T) {
	var rootRuns, langRuns atomic.Int64
	a, _ := newTestApp(t, reuseManifest(&rootRuns, &langRuns))

	first := dataWithReuse(t, a, "/en/a", nil)
	if len(first.Uses) != 3 || first.Uses[0].Deps[0] != "app:session" || first.Uses[1].Params[0] != "lang" || !first.Uses[2].URL {
		t.Fatalf("uses = %+v", first.Uses)
	}
	if rootRuns.Load() != 1 || langRuns.Load() != 1 {
		t.Fatalf("runs = %d, %d", rootRuns.Load(), langRuns.Load())
	}

	// Same lang: both layouts are reused; only the page runs.
	held := []reuseEntry{{Layout: 0}, {Layout: 1, Params: map[string]string{"lang": "en"}}}
	p := dataWithReuse(t, a, "/en/a", held)
	if jsonInts(p.Reused) != "[0,1]" {
		t.Errorf("reused = %v, want [0 1]", p.Reused)
	}
	if rootRuns.Load() != 1 || langRuns.Load() != 1 {
		t.Errorf("reused layouts ran again: %d, %d", rootRuns.Load(), langRuns.Load())
	}
	if !strings.Contains(string(p.Nodes[2]), `"has":""`) {
		t.Errorf("loads must not see the reuse parameter: %s", p.Nodes[2])
	}

	// A different lang reruns the lang layout; the root is still reused.
	p = dataWithReuse(t, a, "/de/a", held)
	if jsonInts(p.Reused) != "[0]" || langRuns.Load() != 2 || !strings.Contains(string(p.Nodes[1]), `"lang":"de"`) {
		t.Errorf("reused = %v, langRuns = %d, node = %s", p.Reused, langRuns.Load(), p.Nodes[1])
	}

	// A page that reads parent data gets it even when the layouts are reused.
	p = dataWithReuse(t, a, "/en/b", held)
	if !strings.Contains(string(p.Nodes[2]), `"parentLang":"en"`) {
		t.Errorf("page node = %s", p.Nodes[2])
	}
	if len(p.Reused) != 0 || p.Nodes[0] == nil {
		// Parent merges every layout above, so both ran and are sent back.
		t.Errorf("layouts computed for Parent are sent, not reused: reused = %v", p.Reused)
	}

	// Full page loads always run everything.
	before := rootRuns.Load()
	do(t, a, "GET", "/en/a", nil)
	if rootRuns.Load() != before+1 {
		t.Error("document requests must run every load")
	}
}

func jsonInts(v []int) string {
	b, _ := json.Marshal(v)
	return string(b)
}

package kit

import (
	"bufio"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

type streamed struct {
	Fast string           `json:"fast"`
	Slow Deferred[string] `json:"slow"`
	Fail Deferred[int]    `json:"fail"`
	Oops Deferred[int]    `json:"oops"`
}

func streamingManifest() *Manifest {
	m := testManifest()
	m.Routes = append(m.Routes, RouteDef{ID: "/stream", Page: &PageDef{
		Component: "src/routes/stream/+page.svelte",
		Load: LoadOf(func(c *Ctx) (streamed, error) {
			return streamed{
				Fast: "now",
				Slow: Defer(c, func() (string, error) {
					time.Sleep(60 * time.Millisecond)
					return "later", nil
				}),
				Fail: Defer(c, func() (int, error) { return 0, errors.New("secret database error") }),
				Oops: Defer(c, func() (int, error) { panic("deferred panic") }),
			}, nil
		}),
	}})
	return m
}

func TestDeferredDocument(t *testing.T) {
	a, logs := newTestApp(t, streamingManifest())
	body := do(t, a, "GET", "/stream", nil).Body.String()

	p := payloadFromHTML(t, body)
	node := string(p.Nodes[0])
	if node != `{"fast":"now","slow":{"__tern_deferred":1},"fail":{"__tern_deferred":2},"oops":{"__tern_deferred":3}}` {
		t.Errorf("node = %s", node)
	}
	if !strings.Contains(body, "__tern_resolve=function") {
		t.Error("missing the resolve bootstrap script")
	}
	slow := strings.Index(body, `__tern_resolve({"id":1,"data":"later"})`)
	fail := strings.Index(body, `__tern_resolve({"id":2,"error":{"message":"Internal Error"}})`)
	if slow < 0 || fail < 0 || fail > slow {
		t.Errorf("chunks missing or out of completion order:\n%s", body)
	}
	if end := strings.Index(body, "</body>"); end < slow {
		t.Error("chunks must be written before </body>")
	}
	if strings.Contains(body, "secret database error") || !strings.Contains(logs.String(), "secret database error") {
		t.Error("unexpected deferred errors must be logged, not shown")
	}
	if !strings.Contains(logs.String(), "deferred panic") {
		t.Error("deferred panics must be recovered and logged")
	}
}

func TestDeferredData(t *testing.T) {
	a, _ := newTestApp(t, streamingManifest())
	sc := bufio.NewScanner(strings.NewReader(do(t, a, "GET", "/stream/__data.json", nil).Body.String()))
	var lines []string
	for sc.Scan() {
		if sc.Text() != "" {
			lines = append(lines, sc.Text())
		}
	}
	if len(lines) != 4 {
		t.Fatalf("want a payload line and 3 chunks, got %d:\n%s", len(lines), strings.Join(lines, "\n"))
	}
	var p payload
	if err := json.Unmarshal([]byte(lines[0]), &p); err != nil || p.Type != "data" {
		t.Fatalf("first line = %s (%v)", lines[0], err)
	}
	if lines[3] != `{"id":1,"data":"later"}` {
		t.Errorf("slowest value should arrive last, got %s", lines[3])
	}
}

func payloadFromHTML(t *testing.T, body string) payload {
	t.Helper()
	m := dataScriptRe.FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("no data script in:\n%s", body)
	}
	var p payload
	if err := json.Unmarshal([]byte(m[1]), &p); err != nil {
		t.Fatal(err)
	}
	return p
}

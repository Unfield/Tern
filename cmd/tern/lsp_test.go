package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func lspProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"go.mod":                                    "module example.com/app\n",
		"src/routes/+layout.svelte":                 "<script lang=\"ts\">\n\tlet { data, children } = $props();\n</script>\n",
		"src/routes/layout.server.go":               "package routes\n\nimport \"github.com/Unfield/Tern/kit\"\n\nfunc LayoutLoad(c *kit.Ctx) (int, error) { return 0, nil }\n",
		"src/routes/+page.svelte":                   "<script lang=\"ts\">\n\timport { route } from '$routes';\n\tlet { data } = $props();\n</script>\n<a href={route('/blog/[slug]', { slug: 'x' })}>x</a>\n",
		"src/routes/page.server.go":                 "package routes\n\n// Load is the home page.\nfunc Load(c *kit.Ctx) (int, error) { return 0, nil }\n",
		"src/routes/blog/slug.param/+page.svelte":   "<script lang=\"ts\">\n\tlet { data } = $props();\n</script>\n",
		"src/routes/blog/slug.param/page.server.go": "package post\n\nfunc Load(c *kit.Ctx) (int, error) { return 0, nil }\n",
		"src/routes/api/items/server.go":            "package items\n\nimport \"github.com/Unfield/Tern/kit\"\n\nfunc GET(c *kit.Ctx) error { return nil }\n",
	}
	for name, body := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// lspClient talks to a server over pipes, as an editor does.
type lspClient struct {
	t     *testing.T
	in    io.WriteCloser
	mu    sync.Mutex
	msgs  []lspMessage
	added chan struct{}
	id    int
}

func startLSP(t *testing.T, root string, check func(string) (map[string][]lspDiagnostic, error)) *lspClient {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	s := &lspServer{out: outW, log: io.Discard, files: map[string]bool{}, runCheck: check, debounce: time.Millisecond}
	go func() { _ = s.serve(inR); _ = outW.Close() }()
	c := &lspClient{t: t, in: inW, added: make(chan struct{}, 100)}
	go func() {
		r := bufio.NewReader(outR)
		for {
			n := 0
			for {
				line, err := r.ReadString('\n')
				if err != nil {
					return
				}
				line = strings.TrimSpace(line)
				if line == "" {
					break
				}
				if v, ok := strings.CutPrefix(line, "Content-Length: "); ok {
					n, _ = strconv.Atoi(v)
				}
			}
			body := make([]byte, n)
			if _, err := io.ReadFull(r, body); err != nil {
				return
			}
			var m lspMessage
			_ = json.Unmarshal(body, &m)
			c.mu.Lock()
			c.msgs = append(c.msgs, m)
			c.mu.Unlock()
			c.added <- struct{}{}
		}
	}()
	t.Cleanup(func() { c.notify("exit", nil); _ = inW.Close() })
	c.call("initialize", map[string]any{"rootUri": pathURI(root)})
	c.notify("initialized", map[string]any{})
	return c
}

func (c *lspClient) write(m map[string]any) {
	m["jsonrpc"] = "2.0"
	b, _ := json.Marshal(m)
	fmt.Fprintf(c.in, "Content-Length: %d\r\n\r\n%s", len(b), b)
}

func (c *lspClient) notify(method string, params any) {
	c.write(map[string]any{"method": method, "params": params})
}

// call sends a request and returns its result, as JSON.
func (c *lspClient) call(method string, params any) string {
	c.t.Helper()
	c.id++
	id := strconv.Itoa(c.id)
	c.write(map[string]any{"id": c.id, "method": method, "params": params})
	deadline := time.After(5 * time.Second)
	for {
		c.mu.Lock()
		for _, m := range c.msgs {
			if string(m.ID) == id {
				c.mu.Unlock()
				b, _ := json.Marshal(m.Result)
				if m.Error != nil {
					return "error: " + m.Error.Message
				}
				return string(b)
			}
		}
		c.mu.Unlock()
		select {
		case <-c.added:
		case <-deadline:
			c.t.Fatalf("no answer to %s", method)
		}
	}
}

// waitNotify waits for a notification of method whose params match.
func (c *lspClient) waitNotify(method string, match func(string) bool) {
	c.t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		c.mu.Lock()
		for _, m := range c.msgs {
			if m.Method == method && match(string(m.Params)) {
				c.mu.Unlock()
				return
			}
		}
		c.mu.Unlock()
		select {
		case <-c.added:
		case <-deadline:
			c.t.Fatalf("no %s notification", method)
		}
	}
}

func pos(file string, line, char int) map[string]any {
	return map[string]any{"textDocument": map[string]any{"uri": pathURI(file)}, "position": map[string]any{"line": line, "character": char}}
}

func TestLSP(t *testing.T) {
	root := lspProject(t)
	var mu sync.Mutex
	checks := 0
	c := startLSP(t, root, func(r string) (map[string][]lspDiagnostic, error) {
		mu.Lock()
		defer mu.Unlock()
		checks++
		if checks == 1 {
			p := lspPosition{Line: 2, Character: 5}
			return map[string][]lspDiagnostic{filepath.Join(r, "src/routes/+page.svelte"): {{Range: lspRange{p, p}, Severity: 1, Source: "tern check", Message: "Property 'nope' does not exist."}}}, nil
		}
		return map[string][]lspDiagnostic{}, nil
	})
	routes := filepath.Join(root, "src", "routes")

	// From a page's data to its Go load, and back.
	if got := c.call("textDocument/definition", pos(filepath.Join(routes, "+page.svelte"), 2, 8)); !strings.Contains(got, "page.server.go") || !strings.Contains(got, `"line":3`) {
		t.Errorf("data -> Load: %s", got)
	}
	if got := c.call("textDocument/definition", pos(filepath.Join(routes, "page.server.go"), 3, 6)); !strings.Contains(got, "%2Bpage.svelte") && !strings.Contains(got, "+page.svelte") {
		t.Errorf("Load -> page: %s", got)
	}
	// A layout's props to its LayoutLoad, and back.
	if got := c.call("textDocument/definition", pos(filepath.Join(routes, "+layout.svelte"), 1, 8)); !strings.Contains(got, "layout.server.go") || !strings.Contains(got, `"line":4`) {
		t.Errorf("layout data -> LayoutLoad: %s", got)
	}
	if got := c.call("textDocument/definition", pos(filepath.Join(routes, "layout.server.go"), 4, 8)); !strings.Contains(got, "layout.svelte") {
		t.Errorf("LayoutLoad -> layout: %s", got)
	}
	// In a nested route too.
	if got := c.call("textDocument/definition", pos(filepath.Join(routes, "blog", "slug.param", "+page.svelte"), 1, 8)); !strings.Contains(got, "slug.param/page.server.go") {
		t.Errorf("nested data -> Load: %s", got)
	}
	if got := c.call("textDocument/definition", pos(filepath.Join(routes, "blog", "slug.param", "page.server.go"), 2, 6)); !strings.Contains(got, "slug.param/%2Bpage.svelte") && !strings.Contains(got, "slug.param/+page.svelte") {
		t.Errorf("nested Load -> page: %s", got)
	}
	// A route pattern to its page.
	if got := c.call("textDocument/definition", pos(filepath.Join(routes, "+page.svelte"), 4, 20)); !strings.Contains(got, "slug.param") {
		t.Errorf("route() -> page: %s", got)
	}
	// Anywhere else, nothing: the editor asks the other servers.
	if got := c.call("textDocument/definition", pos(filepath.Join(routes, "+page.svelte"), 1, 3)); got != "null" {
		t.Errorf("elsewhere: %s", got)
	}

	// Routes as symbols.
	syms := c.call("workspace/symbol", map[string]any{"query": "blog"})
	if !strings.Contains(syms, `"/blog/[slug]"`) || strings.Contains(syms, `"/api/items"`) {
		t.Errorf("symbols: %s", syms)
	}
	if all := c.call("workspace/symbol", map[string]any{"query": ""}); !strings.Contains(all, `"/api/items"`) || !strings.Contains(all, `"endpoint route"`) {
		t.Errorf("all symbols: %s", all)
	}
	if got := c.call("tern/routes", nil); !strings.Contains(got, `"pattern":"/"`) || !strings.Contains(got, `"kind":"page"`) {
		t.Errorf("routes: %s", got)
	}

	// Code lenses lead to the other half.
	if got := c.call("textDocument/codeLens", map[string]any{"textDocument": map[string]any{"uri": pathURI(filepath.Join(routes, "+page.svelte"))}}); !strings.Contains(got, "Go: page.server.go:4") || !strings.Contains(got, "tern.open") {
		t.Errorf("page lens: %s", got)
	}
	if got := c.call("textDocument/codeLens", map[string]any{"textDocument": map[string]any{"uri": pathURI(filepath.Join(routes, "page.server.go"))}}); !strings.Contains(got, "Svelte: +page.svelte") || !strings.Contains(got, `"line":3`) {
		t.Errorf("Go lens: %s", got)
	}

	// The check runs when the server starts, and again on save; fixed problems are cleared.
	c.waitNotify("textDocument/publishDiagnostics", func(p string) bool { return strings.Contains(p, "nope") })
	c.notify("textDocument/didSave", map[string]any{"textDocument": map[string]any{"uri": pathURI(filepath.Join(routes, "+page.svelte"))}})
	c.waitNotify("textDocument/publishDiagnostics", func(p string) bool {
		return strings.Contains(p, "page.svelte") && strings.Contains(p, `"diagnostics":[]`)
	})

	// A change outside the editor checks again; generated files don't.
	mu.Lock()
	before := checks
	mu.Unlock()
	c.notify("workspace/didChangeWatchedFiles", map[string]any{"changes": []map[string]any{{"uri": pathURI(filepath.Join(root, ".tern", "generated", "api.d.ts")), "type": 2}}})
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	if checks != before {
		t.Error("a generated file started a check")
	}
	mu.Unlock()
	c.notify("workspace/didChangeWatchedFiles", map[string]any{"changes": []map[string]any{{"uri": pathURI(filepath.Join(routes, "page.server.go")), "type": 2}}})
	until := time.Now().Add(5 * time.Second)
	for {
		mu.Lock()
		n := checks
		mu.Unlock()
		if n > before {
			break
		}
		if time.Now().After(until) {
			t.Fatal("a changed file didn't start a check")
		}
		time.Sleep(10 * time.Millisecond)
	}

	if got := c.call("tern/nope", nil); !strings.HasPrefix(got, "error:") {
		t.Errorf("unknown method: %s", got)
	}
	if got := c.call("shutdown", nil); got != "null" {
		t.Errorf("shutdown: %s", got)
	}
}

func TestLSPParsers(t *testing.T) {
	root := "/app"
	out := map[string][]lspDiagnostic{}
	parseSvelteCheck(root, `1794499 START "/app"
1794499 ERROR "src/routes/+page.svelte" 15:56 "Parameter 'signal' implicitly has an \"any\" type."
1794499 WARNING ".tern/check/src/lib/Note.svelte" 3:1 "Unused CSS selector"
1794499 COMPLETED 3 FILES 1 ERRORS 1 WARNINGS 2 FILES_WITH_PROBLEMS
`, out)
	page := out[filepath.FromSlash("/app/src/routes/+page.svelte")]
	if len(page) != 1 || page[0].Range.Start.Line != 14 || page[0].Range.Start.Character != 55 || page[0].Message != `Parameter 'signal' implicitly has an "any" type.` || page[0].Severity != 1 {
		t.Errorf("page = %+v", page)
	}
	if note := out[filepath.FromSlash("/app/src/lib/Note.svelte")]; len(note) != 1 || note[0].Severity != 2 {
		t.Errorf("a checked copy should point at the real file: %+v", out)
	}

	goOut := map[string][]lspDiagnostic{}
	parseGoErrors(root, "src/routes/x/page.server.go:7:6: Load must be func(ctx *kit.Ctx) (Data, error)\nother output", goOut)
	if d := goOut[filepath.FromSlash("/app/src/routes/x/page.server.go")]; len(d) != 1 || d[0].Range.Start.Line != 6 || !strings.HasPrefix(d[0].Message, "Load must be") {
		t.Errorf("go = %+v", goOut)
	}
}

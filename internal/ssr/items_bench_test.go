package ssr

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"modernc.org/quickjs"
)

// The vs-sveltekit benchmark's table page: build benchmarks/vs-sveltekit/tern
// first, and capture its payload to $TERN_ITEMS_PAYLOAD.
func itemsBundle(tb testing.TB) (string, []byte) {
	_, here, _, _ := runtime.Caller(0)
	bundle, err := os.ReadFile(filepath.Join(filepath.Dir(here), "..", "..", "benchmarks", "vs-sveltekit", "tern", ".tern", "output", "server", "server.js"))
	if err != nil {
		tb.Skip("build benchmarks/vs-sveltekit/tern first")
	}
	payload, err := os.ReadFile(os.Getenv("TERN_ITEMS_PAYLOAD"))
	if err != nil {
		tb.Skip("set TERN_ITEMS_PAYLOAD")
	}
	return string(bundle), payload
}

func BenchmarkRenderItems(b *testing.B) {
	bundle, payload := itemsBundle(b)
	p, err := NewPool(bundle, Options{Workers: 1})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = p.Close() })
	b.ResetTimer()
	for b.Loop() {
		if _, err := p.Render(context.Background(), payload, "http://localhost/items"); err != nil {
			b.Fatal(err)
		}
	}
}

// TestItemsInsideTheEngine times the parts of a render inside QuickJS, with
// no Go in between.
func TestItemsInsideTheEngine(t *testing.T) {
	bundle, payload := itemsBundle(t)
	vm, err := quickjs.NewVM()
	if err != nil {
		t.Fatal(err)
	}
	defer vm.Close()
	if _, err := vm.Eval(bundle, quickjs.EvalGlobal); err != nil {
		t.Fatal(err)
	}
	if _, err := vm.Eval(fmt.Sprintf("globalThis.__p = %q; globalThis.__html = JSON.parse(__tern_render(__p, 'http://localhost/items')).body;", payload), quickjs.EvalGlobal); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ name, code string }{
		{"JSON.parse of the payload", "JSON.parse(__p)"},
		{"the whole render", "__tern_render(__p, 'http://localhost/items')"},
		{"JSON.stringify of a 35 KB result", "JSON.stringify({ body: __html })"},
		{"string building: 2,000 += of short strings", "{ let s = ''; for (let j = 0; j < 2000; j++) s += '<td>' + j + '</td>'; }"},
		{"600 regexp escapes of short strings", "{ let n = 0; for (let j = 0; j < 600; j++) n += ('Item ' + j).replace(/[&<>\"']/g, (c) => '&#' + c.charCodeAt(0) + ';').length; }"},
		{"600 calls of a small function", "{ let n = 0; const f = (x) => x + 1; for (let j = 0; j < 600; j++) n = f(n); }"},
	} {
		out, err := vm.Eval(fmt.Sprintf("(() => { for (let i = 0; i < 20; i++) %s; const t = Date.now(); for (let i = 0; i < 200; i++) %s; return (Date.now() - t) / 200; })()", c.code, c.code), quickjs.EvalGlobal)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		t.Logf("%-45s %v ms", c.name, out)
	}
}

func TestItemsDumpBody(t *testing.T) {
	bundle, payload := itemsBundle(t)
	p, err := NewPool(bundle, Options{Workers: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	res, err := p.Render(context.Background(), payload, "http://localhost/items")
	if err != nil {
		t.Fatal(err)
	}
	if out := os.Getenv("TERN_ITEMS_BODY"); out != "" {
		_ = os.WriteFile(out, []byte(res.Body), 0o644)
	}
	t.Logf("head %q", res.Head)
}

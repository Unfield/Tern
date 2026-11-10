package i18n

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unicode/utf8"

	"modernc.org/quickjs"
)

// FuzzParity formats random messages with this package and with the
// browser's formatter (packages/tern/src/runtime/format.ts, compiled to
// dist by pnpm build) running in QuickJS. They must agree, or server-rendered
// text would not match what the browser renders. Skipped until the client
// package is built.
func FuzzParity(f *testing.F) {
	_, here, _, _ := runtime.Caller(0)
	src, err := os.ReadFile(filepath.Join(filepath.Dir(here), "..", "..", "packages", "tern", "dist", "runtime", "format.js"))
	if err != nil {
		f.Skip("build packages/tern first (pnpm --filter @ternkit/tern build)")
	}
	vm, err := quickjs.NewVM()
	if err != nil {
		f.Fatal(err)
	}
	f.Cleanup(func() { _ = vm.Close() })
	if _, err := vm.Eval(strings.ReplaceAll(string(src), "export function", "function")+
		"\nglobalThis.__format = (l, m, p) => format(l, m, JSON.parse(p));", quickjs.EvalGlobal); err != nil {
		f.Fatal(err)
	}

	var cases []struct {
		Locale  string         `json:"locale"`
		Message string         `json:"message"`
		Params  map[string]any `json:"params"`
	}
	raw, _ := os.ReadFile(filepath.Join("testdata", "cases.json"))
	_ = json.Unmarshal(raw, &cases)
	for _, c := range cases {
		n, _ := c.Params["count"].(float64)
		f.Add(c.Locale, c.Message, n, "x")
	}
	f.Add("ru", "{n, plural, one {# книга} few {# книги} many {# книг} other {# книги}}", 22.0, "a")
	f.Add("en", "{g, select, a {{n, plural, =1 {one #} other {# #}}} other {'{'g'}'}}", 1.5, "a")

	f.Fuzz(func(t *testing.T, locale, msg string, n float64, g string) {
		if !utf8.ValidString(msg) || !utf8.ValidString(g) || !utf8.ValidString(locale) || math.IsNaN(n) || math.IsInf(n, 0) {
			return
		}
		// Numbers are formatted by String(n) in JavaScript and strconv in Go;
		// both agree for integers and short decimals, which messages use.
		n = math.Round(n*100) / 100
		if math.Abs(n) > 1e15 {
			return
		}
		params := map[string]any{"n": n, "count": n, "g": g, "name": g}
		p, _ := json.Marshal(params)
		js, err := vm.Call("__format", locale, msg, string(p))
		if err != nil {
			t.Fatalf("js: %v", err)
		}
		if got := Format(locale, msg, params); got != js {
			t.Fatalf("Format(%q, %q, n=%v g=%q):\n  go: %q\n  js: %q", locale, msg, n, g, got, js)
		}
	})
}

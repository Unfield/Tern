package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Unfield/Tern/goscript"
)

func TestTypedScript(t *testing.T) {
	src := "<script module lang=\"ts\">\n\timport X from './X.svelte';\n</script>\n\n<script lang=\"go\" live>\nimport \"github.com/Unfield/Tern/goscript\"\n\nfunc Setup(c *goscript.Component) {\n\tgoscript.State(c, \"count\", 0)\n\tc.Func(\"add\", func() {})\n}\n</script>\n\n<button onclick={add}>{count}</button>\n"
	out := typedScript("/app", "src/lib/C.svelte", src)
	if strings.Count(out, "\n") != strings.Count(src, "\n") {
		t.Errorf("lines moved:\n%s", out)
	}
	lines := strings.Split(out, "\n")
	if lines[13] != "<button onclick={add}>{count}</button>" {
		t.Errorf("markup line = %q", lines[13])
	}
	id := goscript.ComponentID("src/lib/C.svelte")
	for _, want := range []string{
		`<script lang="ts">`,
		`from "./../../../goscript/types/` + id + `"`,
		`const count = null as unknown as __S["count"];`,
		`const add = null as unknown as __F["add"];`,
		`const live = null as unknown as __L<__S>;`,
		"<script module lang=\"ts\">",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	// WebAssembly components are loosely typed.
	wasm := typedScript("/app", "src/lib/W.svelte", strings.Replace(src, ` live>`, `>`, 1))
	if !strings.Contains(wasm, "const count: any = null;") || strings.Contains(wasm, "__S") {
		t.Errorf("wasm component:\n%s", wasm)
	}
}

func TestCheckTSConfig(t *testing.T) {
	root := t.TempDir()
	gen := `{"compilerOptions":{"paths":{"$lib":["../src/lib"],"$routes":["./generated/routes"]},"rootDirs":["..","./types"]},"include":["../src/**/*.svelte","./ambient.d.ts"]}`
	_ = os.MkdirAll(filepath.Join(root, ".tern", "check"), 0o755)
	_ = os.WriteFile(filepath.Join(root, ".tern", "tsconfig.json"), []byte(gen), 0o644)
	if err := writeCheckTSConfig(root, filepath.Join(root, ".tern", "check")); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(root, ".tern", "check", "tsconfig.json"))
	var cfg struct {
		Extends         string `json:"extends"`
		CompilerOptions struct {
			Paths    map[string][]string `json:"paths"`
			RootDirs []string            `json:"rootDirs"`
		} `json:"compilerOptions"`
		Include []string `json:"include"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Extends != "../../tsconfig.json" || cfg.CompilerOptions.Paths["$lib"][0] != "./src/lib" || cfg.CompilerOptions.Paths["$routes"][0] != "../generated/routes" ||
		cfg.CompilerOptions.RootDirs[0] != "." || cfg.CompilerOptions.RootDirs[1] != "../types" || cfg.Include[0] != "./src/**/*.svelte" || cfg.Include[1] != "../ambient.d.ts" {
		t.Errorf("tsconfig = %s", raw)
	}
}

func TestGoScriptNames(t *testing.T) {
	values, funcs := goScriptNames(`
	goscript.State(c, "count", 0)
	shared.Show(c, "color")
	note.Show(c, "note")
	room.Show(c, key, "people")
	boards.Show(c, "board:"+strconv.Itoa(id), "here")
	rooms.Show(c, fmt.Sprint("b", id), "crowd")
	c.Func("save", func() {})
`)
	if got := strings.Join(values, ","); got != "count,color,note,people,here,crowd" {
		t.Errorf("values = %s", got)
	}
	if strings.Join(funcs, ",") != "save" {
		t.Errorf("funcs = %v", funcs)
	}
}

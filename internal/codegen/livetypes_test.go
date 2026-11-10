package codegen

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestLiveComponents(t *testing.T) {
	files := map[string]string{
		"main.go":                 fixture["main.go"],
		"src/app.html":            fixture["src/app.html"],
		"src/routes/+page.svelte": ``,
		// Keeps the module required before tern gen writes code using it.
		"deps.go": "package main\n\nimport (\n\t_ \"github.com/Unfield/Tern/goscript\"\n\t_ \"github.com/Unfield/Tern/kit\"\n)\n",
		"src/lib/Todos.svelte": `<script lang="go" live>
import (
	"context"

	"github.com/Unfield/Tern/goscript"
)

type Todo struct {
	ID   int    ` + "`json:\"id\"`" + `
	Text string ` + "`json:\"text\"`" + `
	Done bool   ` + "`json:\"done,omitempty\"`" + `
}

var online = goscript.Presence("todos")

func Setup(c *goscript.Component) {
	todos := goscript.State(c, "todos", []Todo{})
	goscript.State[string](c, "draft", "", goscript.Bind)
	goscript.Derived(c, "count", func() int { return len(todos.Get()) })
	online.Show(c, "online")
	c.Func("add", func(ctx context.Context, text string) (Todo, error) { return Todo{}, nil })
	c.Func("clear", func() {})
	c.Func("type", func(e goscript.Event) {})
}
</script>

<p>{count}</p>
`,
	}
	dir := writeFixture(t, files)
	if _, err := Generate(dir); err != nil {
		t.Fatal(err)
	}
	id := goScriptID("src/lib/Todos.svelte")
	ts, err := os.ReadFile(filepath.Join(dir, LiveTypesDir, id+".ts"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(ts)
	for _, want := range []string{
		"export interface Todo {",
		"\tdone?: boolean;",
		"\ttodos: Array<Todo> | null;",
		"\tdraft: string;",
		"\tcount: number;",
		"\tonline: number;",
		"\tadd: LiveFunc<[text: string], Todo>;",
		"\tclear: LiveFunc<[], void>;",
		"\ttype: LiveFunc<[e: Event | { value?: string; checked?: boolean; key?: string }], void>;",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("types missing %q:\n%s", want, got)
		}
	}
	gen, _ := os.ReadFile(filepath.Join(dir, "tern.gen.go"))
	if !strings.Contains(string(gen), "goscript.Register(\""+id+"\"") || !strings.Contains(string(gen), "Live:") {
		t.Errorf("tern.gen.go does not register the component:\n%s", gen)
	}
	// The generated app builds.
	build := exec.Command("go", "vet", "./...")
	build.Dir = dir
	if out, err := build.CombinedOutput(); err != nil {
		t.Errorf("go vet: %v\n%s", err, out)
	}
}

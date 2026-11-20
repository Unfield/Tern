package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestCheckBars(t *testing.T) {
	root := t.TempDir()
	write := func(rel, text string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("src/routes/+layout.svelte", `<script>
	let x = { a: 1 };
</script>

<header class="top">x</header>

<style>
	.top {
		position: sticky;
		top: 0;
		/* padding: 0.6rem; in a comment */
		padding: 0.6rem 1rem;
		font-size: 0.95rem;
		line-height: 1.5;
	}
	.ok {
		position: fixed;
		top: 0.75rem;
		height: 64px;
		padding: 0.5rem 0.75rem;
		font-size: 0.875rem;
		line-height: 1.25rem;
	}
	.loose {
		padding: 0.6rem;
	}
</style>
`)
	write("src/app.css", "@media (min-width: 40rem) {\n\t.side { position: -webkit-sticky; position: sticky; top: calc(64px + 1rem); min-height: 2.1rem }\n}\n")
	write("src/node_modules/x.css", ".a { position: fixed; top: 0.3rem }")
	got, err := checkBars(root)
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for _, w := range got {
		lines = append(lines, w.File+":"+strconv.Itoa(w.Line)+" "+w.Prop+" "+w.Val)
	}
	want := []string{
		"src/app.css:2 min-height 2.1rem",
		"src/routes/+layout.svelte:12 padding 0.6rem 1rem",
		"src/routes/+layout.svelte:13 font-size 0.95rem",
		"src/routes/+layout.svelte:14 line-height 1.5",
	}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Errorf("got\n%s\nwant\n%s", strings.Join(lines, "\n"), strings.Join(want, "\n"))
	}
	if s := got[1].String(); !strings.Contains(s, "src/routes/+layout.svelte:12\nWarn: padding: 0.6rem 1rem is 9.6px") || !strings.Contains(s, "(10px)") {
		t.Errorf("message: %s", s)
	}
}

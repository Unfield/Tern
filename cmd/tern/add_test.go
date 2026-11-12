package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Unfield/Tern/plugins"
)

func mustFind(t *testing.T, name string) plugins.Plugin {
	t.Helper()
	p, ok := plugins.Find(name)
	if !ok {
		t.Fatalf("no plugin %s", name)
	}
	return p
}

func TestAddToMain(t *testing.T) {
	compress := mustFind(t, "compress")
	cases := []struct {
		name, src string
		pl        plugins.Plugin
		want      []string // in the result
		unchanged bool
	}{
		{
			name: "no plugins yet",
			src:  "package main\n\nimport \"log\"\n\nfunc main() {\n\tapp, err := ternApp()\n\tif err != nil {\n\t\tlog.Fatal(err)\n\t}\n\t_ = app\n}\n",
			pl:   compress,
			want: []string{"ternApp(kit.WithPlugins(compress.New()))", `"github.com/Unfield/Tern/kit"`, `"github.com/Unfield/Tern/plugins/compress"`},
		},
		{
			name: "on one line",
			src:  "package main\n\nimport \"github.com/Unfield/Tern/kit\"\n\nfunc main() {\n\tternApp(kit.WithPlugins(a(), b()))\n}\n",
			pl:   compress,
			want: []string{"kit.WithPlugins(a(), b(), compress.New())"},
		},
		{
			name: "one per line, with comments",
			src:  "package main\n\nimport \"github.com/Unfield/Tern/kit\"\n\nfunc main() {\n\tternApp(kit.WithPlugins(\n\t\ta(), // the first\n\t\tb(),\n\t))\n}\n",
			pl:   compress,
			want: []string{"\t\ta(), // the first\n\t\tb(),\n\t\tcompress.New(),\n\t))"},
		},
		{
			name:      "already there",
			src:       "package main\n\nimport (\n\t\"github.com/Unfield/Tern/kit\"\n\t\"github.com/Unfield/Tern/plugins/compress\"\n)\n\nfunc main() {\n\tternApp(kit.WithPlugins(compress.New(compress.Level(9))))\n}\n",
			pl:        compress,
			unchanged: true,
		},
		{
			name: "with other imports",
			src:  "package main\n\nfunc main() {\n\tternApp()\n}\n",
			pl:   mustFind(t, "sessions"),
			want: []string{`sessions.New(os.Getenv("SESSION_SECRET"))`, "\t\"os\"\n"},
		},
		{
			name: "imported under another name",
			src:  "package main\n\nimport (\n\t\"github.com/Unfield/Tern/kit\"\n\tgz \"github.com/Unfield/Tern/plugins/compress\"\n)\n\nvar _ = gz.Level\n\nfunc main() {\n\tternApp(kit.WithPlugins())\n}\n",
			pl:   compress,
			want: []string{"kit.WithPlugins(gz.New())"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, changed, err := addToMain([]byte(c.src), c.pl)
			if err != nil {
				t.Fatal(err)
			}
			if changed == c.unchanged {
				t.Fatalf("changed = %v:\n%s", changed, out)
			}
			for _, w := range c.want {
				if !strings.Contains(string(out), w) {
					t.Errorf("missing %q in:\n%s", w, out)
				}
			}
		})
	}
	if _, _, err := addToMain([]byte("package main\n\nfunc main() {}\n"), compress); err == nil {
		t.Error("a main.go without ternApp was changed")
	}
}

func TestAddFindsConstructors(t *testing.T) {
	_, here, _, _ := runtime.Caller(0)
	repo := filepath.Join(filepath.Dir(here), "..", "..")
	if got, err := findConstructor(repo, "github.com/Unfield/Tern/plugins/compress"); err != nil || got != "compress.New()" {
		t.Errorf("compress: %q, %v", got, err)
	}
	// A plugin that needs its options can't be added without them.
	if _, err := findConstructor(repo, "github.com/Unfield/Tern/plugins/oidc"); err == nil {
		t.Error("oidc has no constructor without arguments")
	}
}

func TestAddCommand(t *testing.T) {
	dir := t.TempDir()
	main := "package main\n\nfunc main() {\n\tternApp()\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(main), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	var out, errs strings.Builder
	if code := cmdAdd([]string{"health", "metrics"}, &out, &errs); code != 0 {
		t.Fatalf("exit %d: %s", code, errs.String())
	}
	b, _ := os.ReadFile(filepath.Join(dir, "main.go"))
	if !strings.Contains(string(b), "kit.WithPlugins(health.New(), metrics.New())") {
		t.Errorf("main.go:\n%s", b)
	}
	// Twice is once; configuration first is said, not added.
	out.Reset()
	if code := cmdAdd([]string{"health", "oauth"}, &out, &errs); code != 0 || !strings.Contains(out.String(), "already") || !strings.Contains(out.String(), "needs configuration") {
		t.Errorf("again: %d %s", code, out.String())
	}
	if code := cmdAdd([]string{"nope"}, &out, &errs); code == 0 {
		t.Error("an unknown name was added")
	}
	// Without arguments, the list.
	out.Reset()
	if cmdAdd(nil, &out, &errs) != 0 || !strings.Contains(out.String(), "compress") {
		t.Errorf("list: %s", out.String())
	}
}

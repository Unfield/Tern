package plugins_test

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/Unfield/Tern/plugins"
)

// Every plugin tern add adds compiles as it's added, and makes a kit.Plugin.
func TestRegistry(t *testing.T) {
	all := plugins.All()
	seen := map[string]bool{}
	imports := []string{"github.com/Unfield/Tern/kit"}
	var adds []string
	for _, p := range all {
		if p.Name == "" || p.Import == "" || p.Description == "" || p.Docs == "" || p.Author == "" {
			t.Errorf("%+v: name, import, description, docs and author are required", p)
		}
		if seen[p.Name] {
			t.Errorf("%s is listed twice", p.Name)
		}
		seen[p.Name] = true
		if !p.Official() && p.URL == "" {
			t.Errorf("%s: a community plugin needs its URL", p.Name)
		}
		if p.Add == "" {
			if p.Notes == "" {
				t.Errorf("%s: without add, notes must say how to set it up", p.Name)
			}
			continue
		}
		if !strings.HasPrefix(p.Add, path.Base(p.Import)+".") {
			t.Errorf("%s: add should start with its package name, %s", p.Name, path.Base(p.Import))
		}
		if p.Official() {
			imports = append(imports, p.Import)
			imports = append(imports, p.Imports...)
			adds = append(adds, p.Add)
		}
	}
	slices.Sort(imports)
	imports = slices.Compact(imports)

	_, here, _, _ := runtime.Caller(0)
	repo := filepath.Join(filepath.Dir(here), "..")
	dir := t.TempDir()
	var src strings.Builder
	src.WriteString("package main\n\nimport (\n")
	for _, imp := range imports {
		fmt.Fprintf(&src, "\t%q\n", imp)
	}
	src.WriteString(")\n\nfunc main() {\n\tplugins := []kit.Plugin{\n")
	for _, a := range adds {
		fmt.Fprintf(&src, "\t\t%s,\n", a)
	}
	src.WriteString("\t}\n\t_ = kit.WithPlugins(plugins...)\n}\n")
	sum, err := os.ReadFile(filepath.Join(repo, "go.sum"))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"main.go": src.String(),
		"go.mod":  "module example.com/app\n\ngo 1.26\n\nrequire github.com/Unfield/Tern v0.0.0\n\nreplace github.com/Unfield/Tern => " + strconv.Quote(filepath.ToSlash(repo)) + "\n",
		"go.sum":  string(sum),
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{{"mod", "tidy", "-e"}, {"vet", "."}} {
		cmd := exec.Command("go", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod", "GOPROXY=off")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("go %s: %v\n%s\n%s", strings.Join(args, " "), err, out, src.String())
		}
	}
}

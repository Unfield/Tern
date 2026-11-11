package native

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestConformance compiles each program in testdata/conformance to Go,
// runs it, and checks globalThis.out against what Node computed
// (testdata/conformance.json, written by testdata/conformance.mjs).
func TestConformance(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a Go program")
	}
	raw, err := os.ReadFile("testdata/conformance.json")
	if err != nil {
		t.Fatal(err)
	}
	var data struct {
		Cases []struct {
			Name string
			Want string
			IR   *IR
		}
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	gomod := fmt.Sprintf("module conformance\n\ngo 1.25\n\nrequire github.com/Unfield/Tern v0.0.0\n\nreplace github.com/Unfield/Tern => %q\n", root)
	write := func(name, s string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", gomod)
	if sum, err := os.ReadFile(filepath.Join(root, "go.sum")); err == nil {
		write("go.sum", string(sum))
	}
	var main strings.Builder
	main.WriteString("package main\n\nimport (\n\t\"fmt\"\n\n\t\"github.com/Unfield/Tern/native/js\"\n)\n\nfunc run(name string, newRealm func(func(*js.Realm)) *js.Realm) {\n")
	main.WriteString("\tdefer func() {\n\t\tif r := recover(); r != nil {\n\t\t\tfmt.Printf(\"%s\\tpanic: %v\\n\", name, r)\n\t\t}\n\t}()\n")
	main.WriteString("\trt := newRealm(nil)\n\trt.RunJobs()\n\ts, _ := rt.Stringify(rt.Global.Get(\"out\"), js.Undefined, js.Undefined)\n\tfmt.Printf(\"%s\\t%s\\n\", name, s)\n}\n\nfunc main() {\n")
	for i, c := range data.Cases {
		prefix := fmt.Sprintf("c%d", i)
		res, err := Compile(c.IR, Options{Package: "main", Prefix: prefix})
		if err != nil {
			t.Fatalf("%s: %v", c.Name, err)
		}
		for _, p := range res.Problems {
			t.Errorf("%s: not compiled: %s", c.Name, p)
		}
		if c.Name == "svelte_helpers" {
			// The case is there to check the Go versions against Node.
			for _, want := range []string{"svelte.EscapeHTML(", "svelte.Attr(", "svelte.ToClass(", "svelte.AttrClass(", "svelte.Stringify(", "svelte.EnsureArrayLike("} {
				if !strings.Contains(string(res.Source), want) {
					t.Errorf("svelte_helpers doesn't use %s…): %v", want, res.Mismatches)
				}
			}
		}
		write(c.Name+".go", string(res.Source))
		fmt.Fprintf(&main, "\trun(%q, %sNew)\n", c.Name, prefix)
	}
	main.WriteString("}\n")
	write("main.go", main.String())
	cmd := exec.Command("go", "run", ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("running the compiled cases: %v\n%s", err, stderr.String())
	}
	got := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		name, v, _ := strings.Cut(line, "\t")
		got[name] = v
	}
	if len(got) != len(data.Cases) {
		t.Errorf("ran %d of %d cases:\n%s", len(got), len(data.Cases), out)
	}
	for _, c := range data.Cases {
		t.Logf("%s: %s", c.Name, got[c.Name])
		if got[c.Name] != c.Want {
			t.Errorf("%s:\n got %s\nwant %s", c.Name, got[c.Name], c.Want)
		}
	}
}

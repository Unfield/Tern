package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNew(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "my-app")
	var stdout, stderr bytes.Buffer
	if code := run([]string{"new", "-skip-go-get", dir}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit code %d: %s", code, stderr.String())
	}
	for _, f := range []string{"go.mod", "main.go", "package.json", ".gitignore", "README.md", "vite.config.js", "tsconfig.json",
		"src/app.html", "src/routes/+page.svelte", "src/routes/page.server.go", "src/routes/+layout.svelte", "static/favicon.svg"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("missing %s", f)
		}
	}
	pkg, _ := os.ReadFile(filepath.Join(dir, "package.json"))
	if !strings.Contains(string(pkg), `"name": "my-app"`) {
		t.Errorf("package.json not templated:\n%s", pkg)
	}
	gomod, _ := os.ReadFile(filepath.Join(dir, "go.mod"))
	if !strings.HasPrefix(string(gomod), "module my-app\n") {
		t.Errorf("go.mod = %q", gomod)
	}

	if code := run([]string{"new", "-skip-go-get", dir}, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "is not empty") {
		t.Errorf("creating into a non-empty directory should fail, got %d: %s", code, stderr.String())
	}
	if code := run([]string{"new", "Bad Name"}, &stdout, &stderr); code != 2 {
		t.Errorf("invalid name exit code = %d", code)
	}
}

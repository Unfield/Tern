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
		"Dockerfile", ".dockerignore", "src/app.html", "src/routes/+page.svelte", "src/routes/page.server.go", "src/routes/+layout.svelte", "static/favicon.svg"} {
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

func TestNewInteractive(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	oldIn, oldTTY := stdin, stdinIsTerminal
	t.Cleanup(func() { stdin, stdinIsTerminal = oldIn, oldTTY })
	stdinIsTerminal = func() bool { return true }
	// A bad name, then a good one; an unknown answer, then a bare app; an
	// unknown template, then blog; Tern UI yes; git and npm install no.
	stdin = strings.NewReader("Bad Name\nsite\nsomething\n1\nnope\n2\n\nn\nno\n")

	var stdout, stderr bytes.Buffer
	if code := run([]string{"new", "-skip-go-get"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit code %d: %s\n%s", code, stderr.String(), stdout.String())
	}
	for _, f := range []string{"site/src/routes/posts", "site/src/routes/+layout.svelte"} {
		if _, err := os.Stat(filepath.Join(root, f)); err != nil {
			t.Errorf("missing %s", f)
		}
	}
	layout, _ := os.ReadFile(filepath.Join(root, "site/src/routes/+layout.svelte"))
	if !strings.Contains(string(layout), "@ternkit/tern/ui/theme.css") {
		t.Error("the blog with Tern UI should import the theme")
	}
	if _, err := os.Stat(filepath.Join(root, "site/.git")); err == nil {
		t.Error("git was declined")
	}
	if !strings.Contains(stdout.String(), "npm install") {
		t.Error("next steps should still say npm install")
	}

	stdin = strings.NewReader("other\n")
	if code := run([]string{"new", "-skip-go-get"}, &stdout, &stderr); code != 1 {
		t.Errorf("input ending early should fail, got %d", code)
	}
}

func TestNewInteractiveFullApp(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	oldIn, oldTTY := stdin, stdinIsTerminal
	t.Cleanup(func() { stdin, stdinIsTerminal = oldIn, oldTTY })
	stdinIsTerminal = func() bool { return true }
	// A full app asks no template or Tern UI question: git and npm no.
	stdin = strings.NewReader("product\nfull\nn\nn\n")
	var stdout, stderr bytes.Buffer
	if code := run([]string{"new", "-skip-go-get"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit code %d: %s\n%s", code, stderr.String(), stdout.String())
	}
	for _, f := range []string{"product/src/routes/app.group/dashboard/+page.svelte", "product/internal/app/app.go", "product/migrations/0001_projects.sql", "product/.env.example"} {
		if _, err := os.Stat(filepath.Join(root, f)); err != nil {
			t.Errorf("missing %s", f)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "product/src/routes/about")); err == nil {
		t.Error("the bare template's about page is in the full app")
	}
	main, _ := os.ReadFile(filepath.Join(root, "product/main.go"))
	if !strings.Contains(string(main), `"product/internal/app"`) {
		t.Errorf("main.go doesn't import the module's packages:\n%s", main)
	}
	if strings.Contains(stdout.String(), "Style it with Tern UI?") {
		t.Error("a full app asked about Tern UI")
	}
}

func TestNewUI(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "app")
	var stdout, stderr bytes.Buffer
	if code := run([]string{"new", "-skip-go-get", dir, "-ui", "-git"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit code %d: %s", code, stderr.String())
	}
	page, _ := os.ReadFile(filepath.Join(dir, "src/routes/+page.svelte"))
	if !strings.Contains(string(page), "@ternkit/tern/ui") {
		t.Errorf("the minimal page should use Tern UI:\n%s", page)
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		t.Errorf("no git repository: %s", stdout.String())
	}
}

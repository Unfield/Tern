package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func writeFiles(t *testing.T, root string, files ...string) {
	t.Helper()
	for _, f := range files {
		p := filepath.Join(root, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		var data []byte
		if strings.HasSuffix(p, ".go") {
			data = []byte("package page\nfunc Load() {}\nfunc GET() {}\n")
		}
		if err := os.WriteFile(p, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRoutesCommand(t *testing.T) {
	t.Chdir(t.TempDir())
	writeFiles(t, ".",
		"src/routes/+layout.svelte",
		"src/routes/+page.svelte",
		"src/routes/blog/slug.param/+page.svelte",
		"src/routes/blog/slug.param/page.server.go",
		"src/routes/api/health/+page.svelte",
		"src/routes/api/health/server.go",
	)

	var stdout, stderr bytes.Buffer
	if code := run([]string{"routes"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr.String())
	}

	var lines []string
	for _, l := range strings.Split(stdout.String(), "\n") {
		if l = strings.TrimSpace(l); l != "" && !strings.HasSuffix(l, "routes") {
			lines = append(lines, l)
		}
	}
	if len(lines) != 4 {
		t.Fatalf("got %d lines, want 4:\n%s", len(lines), stdout.String())
	}
	wantPrefixes := []string{"ROUTE", "/ ", "/api/health ", "/blog/[slug] "}
	for i, want := range wantPrefixes {
		if !strings.HasPrefix(lines[i], want) {
			t.Errorf("line %d = %q, want prefix %q", i, lines[i], want)
		}
	}
	if !strings.Contains(lines[3], "yes") {
		t.Errorf("blog line = %q, want a load", lines[3])
	}
	if !strings.Contains(lines[2], "page+endpoint") {
		t.Errorf("api line = %q, want page+endpoint kind", lines[2])
	}
}

func TestRoutesCommandErrors(t *testing.T) {
	t.Chdir(t.TempDir())
	writeFiles(t, ".", "app/routes/about/page.server.go")

	var stdout, stderr bytes.Buffer
	if code := run([]string{"routes", "app/routes"}, &stdout, &stderr); code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "there is no +page.svelte") {
		t.Errorf("stderr = %q", stderr.String())
	}

	stderr.Reset()
	if code := run([]string{"routes", "a", "b"}, &stdout, &stderr); code != 2 {
		t.Fatalf("exit code = %d, want 2 for too many arguments", code)
	}
}

func TestLockDev(t *testing.T) {
	root := t.TempDir()
	unlock, err := lockDev(root)
	if err != nil {
		t.Fatal(err)
	}
	// Our own lock file looks stale to a second attempt in the same process,
	// so simulate another live process instead.
	pidFile := filepath.Join(root, ".tern", "dev", "tern-dev.pid")
	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(os.Getppid())+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := lockDev(root); err == nil || !strings.Contains(err.Error(), "already running") {
		t.Fatalf("second lock: %v", err)
	}
	unlock()

	// A stale lock from a process that no longer exists is replaced.
	if err := os.WriteFile(pidFile, []byte("999999\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	unlock, err = lockDev(root)
	if err != nil {
		t.Fatalf("stale lock: %v", err)
	}
	unlock()
}

//go:build !windows

package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// TestDevServer runs `tern dev` against examples/todos. It needs Node.js
// and the workspace's node_modules, so it only runs when TERN_INTEGRATION=1.
func TestDevServer(t *testing.T) {
	if os.Getenv("TERN_INTEGRATION") != "1" {
		t.Skip("set TERN_INTEGRATION=1 to run (needs pnpm install)")
	}
	_, here, _, _ := runtime.Caller(0)
	repo := filepath.Join(filepath.Dir(here), "..", "..")
	app := filepath.Join(repo, "examples", "todos")

	bin := filepath.Join(t.TempDir(), "tern")
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = filepath.Dir(here)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building tern: %v\n%s", err, out)
	}

	port, err := freePort()
	if err != nil {
		t.Fatal(err)
	}
	base := "http://localhost:" + strconv.Itoa(port)
	var logs bytes.Buffer
	out := &syncWriter{w: &logs}
	logsString := func() string {
		out.mu.Lock()
		defer out.mu.Unlock()
		return logs.String()
	}
	cmd := exec.Command(bin, "dev", "-port", strconv.Itoa(port))
	cmd.Dir = app
	cmd.Stdout, cmd.Stderr = out, out
	// Own process group, so cleanup can stop Vite and the app too.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = 5 * time.Second
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan struct{})
	go func() {
		_ = cmd.Wait() // the exit status after a signal is not interesting
		close(exited)
	}()
	t.Cleanup(func() {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		select {
		case <-exited:
		case <-time.After(10 * time.Second):
			t.Log("tern dev did not exit after SIGKILL")
		}
	})

	client := &http.Client{Timeout: 10 * time.Second}
	get := func(path string) (int, string) {
		t.Helper()
		req, _ := http.NewRequest(http.MethodGet, base+path, nil)
		req.Header.Set("Accept", "text/html")
		resp, err := client.Do(req)
		if err != nil {
			return 0, err.Error()
		}
		defer func() { _ = resp.Body.Close() }()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	waitFor := func(what string, timeout time.Duration, ok func() bool) {
		t.Helper()
		t.Logf("waiting for %s", what)
		deadline := time.Now().Add(timeout)
		for !ok() {
			if time.Now().After(deadline) {
				t.Fatalf("timed out waiting for %s\nlogs:\n%s", what, logsString())
			}
			time.Sleep(200 * time.Millisecond)
		}
	}

	waitFor("the dev server", 90*time.Second, func() bool { code, _ := get("/login"); return code == 200 })
	_, body := get("/login")
	for _, want := range []string{"data-tern-ssr", "<h1>Sign in</h1>", "/@vite/client", "/__tern/dev/client.js"} {
		if !strings.Contains(body, want) {
			t.Errorf("dev page missing %q", want)
		}
	}
	if code, js := get("/.tern/generated/client.js"); code != 200 || !strings.Contains(js, "start") {
		t.Errorf("Vite module through the proxy: %d %q", code, js)
	}
	if !strings.Contains(logsString(), "ready in") {
		t.Errorf("missing banner:\n%s", logs.String())
	}

	// A Go change rebuilds and restarts the server.
	file := filepath.Join(app, "src", "routes", "login", "page.server.go")
	original, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.WriteFile(file, original, 0o644) })
	edited := strings.Replace(string(original), "Enter a name of up to 40 characters.", "Names are 1 to 40 characters.", 1)
	if err := os.WriteFile(file, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	waitFor("the restart after a Go change", 60*time.Second, func() bool { return strings.Contains(logsString(), "server restarted") })

	// A compile error is shown in the browser, and fixing it recovers.
	if err := os.WriteFile(file, []byte(edited+"\nvar broken = undefinedName\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	waitFor("the build error page", 60*time.Second, func() bool {
		code, b := get("/login")
		return code == 500 && strings.Contains(b, "undefinedName")
	})
	if err := os.WriteFile(file, original, 0o644); err != nil {
		t.Fatal(err)
	}
	waitFor("recovery after the fix", 60*time.Second, func() bool { code, _ := get("/login"); return code == 200 })

	// Ctrl-C stops everything.
	_ = cmd.Process.Signal(syscall.SIGINT)
	select {
	case <-exited:
	case <-time.After(15 * time.Second):
		t.Fatalf("tern dev did not exit after SIGINT\nlogs:\n%s", logsString())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if out, _ := exec.CommandContext(ctx, "pgrep", "-f", filepath.Join(app, ".tern", "dev", "app")).Output(); len(bytes.TrimSpace(out)) > 0 {
		t.Errorf("the app process is still running after tern dev exited: %s", out)
	}
}

// syncWriter serializes writes from the two output streams.
type syncWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (s *syncWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(p)
}

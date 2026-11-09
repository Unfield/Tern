package main

import (
	"compress/gzip"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/Unfield/Tern/internal/codegen"
)

// cmdBuild produces a production binary with the client embedded.
func cmdBuild(args []string, stdout, stderr io.Writer) int {
	fl := flag.NewFlagSet("build", flag.ContinueOnError)
	fl.SetOutput(stderr)
	out := fl.String("o", "", "output binary (default build/<directory name>)")
	if err := fl.Parse(args); err != nil {
		return 2
	}
	if fl.NArg() > 0 {
		fmt.Fprintln(stderr, "usage: tern build [-o output]")
		return 2
	}
	if *out == "" {
		wd, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(stderr, "tern build: %v\n", err)
			return 1
		}
		*out = filepath.Join("build", filepath.Base(wd))
		if runtime.GOOS == "windows" {
			*out += ".exe"
		}
	}

	start := time.Now()
	step := func(name string, fn func() error) bool {
		fmt.Fprintf(stdout, "  %s\n", name)
		if err := fn(); err != nil {
			fmt.Fprintf(stderr, "tern build: %v\n", err)
			return false
		}
		return true
	}

	fmt.Fprintln(stdout, "Building for production")
	var gen *codegen.Result
	compile := func() error {
		cmd := exec.Command("go", "build", "-trimpath", "-ldflags=-s -w", "-o", *out, ".")
		cmd.Stdout, cmd.Stderr = stdout, stderr
		return cmd.Run()
	}
	ok := step("generating routes and types", func() error {
		var err error
		gen, err = codegen.Generate(".")
		return err
	}) && step("building the client with Vite", func() error {
		return runVite(stdout, stderr, "build", "--logLevel", "warn")
	}) && step("building the server renderer", func() error {
		return runVite(stdout, stderr, "build", "--ssr", "--logLevel", "warn")
	}) && step("compressing assets", func() error {
		return precompress(codegen.ClientOut)
	}) && step("compiling the server", compile)
	if ok && gen.Prerender {
		ok = step("prerendering pages", func() error {
			n, err := prerender(*out)
			if err == nil {
				noun := "pages"
				if n == 1 {
					noun = "page"
				}
				fmt.Fprintf(stdout, "    %d %s\n", n, noun)
			}
			return err
		}) && step("compressing prerendered pages", func() error {
			return precompress(codegen.ClientOut)
		}) && step("embedding prerendered pages", compile)
	}
	if !ok {
		return 1
	}
	fmt.Fprintf(stdout, "Built %s in %s\n", *out, time.Since(start).Round(time.Millisecond))
	return 0
}

// prerender runs the built app in prerender mode and saves the HTML and
// data of every page marked with `const Prerender = true` into the client
// output, so the next compile embeds them as static files.
func prerender(bin string) (int, error) {
	port, err := freePort()
	if err != nil {
		return 0, err
	}
	abs, err := filepath.Abs(bin)
	if err != nil {
		return 0, err
	}
	var logs strings.Builder
	cmd := exec.Command(abs)
	cmd.Env = append(os.Environ(), "TERN_PRERENDER=1", "HOST=127.0.0.1", "PORT="+strconv.Itoa(port))
	cmd.Stdout, cmd.Stderr = &logs, &logs
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	exited := make(chan struct{})
	go func() { _ = cmd.Wait(); close(exited) }()
	defer func() {
		_ = cmd.Process.Kill()
		<-exited
	}()

	base := "http://127.0.0.1:" + strconv.Itoa(port)
	client := &http.Client{Timeout: 30 * time.Second}
	var list struct {
		Paths  []string `json:"paths"`
		Errors []string `json:"errors"`
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		select {
		case <-exited:
			return 0, fmt.Errorf("the app exited while prerendering:\n%s", logs.String())
		default:
		}
		resp, err := client.Get(base + "/__tern/prerender")
		if err == nil {
			err = json.NewDecoder(resp.Body).Decode(&list)
			_ = resp.Body.Close()
			if err != nil {
				return 0, fmt.Errorf("reading the prerender list: %w", err)
			}
			break
		}
		if time.Now().After(deadline) {
			return 0, fmt.Errorf("the app did not start for prerendering:\n%s", logs.String())
		}
		time.Sleep(50 * time.Millisecond)
	}
	if len(list.Errors) > 0 {
		return 0, errors.New(strings.Join(list.Errors, "\n"))
	}

	fetch := func(urlPath, file, accept string) error {
		req, err := http.NewRequest(http.MethodGet, base+urlPath, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Accept", accept)
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("prerendering %s: status %d", urlPath, resp.StatusCode)
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			return err
		}
		return os.WriteFile(file, body, 0o644)
	}
	for _, p := range list.Paths {
		dir, err := urlPathToDir(p)
		if err != nil {
			return 0, err
		}
		dataPath := strings.TrimSuffix(p, "/") + "/__data.json"
		if err := fetch(p, filepath.Join(dir, "index.html"), "text/html"); err != nil {
			return 0, err
		}
		if err := fetch(dataPath, filepath.Join(dir, "__data.json"), "application/json"); err != nil {
			return 0, err
		}
	}
	return len(list.Paths), nil
}

// urlPathToDir maps an escaped URL path to a directory in the client output.
func urlPathToDir(p string) (string, error) {
	dir := codegen.ClientOut
	for _, seg := range strings.Split(strings.Trim(p, "/"), "/") {
		if seg == "" {
			continue
		}
		name, err := url.PathUnescape(seg)
		if err != nil || name == "." || name == ".." || strings.ContainsAny(name, `/\`) {
			return "", fmt.Errorf("cannot prerender %q: invalid path segment", p)
		}
		dir = filepath.Join(dir, name)
	}
	return dir, nil
}

// viteBin returns the project's local Vite executable.
func viteBin() (string, error) {
	name := "vite"
	if runtime.GOOS == "windows" {
		name = "vite.cmd"
	}
	p := filepath.Join("node_modules", ".bin", name)
	if _, err := os.Stat(p); err != nil {
		return "", errors.New("vite is not installed; run your package manager's install command (npm install, pnpm install, …) first")
	}
	return p, nil
}

func runVite(stdout, stderr io.Writer, args ...string) error {
	bin, err := viteBin()
	if err != nil {
		return err
	}
	cmd := exec.Command(bin, args...)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("vite %s: %w", args[0], err)
	}
	return nil
}

var compressible = map[string]bool{
	".js": true, ".mjs": true, ".css": true, ".html": true, ".svg": true, ".json": true,
	".txt": true, ".xml": true, ".wasm": true, ".map": true, ".webmanifest": true, ".ico": true,
}

// precompress writes a .gz next to every compressible asset over 1 KiB
// when compression saves space. The server serves it to clients that
// accept gzip.
func precompress(dir string) error {
	return filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !compressible[strings.ToLower(filepath.Ext(p))] {
			return err
		}
		info, err := d.Info()
		if err != nil || info.Size() < 1024 {
			return err
		}
		src, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		var buf strings.Builder
		zw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
		if err != nil {
			return err
		}
		if _, err := zw.Write(src); err != nil {
			return err
		}
		if err := zw.Close(); err != nil {
			return err
		}
		if buf.Len() >= len(src) {
			return nil
		}
		return os.WriteFile(p+".gz", []byte(buf.String()), 0o644)
	})
}

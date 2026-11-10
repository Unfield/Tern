package main

import (
	"bytes"
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
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/Unfield/Tern/internal/codegen"
	"github.com/Unfield/Tern/internal/ui"
	"github.com/Unfield/Tern/internal/version"
)

// cmdBuild produces a production binary with the client embedded.
func cmdBuild(args []string, stdout, stderr io.Writer) int {
	p := ui.New(stdout)
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
			ui.New(stderr).Error(err)
			return 1
		}
		*out = filepath.Join("build", filepath.Base(wd))
		if runtime.GOOS == "windows" {
			*out += ".exe"
		}
	}

	start := time.Now()
	// Each step's tool output is captured and shown only if the step fails.
	step := func(label string, fn func(log io.Writer) (string, error)) bool {
		var log bytes.Buffer
		s := p.Spin(label)
		note, err := fn(&log)
		if err != nil {
			s.Fail()
			if log.Len() > 0 {
				p.Println()
				p.Println(strings.TrimRight(log.String(), "\n"))
			}
			p.Println()
			ui.New(stderr).Error(err)
			return false
		}
		s.Done(note)
		return true
	}

	p.Printf("\n  %s %s\n\n", p.Brand(p.Bold("tern build")), p.Dim(version.String()))
	var gen *codegen.Result
	compile := func(log io.Writer) (string, error) {
		cmd := exec.Command("go", "build", "-trimpath", "-ldflags=-s -w", "-o", *out, ".")
		cmd.Stdout, cmd.Stderr = log, log
		if err := cmd.Run(); err != nil {
			return "", errors.New("go build failed")
		}
		return "", nil
	}
	ok := step("Generating routes and types", func(io.Writer) (string, error) {
		var err error
		gen, err = codegen.Generate(".")
		if err != nil {
			return "", err
		}
		return plural(len(gen.Manifest.Routes), "route"), nil
	}) && step("Building the client", func(log io.Writer) (string, error) {
		return "", runVite(log, "build", "--logLevel", "warn")
	}) && step("Building the server renderer", func(log io.Writer) (string, error) {
		return "", runVite(log, "build", "--ssr", "--logLevel", "warn")
	}) && step("Compressing assets", func(io.Writer) (string, error) {
		return "", precompress(codegen.ClientOut)
	}) && step("Compiling the server", compile)
	needsCompile := false
	if ok && gen.Prerender {
		ok = step("Prerendering pages", func(io.Writer) (string, error) {
			cfg, err := codegen.LoadConfig(".")
			if err != nil {
				return "", err
			}
			n, err := prerender(*out, cfg.BasePath)
			return plural(n, "page"), err
		}) && step("Compressing prerendered pages", func(io.Writer) (string, error) {
			return "", precompress(codegen.ClientOut)
		})
		needsCompile = true
	}
	// The service worker comes last, so its file lists include every asset
	// and prerendered page.
	if ok && codegen.FindServiceWorker(".") != "" {
		ok = step("Building the service worker", func(log io.Writer) (string, error) {
			return "", runViteEnv(log, []string{"TERN_SW_BUILD=1"}, "build", "--logLevel", "warn")
		})
		needsCompile = true
	}
	if ok && needsCompile {
		ok = step("Embedding the final assets", compile)
	}
	if !ok {
		return 1
	}

	binSize := int64(0)
	if info, err := os.Stat(*out); err == nil {
		binSize = info.Size()
	}
	raw, gz := clientSize(codegen.ClientOut)
	p.Println()
	p.Printf("  %s %s %s\n", p.Green("✓"), p.Bold("Built "+*out), p.Dim("in "+ui.Duration(time.Since(start))))
	p.Printf("    %s %s\n", p.Dim("binary "), ui.Size(binSize))
	p.Printf("    %s %s %s\n", p.Dim("client "), ui.Size(raw), p.Dim("("+ui.Size(gz)+" gzipped)"))
	p.Printf("\n  Run it with %s\n\n", p.Cyan("./"+filepath.ToSlash(*out)))
	return 0
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return strconv.Itoa(n) + " " + noun + "s"
}

// clientSize sums the client output, counting gzipped files at their
// compressed size for the second total.
func clientSize(dir string) (raw, gzipped int64) {
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.HasPrefix(filepath.Base(p), ".") || strings.Contains(p, ".vite") {
			return nil //nolint:nilerr // best effort
		}
		info, err := d.Info()
		if err != nil {
			return nil //nolint:nilerr // best effort
		}
		if strings.HasSuffix(p, ".gz") {
			gzipped += info.Size()
			return nil
		}
		raw += info.Size()
		if _, err := os.Stat(p + ".gz"); err != nil {
			gzipped += info.Size()
		}
		return nil
	})
	return raw, gzipped
}

// prerender runs the built app in prerender mode and saves the HTML and
// data of every page marked with `const Prerender = true` into the client
// output, so the next compile embeds them as static files.
func prerender(bin, basePath string) (int, error) {
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

	origin := "http://127.0.0.1:" + strconv.Itoa(port)
	base := origin + basePath
	client := &http.Client{Timeout: 30 * time.Second}
	var list struct {
		Paths     []string `json:"paths"`
		Endpoints []string `json:"endpoints"`
		Errors    []string `json:"errors"`
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		select {
		case <-exited:
			return 0, fmt.Errorf("the app exited while prerendering:\n%s", logs.String())
		default:
		}
		resp, err := client.Get(base + "/__tern/prerender")
		if err == nil && resp.StatusCode == http.StatusFound {
			_ = resp.Body.Close()
			err = errors.New("redirected")
		}
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
	for _, p := range list.Endpoints {
		dir, err := urlPathToDir(path.Dir(p))
		if err != nil {
			return 0, err
		}
		name, err := url.PathUnescape(path.Base(p))
		if err != nil {
			return 0, err
		}
		if err := fetch(p, filepath.Join(dir, name), "*/*"); err != nil {
			return 0, err
		}
	}
	return len(list.Paths) + len(list.Endpoints), nil
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

func runVite(out io.Writer, args ...string) error {
	return runViteEnv(out, nil, args...)
}

func runViteEnv(out io.Writer, env []string, args ...string) error {
	bin, err := viteBin()
	if err != nil {
		return err
	}
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdout, cmd.Stderr = out, out
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

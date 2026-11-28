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

	"github.com/andybalholm/brotli"

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
	outDir := fl.String("out", "", "write a directory for hosting platforms: the server, the static files and tern-output.json (ADR 0029)")
	if err := fl.Parse(args); err != nil {
		return 2
	}
	if fl.NArg() > 0 || *out != "" && *outDir != "" {
		fmt.Fprintln(stderr, "usage: tern build [-o output | -out directory]")
		return 2
	}
	if *outDir != "" {
		if err := clearOutput(*outDir); err != nil {
			ui.New(stderr).Error(err)
			return 1
		}
		*out = outputBinary(*outDir)
		// A platform runs the server as it is: a static binary, unless
		// the app asks for cgo.
		if os.Getenv("CGO_ENABLED") == "" {
			_ = os.Setenv("CGO_ENABLED", "0")
		}
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
			// The tool's output explains the failure: it goes to stderr with
			// the error, where CI logs and process managers keep it.
			e := ui.New(stderr)
			if log.Len() > 0 {
				e.Println()
				e.Println(strings.TrimRight(log.String(), "\n"))
			}
			e.Println()
			e.Error(err)
			return false
		}
		s.Done(note)
		return true
	}

	p.Printf("\n  %s %s\n\n", p.Brand(p.Bold("tern build")), p.Dim(version.String()))
	cfg, err := codegen.LoadConfig(".")
	if err != nil {
		ui.New(stderr).Error(err)
		return 1
	}
	native := cfg.NativeEnabled() && os.Getenv("TERN_NATIVE") != "0"
	var gen *codegen.Result
	compile := func(log io.Writer) (string, error) {
		goBuild := func(w io.Writer) error { return goBuildTo(w, *out, native, nil) }
		var first bytes.Buffer
		if err := goBuild(&first); err != nil {
			if _, statErr := os.Stat(nativeFile); statErr != nil || !bytes.Contains(first.Bytes(), []byte(nativeFile)) {
				_, _ = log.Write(first.Bytes())
				return "", errors.New("go build failed")
			}
			// The compiled renderer doesn't compile: a bug in Tern's
			// compiler, not the app. Build without it; pages render with the
			// JavaScript engine.
			_ = os.Rename(nativeFile, nativeReport+".go.txt")
			engineFree = false
			if err := goBuild(log); err != nil {
				return "", errors.New("go build failed")
			}
			return "without native rendering: the compiled renderer didn't build (please report it; it's kept in " + nativeReport + ".go.txt)", nil
		}
		if withoutEngine(native) {
			return "without the JavaScript engine: every page compiles to Go", nil
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
		_ = os.Remove(nativeFile) // stale from an earlier build
		var env []string
		if native {
			env = append(env, "TERN_NATIVE=1")
		}
		return "", runViteEnv(log, env, "build", "--ssr", "--logLevel", "warn")
	}) && (!native || step("Compiling the server renderer to Go", compileNative)) && step("Compressing assets", func(io.Writer) (string, error) {
		return "", precompress(codegen.ClientOut)
	}) && step("Compiling the server", compile)
	needsCompile := false
	if ok && gen.Prerender {
		ok = step("Prerendering pages", func(io.Writer) (string, error) {
			cfg, err := codegen.LoadConfig(".")
			if err != nil {
				return "", err
			}
			bin, cleanup, err := hostBinary(*out, native)
			if err != nil {
				return "", err
			}
			defer cleanup()
			n, err := prerender(bin, cfg.BasePath)
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
	if ok && *outDir != "" {
		ok = step("Writing "+*outDir, func(io.Writer) (string, error) {
			bin, cleanup, err := hostBinary(*out, native)
			if err != nil {
				return "", err
			}
			defer cleanup()
			n, err := writeOutput(*outDir, bin, *out, cfg.BasePath)
			return plural(n, "static file"), err
		})
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
	if buildRunHint {
		run := filepath.ToSlash(*out)
		if !filepath.IsAbs(*out) {
			run = "./" + run
		}
		p.Printf("\n  Run it with %s\n\n", p.Cyan(run))
	} else {
		p.Println()
	}
	return 0
}

// hostBinary returns a binary of the app for this system, to run during
// the build: out itself, or, when out is for another system, one built for
// this one, which cleanup removes.
func hostBinary(out string, native bool) (bin string, cleanup func(), err error) {
	if !crossBuild() {
		return out, func() {}, nil
	}
	bin = filepath.Join(os.TempDir(), "tern-prerender-"+strconv.Itoa(os.Getpid()))
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	var log bytes.Buffer
	if err := goBuildTo(&log, bin, native, []string{"GOOS=" + runtime.GOOS, "GOARCH=" + runtime.GOARCH}); err != nil {
		return "", nil, fmt.Errorf("building a binary to run here: %w\n%s", err, log.String())
	}
	return bin, func() { _ = os.Remove(bin) }, nil
}

// goBuildTo builds the app's binary to out, with env added to the
// environment.
func goBuildTo(w io.Writer, out string, native bool, env []string) error {
	args := []string{"build", "-trimpath", "-ldflags=-s -w"}
	if native {
		// No inlining in the main package, which holds the compiled
		// renderer: a tenth of the binary, and no slower renders, since
		// the hot paths are in native/js.
		args = append(args, "-gcflags=-l")
	}
	if withoutEngine(native) {
		args = append(args, "-tags=tern_nojs")
		// Nothing runs the server bundle without the engine: leave it out
		// of the binary, a megabyte or more.
		restore, err := hideServerBundle()
		if err != nil {
			return err
		}
		defer restore()
	}
	cmd := exec.Command("go", append(args, "-o", out, ".")...)
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdout, cmd.Stderr = w, w
	return cmd.Run()
}

// hideServerBundle moves the server bundle out of the directory the app
// embeds, leaving a placeholder, since an embedded directory can't be
// empty. restore puts it back.
func hideServerBundle() (restore func(), err error) {
	dir := filepath.FromSlash(codegen.ServerOut)
	hidden := dir + ".hidden"
	_ = os.RemoveAll(hidden)
	if err := os.Rename(dir, hidden); err != nil {
		return nil, err
	}
	err = os.MkdirAll(dir, 0o755)
	if err == nil {
		err = os.WriteFile(filepath.Join(dir, ".keep"), nil, 0o644)
	}
	if err != nil {
		_ = os.Rename(hidden, dir)
		return nil, err
	}
	return func() {
		_ = os.RemoveAll(dir)
		_ = os.Rename(hidden, dir)
	}, nil
}

// withoutEngine is whether the binary leaves the JavaScript engine out:
// everything compiled to Go, and nothing asks to keep it. TERN_KEEP_ENGINE=1
// keeps it, and so do build tags of the app's own in GOFLAGS, which
// -tags would replace.
func withoutEngine(native bool) bool {
	if !native || !engineFree || os.Getenv("TERN_KEEP_ENGINE") == "1" {
		return false
	}
	if _, err := os.Stat(nativeFile); err != nil {
		return false
	}
	return !strings.Contains(os.Getenv("GOFLAGS"), "-tags")
}

// crossBuild reports whether GOOS or GOARCH name another system than this
// one, as for GOOS=linux tern build on a Mac.
func crossBuild() bool {
	goos, goarch := os.Getenv("GOOS"), os.Getenv("GOARCH")
	return goos != "" && goos != runtime.GOOS || goarch != "" && goarch != runtime.GOARCH
}

// buildRunHint is whether tern build ends by saying how to run the
// binary; tern deploy, which runs it elsewhere, turns it off.
var buildRunHint = true

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
		if strings.HasSuffix(p, ".br") {
			return nil
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

// prerenderList is what an app in prerender mode lists at /__tern/prerender.
type prerenderList struct {
	Paths     []string `json:"paths"`
	Endpoints []string `json:"endpoints"`
	Errors    []string `json:"errors"`
	Plugins   []string `json:"plugins"`
}

// prerenderServer is the built app running in prerender mode.
type prerenderServer struct {
	base string // origin and base path
	list prerenderList
	stop func()
}

// startPrerender runs bin in prerender mode and waits for its list.
func startPrerender(bin, basePath string) (*prerenderServer, error) {
	port, err := freePort()
	if err != nil {
		return nil, err
	}
	abs, err := filepath.Abs(bin)
	if err != nil {
		return nil, err
	}
	var logs strings.Builder
	cmd := exec.Command(abs)
	cmd.Env = append(os.Environ(), "TERN_PRERENDER=1", "HOST=127.0.0.1", "PORT="+strconv.Itoa(port))
	cmd.Stdout, cmd.Stderr = &logs, &logs
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	exited := make(chan struct{})
	go func() { _ = cmd.Wait(); close(exited) }()
	srv := &prerenderServer{
		base: "http://127.0.0.1:" + strconv.Itoa(port) + basePath,
		stop: func() {
			_ = cmd.Process.Kill()
			<-exited
		},
	}
	client := &http.Client{Timeout: 30 * time.Second}
	deadline := time.Now().Add(30 * time.Second)
	for {
		select {
		case <-exited:
			return nil, fmt.Errorf("the app exited while prerendering:\n%s", logs.String())
		default:
		}
		resp, err := client.Get(srv.base + "/__tern/prerender")
		if err == nil && resp.StatusCode == http.StatusFound {
			_ = resp.Body.Close()
			err = errors.New("redirected")
		}
		if err == nil {
			err = json.NewDecoder(resp.Body).Decode(&srv.list)
			_ = resp.Body.Close()
			if err != nil {
				srv.stop()
				return nil, fmt.Errorf("reading the prerender list: %w", err)
			}
			break
		}
		if time.Now().After(deadline) {
			srv.stop()
			return nil, fmt.Errorf("the app did not start for prerendering:\n%s", logs.String())
		}
		time.Sleep(50 * time.Millisecond)
	}
	if len(srv.list.Errors) > 0 {
		srv.stop()
		return nil, errors.New(strings.Join(srv.list.Errors, "\n"))
	}
	return srv, nil
}

// prerender runs the built app in prerender mode and saves the HTML and
// data of every page marked with `const Prerender = true` into the client
// output, so the next compile embeds them as static files.
func prerender(bin, basePath string) (int, error) {
	srv, err := startPrerender(bin, basePath)
	if err != nil {
		return 0, err
	}
	defer srv.stop()
	base, list := srv.base, srv.list
	client := &http.Client{Timeout: 30 * time.Second}

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
		if strings.HasSuffix(file, ".html") {
			// Prerendering renders with an empty nonce: the saved page
			// is allowed its scripts by their hashes instead.
			body = bytes.ReplaceAll(body, []byte(` nonce=""`), nil)
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

// precompress writes a .gz and a .br next to every compressible asset
// over 1 KiB, each only when it saves space. The server sends the one the
// client accepts, brotli first.
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
		var gz bytes.Buffer
		zw, err := gzip.NewWriterLevel(&gz, gzip.BestCompression)
		if err != nil {
			return err
		}
		if _, err := zw.Write(src); err != nil {
			return err
		}
		if err := zw.Close(); err != nil {
			return err
		}
		var br bytes.Buffer
		bw := brotli.NewWriterLevel(&br, brotli.BestCompression)
		if _, err := bw.Write(src); err != nil {
			return err
		}
		if err := bw.Close(); err != nil {
			return err
		}
		for _, c := range []struct {
			ext string
			b   []byte
		}{{".gz", gz.Bytes()}, {".br", br.Bytes()}} {
			if len(c.b) >= len(src) {
				_ = os.Remove(p + c.ext) // stale from an earlier build
				continue
			}
			if err := os.WriteFile(p+c.ext, c.b, 0o644); err != nil {
				return err
			}
		}
		return nil
	})
}

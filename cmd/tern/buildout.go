package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/Unfield/Tern/internal/codegen"
	"github.com/Unfield/Tern/internal/version"
)

// The build output for hosting platforms (ADR 0029): tern build -out <dir>
// writes the server, the static files and tern-output.json, which lists
// each static URL with the headers the app sends for it.

type ternOutput struct {
	Version   int                   `json:"version"`
	Framework outputFramework       `json:"framework"`
	Server    outputServer          `json:"server"`
	BasePath  string                `json:"base_path"`
	Immutable []string              `json:"immutable"`
	NotFound  *string               `json:"not_found"`
	Headers   map[string]string     `json:"headers"`
	Files     map[string]outputFile `json:"files"`
	Jobs      *outputJobs           `json:"jobs,omitempty"`
}

type outputFramework struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type outputServer struct {
	Path     string            `json:"path"`
	GOOS     string            `json:"goos"`
	GOARCH   string            `json:"goarch"`
	Env      map[string]string `json:"env"`
	Ready    *string           `json:"ready"`
	Shutdown outputShutdown    `json:"shutdown"`
}

type outputShutdown struct {
	Signal       string `json:"signal"`
	GraceSeconds int    `json:"grace_seconds"`
}

type outputFile struct {
	File      string            `json:"file"`
	Kind      string            `json:"kind"`
	Encodings []string          `json:"encodings"`
	Headers   map[string]string `json:"headers"`
}

type outputJobs struct {
	Next bool `json:"next"`
}

// perRequestHeaders differ from one response to the next, or describe the
// bytes sent rather than the file, so they aren't part of a file's headers.
var perRequestHeaders = []string{
	"Accept-Ranges", "Content-Encoding", "Content-Length", "Date", "Last-Modified",
	"Server-Timing", "Set-Cookie", "Traceparent", "Vary", "X-Request-Id",
}

// outputBinary is where the server goes in an output directory.
func outputBinary(dir string) string {
	name := "app"
	if goos := os.Getenv("GOOS"); goos == "windows" || goos == "" && runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(dir, "server", name)
}

// clearOutput removes what an earlier tern build -out wrote to dir, and
// nothing else.
func clearOutput(dir string) error {
	for _, name := range []string{"server", "static", "tern-output.json"} {
		if err := os.RemoveAll(filepath.Join(dir, name)); err != nil {
			return err
		}
	}
	return nil
}

// writeOutput copies the static files into dir and writes
// tern-output.json, with the headers hostBin sends for each file. hostBin
// is the app built for this system, run in prerender mode.
func writeOutput(dir, hostBin, binary, basePath string) (int, error) {
	srv, err := startPrerender(hostBin, basePath)
	if err != nil {
		return 0, err
	}
	defer srv.stop()

	kinds := map[string]string{} // by file name in the client output
	for _, p := range srv.list.Paths {
		d := strings.Trim(p, "/")
		kinds[path.Join(d, "index.html")] = "page"
		kinds[path.Join(d, "__data.json")] = "data"
	}
	for _, p := range srv.list.Endpoints {
		kinds[strings.TrimPrefix(p, "/")] = "endpoint"
	}

	out := ternOutput{
		Version:   1,
		Framework: outputFramework{Name: "tern", Version: strings.TrimPrefix(version.String(), "v")},
		BasePath:  basePath,
		Immutable: []string{basePath + "/_app/immutable/"},
		Headers:   map[string]string{},
		Files:     map[string]outputFile{},
	}
	rel, err := filepath.Rel(dir, binary)
	if err != nil {
		return 0, err
	}
	goos, goarch := os.Getenv("GOOS"), os.Getenv("GOARCH")
	if goos == "" {
		goos = runtime.GOOS
	}
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	out.Server = outputServer{
		Path: filepath.ToSlash(rel), GOOS: goos, GOARCH: goarch,
		Env:      map[string]string{"port": "PORT", "host": "HOST"},
		Shutdown: outputShutdown{Signal: "SIGTERM", GraceSeconds: 10},
	}
	if slices.Contains(srv.list.Plugins, "jobs") {
		out.Jobs = &outputJobs{Next: true}
	}

	client := &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	// plugins/health answers /readyz; in prerender mode its checks may
	// fail, but the route is there.
	if resp, err := client.Get(srv.base + "/readyz"); err == nil {
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			ready := basePath + "/readyz"
			out.Server.Ready = &ready
		}
	}

	src := filepath.FromSlash(codegen.ClientOut)
	static := filepath.Join(dir, "static")
	var names []string
	err = filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := filepath.ToSlash(strings.TrimPrefix(p, src+string(filepath.Separator)))
		if d.IsDir() {
			if name == ".vite" {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, ".gz") && !strings.HasSuffix(name, ".br") {
			names = append(names, name)
		}
		return copyFile(p, filepath.Join(static, filepath.FromSlash(name)))
	})
	if err != nil {
		return 0, err
	}

	for _, name := range names {
		u := basePath + "/" + name
		req, err := http.NewRequest(http.MethodGet, srv.base+"/"+name, nil)
		if err != nil {
			return 0, err
		}
		req.Header.Set("Accept-Encoding", "identity")
		resp, err := client.Do(req)
		if err != nil {
			return 0, err
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return 0, fmt.Errorf("the app answered %s with status %d", u, resp.StatusCode)
		}
		f := outputFile{File: name, Kind: kinds[name], Headers: map[string]string{}}
		if f.Kind == "" {
			f.Kind = "asset"
		}
		for k, v := range resp.Header {
			if !slices.Contains(perRequestHeaders, k) {
				f.Headers[k] = strings.Join(v, ", ")
			}
		}
		for _, enc := range []struct{ ext, name string }{{".br", "br"}, {".gz", "gzip"}} {
			if _, err := os.Stat(filepath.Join(src, filepath.FromSlash(name)+enc.ext)); err == nil {
				f.Encodings = append(f.Encodings, enc.name)
			}
		}
		if f.Encodings == nil {
			f.Encodings = []string{}
		}
		out.Files[u] = f
		// A page answers at its directory too, with and without the slash.
		if f.Kind == "page" {
			d := strings.TrimSuffix(strings.TrimSuffix(name, "index.html"), "/")
			if d == "" {
				out.Files[basePath+"/"] = f
				if basePath != "" {
					out.Files[basePath] = f
				}
			} else {
				out.Files[basePath+"/"+d] = f
				out.Files[basePath+"/"+d+"/"] = f
			}
		}
	}

	// Headers every file has go to the top, once.
	first := true
	for _, f := range out.Files {
		if first {
			for k, v := range f.Headers {
				out.Headers[k] = v
			}
			first = false
			continue
		}
		for k, v := range out.Headers {
			if f.Headers[k] != v {
				delete(out.Headers, k)
			}
		}
	}
	for u, f := range out.Files {
		h := map[string]string{}
		for k, v := range f.Headers {
			if _, shared := out.Headers[k]; !shared {
				h[k] = v
			}
		}
		f.Headers = h
		out.Files[u] = f
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(out); err != nil {
		return 0, err
	}
	return len(names), os.WriteFile(filepath.Join(dir, "tern-output.json"), buf.Bytes(), 0o644)
}

// copyFile copies src to dst, creating dst's directory.
func copyFile(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0o644)
}

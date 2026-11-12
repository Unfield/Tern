package main

import (
	_ "embed"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

// The dev overlay: a button on every page in tern dev that opens a panel
// with the page's route, files and data, and the request's spans and logs,
// and an overlay for errors, from the server or the browser, with their
// stacks and the source around each frame. Its script is served as part of
// /__tern/dev/client.js; it reads what it shows from /__tern/dev/api/.

//go:embed devoverlay.js
var devOverlayJS string

// forRequest returns the items of request id, with all the spans of its
// traces, and the newest item's seq, to stream from.
func (l *devLogs) forRequest(id string) (items []devLogItem, seq int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	traces := map[string]bool{}
	for _, it := range l.items {
		if it.RequestID == id && it.Span != nil {
			traces[it.Span.TraceID] = true
		}
	}
	items = []devLogItem{}
	for _, it := range l.items {
		if it.RequestID == id || it.Span != nil && traces[it.Span.TraceID] {
			items = append(items, it)
		}
	}
	return items, l.seq
}

// serveAPI answers /__tern/dev/api/….
func (d *devRunner) serveAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	// The API is for the app's own pages: a page on another site must not
	// read the app's logs, or open files.
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" && site != "none" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	switch strings.TrimPrefix(r.URL.Path, "/__tern/dev/api/") {
	case "request":
		items, seq := d.logs.forRequest(r.URL.Query().Get("id"))
		writeJSON(w, map[string]any{"seq": seq, "items": items})
	case "files":
		writeJSON(w, d.routeFiles(r.URL.Query()["c"]))
	case "source":
		line, _ := strconv.Atoi(r.URL.Query().Get("line"))
		src, err := d.source(r.URL.Query().Get("file"), line, r.URL.Query().Get("find"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		writeJSON(w, src)
	case "open":
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			File string `json:"file"`
			Line int    `json:"line"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		file, err := d.inProject(req.File)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		if err := openInEditor(file, req.Line); err != nil {
			http.Error(w, err.Error(), http.StatusNotImplemented)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.NotFound(w, r)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// inProject resolves file, absolute or relative to the project, and
// refuses anything outside the project.
func (d *devRunner) inProject(file string) (string, error) {
	if file == "" {
		return "", errors.New("no file")
	}
	if !filepath.IsAbs(file) {
		file = filepath.Join(d.root, filepath.FromSlash(file))
	}
	file, err := filepath.EvalSymlinks(file)
	if err != nil {
		return "", err
	}
	root, err := filepath.EvalSymlinks(d.root)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, file)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("not in the project")
	}
	return file, nil
}

// devSource is a few lines of a file around one.
type devSource struct {
	File  string   `json:"file"` // relative to the project
	Abs   string   `json:"abs"`
	Line  int      `json:"line"`
	Start int      `json:"start"`
	Lines []string `json:"lines"`
}

// source returns the lines around line of file. With find, such as an
// error's message, the line is the first after it that contains the text,
// if there is one: errors.New("database is down") rather than the start of
// the function.
func (d *devRunner) source(file string, line int, find string) (*devSource, error) {
	abs, err := d.inProject(file)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil || info.Size() > 4<<20 {
		return nil, errors.New("not a source file")
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(b), "\n")
	line = min(max(line, 1), len(lines))
	if find = strings.TrimSpace(find); len(find) >= 4 {
		for i := line - 1; i < len(lines); i++ {
			if strings.Contains(lines[i], find) {
				line = i + 1
				break
			}
		}
	}
	start := max(1, line-5)
	end := min(len(lines), line+5)
	rel, _ := filepath.Rel(d.root, abs)
	return &devSource{File: filepath.ToSlash(rel), Abs: abs, Line: line, Start: start, Lines: lines[start-1 : end]}, nil
}

// devRouteFile is a component of the route, and its Go load.
type devRouteFile struct {
	Component string `json:"component"`
	// Server is its page.server.go or layout.server.go, "" if it has none;
	// Line is where its load is, or 1.
	Server string `json:"server"`
	Line   int    `json:"line"`
}

var loadFunc = regexp.MustCompile(`(?m)^func (Load|LayoutLoad)\b`)

func (d *devRunner) routeFiles(components []string) []devRouteFile {
	out := []devRouteFile{}
	for _, c := range components {
		f := devRouteFile{Component: c}
		dir, base := filepath.Split(filepath.FromSlash(c))
		server := ""
		switch base {
		case "+page.svelte":
			server = "page.server.go"
		case "+layout.svelte":
			server = "layout.server.go"
		}
		if server != "" {
			if b, err := os.ReadFile(filepath.Join(d.root, dir, server)); err == nil {
				f.Server = filepath.ToSlash(filepath.Join(dir, server))
				f.Line = 1
				if loc := loadFunc.FindIndex(b); loc != nil {
					f.Line = strings.Count(string(b[:loc[0]]), "\n") + 1
				}
			}
		}
		out = append(out, f)
	}
	return out
}

// openInEditor opens file at line in TERN_EDITOR, or else the first of
// Zed, Cursor and VS Code that is installed.
func openInEditor(file string, line int) error {
	at := file
	if line > 0 {
		at += ":" + strconv.Itoa(line)
	}
	if ed := strings.TrimSpace(os.Getenv("TERN_EDITOR")); ed != "" {
		args := strings.Fields(ed)
		if strings.Contains(ed, "{file}") {
			for i, a := range args {
				args[i] = strings.NewReplacer("{file}", file, "{line}", strconv.Itoa(max(line, 1))).Replace(a)
			}
		} else {
			args = append(args, at)
		}
		return exec.Command(args[0], args[1:]...).Start()
	}
	for _, ed := range []struct {
		name string
		args []string
	}{
		{"zed", []string{at}},
		{"cursor", []string{"-g", at}},
		{"code", []string{"-g", at}},
	} {
		if p, err := exec.LookPath(ed.name); err == nil {
			return exec.Command(p, ed.args...).Start()
		}
	}
	if runtime.GOOS == "darwin" {
		for _, app := range []string{"Zed", "Cursor", "Visual Studio Code"} {
			if _, err := os.Stat("/Applications/" + app + ".app"); err == nil {
				// Without its command-line tool, the app opens the file, at its start.
				return exec.Command("open", "-a", app, file).Start()
			}
		}
	}
	return errors.New("no editor found: set TERN_EDITOR, such as TERN_EDITOR=zed or TERN_EDITOR=\"code -g\"")
}

// afterSeq is the stream's starting point from its ?after= query.
func afterSeq(q url.Values) int64 {
	n, _ := strconv.ParseInt(q.Get("after"), 10, 64)
	return n
}

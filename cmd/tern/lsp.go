package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Unfield/Tern/internal/codegen"
	"github.com/Unfield/Tern/internal/routes"
	"github.com/Unfield/Tern/internal/version"
)

// tern lsp is a language server for editors, over stdio, next to gopls and
// Svelte's: it jumps between a page and its Go load, lists the routes as
// workspace symbols, and shows tern check's errors when a file is saved.
// The VS Code and Zed extensions in editors/ start it.

func cmdLSP(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	for _, a := range args {
		if a == "-h" || a == "--help" {
			fmt.Fprintln(stdout, "usage: tern lsp\n\nRuns Tern's language server on stdin and stdout, for editors.")
			return 0
		}
	}
	s := &lspServer{out: stdout, log: stderr, files: map[string]bool{}}
	if err := s.serve(stdin); err != nil && !errors.Is(err, io.EOF) {
		fmt.Fprintln(stderr, "tern lsp:", err)
		return 1
	}
	return 0
}

type lspServer struct {
	out io.Writer
	log io.Writer
	mu  sync.Mutex // writes

	root string

	checkMu  sync.Mutex
	checking bool
	again    bool
	files    map[string]bool // files with diagnostics, to clear them
	runCheck func(root string) (map[string][]lspDiagnostic, error)
	debounce time.Duration
	timer    *time.Timer
}

type lspMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *lspError       `json:"error,omitempty"`
}

type lspError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (s *lspServer) serve(in io.Reader) error {
	r := bufio.NewReader(in)
	for {
		length := 0
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return err
			}
			line = strings.TrimRight(line, "\r\n")
			if line == "" {
				break
			}
			if v, ok := strings.CutPrefix(line, "Content-Length:"); ok {
				length, _ = strconv.Atoi(strings.TrimSpace(v))
			}
		}
		if length <= 0 || length > 64<<20 {
			return fmt.Errorf("bad Content-Length %d", length)
		}
		body := make([]byte, length)
		if _, err := io.ReadFull(r, body); err != nil {
			return err
		}
		var m lspMessage
		if err := json.Unmarshal(body, &m); err != nil {
			continue
		}
		if m.Method == "exit" {
			return nil
		}
		s.handle(m)
	}
}

func (s *lspServer) send(m lspMessage) {
	m.JSONRPC = "2.0"
	b, _ := json.Marshal(m)
	s.mu.Lock()
	defer s.mu.Unlock()
	fmt.Fprintf(s.out, "Content-Length: %d\r\n\r\n", len(b))
	_, _ = s.out.Write(b)
}

func (s *lspServer) reply(id json.RawMessage, result any) {
	if result == nil {
		// null, not a missing result
		s.send(lspMessage{ID: id, Result: json.RawMessage("null")})
		return
	}
	s.send(lspMessage{ID: id, Result: result})
}

func (s *lspServer) notify(method string, params any) {
	b, _ := json.Marshal(params)
	s.send(lspMessage{Method: method, Params: b})
}

func (s *lspServer) logf(format string, args ...any) {
	s.notify("window/logMessage", map[string]any{"type": 3, "message": "tern: " + fmt.Sprintf(format, args...)})
}

func (s *lspServer) handle(m lspMessage) {
	switch m.Method {
	case "initialize":
		var p struct {
			RootURI          string `json:"rootUri"`
			RootPath         string `json:"rootPath"`
			WorkspaceFolders []struct {
				URI string `json:"uri"`
			} `json:"workspaceFolders"`
		}
		_ = json.Unmarshal(m.Params, &p)
		dir := uriPath(p.RootURI)
		if dir == "" && len(p.WorkspaceFolders) > 0 {
			dir = uriPath(p.WorkspaceFolders[0].URI)
		}
		if dir == "" {
			dir = p.RootPath
		}
		s.root = projectRoot(dir)
		s.reply(m.ID, map[string]any{
			"capabilities": map[string]any{
				"textDocumentSync":        map[string]any{"openClose": true, "change": 0, "save": map[string]any{"includeText": false}},
				"definitionProvider":      true,
				"workspaceSymbolProvider": true,
				"codeLensProvider":        map[string]any{"resolveProvider": false},
			},
			"serverInfo": map[string]any{"name": "tern", "version": version.String()},
		})
	case "initialized":
		if s.root != "" {
			s.scheduleCheck()
		}
	case "shutdown":
		s.reply(m.ID, nil)
	case "textDocument/didSave":
		var p struct {
			TextDocument struct {
				URI string `json:"uri"`
			} `json:"textDocument"`
		}
		_ = json.Unmarshal(m.Params, &p)
		switch filepath.Ext(uriPath(p.TextDocument.URI)) {
		case ".go", ".svelte", ".ts", ".js", ".json", ".mod":
			s.scheduleCheck()
		}
	case "workspace/didChangeWatchedFiles":
		// Files changed outside the editor, such as by git or a formatter.
		var p struct {
			Changes []struct {
				URI string `json:"uri"`
			} `json:"changes"`
		}
		_ = json.Unmarshal(m.Params, &p)
		for _, c := range p.Changes {
			f := uriPath(c.URI)
			if strings.Contains(filepath.ToSlash(f), "/"+codegen.GenDir+"/") || strings.Contains(filepath.ToSlash(f), "/node_modules/") || strings.HasSuffix(f, ".gen.go") {
				continue // what the check writes, and dependencies
			}
			switch filepath.Ext(f) {
			case ".go", ".svelte", ".ts", ".js", ".mod":
				s.scheduleCheck()
				return
			}
		}
	case "textDocument/definition":
		var p lspPositionParams
		_ = json.Unmarshal(m.Params, &p)
		s.reply(m.ID, s.definition(p))
	case "workspace/symbol":
		var p struct {
			Query string `json:"query"`
		}
		_ = json.Unmarshal(m.Params, &p)
		s.reply(m.ID, s.symbols(p.Query))
	case "textDocument/codeLens":
		var p struct {
			TextDocument struct {
				URI string `json:"uri"`
			} `json:"textDocument"`
		}
		_ = json.Unmarshal(m.Params, &p)
		s.reply(m.ID, s.codeLenses(uriPath(p.TextDocument.URI)))
	case "tern/routes":
		s.reply(m.ID, s.routeList())
	default:
		if len(m.ID) > 0 {
			s.send(lspMessage{ID: m.ID, Error: &lspError{Code: -32601, Message: "method not found: " + m.Method}})
		}
	}
}

func uriPath(uri string) string {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "file" {
		return ""
	}
	p := u.Path
	if runtime.GOOS == "windows" {
		p = strings.TrimPrefix(p, "/")
	}
	return filepath.FromSlash(p)
}

func pathURI(p string) string {
	p = filepath.ToSlash(p)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return (&url.URL{Scheme: "file", Path: p}).String()
}

// projectRoot is the nearest directory from dir up with a go.mod and
// src/routes, or "".
func projectRoot(dir string) string {
	for d := dir; d != "" && d != filepath.Dir(d); d = filepath.Dir(d) {
		if fileExists(filepath.Join(d, "go.mod")) && fileExists(filepath.Join(d, "src", "routes")) {
			return d
		}
	}
	return ""
}

type lspPosition struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type lspRange struct {
	Start lspPosition `json:"start"`
	End   lspPosition `json:"end"`
}

type lspLocation struct {
	URI   string   `json:"uri"`
	Range lspRange `json:"range"`
}

type lspPositionParams struct {
	TextDocument struct {
		URI string `json:"uri"`
	} `json:"textDocument"`
	Position lspPosition `json:"position"`
}

func at(line int) lspRange {
	return lspRange{Start: lspPosition{Line: line}, End: lspPosition{Line: line}}
}

func (s *lspServer) manifest() (*routes.Manifest, error) {
	if s.root == "" {
		return nil, errors.New("not in a Tern project")
	}
	return routes.Scan(os.DirFS(s.root), defaultRoutesDir)
}

// counterpart is the other half of a route file: a page's or layout's Go
// load for its Svelte component, and the component for its Go file.
type counterpart struct {
	file string
	line int // 0-based
}

// serverFile is the Go file of files, paths under dir, that declares fn,
// and its line.
func serverFile(dir string, files []string, fn string) (string, int) {
	re := regexp.MustCompile(`(?m)^func ` + fn + `\b`)
	for _, f := range files {
		p := filepath.Join(dir, filepath.FromSlash(f))
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if loc := re.FindIndex(b); loc != nil {
			return p, bytes.Count(b[:loc[0]], []byte("\n"))
		}
	}
	return "", 0
}

func (s *lspServer) counterpart(file string) (counterpart, bool) {
	m, err := s.manifest()
	if err != nil {
		return counterpart{}, false
	}
	routesDir := filepath.Join(s.root, filepath.FromSlash(defaultRoutesDir))
	rel, err := filepath.Rel(routesDir, file)
	if err != nil || strings.HasPrefix(rel, "..") {
		return counterpart{}, false
	}
	rel = filepath.ToSlash(rel)
	isGo := strings.HasSuffix(rel, ".go")
	// A directory with a page and a layout sends each Go file to its own half.
	pageLoad := false
	if isGo {
		f, _ := serverFile(routesDir, []string{rel}, "Load")
		pageLoad = f != ""
	}
	for _, r := range m.Routes {
		var goFiles []string
		if r.Go != nil {
			goFiles = r.Go.Files
		}
		switch {
		case r.Page != "" && rel == r.Page && r.HasLoad():
			if f, line := serverFile(routesDir, goFiles, "Load"); f != "" {
				return counterpart{f, line}, true
			}
		case r.Page != "" && isGo && pageLoad && slices.Contains(goFiles, rel):
			return counterpart{filepath.Join(routesDir, filepath.FromSlash(r.Page)), 0}, true
		}
		for _, l := range r.Layouts {
			switch {
			case l.Component != "" && rel == l.Component && l.Go != nil:
				if f, line := serverFile(routesDir, l.Go.Files, "LayoutLoad"); f != "" {
					return counterpart{f, line}, true
				}
			case l.Component != "" && l.Go != nil && isGo && !pageLoad && slices.Contains(l.Go.Files, rel):
				return counterpart{filepath.Join(routesDir, filepath.FromSlash(l.Component)), 0}, true
			}
		}
	}
	return counterpart{}, false
}

var (
	routeCallRe = regexp.MustCompile(`\broute\(\s*['"]([^'"]+)['"]`)
	identRe     = regexp.MustCompile(`[A-Za-z_$][\w$]*`)
)

// definition answers go to definition where Tern knows the answer, and
// null elsewhere, so the editor asks gopls and Svelte's server too:
//   - in a page or layout, on data, form, PageProps or LayoutProps: the Go load;
//   - in its Go file, on Load, LayoutLoad or Actions: the component;
//   - on the pattern in route('/blog/[slug]'): that route's page.
func (s *lspServer) definition(p lspPositionParams) any {
	file := uriPath(p.TextDocument.URI)
	b, err := os.ReadFile(file)
	if err != nil {
		return nil
	}
	lines := strings.Split(string(b), "\n")
	if p.Position.Line >= len(lines) {
		return nil
	}
	line := lines[p.Position.Line]
	col := min(p.Position.Character, len(line))
	for _, m := range routeCallRe.FindAllStringSubmatchIndex(line, -1) {
		if col >= m[2] && col <= m[3] {
			if loc, ok := s.routePage(line[m[2]:m[3]]); ok {
				return loc
			}
		}
	}
	word := ""
	for _, m := range identRe.FindAllStringIndex(line, -1) {
		if col >= m[0] && col <= m[1] {
			word = line[m[0]:m[1]]
		}
	}
	switch {
	case strings.HasSuffix(file, ".svelte") && slices.Contains([]string{"data", "form", "PageProps", "LayoutProps", "PageData", "LayoutData"}, word):
	case strings.HasSuffix(file, ".go") && slices.Contains([]string{"Load", "LayoutLoad", "Actions"}, word):
	default:
		return nil
	}
	c, ok := s.counterpart(file)
	if !ok {
		return nil
	}
	return lspLocation{URI: pathURI(c.file), Range: at(c.line)}
}

// routePage finds the page of a route pattern, such as /blog/[slug].
func (s *lspServer) routePage(pattern string) (lspLocation, bool) {
	m, err := s.manifest()
	if err != nil {
		return lspLocation{}, false
	}
	for _, r := range m.Routes {
		if r.Pattern() == pattern {
			file := r.Page
			if file == "" && r.Go != nil && len(r.Go.Files) > 0 {
				file = r.Go.Files[0]
			}
			if file != "" {
				return lspLocation{URI: pathURI(filepath.Join(s.root, filepath.FromSlash(defaultRoutesDir), filepath.FromSlash(file))), Range: at(0)}, true
			}
		}
	}
	return lspLocation{}, false
}

// lspRoute is a route for the VS Code extension's Routes view.
type lspRoute struct {
	Pattern string `json:"pattern"`
	Kind    string `json:"kind"`
	Page    string `json:"page,omitempty"`
	Server  string `json:"server,omitempty"`
	Line    int    `json:"line"`
}

func (s *lspServer) routeList() []lspRoute {
	m, err := s.manifest()
	if err != nil {
		return []lspRoute{}
	}
	routesDir := filepath.Join(s.root, filepath.FromSlash(defaultRoutesDir))
	list := slices.Clone(m.Routes)
	slices.SortFunc(list, func(a, b *routes.Route) int { return strings.Compare(a.Pattern(), b.Pattern()) })
	out := []lspRoute{}
	for _, r := range list {
		lr := lspRoute{Pattern: r.Pattern(), Kind: kind(r)}
		if r.Page != "" {
			lr.Page = pathURI(filepath.Join(routesDir, filepath.FromSlash(r.Page)))
		}
		if r.Go != nil {
			fn := "Load"
			if r.Page == "" && len(r.Go.Methods) > 0 {
				fn = r.Go.Methods[0]
			}
			if f, line := serverFile(routesDir, r.Go.Files, fn); f != "" {
				lr.Server, lr.Line = pathURI(f), line
			}
		}
		out = append(out, lr)
	}
	return out
}

func (s *lspServer) symbols(query string) []map[string]any {
	q := strings.ToLower(query)
	out := []map[string]any{}
	for _, r := range s.routeList() {
		if q != "" && !strings.Contains(strings.ToLower(r.Pattern), q) {
			continue
		}
		uri, line := r.Page, 0
		if uri == "" {
			uri, line = r.Server, r.Line
		}
		if uri == "" {
			continue
		}
		out = append(out, map[string]any{
			"name": r.Pattern, "kind": 1, // File
			"containerName": r.Kind + " route",
			"location":      lspLocation{URI: uri, Range: at(line)},
		})
	}
	return out
}

func (s *lspServer) codeLenses(file string) []map[string]any {
	c, ok := s.counterpart(file)
	if !ok {
		return []map[string]any{}
	}
	line := 0
	title := "Go: " + filepath.Base(c.file) + ":" + strconv.Itoa(c.line+1)
	if strings.HasSuffix(file, ".go") {
		title = "Svelte: " + filepath.Base(c.file)
		for _, fn := range []string{"Load", "LayoutLoad"} {
			if f, l := serverFile(filepath.Dir(file), []string{filepath.Base(file)}, fn); f != "" && strings.HasSuffix(c.file, map[string]string{"Load": "+page.svelte", "LayoutLoad": "+layout.svelte"}[fn]) {
				line = l
				break
			}
		}
	}
	return []map[string]any{{
		"range":   at(line),
		"command": map[string]any{"title": title, "command": "tern.open", "arguments": []any{pathURI(c.file), c.line}},
	}}
}

// Diagnostics: tern check when a file is saved.

type lspDiagnostic struct {
	Range    lspRange `json:"range"`
	Severity int      `json:"severity"` // 1 error, 2 warning
	Source   string   `json:"source"`
	Message  string   `json:"message"`
}

func (s *lspServer) scheduleCheck() {
	s.checkMu.Lock()
	defer s.checkMu.Unlock()
	if s.timer != nil {
		s.timer.Stop()
	}
	d := s.debounce
	if d == 0 {
		d = 300 * time.Millisecond
	}
	s.timer = time.AfterFunc(d, s.check)
}

func (s *lspServer) check() {
	s.checkMu.Lock()
	if s.checking {
		s.again = true
		s.checkMu.Unlock()
		return
	}
	s.checking = true
	s.checkMu.Unlock()
	for {
		run := s.runCheck
		if run == nil {
			run = ternCheck
		}
		diags, err := run(s.root)
		if err != nil {
			s.logf("%v", err)
		} else {
			s.publish(diags)
		}
		s.checkMu.Lock()
		if !s.again {
			s.checking = false
			s.checkMu.Unlock()
			return
		}
		s.again = false
		s.checkMu.Unlock()
	}
}

func (s *lspServer) publish(diags map[string][]lspDiagnostic) {
	s.checkMu.Lock()
	clear := []string{}
	for f := range s.files {
		if _, ok := diags[f]; !ok {
			clear = append(clear, f)
		}
	}
	s.files = map[string]bool{}
	for f := range diags {
		s.files[f] = true
	}
	s.checkMu.Unlock()
	for _, f := range clear {
		s.notify("textDocument/publishDiagnostics", map[string]any{"uri": pathURI(f), "diagnostics": []lspDiagnostic{}})
	}
	for f, d := range diags {
		s.notify("textDocument/publishDiagnostics", map[string]any{"uri": pathURI(f), "diagnostics": d})
	}
}

var (
	// file:line:col: message, as Go and tern gen report errors.
	goErrorRe = regexp.MustCompile(`(?m)^(\S+?\.(?:go|svelte|ts|js)):(\d+)(?::(\d+))?: (.+)$`)
	// svelte-check --output machine: <time> ERROR "file" line:col "message"
	svelteErrorRe = regexp.MustCompile(`(?m)^\d+ (ERROR|WARNING) "((?:[^"\\]|\\.)*)" (\d+):(\d+) ("(?:[^"\\]|\\.)*")`)
)

// ternCheck runs code generation and svelte-check in root, and returns the
// problems by file.
func ternCheck(root string) (map[string][]lspDiagnostic, error) {
	out := map[string][]lspDiagnostic{}
	if _, err := codegen.Generate(root); err != nil {
		parseGoErrors(root, err.Error(), out)
		if len(out) == 0 {
			return nil, err
		}
		return out, nil // svelte-check needs the generated types
	}
	bin, err := svelteCheckBin(root)
	if err != nil {
		return out, nil // no svelte-check: Svelte's own server still checks the markup
	}
	shadow, err := buildCheckTree(root)
	if err != nil {
		return nil, err
	}
	argv := []string{"--output", "machine", "--tsconfig", "./tsconfig.json"}
	if shadow {
		dir := filepath.Join(root, filepath.FromSlash(checkDir))
		argv = []string{"--output", "machine", "--workspace", dir, "--tsconfig", filepath.Join(dir, "tsconfig.json")}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, argv...)
	cmd.Dir = root
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	_ = cmd.Run() // it exits 1 when there are errors
	parseSvelteCheck(root, buf.String(), out)
	return out, nil
}

func parseGoErrors(root, text string, out map[string][]lspDiagnostic) {
	for _, m := range goErrorRe.FindAllStringSubmatch(text, -1) {
		file := m[1]
		if !filepath.IsAbs(file) {
			file = filepath.Join(root, filepath.FromSlash(file))
		}
		line, _ := strconv.Atoi(m[2])
		col, _ := strconv.Atoi(m[3])
		pos := lspPosition{Line: max(line-1, 0), Character: max(col-1, 0)}
		out[file] = append(out[file], lspDiagnostic{Range: lspRange{pos, pos}, Severity: 1, Source: "tern", Message: m[4]})
	}
}

func parseSvelteCheck(root, text string, out map[string][]lspDiagnostic) {
	shadow := filepath.ToSlash(checkDir) + "/"
	for _, m := range svelteErrorRe.FindAllStringSubmatch(text, -1) {
		var file, msg string
		_ = json.Unmarshal([]byte(`"`+m[2]+`"`), &file)
		_ = json.Unmarshal([]byte(m[5]), &msg)
		// Checked copies of Go components point back at the real files.
		file = filepath.ToSlash(file)
		if i := strings.Index(file, shadow); i >= 0 {
			file = file[:i] + file[i+len(shadow):]
		}
		if !filepath.IsAbs(file) {
			file = filepath.Join(root, filepath.FromSlash(file))
		}
		file = filepath.Clean(filepath.FromSlash(file))
		line, _ := strconv.Atoi(m[3])
		col, _ := strconv.Atoi(m[4])
		sev := 1
		if m[1] == "WARNING" {
			sev = 2
		}
		pos := lspPosition{Line: max(line-1, 0), Character: max(col-1, 0)}
		out[file] = append(out[file], lspDiagnostic{Range: lspRange{pos, pos}, Severity: sev, Source: "tern check", Message: msg})
	}
}

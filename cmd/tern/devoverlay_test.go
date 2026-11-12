package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Unfield/Tern/report"
	"github.com/Unfield/Tern/tracing"
)

func overlayRunner(t *testing.T) *devRunner {
	t.Helper()
	root := t.TempDir()
	write := func(rel, s string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("src/routes/+layout.svelte", "<slot />")
	write("src/routes/layout.server.go", "package routes\n\nimport \"github.com/Unfield/Tern/kit\"\n\nfunc LayoutLoad(c *kit.Ctx) (int, error) { return 0, nil }\n")
	write("src/routes/items/+page.svelte", "<p>items</p>")
	write("src/routes/items/page.server.go", "package items\n\ntype Data struct{}\n\n// Load loads.\nfunc Load(c *kit.Ctx) (Data, error) {\n\tif bad {\n\t\treturn Data{}, errors.New(\"database is down\")\n\t}\n\treturn Data{}, nil\n}\n")
	write("src/routes/about/+page.svelte", "<p>about</p>")
	return &devRunner{root: root, logs: newDevLogs()}
}

func apiGet(t *testing.T, d *devRunner, target string, headers ...string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	rec := httptest.NewRecorder()
	d.ServeHTTP(rec, req)
	return rec
}

func TestOverlayRouteFiles(t *testing.T) {
	d := overlayRunner(t)
	rec := apiGet(t, d, "/__tern/dev/api/files?c=src/routes/%2Blayout.svelte&c=src/routes/items/%2Bpage.svelte&c=src/routes/about/%2Bpage.svelte")
	var files []devRouteFile
	if err := json.Unmarshal(rec.Body.Bytes(), &files); err != nil {
		t.Fatal(err, rec.Body)
	}
	want := []devRouteFile{
		{Component: "src/routes/+layout.svelte", Server: "src/routes/layout.server.go", Line: 5},
		{Component: "src/routes/items/+page.svelte", Server: "src/routes/items/page.server.go", Line: 6},
		{Component: "src/routes/about/+page.svelte"},
	}
	if len(files) != len(want) {
		t.Fatalf("files = %+v", files)
	}
	for i := range want {
		if files[i] != want[i] {
			t.Errorf("files[%d] = %+v, want %+v", i, files[i], want[i])
		}
	}
}

func TestOverlaySource(t *testing.T) {
	d := overlayRunner(t)
	src, err := d.source("src/routes/items/page.server.go", 6, "")
	if err != nil || src.Line != 6 || src.Start != 1 || src.Lines[5] != "func Load(c *kit.Ctx) (Data, error) {" {
		t.Fatalf("source = %+v, %v", src, err)
	}
	// An error's message finds the line that returns it.
	if src, _ := d.source("src/routes/items/page.server.go", 6, "database is down"); src.Line != 8 {
		t.Errorf("find: line %d, want 8", src.Line)
	}
	for _, f := range []string{"../outside.go", "/etc/passwd", "", filepath.Join(d.root, "..", "x")} {
		if _, err := d.source(f, 1, ""); err == nil {
			t.Errorf("%q: read outside the project", f)
		}
	}
	if rec := apiGet(t, d, "/__tern/dev/api/source?file=../../etc/passwd&line=1"); rec.Code != http.StatusNotFound {
		t.Errorf("API outside the project: %d", rec.Code)
	}
}

func TestOverlayRequestItems(t *testing.T) {
	d := overlayRunner(t)
	now := time.Now()
	d.logs.add(devLogItem{Kind: "request", Time: now, Msg: "GET /items", RequestID: "r1", Status: 500})
	d.logs.add(devLogItem{Kind: "log", Time: now, Msg: "loading", RequestID: "r1"})
	d.logs.add(devLogItem{Kind: "log", Time: now, Msg: "other", RequestID: "r2"})
	// A child span without the request's ID, in its trace.
	d.logs.add(devLogItem{Kind: "span", Time: now, Msg: "load page", Span: &tracing.DevSpan{TraceID: "t1", SpanID: "s2", Parent: "s1", Name: "load page"}})
	d.logs.add(devLogItem{Kind: "span", Time: now, Msg: "GET /items", RequestID: "r1", Span: &tracing.DevSpan{TraceID: "t1", SpanID: "s1", Name: "GET /items", RequestID: "r1"}})
	d.logs.add(devLogItem{Kind: "report", Time: now, Msg: "database is down", RequestID: "r1", Report: &report.Report{ID: "x", RequestID: "r1", Message: "database is down"}})

	rec := apiGet(t, d, "/__tern/dev/api/request?id=r1", "Sec-Fetch-Site", "same-origin")
	var got struct {
		Seq   int64        `json:"seq"`
		Items []devLogItem `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	var msgs []string
	for _, it := range got.Items {
		msgs = append(msgs, it.Kind+":"+it.Msg)
	}
	if got.Seq != 6 || strings.Join(msgs, ",") != "request:GET /items,log:loading,span:load page,span:GET /items,report:database is down" {
		t.Errorf("seq %d, items %v", got.Seq, msgs)
	}

	// Another site's page can't read the app's logs.
	if rec := apiGet(t, d, "/__tern/dev/api/request?id=r1", "Sec-Fetch-Site", "cross-site"); rec.Code != http.StatusForbidden {
		t.Errorf("cross-site: %d", rec.Code)
	}
}

func TestOverlayOpenInEditor(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script as the editor")
	}
	d := overlayRunner(t)
	out := filepath.Join(t.TempDir(), "args")
	editor := filepath.Join(t.TempDir(), "editor")
	if err := os.WriteFile(editor, []byte("#!/bin/sh\necho \"$@\" > "+out+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	post := func(body string, headers ...string) int {
		req := httptest.NewRequest(http.MethodPost, "/__tern/dev/api/open", strings.NewReader(body))
		for i := 0; i+1 < len(headers); i += 2 {
			req.Header.Set(headers[i], headers[i+1])
		}
		rec := httptest.NewRecorder()
		d.ServeHTTP(rec, req)
		return rec.Code
	}
	t.Setenv("TERN_EDITOR", editor+" --goto {file}:{line}")
	if code := post(`{"file":"src/routes/items/page.server.go","line":8}`, "Sec-Fetch-Site", "same-origin"); code != http.StatusNoContent {
		t.Fatalf("open: %d", code)
	}
	var args []byte
	for range 100 {
		if args, _ = os.ReadFile(out); len(args) > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !strings.HasPrefix(string(args), "--goto ") || !strings.HasSuffix(strings.TrimSpace(string(args)), "src/routes/items/page.server.go:8") {
		t.Errorf("editor got %q", args)
	}
	if code := post(`{"file":"../../etc/passwd","line":1}`); code != http.StatusNotFound {
		t.Errorf("outside the project: %d", code)
	}
	if code := post(`{"file":"src/routes/items/page.server.go","line":1}`, "Sec-Fetch-Site", "cross-site"); code != http.StatusForbidden {
		t.Errorf("cross-site: %d", code)
	}
	if rec := apiGet(t, d, "/__tern/dev/api/open"); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET: %d", rec.Code)
	}
}

func TestOverlayScriptIsServed(t *testing.T) {
	d := overlayRunner(t)
	rec := apiGet(t, d, "/__tern/dev/client.js")
	if !strings.Contains(rec.Body.String(), "EventSource('/__tern/dev/events')") || !strings.Contains(rec.Body.String(), "tern-dev-overlay") {
		t.Error("client.js lacks the reloader or the overlay")
	}
}

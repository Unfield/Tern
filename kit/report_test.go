package kit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Unfield/Tern/internal/batch"
	"github.com/Unfield/Tern/report"
)

type reportMemory struct {
	mu      sync.Mutex
	reports []report.Report
}

func (m *reportMemory) Send(_ context.Context, r []report.Report) error {
	m.mu.Lock()
	m.reports = append(m.reports, r...)
	m.mu.Unlock()
	return nil
}

func TestReports(t *testing.T) {
	mem := &reportMemory{}
	rep := report.New(report.Options{Sinks: []report.Sink{mem}, Batch: batch.Options{Interval: time.Millisecond}})
	a, err := New(testManifest(), WithReporter(rep))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = a.Close() }()
	do := func(method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if body != "" {
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Sec-Fetch-Site", "same-origin")
		}
		w := httptest.NewRecorder()
		a.ServeHTTP(w, r)
		return w
	}
	do("GET", "/blog/broken", "")
	do("GET", "/blog/panic", "")
	do("GET", "/blog/missing", "") // a 404: not reported
	if w := do("GET", "/blog/hello", ""); !strings.Contains(w.Body.String(), `"errors":true`) {
		t.Error("the page doesn't ask for browser errors")
	}
	if w := do("POST", BrowserErrorsPath, `{"name":"TypeError","message":"x is undefined","stack":"TypeError: x is undefined\n    at onClick (http://localhost/src/routes/+page.svelte:12:9)\n    at http://localhost/node_modules/.vite/deps/svelte.js:100:5","url":"http://localhost/blog?token=s","route":"/blog"}`); w.Code != http.StatusNoContent {
		t.Fatalf("browser report: %d %s", w.Code, w.Body)
	}
	if err := rep.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	mem.mu.Lock()
	defer mem.mu.Unlock()
	if len(mem.reports) != 3 {
		t.Fatalf("%d reports: %+v", len(mem.reports), mem.reports)
	}
	var load, panicked, browser report.Report
	for _, r := range mem.reports {
		switch {
		case r.Source == "browser":
			browser = r
		case r.Panic:
			panicked = r
		default:
			load = r
		}
	}
	if load.Route != "/blog/slug.param" || !strings.Contains(load.Message, "database is down") || load.RequestID == "" || load.Method != http.MethodGet {
		t.Errorf("load error %+v", load)
	}
	if panicked.Message != "boom" || len(panicked.Stack) == 0 || !strings.Contains(panicked.Stack[0].Function, "testManifest") {
		t.Errorf("panic %+v", panicked)
	}
	if browser.Type != "TypeError" || len(browser.Stack) != 2 || !browser.Stack[0].InApp || browser.Stack[1].InApp || strings.Contains(browser.URL, "token=s") {
		t.Errorf("browser %+v", browser)
	}

	// At most 20 a minute from one address.
	codes := map[int]int{}
	for range 25 {
		codes[do("POST", BrowserErrorsPath, `{"message":"m"}`).Code]++
	}
	if codes[http.StatusTooManyRequests] != 6 { // the first report above counted too
		t.Errorf("rate limit %v", codes)
	}
}

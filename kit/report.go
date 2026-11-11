package kit

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Unfield/Tern/logging"
	"github.com/Unfield/Tern/report"
)

// WithReporter sends the App's unexpected errors and panics, and uncaught
// errors in its pages' browsers, to r, grouped by cause. Without it, the
// App uses report.Default(), and under tern dev a reporter that only feeds
// the dev server's log view.
func WithReporter(r *report.Reporter) Option {
	return func(o *options) { o.reporter = r }
}

var (
	devReporterOnce sync.Once
	devReporter     *report.Reporter
)

func (a *App) reporter() *report.Reporter {
	if a.opts.reporter != nil {
		return a.opts.reporter
	}
	if r := report.Default(); r != nil {
		return r
	}
	if a.dev != nil {
		devReporterOnce.Do(func() { devReporter = report.DevOnly() })
		return devReporter
	}
	return nil
}

// reportError reports an unexpected error of c's request.
func (a *App) reportError(c *Ctx, err error) {
	r := a.reporter()
	if r == nil {
		return
	}
	r.Report(c.Request.Context(), err, func(rep *report.Report) {
		rep.Method = c.Request.Method
		if u, err := url.Parse(absoluteURL(c.Request, a.m.BasePath)); err == nil {
			rep.URL = logging.RedactURL(u)
		}
		rep.Route = c.routeID
	})
}

// BrowserErrorsPath is where pages send their uncaught errors.
const BrowserErrorsPath = "/__tern/errors"

// browserError is what the client sends.
type browserError struct {
	Name    string `json:"name"`
	Message string `json:"message"`
	Stack   string `json:"stack"`
	URL     string `json:"url"`
	Route   string `json:"route"`
}

// browserErrors takes reports from pages: at most a few a minute per
// address, and small.
type browserErrors struct {
	mu   sync.Mutex
	seen map[string]*errBucket
}

type errBucket struct {
	start time.Time
	n     int
}

func (b *browserErrors) allow(ip string, now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.seen == nil || len(b.seen) > 10000 {
		b.seen = map[string]*errBucket{}
	}
	e := b.seen[ip]
	if e == nil || now.Sub(e.start) > time.Minute {
		e = &errBucket{start: now}
		b.seen[ip] = e
	}
	e.n++
	return e.n <= 20
}

func (a *App) serveBrowserError(w http.ResponseWriter, r *http.Request) {
	rep := a.reporter()
	if rep == nil || r.Method != http.MethodPost {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if !a.browserErrs.allow(ClientIP(r), time.Now()) {
		http.Error(w, "too many reports", http.StatusTooManyRequests)
		return
	}
	var be browserError
	if err := json.NewDecoder(io.LimitReader(r.Body, 32<<10)).Decode(&be); err != nil || be.Message == "" {
		http.Error(w, "bad report", http.StatusBadRequest)
		return
	}
	typ := be.Name
	if typ == "" {
		typ = "Error"
	}
	pageURL := be.URL
	if u, err := url.Parse(be.URL); err == nil {
		pageURL = logging.RedactURL(u)
	}
	rep.Send(report.Report{
		Source:    "browser",
		Type:      clip(typ, 100),
		Message:   clip(be.Message, 1000),
		Stack:     jsFrames(be.Stack),
		URL:       pageURL,
		Route:     clip(be.Route, 200),
		UserAgent: clip(r.UserAgent(), 300),
	})
	w.WriteHeader(http.StatusNoContent)
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// The two stack formats: V8's "at fn (file:line:col)" and Firefox's and
// Safari's "fn@file:line:col".
var (
	v8Frame    = regexp.MustCompile(`^\s*at (?:(.+?) \()?(.+?):(\d+):\d+\)?$`)
	geckoFrame = regexp.MustCompile(`^(.*?)@(.+?):(\d+):\d+$`)
)

// jsFrames reads a JavaScript stack, innermost first. The app's own
// modules are in app; dependencies' are not.
func jsFrames(stack string) []report.Frame {
	var out []report.Frame
	for _, line := range strings.Split(stack, "\n") {
		var fn, file, ln string
		if m := v8Frame.FindStringSubmatch(line); m != nil {
			fn, file, ln = m[1], m[2], m[3]
		} else if m := geckoFrame.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			fn, file, ln = m[1], m[2], m[3]
		} else {
			continue
		}
		n, _ := strconv.Atoi(ln)
		inApp := !strings.Contains(file, "/node_modules/") && !strings.Contains(file, "/@fs/") && !strings.Contains(file, "/.vite/deps/")
		out = append(out, report.Frame{Function: fn, File: file, Line: n, InApp: inApp})
		if len(out) == 50 {
			break
		}
	}
	return out
}

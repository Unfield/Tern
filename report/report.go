// Package report turns an app's unexpected errors and panics, on the
// server and in the browser, into reports grouped by cause, and sends them
// to an error tracker such as Sentry:
//
//	reporter := report.New(report.Options{Sinks: []report.Sink{sentry.New(sentry.Options{})}})
//	server, err := ternApp(kit.WithReporter(reporter))
//
// kit reports what it would log as an unexpected error: errors that loads,
// actions and endpoints return that aren't answers (NotFound, Fail,
// redirects), panics, and, with the client's help, uncaught errors in the
// browser. Each report has the error, its stack where there is one, the
// request, the route, the trace and the user, if a hook added one to the
// logging context (c.LogWith(slog.String("user", id))).
//
// Reports with the same cause share a group: the same error type and
// message, numbers and IDs aside, at the same route, or for panics, the
// same place in the code. Each group is sent a few times a minute at most,
// and counted. Under tern dev, the log view lists the groups.
package report

import (
	"cmp"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"runtime"
	"runtime/debug"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Unfield/Tern/internal/batch"
	"github.com/Unfield/Tern/logging"
)

// Report is one unexpected error.
type Report struct {
	ID   string    `json:"id"` // 32 hex digits
	Time time.Time `json:"time"`
	// Source is "server", "browser" or "job".
	Source string `json:"source"`
	// Type is the error's Go type, "panic", or the JavaScript error's
	// name; Message is its text.
	Type    string  `json:"type"`
	Message string  `json:"message"`
	Panic   bool    `json:"panic,omitempty"`
	Stack   []Frame `json:"stack,omitempty"` // innermost first
	// Group identifies the cause: reports with the same group are the
	// same problem.
	Group string `json:"group"`

	Method string `json:"method,omitempty"`
	URL    string `json:"url,omitempty"`
	Route  string `json:"route,omitempty"`
	// UserAgent is the browser's, for browser errors.
	UserAgent string `json:"userAgent,omitempty"`

	RequestID string `json:"requestId,omitempty"`
	TraceID   string `json:"traceId,omitempty"`
	SpanID    string `json:"spanId,omitempty"`
	User      string `json:"user,omitempty"`
	// Attrs are the other attributes of the request's logging context.
	Attrs map[string]any `json:"attrs,omitempty"`

	Release     string `json:"release,omitempty"`
	Environment string `json:"environment,omitempty"`
}

// Frame is a place in the code.
type Frame struct {
	Function string `json:"function"`
	File     string `json:"file"`
	Line     int    `json:"line"`
	// InApp is set for the app's own code, not Go's or a dependency's.
	InApp bool `json:"inApp,omitempty"`
}

// A Sink sends reports to an error tracker, in batches, from a goroutine.
type Sink interface {
	Send(ctx context.Context, reports []Report) error
}

// Options configures a reporter.
type Options struct {
	// Sinks get the reports.
	Sinks []Sink
	// Release names the build, such as a version or commit. The default is
	// TERN_RELEASE's, or the VCS revision Go stamped into the binary.
	Release string
	// Environment is "production", "staging"… The default is TERN_ENV's.
	Environment string
	// PerMinute is how many reports of one group are sent each minute;
	// the rest are counted. The default is 10.
	PerMinute int
	// Batch says how reports are batched and retried.
	Batch batch.Options
}

// Reporter groups reports and sends them. Make one with New.
type Reporter struct {
	o     Options
	ships []*batch.Shipper[Report]
	app   string // the main module's path, for InApp

	mu     sync.Mutex
	groups map[string]*Group
	seq    uint64 // counts reports, to order groups seen in the same clock tick
}

// Group is a cause and how often it happened.
type Group struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Message   string    `json:"message"`
	Route     string    `json:"route,omitempty"`
	Source    string    `json:"source"`
	Count     int       `json:"count"`
	FirstSeen time.Time `json:"firstSeen"`
	LastSeen  time.Time `json:"lastSeen"`
	// Last is the latest report.
	Last Report `json:"last"`

	windowStart time.Time
	sent        int
	seq         uint64
}

// New returns a reporter. Under tern dev, it also sends reports to the dev
// server's log view.
func New(o Options) *Reporter {
	if o.Release == "" {
		o.Release = os.Getenv("TERN_RELEASE")
	}
	if o.Release == "" {
		if bi, ok := debug.ReadBuildInfo(); ok {
			for _, s := range bi.Settings {
				if s.Key == "vcs.revision" && len(s.Value) >= 12 {
					o.Release = s.Value[:12]
				}
			}
		}
	}
	if o.Environment == "" {
		o.Environment = os.Getenv("TERN_ENV")
	}
	if o.PerMinute <= 0 {
		o.PerMinute = 10
	}
	r := &Reporter{o: o, groups: map[string]*Group{}}
	if bi, ok := debug.ReadBuildInfo(); ok {
		r.app = bi.Main.Path
	}
	for _, s := range o.Sinks {
		r.ships = append(r.ships, batch.New(batch.Config{Name: sinkName(s), Prefix: "report", What: "reports", ErrOut: os.Stderr}, s.Send, o.Batch))
	}
	if d := devShipper(); d != nil {
		r.ships = append(r.ships, d)
	}
	return r
}

func sinkName(s Sink) string {
	if n, ok := s.(interface{ Name() string }); ok {
		return n.Name()
	}
	return strings.TrimPrefix(reflect.TypeOf(s).String(), "*")
}

// Report records err as an unexpected error, with what ctx carries: the
// request, trace and user. fill can set the report's other fields.
func (r *Reporter) Report(ctx context.Context, err error, fill ...func(*Report)) {
	if r == nil || err == nil {
		return
	}
	rep := Report{Source: "server", Message: err.Error(), Type: errType(err)}
	var p interface{ PanicValue() (any, []uintptr) }
	if errors.As(err, &p) {
		v, pcs := p.PanicValue()
		rep.Panic, rep.Type, rep.Message = true, "panic", fmt.Sprint(v)
		rep.Stack = r.frames(pcs)
	} else if s, ok := stackOf(err); ok {
		rep.Stack = r.frames(s)
	}
	fromContext(ctx, &rep)
	for _, f := range fill {
		f(&rep)
	}
	r.Send(rep)
}

// Send groups and sends a report made elsewhere, such as from a browser.
func (r *Reporter) Send(rep Report) {
	if r == nil {
		return
	}
	if rep.ID == "" {
		var b [16]byte
		_, _ = rand.Read(b[:])
		rep.ID = hex.EncodeToString(b[:])
	}
	if rep.Time.IsZero() {
		rep.Time = time.Now()
	}
	if rep.Release == "" {
		rep.Release = r.o.Release
	}
	if rep.Environment == "" {
		rep.Environment = r.o.Environment
	}
	if rep.Group == "" {
		rep.Group = Fingerprint(rep)
	}
	if !r.count(rep) {
		return
	}
	for _, sh := range r.ships {
		sh.Add(rep)
	}
}

// count records rep in its group, and reports whether to send it.
func (r *Reporter) count(rep Report) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	g := r.groups[rep.Group]
	if g == nil {
		if len(r.groups) >= 1000 {
			r.evict()
		}
		g = &Group{ID: rep.Group, Type: rep.Type, Message: rep.Message, Route: rep.Route, Source: rep.Source, FirstSeen: rep.Time}
		r.groups[rep.Group] = g
	}
	g.Count++
	g.LastSeen, g.Last = rep.Time, rep
	r.seq++
	g.seq = r.seq
	if rep.Time.Sub(g.windowStart) >= time.Minute {
		g.windowStart, g.sent = rep.Time, 0
	}
	if g.sent >= r.o.PerMinute {
		return false
	}
	g.sent++
	return true
}

// evict forgets the group seen longest ago.
func (r *Reporter) evict() {
	var oldest *Group
	for _, g := range r.groups {
		if oldest == nil || g.seq < oldest.seq {
			oldest = g
		}
	}
	if oldest != nil {
		delete(r.groups, oldest.ID)
	}
}

// Groups returns the groups seen since the process started, most recent
// first.
func (r *Reporter) Groups() []Group {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	out := make([]Group, 0, len(r.groups))
	for _, g := range r.groups {
		out = append(out, *g)
	}
	r.mu.Unlock()
	// The order they were seen in, not their times: a coarse clock (on
	// Windows) gives reports moments apart the same time.
	slices.SortFunc(out, func(a, b Group) int { return cmp.Compare(b.seq, a.seq) })
	return out
}

// Flush waits until the reports so far have been sent, or ctx ends.
func (r *Reporter) Flush(ctx context.Context) error {
	if r == nil {
		return nil
	}
	var errs []error
	for _, sh := range r.ships {
		errs = append(errs, sh.Flush(ctx))
	}
	return errors.Join(errs...)
}

func (r *Reporter) frames(pcs []uintptr) []Frame {
	var out []Frame
	fs := runtime.CallersFrames(pcs)
	for {
		f, more := fs.Next()
		if f.Function != "" && !strings.HasPrefix(f.Function, "runtime.") {
			out = append(out, Frame{Function: f.Function, File: f.File, Line: f.Line, InApp: r.inApp(f.Function)})
		}
		if !more || len(out) >= 64 {
			break
		}
	}
	return out
}

func (r *Reporter) inApp(fn string) bool {
	if r.app != "" && r.app != "command-line-arguments" {
		return strings.HasPrefix(fn, r.app+"/") || strings.HasPrefix(fn, r.app+".")
	}
	// Without build information: not Go's own packages, and not Tern's.
	first, _, _ := strings.Cut(fn, "/")
	return strings.Contains(first, ".") && !strings.HasPrefix(fn, "github.com/Unfield/Tern/")
}

// fromContext fills in what the request's context carries.
func fromContext(ctx context.Context, rep *Report) {
	for _, a := range logging.Attrs(ctx) {
		v := a.Value.Resolve()
		switch a.Key {
		case logging.RequestIDKey:
			rep.RequestID = v.String()
		case logging.TraceIDKey:
			rep.TraceID = v.String()
		case logging.SpanIDKey:
			rep.SpanID = v.String()
		case "user", "user_id":
			rep.User = v.String()
		default:
			if rep.Attrs == nil {
				rep.Attrs = map[string]any{}
			}
			rep.Attrs[a.Key] = logging.ValueOf(v)
		}
	}
}

// errType is the Go type of the innermost wrapped error that isn't one of
// the standard wrappers.
func errType(err error) string {
	for {
		t := reflect.TypeOf(err).String()
		if t != "*fmt.wrapError" && t != "*fmt.wrapErrors" && t != "*errors.joinError" {
			return strings.TrimPrefix(t, "*")
		}
		u := errors.Unwrap(err)
		if u == nil {
			return strings.TrimPrefix(t, "*")
		}
		err = u
	}
}

var (
	uuidRe   = regexp.MustCompile(`\b[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}\b`)
	hexRe    = regexp.MustCompile(`\b0x[0-9a-fA-F]+\b|\b[0-9a-fA-F]{12,}\b`)
	numRe    = regexp.MustCompile(`\d+`)
	quotedRe = regexp.MustCompile(`"[^"]*"|'[^']*'`)
)

// Normalize is a message with what changes between occurrences of the
// same problem taken out: quoted values, UUIDs, hex and numbers.
func Normalize(msg string) string {
	msg = quotedRe.ReplaceAllString(msg, `"…"`)
	msg = uuidRe.ReplaceAllString(msg, "<id>")
	msg = hexRe.ReplaceAllString(msg, "<hex>")
	msg = numRe.ReplaceAllString(msg, "<n>")
	if len(msg) > 300 {
		msg = msg[:300]
	}
	return msg
}

// Fingerprint is a report's group: for a panic, the error and the
// innermost frames of the app's code; otherwise the type, the normalized
// message and the route.
func Fingerprint(rep Report) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%s\x00", rep.Source, rep.Type)
	if rep.Panic && len(rep.Stack) > 0 {
		n := 0
		for _, f := range rep.Stack {
			if f.InApp || n == 0 && !slices.ContainsFunc(rep.Stack, func(f Frame) bool { return f.InApp }) {
				fmt.Fprintf(h, "%s:%d\x00", f.Function, f.Line)
				if n++; n == 3 {
					break
				}
			}
		}
		fmt.Fprintf(h, "%s", Normalize(rep.Message))
	} else {
		fmt.Fprintf(h, "%s\x00%s", Normalize(rep.Message), rep.Route)
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// WithStack wraps err with the stack where WithStack is called, so its
// report shows where it came from. Errors without one are reported
// without a stack.
func WithStack(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := stackOf(err); ok {
		return err
	}
	pcs := make([]uintptr, 64)
	n := runtime.Callers(2, pcs)
	return &stacked{err: err, pcs: pcs[:n]}
}

// Errorf is fmt.Errorf with the stack, as WithStack adds it.
func Errorf(format string, args ...any) error {
	pcs := make([]uintptr, 64)
	n := runtime.Callers(2, pcs)
	return &stacked{err: fmt.Errorf(format, args...), pcs: pcs[:n]}
}

type stacked struct {
	err error
	pcs []uintptr
}

func (s *stacked) Error() string         { return s.err.Error() }
func (s *stacked) Unwrap() error         { return s.err }
func (s *stacked) StackTrace() []uintptr { return s.pcs }

func stackOf(err error) ([]uintptr, bool) {
	var s interface{ StackTrace() []uintptr }
	if errors.As(err, &s) {
		return s.StackTrace(), true
	}
	return nil, false
}

var defaultReporter struct {
	mu sync.RWMutex
	r  *Reporter
}

// SetDefault makes r the reporter for code outside requests, such as jobs.
func SetDefault(r *Reporter) {
	defaultReporter.mu.Lock()
	defaultReporter.r = r
	defaultReporter.mu.Unlock()
}

// Default returns the default reporter, or nil.
func Default() *Reporter {
	defaultReporter.mu.RLock()
	defer defaultReporter.mu.RUnlock()
	return defaultReporter.r
}

// Permanent marks a sink's error as one retrying won't fix.
func Permanent(err error) error { return batch.Permanent(err) }

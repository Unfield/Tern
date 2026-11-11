// Package tracing records what a request does as a tree of timed spans,
// and exports them to OpenTelemetry collectors and the services that take
// OTLP, such as Grafana Tempo, Honeycomb, Jaeger or Datadog:
//
//	tracer := tracing.New(tracing.Options{Exporters: []tracing.Exporter{otlp.New(otlp.Options{})}})
//	tracing.SetDefault(tracer)
//	server, err := ternApp(kit.WithTracer(tracer))
//
// kit starts a span for each request, and one for each load, action,
// endpoint and server render in it. Add your own with Start:
//
//	ctx, span := tracing.Start(ctx, "charge card", slog.String("order", id))
//	defer span.End()
//
// Spans join the logs: a record logged inside a span carries its trace_id
// and span_id. Trace context follows work out of the process through
// [Transport] for HTTP requests and through the jobs queue, and comes in
// with a W3C traceparent header from a trusted proxy or service. Under
// tern dev, the log view shows each request's spans as a waterfall.
//
// It is a small, dependency-free implementation of OpenTelemetry's tracing
// model, not its SDK: spans, events, status, parent-based sampling and
// W3C trace context.
package tracing

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"log/slog"
	mrand "math/rand/v2"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Unfield/Tern/internal/batch"
	"github.com/Unfield/Tern/logging"
)

// Kind is what a span does, as OpenTelemetry has it.
type Kind int

// The kinds, numbered as in OTLP.
const (
	Internal Kind = 1 // work inside the process: the default
	Server   Kind = 2 // handling a request
	Client   Kind = 3 // making a request: HTTP, SQL
	Producer Kind = 4 // queueing work: a job
	Consumer Kind = 5 // doing queued work
)

func (k Kind) String() string {
	switch k {
	case Server:
		return "server"
	case Client:
		return "client"
	case Producer:
		return "producer"
	case Consumer:
		return "consumer"
	}
	return "internal"
}

// TraceID identifies a trace, as W3C trace context has it.
type TraceID [16]byte

// SpanID identifies a span within a trace.
type SpanID [8]byte

func (t TraceID) String() string { return hex.EncodeToString(t[:]) }
func (s SpanID) String() string  { return hex.EncodeToString(s[:]) }

// IsValid reports whether the ID is set.
func (t TraceID) IsValid() bool { return t != TraceID{} }

// IsValid reports whether the ID is set.
func (s SpanID) IsValid() bool { return s != SpanID{} }

// SpanContext is what travels between processes: the trace, the span, and
// whether the trace is recorded.
type SpanContext struct {
	TraceID TraceID
	SpanID  SpanID
	Sampled bool
}

// IsValid reports whether both IDs are set.
func (sc SpanContext) IsValid() bool { return sc.TraceID.IsValid() && sc.SpanID.IsValid() }

// Traceparent is the W3C traceparent header for sc.
func (sc SpanContext) Traceparent() string {
	flags := "00"
	if sc.Sampled {
		flags = "01"
	}
	return "00-" + sc.TraceID.String() + "-" + sc.SpanID.String() + "-" + flags
}

// ParseTraceparent reads a W3C traceparent header.
func ParseTraceparent(s string) (SpanContext, bool) {
	parts := strings.Split(strings.TrimSpace(s), "-")
	if len(parts) < 4 || len(parts[0]) != 2 || parts[0] == "ff" || len(parts[1]) != 32 || len(parts[2]) != 16 || len(parts[3]) != 2 {
		return SpanContext{}, false
	}
	if parts[0] == "00" && len(parts) != 4 {
		return SpanContext{}, false
	}
	for _, p := range parts[:4] {
		if strings.ToLower(p) != p {
			return SpanContext{}, false
		}
	}
	var sc SpanContext
	if _, err := hex.Decode(sc.TraceID[:], []byte(parts[1])); err != nil {
		return SpanContext{}, false
	}
	if _, err := hex.Decode(sc.SpanID[:], []byte(parts[2])); err != nil {
		return SpanContext{}, false
	}
	flags, err := strconv.ParseUint(parts[3], 16, 8)
	if err != nil || !sc.IsValid() {
		return SpanContext{}, false
	}
	sc.Sampled = flags&1 == 1
	return sc, true
}

// StatusCode is a span's outcome.
type StatusCode int

// The codes, numbered as in OTLP.
const (
	Unset StatusCode = 0
	OK    StatusCode = 1
	Error StatusCode = 2
)

// Status is a span's outcome and, for errors, its message.
type Status struct {
	Code    StatusCode
	Message string
}

// Event is something that happened at a point in a span, such as an
// exception.
type Event struct {
	Name  string
	Time  time.Time
	Attrs []slog.Attr
}

// SpanData is a finished span, as exporters get it.
type SpanData struct {
	Context SpanContext
	Parent  SpanID // zero for a root span
	Name    string
	Kind    Kind
	Start   time.Time
	End     time.Time
	Attrs   []slog.Attr
	Events  []Event
	Status  Status
}

// Duration is how long the span took.
func (d SpanData) Duration() time.Duration { return d.End.Sub(d.Start) }

// Span is an operation being timed. A Span that isn't recorded (no
// tracer, or not sampled) still carries its IDs, so logs and outgoing
// requests keep the trace; its other methods do nothing. All methods are
// safe on a nil *Span.
type Span struct {
	t      *Tracer // nil: not recorded
	sc     SpanContext
	parent SpanID
	kind   Kind
	start  time.Time

	mu     sync.Mutex
	name   string
	attrs  []slog.Attr
	events []Event
	status Status
	ended  atomic.Bool
}

// Context returns the span's IDs.
func (s *Span) Context() SpanContext {
	if s == nil {
		return SpanContext{}
	}
	return s.sc
}

// IsRecording reports whether the span will be exported.
func (s *Span) IsRecording() bool { return s != nil && s.t != nil && !s.ended.Load() }

// SetName renames the span, as kit does once it knows the request's route.
func (s *Span) SetName(name string) {
	if !s.IsRecording() {
		return
	}
	s.mu.Lock()
	s.name = name
	s.mu.Unlock()
}

// SetAttrs adds attributes, replacing ones with the same key.
func (s *Span) SetAttrs(attrs ...slog.Attr) {
	if !s.IsRecording() {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
next:
	for _, a := range attrs {
		for i, b := range s.attrs {
			if b.Key == a.Key {
				s.attrs[i] = a
				continue next
			}
		}
		s.attrs = append(s.attrs, a)
	}
}

// AddEvent records something that happened now.
func (s *Span) AddEvent(name string, attrs ...slog.Attr) {
	if !s.IsRecording() {
		return
	}
	s.mu.Lock()
	s.events = append(s.events, Event{Name: name, Time: time.Now(), Attrs: attrs})
	s.mu.Unlock()
}

// RecordError records err as an exception event and marks the span
// failed. A nil err does nothing.
func (s *Span) RecordError(err error) {
	if err == nil || !s.IsRecording() {
		return
	}
	s.AddEvent("exception", slog.String("exception.type", errType(err)), slog.String("exception.message", err.Error()))
	s.SetStatus(Error, err.Error())
}

func errType(err error) string {
	for {
		u := errors.Unwrap(err)
		if u == nil {
			break
		}
		err = u
	}
	t := strings.TrimPrefix(typeName(err), "*")
	return t
}

// SetStatus sets the span's outcome. An Error is kept over a later OK.
func (s *Span) SetStatus(code StatusCode, message string) {
	if !s.IsRecording() {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.status.Code == Error && code != Error {
		return
	}
	s.status = Status{Code: code, Message: message}
}

// End finishes the span and hands it to the exporters. Calls after the
// first do nothing.
func (s *Span) End() {
	if s == nil || s.t == nil || s.ended.Swap(true) {
		return
	}
	end := time.Now()
	s.mu.Lock()
	d := SpanData{Context: s.sc, Parent: s.parent, Name: s.name, Kind: s.kind, Start: s.start, End: end, Attrs: s.attrs, Events: s.events, Status: s.status}
	s.mu.Unlock()
	s.t.export(d)
}

type spanKey struct{}

// FromContext returns the span ctx carries, or nil.
func FromContext(ctx context.Context) *Span {
	if ctx == nil {
		return nil
	}
	s, _ := ctx.Value(spanKey{}).(*Span)
	return s
}

// ContextWith returns ctx carrying span, and its IDs for the logs.
func ContextWith(ctx context.Context, span *Span) context.Context {
	if span == nil {
		return ctx
	}
	ctx = context.WithValue(ctx, spanKey{}, span)
	if span.sc.IsValid() {
		ctx = logging.With(ctx, slog.String(logging.TraceIDKey, span.sc.TraceID.String()), slog.String(logging.SpanIDKey, span.sc.SpanID.String()))
	}
	return ctx
}

// StartOption configures a span started with Start.
type StartOption func(*startConfig)

type startConfig struct {
	kind   Kind
	attrs  []slog.Attr
	remote *SpanContext
	root   bool
	tracer *Tracer
}

// WithKind sets the span's kind. The default is Internal.
func WithKind(k Kind) StartOption { return func(c *startConfig) { c.kind = k } }

// WithAttrs sets attributes at the start.
func WithAttrs(attrs ...slog.Attr) StartOption {
	return func(c *startConfig) { c.attrs = append(c.attrs, attrs...) }
}

// WithRemoteParent makes the span a child of a span in another process,
// from a traceparent header or a queued job.
func WithRemoteParent(sc SpanContext) StartOption {
	return func(c *startConfig) {
		if sc.IsValid() {
			c.remote = &sc
		}
	}
}

// AsRoot starts a new trace, ignoring any span in the context.
func AsRoot() StartOption { return func(c *startConfig) { c.root = true } }

// WithTracer records the span with t rather than the parent's tracer or
// the default.
func WithTracer(t *Tracer) StartOption { return func(c *startConfig) { c.tracer = t } }

// Start starts a span as a child of the span in ctx (or a remote parent),
// and returns ctx carrying it. The span is recorded by its parent's tracer,
// or the default one. End it when the work is done. With no tracer and no
// trace to continue, it returns ctx and a nil span, whose methods do
// nothing.
func Start(ctx context.Context, name string, opts ...StartOption) (context.Context, *Span) {
	var c startConfig
	c.kind = Internal
	for _, o := range opts {
		o(&c)
	}
	parent := FromContext(ctx)
	t := c.tracer
	if t == nil && parent != nil && !c.root {
		t = parent.t
	}
	if t == nil {
		t = Default()
	}
	if t == nil && c.remote == nil && (parent == nil || c.root) {
		return ctx, nil // nothing to record, no trace to continue
	}
	s := &Span{name: name, kind: c.kind, start: time.Now(), attrs: c.attrs}
	switch {
	case c.remote != nil:
		s.sc.TraceID, s.parent, s.sc.Sampled = c.remote.TraceID, c.remote.SpanID, c.remote.Sampled
	case parent != nil && !c.root && parent.sc.IsValid():
		s.sc.TraceID, s.parent, s.sc.Sampled = parent.sc.TraceID, parent.sc.SpanID, parent.sc.Sampled
	default:
		s.sc.TraceID = newTraceID()
		s.sc.Sampled = t != nil && t.sample()
	}
	s.sc.SpanID = newSpanID()
	if t != nil && s.sc.Sampled {
		s.t = t
	}
	return ContextWith(ctx, s), s
}

func newTraceID() TraceID {
	var t TraceID
	_, _ = rand.Read(t[:])
	return t
}

func newSpanID() SpanID {
	var s SpanID
	binary.LittleEndian.PutUint64(s[:], mrand.Uint64()|1) // never zero
	return s
}

// An Exporter sends finished spans somewhere, in batches, from one
// goroutine. Return [Permanent] for errors that retrying won't fix.
type Exporter interface {
	Export(ctx context.Context, spans []SpanData) error
}

// ExporterFunc makes a function an Exporter.
type ExporterFunc func(ctx context.Context, spans []SpanData) error

// Export implements Exporter.
func (f ExporterFunc) Export(ctx context.Context, spans []SpanData) error { return f(ctx, spans) }

// Permanent marks an export error as one retrying won't fix.
func Permanent(err error) error { return batch.Permanent(err) }

// Batch says how spans are batched and retried, as logging's sinks are.
type Batch = batch.Options

// Options configures a tracer.
type Options struct {
	// Exporters get every recorded span.
	Exporters []Exporter
	// Sample is the fraction of new traces recorded, from 0 to 1. Traces
	// that come in from elsewhere keep their own decision. The default is
	// TERN_TRACE_SAMPLE's, or 1: everything.
	Sample float64
	// Batch says how spans are batched for exporters.
	Batch Batch
}

// Tracer records spans and hands them to exporters. Make one with New.
type Tracer struct {
	rate  float64
	ships []*batch.Shipper[SpanData]
}

// New returns a tracer. Under tern dev, it also sends spans to the dev
// server's log view.
func New(o Options) *Tracer {
	t := &Tracer{rate: o.Sample}
	if t.rate == 0 {
		t.rate = 1
		if v, err := strconv.ParseFloat(os.Getenv("TERN_TRACE_SAMPLE"), 64); err == nil && v >= 0 && v <= 1 {
			t.rate = v
		}
	}
	for _, e := range o.Exporters {
		c := batch.Config{Name: exporterName(e), Prefix: "tracing", What: "spans", ErrOut: os.Stderr}
		if cl, ok := e.(interface{ Close() error }); ok {
			c.Close = cl.Close
		}
		t.ships = append(t.ships, batch.New(c, e.Export, o.Batch))
	}
	if d := devShipper(); d != nil {
		t.ships = append(t.ships, d)
	}
	return t
}

func exporterName(e Exporter) string {
	if n, ok := e.(interface{ Name() string }); ok {
		return n.Name()
	}
	return strings.TrimPrefix(typeName(e), "*")
}

func (t *Tracer) sample() bool {
	if t.rate >= 1 {
		return true
	}
	return mrand.Float64() < t.rate
}

func (t *Tracer) export(d SpanData) {
	for _, sh := range t.ships {
		sh.Add(d)
	}
}

// Flush waits until the spans ended so far are exported, or ctx ends.
func (t *Tracer) Flush(ctx context.Context) error {
	if t == nil {
		return nil
	}
	var errs []error
	for _, sh := range t.ships {
		errs = append(errs, sh.Flush(ctx))
	}
	return errors.Join(errs...)
}

// Close flushes the exporters and stops them.
func (t *Tracer) Close(ctx context.Context) error {
	if t == nil {
		return nil
	}
	var errs []error
	for _, sh := range t.ships {
		if sh == dev.sh {
			errs = append(errs, sh.Flush(ctx))
			continue
		}
		errs = append(errs, sh.Close(ctx))
	}
	return errors.Join(errs...)
}

// Stats counts spans a tracer didn't export.
type Stats struct {
	Dropped uint64 // an exporter's queue was full
	Failed  uint64 // an exporter didn't take them after retrying
}

// Stats returns t's counts.
func (t *Tracer) Stats() Stats {
	var s Stats
	if t == nil {
		return s
	}
	for _, sh := range t.ships {
		s.Dropped += sh.Dropped()
		s.Failed += sh.Failed()
	}
	return s
}

var defaultTracer atomic.Pointer[Tracer]

// SetDefault makes t the tracer of spans started without a parent, such as
// those of jobs and of code that doesn't run in a request.
func SetDefault(t *Tracer) { defaultTracer.Store(t) }

// Default returns the default tracer, or nil.
func Default() *Tracer { return defaultTracer.Load() }

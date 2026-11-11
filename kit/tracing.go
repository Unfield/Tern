package kit

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"

	"github.com/Unfield/Tern/logging"
	"github.com/Unfield/Tern/tracing"
)

// WithTracer records each request as a trace with t: a server span for the
// request, and spans for its loads, actions, endpoints and server render.
// Without it, the App uses tracing.Default(), and under tern dev a tracer
// that only feeds the dev server's log view.
func WithTracer(t *tracing.Tracer) Option {
	return func(o *options) { o.tracer = t }
}

var (
	devTracerOnce sync.Once
	devTracer     *tracing.Tracer
)

// tracer is the App's tracer for this request, or nil.
func (a *App) tracer() *tracing.Tracer {
	if a.opts.tracer != nil {
		return a.opts.tracer
	}
	if t := tracing.Default(); t != nil {
		return t
	}
	if a.dev != nil {
		devTracerOnce.Do(func() { devTracer = tracing.DevOnly() })
		return devTracer
	}
	return nil
}

// startRequest starts the request's server span, continuing a trusted
// traceparent, and returns r carrying it.
func (a *App) startRequest(r *http.Request, trusted bool) (*http.Request, *tracing.Span) {
	t := a.tracer()
	if t == nil || a.dev != nil && a.dev.handles(r) {
		return r, nil // Vite's modules and assets, under tern dev, aren't the app's
	}
	opts := []tracing.StartOption{
		tracing.WithTracer(t),
		tracing.WithKind(tracing.Server),
		tracing.WithAttrs(
			slog.String("http.request.method", r.Method),
			slog.String("url.path", r.URL.Path),
			slog.String(logging.RequestIDKey, logging.RequestID(r.Context())),
		),
	}
	if trusted {
		if sc, ok := tracing.ParseTraceparent(r.Header.Get("Traceparent")); ok {
			opts = append(opts, tracing.WithRemoteParent(sc))
		}
	}
	ctx, span := tracing.Start(r.Context(), r.Method+" "+r.URL.Path, opts...)
	return r.WithContext(ctx), span
}

// endRequest finishes the server span with the response's status.
func endRequest(span *tracing.Span, status int) {
	if status == 0 {
		status = http.StatusOK
	}
	span.SetAttrs(slog.Int("http.response.status_code", status))
	if status >= 500 {
		span.SetStatus(tracing.Error, fmt.Sprintf("%d %s", status, http.StatusText(status)))
	}
	span.End()
}

// nameRequest names the server span after the route, once it is known.
func nameRequest(r *http.Request, route string) {
	span := tracing.FromContext(r.Context())
	if !span.IsRecording() {
		return
	}
	span.SetName(r.Method + " " + route)
	span.SetAttrs(slog.String("http.route", route))
}

// inSpan runs fn with c's request in a child span named name, recording
// its error.
func inSpan(c *Ctx, name string, fn func() error, attrs ...slog.Attr) error {
	if tracing.FromContext(c.Request.Context()) == nil {
		return fn()
	}
	ctx, span := tracing.Start(c.Request.Context(), name, tracing.WithAttrs(attrs...))
	saved := c.Request
	c.Request = c.Request.WithContext(ctx)
	err := fn()
	c.Request = saved
	if err != nil && !isControlFlow(err) {
		span.RecordError(err)
	}
	span.End()
	return err
}

// isControlFlow reports errors that are how loads and actions answer, not
// failures: redirects, and 4xx errors such as NotFound and Fail.
func isControlFlow(err error) bool {
	var re *RedirectError
	var fe *FailError
	if errors.As(err, &re) || errors.As(err, &fe) {
		return true
	}
	var he *HTTPError
	return errors.As(err, &he) && he.Status < 500
}

// renderSpan starts the server render's span.
func renderSpan(ctx context.Context) (context.Context, *tracing.Span) {
	if tracing.FromContext(ctx) == nil {
		return ctx, nil
	}
	return tracing.Start(ctx, "render")
}

// statusRecorder notes the status a handler responds with, for the
// request's span, passing everything else through.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	if s.status == 0 {
		s.status = code
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	return s.ResponseWriter.Write(b)
}

func (s *statusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (s *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := s.ResponseWriter.(http.Hijacker); ok {
		if s.status == 0 {
			s.status = http.StatusSwitchingProtocols
		}
		return h.Hijack()
	}
	return nil, nil, errors.New("kit: the connection can't be hijacked")
}

func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

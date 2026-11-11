package tracing

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"reflect"

	"github.com/Unfield/Tern/logging"
)

func typeName(v any) string { return reflect.TypeOf(v).String() }

// Inject sets the traceparent header for the span in r's context, so the
// service r goes to continues the trace.
func Inject(r *http.Request) {
	if s := FromContext(r.Context()); s != nil && s.sc.IsValid() {
		r.Header.Set("Traceparent", s.sc.Traceparent())
	}
}

// Transport wraps base (http.DefaultTransport if nil) so each request is a
// client span, and carries the trace to the server it goes to:
//
//	client := &http.Client{Transport: tracing.Transport(nil)}
func Transport(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return &transport{base: base}
}

type transport struct{ base http.RoundTripper }

func (t *transport) RoundTrip(r *http.Request) (*http.Response, error) {
	if FromContext(r.Context()) == nil && Default() == nil {
		return t.base.RoundTrip(r)
	}
	ctx, span := Start(r.Context(), r.Method+" "+r.URL.Host, WithKind(Client), WithAttrs(
		slog.String("http.request.method", r.Method),
		slog.String("server.address", r.URL.Hostname()),
		slog.String("url.full", redactURL(r)),
	))
	r = r.Clone(ctx)
	Inject(r)
	resp, err := t.base.RoundTrip(r)
	if err != nil {
		span.RecordError(err)
	} else {
		span.SetAttrs(slog.Int("http.response.status_code", resp.StatusCode))
		if resp.StatusCode >= 500 {
			span.SetStatus(Error, fmt.Sprintf("%d %s", resp.StatusCode, http.StatusText(resp.StatusCode)))
		}
	}
	span.End()
	return resp, err
}

// redactURL is the URL without credentials or query values that look
// like secrets.
func redactURL(r *http.Request) string {
	u := *r.URL
	return redact(&u)
}

func redact(u *url.URL) string { return logging.RedactURL(u) }

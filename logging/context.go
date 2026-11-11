package logging

import (
	"context"
	"log/slog"
	"slices"
)

type ctxKey struct{}

// ctxAttrs is the list of attributes a context carries. Each With makes a
// new one, so a context's list never changes once it is made.
type ctxAttrs struct {
	attrs []slog.Attr
}

// With returns a copy of ctx that carries attrs: every record logged with
// that context, through a logger from [New] or a handler from [Wrap],
// includes them. kit adds the request's ID this way.
//
//	ctx = logging.With(ctx, slog.String("user", id))
//	slog.InfoContext(ctx, "invoice sent") // ... user=… request_id=…
//
// Attributes with the same key as one already carried replace it.
func With(ctx context.Context, attrs ...slog.Attr) context.Context {
	if len(attrs) == 0 {
		return ctx
	}
	var list []slog.Attr
	if old, ok := ctx.Value(ctxKey{}).(*ctxAttrs); ok {
		list = make([]slog.Attr, 0, len(old.attrs)+len(attrs))
		for _, a := range old.attrs {
			if !slices.ContainsFunc(attrs, func(b slog.Attr) bool { return b.Key == a.Key }) {
				list = append(list, a)
			}
		}
	}
	list = append(list, attrs...)
	return context.WithValue(ctx, ctxKey{}, &ctxAttrs{attrs: list})
}

// Attrs returns the attributes ctx carries, nil if none. The slice must
// not be changed.
func Attrs(ctx context.Context) []slog.Attr {
	if ctx == nil {
		return nil
	}
	if c, ok := ctx.Value(ctxKey{}).(*ctxAttrs); ok {
		return c.attrs
	}
	return nil
}

// RequestID returns the request ID kit gave the request ctx belongs to, or
// "".
func RequestID(ctx context.Context) string {
	for _, a := range Attrs(ctx) {
		if a.Key == RequestIDKey {
			return a.Value.String()
		}
	}
	return ""
}

// The keys kit uses for a request's attributes.
const (
	// RequestIDKey is the key of the request's ID.
	RequestIDKey = "request_id"
	// TraceIDKey and SpanIDKey are the W3C trace context's IDs, from a
	// traceparent header the app trusts. The OTLP sink sends them as the
	// record's trace and span.
	TraceIDKey = "trace_id"
	SpanIDKey  = "span_id"
)

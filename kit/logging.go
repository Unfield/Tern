package kit

import (
	"context"
	"encoding/hex"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"strings"

	"github.com/Unfield/Tern/logging"
)

// Every request gets an ID: the X-Request-Id it came with, from a client
// whose forwarding headers are trusted (see [WithTrustedProxies]), or a new
// one. The response carries it in X-Request-Id, and records logged with
// the request's context include it (see [logging.With]), as do those of
// [Ctx.Logger]. A trusted W3C traceparent header adds its trace and span
// IDs.

// RequestIDHeader is the header that carries a request's ID.
const RequestIDHeader = "X-Request-Id"

// requestContext adds the request's logging attributes to its context.
func requestContext(w http.ResponseWriter, r *http.Request, trusted bool) *http.Request {
	var id string
	attrs := make([]slog.Attr, 0, 3)
	if trusted {
		if v := r.Header.Get(RequestIDHeader); validRequestID(v) {
			id = v
		}
		if trace, span, ok := parseTraceparent(r.Header.Get("Traceparent")); ok {
			attrs = append(attrs, slog.String(logging.TraceIDKey, trace), slog.String(logging.SpanIDKey, span))
		}
	}
	if id == "" {
		id = newRequestID()
	}
	w.Header().Set(RequestIDHeader, id)
	attrs = append([]slog.Attr{slog.String(logging.RequestIDKey, id)}, attrs...)
	return r.WithContext(logging.With(r.Context(), attrs...))
}

func newRequestID() string {
	var b [8]byte
	u := rand.Uint64()
	for i := range b {
		b[i] = byte(u >> (8 * i))
	}
	return hex.EncodeToString(b[:])
}

// validRequestID accepts IDs of up to 128 letters, digits and -_.:
// characters, so a client can't put anything else in the logs.
func validRequestID(s string) bool {
	if s == "" || len(s) > 128 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		ok := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.' || c == ':'
		if !ok {
			return false
		}
	}
	return true
}

// parseTraceparent reads a W3C traceparent header: version-trace-span-flags.
func parseTraceparent(s string) (trace, span string, ok bool) {
	parts := strings.Split(strings.TrimSpace(s), "-")
	if len(parts) < 4 || len(parts[0]) != 2 || parts[0] == "ff" || len(parts[1]) != 32 || len(parts[2]) != 16 || len(parts[3]) != 2 {
		return "", "", false
	}
	if parts[0] == "00" && len(parts) != 4 {
		return "", "", false
	}
	for _, p := range parts[:4] {
		if !lowerHex(p) {
			return "", "", false
		}
	}
	if allZero(parts[1]) || allZero(parts[2]) {
		return "", "", false
	}
	return parts[1], parts[2], true
}

func lowerHex(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func allZero(s string) bool { return strings.Trim(s, "0") == "" }

// Logger returns the app's logger with the request's attributes: its ID
// (and trace, if any) and route. Records logged with it carry them even
// when logged without a context.
func (c *Ctx) Logger() *slog.Logger {
	h := c.app.logger.Handler()
	if c.routeID != "" {
		h = h.WithAttrs([]slog.Attr{slog.String("route", c.routeID)})
	}
	return slog.New(&boundHandler{Handler: h, ctx: c.Request.Context()})
}

// boundHandler logs records without request attributes in their context
// with those of ctx.
type boundHandler struct {
	slog.Handler
	ctx context.Context
}

func (b *boundHandler) Handle(ctx context.Context, r slog.Record) error {
	if logging.Attrs(ctx) == nil {
		ctx = b.ctx
	}
	return b.Handler.Handle(ctx, r)
}

func (b *boundHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &boundHandler{Handler: b.Handler.WithAttrs(attrs), ctx: b.ctx}
}

func (b *boundHandler) WithGroup(name string) slog.Handler {
	return &boundHandler{Handler: b.Handler.WithGroup(name), ctx: b.ctx}
}

// LogWith adds attributes to the records logged in this request from now
// on, through its context and [Ctx.Logger]: in a hook, the signed-in
// user's ID, say.
func (c *Ctx) LogWith(attrs ...slog.Attr) {
	c.Request = c.Request.WithContext(logging.With(c.Request.Context(), attrs...))
	if root := c.rootCtx(); root != c {
		root.Request = root.Request.WithContext(logging.With(root.Request.Context(), attrs...))
	}
}

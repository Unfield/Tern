// Package compress gzips a Tern app's dynamic responses (pages, data,
// endpoint JSON) for clients that accept it:
//
//	ternApp(kit.WithPlugins(compress.New()))
//
// Static assets are already precompressed by tern build and are left
// alone. Flushes pass through, so streamed data still arrives as soon as it
// resolves.
package compress

import (
	"compress/gzip"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/Unfield/Tern/kit"
)

// Option configures the plugin.
type Option func(*Plugin)

// Level sets the gzip level. The default is gzip.DefaultCompression.
func Level(level int) Option { return func(p *Plugin) { p.level = level } }

// MinSize skips responses that declare a Content-Length below n bytes. The
// default is 512.
func MinSize(n int) Option { return func(p *Plugin) { p.minSize = n } }

// Plugin compresses responses. Create it with New.
type Plugin struct {
	level   int
	minSize int
	pool    sync.Pool
}

// New returns the compression plugin.
func New(opts ...Option) *Plugin {
	p := &Plugin{level: gzip.DefaultCompression, minSize: 512}
	for _, o := range opts {
		o(p)
	}
	return p
}

// Name implements kit.Plugin.
func (*Plugin) Name() string { return "compress" }

// Setup implements kit.Plugin.
func (p *Plugin) Setup(s *kit.Setup) error {
	if _, err := gzip.NewWriterLevel(nil, p.level); err != nil {
		return err
	}
	s.Middleware(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Add("Vary", "Accept-Encoding")
			if r.Method == http.MethodHead || !acceptsGzip(r) {
				next.ServeHTTP(w, r)
				return
			}
			cw := &writer{ResponseWriter: w, p: p}
			defer cw.close()
			next.ServeHTTP(cw, r)
		})
	})
	return nil
}

var compressible = map[string]bool{
	"text/html": true, "text/plain": true, "text/css": true, "text/xml": true, "text/javascript": true,
	"application/json": true, "application/xml": true, "application/javascript": true,
	"application/rss+xml": true, "application/atom+xml": true, "image/svg+xml": true,
}

func acceptsGzip(r *http.Request) bool {
	for _, part := range strings.Split(r.Header.Get("Accept-Encoding"), ",") {
		enc, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		if strings.EqualFold(strings.TrimSpace(enc), "gzip") && strings.ReplaceAll(params, " ", "") != "q=0" {
			return true
		}
	}
	return false
}

// writer decides on the first write whether to compress.
type writer struct {
	http.ResponseWriter
	p       *Plugin
	gz      *gzip.Writer
	decided bool
}

func (w *writer) decide() {
	if w.decided {
		return
	}
	w.decided = true
	h := w.Header()
	if h.Get("Content-Encoding") != "" {
		return
	}
	mt, _, _ := mime.ParseMediaType(h.Get("Content-Type"))
	if !compressible[mt] {
		return
	}
	if n, err := strconv.Atoi(h.Get("Content-Length")); err == nil && n < w.p.minSize {
		return
	}
	h.Del("Content-Length")
	h.Set("Content-Encoding", "gzip")
	if gz, ok := w.p.pool.Get().(*gzip.Writer); ok {
		gz.Reset(w.ResponseWriter)
		w.gz = gz
	} else {
		w.gz, _ = gzip.NewWriterLevel(w.ResponseWriter, w.p.level)
	}
}

func (w *writer) WriteHeader(code int) {
	if code != http.StatusNoContent && code != http.StatusNotModified && code >= 200 {
		w.decide()
	}
	w.decided = true
	w.ResponseWriter.WriteHeader(code)
}

func (w *writer) Write(b []byte) (int, error) {
	w.decide()
	if w.gz == nil {
		return w.ResponseWriter.Write(b)
	}
	return w.gz.Write(b)
}

// Flush sends what has been compressed so far, for streamed responses.
func (w *writer) Flush() {
	if w.gz != nil {
		_ = w.gz.Flush()
	}
	_ = http.NewResponseController(w.ResponseWriter).Flush()
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (w *writer) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *writer) close() {
	if w.gz == nil {
		return
	}
	_ = w.gz.Close()
	w.gz.Reset(nil)
	w.p.pool.Put(w.gz)
	w.gz = nil
}

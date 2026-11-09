// Package requestlog logs every request a Tern app serves, with slog:
//
//	ternApp(kit.WithPlugins(requestlog.New()))
//
// Each line has the method, path, status, duration and response size. 5xx
// responses log at error level and 4xx at warn; everything else at info.
package requestlog

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Unfield/Tern/kit"
)

// Option configures the plugin.
type Option func(*Plugin)

// Logger sets the logger. The default is the app's logger.
func Logger(l *slog.Logger) Option { return func(p *Plugin) { p.logger = l } }

// SkipAssets leaves out requests for /_app/ assets and other files with an
// extension, which are usually noise.
func SkipAssets() Option { return func(p *Plugin) { p.skipAssets = true } }

// Plugin logs requests. Create it with New.
type Plugin struct {
	logger     *slog.Logger
	skipAssets bool
}

// New returns the request logging plugin.
func New(opts ...Option) *Plugin {
	p := &Plugin{}
	for _, o := range opts {
		o(p)
	}
	return p
}

// Name implements kit.Plugin.
func (*Plugin) Name() string { return "requestlog" }

// Setup implements kit.Plugin.
func (p *Plugin) Setup(s *kit.Setup) error {
	logger := p.logger
	if logger == nil {
		logger = s.Logger()
	}
	s.Middleware(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if p.skipAssets && isAsset(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			start := time.Now()
			rec := &recorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)
			level := slog.LevelInfo
			switch {
			case rec.status >= 500:
				level = slog.LevelError
			case rec.status >= 400:
				level = slog.LevelWarn
			}
			logger.LogAttrs(r.Context(), level, "request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", rec.status),
				slog.Duration("duration", time.Since(start)),
				slog.Int64("bytes", rec.bytes),
			)
		})
	})
	return nil
}

func isAsset(path string) bool {
	if strings.HasPrefix(path, "/_app/") {
		return true
	}
	last := path[strings.LastIndexByte(path, '/')+1:]
	return strings.Contains(last, ".") && !strings.HasSuffix(last, ".json")
}

type recorder struct {
	http.ResponseWriter
	status int
	bytes  int64
	wrote  bool
}

func (r *recorder) WriteHeader(code int) {
	if !r.wrote {
		r.status, r.wrote = code, true
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *recorder) Write(b []byte) (int, error) {
	r.wrote = true
	n, err := r.ResponseWriter.Write(b)
	r.bytes += int64(n)
	return n, err
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (r *recorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

package kit

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
)

// Plugin extends an App with reusable behavior: hooks, middleware,
// matchers, extra HTTP handlers, tags in every page's <head>, and work to
// do on shutdown. Add plugins with [WithPlugins].
//
//	type Plugin struct{}
//
//	func (Plugin) Name() string { return "hello" }
//
//	func (Plugin) Setup(s *kit.Setup) error {
//		s.Handle("GET /hello", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
//			fmt.Fprintln(w, "hello")
//		}))
//		return nil
//	}
type Plugin interface {
	// Name identifies the plugin in logs and errors.
	Name() string
	// Setup registers the plugin's behavior. It runs once, when the App is
	// created; an error stops the App from starting.
	Setup(s *Setup) error
}

// WithPlugins adds plugins. They are set up in order, after the other
// options, so a plugin's hooks and middleware run after those added
// directly with WithHook and WithMiddleware.
func WithPlugins(plugins ...Plugin) Option {
	return func(o *options) { o.plugins = append(o.plugins, plugins...) }
}

// Setup is how a plugin registers its behavior.
type Setup struct {
	o      *options
	app    *App
	plugin string
}

// Name returns the plugin's name.
func (s *Setup) Name() string { return s.plugin }

// Logger returns a logger tagged with the plugin's name.
func (s *Setup) Logger() *slog.Logger { return s.o.logger.With("plugin", s.plugin) }

// Hook adds a hook that runs before every page, data, action and endpoint request.
func (s *Setup) Hook(h HookFunc) { s.o.hooks = append(s.o.hooks, h) }

// Middleware wraps the App in net/http middleware.
func (s *Setup) Middleware(mw func(http.Handler) http.Handler) {
	s.o.middleware = append(s.o.middleware, mw)
}

// TrustOrigin allows cross-origin form posts and endpoint requests from
// origin, as WithTrustedOrigins does. CORS plugins use it.
func (s *Setup) TrustOrigin(origin string) { s.o.trustedOrigins = append(s.o.trustedOrigins, origin) }

// AllowCrossOrigin exempts requests matching pattern (net/http.ServeMux
// syntax) from cross-origin protection. Use it only for endpoints that
// don't act on cookies, such as an OAuth token endpoint that clients on
// other sites call with their own credentials.
func (s *Setup) AllowCrossOrigin(pattern string) {
	s.o.crossOriginOK = append(s.o.crossOriginOK, pattern)
}

// Matcher registers a parameter matcher, as WithMatcher does.
func (s *Setup) Matcher(name string, match func(value string) bool) { s.o.matchers[name] = match }

// Handle serves pattern with h, before Tern's routes. Patterns use
// net/http.ServeMux syntax, such as "GET /healthz" or "/metrics".
func (s *Setup) Handle(pattern string, h http.Handler) {
	s.app.pluginMux.Handle(pattern, h)
	s.app.hasPluginRoutes = true
}

// Endpoint serves pattern with fn like a route's endpoint: after the hooks,
// with a Ctx, so it can use sessions and Locals, and with errors handled
// like an endpoint's. Patterns use net/http.ServeMux syntax, such as
// "GET /auth/{provider}/login"; read wildcards with ctx.Request.PathValue.
// Endpoints take precedence over the app's routes.
func (s *Setup) Endpoint(pattern string, fn HandlerFunc) {
	s.app.ctxMux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		c := r.Context().Value(ctxKey{}).(*Ctx)
		c.Request = r // with the pattern's path values
		if err := c.app.safely(func() error { return fn(c) }); err != nil {
			c.app.endpointError(c, err)
		}
	})
	s.app.hasCtxRoutes = true
}

type ctxKey struct{}

// Head adds HTML to the <head> of every page, such as a <meta> tag or an
// analytics script. Scripts should use the per-request nonce; see
// [Setup.HeadFunc].
func (s *Setup) Head(html string) {
	s.app.head = append(s.app.head, func(*Ctx, string) string { return html })
}

// HeadFunc adds HTML computed per request to the <head> of every page.
// nonce is the request's Content-Security-Policy nonce, for inline scripts.
func (s *Setup) HeadFunc(fn func(c *Ctx, nonce string) string) {
	s.app.head = append(s.app.head, fn)
}

// OnShutdown registers work to do when the App shuts down, such as
// flushing buffers. Functions run in reverse order of registration.
func (s *Setup) OnShutdown(fn func(ctx context.Context) error) {
	s.app.shutdown = append(s.app.shutdown, fn)
}

// OnStart registers work to do when the server starts, before it accepts
// requests, such as running migrations or starting background workers. ctx
// is canceled when the server begins shutting down; start long-running work
// in a goroutine that stops then. An error stops the server from starting.
// Start functions run in order of registration, and not while tern build
// prerenders pages.
func (s *Setup) OnStart(fn func(ctx context.Context) error) {
	s.app.start = append(s.app.start, fn)
}

// Start runs the plugins' start functions. Serve and ListenAndServe call it;
// call it yourself when serving the App with your own http.Server. It runs
// once; later calls return the first result.
func (a *App) Start() error {
	a.startOnce.Do(func() {
		if a.prerendering {
			return
		}
		for _, fn := range a.start {
			if err := fn(a.shutdownCtx); err != nil {
				a.startErr = err
				return
			}
		}
	})
	return a.startErr
}

// setupPlugins runs each plugin's Setup.
func (a *App) setupPlugins() error {
	seen := map[string]bool{}
	for _, p := range a.opts.plugins {
		name := p.Name()
		if seen[name] {
			return fmt.Errorf("kit: plugin %q added twice", name)
		}
		seen[name] = true
		if err := p.Setup(&Setup{o: &a.opts, app: a, plugin: name}); err != nil {
			return fmt.Errorf("kit: plugin %q: %w", name, err)
		}
	}
	return nil
}

// runShutdown runs the plugins' shutdown functions, newest first.
func (a *App) runShutdown(ctx context.Context) error {
	var errs []error
	for i := len(a.shutdown) - 1; i >= 0; i-- {
		errs = append(errs, a.shutdown[i](ctx))
	}
	a.shutdown = nil
	return errors.Join(errs...)
}

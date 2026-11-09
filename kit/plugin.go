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

// Matcher registers a parameter matcher, as WithMatcher does.
func (s *Setup) Matcher(name string, match func(value string) bool) { s.o.matchers[name] = match }

// Handle serves pattern with h, before Tern's routes. Patterns use
// net/http.ServeMux syntax, such as "GET /healthz" or "/metrics".
func (s *Setup) Handle(pattern string, h http.Handler) {
	s.app.pluginMux.Handle(pattern, h)
	s.app.hasPluginRoutes = true
}

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

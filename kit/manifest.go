package kit

import (
	"io/fs"
	"log/slog"
	"net/http"
	"time"
)

// LoadFunc is a Load or LayoutLoad function with its result type erased.
// Generated code builds it with [LoadOf].
type LoadFunc func(*Ctx) (any, error)

// LoadOf adapts a typed load function.
func LoadOf[T any](f func(*Ctx) (T, error)) LoadFunc {
	return func(c *Ctx) (any, error) { return f(c) }
}

// ActionFunc handles a form submission. Its result is sent to the page as
// the `form` prop. Use [Action] to write actions with a typed result, so the
// generated `form` type is precise.
type ActionFunc func(*Ctx) (any, error)

// Action adapts a typed action function:
//
//	var Actions = kit.Actions{
//		"default": kit.Action(create),
//	}
//
//	func create(ctx *kit.Ctx) (Created, error) { ... }
func Action[T any](f func(*Ctx) (T, error)) ActionFunc {
	return func(c *Ctx) (any, error) { return f(c) }
}

// Actions maps action names to handlers. A form posting to the page with
// no ?/name runs "default"; <form action="?/create"> runs "create".
type Actions map[string]ActionFunc

// HandlerFunc handles an endpoint request, such as an exported GET.
type HandlerFunc func(*Ctx) error

// Manifest describes an app's routes and assets. It is produced by
// `tern gen` in tern.gen.go; you do not write it by hand.
type Manifest struct {
	// Layouts are shared by routes and referenced by index.
	Layouts []LayoutDef
	// Routes, in matching priority order.
	Routes []RouteDef
	// Fallback describes what renders when no route matches.
	Fallback FallbackDef
	// AppHTML is the contents of src/app.html.
	AppHTML string
	// Client is the built client: Vite's output directory, including
	// .vite/manifest.json. It may be nil in development.
	Client fs.FS
	// ClientEntry is the Vite manifest key of the client entry module.
	ClientEntry string
	// ServerBundle is the server renderer built by `vite build --ssr`, or
	// "" to render on the client only.
	ServerBundle string
}

// LayoutDef is a layout directory.
type LayoutDef struct {
	Dir string
	// Component is the +layout.svelte path relative to the project root,
	// or "" if the directory only has a LayoutLoad.
	Component string
	Load      LoadFunc
}

// RouteDef is a page, an endpoint, or both.
type RouteDef struct {
	// ID is the route ID, e.g. "/blog/slug.param".
	ID string
	// Layouts are indices into Manifest.Layouts, outermost first.
	Layouts []int
	// Page is nil for endpoint-only routes.
	Page *PageDef
	// Endpoint maps HTTP methods to handlers.
	Endpoint map[string]HandlerFunc
	// Prerender renders the page to static HTML at build time; set by
	// `const Prerender = true` in the route's package.
	Prerender bool
	// Entries lists the parameters to prerender a dynamic route with; set
	// by an exported Entries function in the route's package.
	Entries func() ([]map[string]string, error)
}

// PageDef is a route's page.
type PageDef struct {
	// Component is the +page.svelte path relative to the project root.
	Component string
	Load      LoadFunc
	Actions   Actions
	// NoSSR disables server-side rendering for this page; set by
	// `const SSR = false` in the route's package.
	NoSSR bool
}

// FallbackDef renders 404s for URLs that match no route.
type FallbackDef struct {
	// Layouts are indices into Manifest.Layouts: the root layout, if any.
	Layouts []int
}

// Option configures an [App].
type Option func(*options)

type options struct {
	hooks           []HookFunc
	middleware      []func(http.Handler) http.Handler
	matchers        map[string]func(string) bool
	trustedOrigins  []string
	csrf            bool
	bodyLimit       int64
	multipartMemory int64
	logger          *slog.Logger
	addr            string
	ssr             bool
	ssrWorkers      int
	ssrTimeout      time.Duration
}

// HookFunc runs before every page, data, action and endpoint request. It
// can read the request, set Locals, and return an error such as
// kit.Redirect to stop the request.
type HookFunc func(*Ctx) error

// WithHook adds a hook. Hooks run in the order they are added.
func WithHook(h HookFunc) Option {
	return func(o *options) { o.hooks = append(o.hooks, h) }
}

// WithMiddleware wraps the app in standard net/http middleware. The first
// middleware added is the outermost.
func WithMiddleware(mw ...func(http.Handler) http.Handler) Option {
	return func(o *options) { o.middleware = append(o.middleware, mw...) }
}

// WithMatcher registers a parameter matcher used by directories such as
// id.param.integer. The built-in matchers "integer" and "uuid" are always
// available.
func WithMatcher(name string, match func(value string) bool) Option {
	return func(o *options) { o.matchers[name] = match }
}

// WithTrustedOrigins allows cross-origin form submissions and endpoint
// requests from the given origins, such as "https://example.com".
func WithTrustedOrigins(origins ...string) Option {
	return func(o *options) { o.trustedOrigins = append(o.trustedOrigins, origins...) }
}

// WithoutCSRFProtection disables cross-origin request protection. Only use
// it if something in front of the app already provides it.
func WithoutCSRFProtection() Option {
	return func(o *options) { o.csrf = false }
}

// WithBodyLimit sets the maximum request body size for actions and
// endpoints, in bytes. The default is 1 MiB.
func WithBodyLimit(n int64) Option {
	return func(o *options) { o.bodyLimit = n }
}

// WithLogger sets the logger for unexpected errors and server events.
// The default logs to slog.Default().
func WithLogger(l *slog.Logger) Option {
	return func(o *options) { o.logger = l }
}

// WithAddr sets the address ListenAndServe listens on. The default is
// HOST:PORT from the environment, or ":3000".
func WithAddr(addr string) Option {
	return func(o *options) { o.addr = addr }
}

// WithoutSSR renders every page on the client only. Use `const SSR = false`
// in a route's package to turn it off for one page instead.
func WithoutSSR() Option {
	return func(o *options) { o.ssr = false }
}

// WithSSRWorkers sets how many pages can render on the server at once.
// Each worker is a JavaScript VM using roughly 5 to 20 MB of memory. The
// default is GOMAXPROCS.
func WithSSRWorkers(n int) Option {
	return func(o *options) { o.ssrWorkers = n }
}

// WithSSRTimeout bounds how long a server render may take before the page
// falls back to client rendering. The default is 5 seconds.
func WithSSRTimeout(d time.Duration) Option {
	return func(o *options) { o.ssrTimeout = d }
}

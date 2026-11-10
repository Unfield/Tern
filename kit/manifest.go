package kit

import (
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/Unfield/Tern/goscript"
	"github.com/Unfield/Tern/native/js"
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
//
// Manifest and the *Def types are the interface between generated code and
// the runtime. They are not covered by Tern's compatibility promise and may
// change in minor releases: `go get -tool` keeps the CLI that generates
// them and the kit package that reads them at the same version.
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
	// Native is the server renderer compiled to Go by tern build, if it
	// was: it makes a realm with the app's modules evaluated. Pages render
	// with it, and with ServerBundle only what it can't render. The
	// generated tern_native.gen.go sets it.
	Native func(setup func(*js.Realm)) *js.Realm
	// BasePath is the URL prefix the app is served under, from tern.json,
	// such as "/app"; "" serves it at the root.
	BasePath string
	// Locales holds src/locales/<locale>.json, or nil without i18n.
	Locales fs.FS
	// Content holds src/content, or nil if the app has none. See Ctx.Content.
	Content fs.FS
	// Live is set when the app has live Go components; tern gen registers
	// them with the goscript package.
	Live bool
	// OpenAPI is the OpenAPI description of the typed endpoints, served
	// with WithOpenAPI.
	OpenAPI []byte
	// DefaultLocale is the fallback locale; "" picks "en" or the first.
	DefaultLocale string
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
	Entries func(*Ctx) ([]map[string]string, error)
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
	trustedProxies  []string
	crossOriginOK   []string
	openAPI         *openAPIOptions
	csrf            bool
	bodyLimit       int64
	multipartMemory int64
	logger          *slog.Logger
	addr            string
	plugins         []Plugin
	csp             string
	cspReportOnly   bool
	ssr             bool
	ssrWorkers      int
	ssrCache        int
	ssrTimeout      time.Duration
	native          bool
	live            LiveOptions
	liveBroker      goscript.Broker
}

// HookFunc runs before every page, data, action and endpoint request. It
// can read the request, set Locals, and return an error such as
// kit.Redirect to stop the request.
type HookFunc func(*Ctx) error

// WithHook adds a hook. Hooks run in the order they are added, after the
// hooks of plugins, so they can use sessions and other plugin state.
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
// The default is GOMAXPROCS. Pages compiled to Go render in parallel on
// every worker; with the JavaScript engine, each worker is a VM using
// roughly 5 to 20 MB, and the default is at most 4: the VMs share a lock
// in the C runtime, so more rarely render faster.
func WithSSRWorkers(n int) Option {
	return func(o *options) { o.ssrWorkers = n }
}

// WithSSRCache sets how much server-rendered HTML is kept in memory, in
// bytes. A render depends only on the page's data and URL, so a page that
// looks the same for many visitors renders once; pages with per-user data
// get their own entries, evicted least recently used first. The default is
// 32 MiB; 0 turns the cache off.
func WithSSRCache(bytes int) Option {
	return func(o *options) { o.ssrCache = bytes }
}

// WithSSRTimeout bounds how long a server render may take before the page
// falls back to client rendering. The default is 5 seconds.
func WithSSRTimeout(d time.Duration) Option {
	return func(o *options) { o.ssrTimeout = d }
}

// WithoutNativeSSR renders every page with the JavaScript engine, even when
// tern build compiled the server renderer to Go. Setting TERN_NATIVE=0 does
// the same without a rebuild.
func WithoutNativeSSR() Option {
	return func(o *options) { o.native = false }
}

// WithContentSecurityPolicy sends a Content-Security-Policy header on every
// page. Tern adds a per-request nonce to script-src (creating the directive
// if needed) and puts it on its own scripts, so a strict policy such as
// "default-src 'self'" works without 'unsafe-inline'. Use %tern.nonce% in
// src/app.html for your own inline scripts.
func WithContentSecurityPolicy(policy string) Option {
	return func(o *options) { o.csp, o.cspReportOnly = policy, false }
}

// WithContentSecurityPolicyReportOnly is like WithContentSecurityPolicy but
// sends Content-Security-Policy-Report-Only, to test a policy without
// enforcing it.
func WithContentSecurityPolicyReportOnly(policy string) Option {
	return func(o *options) { o.csp, o.cspReportOnly = policy, true }
}

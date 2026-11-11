package kit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Unfield/Tern/goscript"
	"github.com/Unfield/Tern/internal/i18n"
	"github.com/Unfield/Tern/internal/jsonenc"
	"github.com/Unfield/Tern/internal/routes"
	"github.com/Unfield/Tern/internal/ssr"
	"github.com/Unfield/Tern/logging"
	"github.com/Unfield/Tern/tracing"
)

// App serves a Tern application. It implements http.Handler, so it can be
// used with any server or router; [App.ListenAndServe] runs it with
// production-ready defaults.
type App struct {
	m       *Manifest
	opts    options
	router  *routes.Router
	defs    map[*routes.Route]*RouteDef
	handler http.Handler
	logger  *slog.Logger
	// tls is how the server terminates TLS, once WrapListener has run.
	tls     *tlsSetup
	shell   *shell
	assets  *assets
	dev     *devServer
	ssr     ssr.Renderer
	closers []io.Closer
	// prerendering is set while `tern build` prerenders pages.
	prerendering bool

	// publicEnv holds PUBLIC_ environment variables, sent to the client.
	publicEnv map[string]string
	// i18n is the message catalog from src/locales, or nil.
	i18n *i18n.Catalog

	// Set up by plugins.
	pluginMux       *http.ServeMux
	hasPluginRoutes bool
	ctxMux          *http.ServeMux // Setup.Endpoint
	hasCtxRoutes    bool
	head            []func(*Ctx, string) string
	shutdown        []func(context.Context) error
	start           []func(context.Context) error
	startOnce       sync.Once
	startErr        error

	emails    ssr.EmailRenderer
	emailsErr error
	emailOnce sync.Once

	// shutdownCtx is canceled when the server begins shutting down, which
	// ends event streams and WebSockets.
	shutdownCtx   context.Context
	beginShutdown context.CancelFunc
}

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// New creates an App from a generated manifest. Generated code calls it
// for you; see the ternApp function in tern.gen.go.
func New(m *Manifest, opts ...Option) (*App, error) {
	o := options{
		matchers: map[string]func(string) bool{
			"integer": func(v string) bool { _, err := strconv.ParseInt(v, 10, 64); return err == nil },
			"uuid":    uuidRe.MatchString,
		},
		csrf:            true,
		ssr:             true,
		ssrCache:        32 << 20,
		native:          true,
		bodyLimit:       1 << 20,
		multipartMemory: 8 << 20,
	}
	for _, opt := range opts {
		opt(&o)
	}
	if o.logger == nil {
		o.logger = slog.Default()
	}
	// Records get the request's attributes from its context, and under
	// tern dev go to the log view too.
	if h := logging.Wrap(o.logger.Handler()); h != o.logger.Handler() {
		o.logger = slog.New(h)
	}

	a := &App{m: m, opts: o, logger: o.logger, defs: map[*routes.Route]*RouteDef{}, prerendering: os.Getenv("TERN_PRERENDER") == "1", pluginMux: http.NewServeMux(), ctxMux: http.NewServeMux()}
	a.shutdownCtx, a.beginShutdown = context.WithCancel(context.Background())
	if m.Live {
		a.ctxMux.HandleFunc("GET "+LivePath, a.endpointHandler(a.serveLive))
		a.hasCtxRoutes = true
	}
	if err := a.setupPlugins(); err != nil {
		return nil, err
	}
	if b := o.liveBroker; b != nil {
		a.start = append(a.start, func(ctx context.Context) error {
			goscript.SetBroker(b)
			context.AfterFunc(ctx, func() { goscript.SetBroker(nil) })
			return nil
		})
	}
	a.publicEnv = publicEnv(os.Environ())
	if m.Locales != nil {
		cat, err := i18n.Load(m.Locales, m.DefaultLocale)
		if err != nil {
			return nil, fmt.Errorf("kit: %w", err)
		}
		a.i18n = cat
	}
	o = a.opts // plugins may have added hooks, middleware and matchers

	list := make([]*routes.Route, 0, len(m.Routes))
	for i := range m.Routes {
		def := &m.Routes[i]
		segs, err := routes.ParseID(def.ID)
		if err != nil {
			return nil, fmt.Errorf("kit: route %s: %w", def.ID, err)
		}
		for _, li := range def.Layouts {
			if li < 0 || li >= len(m.Layouts) {
				return nil, fmt.Errorf("kit: route %s refers to layout %d, which does not exist", def.ID, li)
			}
		}
		r := &routes.Route{ID: def.ID, Segments: segs}
		a.defs[r] = def
		list = append(list, r)
	}
	matchers := make(map[string]routes.Matcher, len(o.matchers))
	for name, fn := range o.matchers {
		matchers[name] = fn
	}
	router, err := routes.NewRouter(list, matchers)
	if err != nil {
		return nil, fmt.Errorf("kit: %w", err)
	}
	a.router = router

	if url := os.Getenv("TERN_DEV_VITE_URL"); url != "" {
		dev, err := newDevServer(url, os.Getenv("TERN_ROOT"), m.ClientEntry, m.BasePath)
		if err != nil {
			return nil, err
		}
		a.dev = dev
		if o.ssr {
			a.ssr = &ssr.Dev{URL: url}
		}
	} else {
		if a.shell, err = parseShell(m.AppHTML); err != nil {
			return nil, err
		}
		if m.Client != nil {
			if a.assets, err = loadAssets(m.Client, m.ClientEntry); err != nil {
				return nil, err
			}
		}
		if o.ssr && m.ServerBundle != "" {
			sopts := ssr.Options{Workers: o.ssrWorkers, Timeout: o.ssrTimeout, Logger: o.logger}
			if m.Live {
				sopts.Funcs = map[string]func(context.Context, []any) (any, error){"__tern_live_initial": liveInitial}
			}
			var renderer ssr.Renderer
			if m.Native != nil && o.native && os.Getenv("TERN_NATIVE") != "0" {
				nopts := ssr.NativeOptions{Verify: os.Getenv("TERN_NATIVE_VERIFY") == "1"}
				native, err := ssr.NewNative(m.Native, m.ServerBundle, sopts, nopts)
				if err != nil {
					return nil, fmt.Errorf("kit: %w", err)
				}
				renderer = native
			} else {
				pool, err := ssr.NewPool(m.ServerBundle, sopts)
				if err != nil {
					return nil, fmt.Errorf("kit: %w", err)
				}
				renderer = pool
			}
			a.ssr = renderer
			// Live components render per user and per moment, so pages
			// with them can't be cached.
			if o.ssrCache > 0 && !m.Live {
				a.ssr = ssr.NewCache(renderer, o.ssrCache)
			}
			if c, ok := renderer.(io.Closer); ok {
				a.closers = append(a.closers, c)
			}
		}
	}

	var h http.Handler = http.HandlerFunc(a.serve)
	if o.csrf {
		cop := http.NewCrossOriginProtection()
		for _, origin := range o.trustedOrigins {
			if err := cop.AddTrustedOrigin(origin); err != nil {
				return nil, fmt.Errorf("kit: trusted origin: %w", err)
			}
		}
		for _, pattern := range o.crossOriginOK {
			cop.AddInsecureBypassPattern(pattern)
		}
		cop.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "Cross-site request blocked", http.StatusForbidden)
		}))
		h = cop.Handler(h)
	}
	for _, mw := range slices.Backward(o.middleware) {
		h = mw(h)
	}
	proxies, err := parseProxies(o.trustedProxies)
	if err != nil {
		return nil, err
	}
	a.handler = proxies.handler(a, h)
	// Work outside requests, such as jobs and background queries, joins
	// the App's tracer when nothing else is the default.
	if t := a.tracer(); t != nil && tracing.Default() == nil {
		tracing.SetDefault(t)
	}
	return a, nil
}

// Close releases the app's resources, such as server-rendering VMs.
// ListenAndServe and Serve call it on shutdown.
func (a *App) Close() error {
	a.beginShutdown()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	errs := []error{a.runShutdown(ctx)}
	for _, c := range a.closers {
		errs = append(errs, c.Close())
	}
	a.closers = nil
	errs = append(errs, logging.Flush(ctx, a.logger))
	if t := a.tracer(); t != nil {
		errs = append(errs, t.Flush(ctx))
	}
	if t := a.tracer(); t != nil && tracing.Default() == t {
		tracing.SetDefault(nil)
	}
	return errors.Join(errs...)
}

// ServeHTTP implements http.Handler.
func (a *App) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	restoreTLS(r)
	a.handler.ServeHTTP(w, r)
}

const dataSuffix = "/__data.json"

func (a *App) serve(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")

	if a.dev != nil && a.dev.handles(r) {
		a.dev.proxy.ServeHTTP(w, r)
		return
	}
	if base := a.m.BasePath; base != "" {
		switch p := r.URL.Path; {
		case p == base || strings.HasPrefix(p, base+"/"):
			r = stripBase(r, base)
		case p == "/" && (r.Method == http.MethodGet || r.Method == http.MethodHead):
			http.Redirect(w, r, base, http.StatusFound)
			return
		default:
			http.NotFound(w, r)
			return
		}
	}
	if a.opts.openAPI != nil && r.URL.Path == a.withBase(a.opts.openAPI.path) && r.Method == http.MethodGet {
		a.serveOpenAPI(w)
		return
	}
	if a.dev != nil && strings.HasPrefix(r.URL.Path, "/__tern/emails") {
		a.emailPreviews(w, r)
		return
	}
	if a.prerendering && r.URL.Path == "/__tern/prerender" {
		a.prerenderList(w, r)
		return
	}
	if a.hasPluginRoutes {
		if h, pattern := a.pluginMux.Handler(r); pattern != "" {
			h.ServeHTTP(w, r)
			return
		}
	}
	safe := r.Method == http.MethodGet || r.Method == http.MethodHead
	if a.assets != nil && safe && a.assets.serve(w, r) {
		return
	}

	path := r.URL.EscapedPath()
	isData := strings.HasSuffix(path, dataSuffix)
	if isData {
		path = strings.TrimSuffix(path, dataSuffix)
		if path == "" {
			path = "/"
		}
	} else if safe && path != "/" && strings.HasSuffix(path, "/") {
		target := strings.TrimRight(path, "/")
		if target == "" {
			target = "/"
		}
		if r.URL.RawQuery != "" {
			target += "?" + r.URL.RawQuery
		}
		http.Redirect(w, r, a.withBase(target), http.StatusPermanentRedirect)
		return
	}

	if !safe {
		r.Body = &bodyLimiter{r: r.Body, limit: a.opts.bodyLimit}
	}

	var reuse string
	if isData && strings.Contains(r.URL.RawQuery, reuseParam) {
		// The reuse list is for Tern; loads see the URL without it.
		q := r.URL.Query()
		reuse = q.Get(reuseParam)
		q.Del(reuseParam)
		u := *r.URL
		u.RawQuery = q.Encode()
		r = r.Clone(r.Context())
		r.URL = &u
	}

	rw := &responseWriter{ResponseWriter: w}
	c := newCtx(a, rw, r)
	c.reuse = reuse
	if a.i18n != nil {
		path = a.i18n.Canonical(path) // /de/projekte matches /lang.optional/projects
	}
	route, params, ok := a.router.Match(path)
	var def *RouteDef
	if ok {
		def = a.defs[route]
		c.routeID = def.ID
		nameRequest(r, def.ID)
		c.params = params
	}
	a.resolveLocale(c)

	for _, hook := range a.opts.hooks {
		if err := a.safely(func() error { return hook(c) }); err != nil {
			a.failRequest(c, def, isData, err)
			return
		}
	}

	if a.hasCtxRoutes && !isData {
		if _, pattern := a.ctxMux.Handler(r); pattern != "" {
			// Through the mux, so the handler sees the pattern's path values.
			a.ctxMux.ServeHTTP(rw, r.WithContext(context.WithValue(r.Context(), ctxKey{}, c)))
			return
		}
	}

	switch {
	case def == nil:
		a.renderError(c, nil, isData, NotFound())
	case isData:
		if def.Page == nil {
			a.writeDataJSON(c, http.StatusNotFound, errorPayload(c, nil, NotFound(), a.dev != nil))
			return
		}
		a.renderPage(c, def, true, http.StatusOK, nil)
	case def.Page != nil && a.wantsPage(r, def):
		if r.Method == http.MethodPost {
			a.runAction(c, def)
			return
		}
		a.renderPage(c, def, false, http.StatusOK, nil)
	default:
		a.runEndpoint(c, def)
	}
}

// prerenderList responds with the paths `tern build` should prerender.
func (a *App) prerenderList(w http.ResponseWriter, req *http.Request) {
	var paths, endpoints, errs []string
	for r, def := range a.defs {
		if !def.Prerender || (def.Page == nil && def.Endpoint[http.MethodGet] == nil) {
			continue
		}
		entries := []map[string]string{nil}
		if len(r.Params()) > 0 {
			if def.Entries == nil {
				errs = append(errs, fmt.Sprintf("route %s has parameters, so it needs an Entries function to be prerendered", def.ID))
				continue
			}
			var err error
			if entries, err = def.Entries(newCtx(a, w, req)); err != nil {
				errs = append(errs, fmt.Sprintf("route %s: Entries: %v", def.ID, err))
				continue
			}
		}
		for _, params := range entries {
			p, err := routes.Build(r.Segments, params)
			if err != nil {
				errs = append(errs, fmt.Sprintf("route %s: %v", def.ID, err))
				continue
			}
			if def.Page == nil {
				if path.Ext(p) == "" {
					errs = append(errs, fmt.Sprintf("route %s: a prerendered endpoint needs a file extension in its URL, such as sitemap.xml, so its type is known", def.ID))
					continue
				}
				endpoints = append(endpoints, p)
				continue
			}
			paths = append(paths, p)
		}
	}
	slices.Sort(paths)
	slices.Sort(endpoints)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string][]string{"paths": slices.Compact(paths), "endpoints": slices.Compact(endpoints), "errors": errs})
}

// wantsPage reports whether a request to a route with a page should be
// handled by the page rather than by an endpoint handler.
func (a *App) wantsPage(r *http.Request, def *RouteDef) bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		_, hasGet := def.Endpoint[http.MethodGet]
		return !hasGet || acceptsHTML(r)
	case http.MethodPost:
		_, hasPost := def.Endpoint[http.MethodPost]
		return !hasPost || r.Header.Get("X-Tern-Action") == "true" || acceptsHTML(r)
	default:
		return false
	}
}

func acceptsHTML(r *http.Request) bool {
	for _, part := range strings.Split(r.Header.Get("Accept"), ",") {
		mt, _, _ := mime.ParseMediaType(strings.TrimSpace(part))
		switch mt {
		case "text/html":
			return true
		case "application/json":
			return false
		}
	}
	return false
}

// failRequest responds to an error raised before a route handler ran.
func (a *App) failRequest(c *Ctx, def *RouteDef, isData bool, err error) {
	var re *RedirectError
	if errors.As(err, &re) {
		a.redirect(c, isData, re)
		return
	}
	if def != nil && def.Page == nil {
		a.endpointError(c, err)
		return
	}
	a.renderError(c, def, isData, err)
}

func (a *App) redirect(c *Ctx, isData bool, re *RedirectError) {
	location := a.withBase(re.Location)
	if isData {
		a.writeDataJSON(c, http.StatusOK, payload{Type: "redirect", Status: re.Status, Location: location})
		return
	}
	c.applyHeaders()
	http.Redirect(c.writer, c.Request, location, re.Status)
}

// publicEnvPrefix marks environment variables that are safe to send to the
// browser.
const publicEnvPrefix = "PUBLIC_"

func publicEnv(environ []string) map[string]string {
	env := map[string]string{}
	for _, kv := range environ {
		k, v, ok := strings.Cut(kv, "=")
		if ok && strings.HasPrefix(k, publicEnvPrefix) {
			env[k] = v
		}
	}
	return env
}

// withBase adds the base path to an absolute app path such as "/login".
// Full URLs, protocol-relative URLs and already-prefixed paths are kept.
func (a *App) withBase(location string) string {
	base := a.m.BasePath
	if base == "" || !strings.HasPrefix(location, "/") || strings.HasPrefix(location, "//") ||
		location == base || strings.HasPrefix(location, base+"/") || strings.HasPrefix(location, base+"?") {
		return location
	}
	if location == "/" || strings.HasPrefix(location, "/?") || strings.HasPrefix(location, "/#") {
		return base + location[1:] // the home page is the bare base path
	}
	return base + location
}

// stripBase returns r with the base path removed from its URL.
func stripBase(r *http.Request, base string) *http.Request {
	r2 := r.Clone(r.Context())
	u := *r.URL
	u.Path = strings.TrimPrefix(u.Path, base)
	if u.Path == "" {
		u.Path = "/"
	}
	if u.RawPath != "" {
		u.RawPath = strings.TrimPrefix(u.RawPath, base)
		if u.RawPath == "" {
			u.RawPath = "/"
		}
	}
	r2.URL = &u
	return r2
}

// runEndpoint dispatches to an exported GET, POST, … handler.
func (a *App) runEndpoint(c *Ctx, def *RouteDef) {
	method := c.Request.Method
	h, ok := def.Endpoint[method]
	if !ok && method == http.MethodHead {
		h, ok = def.Endpoint[http.MethodGet]
	}
	if !ok {
		allow := slices.Sorted(func(yield func(string) bool) {
			for m := range def.Endpoint {
				if !yield(m) {
					return
				}
			}
		})
		if def.Page != nil {
			allow = append(allow, http.MethodGet)
			if def.Page.Actions != nil {
				allow = append(allow, http.MethodPost)
			}
		}
		if slices.Contains(allow, http.MethodGet) && !slices.Contains(allow, http.MethodHead) {
			allow = append(allow, http.MethodHead)
		}
		slices.Sort(allow)
		c.writer.Header().Set("Allow", strings.Join(slices.Compact(allow), ", "))
		a.endpointError(c, Error(http.StatusMethodNotAllowed, method+" is not allowed"))
		return
	}
	if err := inSpan(c, "endpoint "+method, func() error { return a.safely(func() error { return h(c) }) }); err != nil {
		a.endpointError(c, err)
		return
	}
	if !c.writer.(*responseWriter).wrote {
		c.applyHeaders()
		c.writer.WriteHeader(http.StatusNoContent)
	}
}

func (a *App) endpointError(c *Ctx, err error) {
	var re *RedirectError
	if errors.As(err, &re) {
		a.redirect(c, false, re)
		return
	}
	status, msg, expected := classify(err)
	if !expected {
		a.logError(c, err)
		if a.dev != nil {
			msg = err.Error()
		}
	}
	if c.writer.(*responseWriter).wrote {
		return // the handler already started its response
	}
	if acceptsHTML(c.Request) {
		c.applyHeaders()
		http.Error(c.writer, msg, status)
		return
	}
	_ = c.JSON(status, map[string]string{"message": msg})
}

// runAction handles a form submission to a page.
func (a *App) runAction(c *Ctx, def *RouteDef) {
	enhanced := c.Request.Header.Get("X-Tern-Action") == "true"
	fail := func(err error) {
		if enhanced {
			status, msg, expected := classify(err)
			if !expected {
				a.logError(c, err)
				if a.dev != nil {
					msg = err.Error()
				}
			}
			a.writeDataJSON(c, http.StatusOK, payload{Type: "error", Status: status, Error: &errorBody{Message: msg}})
			return
		}
		a.renderError(c, def, false, err)
	}

	if len(def.Page.Actions) == 0 {
		c.writer.Header().Set("Allow", "GET, HEAD")
		fail(Error(http.StatusMethodNotAllowed, "This page has no form actions"))
		return
	}
	name := "default"
	if q := c.Request.URL.RawQuery; strings.HasPrefix(q, "/") {
		name, _, _ = strings.Cut(q[1:], "&")
	}
	action, ok := def.Page.Actions[name]
	if !ok {
		fail(Error(http.StatusNotFound, fmt.Sprintf("No action named %q", name)))
		return
	}
	ct, _, _ := mime.ParseMediaType(c.Request.Header.Get("Content-Type"))
	if ct != "application/x-www-form-urlencoded" && ct != "multipart/form-data" {
		fail(Error(http.StatusUnsupportedMediaType, "Form actions expect form data"))
		return
	}

	var result any
	err := inSpan(c, "action "+name, func() error {
		return a.safely(func() error {
			var err error
			result, err = action(c)
			return err
		})
	})

	var re *RedirectError
	var fe *FailError
	switch {
	case errors.As(err, &re):
		if enhanced {
			a.writeDataJSON(c, http.StatusOK, payload{Type: "redirect", Status: re.Status, Location: a.withBase(re.Location)})
			return
		}
		a.redirect(c, false, re)
	case errors.As(err, &fe):
		if enhanced {
			a.writeDataJSON(c, http.StatusOK, payload{Type: "failure", Status: fe.Status, Data: fe.Data})
			return
		}
		a.renderPage(c, def, false, fe.Status, &formValue{fe.Data})
	case err != nil:
		fail(err)
	case enhanced:
		a.writeDataJSON(c, http.StatusOK, payload{Type: "success", Status: http.StatusOK, Data: result})
	default:
		a.renderPage(c, def, false, http.StatusOK, &formValue{result})
	}
}

// formValue wraps an action result so a nil result still counts as set.
type formValue struct{ v any }

// renderPage runs the page's loads and responds with HTML or JSON.
func (a *App) renderPage(c *Ctx, def *RouteDef, isData bool, status int, form *formValue) {
	var skip map[int]bool
	if isData {
		skip = a.reusable(c, def)
	}
	out := a.runLoads(c, a.loaders(def), a.loadNames(def), skip)
	if out.err != nil {
		var re *RedirectError
		if errors.As(out.err, &re) {
			a.redirect(c, isData, re)
			return
		}
		a.renderErrorWithNodes(c, def, out.nodes, out.values, out.failed, isData, out.err, out.reused...)
		return
	}
	p := payload{Type: "data", Route: def.ID, Params: c.params, Status: status, Nodes: out.nodes, Uses: out.uses, Reused: out.reused, values: out.values}
	if form != nil {
		p.Form = form.v
		p.HasForm = true
	}
	if isData {
		a.writeDataJSON(c, http.StatusOK, p)
		return
	}
	a.writeDocument(c, status, def, p)
}

// reuseParam is the query parameter a client uses to list the layout data
// it already has: a JSON array of {i: layout index, p: the params it read}.
const reuseParam = "x-tern-reuse"

type reuseEntry struct {
	Layout int               `json:"i"`
	Params map[string]string `json:"p"`
}

// reusable decides which of a route's layout loads can be skipped because
// the client's copy of their data is still valid.
func (a *App) reusable(c *Ctx, def *RouteDef) map[int]bool {
	raw := c.reuse
	if raw == "" {
		return nil
	}
	var entries []reuseEntry
	if json.Unmarshal([]byte(raw), &entries) != nil {
		return nil
	}
	held := make(map[int]reuseEntry, len(entries))
	for _, e := range entries {
		held[e.Layout] = e
	}
	skip := map[int]bool{}
	for k, li := range def.Layouts {
		// Reuse is a prefix: once a layout reruns, the ones below it rerun
		// too, since they may read its data.
		e, ok := held[li]
		if !ok {
			break
		}
		same := true
		for name, v := range e.Params {
			if c.params[name] != v {
				same = false
			}
		}
		if !same {
			break
		}
		skip[k] = true
	}
	return skip
}

// loaders returns the load functions for a route's layouts and page, in
// node order. Nodes without a load are nil.
// loadNames names the loads of loaders(def), for their spans: "layout /",
// "layout /blog", then "page".
func (a *App) loadNames(def *RouteDef) []string {
	var names []string
	for _, li := range def.Layouts {
		names = append(names, layoutName(a.m.Layouts[li].Dir))
	}
	if def.Page != nil {
		names = append(names, "page")
	}
	return names
}

func layoutName(dir string) string { return "layout /" + dir }

func (a *App) loaders(def *RouteDef) []LoadFunc {
	var fns []LoadFunc
	for _, li := range def.Layouts {
		fns = append(fns, a.m.Layouts[li].Load)
	}
	if def.Page != nil {
		fns = append(fns, def.Page.Load)
	}
	return fns
}

// renderError renders the nearest error page for err. def may be nil when
// no route matched, in which case the fallback layouts are used.
func (a *App) renderError(c *Ctx, def *RouteDef, isData bool, err error) {
	layouts := a.m.Fallback.Layouts
	if def != nil {
		layouts = def.Layouts
	}
	var fns []LoadFunc
	var names []string
	for _, li := range layouts {
		fns = append(fns, a.m.Layouts[li].Load)
		names = append(names, layoutName(a.m.Layouts[li].Dir))
	}
	out := a.runLoads(c, fns, names, nil)
	nodes, values := out.nodes, out.values
	if out.err != nil {
		// A layout failed while rendering an error: render without layouts.
		a.logLoadErr(c, out.err)
		nodes, values = nil, nil
	}
	a.renderErrorWithNodes(c, def, nodes, values, len(nodes), isData, err)
}

func (a *App) logLoadErr(c *Ctx, err error) {
	if _, _, expected := classify(err); !expected {
		var re *RedirectError
		if !errors.As(err, &re) {
			a.logError(c, err)
		}
	}
}

func (a *App) renderErrorWithNodes(c *Ctx, def *RouteDef, nodes []jsonenc.Verbatim, values []any, failed int, isData bool, err error, reused ...int) {
	p := errorPayload(c, def, err, a.dev != nil)
	for _, k := range reused {
		if k < failed {
			p.Reused = append(p.Reused, k)
		}
	}
	if _, _, expected := classify(err); !expected {
		a.logError(c, err)
	}
	p.Nodes, p.values = nodes, values
	p.Failed = &failed
	if isData {
		a.writeDataJSON(c, http.StatusOK, p)
		return
	}
	a.writeDocument(c, p.Status, def, p)
}

func errorPayload(c *Ctx, def *RouteDef, err error, dev bool) payload {
	status, msg, expected := classify(err)
	if !expected && dev {
		msg = err.Error()
	}
	p := payload{Type: "error", Status: status, Params: c.params, Error: &errorBody{Message: msg}}
	if def != nil {
		p.Route = def.ID
	}
	return p
}

func (a *App) logError(c *Ctx, err error) {
	a.logger.ErrorContext(c.Request.Context(), "unexpected error",
		"method", c.Request.Method,
		"path", c.Request.URL.Path,
		"route", c.routeID,
		"error", err,
	)
}

// safely runs f, turning a panic into an error so one bad request cannot
// take down the server.
func (a *App) safely(f func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(error); ok && errors.Is(e, http.ErrAbortHandler) {
				panic(r)
			}
			err = &panicError{value: r, stack: stack()}
		}
	}()
	return f()
}

// responseWriter records whether a response has been started.
type responseWriter struct {
	http.ResponseWriter
	wrote bool
}

func (w *responseWriter) WriteHeader(status int) {
	w.wrote = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *responseWriter) Write(b []byte) (int, error) {
	w.wrote = true
	return w.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (w *responseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

package kit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Ctx is the request context passed to Load, LayoutLoad, actions,
// endpoints and hooks. It implements context.Context, so it can be passed
// directly to database calls and other APIs that take a context.
type Ctx struct {
	// Request is the incoming request.
	Request *http.Request
	// Locals holds request-scoped values, typically set by a hook (for
	// example the signed-in user) and read by loads and actions.
	Locals map[string]any

	app     *App
	routeID string
	params  map[string]string
	writer  http.ResponseWriter

	mu      sync.Mutex
	header  http.Header
	cookies []*http.Cookie

	// parent returns the merged data of the layouts above the running load.
	parent func() (map[string]json.RawMessage, error)
	// root is the Ctx that owns response state, when this one is a fork.
	root *Ctx
}

var _ context.Context = (*Ctx)(nil)

func newCtx(app *App, w http.ResponseWriter, r *http.Request) *Ctx {
	return &Ctx{
		Request: r,
		Locals:  map[string]any{},
		app:     app,
		params:  map[string]string{},
		writer:  w,
		header:  http.Header{},
	}
}

// fork returns a copy of c for one load, sharing everything except parent.
func (c *Ctx) fork(parent func() (map[string]json.RawMessage, error)) *Ctx {
	return &Ctx{
		Request: c.Request,
		Locals:  c.Locals,
		app:     c.app,
		routeID: c.routeID,
		params:  c.params,
		writer:  c.writer,
		parent:  parent,
		root:    c.rootCtx(),
	}
}

// Deadline implements context.Context.
func (c *Ctx) Deadline() (time.Time, bool) { return c.Request.Context().Deadline() }

// Done implements context.Context.
func (c *Ctx) Done() <-chan struct{} { return c.Request.Context().Done() }

// Err implements context.Context.
func (c *Ctx) Err() error { return c.Request.Context().Err() }

// Value implements context.Context.
func (c *Ctx) Value(key any) any { return c.Request.Context().Value(key) }

// URL returns the request URL.
func (c *Ctx) URL() *url.URL { return c.Request.URL }

// RouteID returns the matched route's ID, e.g. "/blog/slug.param".
func (c *Ctx) RouteID() string { return c.routeID }

// Param returns the value of a route parameter, or "" if it is absent.
func (c *Ctx) Param(name string) string { return c.params[name] }

// Params returns all route parameters. The map must not be modified.
func (c *Ctx) Params() map[string]string { return c.params }

// Header returns a request header value.
func (c *Ctx) Header(name string) string { return c.Request.Header.Get(name) }

// SetHeader sets a response header. Loads may call it concurrently.
func (c *Ctx) SetHeader(name, value string) {
	r := c.rootCtx()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.header.Set(name, value)
}

// Cookie returns the value of a request cookie.
func (c *Ctx) Cookie(name string) (string, bool) {
	ck, err := c.Request.Cookie(name)
	if err != nil {
		return "", false
	}
	return ck.Value, true
}

// SetCookie adds a Set-Cookie header. Unset fields get safe defaults:
// Path "/", SameSite=Lax, and Secure unless the request is plain HTTP to
// localhost. Set HttpOnly yourself for cookies scripts must not read, such
// as session cookies. Loads may call it concurrently.
func (c *Ctx) SetCookie(ck *http.Cookie) {
	cp := *ck
	if cp.Path == "" {
		cp.Path = "/"
	}
	if cp.SameSite == 0 || cp.SameSite == http.SameSiteDefaultMode {
		cp.SameSite = http.SameSiteLaxMode
	}
	if !cp.Secure && !isLocalHTTP(c.Request) {
		cp.Secure = true
	}
	r := c.rootCtx()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cookies = append(r.cookies, &cp)
}

// DeleteCookie removes a cookie set with the default path "/".
func (c *Ctx) DeleteCookie(name string) {
	c.SetCookie(&http.Cookie{Name: name, Value: "", MaxAge: -1, HttpOnly: true})
}

// Form parses and returns the submitted form, including multipart forms.
func (c *Ctx) Form() (url.Values, error) {
	ct, _, _ := mime.ParseMediaType(c.Request.Header.Get("Content-Type"))
	var err error
	if ct == "multipart/form-data" {
		err = c.Request.ParseMultipartForm(c.app.opts.multipartMemory)
	} else {
		err = c.Request.ParseForm()
	}
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			return nil, Error(http.StatusRequestEntityTooLarge, "Request body too large")
		}
		return nil, Error(http.StatusBadRequest, "Invalid form data")
	}
	return c.Request.PostForm, nil
}

// Writer returns the response writer for endpoints, with headers and
// cookies set through the Ctx already applied. Do not use it in loads.
func (c *Ctx) Writer() http.ResponseWriter {
	c.applyHeaders()
	return c.writer
}

// JSON writes v as a JSON response. It is meant for endpoints.
func (c *Ctx) JSON(status int, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("kit: encoding JSON response: %w", err)
	}
	w := c.Writer()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, err = w.Write(b)
	return err
}

// Text writes a plain-text response. It is meant for endpoints.
func (c *Ctx) Text(status int, s string) error {
	w := c.Writer()
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, err := w.Write([]byte(s))
	return err
}

// root points at the Ctx that owns response state; forks share it.
func (c *Ctx) rootCtx() *Ctx {
	if c.root != nil {
		return c.root
	}
	return c
}

// applyHeaders copies headers and cookies collected so far to the response.
func (c *Ctx) applyHeaders() {
	r := c.rootCtx()
	r.mu.Lock()
	defer r.mu.Unlock()
	h := c.writer.Header()
	for k, vs := range r.header {
		h[k] = vs
	}
	for _, ck := range r.cookies {
		if v := ck.String(); v != "" {
			h.Add("Set-Cookie", v)
		}
	}
	r.header = http.Header{}
	r.cookies = nil
}

// Parent waits for the layouts above the current load, merges their data,
// and decodes it into T. Use it in Load or LayoutLoad to build on data a
// parent layout loaded:
//
//	type Parent struct{ User User `json:"user"` }
//	p, err := kit.Parent[Parent](ctx)
//
// Loads that do not call Parent run fully in parallel.
func Parent[T any](c *Ctx) (T, error) {
	var out T
	if c.parent == nil {
		return out, errors.New("kit.Parent: only available in Load and LayoutLoad")
	}
	merged, err := c.parent()
	if err != nil {
		return out, err
	}
	b, err := json.Marshal(merged)
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return out, fmt.Errorf("kit.Parent: decoding parent data: %w", err)
	}
	return out, nil
}

func isLocalHTTP(r *http.Request) bool {
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		return false
	}
	host := r.Host
	if i := strings.LastIndexByte(host, ':'); i >= 0 && !strings.HasSuffix(host, "]") {
		host = host[:i]
	}
	host = strings.Trim(host, "[]")
	return host == "localhost" || host == "127.0.0.1" || host == "::1" || strings.HasSuffix(host, ".localhost")
}

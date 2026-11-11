// Package kittest tests Tern apps from Go: call a load, an action or an
// endpoint with a request made up for the test, and check what it returns;
// or run the whole app and query the HTML its pages render.
//
// A load, as a signed-in user, against a database made for the test:
//
//	func TestProjects(t *testing.T) {
//		db := kittest.SQLite(t, migrations.FS)
//		acc := ... // accounts.New(db, …)
//		app := kittest.App(t, kit.WithPlugins(acc))
//		c := kittest.Ctx(t, app, kittest.Path("/projects"))
//		acc.ActAs(c, user)
//		data, err := projects.Load(c)
//		...
//	}
//
// The whole app, in a test in package main, with its generated ternApp:
//
//	func TestHome(t *testing.T) {
//		app, err := ternApp()
//		...
//		client := kittest.NewClient(t, app)
//		page := client.Get("/")
//		if page.Status != 200 || page.Text("h1") != "Welcome" { ... }
//	}
package kittest

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/Unfield/Tern/kit"
	"github.com/Unfield/Tern/migrate"
)

// shell is the HTML of apps made with App, which have no app.html.
const shell = `<!doctype html><html><head>%tern.head%</head><body>%tern.body%</body></html>`

// App returns an App with no routes, for calling loads, actions and
// endpoints with Ctx. Pass the options the real app uses, its plugins
// among them, so their hooks run. It is closed when the test ends.
func App(t testing.TB, opts ...kit.Option) *kit.App {
	t.Helper()
	a, err := kit.New(&kit.Manifest{AppHTML: shell}, opts...)
	if err != nil {
		t.Fatalf("kittest: %v", err)
	}
	t.Cleanup(func() { _ = a.Close() })
	return a
}

// Option describes the request of a Ctx.
type Option func(*request)

type request struct {
	method  string
	target  string
	body    io.Reader
	ctype   string
	header  http.Header
	cookies []*http.Cookie
	opts    kit.CtxOptions
	ctx     context.Context
}

// Method sets the request's method. The default is GET, or POST with a
// form or JSON body.
func Method(m string) Option { return func(r *request) { r.method = m } }

// Path sets the request's path and query: "/blog/hello?page=2". The
// default is "/".
func Path(p string) Option { return func(r *request) { r.target = p } }

// Form sends values as a form, as an action gets them.
func Form(values url.Values) Option {
	return func(r *request) {
		r.body, r.ctype = strings.NewReader(values.Encode()), "application/x-www-form-urlencoded"
		if r.method == "" {
			r.method = http.MethodPost
		}
	}
}

// JSON sends v as a JSON body, as a typed endpoint gets it.
func JSON(v any) Option {
	return func(r *request) {
		b, err := json.Marshal(v)
		if err != nil {
			panic(fmt.Sprintf("kittest.JSON: %v", err))
		}
		r.body, r.ctype = bytes.NewReader(b), "application/json"
		if r.method == "" {
			r.method = http.MethodPost
		}
	}
}

// Header sets a request header.
func Header(name, value string) Option {
	return func(r *request) { r.header.Set(name, value) }
}

// Cookie adds a cookie to the request.
func Cookie(c *http.Cookie) Option { return func(r *request) { r.cookies = append(r.cookies, c) } }

// Route sets the route's ID and parameters, as the route's Load gets them:
// Route("/blog/slug.param", "slug", "hello").
func Route(id string, params ...string) Option {
	return func(r *request) {
		r.opts.Route = id
		if r.opts.Params == nil {
			r.opts.Params = map[string]string{}
		}
		for i := 0; i+1 < len(params); i += 2 {
			r.opts.Params[params[i]] = params[i+1]
		}
	}
}

// Parent sets the layouts' data that kit.Parent decodes.
func Parent(v any) Option { return func(r *request) { r.opts.Parent = v } }

// Local sets a value in the Ctx's Locals.
func Local(key string, v any) Option {
	return func(r *request) {
		if r.opts.Locals == nil {
			r.opts.Locals = map[string]any{}
		}
		r.opts.Locals[key] = v
	}
}

// NoHooks skips the App's hooks.
func NoHooks() Option { return func(r *request) { r.opts.NoHooks = true } }

// Context sets the request's context, for deadlines or values of your own.
func Context(ctx context.Context) Option { return func(r *request) { r.ctx = ctx } }

// Ctx returns the Ctx of a request made up of opts, with app's hooks run:
// what a load, action or endpoint gets. Its response is a recorder:
// Response returns it.
func Ctx(t testing.TB, app *kit.App, opts ...Option) *kit.Ctx {
	t.Helper()
	r := &request{target: "/", header: http.Header{}}
	for _, o := range opts {
		o(r)
	}
	if r.method == "" {
		r.method = http.MethodGet
	}
	req := httptest.NewRequest(r.method, r.target, r.body)
	if r.ctx != nil {
		req = req.WithContext(r.ctx)
	}
	if r.ctype != "" {
		req.Header.Set("Content-Type", r.ctype)
	}
	for k, vs := range r.header {
		req.Header[k] = vs
	}
	for _, c := range r.cookies {
		req.AddCookie(c)
	}
	w := httptest.NewRecorder()
	c, err := app.NewCtx(w, req, r.opts)
	if err != nil {
		t.Fatalf("kittest: a hook failed: %v", err)
	}
	recorders.Store(c, w)
	return c
}

// recorders keeps each Ctx's response recorder, for Response.
var recorders syncMap

// Response is the response written to a Ctx from Ctx so far, with the
// headers and cookies it set.
func Response(c *kit.Ctx) *httptest.ResponseRecorder {
	c.Flush()
	if w, ok := recorders.Load(c); ok {
		return w
	}
	return nil
}

// Redirected reports whether err redirects, and where.
func Redirected(err error) (status int, location string, ok bool) {
	var re *kit.RedirectError
	if errors.As(err, &re) {
		return re.Status, re.Location, true
	}
	return 0, "", false
}

// Failed reports whether err is an action's Fail, with its data.
func Failed(err error) (status int, data any, ok bool) {
	var fe *kit.FailError
	if errors.As(err, &fe) {
		return fe.Status, fe.Data, true
	}
	return 0, nil, false
}

// Status is the HTTP status err responds with: 200 for nil, the status of
// kit.Error, Redirect and Fail, and 500 for anything else.
func Status(err error) int {
	if err == nil {
		return http.StatusOK
	}
	var re *kit.RedirectError
	var fe *kit.FailError
	var he *kit.HTTPError
	switch {
	case errors.As(err, &re):
		return re.Status
	case errors.As(err, &fe):
		return fe.Status
	case errors.As(err, &he):
		return he.Status
	}
	return http.StatusInternalServerError
}

var dbSeq atomic.Int64

// SQLite returns an empty SQLite database in memory for this test, with
// the migrations in each fsys applied (see package migrate). It is closed
// when the test ends; every test gets its own.
func SQLite(t testing.TB, migrations ...fs.FS) *sql.DB {
	t.Helper()
	name := fmt.Sprintf("file:kittest-%d?mode=memory&cache=shared&_pragma=foreign_keys(1)", dbSeq.Add(1))
	db, err := sql.Open("sqlite", name)
	if err != nil {
		t.Fatalf("kittest: %v", err)
	}
	// One connection: the in-memory database lives as long as it does.
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	for _, fsys := range migrations {
		if _, err := migrate.Up(context.Background(), db, fsys); err != nil {
			t.Fatalf("kittest: migrating: %v", err)
		}
	}
	return db
}

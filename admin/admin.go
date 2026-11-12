// Package admin generates screens to manage an app's data from Go structs:
// a list with search, filters, sorting and pages for each kind of record,
// and a form to create, edit and delete one, checked against the struct's
// validate tags.
//
// Describe each kind of record with a struct, and keep it in a Table, or in
// a Store of your own:
//
//	type Link struct {
//		ID      int64     `admin:"key"`
//		Title   string    `validate:"required,max=200" admin:"list,search"`
//		URL     string    `validate:"required,url" admin:"list,search"`
//		Read    bool      `admin:"list,filter"`
//		AddedAt time.Time `admin:"list,readonly"`
//	}
//
//	links, err := admin.NewTable[Link](db, "links", "")
//	panel := admin.New(admin.Options{Authorize: onlyAdmins},
//		admin.Resource("links", links),
//	)
//
// Add the panel to the app with kit.WithPlugins, and give it a route, such
// as src/routes/admin/path.rest:
//
//	// page.server.go
//	func Load(c *kit.Ctx) (admin.Page, error) { return admin.Load(c) }
//
//	var Actions = admin.Actions
//
//	<!-- +page.svelte -->
//	<script lang="ts">
//		import { Admin } from '@ternkit/tern/ui';
//		let { data, form } = $props();
//	</script>
//	<Admin {data} {form} />
//
// The admin tag takes key, list, search, filter, readonly, textarea, date
// and ref=resource; "-" leaves a field out. Fields take their label, help
// and rules from the label, description and validate tags, as forms do.
package admin

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"

	"github.com/Unfield/Tern/internal/formrules"
	"github.com/Unfield/Tern/kit"
)

// Options configure a panel.
type Options struct {
	// Title is the panel's name, shown above its menu. Default: "Admin".
	Title string
	// Path is where its route is. Default: "/admin".
	Path string
	// Authorize runs before every screen and action, and returns an error,
	// such as a redirect to sign in or kit.Error(403, ""), for anyone who
	// may not use the panel. It's required: a panel without it would let
	// anyone change everything.
	Authorize func(c *kit.Ctx) error
	// PerPage is how many records a list shows at a time. Default: 25.
	PerPage int
}

// Admin is a panel of screens. It's a kit.Plugin.
type Admin struct {
	opts      Options
	resources []*resource
	err       error
}

// A resource is one kind of record.
type resource struct {
	name, label, singular string
	fields                []field
	store                 any
	list                  func(ctx context.Context, q Query) ([]reflect.Value, int, error)
	get                   func(ctx context.Context, key string) (reflect.Value, error)
	create                func(ctx context.Context, v reflect.Value) (string, error)
	update                func(ctx context.Context, key string, v reflect.Value, cols []string) error
	remove                func(ctx context.Context, key string) error
	typ                   reflect.Type
	readOnly, noCreate    bool
	noDelete              bool
	afterSave, afterDel   func(ctx context.Context, key string) error
	actions               []action
}

// action is a button on a record's page.
type action struct {
	name, label string
	danger      bool
	fn          func(ctx context.Context, key string) error
}

// An Option changes how a resource is shown.
type Option func(*resource)

// Label names the resource in the menu and its screens, such as
// Label("People", "Person"). The default comes from its name.
func Label(plural, singular string) Option {
	return func(r *resource) { r.label, r.singular = plural, singular }
}

// ReadOnly shows the records without a way to create, edit or delete them.
func ReadOnly() Option { return func(r *resource) { r.readOnly = true } }

// NoCreate leaves out creating records.
func NoCreate() Option { return func(r *resource) { r.noCreate = true } }

// NoDelete leaves out deleting records.
func NoDelete() Option { return func(r *resource) { r.noDelete = true } }

// AfterSave runs fn after a record is created or saved, with its key,
// such as to update a search index or a cache. An error fails the
// request, though the record stays saved.
func AfterSave(fn func(ctx context.Context, key string) error) Option {
	return func(r *resource) { r.afterSave = fn }
}

// AfterDelete runs fn after a record is deleted, with its key.
func AfterDelete(fn func(ctx context.Context, key string) error) Option {
	return func(r *resource) { r.afterDel = fn }
}

// Action adds a button to each record's page that runs fn with the
// record's key, such as "Retry" for a failed job or "Sign out everywhere"
// for a user. An error made with kit.FormError shows on the page; others
// fail the request.
func Action(name, label string, fn func(ctx context.Context, key string) error) Option {
	return func(r *resource) { r.actions = append(r.actions, action{name: name, label: label, fn: fn}) }
}

// DangerAction is an Action that asks first, for what can't be undone.
func DangerAction(name, label string, fn func(ctx context.Context, key string) error) Option {
	return func(r *resource) {
		r.actions = append(r.actions, action{name: name, label: label, fn: fn, danger: true})
	}
}

// Res is a resource for New; make one with Resource.
type Res struct{ r *resource }

// Resource is a kind of record, named for its address, such as "links",
// kept in store.
func Resource[T any](name string, store Store[T], opts ...Option) Res {
	r := &resource{name: name, store: store, typ: reflect.TypeFor[T]()}
	fields, err := fieldsOf(r.typ)
	if err != nil {
		panic(err)
	}
	r.fields = fields
	r.label = formrules.Humanize(name)
	r.singular = strings.TrimSuffix(r.label, "s")
	r.list = func(ctx context.Context, q Query) ([]reflect.Value, int, error) {
		recs, n, err := store.List(ctx, q)
		out := make([]reflect.Value, len(recs))
		for i := range recs {
			out[i] = reflect.ValueOf(&recs[i]).Elem()
		}
		return out, n, err
	}
	r.get = func(ctx context.Context, key string) (reflect.Value, error) {
		rec, err := store.Get(ctx, key)
		return reflect.ValueOf(&rec).Elem(), err
	}
	r.create = func(ctx context.Context, v reflect.Value) (string, error) {
		return store.Create(ctx, v.Interface().(T))
	}
	r.update = func(ctx context.Context, key string, v reflect.Value, cols []string) error {
		return store.Update(ctx, key, v.Interface().(T), cols)
	}
	r.remove = store.Delete
	for _, o := range opts {
		o(r)
	}
	return Res{r}
}

// New returns a panel with these resources, shown in this order.
func New(opts Options, resources ...Res) *Admin {
	if opts.Title == "" {
		opts.Title = "Admin"
	}
	if opts.Path == "" {
		opts.Path = "/admin"
	}
	opts.Path = "/" + strings.Trim(opts.Path, "/")
	if opts.PerPage <= 0 {
		opts.PerPage = 25
	}
	a := &Admin{opts: opts}
	seen := map[string]bool{}
	for _, r := range resources {
		if seen[r.r.name] || r.r.name == "" || strings.Contains(r.r.name, "/") {
			a.err = fmt.Errorf("admin: resource name %q is empty, taken or has a slash", r.r.name)
		}
		seen[r.r.name] = true
		a.resources = append(a.resources, r.r)
	}
	for _, r := range a.resources {
		for _, f := range r.fields {
			if f.ref != "" && !seen[f.ref] {
				a.err = fmt.Errorf("admin: %s.%s refers to %q, which isn't a resource", r.name, f.column, f.ref)
			}
		}
	}
	return a
}

// Name implements kit.Plugin.
func (a *Admin) Name() string { return "admin" }

const localsKey = "tern.admin"

// Setup implements kit.Plugin.
func (a *Admin) Setup(s *kit.Setup) error {
	if a.err != nil {
		return a.err
	}
	if a.opts.Authorize == nil {
		return errors.New("admin: Options.Authorize is required")
	}
	s.Hook(func(c *kit.Ctx) error {
		c.Locals[localsKey] = a
		return nil
	})
	return nil
}

func from(c *kit.Ctx) (*Admin, error) {
	a, ok := c.Locals[localsKey].(*Admin)
	if !ok {
		return nil, errors.New("admin: the panel isn't one of the app's plugins; add it with kit.WithPlugins")
	}
	return a, nil
}

// Page is what a panel's route loads.
type Page struct {
	Title     string `json:"title"`
	Base      string `json:"base"`
	Resources []Nav  `json:"resources"`
	// View is "home", "list" or "form".
	View     string  `json:"view"`
	Resource *Info   `json:"resource"`
	List     *List   `json:"list"`
	Record   *Record `json:"record"`
	Counts   []int   `json:"counts"`
	Notice   string  `json:"notice"`
}

// Nav is a resource in the menu.
type Nav struct {
	Name  string `json:"name"`
	Label string `json:"label"`
	Href  string `json:"href"`
}

// Info describes the resource on screen.
type Info struct {
	Name     string  `json:"name"`
	Label    string  `json:"label"`
	Singular string  `json:"singular"`
	Href     string  `json:"href"`
	Key      string  `json:"key"`
	Fields   []Field `json:"fields"`
	Search   bool    `json:"search"`
	Create   bool    `json:"create"`
	Edit     bool    `json:"edit"`
	Delete   bool    `json:"delete"`
	// Actions are the buttons on a record's page.
	Actions []ActionInfo `json:"actions"`
}

// ActionInfo is a button on a record's page.
type ActionInfo struct {
	Name   string `json:"name"`
	Label  string `json:"label"`
	Danger bool   `json:"danger"`
}

// Field is a field on screen.
type Field struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Description string `json:"description"`
	// Input is the control: text, textarea, number, checkbox, date,
	// datetime-local, email, url, password or select.
	Input    string   `json:"input"`
	Options  []string `json:"options"`
	List     bool     `json:"list"`
	Filter   bool     `json:"filter"`
	ReadOnly bool     `json:"readonly"`
	// Ref is the resource this field holds a key of, linked from the list.
	Ref string `json:"ref"`
	// Rules are the field's validate rules, for the browser to check.
	Rules formrules.Field `json:"rules"`
}

// List is a page of records.
type List struct {
	Rows    []Row             `json:"rows"`
	Total   int               `json:"total"`
	Page    int               `json:"page"`
	Pages   int               `json:"pages"`
	PerPage int               `json:"perPage"`
	Search  string            `json:"search"`
	Sort    string            `json:"sort"`
	Desc    bool              `json:"desc"`
	Filters map[string]string `json:"filters"`
}

// Row is a record in a list: its key, and each listed field's value, as a
// form would post it.
type Row struct {
	Key    string            `json:"key"`
	Values map[string]string `json:"values"`
}

// Record is the record being created or edited.
type Record struct {
	// Key is "" for a new record.
	Key    string            `json:"key"`
	Values map[string]string `json:"values"`
}

func (a *Admin) find(name string) *resource {
	for _, r := range a.resources {
		if r.name == name {
			return r
		}
	}
	return nil
}

// route splits the route's path into the resource and the record's key,
// which is "new" for a new one.
func (a *Admin) route(c *kit.Ctx) (r *resource, key string, err error) {
	path := strings.Trim(c.Param("path"), "/")
	if path == "" {
		return nil, "", nil
	}
	name, key, _ := strings.Cut(path, "/")
	if r = a.find(name); r == nil || strings.Contains(key, "/") {
		return nil, "", kit.NotFound()
	}
	if key, err = url.PathUnescape(key); err != nil {
		return nil, "", kit.NotFound()
	}
	if key == "new" && (r.readOnly || r.noCreate) {
		return nil, "", kit.NotFound()
	}
	return r, key, nil
}

func (a *Admin) href(c *kit.Ctx, parts ...string) string {
	p := a.opts.Path
	for _, s := range parts {
		p += "/" + url.PathEscape(s)
	}
	return c.Path(p)
}

func (a *Admin) info(c *kit.Ctx, r *resource) *Info {
	editable := false
	for _, f := range r.fields {
		if !f.readonly && !f.key {
			editable = true
		}
	}
	in := &Info{
		Name: r.name, Label: r.label, Singular: r.singular, Href: a.href(c, r.name),
		Create: !r.readOnly && !r.noCreate, Edit: !r.readOnly && editable, Delete: !r.readOnly && !r.noDelete,
		Actions: []ActionInfo{},
	}
	for _, act := range r.actions {
		in.Actions = append(in.Actions, ActionInfo{Name: act.name, Label: act.label, Danger: act.danger})
	}
	for _, f := range r.fields {
		if f.key {
			in.Key = f.column
		}
		in.Search = in.Search || f.search
		in.Fields = append(in.Fields, Field{
			Name: f.column, Label: f.rules.Label, Description: f.help, Input: f.input, Options: f.options,
			List: f.list, Filter: f.filter, ReadOnly: f.readonly || f.key || r.readOnly, Ref: f.ref, Rules: f.rules,
		})
	}
	return in
}

func (r *resource) keyField() *field {
	for i := range r.fields {
		if r.fields[i].key {
			return &r.fields[i]
		}
	}
	return nil
}

func (r *resource) values(v reflect.Value, listed bool) map[string]string {
	out := map[string]string{}
	for _, f := range r.fields {
		if listed && !f.list || f.rules.Secret {
			continue
		}
		out[f.column] = f.text(v.FieldByIndex(f.index))
	}
	return out
}

// Load is the panel's route's load: it returns the screen for its path.
func Load(c *kit.Ctx) (Page, error) {
	a, err := from(c)
	if err != nil {
		return Page{}, err
	}
	if err := a.opts.Authorize(c); err != nil {
		return Page{}, err
	}
	r, key, err := a.route(c)
	if err != nil {
		return Page{}, err
	}
	p := Page{Title: a.opts.Title, Base: c.Path(a.opts.Path), Resources: []Nav{}, Counts: []int{}, Notice: c.URL().Query().Get("done")}
	for _, r := range a.resources {
		p.Resources = append(p.Resources, Nav{Name: r.name, Label: r.label, Href: a.href(c, r.name)})
	}
	ctx := c.Request.Context()
	switch {
	case r == nil:
		p.View = "home"
		for _, r := range a.resources {
			_, n, err := r.list(ctx, Query{Limit: 0})
			if err != nil {
				return Page{}, err
			}
			p.Counts = append(p.Counts, n)
		}
	case key == "":
		p.View, p.Resource = "list", a.info(c, r)
		if p.List, err = a.list(c, r); err != nil {
			return Page{}, err
		}
	default:
		p.View, p.Resource = "form", a.info(c, r)
		p.Record = &Record{Values: map[string]string{}}
		if key != "new" {
			v, err := r.get(ctx, key)
			if errors.Is(err, ErrNotFound) {
				return Page{}, kit.NotFound()
			}
			if err != nil {
				return Page{}, err
			}
			p.Record = &Record{Key: key, Values: r.values(v, false)}
		}
	}
	return p, nil
}

func (a *Admin) list(c *kit.Ctx, r *resource) (*List, error) {
	qs := c.URL().Query()
	l := &List{Search: strings.TrimSpace(qs.Get("q")), Filters: map[string]string{}, Rows: []Row{}, PerPage: a.opts.PerPage}
	l.Page, _ = strconv.Atoi(qs.Get("page"))
	l.Page = max(l.Page, 1)
	sort := qs.Get("sort")
	l.Desc = strings.HasPrefix(sort, "-")
	sort = strings.TrimPrefix(sort, "-")
	q := Query{Search: l.Search, Filters: map[string]string{}, Limit: a.opts.PerPage, Offset: (l.Page - 1) * a.opts.PerPage}
	for _, f := range r.fields {
		if f.column == sort {
			l.Sort, q.Sort, q.Desc = sort, sort, l.Desc
		}
		if v := qs.Get(f.column); qs.Has(f.column) && (f.filter || f.ref != "") {
			// A filter on a checkbox is "true" or "false".
			if f.rules.Kind == formrules.Bool && v != "true" && v != "false" {
				continue
			}
			q.Filters[f.column] = v
			l.Filters[f.column] = v
		}
	}
	if !searchable(r.fields) {
		q.Search = ""
	}
	recs, total, err := r.list(c.Request.Context(), q)
	if err != nil {
		return nil, err
	}
	l.Total = total
	l.Pages = max(1, int(math.Ceil(float64(total)/float64(a.opts.PerPage))))
	key := r.keyField()
	for _, v := range recs {
		l.Rows = append(l.Rows, Row{Key: key.text(v.FieldByIndex(key.index)), Values: r.values(v, true)})
	}
	return l, nil
}

func searchable(fields []field) bool {
	for _, f := range fields {
		if f.search {
			return true
		}
	}
	return false
}

// Actions are the panel's route's actions: save and delete.
var Actions = kit.Actions{
	"save":   save,
	"delete": remove,
	"do":     do,
}

// do runs one of a record's actions, named by the form's action field.
func do(c *kit.Ctx) (any, error) {
	a, err := from(c)
	if err != nil {
		return nil, err
	}
	if err := a.opts.Authorize(c); err != nil {
		return nil, err
	}
	r, key, err := a.route(c)
	if err != nil {
		return nil, err
	}
	if r == nil || key == "" || key == "new" {
		return nil, kit.Error(http.StatusMethodNotAllowed, "")
	}
	form, err := c.Form()
	if err != nil {
		return nil, err
	}
	name := form.Get("action")
	for _, act := range r.actions {
		if act.name != name {
			continue
		}
		if _, err := r.get(c.Request.Context(), key); errors.Is(err, ErrNotFound) {
			return nil, kit.NotFound()
		} else if err != nil {
			return nil, err
		}
		err := act.fn(c.Request.Context(), key)
		if fe, ok := errors.AsType[kit.FieldErrors](err); ok {
			return nil, kit.Fail(http.StatusBadRequest, kit.Invalid{Errors: map[string]string{}, Values: map[string]any{}, Message: fe.Error()})
		}
		if err != nil {
			return nil, err
		}
		return nil, kit.Redirect(http.StatusSeeOther, a.href(c, r.name, key)+"?done="+url.QueryEscape(act.name))
	}
	return nil, kit.NotFound()
}

func (a *Admin) editable(c *kit.Ctx) (*Admin, *resource, string, error) {
	if err := a.opts.Authorize(c); err != nil {
		return nil, nil, "", err
	}
	r, key, err := a.route(c)
	if err != nil {
		return nil, nil, "", err
	}
	if r == nil || key == "" || r.readOnly {
		return nil, nil, "", kit.Error(http.StatusMethodNotAllowed, "")
	}
	return a, r, key, nil
}

func save(c *kit.Ctx) (any, error) {
	a, err := from(c)
	if err != nil {
		return nil, err
	}
	a, r, key, err := a.editable(c)
	if err != nil {
		return nil, err
	}
	form, err := c.Form()
	if err != nil {
		return nil, err
	}
	creating := key == "new"
	ctx := c.Request.Context()
	v := reflect.New(r.typ).Elem()
	if !creating {
		if v, err = r.get(ctx, key); errors.Is(err, ErrNotFound) {
			return nil, kit.NotFound()
		} else if err != nil {
			return nil, err
		}
	}
	// The fields this form edits: all but those read only, and the key
	// once it's set, or when the database sets it.
	var edited []*field
	labels := map[string]string{}
	for i := range r.fields {
		f := &r.fields[i]
		labels[f.column] = f.rules.Label
		if f.readonly || f.key && (!creating || f.rules.Kind != formrules.String) {
			continue
		}
		// A password left empty when editing stays as it was.
		if f.rules.Secret && !creating && strings.TrimSpace(form.Get(f.column)) == "" {
			continue
		}
		edited = append(edited, f)
	}
	errs := map[string]string{}
	for _, f := range edited {
		if m := formrules.Check(f.rules, form[f.column], form.Get, func(n string) string { return labels[n] }, c.TB); m != "" {
			errs[f.column] = m
		} else if err := f.set(v.FieldByIndex(f.index), form.Get(f.column)); err != nil {
			// Such as a number too big for its type.
			errs[f.column] = c.TB("tern.validate.number", map[string]any{"field": f.rules.Label, "fieldLower": strings.ToLower(f.rules.Label)})
		}
	}
	if len(errs) == 0 {
		if val, ok := v.Addr().Interface().(kit.Validator); ok {
			err = val.Validate()
		}
		if fe, ok := errors.AsType[kit.FieldErrors](err); ok {
			errs = fe
		} else if err != nil {
			return nil, err
		}
	}
	if len(errs) > 0 {
		return nil, kit.Fail(http.StatusBadRequest, invalid(r, form, errs, ""))
	}
	cols := make([]string, len(edited))
	for i, f := range edited {
		cols[i] = f.column
	}
	done := "saved"
	if creating {
		key, err = r.create(ctx, v)
		done = "created"
	} else {
		err = r.update(ctx, key, v, cols)
	}
	if err == nil && r.afterSave != nil {
		err = r.afterSave(ctx, key)
	}
	if errors.Is(err, ErrNotFound) {
		return nil, kit.NotFound()
	}
	if fe, ok := errors.AsType[kit.FieldErrors](err); ok {
		return nil, kit.Fail(http.StatusBadRequest, invalid(r, form, fe, ""))
	}
	if unique(err) {
		return nil, kit.Fail(http.StatusConflict, invalid(r, form, nil, c.TB("tern.admin.error.unique", map[string]any{"name": strings.ToLower(r.singular)})))
	}
	if err != nil {
		return nil, err
	}
	return nil, kit.Redirect(http.StatusSeeOther, a.href(c, r.name)+"?done="+done)
}

func remove(c *kit.Ctx) (any, error) {
	a, err := from(c)
	if err != nil {
		return nil, err
	}
	a, r, key, err := a.editable(c)
	if err != nil {
		return nil, err
	}
	if r.noDelete || key == "new" {
		return nil, kit.Error(http.StatusMethodNotAllowed, "")
	}
	err = r.remove(c.Request.Context(), key)
	if err == nil && r.afterDel != nil {
		err = r.afterDel(c.Request.Context(), key)
	}
	switch {
	case errors.Is(err, ErrNotFound):
		return nil, kit.NotFound()
	case foreign(err):
		return nil, kit.Fail(http.StatusConflict, kit.Invalid{Errors: map[string]string{}, Values: map[string]any{}, Message: c.TB("tern.admin.error.referenced", map[string]any{"name": strings.ToLower(r.singular)})})
	case err != nil:
		return nil, err
	}
	return nil, kit.Redirect(http.StatusSeeOther, a.href(c, r.name)+"?done=deleted")
}

func invalid(r *resource, form url.Values, errs map[string]string, msg string) kit.Invalid {
	out := kit.Invalid{Errors: map[string]string{}, Values: map[string]any{}, Message: msg}
	for k, m := range errs {
		if k == "" {
			out.Message = m
		} else {
			out.Errors[k] = m
		}
	}
	for _, f := range r.fields {
		if !f.rules.Secret && form.Has(f.column) {
			out.Values[f.column] = form.Get(f.column)
		}
	}
	return out
}

// unique reports whether err is a unique constraint failing, in SQLite or
// PostgreSQL, whose drivers say so only in their messages.
func unique(err error) bool {
	if err == nil {
		return false
	}
	m := err.Error()
	return strings.Contains(m, "UNIQUE constraint failed") || strings.Contains(m, "SQLSTATE 23505") || strings.Contains(m, "duplicate key value")
}

// foreign reports whether err is a foreign key failing.
func foreign(err error) bool {
	if err == nil {
		return false
	}
	m := err.Error()
	return strings.Contains(m, "FOREIGN KEY constraint failed") || strings.Contains(m, "SQLSTATE 23503") || strings.Contains(m, "violates foreign key constraint")
}

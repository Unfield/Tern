// Package routes discovers Tern routes on the filesystem and matches URLs
// against them.
//
// Every directory under the routes root is a URL segment. Svelte files
// starting with "+" define the UI, and the Go package in the directory, if
// any, defines the server side:
//
//	+page.svelte      page component
//	+layout.svelte    layout wrapping this directory and its children
//	+error.svelte     error boundary
//	*.go              Go package; exports Load and Actions (page),
//	                  LayoutLoad (layout) and GET, POST, … (endpoint)
//
// Go import paths cannot contain brackets or parentheses, so dynamic
// segments use suffixes: slug.param, id.param.integer, lang.optional,
// path.rest and app.group.
package routes

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path"
	"slices"
	"strconv"
	"strings"
)

// Svelte route file names.
const (
	FilePage   = "+page.svelte"
	FileLayout = "+layout.svelte"
	FileError  = "+error.svelte"
	// FilePageMarkdown is a page written in Markdown; tern gen renders it
	// into a Svelte component.
	FilePageMarkdown = "+page.md"
)

var svelteFiles = []string{FilePage, FileLayout, FileError}

// HTTPMethods are the endpoint handler names a route package may export.
var HTTPMethods = []string{"GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}

// Route is a URL pattern backed by a page, an endpoint, or both.
// All file paths are slash-separated and relative to the routes root.
type Route struct {
	// ID identifies the route by its directory, including groups,
	// e.g. "/app.group/blog/slug.param". The root route's ID is "/".
	ID string
	// Dir is the route's directory, or "" for the root.
	Dir string
	// Segments are the URL segments, with groups removed.
	Segments []Segment
	// Page is the +page.svelte file, or "".
	Page string
	// Go is the route directory's Go package, or nil.
	Go *GoPackage
	// Layouts that wrap the page, outermost first.
	Layouts []Layout
	// ErrorPages are the +error.svelte files that apply, outermost first.
	ErrorPages []string
	// Access is the package whose `var Access` applies to the route: its
	// own, or a directory above's SignedIn or Permission rule; nil when
	// none does.
	Access *GoPackage
}

// GoPackage describes the Go package in a route directory.
type GoPackage struct {
	Dir   string
	Name  string   // package name
	Files []string // non-test .go files
	// Load reports whether the package exports a page Load function.
	Load bool
	// Actions reports whether the package exports a page Actions variable.
	Actions bool
	// LayoutLoad reports whether the package exports a layout load function.
	LayoutLoad bool
	// Methods lists the exported endpoint handlers, in HTTPMethods order.
	Methods []string
	// Access is the package's `var Access` rule: "" for none, "public",
	// "signedIn" or "permission".
	Access string
	// Perms are a "permission" rule's permissions, where they're written
	// as string literals.
	Perms []string
}

// AccessLabel describes the package's rule for people: "public",
// "signed in" or "permission links.edit".
func (p *GoPackage) AccessLabel() string {
	switch {
	case p == nil:
		return ""
	case p.Access == "public":
		return "public"
	case p.Access == "signedIn":
		return "signed in"
	case p.Access == "permission" && len(p.Perms) > 0:
		return "permission " + strings.Join(p.Perms, ", ")
	case p.Access == "permission":
		return "permission"
	}
	return ""
}

// Restricted reports whether the package's Access rule keeps anyone out,
// and so applies to the directories below it too.
func (p *GoPackage) Restricted() bool {
	return p != nil && (p.Access == "signedIn" || p.Access == "permission")
}

// ServerCode reports whether the package has code that runs on the
// server for requests: loads, actions or endpoints.
func (p *GoPackage) ServerCode() bool {
	return p != nil && (p.Load || p.LayoutLoad || p.Actions || len(p.Methods) > 0)
}

// Layout is a directory that has a +layout.svelte, a LayoutLoad, or both.
type Layout struct {
	Dir string
	// Component is the +layout.svelte file, or "".
	Component string
	// Go is the directory's Go package if it exports LayoutLoad, or nil.
	Go *GoPackage
}

// HasLoad reports whether the route's page has a Load function.
func (r *Route) HasLoad() bool { return r.Go != nil && r.Go.Load }

// HasActions reports whether the route's page has form actions.
func (r *Route) HasActions() bool { return r.Go != nil && r.Go.Actions }

// IsEndpoint reports whether the route handles requests with HTTP method
// handlers (GET, POST, …).
func (r *Route) IsEndpoint() bool { return r.Go != nil && len(r.Go.Methods) > 0 }

// Pattern returns the route's URL pattern in bracket notation,
// e.g. "/blog/[slug]".
func (r *Route) Pattern() string {
	if len(r.Segments) == 0 {
		return "/"
	}
	var b strings.Builder
	for _, s := range r.Segments {
		b.WriteByte('/')
		b.WriteString(s.Pattern())
	}
	return b.String()
}

// Params returns the route's parameter segments in URL order.
func (r *Route) Params() []Segment {
	var params []Segment
	for _, s := range r.Segments {
		if s.Kind != Static {
			params = append(params, s)
		}
	}
	return params
}

// Manifest is the result of scanning a routes directory.
type Manifest struct {
	// Root is the routes directory that was scanned.
	Root string
	// Routes, in matching priority order.
	Routes []*Route
	// Open lists the directories with server code (loads, actions or
	// endpoints) and no Access rule, of their own or from above. Code
	// generation refuses them (see CheckAccess); tools that only read
	// routes don't.
	Open []string
}

// CheckAccess reports the routes with server code and no Access rule.
func (m *Manifest) CheckAccess() error {
	var errs []error
	for _, dir := range m.Open {
		errs = append(errs, fmt.Errorf("%s: this route has server code but no Access rule: add `var Access = kit.SignedIn`, `kit.Permission(\"…\")` or `kit.Public` to its package (`go tool tern fix access` marks every such route kit.Public)", path.Join(m.Root, dir)))
	}
	return errors.Join(errs...)
}

// Scan reads the routes directory root in fsys and returns its manifest.
// All problems found are reported together, joined into one error.
func Scan(fsys fs.FS, root string) (*Manifest, error) {
	info, err := fs.Stat(fsys, root)
	if err != nil {
		return nil, fmt.Errorf("routes directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("routes directory: %s is not a directory", root)
	}

	s := &scanner{fsys: fsys, root: root, fset: token.NewFileSet()}
	s.walk("", nil)
	s.checkConflicts()
	if len(s.errs) > 0 {
		return nil, errors.Join(s.errs...)
	}
	Sort(s.routes)
	return &Manifest{Root: root, Routes: s.routes, Open: s.open}, nil
}

type scanner struct {
	fsys   fs.FS
	root   string
	fset   *token.FileSet
	routes []*Route
	errs   []error
	// open lists the directories with server code and no Access rule.
	open []string
}

type node struct {
	parent    *node
	access    *GoPackage // whose Access applies here
	dir       string
	id        string
	segments  []Segment
	layout    *Layout
	errorPage string
}

func (s *scanner) errorf(rel, format string, args ...any) {
	s.errs = append(s.errs, fmt.Errorf("%s: %s", path.Join(s.root, rel), fmt.Sprintf(format, args...)))
}

func (s *scanner) walk(dir string, parent *node) {
	n := &node{parent: parent, dir: dir, id: "/"}
	if parent != nil {
		name := path.Base(dir)
		seg, group, err := parseDirName(name)
		if err != nil {
			s.errorf(dir, "%v", err)
			return
		}
		n.segments = slices.Clone(parent.segments)
		if !group {
			n.segments = append(n.segments, seg)
		}
		n.id = strings.TrimSuffix(parent.id, "/") + "/" + name
	}

	entries, err := fs.ReadDir(s.fsys, path.Join(s.root, dir))
	if err != nil {
		s.errorf(dir, "%v", err)
		return
	}

	files := map[string]string{}
	var goFiles, subdirs []string
	for _, e := range entries {
		name := e.Name()
		rel := path.Join(dir, name)
		switch {
		case strings.HasPrefix(name, "."):
			continue
		case e.IsDir():
			subdirs = append(subdirs, name)
		case strings.HasSuffix(name, ".go"):
			if strings.HasPrefix(name, "+") {
				s.errorf(rel, "Go does not allow file names starting with '+'; rename it to %q", strings.TrimPrefix(name, "+"))
				continue
			}
			if !strings.HasSuffix(name, "_test.go") && !strings.HasPrefix(name, "_") {
				goFiles = append(goFiles, rel)
			}
		case !strings.HasPrefix(name, "+"):
			continue // colocated components, styles, assets
		case slices.Contains(svelteFiles, name):
			files[name] = rel
		case name == FilePageMarkdown:
			if files[FilePage] != "" || slices.ContainsFunc(entries, func(e fs.DirEntry) bool { return e.Name() == FilePage }) {
				s.errorf(rel, "a directory has either %s or %s, not both", FilePage, FilePageMarkdown)
				continue
			}
			files[FilePage] = rel
		case strings.HasSuffix(name, ".ts") || strings.HasSuffix(name, ".js"):
			s.errorf(rel, "load functions run in Go in Tern; export Load from a .go file in this directory instead")
		default:
			s.errorf(rel, "unknown route file %q (expected one of %s)", name, strings.Join(append(svelteFiles, FilePageMarkdown), ", "))
		}
	}

	var pkg *GoPackage
	if len(goFiles) > 0 {
		pkg = s.parseGoPackage(dir, goFiles)
	}
	switch {
	case pkg != nil && pkg.Access != "":
		n.access = pkg
	case parent != nil && parent.access.Restricted():
		n.access = parent.access
	}
	if pkg.ServerCode() && n.access == nil {
		s.open = append(s.open, dir)
	}

	if files[FileLayout] != "" || (pkg != nil && pkg.LayoutLoad) {
		n.layout = &Layout{Dir: dir, Component: files[FileLayout]}
		if pkg != nil && pkg.LayoutLoad {
			n.layout.Go = pkg
		}
	}
	n.errorPage = files[FileError]

	if pkg != nil && (pkg.Load || pkg.Actions) && files[FilePage] == "" {
		s.errorf(dir, "package %s exports Load or Actions, but there is no %s in this directory", pkg.Name, FilePage)
	}
	if files[FilePage] != "" || (pkg != nil && len(pkg.Methods) > 0) {
		s.addRoute(n, files[FilePage], pkg)
	}

	for _, sub := range subdirs {
		s.walk(path.Join(dir, sub), n)
	}
}

// parseGoPackage reads the exported declarations of the Go files in dir.
// Signatures are checked later, by the code generator, with full type
// information.
func (s *scanner) parseGoPackage(dir string, files []string) *GoPackage {
	pkg := &GoPackage{Dir: dir, Files: files}
	if dir != "" {
		for _, elem := range strings.Split(dir, "/") {
			if reason := goSafeDir(elem); reason != "" {
				s.errorf(dir, "%s", reason)
				return nil
			}
		}
	}

	methods := map[string]bool{}
	for _, rel := range files {
		src, err := fs.ReadFile(s.fsys, path.Join(s.root, rel))
		if err != nil {
			s.errorf(rel, "%v", err)
			continue
		}
		f, err := parser.ParseFile(s.fset, path.Join(s.root, rel), src, parser.SkipObjectResolution)
		if err != nil {
			s.errs = append(s.errs, err)
			continue
		}
		if pkg.Name == "" {
			pkg.Name = f.Name.Name
		} else if pkg.Name != f.Name.Name {
			s.errorf(rel, "package %s conflicts with package %s in the same directory", f.Name.Name, pkg.Name)
		}

		for _, decl := range f.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if d.Recv != nil {
					continue
				}
				switch name := d.Name.Name; {
				case name == "Load":
					pkg.Load = true
				case name == "LayoutLoad":
					pkg.LayoutLoad = true
				case slices.Contains(HTTPMethods, name):
					methods[name] = true
				}
			case *ast.GenDecl:
				if d.Tok != token.VAR {
					continue
				}
				for _, spec := range d.Specs {
					vs := spec.(*ast.ValueSpec)
					for i, id := range vs.Names {
						switch id.Name {
						case "Actions":
							pkg.Actions = true
						case "Access":
							var value ast.Expr
							if i < len(vs.Values) {
								value = vs.Values[i]
							}
							pkg.Access, pkg.Perms = accessRule(f, value)
							if pkg.Access == "" {
								s.errorf(rel, "Access must be kit.Public, kit.SignedIn or kit.Permission(\"…\")")
							}
						}
					}
				}
			}
		}
	}
	for _, m := range HTTPMethods {
		if methods[m] {
			pkg.Methods = append(pkg.Methods, m)
		}
	}
	return pkg
}

func (s *scanner) addRoute(n *node, page string, pkg *GoPackage) {
	r := &Route{ID: n.id, Dir: n.dir, Segments: n.segments, Page: page, Go: pkg, Access: n.access}
	for a := n; a != nil; a = a.parent {
		if a.layout != nil {
			r.Layouts = append(r.Layouts, *a.layout)
		}
		if a.errorPage != "" {
			r.ErrorPages = append(r.ErrorPages, a.errorPage)
		}
	}
	slices.Reverse(r.Layouts)
	slices.Reverse(r.ErrorPages)

	seen := map[string]bool{}
	for _, p := range r.Params() {
		if seen[p.Value] {
			s.errorf(n.dir, "route %s uses the parameter name %q more than once", r.ID, p.Value)
		}
		seen[p.Value] = true
	}
	s.routes = append(s.routes, r)
}

// checkConflicts reports routes that would match exactly the same URLs,
// such as "/a.group/about" and "/b.group/about", or "/id.param" and
// "/slug.param".
func (s *scanner) checkConflicts() {
	byKey := map[string]*Route{}
	for _, r := range s.routes {
		parts := make([]string, len(r.Segments))
		for i, seg := range r.Segments {
			parts[i] = seg.canonical()
		}
		key := "/" + strings.Join(parts, "/")
		if other, ok := byKey[key]; ok {
			s.errorf(r.Dir, "route %s conflicts with %s: both match %s", r.ID, other.ID, r.Pattern())
			continue
		}
		byKey[key] = r
	}
}

// MissingAccess lists the route directories with server code (loads,
// actions or endpoints) and no Access rule, of their own or from above.
func MissingAccess(fsys fs.FS, root string) ([]string, error) {
	m, err := Scan(fsys, root)
	if err != nil {
		return nil, err
	}
	return m.Open, nil
}

// accessRule classifies the value of a route package's `var Access`:
// kit.Public, kit.SignedIn or kit.Permission(…), with kit being the
// import of github.com/Unfield/Tern/kit under any name.
func accessRule(f *ast.File, value ast.Expr) (rule string, perms []string) {
	kitName := ""
	for _, imp := range f.Imports {
		if imp.Path.Value != `"github.com/Unfield/Tern/kit"` {
			continue
		}
		kitName = "kit"
		if imp.Name != nil {
			kitName = imp.Name.Name
		}
	}
	isKit := func(e ast.Expr, name string) bool {
		sel, ok := e.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != name {
			return false
		}
		x, ok := sel.X.(*ast.Ident)
		return ok && kitName != "" && x.Name == kitName
	}
	switch {
	case value == nil:
		return "", nil
	case isKit(value, "Public"):
		return "public", nil
	case isKit(value, "SignedIn"):
		return "signedIn", nil
	}
	if call, ok := value.(*ast.CallExpr); ok && isKit(call.Fun, "Permission") && len(call.Args) > 0 {
		for _, arg := range call.Args {
			if lit, ok := arg.(*ast.BasicLit); ok && lit.Kind == token.STRING {
				if v, err := strconv.Unquote(lit.Value); err == nil {
					perms = append(perms, v)
				}
			}
		}
		return "permission", perms
	}
	return "", nil
}

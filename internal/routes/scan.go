// Package routes discovers Tern routes on the filesystem and matches URLs
// against them.
//
// Routes follow SvelteKit's conventions: every directory under the routes
// root is a URL segment, and files starting with "+" define what the
// directory does:
//
//	+page.svelte        page component
//	+page.server.go     page Load and Actions
//	+layout.svelte      layout wrapping this directory and its children
//	+layout.server.go   layout Load
//	+error.svelte       error boundary
//	+server.go          endpoint (GET, POST, … handlers)
//
// Directory names may contain parameters ([slug], [[optional]],
// [...rest], [id=matcher]) and groups ((name)), which organize routes
// without affecting the URL.
package routes

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"
)

// Route file names.
const (
	FilePage         = "+page.svelte"
	FilePageServer   = "+page.server.go"
	FileLayout       = "+layout.svelte"
	FileLayoutServer = "+layout.server.go"
	FileError        = "+error.svelte"
	FileEndpoint     = "+server.go"
)

var knownFiles = []string{FilePage, FilePageServer, FileLayout, FileLayoutServer, FileError, FileEndpoint}

// Route is a URL pattern backed by a page, an endpoint, or both.
// All file paths are slash-separated and relative to the routes root.
type Route struct {
	// ID identifies the route by its directory, including groups,
	// e.g. "/(app)/blog/[slug]". The root route's ID is "/".
	ID string
	// Dir is the route's directory, or "" for the root.
	Dir string
	// Segments are the URL segments, with groups removed.
	Segments []Segment

	Page       string // +page.svelte, or ""
	PageServer string // +page.server.go, or ""
	Endpoint   string // +server.go, or ""

	// Layouts that wrap the page, outermost first.
	Layouts []Layout
	// ErrorPages are the +error.svelte files that apply, outermost first.
	ErrorPages []string
}

// Layout is a directory that has a +layout.svelte, a +layout.server.go, or both.
type Layout struct {
	Dir       string
	Component string // +layout.svelte, or ""
	Server    string // +layout.server.go, or ""
}

// Pattern returns the route's URL pattern, e.g. "/blog/[slug]".
func (r *Route) Pattern() string {
	if len(r.Segments) == 0 {
		return "/"
	}
	var b strings.Builder
	for _, s := range r.Segments {
		b.WriteByte('/')
		b.WriteString(s.Raw)
	}
	return b.String()
}

// Params returns the route's parameters in URL order.
func (r *Route) Params() []Part {
	var params []Part
	for _, s := range r.Segments {
		for _, p := range s.Parts {
			if p.Kind != Static {
				params = append(params, p)
			}
		}
	}
	return params
}

// Manifest is the result of scanning a routes directory.
type Manifest struct {
	// Routes, in matching priority order.
	Routes []*Route
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

	s := &scanner{fsys: fsys, root: root}
	s.walk("", nil)
	s.checkConflicts()
	if len(s.errs) > 0 {
		return nil, errors.Join(s.errs...)
	}
	Sort(s.routes)
	return &Manifest{Routes: s.routes}, nil
}

type scanner struct {
	fsys   fs.FS
	root   string
	routes []*Route
	errs   []error
}

type node struct {
	parent    *node
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
	var subdirs []string
	for _, e := range entries {
		name := e.Name()
		switch {
		case strings.HasPrefix(name, "."):
			continue
		case e.IsDir():
			subdirs = append(subdirs, name)
		case !strings.HasPrefix(name, "+"):
			continue // colocated helpers, components, Go files
		case slices.Contains(knownFiles, name):
			files[name] = path.Join(dir, name)
		case strings.HasSuffix(name, "_test.go"):
			continue
		default:
			s.errorf(path.Join(dir, name), "unknown route file %q (expected one of %s)", name, strings.Join(knownFiles, ", "))
		}
	}

	if files[FileLayout] != "" || files[FileLayoutServer] != "" {
		n.layout = &Layout{Dir: dir, Component: files[FileLayout], Server: files[FileLayoutServer]}
	}
	n.errorPage = files[FileError]

	if files[FilePageServer] != "" && files[FilePage] == "" {
		s.errorf(files[FilePageServer], "%s requires a %s in the same directory", FilePageServer, FilePage)
	}
	if files[FilePage] != "" || files[FileEndpoint] != "" {
		s.addRoute(n, files)
	}

	for _, sub := range subdirs {
		s.walk(path.Join(dir, sub), n)
	}
}

func (s *scanner) addRoute(n *node, files map[string]string) {
	r := &Route{
		ID:         n.id,
		Dir:        n.dir,
		Segments:   n.segments,
		Page:       files[FilePage],
		PageServer: files[FilePageServer],
		Endpoint:   files[FileEndpoint],
	}
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
// such as "/(a)/about" and "/(b)/about", or "/[id]" and "/[slug]".
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

package routes

import (
	"cmp"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
)

// Sort orders routes by matching priority, most specific first. Segments are
// compared left to right: static text beats the end of a route, which beats
// mixed segments ("[slug].json"), then parameters, then optional parameters,
// then rest parameters. Parameters with a matcher beat those without. Ties
// are broken by pattern, then ID, so the order is deterministic.
func Sort(routes []*Route) {
	slices.SortStableFunc(routes, compareRoutes)
}

func compareRoutes(a, b *Route) int {
	for i := range max(len(a.Segments), len(b.Segments)) {
		if c := cmp.Compare(rankAt(a, i), rankAt(b, i)); c != 0 {
			return c
		}
	}
	if c := strings.Compare(a.Pattern(), b.Pattern()); c != 0 {
		return c
	}
	return strings.Compare(a.ID, b.ID)
}

func rankAt(r *Route, i int) int {
	if i >= len(r.Segments) {
		return rankMissing
	}
	return r.Segments[i].rank()
}

// Matcher reports whether a parameter value is acceptable, e.g. whether
// "42" is an integer for [id=integer].
type Matcher func(value string) bool

// Router matches URL paths to routes.
type Router struct {
	routes []compiledRoute
}

type compiledRoute struct {
	route    *Route
	re       *regexp.Regexp
	params   []Part
	matchers []Matcher // parallel to params; nil when the param has no matcher
}

// NewRouter compiles routes for matching. Every matcher a route refers to
// must be present in matchers. The routes are matched in the order given;
// use [Sort] (or a [Manifest]'s order) to match by priority.
func NewRouter(routes []*Route, matchers map[string]Matcher) (*Router, error) {
	r := &Router{routes: make([]compiledRoute, 0, len(routes))}
	for _, route := range routes {
		var b strings.Builder
		b.WriteByte('^')
		for _, s := range route.Segments {
			b.WriteString(s.pattern())
		}
		b.WriteByte('$')

		c := compiledRoute{route: route, re: regexp.MustCompile(b.String()), params: route.Params()}
		c.matchers = make([]Matcher, len(c.params))
		for i, p := range c.params {
			if p.Matcher == "" {
				continue
			}
			m, ok := matchers[p.Matcher]
			if !ok {
				return nil, fmt.Errorf("route %s: unknown matcher %q", route.ID, p.Matcher)
			}
			c.matchers[i] = m
		}
		r.routes = append(r.routes, c)
	}
	return r, nil
}

// Match finds the first route matching urlPath, which should be the escaped
// path of a request URL (url.URL.EscapedPath). It returns the route and its
// decoded parameter values. Absent optional parameters are left out of the
// map; an empty rest parameter is present with the value "".
//
// A trailing slash is ignored: "/about/" matches the same route as "/about".
func (r *Router) Match(urlPath string) (*Route, map[string]string, bool) {
	normalized, ok := normalizePath(urlPath)
	if !ok {
		return nil, nil, false
	}
	for _, c := range r.routes {
		m := c.re.FindStringSubmatch(normalized)
		if m == nil {
			continue
		}
		if params, ok := c.extract(m[1:]); ok {
			return c.route, params, true
		}
	}
	return nil, nil, false
}

func (c *compiledRoute) extract(values []string) (map[string]string, bool) {
	params := make(map[string]string, len(c.params))
	for i, p := range c.params {
		raw := values[i]
		if raw == "" && p.Kind == OptionalParam {
			continue
		}
		v, err := url.PathUnescape(raw)
		if err != nil {
			return nil, false
		}
		if m := c.matchers[i]; m != nil && !m(v) {
			return nil, false
		}
		params[p.Value] = v
	}
	return params, true
}

// normalizePath decodes each segment of an escaped path, then re-escapes only
// "%" and "/". Static route text is compared in the same form, so "%C3%BC"
// matches a directory named "ü" while "a%2Fb" stays a single segment.
func normalizePath(p string) (string, bool) {
	if !strings.HasPrefix(p, "/") {
		return "", false
	}
	p = strings.TrimSuffix(p, "/")
	if p == "" {
		return "", true
	}
	segs := strings.Split(p[1:], "/")
	for i, s := range segs {
		d, err := url.PathUnescape(s)
		if err != nil {
			return "", false
		}
		segs[i] = pathEscaper.Replace(d)
	}
	return "/" + strings.Join(segs, "/"), true
}

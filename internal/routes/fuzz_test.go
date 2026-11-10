package routes

import (
	"maps"
	"strings"
	"testing"
	"unicode/utf8"
)

var fuzzRouteIDs = []string{
	"/", "/about", "/marketing.group/pricing", "/blog/slug.param", "/posts/id.param.integer",
	"/posts/slug.param", "/lang.optional.locale/docs", "/files/path.rest/edit", "/files/path.rest",
	"/über", "/a/b.param/c.param/d", "/catchall.rest",
}

// FuzzMatch checks that matching never panics, and that building a URL from
// a match's parameters leads back to the same route and parameters.
func FuzzMatch(f *testing.F) {
	for _, s := range []string{"/", "/about/", "/blog/a%2Fb", "/de/docs", "/files/x/y/edit", "/%C3%BCber", "//x", "/a/%", "/posts/007", "/a/1/2/d", "/%00", "/a/b/../c"} {
		f.Add(s)
	}
	r := newFuzzRouter(f)
	f.Fuzz(func(t *testing.T, path string) {
		route, params, ok := r.Match(path)
		if !ok {
			return
		}
		for _, v := range params {
			if !utf8.ValidString(v) && utf8.ValidString(path) && !strings.Contains(path, "%") {
				t.Fatalf("invalid UTF-8 parameter %q from %q", v, path)
			}
		}
		if strings.Contains(strings.ToLower(path), "%2f") && strings.Contains(route.ID, ".rest") {
			return // an encoded slash in a rest parameter decodes to a separator
		}
		built, err := Build(route.Segments, params)
		if err != nil {
			t.Fatalf("Build(%s, %v): %v", route.ID, params, err)
		}
		route2, params2, ok := r.Match(built)
		if !ok || route2.ID != route.ID || !maps.Equal(params, params2) {
			t.Fatalf("%q matched %s %v; rebuilt %q matched %v %v", path, route.ID, params, built, route2, params2)
		}
	})
}

// FuzzBuildParam checks that any parameter value survives Build and Match.
func FuzzBuildParam(f *testing.F) {
	for _, s := range []string{"hello", "a/b", "100%", "über", " ", "?x=1#y", "..", "."} {
		f.Add(s, "x/y")
	}
	r := newFuzzRouter(f)
	slug := routeFor(f, "/blog/slug.param")
	rest := routeFor(f, "/files/path.rest/edit")
	f.Fuzz(func(t *testing.T, value, restValue string) {
		if value == "" || !utf8.ValidString(value) || !utf8.ValidString(restValue) {
			return
		}
		u, err := Build(slug.Segments, map[string]string{"slug": value})
		if err != nil {
			return // values Build rejects are fine
		}
		if route, params, ok := r.Match(u); !ok || route.ID != slug.ID || params["slug"] != value {
			t.Fatalf("slug %q: built %q, matched %v %v", value, u, route, params)
		}
		if strings.Contains(restValue, "//") || strings.HasPrefix(restValue, "/") || strings.HasSuffix(restValue, "/") {
			return // empty segments are not representable in a rest parameter
		}
		u, err = Build(rest.Segments, map[string]string{"path": restValue})
		if err != nil {
			return
		}
		if route, params, ok := r.Match(u); !ok || route.ID != rest.ID || params["path"] != restValue {
			t.Fatalf("rest %q: built %q, matched %v %v", restValue, u, route, params)
		}
	})
}

// FuzzParseID checks that parsing directory names never panics.
func FuzzParseID(f *testing.F) {
	for _, s := range fuzzRouteIDs {
		f.Add(s)
	}
	f.Add("/x.param.")
	f.Add("/.param")
	f.Fuzz(func(t *testing.T, id string) {
		_, _ = ParseID(id)
	})
}

func newFuzzRouter(f *testing.F) *Router {
	routes := make([]*Route, len(fuzzRouteIDs))
	for i, id := range fuzzRouteIDs {
		routes[i] = routeFor(f, id)
	}
	Sort(routes)
	r, err := NewRouter(routes, map[string]Matcher{
		"integer": func(v string) bool {
			for _, c := range v {
				if c < '0' || c > '9' {
					return false
				}
			}
			return v != ""
		},
		"locale": func(v string) bool { return v == "en" || v == "de" },
	})
	if err != nil {
		f.Fatal(err)
	}
	return r
}

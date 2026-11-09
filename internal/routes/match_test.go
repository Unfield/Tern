package routes

import (
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func newTestRouter(t *testing.T, routeIDs ...string) *Router {
	t.Helper()
	routes := make([]*Route, len(routeIDs))
	for i, id := range routeIDs {
		routes[i] = routeFor(t, id)
	}
	Sort(routes)
	r, err := NewRouter(routes, map[string]Matcher{
		"integer": func(v string) bool { _, err := strconv.Atoi(v); return err == nil },
		"locale":  func(v string) bool { return v == "en" || v == "de" },
	})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	return r
}

func TestMatch(t *testing.T) {
	r := newTestRouter(t,
		"/",
		"/about",
		"/marketing.group/pricing",
		"/blog/slug.param",
		"/posts/id.param.integer",
		"/posts/slug.param",
		"/lang.optional.locale/docs",
		"/files/path.rest/edit",
		"/files/path.rest",
		"/über",
		"/catchall.rest",
	)

	tests := []struct {
		path       string
		wantID     string
		wantParams map[string]string
	}{
		{"/", "/", map[string]string{}},
		{"/about", "/about", map[string]string{}},
		{"/about/", "/about", map[string]string{}},
		{"/pricing", "/marketing.group/pricing", map[string]string{}},
		{"/blog/hello", "/blog/slug.param", map[string]string{"slug": "hello"}},
		{"/blog/hello%20world", "/blog/slug.param", map[string]string{"slug": "hello world"}},
		{"/blog/a%2Fb", "/blog/slug.param", map[string]string{"slug": "a/b"}},
		{"/blog/100%25", "/blog/slug.param", map[string]string{"slug": "100%"}},
		{"/posts/42", "/posts/id.param.integer", map[string]string{"id": "42"}},
		{"/posts/hello", "/posts/slug.param", map[string]string{"slug": "hello"}},
		{"/docs", "/lang.optional.locale/docs", map[string]string{}},
		{"/de/docs", "/lang.optional.locale/docs", map[string]string{"lang": "de"}},
		{"/fr/docs", "/catchall.rest", map[string]string{"catchall": "fr/docs"}},
		{"/files/a/b/c", "/files/path.rest", map[string]string{"path": "a/b/c"}},
		{"/files", "/files/path.rest", map[string]string{"path": ""}},
		{"/files/a/b/edit", "/files/path.rest/edit", map[string]string{"path": "a/b"}},
		{"/files/edit", "/files/path.rest/edit", map[string]string{"path": ""}},
		{"/%C3%BCber", "/über", map[string]string{}},
		{"/nope/really", "/catchall.rest", map[string]string{"catchall": "nope/really"}},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			route, params, ok := r.Match(tt.path)
			if !ok {
				t.Fatalf("Match(%q) found nothing, want %s", tt.path, tt.wantID)
			}
			if route.ID != tt.wantID {
				t.Errorf("Match(%q) = %s, want %s", tt.path, route.ID, tt.wantID)
			}
			if !reflect.DeepEqual(params, tt.wantParams) {
				t.Errorf("params = %v, want %v", params, tt.wantParams)
			}
		})
	}
}

func TestMatchNoRoute(t *testing.T) {
	r := newTestRouter(t, "/", "/blog/slug.param")
	for _, p := range []string{"/blog", "/blog/a/b", "/blog//", "relative", "", "/blog/%zz"} {
		if route, _, ok := r.Match(p); ok {
			t.Errorf("Match(%q) = %s, want no match", p, route.ID)
		}
	}
}

func TestNewRouterUnknownMatcher(t *testing.T) {
	_, err := NewRouter([]*Route{routeFor(t, "/id.param.uuid")}, nil)
	if err == nil || !strings.Contains(err.Error(), `unknown matcher "uuid"`) {
		t.Fatalf("err = %v, want unknown matcher error", err)
	}
}

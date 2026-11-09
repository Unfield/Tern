package routes

import (
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
)

func mapFS(files ...string) fstest.MapFS {
	fsys := fstest.MapFS{}
	for _, f := range files {
		fsys[f] = &fstest.MapFile{}
	}
	return fsys
}

func ids(routes []*Route) []string {
	out := make([]string, len(routes))
	for i, r := range routes {
		out[i] = r.ID
	}
	return out
}

func findRoute(t *testing.T, m *Manifest, id string) *Route {
	t.Helper()
	for _, r := range m.Routes {
		if r.ID == id {
			return r
		}
	}
	t.Fatalf("route %s not found; have %v", id, ids(m.Routes))
	return nil
}

func TestScan(t *testing.T) {
	fsys := mapFS(
		"src/routes/+layout.svelte",
		"src/routes/+layout.server.go",
		"src/routes/+error.svelte",
		"src/routes/+page.svelte",
		"src/routes/about/+page.svelte",
		"src/routes/blog/+page.svelte",
		"src/routes/blog/+page.server.go",
		"src/routes/blog/[slug]/+page.svelte",
		"src/routes/blog/[slug]/+page.server.go",
		"src/routes/blog/[slug]/Comments.svelte", // colocated, ignored
		"src/routes/blog/[slug]/helpers.go",      // colocated, ignored
		"src/routes/blog/[slug]/+page.server_test.go",
		"src/routes/(app)/+layout.svelte",
		"src/routes/(app)/dashboard/+error.svelte",
		"src/routes/(app)/dashboard/+page.svelte",
		"src/routes/api/health/+server.go",
		"src/routes/docs/[...path]/+page.svelte",
		"src/routes/.hidden/+page.svelte", // dot directories are ignored
		"src/routes/layout-only/+layout.svelte",
	)

	m, err := Scan(fsys, "src/routes")
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	wantIDs := []string{
		"/api/health",
		"/about",
		"/blog",
		"/(app)/dashboard",
		"/blog/[slug]",
		"/docs/[...path]",
		"/",
	}
	if got := ids(m.Routes); !reflect.DeepEqual(got, wantIDs) {
		t.Errorf("route order:\n got  %v\n want %v", got, wantIDs)
	}

	root := findRoute(t, m, "/")
	if root.Pattern() != "/" || root.Dir != "" || root.Page != "+page.svelte" {
		t.Errorf("root route = %+v", root)
	}

	slug := findRoute(t, m, "/blog/[slug]")
	if got, want := slug.Pattern(), "/blog/[slug]"; got != want {
		t.Errorf("Pattern() = %q, want %q", got, want)
	}
	if got, want := slug.PageServer, "blog/[slug]/+page.server.go"; got != want {
		t.Errorf("PageServer = %q, want %q", got, want)
	}
	if got := slug.Params(); len(got) != 1 || got[0].Value != "slug" {
		t.Errorf("Params() = %+v", got)
	}

	dash := findRoute(t, m, "/(app)/dashboard")
	if got, want := dash.Pattern(), "/dashboard"; got != want {
		t.Errorf("group should not appear in pattern: got %q, want %q", got, want)
	}
	wantLayouts := []Layout{
		{Dir: "", Component: "+layout.svelte", Server: "+layout.server.go"},
		{Dir: "(app)", Component: "(app)/+layout.svelte"},
	}
	if !reflect.DeepEqual(dash.Layouts, wantLayouts) {
		t.Errorf("Layouts:\n got  %+v\n want %+v", dash.Layouts, wantLayouts)
	}
	wantErrors := []string{"+error.svelte", "(app)/dashboard/+error.svelte"}
	if !reflect.DeepEqual(dash.ErrorPages, wantErrors) {
		t.Errorf("ErrorPages = %v, want %v", dash.ErrorPages, wantErrors)
	}

	health := findRoute(t, m, "/api/health")
	if health.Endpoint != "api/health/+server.go" || health.Page != "" {
		t.Errorf("endpoint route = %+v", health)
	}
}

func TestScanErrors(t *testing.T) {
	tests := []struct {
		name  string
		files []string
		want  []string
	}{
		{
			name:  "invalid directory name",
			files: []string{"r/[slug/+page.svelte"},
			want:  []string{"r/[slug: invalid directory name"},
		},
		{
			name:  "server file without page",
			files: []string{"r/about/+page.server.go"},
			want:  []string{"r/about/+page.server.go: +page.server.go requires a +page.svelte"},
		},
		{
			name:  "unknown route file",
			files: []string{"r/+page.ts"},
			want:  []string{`r/+page.ts: unknown route file "+page.ts"`},
		},
		{
			name:  "groups that conflict",
			files: []string{"r/(a)/about/+page.svelte", "r/(b)/about/+page.svelte"},
			want:  []string{"route /(b)/about conflicts with /(a)/about"},
		},
		{
			name:  "params that conflict",
			files: []string{"r/[id]/+page.svelte", "r/[slug]/+page.svelte"},
			want:  []string{"route /[slug] conflicts with /[id]"},
		},
		{
			name:  "duplicate parameter name",
			files: []string{"r/[id]/x/[id]/+page.svelte"},
			want:  []string{`uses the parameter name "id" more than once`},
		},
		{
			name:  "all problems are reported",
			files: []string{"r/+page.js", "r/x/+page.server.go"},
			want:  []string{`unknown route file "+page.js"`, "+page.server.go requires a +page.svelte"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Scan(mapFS(tt.files...), "r")
			if err == nil {
				t.Fatal("expected an error, got nil")
			}
			for _, w := range tt.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("error %q does not contain %q", err, w)
				}
			}
		})
	}
}

func TestScanMissingRoot(t *testing.T) {
	if _, err := Scan(mapFS("other/+page.svelte"), "src/routes"); err == nil {
		t.Fatal("expected an error for a missing routes directory")
	}
	if _, err := Scan(mapFS("src/routes"), "src/routes"); err == nil {
		t.Fatal("expected an error when the routes root is a file")
	}
}

func TestSort(t *testing.T) {
	want := []string{
		"/blog/new",
		"/about",
		"/blog/[slug].json",
		"/blog/[id=integer]",
		"/blog/[slug]",
		"/blog/[slug]/[[page]]/x",
		"/blog/[...rest]",
		"/",
		"/[slug]",
		"/[[lang]]",
		"/[...catchall]",
	}
	var routes []*Route
	for i := len(want) - 1; i >= 0; i-- { // reverse order in
		routes = append(routes, routeFor(t, want[i]))
	}
	Sort(routes)
	if got := ids(routes); !reflect.DeepEqual(got, want) {
		t.Errorf("order:\n got  %v\n want %v", got, want)
	}
}

// routeFor builds a Route from an ID by parsing each segment.
func routeFor(t *testing.T, id string) *Route {
	t.Helper()
	r := &Route{ID: id}
	for _, name := range strings.Split(strings.Trim(id, "/"), "/") {
		if name == "" {
			continue
		}
		seg, group, err := parseDirName(name)
		if err != nil {
			t.Fatalf("parse %q: %v", name, err)
		}
		if !group {
			r.Segments = append(r.Segments, seg)
		}
	}
	return r
}

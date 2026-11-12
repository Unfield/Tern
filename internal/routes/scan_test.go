package routes

import (
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
)

// mapFS builds a filesystem from "path" or "path=contents" entries.
func mapFS(files ...string) fstest.MapFS {
	fsys := fstest.MapFS{}
	for _, f := range files {
		name, data, _ := strings.Cut(f, "=")
		fsys[name] = &fstest.MapFile{Data: []byte(data)}
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

const (
	goLoad       = "package page\nfunc Load() {}\n"
	goLayoutLoad = "package page\nfunc LayoutLoad() {}\n"
	goActions    = "package page\nvar Actions = 1\n"
	goEndpoint   = "package health\nfunc GET() {}\nfunc POST() {}\nfunc (x T) PUT() {}\n"
	goHelper     = "package page\nfunc helper() {}\n"
)

// access is a file declaring an Access rule, such as access("page", "kit.Public").
func access(pkg, rule string) string {
	return "package " + pkg + "\nimport \"github.com/Unfield/Tern/kit\"\nvar Access = " + rule + "\n"
}

func TestScan(t *testing.T) {
	fsys := mapFS(
		"src/routes/+layout.svelte",
		"src/routes/layout.server.go="+goLayoutLoad,
		"src/routes/access.go="+access("page", "kit.Public"),
		"src/routes/+error.svelte",
		"src/routes/+page.svelte",
		"src/routes/about/+page.svelte",
		"src/routes/blog/+page.svelte",
		"src/routes/blog/page.server.go="+goLoad,
		"src/routes/blog/access.go="+access("page", "kit.SignedIn"),
		"src/routes/blog/slug.param/+page.svelte",
		"src/routes/blog/slug.param/page.server.go="+goLoad,
		"src/routes/blog/slug.param/actions.go="+goActions,
		"src/routes/blog/slug.param/Comments.svelte",                // colocated, ignored
		"src/routes/blog/slug.param/page_test.go=package page_test", // tests are ignored
		"src/routes/app.group/+layout.svelte",
		"src/routes/app.group/dashboard/+error.svelte",
		"src/routes/app.group/dashboard/+page.svelte",
		"src/routes/api/health/server.go="+goEndpoint,
		"src/routes/api/health/access.go="+access("health", "kit.Public"),
		"src/routes/api/util/util.go="+goHelper, // Go without a route role
		"src/routes/docs/path.rest/+page.svelte",
		"src/routes/.hidden/+page.svelte",
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
		"/app.group/dashboard",
		"/blog/slug.param",
		"/docs/path.rest",
		"/",
	}
	if got := ids(m.Routes); !reflect.DeepEqual(got, wantIDs) {
		t.Errorf("route order:\n got  %v\n want %v", got, wantIDs)
	}

	root := findRoute(t, m, "/")
	if root.Pattern() != "/" || root.Dir != "" || root.Page != "+page.svelte" || root.HasLoad() {
		t.Errorf("root route = %+v", root)
	}
	if len(root.Layouts) != 1 || root.Layouts[0].Go == nil || !root.Layouts[0].Go.LayoutLoad {
		t.Errorf("root layout should have a LayoutLoad: %+v", root.Layouts)
	}

	slug := findRoute(t, m, "/blog/slug.param")
	if got, want := slug.Pattern(), "/blog/[slug]"; got != want {
		t.Errorf("Pattern() = %q, want %q", got, want)
	}
	if !slug.HasLoad() || !slug.HasActions() || slug.IsEndpoint() {
		t.Errorf("slug route roles: load=%v actions=%v endpoint=%v", slug.HasLoad(), slug.HasActions(), slug.IsEndpoint())
	}
	if got, want := slug.Go.Files, []string{"blog/slug.param/actions.go", "blog/slug.param/page.server.go"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Go files = %v, want %v", got, want)
	}
	if got := slug.Params(); len(got) != 1 || got[0].Value != "slug" {
		t.Errorf("Params() = %+v", got)
	}

	dash := findRoute(t, m, "/app.group/dashboard")
	if got, want := dash.Pattern(), "/dashboard"; got != want {
		t.Errorf("group should not appear in pattern: got %q, want %q", got, want)
	}
	if len(dash.Layouts) != 2 || dash.Layouts[1].Component != "app.group/+layout.svelte" || dash.Layouts[1].Go != nil {
		t.Errorf("Layouts = %+v", dash.Layouts)
	}
	wantErrors := []string{"+error.svelte", "app.group/dashboard/+error.svelte"}
	if !reflect.DeepEqual(dash.ErrorPages, wantErrors) {
		t.Errorf("ErrorPages = %v, want %v", dash.ErrorPages, wantErrors)
	}

	health := findRoute(t, m, "/api/health")
	if !health.IsEndpoint() || health.Page != "" || health.Go.Name != "health" {
		t.Errorf("endpoint route = %+v", health)
	}
	if got, want := health.Go.Methods, []string{"GET", "POST"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Methods = %v, want %v (methods with receivers do not count)", got, want)
	}
}

func TestScanErrors(t *testing.T) {
	tests := []struct {
		name  string
		files []string
		want  []string
	}{
		{
			name:  "SvelteKit directory name",
			files: []string{"r/blog/[slug]/+page.svelte"},
			want:  []string{`r/blog/[slug]: invalid directory name "[slug]"`, `rename it to "slug.param"`},
		},
		{
			name:  "SvelteKit Go file name",
			files: []string{"r/+page.svelte", "r/+page.server.go=" + goLoad},
			want:  []string{`r/+page.server.go: Go does not allow file names starting with '+'; rename it to "page.server.go"`},
		},
		{
			name:  "Load without a page",
			files: []string{"r/about/page.server.go=" + goLoad},
			want:  []string{"r/about: package page exports Load or Actions, but there is no +page.svelte"},
		},
		{
			name:  "JavaScript load file",
			files: []string{"r/+page.svelte", "r/+page.ts"},
			want:  []string{"r/+page.ts: load functions run in Go"},
		},
		{
			name:  "unknown route file",
			files: []string{"r/+pgae.svelte"},
			want:  []string{`r/+pgae.svelte: unknown route file "+pgae.svelte"`},
		},
		{
			name:  "Go in a directory Go cannot import",
			files: []string{"r/über/+page.svelte", "r/über/page.server.go=" + goLoad},
			want:  []string{"r/über: Go packages cannot live in a directory whose name contains 'ü'"},
		},
		{
			name:  "Go syntax error",
			files: []string{"r/+page.svelte", "r/page.server.go=package page\nfunc {"},
			want:  []string{"r/page.server.go:2:6"},
		},
		{
			name:  "mixed package names",
			files: []string{"r/+page.svelte", "r/a.go=package a", "r/b.go=package b"},
			want:  []string{"r/b.go: package b conflicts with package a"},
		},
		{
			name:  "groups that conflict",
			files: []string{"r/a.group/about/+page.svelte", "r/b.group/about/+page.svelte"},
			want:  []string{"route /b.group/about conflicts with /a.group/about"},
		},
		{
			name:  "params that conflict",
			files: []string{"r/id.param/+page.svelte", "r/slug.param/+page.svelte"},
			want:  []string{"route /slug.param conflicts with /id.param"},
		},
		{
			name:  "duplicate parameter name",
			files: []string{"r/id.param/x/id.param/+page.svelte"},
			want:  []string{`uses the parameter name "id" more than once`},
		},
		{
			name:  "all problems are reported",
			files: []string{"r/+page.js", "r/x/page.server.go=" + goLoad},
			want:  []string{"r/+page.js", "r/x: package page exports Load"},
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
		"/blog/id.param.integer",
		"/blog/slug.param",
		"/blog/slug.param/page.optional/x",
		"/blog/rest.rest",
		"/",
		"/slug.param",
		"/lang.optional",
		"/catchall.rest",
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
func routeFor(t testing.TB, id string) *Route {
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

func TestAccessRules(t *testing.T) {
	fsys := mapFS(
		"r/+page.svelte",
		"r/page.server.go="+goLoad,
		"r/access.go="+access("page", "kit.Public"),
		// Public applies to its own directory only.
		"r/about/+page.svelte",
		"r/news/+page.svelte",
		"r/news/page.server.go="+goLoad,
		// SignedIn and Permission apply below, unless a directory says otherwise.
		"r/app.group/+layout.svelte",
		"r/app.group/access.go="+access("app", "kit.SignedIn"),
		"r/app.group/projects/+page.svelte",
		"r/app.group/projects/page.server.go="+goLoad,
		"r/app.group/admin/+page.svelte",
		"r/app.group/admin/page.server.go=package admin\nimport t \"github.com/Unfield/Tern/kit\"\nfunc Load() {}\nvar Access = t.Permission(\"admin\")\n",
		"r/app.group/login/+page.svelte",
		"r/app.group/login/page.server.go=package login\nimport \"github.com/Unfield/Tern/kit\"\nvar Actions = 1\nvar Access = kit.Public\n",
	)
	missing, err := MissingAccess(fsys, "r")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(missing, []string{"news"}) {
		t.Errorf("MissingAccess = %v, want [news]", missing)
	}
	if m, err := Scan(fsys, "r"); err != nil || m.CheckAccess() == nil || !strings.Contains(m.CheckAccess().Error(), "r/news: this route has server code but no Access rule") {
		t.Errorf("Scan: %v", err)
	}
	delete(fsys, "r/news/page.server.go")
	m, err := Scan(fsys, "r")
	if err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]string{
		"/":                   "public",
		"/about":              "",
		"/app.group/projects": "signedIn",
		"/app.group/admin":    "permission",
		"/app.group/login":    "public",
	} {
		r := findRoute(t, m, id)
		got := ""
		if r.Access != nil {
			got = r.Access.Access
		}
		if got != want {
			t.Errorf("%s: Access %q, want %q", id, got, want)
		}
	}
	if r := findRoute(t, m, "/app.group/projects"); r.Access.Dir != "app.group" {
		t.Errorf("projects' rule comes from %q, want app.group", r.Access.Dir)
	}

	bad := mapFS("r/+page.svelte", "r/page.server.go="+goLoad, "r/access.go="+access("page", "something()"))
	if _, err := Scan(bad, "r"); err == nil || !strings.Contains(err.Error(), "Access must be kit.Public") {
		t.Errorf("an Access of another kind: %v", err)
	}
}

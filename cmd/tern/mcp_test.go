package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRouteDirs(t *testing.T) {
	cases := map[string]string{
		"/":                         "",
		"/about":                    "about",
		"/blog/[slug]":              "blog/slug.param",
		"/[[lang]]/docs/[...path]":  "lang.optional/docs/path.rest",
		"/(app)/posts/[id=integer]": "app.group/posts/id.param.integer",
		"/users/[user-id]":          "users/userid.param",
	}
	for in, want := range cases {
		dirs, err := routeDirs(in)
		if err != nil || strings.Join(dirs, "/") != want {
			t.Errorf("routeDirs(%q) = %v, %v; want %q", in, dirs, err, want)
		}
	}
	for _, bad := range []string{"/a[b]c", "/[]"} {
		if _, err := routeDirs(bad); err == nil {
			t.Errorf("routeDirs(%q) should fail", bad)
		}
	}
	if p := svelteKitToPattern("/(app)/blog/[slug]"); p != "/blog/[slug]" {
		t.Errorf("pattern = %q", p)
	}
}

func call(t *testing.T, fn func(context.Context, json.RawMessage) (string, error), args string) string {
	t.Helper()
	out, err := fn(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("%s: %v", args, err)
	}
	return out
}

func TestMCPTools(t *testing.T) {
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/app\n\ngo 1.26\n\nrequire github.com/Unfield/Tern v1.2.1\n"), 0o644)
	_ = os.MkdirAll(filepath.Join(root, "src", "routes"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "src", "routes", "+page.svelte"), nil, 0o644)
	tools := &mcpTools{root: root}

	out := call(t, tools.newRoute, `{"path":"/blog/[slug]","load":true,"actions":true,"methods":["delete"]}`)
	for _, want := range []string{"src/routes/blog/slug.param/+page.svelte: created", "page.server.go: created", "endpoint.go: created", "URL: /blog/[slug]"} {
		if !strings.Contains(out, want) {
			t.Errorf("new_route output missing %q:\n%s", want, out)
		}
	}
	goFile, _ := os.ReadFile(filepath.Join(root, "src/routes/blog/slug.param/page.server.go"))
	if !strings.HasPrefix(string(goFile), "package slug\n") || !strings.Contains(string(goFile), "func Load(ctx *kit.Ctx) (Page, error)") || !strings.Contains(string(goFile), "var Actions = kit.Actions{") {
		t.Errorf("page.server.go:\n%s", goFile)
	}
	// Signed-in users by default, declared once per package.
	endpoint, _ := os.ReadFile(filepath.Join(root, "src/routes/blog/slug.param/endpoint.go"))
	if !strings.Contains(string(goFile), "var Access = kit.SignedIn") || strings.Contains(string(endpoint), "var Access") {
		t.Errorf("Access:\n%s\n%s", goFile, endpoint)
	}
	call(t, tools.newRoute, `{"path":"/api/items","methods":["GET"],"access":"permission:items.read"}`)
	if api, _ := os.ReadFile(filepath.Join(root, "src/routes/api/items/endpoint.go")); !strings.Contains(string(api), `var Access = kit.Permission("items.read")`) {
		t.Errorf("a permission:\n%s", api)
	}
	if _, err := tools.newRoute(context.Background(), json.RawMessage(`{"path":"/y","load":true,"access":"everyone"}`)); err == nil {
		t.Error("an unknown access should fail")
	}
	if again := call(t, tools.newRoute, `{"path":"/blog/[slug]"}`); !strings.Contains(again, "exists, left alone") {
		t.Errorf("existing files should be kept: %s", again)
	}
	if _, err := tools.newRoute(context.Background(), json.RawMessage(`{"path":"/x","methods":["FETCH"]}`)); err == nil {
		t.Error("unknown methods should fail")
	}

	var list []routeInfo
	_ = json.Unmarshal([]byte(call(t, tools.routes, `{}`)), &list)
	if len(list) != 3 || list[2].Pattern != "/blog/[slug]" || !list[2].Load || !list[2].Actions || list[2].Kind != "page+endpoint" {
		t.Errorf("routes = %+v", list)
	}

	if out := call(t, tools.newLiveComponent, `{"path":"src/lib/Counter.svelte"}`); !strings.Contains(out, "created") {
		t.Errorf("new_live_component: %s", out)
	}
	if _, err := tools.newLiveComponent(context.Background(), json.RawMessage(`{"path":"../evil.svelte"}`)); err == nil {
		t.Error("paths outside src must be refused")
	}
	var comps []goComponent
	_ = json.Unmarshal([]byte(call(t, tools.liveComponents, `{}`)), &comps)
	if len(comps) != 1 || !comps[0].Live || comps[0].Path != "src/lib/Counter.svelte" {
		t.Errorf("live_components = %+v", comps)
	}

	var info map[string]any
	_ = json.Unmarshal([]byte(call(t, tools.project, `{}`)), &info)
	if info["module"] != "example.com/app" || info["tern"] != "v1.2.1" || info["routes"] != float64(3) || info["liveComponents"] != float64(1) {
		t.Errorf("project = %v", info)
	}

	if out := call(t, tools.searchDocs, `{"query":"WithLiveBroker"}`); !strings.Contains(out, "live-components.md") {
		t.Errorf("search_docs: %s", out)
	}
	if out := call(t, tools.readDoc, `{}`); !strings.Contains(out, "guide.md") || !strings.Contains(out, "adr/0010-live-components.md") {
		t.Errorf("read_doc list: %s", out)
	}
	if out := call(t, tools.readDoc, `{"name":"tern://docs/live-components.md"}`); !strings.HasPrefix(out, "# Live components") {
		t.Errorf("read_doc: %.40s", out)
	}
	if out := call(t, tools.newMigration, `{"name":"add users"}`); !strings.Contains(out, "add_users.sql") {
		t.Errorf("new_migration: %s", out)
	}
	if len(mcpServer(root).Resources) < 10 {
		t.Error("the docs should be resources")
	}
}

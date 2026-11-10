package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"unicode"

	"github.com/Unfield/Tern/docs"
	"github.com/Unfield/Tern/goscript"
	"github.com/Unfield/Tern/internal/codegen"
	"github.com/Unfield/Tern/internal/mcp"
	"github.com/Unfield/Tern/internal/routes"
	"github.com/Unfield/Tern/internal/version"
)

const mcpInstructions = `Tern is a web framework: Svelte 5 components on the client, Go on the server, built into one binary. Use these tools to inspect and change a Tern app; run them from the app's root (where go.mod is).

Conventions:
- Routes live in src/routes. Directory names are Go-safe: slug.param for [slug], lang.optional for [[lang]], path.rest for [...path], name.group for (name), id.param.integer for a matcher. new_route converts SvelteKit-style paths.
- A route directory holds +page.svelte, +layout.svelte, +error.svelte, and any .go files forming one package: func Load(ctx *kit.Ctx) (T, error) for page data, LayoutLoad for layouts, var Actions = kit.Actions{...} for forms, and GET/POST/... for endpoints.
- Run generate after changing Go route files or adding routes: it writes tern.gen.go and the TypeScript types ($types, $routes, $api). Then check type-checks everything, including the markup of Go components.
- <script lang="go" live> makes a live component: its logic is Go on the server (State, Derived, Func from the goscript package) and its markup is Svelte.
- The CLI is go tool tern (dev, build, gen, check, routes, migrate new).
Search the docs (search_docs, read_doc) before guessing an API.`

// cmdMCP serves the Model Context Protocol on stdin and stdout, for AI
// coding tools.
func cmdMCP(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && (args[0] == "-h" || args[0] == "--help") {
		fmt.Fprintln(stdout, "usage: tern mcp\n\nServes the Model Context Protocol on stdin and stdout, for AI coding tools. Configure it as the command `go tool tern mcp`, run from the app's root.")
		return 0
	}
	root, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	s := mcpServer(root)
	if err := s.Serve(context.Background(), os.Stdin, stdout); err != nil && !errors.Is(err, io.EOF) {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

func obj(props map[string]any, required ...string) map[string]any {
	s := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

func str(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }
func boolean(desc string) map[string]any {
	return map[string]any{"type": "boolean", "description": desc}
}

func mcpServer(root string) *mcp.Server {
	t := &mcpTools{root: root}
	s := &mcp.Server{Name: "tern", Version: version.String(), Instructions: mcpInstructions}
	s.Tools = []mcp.Tool{
		{Name: "project", Title: "Project overview", ReadOnly: true,
			Description: "Describes the app: Go module, Tern version, routes, live components, configuration (tern.json), plugins in use and the scripts in package.json.",
			Handler:     t.project},
		{Name: "routes", Title: "List routes", ReadOnly: true,
			Description: "Lists every route with its URL pattern, directory, kind (page, endpoint or both), files, whether it has a Load, Actions or LayoutLoad, endpoint methods, and its layouts.",
			Handler:     t.routes},
		{Name: "route_types", Title: "Route types", ReadOnly: true,
			Description: "Returns the generated TypeScript types ($types.d.ts) for a route: PageData, PageProps, ActionData, RouteParams and the layout types. Run generate first if they may be stale.",
			Schema:      obj(map[string]any{"route": str("The route's URL pattern (/blog/[slug]) or ID (/blog/slug.param)")}, "route"),
			Handler:     t.routeTypes},
		{Name: "live_components", Title: "List live components", ReadOnly: true,
			Description: "Lists the components with <script lang=\"go\" live> or <script lang=\"go\">, with their IDs and the TypeScript types of the state and Funcs their markup can use.",
			Handler:     t.liveComponents},
		{Name: "generate", Title: "Generate code",
			Description: "Runs tern gen: writes tern.gen.go, the client manifest, route types, $routes, $api and live component types. Reports changed files and warnings.",
			Handler:     t.generate},
		{Name: "check", Title: "Type-check",
			Description: "Runs tern check: generate, then svelte-check with Go components' markup checked against their Go types. Returns the errors and warnings with file and line.",
			Handler:     t.check},
		{Name: "go_vet", Title: "go vet", ReadOnly: true,
			Description: "Runs go vet on the app's packages (default ./...).",
			Schema:      obj(map[string]any{"packages": str("Package pattern, default ./...")}),
			Handler:     t.goVet},
		{Name: "go_test", Title: "go test", ReadOnly: true,
			Description: "Runs go test (default ./...), optionally only tests matching run.",
			Schema:      obj(map[string]any{"packages": str("Package pattern, default ./..."), "run": str("Only tests matching this regular expression")}),
			Handler:     t.goTest},
		{Name: "build", Title: "Build",
			Description: "Runs tern build: the production binary with the client, the server renderer and prerendered pages embedded. Takes a while.",
			Schema:      obj(map[string]any{"output": str("Output path, default build/app")}),
			Handler:     t.build},
		{Name: "new_route", Title: "Create a route",
			Description: "Creates a route directory from a URL path such as /blog/[slug] (SvelteKit-style brackets are converted to Go-safe names), with +page.svelte and, if asked, a Go file with Load, Actions or endpoint handlers. Existing files are left alone.",
			Schema: obj(map[string]any{
				"path":     str("URL path, such as /blog/[slug], /docs/[...path] or /(app)/settings"),
				"page":     boolean("Create +page.svelte (default: yes, unless only methods are given)"),
				"load":     boolean("Add a Load function in page.server.go"),
				"actions":  boolean("Add a default form action"),
				"layout":   boolean("Also create +layout.svelte"),
				"methods":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Endpoint methods to add, such as GET and POST"},
				"livePage": boolean("Make +page.svelte a live component with Go logic"),
			}, "path"),
			Handler: t.newRoute},
		{Name: "new_live_component", Title: "Create a live component",
			Description: "Creates a .svelte component whose logic is Go running on the server (<script lang=\"go\" live>), with a State, a Derived value and a Func to start from.",
			Schema:      obj(map[string]any{"path": str("Where to create it, such as src/lib/Counter.svelte")}, "path"),
			Handler:     t.newLiveComponent},
		{Name: "new_migration", Title: "Create a migration",
			Description: "Creates a SQL migration in migrations/, named with a timestamp, with an up part and a -- +down part.",
			Schema:      obj(map[string]any{"name": str("What it does, such as add users table")}, "name"),
			Handler:     t.newMigration},
		{Name: "ui_add", Title: "Copy Tern UI components",
			Description: "Copies Tern UI components (and the helpers they use) from the installed @ternkit/tern into src/lib/ui, to change them. Without names, lists the components. Usually importing them from @ternkit/tern/ui is enough; copy only to change one.",
			Schema:      obj(map[string]any{"components": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Components such as Button or DataTable"}}),
			Handler:     t.uiAdd},
		{Name: "search_docs", Title: "Search the docs", ReadOnly: true,
			Description: "Searches Tern's documentation for this version (the guide, live components, deployment, the SvelteKit migration guide and the decision records) and returns the best matching sections.",
			Schema:      obj(map[string]any{"query": str("What to look for, such as \"form actions validation\" or \"WithLiveBroker\"")}, "query"),
			Handler:     t.searchDocs},
		{Name: "read_doc", Title: "Read a doc", ReadOnly: true,
			Description: "Returns a whole documentation file, such as guide.md, live-components.md, deployment.md or adr/0010-live-components.md. Without a name, lists them.",
			Schema:      obj(map[string]any{"name": str("The file, such as guide.md")}),
			Handler:     t.readDoc},
	}
	for _, name := range docNames() {
		s.Resources = append(s.Resources, mcp.Resource{
			URI: "tern://docs/" + name, Name: name, MimeType: "text/markdown",
			Description: docTitle(name),
			Read: func() (string, error) {
				b, err := fs.ReadFile(docs.FS, name)
				return string(b), err
			},
		})
	}
	return s
}

type mcpTools struct{ root string }

func (t *mcpTools) path(rel string) string { return filepath.Join(t.root, filepath.FromSlash(rel)) }

func jsonText(v any) (string, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	return string(b), err
}

func (t *mcpTools) project(context.Context, json.RawMessage) (string, error) {
	info := map[string]any{"root": t.root, "cli": version.String()}
	if mod, err := os.ReadFile(t.path("go.mod")); err == nil {
		if m := regexp.MustCompile(`(?m)^module\s+(\S+)`).FindSubmatch(mod); m != nil {
			info["module"] = string(m[1])
		}
		if m := regexp.MustCompile(`(?m)github\.com/Unfield/Tern\s+(v\S+)`).FindSubmatch(mod); m != nil {
			info["tern"] = string(m[1])
		}
	} else {
		return "", errors.New("no go.mod here: run tern mcp from the app's root")
	}
	if raw, err := os.ReadFile(t.path("tern.json")); err == nil {
		info["ternJSON"] = json.RawMessage(raw)
	}
	if raw, err := os.ReadFile(t.path("package.json")); err == nil {
		var p struct {
			Scripts         map[string]string `json:"scripts"`
			DevDependencies map[string]string `json:"devDependencies"`
			Dependencies    map[string]string `json:"dependencies"`
		}
		if json.Unmarshal(raw, &p) == nil {
			info["scripts"] = p.Scripts
			deps := map[string]string{}
			maps := []map[string]string{p.Dependencies, p.DevDependencies}
			for _, m := range maps {
				for k, v := range m {
					deps[k] = v
				}
			}
			info["packages"] = deps
		}
	}
	if m, err := routes.Scan(os.DirFS(t.root), defaultRoutesDir); err == nil {
		info["routes"] = len(m.Routes)
	}
	comps := t.goComponents()
	info["liveComponents"] = len(comps)
	// Plugins and Tern packages the app imports.
	used := map[string]bool{}
	_ = filepath.WalkDir(t.root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // skip unreadable directories
		}
		if d.IsDir() && (strings.HasPrefix(d.Name(), ".") || d.Name() == "node_modules" || d.Name() == "build") {
			return filepath.SkipDir
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return nil //nolint:nilerr // skip unreadable files
		}
		for _, m := range regexp.MustCompile(`"github\.com/Unfield/Tern/([\w/]+)"`).FindAllSubmatch(raw, -1) {
			used[string(m[1])] = true
		}
		return nil
	})
	pkgs := make([]string, 0, len(used))
	for p := range used {
		pkgs = append(pkgs, p)
	}
	sort.Strings(pkgs)
	info["ternPackages"] = pkgs
	return jsonText(info)
}

type routeInfo struct {
	Pattern    string   `json:"pattern"`
	ID         string   `json:"id"`
	Dir        string   `json:"dir"`
	Kind       string   `json:"kind"`
	Files      []string `json:"files"`
	Load       bool     `json:"load,omitempty"`
	Actions    bool     `json:"actions,omitempty"`
	LayoutLoad bool     `json:"layoutLoad,omitempty"`
	Methods    []string `json:"methods,omitempty"`
	Layouts    []string `json:"layouts,omitempty"`
}

func (t *mcpTools) scanRoutes() ([]routeInfo, error) {
	m, err := routes.Scan(os.DirFS(t.root), defaultRoutesDir)
	if err != nil {
		return nil, err
	}
	out := []routeInfo{}
	for _, r := range m.Routes {
		dir := path.Join(defaultRoutesDir, r.Dir)
		ri := routeInfo{Pattern: r.Pattern(), ID: r.ID, Dir: dir, Kind: kind(r)}
		if entries, err := os.ReadDir(t.path(dir)); err == nil {
			for _, e := range entries {
				if !e.IsDir() {
					ri.Files = append(ri.Files, e.Name())
				}
			}
		}
		if r.Go != nil {
			ri.Load, ri.Actions, ri.LayoutLoad, ri.Methods = r.Go.Load, r.Go.Actions, r.Go.LayoutLoad, r.Go.Methods
		}
		for _, l := range r.Layouts {
			ri.Layouts = append(ri.Layouts, path.Join(defaultRoutesDir, l.Dir))
		}
		out = append(out, ri)
	}
	slices.SortFunc(out, func(a, b routeInfo) int { return strings.Compare(a.Pattern, b.Pattern) })
	return out, nil
}

func (t *mcpTools) routes(context.Context, json.RawMessage) (string, error) {
	list, err := t.scanRoutes()
	if err != nil {
		return "", err
	}
	return jsonText(list)
}

func (t *mcpTools) routeTypes(_ context.Context, raw json.RawMessage) (string, error) {
	a, err := mcp.Args[struct {
		Route string `json:"route"`
	}](raw)
	if err != nil {
		return "", err
	}
	list, err := t.scanRoutes()
	if err != nil {
		return "", err
	}
	want := strings.TrimSuffix(a.Route, "/")
	if want == "" {
		want = "/"
	}
	for _, r := range list {
		if r.Pattern == want || r.ID == want || r.Pattern == svelteKitToPattern(want) {
			file := t.path(path.Join(codegen.GenDir, "types", r.Dir, "$types.d.ts"))
			b, err := os.ReadFile(file)
			if err != nil {
				return "", fmt.Errorf("no generated types for %s yet; run generate", r.Pattern)
			}
			return string(b), nil
		}
	}
	return "", fmt.Errorf("no route %q; the routes tool lists them", a.Route)
}

type goComponent struct {
	Path  string `json:"path"`
	ID    string `json:"id"`
	Live  bool   `json:"live"`
	Types string `json:"types,omitempty"`
}

func (t *mcpTools) goComponents() []goComponent {
	var out []goComponent
	_ = filepath.WalkDir(t.path("src"), func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".svelte") {
			return nil //nolint:nilerr // skip unreadable files
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return nil //nolint:nilerr // skip unreadable files
		}
		m := goScriptTag.FindSubmatch(raw)
		if m == nil {
			return nil
		}
		rel, _ := filepath.Rel(t.root, p)
		rel = filepath.ToSlash(rel)
		c := goComponent{Path: rel, ID: goscript.ComponentID(rel), Live: liveAttr.Match(m[1])}
		if c.Live {
			if b, err := os.ReadFile(t.path(path.Join(codegen.LiveTypesDir, c.ID+".ts"))); err == nil {
				c.Types = string(b)
			}
		}
		out = append(out, c)
		return nil
	})
	return out
}

func (t *mcpTools) liveComponents(context.Context, json.RawMessage) (string, error) {
	comps := t.goComponents()
	if len(comps) == 0 {
		return "No components with <script lang=\"go\"> in src. new_live_component creates one.", nil
	}
	return jsonText(comps)
}

func (t *mcpTools) generate(context.Context, json.RawMessage) (string, error) {
	res, err := codegen.Generate(t.root)
	if err != nil {
		return "", err
	}
	return jsonText(map[string]any{"routes": len(res.Manifest.Routes), "changed": res.Changed, "warnings": res.Warnings, "goChanged": res.GoChanged})
}

// inRoot runs fn with the working directory set to the app's root, for
// commands that work relative to it.
func (t *mcpTools) inRoot(fn func() int) int {
	wd, _ := os.Getwd()
	if wd != t.root {
		_ = os.Chdir(t.root)
		defer func() { _ = os.Chdir(wd) }()
	}
	return fn()
}

func (t *mcpTools) check(context.Context, json.RawMessage) (string, error) {
	var out bytes.Buffer
	code := t.inRoot(func() int { return cmdCheck([]string{"--output", "human"}, &out, &out) })
	text := strings.TrimSpace(tail(out.String(), 200))
	if code != 0 {
		return "", fmt.Errorf("check failed:\n%s", text)
	}
	return text, nil
}

func (t *mcpTools) runGo(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = t.root
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(tail(string(out), 300))
	if err != nil {
		return "", fmt.Errorf("go %s failed:\n%s", strings.Join(args, " "), text)
	}
	if text == "" {
		text = "ok"
	}
	return text, nil
}

func (t *mcpTools) goVet(ctx context.Context, raw json.RawMessage) (string, error) {
	a, err := mcp.Args[struct {
		Packages string `json:"packages"`
	}](raw)
	if err != nil {
		return "", err
	}
	return t.runGo(ctx, "vet", or(a.Packages, "./..."))
}

func (t *mcpTools) goTest(ctx context.Context, raw json.RawMessage) (string, error) {
	a, err := mcp.Args[struct {
		Packages string `json:"packages"`
		Run      string `json:"run"`
	}](raw)
	if err != nil {
		return "", err
	}
	args := []string{"test", "-count=1"}
	if a.Run != "" {
		args = append(args, "-run", a.Run)
	}
	return t.runGo(ctx, append(args, or(a.Packages, "./..."))...)
}

func (t *mcpTools) build(_ context.Context, raw json.RawMessage) (string, error) {
	a, err := mcp.Args[struct {
		Output string `json:"output"`
	}](raw)
	if err != nil {
		return "", err
	}
	args := []string{}
	if a.Output != "" {
		args = append(args, "-o", a.Output)
	}
	var out bytes.Buffer
	code := t.inRoot(func() int { return cmdBuild(args, &out, &out) })
	text := strings.TrimSpace(tail(out.String(), 200))
	if code != 0 {
		return "", fmt.Errorf("build failed:\n%s", text)
	}
	return text, nil
}

func or(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func tail(s string, lines int) string {
	parts := strings.Split(s, "\n")
	if len(parts) <= lines {
		return s
	}
	return "…\n" + strings.Join(parts[len(parts)-lines:], "\n")
}

// routeDirs converts a SvelteKit-style URL path to Tern's directory names.
func routeDirs(p string) ([]string, error) {
	var out []string
	for _, seg := range strings.Split(strings.Trim(p, "/"), "/") {
		if seg == "" {
			continue
		}
		switch {
		case strings.HasPrefix(seg, "[[") && strings.HasSuffix(seg, "]]"):
			seg = paramName(seg[2:len(seg)-2]) + ".optional"
		case strings.HasPrefix(seg, "[...") && strings.HasSuffix(seg, "]"):
			seg = paramName(seg[4:len(seg)-1]) + ".rest"
		case strings.HasPrefix(seg, "[") && strings.HasSuffix(seg, "]"):
			name, matcher, _ := strings.Cut(seg[1:len(seg)-1], "=")
			seg = paramName(name) + ".param"
			if matcher != "" {
				seg += "." + matcher
			}
		case strings.HasPrefix(seg, "(") && strings.HasSuffix(seg, ")"):
			seg = paramName(seg[1:len(seg)-1]) + ".group"
		case strings.ContainsAny(seg, "[]()"):
			return nil, fmt.Errorf("can't convert the segment %q; use one parameter per segment", seg)
		}
		if seg == "" || strings.HasPrefix(seg, ".") {
			return nil, fmt.Errorf("a parameter in %q has no name", p)
		}
		out = append(out, seg)
	}
	return out, nil
}

func paramName(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			return r
		}
		return -1
	}, s)
}

// svelteKitToPattern turns /blog/[slug] into the pattern Tern shows.
func svelteKitToPattern(p string) string {
	dirs, err := routeDirs(p)
	if err != nil {
		return p
	}
	var segs []string
	for _, d := range dirs {
		switch {
		case strings.HasSuffix(d, ".group"):
			continue
		case strings.Contains(d, ".param"):
			name, _, _ := strings.Cut(d, ".")
			segs = append(segs, "["+name+"]")
		case strings.HasSuffix(d, ".optional"):
			segs = append(segs, "[["+strings.TrimSuffix(d, ".optional")+"]]")
		case strings.HasSuffix(d, ".rest"):
			segs = append(segs, "[..."+strings.TrimSuffix(d, ".rest")+"]")
		default:
			segs = append(segs, d)
		}
	}
	return "/" + strings.Join(segs, "/")
}

// goPackageName is the package name for a route directory.
func goPackageName(dirs []string) string {
	if len(dirs) == 0 {
		return "routes"
	}
	name, _, _ := strings.Cut(dirs[len(dirs)-1], ".")
	name = strings.ToLower(paramName(name))
	if name == "" || unicode.IsDigit(rune(name[0])) {
		name = "page" + name
	}
	return name
}

// writeNew writes a file unless it exists, and reports what happened.
func writeNew(p, content string) (string, error) {
	if _, err := os.Stat(p); err == nil {
		return "exists, left alone", nil
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return "", err
	}
	return "created", os.WriteFile(p, []byte(content), 0o644)
}

func (t *mcpTools) newRoute(_ context.Context, raw json.RawMessage) (string, error) {
	a, err := mcp.Args[struct {
		Path     string   `json:"path"`
		Page     *bool    `json:"page"`
		Load     bool     `json:"load"`
		Actions  bool     `json:"actions"`
		Layout   bool     `json:"layout"`
		Methods  []string `json:"methods"`
		LivePage bool     `json:"livePage"`
	}](raw)
	if err != nil {
		return "", err
	}
	dirs, err := routeDirs(a.Path)
	if err != nil {
		return "", err
	}
	// A page unless only endpoint methods were asked for.
	page := len(a.Methods) == 0 || a.Load || a.Actions || a.LivePage
	if a.Page != nil {
		page = *a.Page
	}
	dir := path.Join(append([]string{defaultRoutesDir}, dirs...)...)
	pkg := goPackageName(dirs)
	var report []string
	add := func(rel, content string) error {
		what, err := writeNew(t.path(rel), content)
		if err != nil {
			return err
		}
		report = append(report, rel+": "+what)
		return nil
	}
	title := strings.Trim(a.Path, "/")
	if title == "" {
		title = "Home"
	}
	if page {
		var body string
		switch {
		case a.LivePage:
			body = liveTemplate(title)
		case a.Load || a.Actions:
			body = "<script lang=\"ts\">\n\timport type { PageProps } from './$types';\n\n\tlet { data, form }: PageProps = $props();\n</script>\n\n<svelte:head><title>" + title + "</title></svelte:head>\n\n<h1>" + title + "</h1>\n"
			if a.Actions {
				body += "\n<form method=\"POST\">\n\t<input name=\"name\" aria-label=\"Name\" />\n\t<button>Save</button>\n\t{#if form?.error}<p role=\"alert\">{form.error}</p>{/if}\n</form>\n"
			}
		default:
			body = "<svelte:head><title>" + title + "</title></svelte:head>\n\n<h1>" + title + "</h1>\n"
		}
		if err := add(dir+"/+page.svelte", body); err != nil {
			return "", err
		}
	}
	if a.Layout {
		if err := add(dir+"/+layout.svelte", "<script lang=\"ts\">\n\timport type { LayoutProps } from './$types';\n\n\tlet { children }: LayoutProps = $props();\n</script>\n\n{@render children()}\n"); err != nil {
			return "", err
		}
	}
	if a.Load || a.Actions {
		if err := add(dir+"/page.server.go", pageGo(pkg, a.Load, a.Actions)); err != nil {
			return "", err
		}
	}
	if len(a.Methods) > 0 {
		var b strings.Builder
		fmt.Fprintf(&b, "package %s\n\nimport \"github.com/Unfield/Tern/kit\"\n", pkg)
		for _, m := range a.Methods {
			m = strings.ToUpper(m)
			if !slices.Contains(routes.HTTPMethods, m) {
				return "", fmt.Errorf("%q is not an HTTP method Tern routes", m)
			}
			fmt.Fprintf(&b, "\n// %s handles %s %s.\nfunc %s(ctx *kit.Ctx) error {\n\treturn ctx.JSON(200, map[string]string{\"ok\": \"yes\"})\n}\n", m, m, svelteKitToPattern(a.Path), m)
		}
		if err := add(dir+"/endpoint.go", b.String()); err != nil {
			return "", err
		}
	}
	report = append(report, "", "URL: "+svelteKitToPattern(a.Path), "Run generate to update tern.gen.go and the types.")
	return strings.Join(report, "\n"), nil
}

func pageGo(pkg string, load, actions bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "package %s\n\nimport \"github.com/Unfield/Tern/kit\"\n", pkg)
	if load {
		b.WriteString(`
// Page is the data +page.svelte receives as data.
type Page struct {
	Message string ` + "`json:\"message\"`" + `
}

func Load(ctx *kit.Ctx) (Page, error) {
	return Page{Message: "Hello from Go"}, nil
}
`)
	}
	if actions {
		b.WriteString(`
// Result is what +page.svelte receives as form after the action.
type Result struct {
	Saved bool   ` + "`json:\"saved,omitempty\"`" + `
	Error string ` + "`json:\"error,omitempty\"`" + `
}

var Actions = kit.Actions{
	"default": kit.Action(func(ctx *kit.Ctx) (Result, error) {
		form, err := ctx.Form()
		if err != nil {
			return Result{}, err
		}
		if form.Get("name") == "" {
			return Result{}, kit.Fail(400, Result{Error: "Enter a name."})
		}
		return Result{Saved: true}, nil
	}),
}
`)
	}
	return b.String()
}

func liveTemplate(title string) string {
	return `<script lang="go" live>
import (
	"strings"

	"github.com/Unfield/Tern/goscript"
)

func Setup(c *goscript.Component) {
	// Check access first: c.Locals() holds what your hooks set, such as the user.
	name := goscript.State(c, "name", "", goscript.Bind)
	count := goscript.State(c, "count", 0)
	goscript.Derived(c, "greeting", func() string {
		if strings.TrimSpace(name.Get()) == "" {
			return "Hello!"
		}
		return "Hello, " + name.Get() + "!"
	})
	c.Func("increment", func() int {
		count.Update(func(n int) int { return n + 1 })
		return count.Peek()
	})
}
</script>

<svelte:head><title>` + title + `</title></svelte:head>

<h1>{greeting}</h1>
<input bind:value={live.bind.name} aria-label="Your name" />
<button onclick={increment} disabled={increment.pending}>Clicked {count} times</button>
`
}

func (t *mcpTools) newLiveComponent(_ context.Context, raw json.RawMessage) (string, error) {
	a, err := mcp.Args[struct {
		Path string `json:"path"`
	}](raw)
	if err != nil {
		return "", err
	}
	rel := path.Clean(filepath.ToSlash(a.Path))
	if !strings.HasPrefix(rel, "src/") || !strings.HasSuffix(rel, ".svelte") || strings.Contains(rel, "..") {
		return "", errors.New("the path must be a .svelte file under src/, such as src/lib/Counter.svelte")
	}
	name := strings.TrimSuffix(path.Base(rel), ".svelte")
	tpl := strings.Replace(liveTemplate(name), "<svelte:head><title>"+name+"</title></svelte:head>\n\n", "", 1)
	what, err := writeNew(t.path(rel), tpl)
	if err != nil {
		return "", err
	}
	return rel + ": " + what + "\nImport it from a page, then run generate (or tern dev) to compile its Go into the app. See read_doc live-components.md.", nil
}

func (t *mcpTools) newMigration(_ context.Context, raw json.RawMessage) (string, error) {
	a, err := mcp.Args[struct {
		Name string `json:"name"`
	}](raw)
	if err != nil {
		return "", err
	}
	var out bytes.Buffer
	code := t.inRoot(func() int { return cmdMigrate([]string{"new", a.Name}, &out, &out) })
	if code != 0 {
		return "", errors.New(strings.TrimSpace(out.String()))
	}
	return strings.TrimSpace(out.String()), nil
}

func (t *mcpTools) uiAdd(_ context.Context, raw json.RawMessage) (string, error) {
	a, err := mcp.Args[struct {
		Components []string `json:"components"`
	}](raw)
	if err != nil {
		return "", err
	}
	args := []string{"list"}
	if len(a.Components) > 0 {
		args = append([]string{"add"}, a.Components...)
	}
	var out bytes.Buffer
	if code := t.inRoot(func() int { return cmdUI(args, &out, &out) }); code != 0 {
		return "", errors.New(strings.TrimSpace(out.String()))
	}
	return strings.TrimSpace(out.String()), nil
}

func docNames() []string {
	var names []string
	_ = fs.WalkDir(docs.FS, ".", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(p, ".md") {
			names = append(names, p)
		}
		return nil
	})
	sort.Strings(names)
	return names
}

func docTitle(name string) string {
	b, err := fs.ReadFile(docs.FS, name)
	if err != nil {
		return name
	}
	first, _, _ := strings.Cut(string(b), "\n")
	return strings.TrimSpace(strings.TrimLeft(first, "# "))
}

func (t *mcpTools) readDoc(_ context.Context, raw json.RawMessage) (string, error) {
	a, err := mcp.Args[struct {
		Name string `json:"name"`
	}](raw)
	if err != nil {
		return "", err
	}
	if a.Name == "" {
		var b strings.Builder
		for _, n := range docNames() {
			fmt.Fprintf(&b, "%s: %s\n", n, docTitle(n))
		}
		return b.String(), nil
	}
	name := strings.TrimPrefix(strings.TrimPrefix(a.Name, "tern://docs/"), "docs/")
	b, err := fs.ReadFile(docs.FS, name)
	if err != nil {
		return "", fmt.Errorf("no doc %q; call read_doc without a name to list them", a.Name)
	}
	return string(b), nil
}

type docSection struct {
	file, heading, text string
}

func docSections() []docSection {
	var out []docSection
	for _, name := range docNames() {
		b, _ := fs.ReadFile(docs.FS, name)
		heading := docTitle(name)
		var cur strings.Builder
		flush := func() {
			if strings.TrimSpace(cur.String()) != "" {
				out = append(out, docSection{name, heading, cur.String()})
			}
			cur.Reset()
		}
		inCode := false
		for _, line := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(line, "```") {
				inCode = !inCode
			}
			if !inCode && (strings.HasPrefix(line, "## ") || strings.HasPrefix(line, "### ")) {
				flush()
				heading = strings.TrimSpace(strings.TrimLeft(line, "# "))
			}
			cur.WriteString(line)
			cur.WriteByte('\n')
		}
		flush()
	}
	return out
}

func (t *mcpTools) searchDocs(_ context.Context, raw json.RawMessage) (string, error) {
	a, err := mcp.Args[struct {
		Query string `json:"query"`
	}](raw)
	if err != nil {
		return "", err
	}
	terms := strings.Fields(strings.ToLower(a.Query))
	if len(terms) == 0 {
		return "", errors.New("give a query")
	}
	type hit struct {
		s     docSection
		score int
	}
	var hits []hit
	for _, s := range docSections() {
		text, head := strings.ToLower(s.text), strings.ToLower(s.heading)
		score := 0
		for _, term := range terms {
			n := strings.Count(text, term)
			if n == 0 {
				continue
			}
			score += min(n, 10) + 10*strings.Count(head, term) + 5
		}
		if score > 0 {
			hits = append(hits, hit{s, score})
		}
	}
	if len(hits) == 0 {
		return "Nothing matched. read_doc without a name lists the docs.", nil
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].score > hits[j].score })
	var b strings.Builder
	for i, h := range hits {
		if i == 5 {
			break
		}
		text := h.s.text
		if len(text) > 4000 {
			text = text[:4000] + "\n…(read_doc " + h.s.file + " for the rest)"
		}
		fmt.Fprintf(&b, "=== %s § %s\n%s\n", h.s.file, h.s.heading, strings.TrimSpace(text))
	}
	return b.String(), nil
}

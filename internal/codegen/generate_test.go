package codegen

import (
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "update golden files")

// fixture is a small app exercising every route feature.
var fixture = map[string]string{
	"main.go": `package main

import "log"

func main() {
	app, err := ternApp()
	if err != nil {
		log.Fatal(err)
	}
	log.Fatal(app.ListenAndServe())
}
`,
	"src/app.html":              `<!doctype html><html><head>%tern.head%</head><body>%tern.body%</body></html>`,
	"src/routes/+layout.svelte": ``,
	"src/routes/+error.svelte":  ``,
	"src/routes/+page.svelte":   ``,
	"src/routes/layout.server.go": `package routes

import (
	"time"

	"github.com/Unfield/Tern/kit"
)

type User struct {
	ID       int       ` + "`json:\"id\"`" + `
	Name     string    ` + "`json:\"name\"`" + `
	Email    string    ` + "`json:\"email,omitempty\"`" + `
	Joined   time.Time ` + "`json:\"joined\"`" + `
	password string
}

type Session struct {
	User *User ` + "`json:\"user\"`" + `
}

func LayoutLoad(ctx *kit.Ctx) (Session, error) { return Session{}, nil }
`,
	"src/routes/blog/+layout.svelte":           ``,
	"src/routes/blog/slug.param/+page.svelte":  ``,
	"src/routes/blog/slug.param/+error.svelte": ``,
	"src/routes/blog/slug.param/page.server.go": `package page

import "github.com/Unfield/Tern/kit"

type Base struct {
	Title string ` + "`json:\"title\"`" + `
}

// Comment is recursive to check named declarations.
type Comment struct {
	Body    string    ` + "`json:\"body\"`" + `
	Replies []Comment ` + "`json:\"replies\"`" + `
}

type Post struct {
	Base
	Slug     string            ` + "`json:\"slug\"`" + `
	Views    int64             ` + "`json:\"views,string\"`" + `
	Tags     []string          ` + "`json:\"tags\"`" + `
	Meta     map[string]any    ` + "`json:\"meta\"`" + `
	Comments []Comment         ` + "`json:\"comments\"`" + `
	Draft    *bool             ` + "`json:\"draft,omitzero\"`" + `
	Internal string            ` + "`json:\"-\"`" + `
	Raw      []byte            ` + "`json:\"raw\"`" + `
	Scores   [3]float64        ` + "`json:\"scores\"`" + `
	ByID     map[int]string    ` + "`json:\"by_id\"`" + `
	Odd      string            ` + "`json:\"odd-name\"`" + `
	Untagged bool
	Related  kit.Deferred[[]Base] ` + "`json:\"related\"`" + `
}

type Saved struct {
	OK bool ` + "`json:\"ok\"`" + `
}

type Invalid struct {
	Errors map[string]string ` + "`json:\"errors\"`" + `
}

func Load(ctx *kit.Ctx) (*Post, error) { return &Post{}, nil }

var Actions = kit.Actions{
	"save": kit.Action(save),
	"raw":  func(ctx *kit.Ctx) (any, error) { return nil, nil },
}

func save(ctx *kit.Ctx) (Saved, error) {
	if ctx.Param("slug") == "" {
		return Saved{}, kit.Fail(400, Invalid{})
	}
	return Saved{OK: true}, nil
}
`,
	"src/routes/docs/path.rest/+page.svelte": ``,
	"src/routes/docs/path.rest/page.server.go": `package docs

const Prerender = true

func Entries() ([]map[string]string, error) {
	return []map[string]string{{"path": "intro"}}, nil
}
`,
	"src/routes/lang.optional/about/+page.svelte": ``,
	"src/routes/news/+page.svelte":                ``,
	"src/routes/news/page.server.go": `package news

const Revalidate = 60

const CacheControl = "public, max-age=60"

const CSR = false
`,
	"src/routes/news/latest/+page.svelte": ``,
	"src/routes/news/latest/page.server.go": `package latest

import "time"

const Revalidate = 5 * time.Minute
`,
	"src/routes/api/items/id.param.integer/server.go": `package items

import "github.com/Unfield/Tern/kit"

func GET(ctx *kit.Ctx) error    { return nil }
func DELETE(ctx *kit.Ctx) error { return nil }
`,
	"src/routes/api/ask/server.go": `package ask

import "github.com/Unfield/Tern/kit"

type Ask struct {
	Question string ` + "`json:\"question\"`" + `
}

type Delta struct {
	Text string ` + "`json:\"text\"`" + `
}

func POST(ctx *kit.Ctx, in Ask, out *kit.Stream[Delta]) error { return out.Send(Delta{Text: in.Question}) }

func GET(ctx *kit.Ctx, out *kit.Stream[Delta]) error { return nil }
`,
}

func writeFixture(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	_, here, _, _ := runtime.Caller(0)
	repo := filepath.Join(filepath.Dir(here), "..", "..")
	sum, err := os.ReadFile(filepath.Join(repo, "go.sum"))
	if err != nil {
		t.Fatal(err)
	}
	all := map[string]string{
		"go.mod": "module example.com/app\n\ngo 1.26\n\nrequire github.com/Unfield/Tern v0.0.0\n\nreplace github.com/Unfield/Tern => " + strconv.Quote(filepath.ToSlash(repo)) + "\n",
		"go.sum": string(sum),
	}
	for k, v := range files {
		all[k] = v
	}
	for name, content := range all {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	tidy := exec.Command("go", "mod", "tidy", "-e")
	tidy.Dir = dir
	tidy.Env = append(os.Environ(), "GOFLAGS=-mod=mod", "GOPROXY=off")
	if out, err := tidy.CombinedOutput(); err != nil {
		t.Fatalf("go mod tidy: %v\n%s", err, out)
	}
	return dir
}

func TestGenerate(t *testing.T) {
	dir := writeFixture(t, fixture)
	res, err := Generate(dir)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !res.GoChanged || len(res.Changed) == 0 {
		t.Errorf("first run should report changes: %+v", res)
	}

	golden := []string{
		"tern.gen.go",
		".tern/generated/manifest.js",
		".tern/generated/client.js",
		".tern/generated/routes.d.ts",
		".tern/types/src/routes/$types.d.ts",
		".tern/types/src/routes/blog/$types.d.ts",
		".tern/types/src/routes/blog/slug.param/$types.d.ts",
		".tern/types/src/routes/lang.optional/about/$types.d.ts",
		".tern/types/src/routes/api/items/id.param.integer/$types.d.ts",
		".tern/generated/api.d.ts",
		".tern/generated/openapi.json",
	}
	for _, name := range golden {
		got, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
		if err != nil {
			t.Errorf("missing %s: %v", name, err)
			continue
		}
		goldenPath := filepath.Join("testdata", "golden", strings.ReplaceAll(strings.TrimPrefix(name, "."), "/", "_")+".golden")
		if *update {
			if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(goldenPath, got, 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		want, err := os.ReadFile(goldenPath)
		if err != nil {
			t.Errorf("reading golden file (run go test -update): %v", err)
			continue
		}
		if string(got) != string(want) {
			t.Errorf("%s differs from %s; run go test ./internal/codegen -update and review the diff\n--- got ---\n%s", name, goldenPath, got)
		}
	}

	// The generated server must compile and pass vet.
	cmd := exec.Command("go", "vet", "./...")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go vet on the generated app failed: %v\n%s", err, out)
	}

	// A second run changes nothing.
	res, err = Generate(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Changed) != 0 || res.GoChanged {
		t.Errorf("second run should change nothing, changed %v", res.Changed)
	}

	// Removing a route removes its types.
	if err := os.RemoveAll(filepath.Join(dir, "src/routes/lang.optional")); err != nil {
		t.Fatal(err)
	}
	if _, err := Generate(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".tern/types/src/routes/lang.optional/about/$types.d.ts")); !os.IsNotExist(err) {
		t.Errorf("stale types were not removed: %v", err)
	}
}

func TestGenerateSignatureErrors(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{"load without ctx", "func Load() (map[string]int, error) { return nil, nil }", "Load must have the signature func(ctx *kit.Ctx) (T, error)"},
		{"load returning a scalar", "func Load(ctx *kit.Ctx) (int, error) { return 0, nil }", "where T is a struct or map"},
		{"handler without error", "func GET(ctx *kit.Ctx) {}", "GET must be func(ctx *kit.Ctx) error"},
		{"actions of the wrong type", "var Actions = map[string]int{}", "Actions must be a kit.Actions"},
		{"prerender of the wrong type", "const Prerender = 1", "Prerender must be a boolean constant"},
		{"entries with a bad signature", "func Entries() []string { return nil }", "Entries must have the signature func(ctx *kit.Ctx) ([]map[string]string, error)"},
		{"compile error", "func Load(ctx *kit.Ctx) (map[string]int, error) { return nope, nil }", "undefined: nope"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := writeFixture(t, map[string]string{
				"src/app.html":            "%tern.head%%tern.body%",
				"src/routes/+page.svelte": "",
				"src/routes/page.go":      "package routes\n\nimport \"github.com/Unfield/Tern/kit\"\n\nvar _ *kit.Ctx\n\n" + tt.src + "\n",
			})
			_, err := Generate(dir)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want it to contain %q", err, tt.want)
			}
			if !strings.Contains(err.Error(), "page.go:") {
				t.Errorf("error should point at the source file: %v", err)
			}
		})
	}
}

func TestGenerateUntidyModule(t *testing.T) {
	dir := writeFixture(t, map[string]string{
		"src/app.html":            "%tern.head%%tern.body%",
		"src/routes/+page.svelte": "",
		"src/routes/page.go":      "package routes\n\nimport (\n\t\"github.com/Unfield/Tern/kit\"\n\t\"example.com/missing\"\n)\n\nvar _ = missing.X\n\nfunc Load(ctx *kit.Ctx) (map[string]int, error) { return nil, nil }\n",
	})
	_, err := Generate(dir)
	if err == nil || !strings.Contains(err.Error(), "example.com/missing") {
		t.Fatalf("err = %v, want the go list error naming the missing module", err)
	}
}

func TestGenerateRequiresAppHTML(t *testing.T) {
	dir := writeFixture(t, map[string]string{"src/routes/+page.svelte": ""})
	if _, err := Generate(dir); err == nil || !strings.Contains(err.Error(), "src/app.html is missing") {
		t.Fatalf("err = %v", err)
	}
}

func TestGenerateMarkdown(t *testing.T) {
	dir := writeFixture(t, map[string]string{
		"src/app.html":                    "<html><head>%tern.head%</head><body>%tern.body%</body></html>",
		"main.go":                         "package main\n\nfunc main() {}\n",
		"src/routes/about/page.server.go": "package about\n\nimport \"github.com/Unfield/Tern/kit\"\n\nfunc Load(ctx *kit.Ctx) (struct{}, error) { return struct{}{}, nil }\n",
		"src/routes/about/+page.md":       "---\ntitle: About us\ndescription: Who we are\n---\n# About\n\nWe use `{braces}` and <b>html</b>.\n",
		"src/content/blog/post.md":        "---\ntitle: Post\n---\nHi\n",
		"src/routes/both/+page.md":        "x",
		"src/routes/both/+page.svelte":    "<p>x</p>",
	})
	if _, err := Generate(dir); err == nil || !strings.Contains(err.Error(), "not both") {
		t.Fatalf("a directory with +page.md and +page.svelte: %v", err)
	}
	if err := os.Remove(filepath.Join(dir, "src/routes/both/+page.svelte")); err != nil {
		t.Fatal(err)
	}
	if _, err := Generate(dir); err != nil {
		t.Fatal(err)
	}
	page, err := os.ReadFile(filepath.Join(dir, ".tern/generated/pages/about/page.svelte"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`export const metadata = {"description":"Who we are","title":"About us"};`,
		"<title>{metadata.title}</title>",
		`<meta name="description" content={metadata.description} />`,
		`\u003ccode\u003e{braces}\u003c/code\u003e`,
		"{@html html}",
	} {
		if !strings.Contains(string(page), want) {
			t.Errorf("missing %q in\n%s", want, page)
		}
	}
	manifest, _ := os.ReadFile(filepath.Join(dir, ".tern/generated/manifest.js"))
	if !strings.Contains(string(manifest), `import("../../.tern/generated/pages/about/page.svelte")`) {
		t.Errorf("the client manifest imports the generated page:\n%s", manifest)
	}
	goFile, _ := os.ReadFile(filepath.Join(dir, GoFile))
	for _, want := range []string{"//go:embed src/content", "ternManifest.Content = content"} {
		if !strings.Contains(string(goFile), want) {
			t.Errorf("missing %q in %s", want, GoFile)
		}
	}
	build := exec.Command("go", "build", "-o", os.DevNull, ".")
	build.Dir = dir
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("the generated code builds: %v\n%s", err, out)
	}
}

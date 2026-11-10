package content_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/Unfield/Tern/content"
	"github.com/Unfield/Tern/kit"
)

type post struct {
	Title string    `yaml:"title" json:"title"`
	Date  time.Time `yaml:"date" json:"date"`
	Tags  []string  `yaml:"tags" json:"tags"`
}

var files = fstest.MapFS{
	"blog/first.md":          {Data: []byte("---\ntitle: First\ndate: 2026-01-01\n---\nHello **world**.\n\n## Part one\n")},
	"blog/second.md":         {Data: []byte("---\ntitle: Second\ndate: 2026-03-01\ntags: [go]\n---\nAnother post.\n")},
	"blog/draft.md":          {Data: []byte("---\ntitle: Draft\ndate: 2026-04-01\ndraft: true\n---\nNot yet.\n")},
	"blog/_partial.md":       {Data: []byte("ignored")},
	"blog/notes.txt":         {Data: []byte("ignored")},
	"blog/guides/index.md":   {Data: []byte("---\ntitle: Guides\ndate: 2025-01-01\n---\nAll guides.\n")},
	"blog/guides/install.md": {Data: []byte("---\ntitle: Install\ndate: 2025-02-01\n---\nRun it.\n")},
	"broken/bad.md":          {Data: []byte("---\ntitle: [unclosed\n---\nx")},
}

var posts = &content.Collection[post]{Dir: "blog", Sort: content.Newest(func(p post) time.Time { return p.Date })}

func app(t *testing.T) *kit.App {
	t.Helper()
	broken := &content.Collection[post]{Dir: "broken"}
	missing := &content.Collection[post]{Dir: "nope"}
	m := &kit.Manifest{
		AppHTML: `<html><head>%tern.head%</head><body>%tern.body%</body></html>`,
		Content: files,
		Routes: []kit.RouteDef{
			{ID: "/all", Endpoint: map[string]kit.HandlerFunc{"GET": func(c *kit.Ctx) error {
				all, err := posts.Summaries(c)
				if err != nil {
					return err
				}
				return c.JSON(200, all)
			}}},
			{ID: "/get", Endpoint: map[string]kit.HandlerFunc{"GET": func(c *kit.Ctx) error {
				e, err := posts.Get(c, c.Request.URL.Query().Get("slug"))
				if err != nil {
					return err
				}
				return c.JSON(200, e)
			}}},
			{ID: "/broken", Endpoint: map[string]kit.HandlerFunc{"GET": func(c *kit.Ctx) error {
				_, err := broken.All(c)
				if err == nil {
					return c.Text(200, "no error")
				}
				return c.Text(500, err.Error())
			}}},
			{ID: "/missing", Endpoint: map[string]kit.HandlerFunc{"GET": func(c *kit.Ctx) error {
				_, err := missing.All(c)
				return c.Text(500, err.Error())
			}}},
		},
	}
	a, err := kit.New(m)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func get(t *testing.T, a *kit.App, url string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	a.ServeHTTP(w, httptest.NewRequest(http.MethodGet, url, nil))
	return w
}

func TestCollection(t *testing.T) {
	a := app(t)
	var all []content.Summary[post]
	if err := json.Unmarshal(get(t, a, "/all").Body.Bytes(), &all); err != nil {
		t.Fatal(err)
	}
	var slugs []string
	for _, s := range all {
		slugs = append(slugs, s.Slug)
	}
	if strings.Join(slugs, ",") != "second,first,guides/install,guides" {
		t.Errorf("slugs = %v (newest first, drafts and partials left out)", slugs)
	}
	if all[0].Data.Tags[0] != "go" || all[1].Excerpt != "Hello world." {
		t.Errorf("summaries = %+v", all[:2])
	}

	var e content.Entry[post]
	if err := json.Unmarshal(get(t, a, "/get?slug=first").Body.Bytes(), &e); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(e.HTML, "<strong>world</strong>") || len(e.Headings) != 1 || e.Data.Title != "First" {
		t.Errorf("entry = %+v", e)
	}
	if w := get(t, a, "/get?slug=draft"); w.Code != 404 {
		t.Errorf("drafts are hidden in production: %d", w.Code)
	}
	if w := get(t, a, "/get?slug=nope"); w.Code != 404 {
		t.Errorf("missing entry: %d", w.Code)
	}
	if w := get(t, a, "/broken"); !strings.Contains(w.Body.String(), "src/content/broken/bad.md") {
		t.Errorf("broken front matter names the file: %s", w.Body)
	}
	if w := get(t, a, "/missing"); !strings.Contains(w.Body.String(), "no directory src/content/nope") {
		t.Errorf("missing directory: %s", w.Body)
	}
}

func TestRender(t *testing.T) {
	html, err := content.Render("**hi** <script>x</script>")
	if err != nil || !strings.Contains(html, "<strong>hi</strong>") || strings.Contains(html, "<script") {
		t.Errorf("Render = %q, %v", html, err)
	}
}

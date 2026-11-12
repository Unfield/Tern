// Package docs makes the site's documentation from the Markdown in the
// repository: docs/, the README, the changelog and the decision records.
// Links between the files become links between pages; links to other files
// go to GitHub.
package docs

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	gmhtml "github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// RepoURL is the repository on GitHub.
const RepoURL = "https://github.com/Unfield/Tern"

// Page is one page of the docs.
type Page struct {
	Source  string `json:"source"` // repository-relative path
	Slug    string `json:"slug"`   // below /docs/, "" for the introduction
	Title   string `json:"title"`
	Section string `json:"section"`
	// Summary is a sentence under the title.
	Summary string `json:"summary,omitempty"`
}

// URL is the page's path on the site.
func (p *Page) URL() string {
	if p.Slug == "" {
		return "/docs"
	}
	return "/docs/" + p.Slug
}

// Heading is an h2 or h3, for the table of contents.
type Heading struct {
	ID    string `json:"id"`
	Text  string `json:"text"`
	Level int    `json:"level"`
}

// Doc is a rendered page.
type Doc struct {
	Page
	HTML     string    `json:"html"`
	Headings []Heading `json:"headings"`
	Edit     string    `json:"edit"`
	Prev     *Link     `json:"prev,omitempty"`
	Next     *Link     `json:"next,omitempty"`
	// Words is the page's length, for the reading time.
	Words int `json:"words"`
	// text is the plain text, for search.
	text string
}

// Link is a link to another page.
type Link struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

// Section is a group of pages in the sidebar.
type Section struct {
	Name  string `json:"name"`
	Links []Link `json:"links"`
}

// Root is the repository, relative to the site: the app runs in website/.
var Root = ".."

var (
	once    sync.Once
	pages   []*Page
	bySrc   map[string]*Page
	bySlug  map[string]*Page
	loadErr error
)

func list() ([]*Page, error) {
	once.Do(func() {
		pages = []*Page{
			{Source: "README.md", Slug: "", Title: "Introduction", Section: "Getting started", Summary: "What Tern is, and how its pieces fit."},
			{Source: "docs/guide.md", Slug: "guide", Title: "Guide", Section: "Getting started", Summary: "Everything a Tern app is made of, from routes to realtime."},
			{Source: "docs/migrating-from-sveltekit.md", Slug: "migrating-from-sveltekit", Title: "Coming from SvelteKit", Section: "Getting started"},
			{Source: "docs/tutorial/README.md", Slug: "tutorial", Title: "Tutorial", Section: "Tutorial", Summary: "Build a reading list with accounts, tests and a deploy, one step per page."},
			{Source: "docs/tutorial/01-create.md", Slug: "tutorial/create", Title: "1. Create the app", Section: "Tutorial"},
			{Source: "docs/tutorial/02-first-page.md", Slug: "tutorial/first-page", Title: "2. Your first page", Section: "Tutorial"},
			{Source: "docs/tutorial/03-database.md", Slug: "tutorial/database", Title: "3. A database", Section: "Tutorial"},
			{Source: "docs/tutorial/04-forms.md", Slug: "tutorial/forms", Title: "4. Saving links", Section: "Tutorial"},
			{Source: "docs/tutorial/05-link-pages.md", Slug: "tutorial/link-pages", Title: "5. A page for each link", Section: "Tutorial"},
			{Source: "docs/tutorial/06-filters.md", Slug: "tutorial/filters", Title: "6. Filtering", Section: "Tutorial"},
			{Source: "docs/tutorial/07-accounts.md", Slug: "tutorial/accounts", Title: "7. Accounts", Section: "Tutorial"},
			{Source: "docs/tutorial/08-tests.md", Slug: "tutorial/tests", Title: "8. Tests", Section: "Tutorial"},
			{Source: "docs/tutorial/09-ship.md", Slug: "tutorial/ship", Title: "9. Ship it", Section: "Tutorial"},
			{Source: "docs/accounts.md", Slug: "accounts", Title: "Accounts", Section: "Building"},
			{Source: "docs/billing.md", Slug: "billing", Title: "Billing", Section: "Building"},
			{Source: "docs/flags.md", Slug: "flags", Title: "Feature flags", Section: "Building"},
			{Source: "docs/search.md", Slug: "search", Title: "Search", Section: "Building"},
			{Source: "docs/admin.md", Slug: "admin", Title: "Admin screens", Section: "Building"},
			{Source: "docs/ui.md", Slug: "ui", Title: "Tern UI", Section: "Building"},
			{Source: "docs/live-components.md", Slug: "live-components", Title: "Live components", Section: "Building"},
			{Source: "docs/logging.md", Slug: "logging", Title: "Logging", Section: "Building"},
			{Source: "docs/caching.md", Slug: "caching", Title: "Caching", Section: "Building"},
			{Source: "docs/tracing.md", Slug: "tracing", Title: "Tracing", Section: "Building"},
			{Source: "docs/errors.md", Slug: "errors", Title: "Error reports", Section: "Building"},
			{Source: "docs/editors.md", Slug: "editors", Title: "Editors", Section: "Building"},
			{Source: "docs/dev-tools.md", Slug: "dev-tools", Title: "Dev tools", Section: "Building"},
			{Source: "docs/testing.md", Slug: "testing", Title: "Testing", Section: "Building"},
			{Source: "docs/signals.md", Slug: "signals", Title: "Security signals", Section: "Building"},
			{Source: "docs/mcp-server.md", Slug: "assistants", Title: "Apps for assistants (MCP)", Section: "Building"},
			{Source: "docs/mcp.md", Slug: "mcp", Title: "AI coding tools (MCP)", Section: "Building"},
			{Source: "docs/experimental-go-script.md", Slug: "go-script", Title: "Go in components", Section: "Building"},
			{Source: "docs/deployment.md", Slug: "deployment", Title: "Deployment", Section: "Shipping"},
			{Source: "docs/benchmarks.md", Slug: "benchmarks", Title: "Benchmarks", Section: "Shipping"},
			{Source: "docs/stability.md", Slug: "stability", Title: "Stability", Section: "Shipping"},
			{Source: "docs/ARCHITECTURE.md", Slug: "architecture", Title: "Architecture", Section: "Project"},
			{Source: "docs/ROADMAP.md", Slug: "roadmap", Title: "Roadmap", Section: "Project"},
			{Source: "CHANGELOG.md", Slug: "changelog", Title: "Changelog", Section: "Project"},
			{Source: "CONTRIBUTING.md", Slug: "contributing", Title: "Contributing", Section: "Project"},
			{Source: "docs/adr/README.md", Slug: "decisions", Title: "Decision records", Section: "Decisions"},
		}
		adrs, err := filepath.Glob(filepath.Join(Root, "docs", "adr", "0*.md"))
		if err != nil {
			loadErr = err
			return
		}
		sort.Strings(adrs)
		for _, f := range adrs {
			name := strings.TrimSuffix(filepath.Base(f), ".md")
			src, err := os.ReadFile(f)
			if err != nil {
				loadErr = err
				return
			}
			title := name
			if m := titleRe.FindSubmatch(src); m != nil {
				title = string(m[1])
			}
			pages = append(pages, &Page{Source: "docs/adr/" + filepath.Base(f), Slug: "decisions/" + name, Title: title, Section: "Decisions"})
		}
		bySrc, bySlug = map[string]*Page{}, map[string]*Page{}
		for _, p := range pages {
			bySrc[p.Source] = p
			bySlug[p.Slug] = p
		}
	})
	return pages, loadErr
}

var titleRe = regexp.MustCompile(`(?m)^#\s+(.+)$`)

// Slugs lists the pages' slugs under prefix ("" for all), for prerendering.
func Slugs(prefix string) ([]string, error) {
	ps, err := list()
	if err != nil {
		return nil, err
	}
	var out []string
	for _, p := range ps {
		if p.Slug != "" && strings.HasPrefix(p.Slug, prefix) {
			out = append(out, strings.TrimPrefix(p.Slug, prefix))
		}
	}
	return out, nil
}

// Nav is the sidebar.
func Nav() ([]Section, error) {
	ps, err := list()
	if err != nil {
		return nil, err
	}
	var out []Section
	for _, p := range ps {
		if len(out) == 0 || out[len(out)-1].Name != p.Section {
			out = append(out, Section{Name: p.Section})
		}
		s := &out[len(out)-1]
		s.Links = append(s.Links, Link{Title: shortTitle(p), URL: p.URL()})
	}
	return out, nil
}

// shortTitle is a decision record's number and name: "0013 · Native
// rendering, phase 2".
func shortTitle(p *Page) string {
	if strings.HasPrefix(p.Slug, "decisions/") {
		num, rest, _ := strings.Cut(p.Title, ". ")
		if rest != "" {
			return num + " · " + rest
		}
	}
	return p.Title
}

// ErrNotFound is returned for a slug with no page.
var ErrNotFound = errors.New("docs: no such page")

var (
	cacheMu sync.Mutex
	cache   = map[string]*Doc{}
)

// Get renders the page at slug.
func Get(slug string) (*Doc, error) {
	ps, err := list()
	if err != nil {
		return nil, err
	}
	p, ok := bySlug[slug]
	if !ok {
		return nil, ErrNotFound
	}
	cacheMu.Lock()
	defer cacheMu.Unlock()
	if d, ok := cache[slug]; ok && os.Getenv("TERN_DEV_VITE_URL") == "" {
		return d, nil
	}
	d, err := render(p)
	if err != nil {
		return nil, err
	}
	for i, q := range ps {
		if q != p {
			continue
		}
		if i > 0 {
			d.Prev = &Link{Title: shortTitle(ps[i-1]), URL: ps[i-1].URL()}
		}
		if i+1 < len(ps) {
			d.Next = &Link{Title: shortTitle(ps[i+1]), URL: ps[i+1].URL()}
		}
	}
	cache[slug] = d
	return d, nil
}

func render(p *Page) (*Doc, error) {
	src, err := os.ReadFile(filepath.Join(Root, filepath.FromSlash(p.Source)))
	if err != nil {
		return nil, err
	}
	if p.Source == "README.md" {
		src = trimReadme(src)
	}
	// The page's title is shown by the page itself.
	if loc := titleRe.FindIndex(src); loc != nil && bytes.TrimSpace(src[:loc[0]]) == nil {
		src = src[loc[1]:]
	}
	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM, highlighting.NewHighlighting(
			highlighting.WithFormatOptions(html.WithClasses(true)),
			highlighting.WithGuessLanguage(false),
		)),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
			parser.WithASTTransformers(util.Prioritized(&linker{from: p.Source}, 100)),
		),
		goldmark.WithRendererOptions(gmhtml.WithUnsafe()),
	)
	doc := md.Parser().Parse(text.NewReader(src))
	d := &Doc{Page: *p, Edit: RepoURL + "/edit/main/" + p.Source}
	var plain strings.Builder
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n := n.(type) {
		case *ast.Heading:
			if n.Level == 2 || n.Level == 3 {
				id, _ := n.AttributeString("id")
				ids, _ := id.([]byte)
				d.Headings = append(d.Headings, Heading{ID: string(ids), Text: nodeText(n, src), Level: n.Level})
			}
		case *ast.Text:
			plain.Write(n.Segment.Value(src))
			plain.WriteByte(' ')
		}
		return ast.WalkContinue, nil
	})
	var buf bytes.Buffer
	if err := md.Renderer().Render(&buf, src, doc); err != nil {
		return nil, fmt.Errorf("docs: rendering %s: %w", p.Source, err)
	}
	d.HTML = anchorHeadings(buf.String())
	d.text = plain.String()
	d.Words = len(strings.Fields(d.text))
	return d, nil
}

func nodeText(n ast.Node, src []byte) string {
	var b strings.Builder
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			if t, ok := c.(*ast.Text); ok {
				b.Write(t.Segment.Value(src))
			}
			if t, ok := c.(*ast.String); ok {
				b.Write(t.Value)
			}
		}
		return ast.WalkContinue, nil
	})
	return b.String()
}

var headingRe = regexp.MustCompile(`<h([23]) id="([^"]+)">`)

// anchorHeadings adds a link to each h2 and h3, for sharing a section.
func anchorHeadings(s string) string {
	return headingRe.ReplaceAllString(s, `<h$1 id="$2"><a class="anchor" href="#$2" aria-label="Link to this section">#</a>`)
}

// trimReadme drops the README's HTML title block, which the page replaces.
func trimReadme(src []byte) []byte {
	if i := bytes.Index(src, []byte("\n---\n")); i >= 0 && bytes.HasPrefix(bytes.TrimSpace(src), []byte("<")) {
		return bytes.TrimLeft(src[i+5:], "\n")
	}
	return src
}

// linker points links between Markdown files at their pages, and other
// repository files at GitHub.
type linker struct{ from string }

func (l *linker) Transform(doc *ast.Document, _ text.Reader, _ parser.Context) {
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch x := n.(type) {
		case *ast.Link:
			x.Destination = []byte(rewrite(l.from, string(x.Destination)))
		case *ast.Image:
			x.Destination = []byte(rewrite(l.from, string(x.Destination)))
		}
		return ast.WalkContinue, nil
	})
}

func rewrite(from, dest string) string {
	if dest == "" || strings.HasPrefix(dest, "#") || strings.Contains(dest, "://") || strings.HasPrefix(dest, "mailto:") {
		return dest
	}
	target, anchor, _ := strings.Cut(dest, "#")
	resolved := path.Clean(path.Join(path.Dir(from), target))
	if p, ok := bySrc[resolved]; ok {
		if anchor != "" {
			return p.URL() + "#" + anchor
		}
		return p.URL()
	}
	kind := "blob"
	if !strings.Contains(path.Base(resolved), ".") {
		kind = "tree"
	}
	if strings.HasPrefix(resolved, "docs/assets/") {
		kind = "raw"
	}
	u := RepoURL + "/" + kind + "/main/" + resolved
	if anchor != "" {
		u += "#" + anchor
	}
	return u
}

// SearchEntry is a page or one of its sections, for the site's search.
type SearchEntry struct {
	Title   string `json:"title"`
	Section string `json:"section"`
	URL     string `json:"url"`
	Text    string `json:"text"`
}

// Index lists every page and its sections, with a little of each page's
// text.
func Index() ([]SearchEntry, error) {
	ps, err := list()
	if err != nil {
		return nil, err
	}
	var out []SearchEntry
	for _, p := range ps {
		d, err := Get(p.Slug)
		if err != nil {
			return nil, err
		}
		snippet := d.text
		if len(snippet) > 400 {
			snippet = snippet[:400]
			if i := strings.LastIndexByte(snippet, ' '); i > 0 {
				snippet = snippet[:i]
			}
		}
		out = append(out, SearchEntry{Title: shortTitle(p), Section: p.Section, URL: p.URL(), Text: strings.Join(strings.Fields(snippet), " ")})
		for _, h := range d.Headings {
			out = append(out, SearchEntry{Title: h.Text, Section: shortTitle(p), URL: p.URL() + "#" + h.ID})
		}
	}
	return out, nil
}

//go:generate go run ../codecss

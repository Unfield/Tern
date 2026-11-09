// Command website builds Tern's documentation site from the Markdown in
// the repository: go run . -out dist -base /Tern/
package main

import (
	"bytes"
	"flag"
	"fmt"
	"html/template"
	"log"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/alecthomas/chroma/v2/formatters/html"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
	gmhtml "github.com/yuin/goldmark/renderer/html"
)

const repoURL = "https://github.com/Unfield/Tern"

// page is one page of the site, built from a Markdown file in the repo.
type page struct {
	Source  string // repo-relative path
	Slug    string // URL path below the base, "" for the home page
	Title   string
	Section string
	HTML    template.HTML
}

func pages(repo string) []*page {
	ps := []*page{
		{Source: "README.md", Slug: "", Title: "Introduction", Section: "Start"},
		{Source: "docs/guide.md", Slug: "guide", Title: "Guide", Section: "Start"},
		{Source: "docs/deployment.md", Slug: "deployment", Title: "Deployment", Section: "Start"},
		{Source: "docs/benchmarks.md", Slug: "benchmarks", Title: "Benchmarks", Section: "Reference"},
		{Source: "docs/ARCHITECTURE.md", Slug: "architecture", Title: "Architecture", Section: "Reference"},
		{Source: "docs/ROADMAP.md", Slug: "roadmap", Title: "Roadmap", Section: "Project"},
		{Source: "CHANGELOG.md", Slug: "changelog", Title: "Changelog", Section: "Project"},
		{Source: "CONTRIBUTING.md", Slug: "contributing", Title: "Contributing", Section: "Project"},
		{Source: "docs/adr/README.md", Slug: "decisions", Title: "Decision records", Section: "Decisions"},
	}
	adrs, _ := filepath.Glob(filepath.Join(repo, "docs", "adr", "0*.md"))
	sort.Strings(adrs)
	for _, f := range adrs {
		name := strings.TrimSuffix(filepath.Base(f), ".md")
		ps = append(ps, &page{Source: "docs/adr/" + filepath.Base(f), Slug: "decisions/" + name, Section: "Decisions"})
	}
	return ps
}

var titleRe = regexp.MustCompile(`(?m)^#\s+(.+)$`)

func main() {
	out := flag.String("out", "dist", "output directory")
	base := flag.String("base", "/", "URL path the site is served under, such as /Tern/")
	repo := flag.String("repo", "..", "repository root")
	flag.Parse()
	if !strings.HasSuffix(*base, "/") {
		*base += "/"
	}

	ps := pages(*repo)
	bySource := map[string]*page{}
	for _, p := range ps {
		bySource[p.Source] = p
	}
	url := func(p *page) string {
		if p.Slug == "" {
			return *base
		}
		return *base + p.Slug + "/"
	}

	// Read every title first: each page's sidebar lists all pages.
	for _, p := range ps {
		if p.Title != "" {
			continue
		}
		src, err := os.ReadFile(filepath.Join(*repo, filepath.FromSlash(p.Source)))
		if err != nil {
			log.Fatal(err)
		}
		if m := titleRe.FindSubmatch(src); m != nil {
			p.Title = string(m[1])
		}
	}

	tmpl := template.Must(template.New("page").Parse(pageTemplate))
	if err := os.RemoveAll(*out); err != nil {
		log.Fatal(err)
	}
	for _, p := range ps {
		src, err := os.ReadFile(filepath.Join(*repo, filepath.FromSlash(p.Source)))
		if err != nil {
			log.Fatal(err)
		}
		if p.Source == "README.md" {
			src = trimReadmeHeader(src)
		}
		md := goldmark.New(
			goldmark.WithExtensions(extension.GFM, highlighting.NewHighlighting(
				highlighting.WithStyle("github"),
				highlighting.WithFormatOptions(html.WithClasses(true)),
			)),
			goldmark.WithParserOptions(
				parser.WithAutoHeadingID(),
				parser.WithASTTransformers(util.Prioritized(&linkRewriter{from: p.Source, bySource: bySource, url: url}, 100)),
			),
			goldmark.WithRendererOptions(gmhtml.WithUnsafe()),
		)
		var buf bytes.Buffer
		if err := md.Convert(src, &buf); err != nil {
			log.Fatal(err)
		}
		p.HTML = template.HTML(buf.String())

		var sections []navSection
		for _, q := range ps {
			if len(sections) == 0 || sections[len(sections)-1].Name != q.Section {
				sections = append(sections, navSection{Name: q.Section})
			}
			s := &sections[len(sections)-1]
			s.Links = append(s.Links, navLink{Title: shortTitle(q), URL: url(q), Active: q == p})
		}

		var html bytes.Buffer
		err = tmpl.Execute(&html, map[string]any{
			"Page": p, "Base": *base, "Nav": sections, "Repo": repoURL,
			"Edit": repoURL + "/edit/main/" + p.Source,
		})
		if err != nil {
			log.Fatal(err)
		}
		dest := filepath.Join(*out, filepath.FromSlash(p.Slug), "index.html")
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			log.Fatal(err)
		}
		if err := os.WriteFile(dest, html.Bytes(), 0o644); err != nil {
			log.Fatal(err)
		}
	}

	var css bytes.Buffer
	css.WriteString(siteCSS)
	if err := html.New(html.WithClasses(true)).WriteCSS(&css, chromaStyle("github")); err != nil {
		log.Fatal(err)
	}
	css.WriteString("@media (prefers-color-scheme: dark) {\n")
	if err := html.New(html.WithClasses(true)).WriteCSS(&css, chromaStyle("github-dark")); err != nil {
		log.Fatal(err)
	}
	css.WriteString("}\n")
	for name, data := range map[string][]byte{"site.css": css.Bytes(), "logo.svg": mustRead(filepath.Join(*repo, "docs", "assets", "logo.svg")), ".nojekyll": nil} {
		if err := os.WriteFile(filepath.Join(*out, name), data, 0o644); err != nil {
			log.Fatal(err)
		}
	}
	fmt.Printf("Built %d pages into %s\n", len(ps), *out)
}

func mustRead(p string) []byte {
	b, err := os.ReadFile(p)
	if err != nil {
		log.Fatal(err)
	}
	return b
}

func shortTitle(p *page) string {
	if strings.HasPrefix(p.Slug, "decisions/") {
		num, rest, _ := strings.Cut(p.Title, ". ")
		if rest != "" {
			return num + " · " + rest
		}
	}
	return p.Title
}

// trimReadmeHeader drops the README's HTML title block, which the site
// header replaces.
func trimReadmeHeader(src []byte) []byte {
	if i := bytes.Index(src, []byte("\n---\n")); i >= 0 && bytes.HasPrefix(bytes.TrimSpace(src), []byte("<")) {
		return bytes.TrimLeft(src[i+5:], "\n")
	}
	return src
}

type navSection struct {
	Name  string
	Links []navLink
}

type navLink struct {
	Title  string
	URL    string
	Active bool
}

// linkRewriter points links between Markdown files at their pages on the
// site, and other repository files at GitHub.
type linkRewriter struct {
	from     string
	bySource map[string]*page
	url      func(*page) string
}

func (r *linkRewriter) Transform(doc *ast.Document, _ text.Reader, _ parser.Context) {
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch l := n.(type) {
		case *ast.Link:
			l.Destination = []byte(r.rewrite(string(l.Destination)))
		case *ast.Image:
			l.Destination = []byte(r.rewrite(string(l.Destination)))
		}
		return ast.WalkContinue, nil
	})
}

func (r *linkRewriter) rewrite(dest string) string {
	if dest == "" || strings.HasPrefix(dest, "#") || strings.Contains(dest, "://") || strings.HasPrefix(dest, "mailto:") {
		return dest
	}
	target, anchor, _ := strings.Cut(dest, "#")
	resolved := path.Clean(path.Join(path.Dir(r.from), target))
	if p, ok := r.bySource[resolved]; ok {
		if anchor != "" {
			return r.url(p) + "#" + anchor
		}
		return r.url(p)
	}
	kind := "blob"
	if !strings.Contains(path.Base(resolved), ".") {
		kind = "tree"
	}
	u := repoURL + "/" + kind + "/main/" + resolved
	if anchor != "" {
		u += "#" + anchor
	}
	return u
}

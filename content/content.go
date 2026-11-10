// Package content loads Markdown collections from an app's src/content
// directory, with typed YAML front matter:
//
//	// src/content/blog/hello-world.md
//	---
//	title: Hello, world
//	date: 2026-11-10
//	---
//	The first post.
//
//	type Post struct {
//		Title string    `yaml:"title" json:"title"`
//		Date  time.Time `yaml:"date" json:"date"`
//	}
//
//	var posts = &content.Collection[Post]{Dir: "blog", Sort: content.Newest(func(p Post) time.Time { return p.Date })}
//
//	func Load(ctx *kit.Ctx) (PageData, error) {
//		post, err := posts.Get(ctx, ctx.Param("slug")) // a 404 if there is none
//		...
//	}
//
// Entries render to HTML with GitHub-flavored Markdown, heading IDs,
// footnotes and highlighted code; show it with {@html entry.html}. The
// collection is read once in production and on every request under tern dev.
// Entries with `draft: true` in their front matter are left out in
// production.
package content

import (
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"
	"sync"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/Unfield/Tern/internal/markdown"
	"github.com/Unfield/Tern/kit"
)

// Heading is a section heading of an entry, for tables of contents.
type Heading = markdown.Heading

// Entry is one Markdown file of a collection.
type Entry[T any] struct {
	// Slug is the file's path in the collection without ".md", such as
	// "hello-world" or "guides/install"; "guides/index.md" is "guides".
	Slug string `json:"slug"`
	// Data is the decoded front matter.
	Data T `json:"data"`
	// HTML is the rendered body.
	HTML string `json:"html"`
	// Headings lists the h2 to h4 headings.
	Headings []Heading `json:"headings"`
	// Excerpt is the first paragraph as plain text.
	Excerpt string `json:"excerpt"`
	// ReadingMinutes estimates the reading time.
	ReadingMinutes int `json:"readingMinutes"`
}

// Summary is an entry without its body, for lists of entries.
type Summary[T any] struct {
	Slug           string `json:"slug"`
	Data           T      `json:"data"`
	Excerpt        string `json:"excerpt"`
	ReadingMinutes int    `json:"readingMinutes"`
}

// Summary returns e without its HTML and headings, which keeps list pages
// small.
func (e Entry[T]) Summary() Summary[T] {
	return Summary[T]{Slug: e.Slug, Data: e.Data, Excerpt: e.Excerpt, ReadingMinutes: e.ReadingMinutes}
}

// Collection is a directory of Markdown files below src/content. Declare
// it once, as a package variable, so production loads it once.
type Collection[T any] struct {
	// Dir is the collection's directory relative to src/content.
	Dir string
	// Sort orders All; the default is by slug.
	Sort func(a, b Entry[T]) int
	// Filter, if set, leaves out entries for which it returns false.
	Filter func(Entry[T]) bool

	once    sync.Once
	entries []Entry[T]
	bySlug  map[string]int
	err     error
}

// All returns every entry, sorted.
func (c *Collection[T]) All(ctx *kit.Ctx) ([]Entry[T], error) {
	entries, _, err := c.load(ctx)
	return entries, err
}

// Summaries returns every entry without its body, sorted.
func (c *Collection[T]) Summaries(ctx *kit.Ctx) ([]Summary[T], error) {
	entries, _, err := c.load(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Summary[T], len(entries))
	for i, e := range entries {
		out[i] = e.Summary()
	}
	return out, nil
}

// Get returns the entry with the given slug, or a 404 error.
func (c *Collection[T]) Get(ctx *kit.Ctx, slug string) (Entry[T], error) {
	entries, bySlug, err := c.load(ctx)
	if err != nil {
		return Entry[T]{}, err
	}
	i, ok := bySlug[strings.Trim(slug, "/")]
	if !ok {
		return Entry[T]{}, kit.NotFound()
	}
	return entries[i], nil
}

// Slugs returns every slug, for a prerendered route's Entries function.
func (c *Collection[T]) Slugs(ctx *kit.Ctx) ([]string, error) {
	entries, _, err := c.load(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Slug
	}
	return out, nil
}

func (c *Collection[T]) load(ctx *kit.Ctx) ([]Entry[T], map[string]int, error) {
	if ctx.Dev() {
		return c.read(ctx.Content(), true)
	}
	c.once.Do(func() { c.entries, c.bySlug, c.err = c.read(ctx.Content(), false) })
	return c.entries, c.bySlug, c.err
}

func (c *Collection[T]) read(fsys fs.FS, drafts bool) ([]Entry[T], map[string]int, error) {
	dir := path.Clean(strings.Trim(c.Dir, "/"))
	if dir == "" {
		dir = "."
	}
	var entries []Entry[T]
	err := fs.WalkDir(fsys, dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == dir && errors.Is(err, fs.ErrNotExist) {
				return fmt.Errorf("content: no directory src/content/%s", dir)
			}
			return err
		}
		if d.IsDir() || path.Ext(p) != ".md" || strings.HasPrefix(d.Name(), "_") {
			return nil
		}
		src, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		e, draft, err := parse[T](src)
		if err != nil {
			return fmt.Errorf("content: src/content/%s: %w", p, err)
		}
		if draft && !drafts {
			return nil
		}
		rel := strings.TrimPrefix(strings.TrimSuffix(p, ".md"), dir+"/")
		if dir == "." {
			rel = strings.TrimSuffix(p, ".md")
		}
		if rel == "index" {
			rel = ""
		}
		e.Slug = strings.TrimSuffix(rel, "/index")
		if c.Filter == nil || c.Filter(e) {
			entries = append(entries, e)
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	sortFn := c.Sort
	if sortFn == nil {
		sortFn = func(a, b Entry[T]) int { return cmp.Compare(a.Slug, b.Slug) }
	}
	slices.SortStableFunc(entries, sortFn)
	bySlug := make(map[string]int, len(entries))
	for i, e := range entries {
		bySlug[e.Slug] = i
	}
	return entries, bySlug, nil
}

func parse[T any](src []byte) (Entry[T], bool, error) {
	var e Entry[T]
	front, _ := markdown.Split(src)
	var meta struct {
		Draft bool `yaml:"draft"`
	}
	if len(front) > 0 {
		if err := yaml.Unmarshal(front, &meta); err != nil {
			return e, false, fmt.Errorf("front matter: %w", err)
		}
	}
	doc, err := markdown.Parse(src, &e.Data)
	if err != nil {
		return e, false, err
	}
	e.HTML = doc.HTML
	e.Headings = doc.Headings
	e.Excerpt = doc.Excerpt
	e.ReadingMinutes = doc.ReadingMinutes
	return e, meta.Draft, nil
}

// Render renders Markdown text, such as user-written content from a
// database, with the same options as collections. Unlike collections, it
// escapes raw HTML, since such text is not trusted.
func Render(text string) (string, error) {
	doc, err := markdown.RenderSafe([]byte(text))
	return doc.HTML, err
}

// Newest sorts entries by a date in their data, newest first, then by slug.
func Newest[T any](date func(T) time.Time) func(a, b Entry[T]) int {
	return func(a, b Entry[T]) int {
		if c := date(b.Data).Compare(date(a.Data)); c != 0 {
			return c
		}
		return cmp.Compare(a.Slug, b.Slug)
	}
}

// CodeCSS returns the CSS for highlighted code in a chroma style, such as
// "github", "github-dark" or "dracula". Serve it, or paste it into app.css.
func CodeCSS(style string) (string, error) { return markdown.CodeCSS(style) }

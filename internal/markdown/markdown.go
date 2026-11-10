// Package markdown renders Markdown with YAML front matter. It is shared by
// the content package, which renders collections at runtime, and by the code
// generator, which renders +page.md routes, so both produce the same HTML.
package markdown

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"go.yaml.in/yaml/v3"
)

// Heading is a section heading, for tables of contents.
type Heading struct {
	Level int    `json:"level"`
	ID    string `json:"id"`
	Text  string `json:"text"`
}

// Document is rendered Markdown.
type Document struct {
	// HTML is the rendered body.
	HTML string `json:"html"`
	// Headings lists the h2 to h4 headings in order.
	Headings []Heading `json:"headings"`
	// Words counts the words of the text.
	Words int `json:"words"`
	// ReadingMinutes estimates the reading time at 200 words a minute, at
	// least 1.
	ReadingMinutes int `json:"readingMinutes"`
	// Excerpt is the first paragraph as plain text.
	Excerpt string `json:"excerpt"`
}

func newMarkdown(unsafe bool) goldmark.Markdown {
	rendererOpts := []renderer.Option{}
	if unsafe {
		rendererOpts = append(rendererOpts, html.WithUnsafe())
	}
	return goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			extension.Footnote,
			extension.Typographer,
			highlighting.NewHighlighting(
				highlighting.WithFormatOptions(chromahtml.WithClasses(true)),
			),
		),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
		goldmark.WithRendererOptions(rendererOpts...),
	)
}

var (
	// md renders the app's own content, which may contain raw HTML, as a
	// .svelte file may.
	md = newMarkdown(true)
	// mdSafe escapes raw HTML, for text from users.
	mdSafe = newMarkdown(false)
)

// Split separates YAML front matter, between "---" lines at the very start,
// from the Markdown body.
func Split(src []byte) (front, body []byte) {
	src = bytes.TrimPrefix(src, []byte("\ufeff"))
	normalized := bytes.ReplaceAll(src, []byte("\r\n"), []byte("\n"))
	if !bytes.HasPrefix(normalized, []byte("---\n")) {
		return nil, src
	}
	rest := normalized[4:]
	if bytes.HasPrefix(rest, []byte("---\n")) || bytes.Equal(rest, []byte("---")) {
		return nil, bytes.TrimPrefix(rest[3:], []byte("\n"))
	}
	end := bytes.Index(rest, []byte("\n---\n"))
	if end < 0 {
		if bytes.HasSuffix(rest, []byte("\n---")) {
			return rest[:len(rest)-3], nil
		}
		return nil, src
	}
	return rest[:end+1], rest[end+5:]
}

// Parse decodes the front matter into v (a pointer to a struct or map) and
// renders the body. v may be nil.
func Parse(src []byte, v any) (Document, error) {
	front, body := Split(src)
	if v != nil && len(bytes.TrimSpace(front)) > 0 {
		if err := yaml.Unmarshal(front, v); err != nil {
			return Document{}, fmt.Errorf("front matter: %w", err)
		}
	}
	return Render(body)
}

// Render renders Markdown without front matter.
func Render(body []byte) (Document, error) { return render(md, body) }

// RenderSafe renders Markdown from untrusted sources: raw HTML is replaced
// with a comment, and links with javascript: URLs are dropped.
func RenderSafe(body []byte) (Document, error) { return render(mdSafe, body) }

func render(md goldmark.Markdown, body []byte) (Document, error) {
	ctx := parser.NewContext()
	doc := md.Parser().Parse(text.NewReader(body), parser.WithContext(ctx))
	var out Document
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n := n.(type) {
		case *ast.Heading:
			if n.Level >= 2 && n.Level <= 4 {
				id, _ := n.AttributeString("id")
				idStr, _ := id.([]byte)
				out.Headings = append(out.Headings, Heading{Level: n.Level, ID: string(idStr), Text: plain(n, body)})
			}
		case *ast.Paragraph:
			if out.Excerpt == "" && n.Parent() == doc {
				out.Excerpt = plain(n, body)
			}
		}
		return ast.WalkContinue, nil
	})
	var buf bytes.Buffer
	if err := md.Renderer().Render(&buf, body, doc); err != nil {
		return Document{}, err
	}
	out.HTML = buf.String()
	out.Words = countWords(textOf(doc, body))
	out.ReadingMinutes = max(1, (out.Words+199)/200)
	return out, nil
}

func plain(n ast.Node, src []byte) string {
	return strings.Join(strings.Fields(textOf(n, src)), " ")
}

func textOf(n ast.Node, src []byte) string {
	var b strings.Builder
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch c := c.(type) {
		case *ast.Text:
			b.Write(c.Segment.Value(src))
			if c.SoftLineBreak() || c.HardLineBreak() {
				b.WriteByte(' ')
			}
		case *ast.String:
			b.Write(c.Value)
		case *ast.CodeSpan:
			for ch := c.FirstChild(); ch != nil; ch = ch.NextSibling() {
				if t, ok := ch.(*ast.Text); ok {
					b.Write(t.Segment.Value(src))
				}
			}
			return ast.WalkSkipChildren, nil
		case *ast.FencedCodeBlock, *ast.CodeBlock:
			lines := c.Lines()
			for i := 0; i < lines.Len(); i++ {
				seg := lines.At(i)
				b.Write(seg.Value(src))
			}
			return ast.WalkSkipChildren, nil
		}
		if c.Type() == ast.TypeBlock && b.Len() > 0 {
			b.WriteByte(' ')
		}
		return ast.WalkContinue, nil
	})
	return b.String()
}

func countWords(s string) int {
	return len(strings.FieldsFunc(s, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsPunct(r) && r != '\'' && r != '-' }))
}

var validStyle = regexp.MustCompile(`^[a-z0-9_-]+$`)

// CodeCSS returns the CSS for highlighted code blocks in a chroma style,
// such as "github" or "dracula" (https://xyproto.github.io/splash/docs/).
func CodeCSS(style string) (string, error) {
	if !validStyle.MatchString(style) {
		return "", fmt.Errorf("markdown: invalid style %q", style)
	}
	s := styles.Get(style)
	if s == styles.Fallback && style != "swapoff" {
		return "", fmt.Errorf("markdown: unknown style %q", style)
	}
	var buf bytes.Buffer
	if err := chromahtml.New(chromahtml.WithClasses(true)).WriteCSS(&buf, s); err != nil {
		return "", err
	}
	return buf.String(), nil
}

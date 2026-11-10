package markdown

import (
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	src := "---\r\ntitle: Hello\r\ntags: [go, svelte]\r\ndraft: true\r\n---\r\n# Hello\n\nFirst *paragraph* with `code`.\n\n## Getting started\n\n| a | b |\n|---|---|\n| 1 | 2 |\n\n- [x] done\n\n```go\nfunc main() {}\n```\n\nA footnote[^1]. \"Quotes\" -- dash.\n\n[^1]: Note.\n\n<div class=\"raw\">raw html</div>\n"
	var front struct {
		Title string   `yaml:"title"`
		Tags  []string `yaml:"tags"`
		Draft bool     `yaml:"draft"`
	}
	doc, err := Parse([]byte(src), &front)
	if err != nil {
		t.Fatal(err)
	}
	if front.Title != "Hello" || len(front.Tags) != 2 || !front.Draft {
		t.Errorf("front matter = %+v", front)
	}
	for _, want := range []string{
		`<h1 id="hello">Hello</h1>`,
		`<h2 id="getting-started">Getting started</h2>`,
		"<table>",
		`type="checkbox"`,
		`<span class="kd">func</span>`,
		`class="footnotes"`,
		"&ldquo;Quotes&rdquo; &ndash; dash",
		`<div class="raw">raw html</div>`,
	} {
		if !strings.Contains(doc.HTML, want) {
			t.Errorf("missing %q in\n%s", want, doc.HTML)
		}
	}
	if len(doc.Headings) != 1 || doc.Headings[0] != (Heading{Level: 2, ID: "getting-started", Text: "Getting started"}) {
		t.Errorf("headings = %+v", doc.Headings)
	}
	if doc.Excerpt != "First paragraph with code." {
		t.Errorf("excerpt = %q", doc.Excerpt)
	}
	if doc.Words < 15 || doc.ReadingMinutes != 1 {
		t.Errorf("words %d, minutes %d", doc.Words, doc.ReadingMinutes)
	}
}

func TestSplit(t *testing.T) {
	for _, tt := range []struct{ in, front, body string }{
		{"no front matter", "", "no front matter"},
		{"---\na: 1\n---\nbody", "a: 1\n", "body"},
		{"---\n---\nbody", "", "body"},
		{"---\na: 1\n---", "a: 1\n", ""},
		{"---\nunclosed", "", "---\nunclosed"},
		{"\ufeff---\na: 1\n---\nx", "a: 1\n", "x"},
	} {
		front, body := Split([]byte(tt.in))
		if string(front) != tt.front || string(body) != tt.body {
			t.Errorf("Split(%q) = %q, %q", tt.in, front, body)
		}
	}
}

func TestBadFrontMatter(t *testing.T) {
	var v map[string]any
	if _, err := Parse([]byte("---\n: : :\n---\nx"), &v); err == nil {
		t.Error("want a front matter error")
	}
}

func TestCodeCSS(t *testing.T) {
	css, err := CodeCSS("github")
	if err != nil || !strings.Contains(css, ".chroma") {
		t.Fatalf("CodeCSS: %v %.100s", err, css)
	}
	if _, err := CodeCSS("nope-style"); err == nil {
		t.Error("want an error for an unknown style")
	}
}

func TestRenderSafe(t *testing.T) {
	doc, err := RenderSafe([]byte("<script>alert(1)</script>\n\n[x](javascript:alert(1)) <img src=x onerror=alert(1)>"))
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"<script", "javascript:", "<img"} {
		if strings.Contains(doc.HTML, bad) {
			t.Errorf("unsafe %q in %s", bad, doc.HTML)
		}
	}
}

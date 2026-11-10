// Package htmltext turns HTML into readable plain text, for the text part
// of emails.
package htmltext

import (
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

var blocks = map[string]bool{
	"p": true, "div": true, "h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"ul": true, "ol": true, "table": true, "tr": true, "blockquote": true, "pre": true, "section": true,
	"article": true, "header": true, "footer": true, "hr": true,
}

var spaces = regexp.MustCompile(`[ \t\r\n]+`)
var blankLines = regexp.MustCompile(`\n{3,}`)

// Convert returns the text of an HTML document: block elements are
// separated by blank lines, list items start with "- ", links show their
// URL in parentheses, and <head>, <style> and <script> are left out.
func Convert(doc string) string {
	z := html.NewTokenizer(strings.NewReader(doc))
	var b strings.Builder
	skip := 0
	var href []string
	pre := 0
	write := func(s string) {
		if skip == 0 {
			b.WriteString(s)
		}
	}
	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			out := blankLines.ReplaceAllString(b.String(), "\n\n")
			lines := strings.Split(out, "\n")
			for i, l := range lines {
				lines[i] = strings.TrimRight(l, " ")
			}
			return strings.TrimSpace(strings.Join(lines, "\n"))
		case html.TextToken:
			t := string(z.Text())
			if pre == 0 {
				t = spaces.ReplaceAllString(t, " ")
				if strings.HasSuffix(b.String(), "\n") || b.Len() == 0 {
					t = strings.TrimLeft(t, " ")
				}
			}
			write(t)
		case html.StartTagToken, html.SelfClosingTagToken, html.EndTagToken:
			name, hasAttr := z.TagName()
			tag := string(name)
			end := tt == html.EndTagToken
			switch tag {
			case "head", "style", "script", "title":
				if end {
					skip--
				} else if tt == html.StartTagToken {
					skip++
				}
				continue
			case "br":
				write("\n")
			case "li":
				if !end {
					write("\n- ")
				}
			case "pre":
				if end {
					pre--
				} else {
					pre++
				}
				write("\n\n")
			case "a":
				if !end {
					link := ""
					for hasAttr {
						var k, v []byte
						k, v, hasAttr = z.TagAttr()
						if string(k) == "href" {
							link = string(v)
						}
					}
					href = append(href, link)
				} else if n := len(href); n > 0 {
					link := href[n-1]
					href = href[:n-1]
					text := b.String()
					if link != "" && !strings.HasPrefix(link, "#") && !strings.HasPrefix(link, "mailto:") && !strings.HasSuffix(text, link) {
						write(" (" + link + ")")
					}
				}
			case "td", "th":
				if end {
					write("  ")
				}
			case "img":
				for hasAttr {
					var k, v []byte
					k, v, hasAttr = z.TagAttr()
					if string(k) == "alt" && len(v) > 0 {
						write(string(v))
					}
				}
			default:
				if blocks[tag] {
					write("\n\n")
				}
				if tag == "hr" {
					write("---\n\n")
				}
			}
		}
	}
}

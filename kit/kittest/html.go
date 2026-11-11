package kittest

import (
	"strings"

	"golang.org/x/net/html"
)

// Element is an HTML element of a Page.
type Element struct{ n *html.Node }

// Tag is the element's name: "a", "h1".
func (e *Element) Tag() string { return e.n.Data }

// Attr is the value of an attribute, "" if the element hasn't it.
func (e *Element) Attr(name string) string {
	v, _ := attr(e.n, name)
	return v
}

// Text is the element's text, with spaces collapsed.
func (e *Element) Text() string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		switch n.Type {
		case html.TextNode:
			b.WriteString(n.Data)
		case html.ElementNode:
			if n.Data == "script" || n.Data == "style" {
				return
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(e.n)
	return strings.Join(strings.Fields(b.String()), " ")
}

// Find returns the elements inside e matching selector.
func (e *Element) Find(selector string) []*Element { return find(e.n, selector) }

func attr(n *html.Node, name string) (string, bool) {
	for _, a := range n.Attr {
		if a.Key == name {
			return a.Val, true
		}
	}
	return "", false
}

// compound is one selector step: a tag, an id, classes and attributes.
type compound struct {
	tag     string
	id      string
	classes []string
	attrs   [][2]string // name, value; value "\x00" for presence
	child   bool        // joined to the previous step with >
}

func parseSelector(s string) [][]compound {
	var out [][]compound
	for _, part := range strings.Split(s, ",") {
		var steps []compound
		child := false
		for _, tok := range strings.Fields(strings.ReplaceAll(part, ">", " > ")) {
			if tok == ">" {
				child = true
				continue
			}
			c := parseCompound(tok)
			c.child = child
			child = false
			steps = append(steps, c)
		}
		if len(steps) > 0 {
			out = append(out, steps)
		}
	}
	return out
}

func parseCompound(tok string) compound {
	var c compound
	i := 0
	read := func() string {
		j := i
		for j < len(tok) && !strings.ContainsRune("#.[", rune(tok[j])) {
			j++
		}
		v := tok[i:j]
		i = j
		return v
	}
	c.tag = strings.ToLower(read())
	for i < len(tok) {
		switch tok[i] {
		case '#':
			i++
			c.id = read()
		case '.':
			i++
			c.classes = append(c.classes, read())
		case '[':
			end := strings.IndexByte(tok[i:], ']')
			if end < 0 {
				end = len(tok) - i
			}
			body := tok[i+1 : i+end]
			i += end + 1
			name, val, ok := strings.Cut(body, "=")
			if !ok {
				val = "\x00"
			}
			c.attrs = append(c.attrs, [2]string{name, strings.Trim(val, `"'`)})
		default:
			i++
		}
	}
	return c
}

func (c compound) matches(n *html.Node) bool {
	if n.Type != html.ElementNode {
		return false
	}
	if c.tag != "" && c.tag != "*" && n.Data != c.tag {
		return false
	}
	if c.id != "" {
		if v, _ := attr(n, "id"); v != c.id {
			return false
		}
	}
	if len(c.classes) > 0 {
		have := strings.Fields(func() string { v, _ := attr(n, "class"); return v }())
		for _, want := range c.classes {
			found := false
			for _, h := range have {
				found = found || h == want
			}
			if !found {
				return false
			}
		}
	}
	for _, a := range c.attrs {
		v, ok := attr(n, a[0])
		if !ok || a[1] != "\x00" && v != a[1] {
			return false
		}
	}
	return true
}

// matchSteps reports whether n matches the last step, with its ancestors
// matching the ones before.
func matchSteps(n *html.Node, steps []compound) bool {
	last := steps[len(steps)-1]
	if !last.matches(n) {
		return false
	}
	if len(steps) == 1 {
		return true
	}
	rest := steps[:len(steps)-1]
	for p := n.Parent; p != nil; p = p.Parent {
		if matchSteps(p, rest) {
			return true
		}
		if last.child {
			return false // > : only the parent may match
		}
	}
	return false
}

func find(root *html.Node, selector string) []*Element {
	groups := parseSelector(selector)
	var out []*Element
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n != root {
			for _, steps := range groups {
				if matchSteps(n, steps) {
					out = append(out, &Element{n})
					break
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	return out
}

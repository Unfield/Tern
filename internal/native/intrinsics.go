package native

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

// An intrinsic is a Go function compiled code uses in place of a module's
// function: one of Svelte's hot server helpers, written in Go. It's used
// only when the function's syntax tree has the fingerprint it was written
// against, so a Svelte release that changes the function gets it compiled
// from source instead.
type intrinsic struct {
	module      string // the end of the module's path
	function    string
	fingerprint string
	goExpr      string
}

var intrinsics = []intrinsic{
	{module: "/svelte/src/escaping.js", function: "escape_html", fingerprint: "6dac8776e10d8e8d", goExpr: "svelte.EscapeHTML(r.Realm)"},
}

// A method intrinsic replaces a class's method, named Class.method or
// Class#private. Its Go expression may use the class's private names as
// {#name}, its constructor as {ctor} (a *js.Object variable, set once the
// class is made, passed by address), and the compiled method as {orig},
// for the cases the Go code leaves to it.
var methodIntrinsics = []intrinsic{
	{module: "/svelte/src/internal/server/renderer.js", function: "Renderer.push", fingerprint: "2635664ad54a4aa9", goExpr: "svelte.RendererPush(r.Realm, {#out}, {orig})"},
	{module: "/svelte/src/internal/server/renderer.js", function: "Renderer#collect_content", fingerprint: "0454140b7300c2b8", goExpr: "svelte.RendererCollect(r.Realm, {#out}, {#collect_content}, &{ctor}, {orig})"},
}

// methodIntrinsicFor returns the Go expression replacing a class's method,
// if there is one and the method is the one it was written against. orig
// is the compiled method; priv looks up the class's private names.
func (c *Compiler) methodIntrinsicFor(m *module, class, member string, fn *Node, orig, ctor string, priv func(string) (string, bool)) (string, bool) {
	name := class + "." + member
	if strings.HasPrefix(member, "#") {
		name = class + member
	}

	for _, in := range methodIntrinsics {
		if in.function != name || !strings.HasSuffix(m.id, in.module) {
			continue
		}
		if fp := fingerprint(fn); fp != in.fingerprint {
			c.mismatches = append(c.mismatches, m.id+": "+name+" has fingerprint "+fp)
			return "", false
		}
		// One pass over the template: what goes in isn't looked at again.
		var b strings.Builder
		rest := in.goExpr
		for {
			i := strings.IndexByte(rest, '{')
			if i < 0 {
				b.WriteString(rest)
				break
			}
			j := strings.IndexByte(rest[i:], '}')
			if j < 0 {
				c.mismatches = append(c.mismatches, m.id+": "+name+": bad template "+in.goExpr)
				return "", false
			}
			b.WriteString(rest[:i])
			switch ref := rest[i+1 : i+j]; {
			case ref == "orig":
				b.WriteString(orig)
			case ref == "ctor":
				b.WriteString(ctor)
			case strings.HasPrefix(ref, "#"):
				g, ok := priv(ref)
				if !ok {
					c.mismatches = append(c.mismatches, m.id+": "+name+" has no private name "+ref)
					return "", false
				}
				b.WriteString(g)
			default:
				c.mismatches = append(c.mismatches, m.id+": "+name+": unknown {"+ref+"} in its template")
				return "", false
			}
			rest = rest[i+j+1:]
		}
		expr := b.String()
		return expr, true
	}
	return "", false
}

// intrinsicFor returns the Go expression replacing a module's function, if
// there is one and the function is the one it was written against.
func (c *Compiler) intrinsicFor(m *module, name string, fn *Node) (string, bool) {
	for _, in := range intrinsics {
		if in.function != name || !strings.HasSuffix(m.id, in.module) {
			continue
		}
		if fp := fingerprint(fn); fp != in.fingerprint {
			c.mismatches = append(c.mismatches, m.id+": "+name+" has fingerprint "+fp)
			return "", false
		}
		return in.goExpr, true
	}
	return "", false
}

// fingerprint hashes a syntax tree, leaving out where it is in the file.
func fingerprint(n *Node) string {
	var strip func(v any) any
	strip = func(v any) any {
		switch x := v.(type) {
		case map[string]any:
			for _, k := range []string{"start", "end", "loc", "range"} {
				delete(x, k)
			}
			for k, e := range x {
				x[k] = strip(e)
			}
		case []any:
			for i, e := range x {
				x[i] = strip(e)
			}
		}
		return v
	}
	tree := map[string]any{}
	for k, raw := range n.fields {
		var v any
		_ = json.Unmarshal(raw, &v)
		tree[k] = v
	}
	b, _ := json.Marshal(strip(tree))
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:8])
}

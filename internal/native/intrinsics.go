package native

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

// An intrinsic is a Go function compiled code uses in place of a module's
// function or a class's method: one of Svelte's hot server helpers,
// written in Go. It's used only when the function's syntax tree has the
// fingerprint it was written against, so a Svelte release that changes
// the function gets it compiled from source instead.
//
// goExpr is a template. Every intrinsic may use {orig}, the compiled
// function, for the cases its Go code leaves to it. A module function's
// may use {@name}, a func() js.Value that reads the module binding name
// when called (module constants are set after functions are hoisted). A
// method's may use {#name}, the class's private name, and {ctor}, the
// class's *js.Object variable, set once the class is made.
type intrinsic struct {
	module      string // the end of the module's path
	function    string // a function's name, or Class.method or Class#private
	fingerprint string
	goExpr      string
}

var intrinsics = []intrinsic{
	{module: "/svelte/src/escaping.js", function: "escape_html", fingerprint: "6dac8776e10d8e8d", goExpr: "svelte.EscapeHTML(r.Realm)"},
	{module: "/svelte/src/internal/shared/attributes.js", function: "attr", fingerprint: "7212ec5bdfb35ea8", goExpr: "svelte.Attr(r.Realm, {@replacements}, {@has_own_property}, {@escape_html}, {orig})"},
	{module: "/svelte/src/internal/shared/attributes.js", function: "to_class", fingerprint: "f540d3cf600b505b", goExpr: "svelte.ToClass(r.Realm, {@whitespace}, {orig})"},
	{module: "/svelte/src/internal/server/index.js", function: "attr_class", fingerprint: "3434e1cf8bcc329a", goExpr: "svelte.AttrClass(r.Realm, {@to_class}, {@escape_html}, {orig})"},
	{module: "/svelte/src/internal/server/index.js", function: "stringify", fingerprint: "e37a3d457bd970a7", goExpr: "svelte.Stringify(r.Realm)"},
	{module: "/svelte/src/internal/server/index.js", function: "ensure_array_like", fingerprint: "6837faa2790b73b3", goExpr: "svelte.EnsureArrayLike(r.Realm, {orig})"},
}

var methodIntrinsics = []intrinsic{
	{module: "/svelte/src/internal/server/renderer.js", function: "Renderer.push", fingerprint: "2635664ad54a4aa9", goExpr: "svelte.RendererPush(r.Realm, {#out}, {orig})"},
	{module: "/svelte/src/internal/server/renderer.js", function: "Renderer#collect_content", fingerprint: "0454140b7300c2b8", goExpr: "svelte.RendererCollect(r.Realm, {#out}, {#collect_content}, &{ctor}, {orig})"},
}

// intrinsicFor returns the Go expression replacing a module's function, if
// there is one and the function is the one it was written against. orig
// compiles the function; binding gives a module binding's Go expression.
func (c *Compiler) intrinsicFor(m *module, name string, fn *Node, orig func() string, binding func(string) (string, bool)) (string, bool) {
	in, ok := c.find(intrinsics, m, name, fn)
	if !ok {
		return "", false
	}
	return c.expand(m, name, in.goExpr, func(ref string) (string, bool) {
		switch {
		case ref == "orig":
			return orig(), true
		case strings.HasPrefix(ref, "@"):
			e, ok := binding(ref[1:])
			if !ok {
				return "", false
			}
			return "func() js.Value { return " + e + " }", true
		}
		return "", false
	})
}

// methodIntrinsicFor returns the Go expression replacing a class's method,
// if there is one and the method is the one it was written against. orig
// is the compiled method; priv looks up the class's private names.
func (c *Compiler) methodIntrinsicFor(m *module, class, member string, fn *Node, orig, ctor string, priv func(string) (string, bool)) (string, bool) {
	name := class + "." + member
	if strings.HasPrefix(member, "#") {
		name = class + member
	}
	in, ok := c.find(methodIntrinsics, m, name, fn)
	if !ok {
		return "", false
	}
	return c.expand(m, name, in.goExpr, func(ref string) (string, bool) {
		switch {
		case ref == "orig":
			return orig, true
		case ref == "ctor":
			return ctor, true
		case strings.HasPrefix(ref, "#"):
			return priv(ref)
		}
		return "", false
	})
}

// find looks up the intrinsic for name in module m, checking fn's
// fingerprint; a mismatch is reported.
func (c *Compiler) find(list []intrinsic, m *module, name string, fn *Node) (intrinsic, bool) {
	for _, in := range list {
		if in.function != name || !strings.HasSuffix(m.id, in.module) {
			continue
		}
		if fp := fingerprint(fn); fp != in.fingerprint {
			c.mismatches = append(c.mismatches, m.id+": "+name+" has fingerprint "+fp)
			return intrinsic{}, false
		}
		return in, true
	}
	return intrinsic{}, false
}

// expand fills in an intrinsic's template: each {ref} is what resolve
// gives for it, in one pass, so what goes in isn't read again.
func (c *Compiler) expand(m *module, name, tmpl string, resolve func(string) (string, bool)) (string, bool) {
	var b strings.Builder
	rest := tmpl
	for {
		i := strings.IndexByte(rest, '{')
		if i < 0 {
			b.WriteString(rest)
			return b.String(), true
		}
		j := strings.IndexByte(rest[i:], '}')
		if j < 0 {
			c.mismatches = append(c.mismatches, m.id+": "+name+": bad template "+tmpl)
			return "", false
		}
		b.WriteString(rest[:i])
		ref := rest[i+1 : i+j]
		v, ok := resolve(ref)
		if !ok {
			c.mismatches = append(c.mismatches, m.id+": "+name+": nothing for {"+ref+"} in its template")
			return "", false
		}
		b.WriteString(v)
		rest = rest[i+j+1:]
	}
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

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

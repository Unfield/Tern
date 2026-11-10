// Package native compiles the modules of an app's server build to Go, so
// pages render without the JavaScript engine (ADR 0011). The modules come
// from the Vite plugin in packages/tern/src/vite/native.ts, as ESTree.
package native

import (
	"encoding/json"
	"fmt"
	"os"
)

// IRVersion is the format of .tern/native/ir.json that this package reads.
const IRVersion = 1

// IR is the server build's modules, as the Vite plugin recorded them.
type IR struct {
	Version int    `json:"version"`
	Svelte  string `json:"svelte"`
	Entry   string `json:"entry"`
	// Bundled holds the modules the bundle has; the rest were tree-shaken.
	// Nil means all of them.
	Bundled map[string]bool `json:"bundled"`
	// Env is import.meta.env in the server build.
	Env map[string]any `json:"env"`
	// FileURLs maps import.meta's asset URL placeholders to the URLs
	// the bundle has.
	FileURLs map[string]string `json:"fileUrls"`
	Modules  []struct {
		ID      string             `json:"id"`
		Imports map[string]*string `json:"imports"`
		AST     *Node              `json:"ast"`
		// Code is the module's source, for functions' toString.
		Code string `json:"code"`
	} `json:"modules"`
}

// LoadIR reads the IR file.
func LoadIR(path string) (*IR, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var ir IR
	if err := json.Unmarshal(raw, &ir); err != nil {
		return nil, fmt.Errorf("native: reading %s: %w", path, err)
	}
	if ir.Version != IRVersion {
		return nil, fmt.Errorf("native: %s is version %d; this tern reads version %d", path, ir.Version, IRVersion)
	}
	return &ir, nil
}

// Node is an ESTree node.
type Node struct {
	Type   string
	Start  int
	End    int
	fields map[string]json.RawMessage
	cache  map[string]any
}

// UnmarshalJSON keeps a node's fields raw and decodes them as they are used.
func (n *Node) UnmarshalJSON(b []byte) error {
	if err := json.Unmarshal(b, &n.fields); err != nil {
		return err
	}
	if t, ok := n.fields["type"]; ok {
		_ = json.Unmarshal(t, &n.Type)
	}
	if s, ok := n.fields["start"]; ok {
		_ = json.Unmarshal(s, &n.Start)
	}
	if e, ok := n.fields["end"]; ok {
		_ = json.Unmarshal(e, &n.End)
	}
	return nil
}

// memo decodes a field once per kind of access: kind keeps Child and List
// of the same field from sharing a cached value.
func (n *Node) memo(kind, key string, decode func(json.RawMessage) any) any {
	ck := kind + key
	if v, ok := n.cache[ck]; ok {
		return v
	}
	var v any
	if raw, ok := n.fields[key]; ok && string(raw) != "null" {
		v = decode(raw)
	}
	if n.cache == nil {
		n.cache = map[string]any{}
	}
	n.cache[ck] = v
	return v
}

// Child is a node-valued field, or nil.
func (n *Node) Child(key string) *Node {
	v := n.memo("c", key, func(raw json.RawMessage) any {
		var c Node
		if raw[0] != '{' || json.Unmarshal(raw, &c) != nil {
			return (*Node)(nil)
		}
		return &c
	})
	c, _ := v.(*Node)
	return c
}

// List is a list-of-nodes field; elements may be nil, for array holes.
func (n *Node) List(key string) []*Node {
	v := n.memo("l", key, func(raw json.RawMessage) any {
		var cs []*Node
		_ = json.Unmarshal(raw, &cs)
		return cs
	})
	cs, _ := v.([]*Node)
	return cs
}

// Str is a string field.
func (n *Node) Str(key string) string {
	v := n.memo("s", key, func(raw json.RawMessage) any {
		var s string
		_ = json.Unmarshal(raw, &s)
		return s
	})
	s, _ := v.(string)
	return s
}

// Bool is a boolean field.
func (n *Node) Bool(key string) bool {
	v := n.memo("b", key, func(raw json.RawMessage) any {
		var b bool
		_ = json.Unmarshal(raw, &b)
		return b
	})
	b, _ := v.(bool)
	return b
}

// Raw is a field's JSON, for literal values.
func (n *Node) Raw(key string) json.RawMessage { return n.fields[key] }

// Has reports whether the node has a non-null field.
func (n *Node) Has(key string) bool {
	raw, ok := n.fields[key]
	return ok && string(raw) != "null"
}

// walkNodes calls fn on n and every node below it.
func walkNodes(n *Node, fn func(*Node)) {
	if n == nil {
		return
	}
	fn(n)
	for key, raw := range n.fields {
		if len(raw) == 0 {
			continue
		}
		switch raw[0] {
		case '{':
			walkNodes(n.Child(key), fn)
		case '[':
			for _, ch := range n.List(key) {
				walkNodes(ch, fn)
			}
		}
	}
}

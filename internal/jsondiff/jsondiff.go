// Package jsondiff computes small patches between two JSON values, so live
// components send what changed in a list instead of the whole list.
//
// A patch is a list of operations, each a JSON array:
//
//	["s", path, value]                  set the value at path
//	["d", path]                         delete the object key at path
//	["a", path, start, deleteCount, items] splice the array at path
//
// A path is a list of object keys and array indexes from the root.
package jsondiff

import (
	"bytes"
	"encoding/json"
	"reflect"
)

// Decode parses JSON for Diff, keeping numbers exact.
func Decode(raw []byte) (any, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var v any
	err := d.Decode(&v)
	return v, err
}

// Diff returns the operations that turn a into b. It returns nil when they
// are equal, and a single set of the root when they differ entirely.
func Diff(a, b any) [][]any {
	var ops [][]any
	diff(nil, a, b, &ops)
	return ops
}

func clonePath(p []any, next any) []any {
	out := make([]any, len(p)+1)
	copy(out, p)
	out[len(p)] = next
	return out
}

func diff(path []any, a, b any, ops *[][]any) {
	if reflect.DeepEqual(a, b) {
		return
	}
	switch av := a.(type) {
	case map[string]any:
		if bv, ok := b.(map[string]any); ok {
			for k, x := range bv {
				if y, ok := av[k]; ok {
					diff(clonePath(path, k), y, x, ops)
				} else {
					*ops = append(*ops, []any{"s", clonePath(path, k), x})
				}
			}
			for k := range av {
				if _, ok := bv[k]; !ok {
					*ops = append(*ops, []any{"d", clonePath(path, k)})
				}
			}
			return
		}
	case []any:
		if bv, ok := b.([]any); ok {
			diffArray(path, av, bv, ops)
			return
		}
	}
	*ops = append(*ops, []any{"s", pathOrEmpty(path), b})
}

func pathOrEmpty(p []any) []any {
	if p == nil {
		return []any{}
	}
	return p
}

func diffArray(path []any, a, b []any, ops *[][]any) {
	pre := 0
	for pre < len(a) && pre < len(b) && reflect.DeepEqual(a[pre], b[pre]) {
		pre++
	}
	suf := 0
	for suf < len(a)-pre && suf < len(b)-pre && reflect.DeepEqual(a[len(a)-1-suf], b[len(b)-1-suf]) {
		suf++
	}
	midA, midB := a[pre:len(a)-suf], b[pre:len(b)-suf]
	if len(midA) == len(midB) {
		for i := range midA {
			diff(clonePath(path, pre+i), midA[i], midB[i], ops)
		}
		return
	}
	items := midB
	if items == nil {
		items = []any{}
	}
	*ops = append(*ops, []any{"a", pathOrEmpty(path), pre, len(midA), items})
}

// Package nilslices makes nil slices empty before encoding/json sees
// them. encoding/json writes a nil slice as null; Tern sends [] for pages,
// typed endpoints and live components instead, so their TypeScript types
// are T[] rather than T[] | null. Remove copies only what lies on a path
// to a nil slice: everything else is shared, never changed, so data
// cached across requests is safe. Values that encode themselves are left
// to do so.
package nilslices

import (
	"encoding"
	"encoding/json"
	"reflect"
	"sync"
)

var (
	marshalerType     = reflect.TypeFor[json.Marshaler]()
	textMarshalerType = reflect.TypeFor[encoding.TextMarshaler]()
	mayHaveSlices     sync.Map // reflect.Type → bool
)

// Remove returns v, or a copy of it with nil slices made empty.
func Remove(v any) any {
	if v == nil {
		return nil
	}
	rv := reflect.ValueOf(v)
	if !slicesIn(rv.Type(), map[reflect.Type]bool{}) {
		return v
	}
	out, changed := fixSlices(rv, 0)
	if !changed {
		return v
	}
	return out.Interface()
}

// slicesIn reports whether values of t can hold a slice encoding/json
// would write, so most data skips the walk.
func slicesIn(t reflect.Type, seen map[reflect.Type]bool) bool {
	if r, ok := mayHaveSlices.Load(t); ok {
		return r.(bool)
	}
	if seen[t] {
		return false // a cycle: its other paths decide
	}
	seen[t] = true
	r := false
	switch {
	case encodesItself(t):
	default:
		switch t.Kind() {
		case reflect.Slice:
			r = t.Elem().Kind() != reflect.Uint8 // []byte is base64
		case reflect.Interface:
			r = true
		case reflect.Pointer, reflect.Array, reflect.Map:
			r = slicesIn(t.Elem(), seen)
		case reflect.Struct:
			for i := range t.NumField() {
				if f := t.Field(i); (f.IsExported() || f.Anonymous) && f.Tag.Get("json") != "-" && slicesIn(f.Type, seen) {
					r = true
					break
				}
			}
		}
	}
	mayHaveSlices.Store(t, r)
	return r
}

func encodesItself(t reflect.Type) bool {
	return t.Implements(marshalerType) || t.Implements(textMarshalerType) ||
		(t.Kind() != reflect.Pointer && (reflect.PointerTo(t).Implements(marshalerType) || reflect.PointerTo(t).Implements(textMarshalerType)))
}

// fixSlices returns v with nil slices made empty, and whether it changed.
func fixSlices(v reflect.Value, depth int) (reflect.Value, bool) {
	if depth > 64 || !v.IsValid() {
		return v, false
	}
	t := v.Type()
	if encodesItself(t) || !slicesIn(t, map[reflect.Type]bool{}) {
		return v, false
	}
	switch t.Kind() {
	case reflect.Slice:
		if v.IsNil() {
			return reflect.MakeSlice(t, 0, 0), true
		}
		var out reflect.Value
		for i := range v.Len() {
			e, changed := fixSlices(v.Index(i), depth+1)
			if changed {
				if !out.IsValid() {
					out = reflect.MakeSlice(t, v.Len(), v.Len())
					reflect.Copy(out, v)
				}
				out.Index(i).Set(e)
			}
		}
		if out.IsValid() {
			return out, true
		}
	case reflect.Array:
		var out reflect.Value
		for i := range v.Len() {
			e, changed := fixSlices(v.Index(i), depth+1)
			if changed {
				if !out.IsValid() {
					out = reflect.New(t).Elem()
					out.Set(v)
				}
				out.Index(i).Set(e)
			}
		}
		if out.IsValid() {
			return out, true
		}
	case reflect.Pointer:
		if v.IsNil() {
			return v, false
		}
		e, changed := fixSlices(v.Elem(), depth+1)
		if changed {
			p := reflect.New(t.Elem())
			p.Elem().Set(e)
			return p, true
		}
	case reflect.Interface:
		if v.IsNil() {
			return v, false
		}
		e, changed := fixSlices(v.Elem(), depth+1)
		if changed {
			out := reflect.New(t).Elem()
			out.Set(e)
			return out, true
		}
	case reflect.Map:
		if v.IsNil() {
			return v, false
		}
		var out reflect.Value
		it := v.MapRange()
		for it.Next() {
			e, changed := fixSlices(it.Value(), depth+1)
			if changed {
				if !out.IsValid() {
					out = reflect.MakeMapWithSize(t, v.Len())
					for _, k := range v.MapKeys() {
						out.SetMapIndex(k, v.MapIndex(k))
					}
				}
				out.SetMapIndex(it.Key(), e)
			}
		}
		if out.IsValid() {
			return out, true
		}
	case reflect.Struct:
		var out reflect.Value
		for i := range t.NumField() {
			f := t.Field(i)
			if !f.IsExported() && !f.Anonymous || f.Tag.Get("json") == "-" {
				continue
			}
			e, changed := fixSlices(v.Field(i), depth+1)
			if changed {
				if !out.IsValid() {
					out = reflect.New(t).Elem()
					out.Set(v)
				}
				if out.Field(i).CanSet() {
					out.Field(i).Set(e)
				}
			}
		}
		if out.IsValid() {
			return out, true
		}
	}
	return v, false
}

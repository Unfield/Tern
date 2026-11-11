// Package jsvalue makes native rendering's JavaScript values straight from
// Go values: what JSON.parse would make of the JSON internal/jsonenc
// writes for them, value for value and in the same key order, without
// writing or parsing the JSON. Page data reaches components this way.
//
// It follows jsonenc's rules: field names, omitempty and omitzero,
// embedded structs, nil slices as [], []byte as base64, sorted map keys.
// Values that encode themselves (json.Marshaler, encoding.TextMarshaler)
// are encoded by jsonenc and parsed, as are the rare struct tag options
// jsonenc leaves to encoding/json.
package jsvalue

import (
	"encoding/base64"
	"errors"
	"fmt"
	"math"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/Unfield/Tern/internal/jsonenc"
	"github.com/Unfield/Tern/native/js"
)

// Convert returns the JavaScript value for v in rt.
func Convert(rt *js.Realm, v any) (js.Value, error) {
	c := &state{rt: rt}
	if v == nil {
		return js.Null, nil
	}
	rv := reflect.ValueOf(v)
	return converterOf(rv.Type())(c, rv)
}

type state struct {
	rt    *js.Realm
	depth int
	// keys and vals collect the members of the objects being made, nested
	// ones after their parents', as the JSON parser does.
	keys []js.PropKey
	vals []js.Value
}

type converter func(c *state, v reflect.Value) (js.Value, error)

var cache sync.Map // reflect.Type → converter

func converterOf(t reflect.Type) converter {
	if f, ok := cache.Load(t); ok {
		return f.(converter)
	}
	var (
		wg sync.WaitGroup
		f  converter
	)
	wg.Add(1)
	fi, loaded := cache.LoadOrStore(t, converter(func(c *state, v reflect.Value) (js.Value, error) { wg.Wait(); return f(c, v) }))
	if loaded {
		return fi.(converter)
	}
	f = build(t)
	wg.Done()
	cache.Store(t, f)
	return f
}

var errDepth = errors.New("jsvalue: value is too deeply nested or has a cycle")

func (c *state) enter() error {
	c.depth++
	if c.depth > 1000 {
		return errDepth
	}
	return nil
}

func build(t reflect.Type) converter {
	if jsonenc.EncodesItself(t) {
		return viaJSON
	}
	switch t.Kind() {
	case reflect.Bool:
		return func(_ *state, v reflect.Value) (js.Value, error) { return v.Bool(), nil }
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return func(_ *state, v reflect.Value) (js.Value, error) { return js.Box(float64(v.Int())), nil }
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return func(_ *state, v reflect.Value) (js.Value, error) { return js.Box(float64(v.Uint())), nil }
	case reflect.Float32:
		return func(_ *state, v reflect.Value) (js.Value, error) {
			f := v.Float()
			if math.IsNaN(f) || math.IsInf(f, 0) {
				return nil, fmt.Errorf("jsvalue: unsupported value %v", f)
			}
			// The float32's shortest digits, as JSON has them, read as a float64.
			p, err := strconv.ParseFloat(strconv.FormatFloat(f, 'g', -1, 32), 64)
			if err != nil {
				return nil, err
			}
			return js.Box(p), nil
		}
	case reflect.Float64:
		return func(_ *state, v reflect.Value) (js.Value, error) {
			f := v.Float()
			if math.IsNaN(f) || math.IsInf(f, 0) {
				return nil, fmt.Errorf("jsvalue: unsupported value %v", f)
			}
			return js.Box(f), nil
		}
	case reflect.String:
		return func(_ *state, v reflect.Value) (js.Value, error) { return validString(v.String()), nil }
	case reflect.Interface:
		return func(c *state, v reflect.Value) (js.Value, error) {
			if v.IsNil() {
				return js.Null, nil
			}
			e := v.Elem()
			return converterOf(e.Type())(c, e)
		}
	case reflect.Pointer:
		elem := t.Elem()
		return func(c *state, v reflect.Value) (js.Value, error) {
			if v.IsNil() {
				return js.Null, nil
			}
			if err := c.enter(); err != nil {
				return nil, err
			}
			out, err := converterOf(elem)(c, v.Elem())
			c.depth--
			return out, err
		}
	case reflect.Slice:
		if t.Elem().Kind() == reflect.Uint8 && !jsonenc.EncodesItself(reflect.PointerTo(t.Elem())) && !jsonenc.EncodesItself(t.Elem()) {
			return func(_ *state, v reflect.Value) (js.Value, error) {
				if v.IsNil() {
					return js.Null, nil
				}
				return base64.StdEncoding.EncodeToString(v.Bytes()), nil
			}
		}
		elem := t.Elem()
		return func(c *state, v reflect.Value) (js.Value, error) {
			if v.IsNil() {
				return c.rt.NewArray(nil), nil // [], as jsonenc writes nil slices
			}
			if err := c.enter(); err != nil {
				return nil, err
			}
			out, err := array(c, v, elem)
			c.depth--
			return out, err
		}
	case reflect.Array:
		elem := t.Elem()
		return func(c *state, v reflect.Value) (js.Value, error) { return array(c, v, elem) }
	case reflect.Map:
		return mapConverter(t)
	case reflect.Struct:
		return structConverter(t)
	}
	return func(_ *state, v reflect.Value) (js.Value, error) {
		return nil, fmt.Errorf("jsvalue: unsupported type %s", v.Type())
	}
}

func array(c *state, v reflect.Value, elem reflect.Type) (js.Value, error) {
	f := converterOf(elem)
	out := make([]js.Value, v.Len())
	for i := range out {
		e, err := f(c, v.Index(i))
		if err != nil {
			return nil, err
		}
		out[i] = e
	}
	return c.rt.NewArray(out), nil
}

// validString is s as JSON carries it: each byte of invalid UTF-8, which
// includes encoded surrogates, becomes U+FFFD.
func validString(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			b.WriteRune(utf8.RuneError)
		} else {
			b.WriteString(s[i : i+size])
		}
		i += size
	}
	return b.String()
}

// viaJSON makes a value that encodes itself: its JSON, parsed.
func viaJSON(c *state, v reflect.Value) (js.Value, error) {
	if v.Kind() == reflect.Pointer && v.IsNil() {
		return js.Null, nil
	}
	x := v.Interface()
	if v.CanAddr() && v.Kind() != reflect.Pointer {
		x = v.Addr().Interface() // pointer-receiver marshalers, as jsonenc finds them
	}
	b, err := jsonenc.Marshal(x)
	if err != nil {
		return nil, err
	}
	return c.rt.ParseJSON(string(b))
}

func mapConverter(t reflect.Type) converter {
	if _, ok := jsonenc.MapKey(reflect.New(t.Key()).Elem()); !ok {
		return viaJSON // text-marshaled keys, and the errors for the rest
	}
	elem := t.Elem()
	return func(c *state, v reflect.Value) (js.Value, error) {
		if v.IsNil() {
			return js.Null, nil
		}
		if err := c.enter(); err != nil {
			return nil, err
		}
		defer func() { c.depth-- }()
		type kv struct {
			k string
			v reflect.Value
		}
		list := make([]kv, 0, v.Len())
		it := v.MapRange()
		for it.Next() {
			k, _ := jsonenc.MapKey(it.Key())
			list = append(list, kv{k, it.Value()})
		}
		slices.SortFunc(list, func(a, b kv) int { return strings.Compare(a.k, b.k) })
		f := converterOf(elem)
		base := len(c.keys)
		defer func() { c.keys, c.vals = c.keys[:base], c.vals[:base] }()
		for _, p := range list {
			e, err := f(c, p.v)
			if err != nil {
				return nil, err
			}
			c.keys = append(c.keys, js.PropKey(validString(p.k)))
			c.vals = append(c.vals, e)
		}
		return c.rt.NewObjectFrom(c.keys[base:], c.vals[base:]), nil
	}
}

type field struct {
	key       js.PropKey // made once: boxing a string allocates
	index     []int
	conv      converter
	omitEmpty bool
	omitZero  bool
}

func structConverter(t reflect.Type) converter {
	fs, ok := jsonenc.Fields(t)
	if !ok {
		return viaJSON
	}
	fields := make([]field, len(fs))
	for i, f := range fs {
		fields[i] = field{key: js.PropKey(validString(f.Name)), index: f.Index, conv: converterOf(f.Type), omitEmpty: f.OmitEmpty, omitZero: f.OmitZero}
	}
	return func(c *state, v reflect.Value) (js.Value, error) {
		base := len(c.keys)
		defer func() { c.keys, c.vals = c.keys[:base], c.vals[:base] }()
	next:
		for i := range fields {
			f := &fields[i]
			fv := v
			for _, x := range f.index {
				if fv.Kind() == reflect.Pointer {
					if fv.IsNil() {
						continue next // an embedded nil pointer has no fields
					}
					fv = fv.Elem()
				}
				fv = fv.Field(x)
			}
			if f.omitEmpty && jsonenc.IsEmpty(fv) || f.omitZero && jsonenc.IsZero(fv) {
				continue
			}
			e, err := f.conv(c, fv)
			if err != nil {
				return nil, err
			}
			c.keys = append(c.keys, f.key)
			c.vals = append(c.vals, e)
		}
		return c.rt.NewObjectFrom(c.keys[base:], c.vals[base:]), nil
	}
}

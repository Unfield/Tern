// Package jsonenc encodes values as encoding/json does, byte for byte,
// except that a nil slice is written as [] instead of null: Tern sends []
// for pages, typed endpoints and live components, so their TypeScript
// types are T[] rather than T[] | null.
//
// It's faster than encoding/json followed by a copy that fills in nil
// slices: each type's encoder is built once, and nothing is copied. Values
// that encode themselves (json.Marshaler, encoding.TextMarshaler) and the
// rare struct tag options it doesn't implement go through encoding/json.
package jsonenc

import (
	"bytes"
	"encoding"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/Unfield/Tern/internal/nilslices"
)

// Marshal returns the JSON encoding of v.
func Marshal(v any) ([]byte, error) {
	e := getState()
	defer putState(e)
	if err := e.marshal(v); err != nil {
		return nil, err
	}
	return bytes.Clone(e.buf), nil
}

// Append appends the JSON encoding of v to b.
func Append(b []byte, v any) ([]byte, error) {
	e := &state{buf: b}
	if err := e.marshal(v); err != nil {
		return b, err
	}
	return e.buf, nil
}

type state struct {
	buf   []byte
	depth int
}

var pool = sync.Pool{New: func() any { return &state{buf: make([]byte, 0, 4096)} }}

func getState() *state { return pool.Get().(*state) }

func putState(e *state) {
	if cap(e.buf) > 1<<20 {
		return
	}
	e.buf, e.depth = e.buf[:0], 0
	pool.Put(e)
}

// failure carries an error up through the encoders.
type failure struct{ err error }

func (e *state) marshal(v any) (err error) {
	defer func() {
		if r := recover(); r != nil {
			f, ok := r.(failure)
			if !ok {
				panic(r)
			}
			err = f.err
		}
	}()
	if v == nil {
		e.buf = append(e.buf, "null"...)
		return nil
	}
	rv := reflect.ValueOf(v)
	encoderOf(rv.Type())(e, rv)
	return nil
}

func (e *state) fail(err error) { panic(failure{err}) }

type encoder func(e *state, v reflect.Value)

var cache sync.Map // reflect.Type → encoder

func encoderOf(t reflect.Type) encoder {
	if f, ok := cache.Load(t); ok {
		return f.(encoder)
	}
	// Recursive types use the encoder through an indirection until it's built.
	var (
		wg sync.WaitGroup
		f  encoder
	)
	wg.Add(1)
	fi, loaded := cache.LoadOrStore(t, encoder(func(e *state, v reflect.Value) { wg.Wait(); f(e, v) }))
	if loaded {
		return fi.(encoder)
	}
	f = build(t)
	wg.Done()
	cache.Store(t, f)
	return f
}

// Verbatim is JSON this package wrote, written again as it is: no
// checking, compacting or escaping. Only for its own output, which is
// already compact and safe to inline in HTML.
type Verbatim []byte

// UnmarshalJSON keeps the raw JSON, as json.RawMessage does.
func (v *Verbatim) UnmarshalJSON(b []byte) error {
	*v = append((*v)[:0], b...)
	return nil
}

// MarshalJSON is for encoding/json, which checks and compacts it.
func (v Verbatim) MarshalJSON() ([]byte, error) {
	if v == nil {
		return []byte("null"), nil
	}
	return v, nil
}

var (
	verbatimType      = reflect.TypeFor[Verbatim]()
	marshalerType     = reflect.TypeFor[json.Marshaler]()
	textMarshalerType = reflect.TypeFor[encoding.TextMarshaler]()
	rawMessageType    = reflect.TypeFor[json.RawMessage]()
)

func build(t reflect.Type) encoder {
	if t == rawMessageType {
		return rawMessage
	}
	if t == verbatimType {
		return func(e *state, v reflect.Value) {
			if v.IsNil() {
				e.buf = append(e.buf, "null"...)
				return
			}
			e.buf = append(e.buf, v.Bytes()...)
		}
	}
	if t.Kind() != reflect.Pointer && reflect.PointerTo(t).Implements(marshalerType) ||
		t.Kind() != reflect.Pointer && reflect.PointerTo(t).Implements(textMarshalerType) ||
		t.Implements(marshalerType) || t.Implements(textMarshalerType) {
		return viaStdlib
	}
	switch t.Kind() {
	case reflect.Bool:
		return func(e *state, v reflect.Value) { e.buf = strconv.AppendBool(e.buf, v.Bool()) }
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return func(e *state, v reflect.Value) { e.buf = strconv.AppendInt(e.buf, v.Int(), 10) }
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return func(e *state, v reflect.Value) { e.buf = strconv.AppendUint(e.buf, v.Uint(), 10) }
	case reflect.Float32:
		return func(e *state, v reflect.Value) { e.float(v.Float(), 32) }
	case reflect.Float64:
		return func(e *state, v reflect.Value) { e.float(v.Float(), 64) }
	case reflect.String:
		return func(e *state, v reflect.Value) { e.buf = appendString(e.buf, v.String()) }
	case reflect.Interface:
		return func(e *state, v reflect.Value) {
			if v.IsNil() {
				e.buf = append(e.buf, "null"...)
				return
			}
			el := v.Elem()
			encoderOf(el.Type())(e, el)
		}
	case reflect.Pointer:
		elem := t.Elem()
		return func(e *state, v reflect.Value) {
			if v.IsNil() {
				e.buf = append(e.buf, "null"...)
				return
			}
			e.enter()
			encoderOf(elem)(e, v.Elem())
			e.depth--
		}
	case reflect.Slice:
		if t.Elem().Kind() == reflect.Uint8 && !reflect.PointerTo(t.Elem()).Implements(marshalerType) && !reflect.PointerTo(t.Elem()).Implements(textMarshalerType) {
			return bytesEncoder
		}
		elem := t.Elem()
		return func(e *state, v reflect.Value) {
			if v.IsNil() {
				e.buf = append(e.buf, "[]"...) // the difference from encoding/json
				return
			}
			e.enter()
			arrayOf(e, v, elem)
			e.depth--
		}
	case reflect.Array:
		elem := t.Elem()
		return func(e *state, v reflect.Value) { arrayOf(e, v, elem) }
	case reflect.Map:
		return mapEncoder(t)
	case reflect.Struct:
		return structEncoder(t)
	}
	return func(e *state, v reflect.Value) {
		e.fail(&json.UnsupportedTypeError{Type: v.Type()})
	}
}

func (e *state) enter() {
	e.depth++
	if e.depth > 1000 {
		e.fail(errors.New("json: value is too deeply nested or has a cycle"))
	}
}

func arrayOf(e *state, v reflect.Value, elem reflect.Type) {
	f := encoderOf(elem)
	e.buf = append(e.buf, '[')
	for i := range v.Len() {
		if i > 0 {
			e.buf = append(e.buf, ',')
		}
		f(e, v.Index(i))
	}
	e.buf = append(e.buf, ']')
}

func bytesEncoder(e *state, v reflect.Value) {
	if v.IsNil() {
		e.buf = append(e.buf, "null"...)
		return
	}
	b := v.Bytes()
	e.buf = append(e.buf, '"')
	e.buf = base64.StdEncoding.AppendEncode(e.buf, b)
	e.buf = append(e.buf, '"')
}

// viaStdlib encodes a value that encodes itself as encoding/json does,
// after filling in the nil slices it holds, as Tern always has.
func viaStdlib(e *state, v reflect.Value) {
	if v.Kind() == reflect.Pointer && v.IsNil() {
		e.buf = append(e.buf, "null"...)
		return
	}
	x := v.Interface()
	if v.CanAddr() && v.Kind() != reflect.Pointer {
		x = v.Addr().Interface() // pointer-receiver marshalers, as encoding/json finds them
	}
	b, err := json.Marshal(nilslices.Remove(x))
	if err != nil {
		e.fail(err)
	}
	e.buf = append(e.buf, b...)
}

func rawMessage(e *state, v reflect.Value) {
	raw := v.Bytes()
	if raw == nil {
		e.buf = append(e.buf, "null"...)
		return
	}
	start := len(e.buf)
	var out bytes.Buffer
	if err := json.Compact(&out, raw); err != nil {
		e.fail(&json.MarshalerError{Type: v.Type(), Err: err})
	}
	b := out.Bytes()
	if needsHTMLEscape(b) {
		out2 := bytes.Buffer{}
		json.HTMLEscape(&out2, b)
		b = out2.Bytes()
	}
	e.buf = append(e.buf[:start], b...)
}

func needsHTMLEscape(b []byte) bool {
	return bytes.ContainsAny(b, "<>&") || bytes.Contains(b, []byte(" ")) || bytes.Contains(b, []byte(" "))
}

func mapEncoder(t reflect.Type) encoder {
	k := t.Key()
	switch {
	case k.Kind() == reflect.String && !k.Implements(textMarshalerType):
	case k.Kind() >= reflect.Int && k.Kind() <= reflect.Uintptr && !k.Implements(textMarshalerType):
	default:
		return viaStdlib // text-marshaled keys, and errors for the rest
	}
	elem := encoderOf(t.Elem())
	return func(e *state, v reflect.Value) {
		if v.IsNil() {
			e.buf = append(e.buf, "null"...)
			return
		}
		e.enter()
		type kv struct {
			k string
			v reflect.Value
		}
		list := make([]kv, 0, v.Len())
		it := v.MapRange()
		for it.Next() {
			key := it.Key()
			var s string
			switch key.Kind() {
			case reflect.String:
				s = key.String()
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				s = strconv.FormatInt(key.Int(), 10)
			default:
				s = strconv.FormatUint(key.Uint(), 10)
			}
			list = append(list, kv{s, it.Value()})
		}
		slices.SortFunc(list, func(a, b kv) int { return strings.Compare(a.k, b.k) })
		e.buf = append(e.buf, '{')
		for i, p := range list {
			if i > 0 {
				e.buf = append(e.buf, ',')
			}
			e.buf = appendString(e.buf, p.k)
			e.buf = append(e.buf, ':')
			elem(e, p.v)
		}
		e.buf = append(e.buf, '}')
		e.depth--
	}
}

// field is a struct field encoding/json writes.
type field struct {
	name      string
	nameJSON  []byte // `"name":`, escaped
	index     []int
	typ       reflect.Type
	tagged    bool
	omitEmpty bool
	omitZero  bool
	enc       encoder
}

func structEncoder(t reflect.Type) encoder {
	fields, ok := typeFields(t)
	if !ok {
		return viaStdlib
	}
	for i := range fields {
		fields[i].enc = encoderOf(fields[i].typ)
		fields[i].nameJSON = append(appendString(nil, fields[i].name), ':')
	}
	return func(e *state, v reflect.Value) {
		e.buf = append(e.buf, '{')
		first := true
	next:
		for i := range fields {
			f := &fields[i]
			fv := v
			for _, x := range f.index {
				if fv.Kind() == reflect.Pointer {
					if fv.IsNil() {
						continue next // an embedded nil pointer has no fields to write
					}
					fv = fv.Elem()
				}
				fv = fv.Field(x)
			}
			if f.omitEmpty && isEmpty(fv) || f.omitZero && isZero(fv) {
				continue
			}
			if !first {
				e.buf = append(e.buf, ',')
			}
			first = false
			e.buf = append(e.buf, f.nameJSON...)
			f.enc(e, fv)
		}
		e.buf = append(e.buf, '}')
	}
}

var zeroerType = reflect.TypeFor[interface{ IsZero() bool }]()

func isZero(v reflect.Value) bool {
	if v.Type().Implements(zeroerType) {
		if v.Kind() == reflect.Pointer && v.IsNil() {
			return true
		}
		return v.Interface().(interface{ IsZero() bool }).IsZero()
	}
	if v.CanAddr() && reflect.PointerTo(v.Type()).Implements(zeroerType) {
		return v.Addr().Interface().(interface{ IsZero() bool }).IsZero()
	}
	return v.IsZero()
}

func isEmpty(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return v.Len() == 0
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64, reflect.Interface, reflect.Pointer:
		return v.IsZero()
	}
	return false
}

// typeFields finds the fields encoding/json writes for t, with its rules
// for embedded structs and names. ok is false for what's left to
// encoding/json: the ",string" option.
func typeFields(t reflect.Type) ([]field, bool) {
	type queued struct {
		typ   reflect.Type
		index []int
	}
	current := []queued{}
	next := []queued{{typ: t}}
	var count map[reflect.Type]int
	nextCount := map[reflect.Type]int{}
	visited := map[reflect.Type]bool{}
	var fields []field
	for len(next) > 0 {
		current, next = next, current[:0]
		count, nextCount = nextCount, map[reflect.Type]int{}
		for _, q := range current {
			if visited[q.typ] {
				continue
			}
			visited[q.typ] = true
			for i := range q.typ.NumField() {
				sf := q.typ.Field(i)
				if sf.Anonymous {
					ft := sf.Type
					if ft.Kind() == reflect.Pointer {
						ft = ft.Elem()
					}
					if !sf.IsExported() && ft.Kind() != reflect.Struct {
						continue
					}
				} else if !sf.IsExported() {
					continue
				}
				tag := sf.Tag.Get("json")
				if tag == "-" {
					continue
				}
				name, opts, _ := strings.Cut(tag, ",")
				if !validTag(name) {
					name = ""
				}
				index := append(slices.Clone(q.index), i)
				ft := sf.Type
				if ft.Name() == "" && ft.Kind() == reflect.Pointer {
					ft = ft.Elem()
				}
				hasOpt := func(o string) bool {
					for _, x := range strings.Split(opts, ",") {
						if x == o {
							return true
						}
					}
					return false
				}
				if hasOpt("string") {
					return nil, false
				}
				if name != "" || !sf.Anonymous || ft.Kind() != reflect.Struct {
					tagged := name != ""
					if name == "" {
						name = sf.Name
					}
					f := field{name: name, index: index, typ: sf.Type, tagged: tagged, omitEmpty: hasOpt("omitempty"), omitZero: hasOpt("omitzero")}
					fields = append(fields, f)
					if count[q.typ] > 1 {
						// Two embedded types at one depth: both fields are there,
						// and dominantField drops them.
						fields = append(fields, fields[len(fields)-1])
					}
					continue
				}
				nextCount[ft]++
				if nextCount[ft] == 1 {
					next = append(next, queued{typ: ft, index: index})
				}
			}
		}
	}
	// encoding/json's order: by name, then depth, then tagged, then index.
	slices.SortFunc(fields, func(a, b field) int {
		if c := strings.Compare(a.name, b.name); c != 0 {
			return c
		}
		if len(a.index) != len(b.index) {
			return len(a.index) - len(b.index)
		}
		if a.tagged != b.tagged {
			if a.tagged {
				return -1
			}
			return 1
		}
		return slices.Compare(a.index, b.index)
	})
	out := fields[:0]
	for adv, i := 0, 0; i < len(fields); i += adv {
		fi := fields[i]
		for adv = 1; i+adv < len(fields); adv++ {
			if fields[i+adv].name != fi.name {
				break
			}
		}
		if adv == 1 {
			out = append(out, fi)
			continue
		}
		if dominant, ok := dominantField(fields[i : i+adv]); ok {
			out = append(out, dominant)
		}
	}
	fields = out
	slices.SortFunc(fields, func(a, b field) int { return slices.Compare(a.index, b.index) })
	return fields, true
}

func dominantField(fs []field) (field, bool) {
	if len(fs) > 1 && len(fs[0].index) == len(fs[1].index) && fs[0].tagged == fs[1].tagged {
		return field{}, false
	}
	return fs[0], true
}

func validTag(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		switch {
		case strings.ContainsRune("!#$%&()*+-./:;<=>?@[]^_{|}~ ", c):
		case (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c <= 127:
			return false
		}
	}
	return true
}

// float writes f as encoding/json does: like ES6's Number#toString.
func (e *state) float(f float64, bits int) {
	if math.IsInf(f, 0) || math.IsNaN(f) {
		e.fail(&json.UnsupportedValueError{Value: reflect.ValueOf(f), Str: strconv.FormatFloat(f, 'g', -1, bits)})
	}
	abs := math.Abs(f)
	format := byte('f')
	if abs != 0 {
		if bits == 64 && (abs < 1e-6 || abs >= 1e21) || bits == 32 && (float32(abs) < 1e-6 || float32(abs) >= 1e21) {
			format = 'e'
		}
	}
	b := strconv.AppendFloat(e.buf, f, format, -1, bits)
	if format == 'e' {
		// clean up e-09 to e-9
		n := len(b)
		if n >= 4 && b[n-4] == 'e' && b[n-3] == '-' && b[n-2] == '0' {
			b[n-2] = b[n-1]
			b = b[:n-1]
		}
	}
	e.buf = b
}

const hexDigits = "0123456789abcdef"

// replacement is what encoding/json writes for invalid UTF-8: U+FFFD, as
// the character or escaped, depending on the Go release.
var replacement = func() string {
	b, _ := json.Marshal("\xff")
	return string(b[1 : len(b)-1])
}()

// appendString writes s quoted, escaped as encoding/json does by default:
// HTML characters too, and invalid UTF-8 as U+FFFD.
func appendString(b []byte, s string) []byte {
	b = append(b, '"')
	start := 0
	for i := 0; i < len(s); {
		if c := s[i]; c < utf8.RuneSelf {
			if c >= 0x20 && c != '"' && c != '\\' && c != '<' && c != '>' && c != '&' {
				i++
				continue
			}
			b = append(b, s[start:i]...)
			switch c {
			case '\\', '"':
				b = append(b, '\\', c)
			case '\b':
				b = append(b, '\\', 'b')
			case '\f':
				b = append(b, '\\', 'f')
			case '\n':
				b = append(b, '\\', 'n')
			case '\r':
				b = append(b, '\\', 'r')
			case '\t':
				b = append(b, '\\', 't')
			default:
				b = append(b, '\\', 'u', '0', '0', hexDigits[c>>4], hexDigits[c&0xf])
			}
			i++
			start = i
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			b = append(b, s[start:i]...)
			b = append(b, replacement...)
			i += size
			start = i
			continue
		}
		if r == ' ' || r == ' ' {
			b = append(b, s[start:i]...)
			b = append(b, '\\', 'u', '2', '0', '2', hexDigits[r&0xf])
			i += size
			start = i
			continue
		}
		i += size
	}
	b = append(b, s[start:]...)
	return append(b, '"')
}

// Field is a struct field as encoding/json writes it, for other encoders
// that follow its rules.
type Field struct {
	Name string
	// Index is the path to the field through embedded structs.
	Index     []int
	Type      reflect.Type
	OmitEmpty bool
	OmitZero  bool
}

// Fields returns the fields encoding/json writes for struct type t, in
// order. ok is false for structs it leaves to encoding/json (the ",string"
// option).
func Fields(t reflect.Type) (fields []Field, ok bool) {
	fs, ok := typeFields(t)
	if !ok {
		return nil, false
	}
	out := make([]Field, len(fs))
	for i, f := range fs {
		out[i] = Field{Name: f.name, Index: f.index, Type: f.typ, OmitEmpty: f.omitEmpty, OmitZero: f.omitZero}
	}
	return out, true
}

// EncodesItself reports whether values of t are written by their own
// MarshalJSON or MarshalText, or need encoding/json for another reason:
// what this package hands to encoding/json.
func EncodesItself(t reflect.Type) bool {
	if t == rawMessageType || t == verbatimType {
		return true
	}
	if t.Kind() != reflect.Pointer && (reflect.PointerTo(t).Implements(marshalerType) || reflect.PointerTo(t).Implements(textMarshalerType)) {
		return true
	}
	return t.Implements(marshalerType) || t.Implements(textMarshalerType)
}

// IsEmpty and IsZero are the omitempty and omitzero tests.
func IsEmpty(v reflect.Value) bool { return isEmpty(v) }

// IsZero is the omitzero test.
func IsZero(v reflect.Value) bool { return isZero(v) }

// MapKey is a map key as encoding/json writes it, for the map kinds this
// package encodes itself: string and integer keys.
func MapKey(k reflect.Value) (string, bool) {
	t := k.Type()
	switch {
	case t.Kind() == reflect.String && !t.Implements(textMarshalerType):
		return k.String(), true
	case t.Kind() >= reflect.Int && t.Kind() <= reflect.Int64 && !t.Implements(textMarshalerType):
		return strconv.FormatInt(k.Int(), 10), true
	case t.Kind() >= reflect.Uint && t.Kind() <= reflect.Uintptr && !t.Implements(textMarshalerType):
		return strconv.FormatUint(k.Uint(), 10), true
	}
	return "", false
}

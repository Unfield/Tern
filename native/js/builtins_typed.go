package js

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"math"
	"slices"
	"strings"
)

// ArrayBuffer, the typed arrays and DataView, as QuickJS has them. The
// BigInt arrays are missing: there are no BigInts natively, so code that
// builds one falls back to the JavaScript engine.

// arrayBuffer is an ArrayBuffer's data.
type arrayBuffer struct{ data []byte }

// typedKind is one typed array type, such as Uint8Array.
type typedKind struct {
	name  string
	size  int
	get   func(b []byte) float64
	put   func(b []byte, v float64)
	proto *Object
	ctor  *Object
}

// typedArray is the internal state of a typed array: a view of a buffer.
type typedArray struct {
	kind   *typedKind
	buffer *Object
	offset int
	length int
}

func (t *typedArray) bytes() []byte { return t.buffer.Internal.(*arrayBuffer).data }

func (t *typedArray) at(i int) float64 {
	return t.kind.get(t.bytes()[t.offset+i*t.kind.size:])
}

func (t *typedArray) put(i int, v Value) {
	n := ToNumber(v) // converted even when out of range, as the spec says
	if i >= 0 && i < t.length {
		t.kind.put(t.bytes()[t.offset+i*t.kind.size:], n)
	}
}

var le = binary.LittleEndian

func toIntN(v float64, bits uint) int64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	m := math.Mod(math.Trunc(v), math.Exp2(float64(bits)))
	return int64(m)
}

func clampUint8(v float64) byte {
	switch {
	case math.IsNaN(v) || v <= 0:
		return 0
	case v >= 255:
		return 255
	}
	return byte(math.RoundToEven(v))
}

var typedKinds = []*typedKind{
	{name: "Int8Array", size: 1, get: func(b []byte) float64 { return float64(int8(b[0])) }, put: func(b []byte, v float64) { b[0] = byte(toIntN(v, 8)) }},
	{name: "Uint8Array", size: 1, get: func(b []byte) float64 { return float64(b[0]) }, put: func(b []byte, v float64) { b[0] = byte(toIntN(v, 8)) }},
	{name: "Uint8ClampedArray", size: 1, get: func(b []byte) float64 { return float64(b[0]) }, put: func(b []byte, v float64) { b[0] = clampUint8(v) }},
	{name: "Int16Array", size: 2, get: func(b []byte) float64 { return float64(int16(le.Uint16(b))) }, put: func(b []byte, v float64) { le.PutUint16(b, uint16(toIntN(v, 16))) }},
	{name: "Uint16Array", size: 2, get: func(b []byte) float64 { return float64(le.Uint16(b)) }, put: func(b []byte, v float64) { le.PutUint16(b, uint16(toIntN(v, 16))) }},
	{name: "Int32Array", size: 4, get: func(b []byte) float64 { return float64(int32(le.Uint32(b))) }, put: func(b []byte, v float64) { le.PutUint32(b, uint32(toIntN(v, 32))) }},
	{name: "Uint32Array", size: 4, get: func(b []byte) float64 { return float64(le.Uint32(b)) }, put: func(b []byte, v float64) { le.PutUint32(b, uint32(toIntN(v, 32))) }},
	{name: "Float32Array", size: 4, get: func(b []byte) float64 { return float64(math.Float32frombits(le.Uint32(b))) }, put: func(b []byte, v float64) { le.PutUint32(b, math.Float32bits(float32(v))) }},
	{name: "Float64Array", size: 8, get: func(b []byte) float64 { return math.Float64frombits(le.Uint64(b)) }, put: func(b []byte, v float64) { le.PutUint64(b, math.Float64bits(v)) }},
}

// ArrayBufferPrototype is ArrayBuffer.prototype.
var ArrayBufferPrototype = &Object{proto: ObjectPrototype}

// TypedArrayPrototype is %TypedArray%.prototype, shared by every kind.
var TypedArrayPrototype = &Object{proto: ObjectPrototype}

// NewArrayBuffer makes an ArrayBuffer holding data.
func NewArrayBuffer(data []byte) *Object {
	o := NewObject(ArrayBufferPrototype)
	o.Class, o.Internal = "ArrayBuffer", &arrayBuffer{data: data}
	return o
}

func thisBuffer(v Value, name string) *arrayBuffer {
	if o, ok := v.(*Object); ok && o.Class == "ArrayBuffer" {
		return o.Internal.(*arrayBuffer)
	}
	panic(TypeErrorf("%s called on incompatible receiver", name))
}

func thisTyped(v Value) *typedArray {
	if o, ok := v.(*Object); ok && o.typed != nil {
		return o.typed
	}
	panic(TypeErrorf("not a TypedArray"))
}

func toIndex(v Value, what string) int {
	if v == Undefined {
		return 0
	}
	n := ToIntegerOrInfinity(v)
	if n < 0 || n > 1<<53-1 {
		panic(RangeErrorf("invalid %s", what))
	}
	if n > 1<<31 {
		panic(RangeErrorf("invalid array buffer length"))
	}
	return int(n)
}

func newTyped(k *typedKind, proto *Object, buffer *Object, offset, length int) *Object {
	o := NewObject(proto)
	o.Class = k.name
	o.typed = &typedArray{kind: k, buffer: buffer, offset: offset, length: length}
	return o
}

func allocTyped(k *typedKind, length int) *Object {
	return newTyped(k, k.proto, NewArrayBuffer(make([]byte, length*k.size)), 0, length)
}

// typedFrom builds a typed array of kind k from values.
func typedFrom(k *typedKind, proto *Object, vals []Value) *Object {
	o := newTyped(k, proto, NewArrayBuffer(make([]byte, len(vals)*k.size)), 0, len(vals))
	for i, v := range vals {
		o.typed.put(i, v)
	}
	return o
}

func (t *typedArray) values() []Value {
	out := make([]Value, t.length)
	for i := range out {
		out[i] = t.at(i)
	}
	return out
}

// speciesCreate makes a typed array of the same kind as t, as the
// prototype methods that return a new array do.
func (t *typedArray) create(length int) *Object { return allocTyped(t.kind, length) }

func setupTypedArrays() {
	setupArrayBuffer()

	ta := NewConstructor("TypedArray", 0, func(Value, []Value) Value {
		panic(TypeErrorf("cannot be called"))
	}, func([]Value, *Object) Value { panic(TypeErrorf("cannot be called")) }, TypedArrayPrototype)
	setupTypedPrototype(ta)

	for _, k := range typedKinds {
		k.proto = &Object{proto: TypedArrayPrototype}
		k.ctor = NewConstructor(k.name, 3, func(Value, []Value) Value {
			panic(TypeErrorf("constructor requires 'new'"))
		}, func(args []Value, nt *Object) Value {
			proto := OrdinaryCreateFromConstructor(nt, k.proto).proto
			return constructTyped(k, proto, args)
		}, k.proto)
		k.ctor.SetProto(ta)
		k.ctor.add(prop{key: "BYTES_PER_ELEMENT", value: float64(k.size)})
		k.proto.add(prop{key: "BYTES_PER_ELEMENT", value: float64(k.size)})
		Global.DefineHidden(k.name, k.ctor)
	}
	setupUint8Base64(typedKinds[1])
	setupDataView()
}

func constructTyped(k *typedKind, proto *Object, args []Value) *Object {
	first := Arg(args, 0)
	src, ok := first.(*Object)
	if !ok {
		n := toIndex(first, "length")
		return newTyped(k, proto, NewArrayBuffer(make([]byte, n*k.size)), 0, n)
	}
	if src.Class == "ArrayBuffer" {
		data := src.Internal.(*arrayBuffer).data
		off := toIndex(Arg(args, 1), "offset")
		if off%k.size != 0 {
			panic(RangeErrorf("invalid offset"))
		}
		var n int
		if l := Arg(args, 2); l != Undefined {
			n = toIndex(l, "length")
			if off+n*k.size > len(data) {
				panic(RangeErrorf("invalid length"))
			}
		} else {
			if len(data)%k.size != 0 || off > len(data) {
				panic(RangeErrorf("invalid length"))
			}
			n = (len(data) - off) / k.size
		}
		return newTyped(k, proto, src, off, n)
	}
	if src.typed != nil {
		return typedFrom(k, proto, src.typed.values())
	}
	if it, ok := src.Get(SymbolIterator).(*Object); ok && it.call != nil {
		return typedFrom(k, proto, Spread(src))
	}
	return typedFrom(k, proto, ListFromArrayLike(src))
}

func setupArrayBuffer() {
	ctor := NewConstructor("ArrayBuffer", 1, func(Value, []Value) Value {
		panic(TypeErrorf("constructor requires 'new'"))
	}, func(args []Value, nt *Object) Value {
		o := OrdinaryCreateFromConstructor(nt, ArrayBufferPrototype)
		o.Class, o.Internal = "ArrayBuffer", &arrayBuffer{data: make([]byte, toIndex(Arg(args, 0), "array buffer length"))}
		return o
	}, ArrayBufferPrototype)
	Global.DefineHidden("ArrayBuffer", ctor)
	method(ctor, "isView", 1, func(_ Value, args []Value) Value {
		o, ok := Arg(args, 0).(*Object)
		return ok && (o.typed != nil || o.Class == "DataView")
	})
	p := ArrayBufferPrototype
	p.DefineHidden(SymbolToStringTag, "ArrayBuffer")
	p.DefineAccessor("byteLength", NewFunction("get byteLength", 0, func(t Value, _ []Value) Value {
		return float64(len(thisBuffer(t, "ArrayBuffer.prototype.byteLength").data))
	}), nil, false)
	method(p, "slice", 2, func(t Value, args []Value) Value {
		d := thisBuffer(t, "ArrayBuffer.prototype.slice").data
		start := relIndex(Arg(args, 0), len(d), 0)
		end := relIndex(Arg(args, 1), len(d), len(d))
		return NewArrayBuffer(append([]byte(nil), d[start:max(start, end)]...))
	})
}

func setupTypedPrototype(ctor *Object) {
	p := TypedArrayPrototype
	getter := func(name string, fn func(t *typedArray) Value) {
		p.DefineAccessor(name, NewFunction("get "+name, 0, func(this Value, _ []Value) Value { return fn(thisTyped(this)) }), nil, false)
	}
	getter("buffer", func(t *typedArray) Value { return t.buffer })
	getter("byteLength", func(t *typedArray) Value { return float64(t.length * t.kind.size) })
	getter("byteOffset", func(t *typedArray) Value { return float64(t.offset) })
	getter("length", func(t *typedArray) Value { return float64(t.length) })
	p.DefineAccessor(SymbolToStringTag, NewFunction("get [Symbol.toStringTag]", 0, func(this Value, _ []Value) Value {
		if o, ok := this.(*Object); ok && o.typed != nil {
			return o.typed.kind.name
		}
		return Undefined
	}), nil, false)

	method(ctor, "from", 1, func(this Value, args []Value) Value {
		k := kindOfCtor(this)
		src := Arg(args, 0)
		var vals []Value
		if o, ok := src.(*Object); ok {
			if it, ok := o.Get(SymbolIterator).(*Object); ok && it.call != nil {
				vals = Spread(o)
			} else {
				vals = ListFromArrayLike(o)
			}
		} else {
			vals = ListFromArrayLike(ToObject(src))
		}
		if f := Arg(args, 1); f != Undefined {
			fn := callback(args[1:], k.name+".from")
			for i, v := range vals {
				vals[i] = fn.invoke(Arg(args, 2), v, float64(i))
			}
		}
		return typedFrom(k, k.proto, vals)
	})
	method(ctor, "of", 0, func(this Value, args []Value) Value {
		k := kindOfCtor(this)
		return typedFrom(k, k.proto, args)
	})

	method(p, "at", 1, func(this Value, args []Value) Value {
		t := thisTyped(this)
		n := ToIntegerOrInfinity(Arg(args, 0))
		if n < 0 {
			n += float64(t.length)
		}
		if n < 0 || n >= float64(t.length) {
			return Undefined
		}
		return t.at(int(n))
	})
	method(p, "fill", 1, func(this Value, args []Value) Value {
		t := thisTyped(this)
		v := ToNumber(Arg(args, 0))
		start := relIndex(Arg(args, 1), t.length, 0)
		end := relIndex(Arg(args, 2), t.length, t.length)
		for i := start; i < end; i++ {
			t.put(i, v)
		}
		return this
	})
	method(p, "set", 1, func(this Value, args []Value) Value {
		t := thisTyped(this)
		off := ToIntegerOrInfinity(Arg(args, 1))
		if off < 0 {
			panic(RangeErrorf("invalid offset"))
		}
		var vals []Value
		if o, ok := Arg(args, 0).(*Object); ok && o.typed != nil {
			vals = o.typed.values()
		} else {
			vals = ListFromArrayLike(ToObject(Arg(args, 0)))
		}
		if off+float64(len(vals)) > float64(t.length) {
			panic(RangeErrorf("offset is out of bounds"))
		}
		for i, v := range vals {
			t.put(int(off)+i, v)
		}
		return Undefined
	})
	method(p, "subarray", 2, func(this Value, args []Value) Value {
		t := thisTyped(this)
		start := relIndex(Arg(args, 0), t.length, 0)
		end := max(start, relIndex(Arg(args, 1), t.length, t.length))
		return newTyped(t.kind, t.kind.proto, t.buffer, t.offset+start*t.kind.size, end-start)
	})
	method(p, "slice", 2, func(this Value, args []Value) Value {
		t := thisTyped(this)
		start := relIndex(Arg(args, 0), t.length, 0)
		end := max(start, relIndex(Arg(args, 1), t.length, t.length))
		o := t.create(end - start)
		copy(o.typed.bytes(), t.bytes()[t.offset+start*t.kind.size:t.offset+end*t.kind.size])
		return o
	})
	method(p, "copyWithin", 2, func(this Value, args []Value) Value {
		t := thisTyped(this)
		to := relIndex(Arg(args, 0), t.length, 0)
		from := relIndex(Arg(args, 1), t.length, 0)
		end := relIndex(Arg(args, 2), t.length, t.length)
		n := min(end-from, t.length-to)
		if n > 0 {
			b, s := t.bytes(), t.kind.size
			copy(b[t.offset+to*s:], b[t.offset+from*s:t.offset+(from+n)*s])
		}
		return this
	})
	method(p, "reverse", 0, func(this Value, _ []Value) Value {
		t := thisTyped(this)
		vals := t.values()
		for i, v := range vals {
			t.put(t.length-1-i, v)
		}
		return this
	})
	method(p, "toReversed", 0, func(this Value, _ []Value) Value {
		t := thisTyped(this)
		vals := t.values()
		slices.Reverse(vals)
		return typedFrom(t.kind, t.kind.proto, vals)
	})
	sortTyped := func(vals []Value, cmp Value) {
		if cmp != Undefined {
			if f, ok := cmp.(*Object); !ok || f.call == nil {
				panic(TypeErrorf("not a function"))
			}
			sortValues(vals, cmp)
			return
		}
		slices.SortStableFunc(vals, func(a, b Value) int {
			x, y := a.(float64), b.(float64)
			switch {
			case math.IsNaN(x) && math.IsNaN(y):
				return 0
			case math.IsNaN(x):
				return 1
			case math.IsNaN(y):
				return -1
			case x < y:
				return -1
			case x > y:
				return 1
			case x == 0 && y == 0:
				if math.Signbit(x) && !math.Signbit(y) {
					return -1
				}
				if !math.Signbit(x) && math.Signbit(y) {
					return 1
				}
			}
			return 0
		})
	}
	method(p, "sort", 1, func(this Value, args []Value) Value {
		t := thisTyped(this)
		vals := t.values()
		sortTyped(vals, Arg(args, 0))
		for i, v := range vals {
			t.put(i, v)
		}
		return this
	})
	method(p, "toSorted", 1, func(this Value, args []Value) Value {
		t := thisTyped(this)
		vals := t.values()
		sortTyped(vals, Arg(args, 0))
		return typedFrom(t.kind, t.kind.proto, vals)
	})
	method(p, "with", 2, func(this Value, args []Value) Value {
		t := thisTyped(this)
		n := ToIntegerOrInfinity(Arg(args, 0))
		if n < 0 {
			n += float64(t.length)
		}
		v := ToNumber(Arg(args, 1))
		if n < 0 || n >= float64(t.length) {
			panic(RangeErrorf("invalid array index"))
		}
		vals := t.values()
		vals[int(n)] = v
		return typedFrom(t.kind, t.kind.proto, vals)
	})
	method(p, "join", 1, func(this Value, args []Value) Value {
		t := thisTyped(this)
		sep := ","
		if s := Arg(args, 0); s != Undefined {
			sep = ToString(s)
		}
		return Join(t.values(), sep)
	})
	method(p, "toLocaleString", 0, func(this Value, _ []Value) Value { return Join(thisTyped(this).values(), ",") })
	p.DefineHidden("toString", ArrayPrototype.Get("toString"))

	search := func(name string, last, includes bool) {
		method(p, name, 1, func(this Value, args []Value) Value {
			t := thisTyped(this)
			x := Arg(args, 0)
			n := t.length
			if n == 0 {
				if includes {
					return false
				}
				return -1.0
			}
			match := func(i int) bool {
				if includes {
					return SameValueZero(t.at(i), x)
				}
				return StrictEquals(t.at(i), x)
			}
			if last {
				from := n - 1
				if len(args) > 1 {
					f := ToIntegerOrInfinity(args[1])
					if f < 0 {
						f += float64(n)
					}
					from = int(math.Min(f, float64(n-1)))
				}
				for i := from; i >= 0; i-- {
					if match(i) {
						return float64(i)
					}
				}
				return -1.0
			}
			for i := relIndex(Arg(args, 1), n, 0); i < n; i++ {
				if match(i) {
					if includes {
						return true
					}
					return float64(i)
				}
			}
			if includes {
				return false
			}
			return -1.0
		})
	}
	search("indexOf", false, false)
	search("lastIndexOf", true, false)
	search("includes", false, true)

	// The callback methods.
	each := func(name string, fn func(t *typedArray, cb *Object, thisArg Value, this Value) Value) {
		method(p, name, 1, func(this Value, args []Value) Value {
			t := thisTyped(this)
			cb := callback(args, "TypedArray.prototype."+name)
			return fn(t, cb, Arg(args, 1), this)
		})
	}
	call := func(cb *Object, thisArg Value, t *typedArray, i int, this Value) Value {
		return cb.invoke(thisArg, t.at(i), float64(i), this)
	}
	each("forEach", func(t *typedArray, cb *Object, ta Value, this Value) Value {
		for i := 0; i < t.length; i++ {
			call(cb, ta, t, i, this)
		}
		return Undefined
	})
	each("map", func(t *typedArray, cb *Object, ta Value, this Value) Value {
		o := t.create(t.length)
		for i := 0; i < t.length; i++ {
			o.typed.put(i, call(cb, ta, t, i, this))
		}
		return o
	})
	each("filter", func(t *typedArray, cb *Object, ta Value, this Value) Value {
		var kept []Value
		for i := 0; i < t.length; i++ {
			v := t.at(i)
			if Truthy(cb.invoke(ta, v, float64(i), this)) {
				kept = append(kept, v)
			}
		}
		return typedFrom(t.kind, t.kind.proto, kept)
	})
	each("every", func(t *typedArray, cb *Object, ta Value, this Value) Value {
		for i := 0; i < t.length; i++ {
			if !Truthy(call(cb, ta, t, i, this)) {
				return false
			}
		}
		return true
	})
	each("some", func(t *typedArray, cb *Object, ta Value, this Value) Value {
		for i := 0; i < t.length; i++ {
			if Truthy(call(cb, ta, t, i, this)) {
				return true
			}
		}
		return false
	})
	find := func(name string, last, index bool) {
		each(name, func(t *typedArray, cb *Object, ta Value, this Value) Value {
			for j := 0; j < t.length; j++ {
				i := j
				if last {
					i = t.length - 1 - j
				}
				if Truthy(call(cb, ta, t, i, this)) {
					if index {
						return float64(i)
					}
					return t.at(i)
				}
			}
			if index {
				return -1.0
			}
			return Undefined
		})
	}
	find("find", false, false)
	find("findIndex", false, true)
	find("findLast", true, false)
	find("findLastIndex", true, true)
	reduce := func(name string, right bool) {
		method(p, name, 1, func(this Value, args []Value) Value {
			t := thisTyped(this)
			cb := callback(args, "TypedArray.prototype."+name)
			idx := func(j int) int {
				if right {
					return t.length - 1 - j
				}
				return j
			}
			j := 0
			var acc Value
			if len(args) > 1 {
				acc = args[1]
			} else {
				if t.length == 0 {
					panic(TypeErrorf("empty array"))
				}
				acc = t.at(idx(0))
				j = 1
			}
			for ; j < t.length; j++ {
				i := idx(j)
				acc = cb.invoke(Undefined, acc, t.at(i), float64(i), this)
			}
			return acc
		})
	}
	reduce("reduce", false)
	reduce("reduceRight", true)

	iter := func(name string, item func(t *typedArray, i int) Value) *Object {
		return method(p, name, 0, func(this Value, _ []Value) Value {
			t := thisTyped(this)
			i := 0
			return NewIterator(func() (Value, bool) {
				if i >= t.length {
					return Undefined, false
				}
				i++
				return item(t, i-1), true
			})
		})
	}
	iter("keys", func(_ *typedArray, i int) Value { return float64(i) })
	iter("entries", func(t *typedArray, i int) Value { return NewArray([]Value{float64(i), t.at(i)}) })
	values := iter("values", func(t *typedArray, i int) Value { return t.at(i) })
	p.DefineHidden(SymbolIterator, values)
}

func kindOfCtor(v Value) *typedKind {
	for _, k := range typedKinds {
		if v == Value(k.ctor) {
			return k
		}
	}
	panic(TypeErrorf("not a constructor"))
}

// Uint8Array's base64 and hex methods (ES2026), which QuickJS has.
func setupUint8Base64(k *typedKind) {
	encoding := func(opts Value) *base64.Encoding {
		url := false
		if o, ok := opts.(*Object); ok {
			switch a := o.Get("alphabet"); a {
			case Undefined, "base64":
			case "base64url":
				url = true
			default:
				panic(TypeErrorf("expected 'base64' or 'base64url'"))
			}
		} else if opts != Undefined {
			panic(TypeErrorf("options must be an object"))
		}
		if url {
			return base64.URLEncoding
		}
		return base64.StdEncoding
	}
	method(k.ctor, "fromBase64", 1, func(_ Value, args []Value) Value {
		s, ok := Arg(args, 0).(string)
		if !ok {
			panic(TypeErrorf("expected string"))
		}
		enc := encoding(Arg(args, 1))
		handling := "loose"
		if o, ok := Arg(args, 1).(*Object); ok {
			if h := o.Get("lastChunkHandling"); h != Undefined {
				handling = ToString(h)
			}
		}
		clean := strings.Map(func(r rune) rune {
			switch r {
			case ' ', '\t', '\n', '\f', '\r':
				return -1
			}
			return r
		}, s)
		data, err := decodeBase64(clean, enc, handling)
		if err != "" {
			panic(newError(SyntaxError, err))
		}
		o := allocTyped(k, len(data))
		copy(o.typed.bytes(), data)
		return o
	})
	method(k.ctor, "fromHex", 1, func(_ Value, args []Value) Value {
		s, ok := Arg(args, 0).(string)
		if !ok {
			panic(TypeErrorf("expected string"))
		}
		data, err := hex.DecodeString(s)
		if err != nil {
			panic(newError(SyntaxError, "invalid hex string"))
		}
		o := allocTyped(k, len(data))
		copy(o.typed.bytes(), data)
		return o
	})
	method(k.proto, "toBase64", 0, func(this Value, args []Value) Value {
		t := thisUint8(this, k)
		enc := encoding(Arg(args, 0))
		if o, ok := Arg(args, 0).(*Object); ok && Truthy(o.Get("omitPadding")) {
			enc = enc.WithPadding(base64.NoPadding)
		}
		return enc.EncodeToString(t.bytes()[t.offset : t.offset+t.length])
	})
	method(k.proto, "toHex", 0, func(this Value, _ []Value) Value {
		t := thisUint8(this, k)
		return hex.EncodeToString(t.bytes()[t.offset : t.offset+t.length])
	})
}

func thisUint8(v Value, k *typedKind) *typedArray {
	if o, ok := v.(*Object); ok && o.typed != nil && o.typed.kind == k {
		return o.typed
	}
	panic(TypeErrorf("not a Uint8Array"))
}

// decodeBase64 decodes s, with lastChunkHandling "loose", "strict" or
// "stop-before-partial". It returns an error message on bad input.
func decodeBase64(s string, enc *base64.Encoding, handling string) ([]byte, string) {
	raw := strings.TrimRight(s, "=")
	pad := len(s) - len(raw)
	if pad > 2 || strings.Contains(raw, "=") {
		return nil, "invalid base64 string"
	}
	rem := len(raw) % 4
	if rem == 1 {
		return nil, "invalid base64 string"
	}
	switch handling {
	case "strict":
		if pad > 0 && (len(raw)+pad)%4 != 0 || pad == 0 && rem != 0 {
			return nil, "invalid base64 string"
		}
	case "stop-before-partial":
		if pad == 0 && rem != 0 {
			raw = raw[:len(raw)-rem]
		}
	case "loose":
	default:
		return nil, "invalid lastChunkHandling"
	}
	dec := enc.WithPadding(base64.NoPadding)
	if handling == "strict" {
		dec = dec.Strict() // the bits past the last byte must be zero
	}
	data, err := dec.DecodeString(raw)
	if err != nil {
		return nil, "invalid base64 string"
	}
	return data, ""
}

// DataViewPrototype is DataView.prototype.
var DataViewPrototype = &Object{proto: ObjectPrototype}

type dataView struct {
	buffer *Object
	offset int
	length int
}

func setupDataView() {
	ctor := NewConstructor("DataView", 1, func(Value, []Value) Value {
		panic(TypeErrorf("constructor requires 'new'"))
	}, func(args []Value, nt *Object) Value {
		b, ok := Arg(args, 0).(*Object)
		if !ok || b.Class != "ArrayBuffer" {
			panic(TypeErrorf("argument must be an ArrayBuffer"))
		}
		size := len(b.Internal.(*arrayBuffer).data)
		off := toIndex(Arg(args, 1), "byteOffset")
		if off > size {
			panic(RangeErrorf("invalid byteOffset"))
		}
		n := size - off
		if l := Arg(args, 2); l != Undefined {
			n = toIndex(l, "byteLength")
			if off+n > size {
				panic(RangeErrorf("invalid byteLength"))
			}
		}
		o := OrdinaryCreateFromConstructor(nt, DataViewPrototype)
		o.Class, o.Internal = "DataView", &dataView{buffer: b, offset: off, length: n}
		return o
	}, DataViewPrototype)
	Global.DefineHidden("DataView", ctor)
	p := DataViewPrototype
	p.DefineHidden(SymbolToStringTag, "DataView")
	this := func(v Value) *dataView {
		if o, ok := v.(*Object); ok && o.Class == "DataView" {
			return o.Internal.(*dataView)
		}
		panic(TypeErrorf("not a DataView"))
	}
	p.DefineAccessor("buffer", NewFunction("get buffer", 0, func(t Value, _ []Value) Value { return this(t).buffer }), nil, false)
	p.DefineAccessor("byteLength", NewFunction("get byteLength", 0, func(t Value, _ []Value) Value { return float64(this(t).length) }), nil, false)
	p.DefineAccessor("byteOffset", NewFunction("get byteOffset", 0, func(t Value, _ []Value) Value { return float64(this(t).offset) }), nil, false)
	for _, k := range typedKinds {
		if k.name == "Uint8ClampedArray" {
			continue
		}
		name := strings.TrimSuffix(k.name, "Array")
		at := func(t Value, args []Value) (*dataView, []byte, bool) {
			d := this(t)
			i := toIndex(Arg(args, 0), "offset")
			if i+k.size > d.length {
				panic(RangeErrorf("out of bound"))
			}
			b := d.buffer.Internal.(*arrayBuffer).data[d.offset+i : d.offset+i+k.size]
			return d, b, false
		}
		method(p, "get"+name, 1, func(t Value, args []Value) Value {
			_, b, _ := at(t, args)
			buf := append([]byte(nil), b...)
			if !Truthy(Arg(args, 1)) {
				slices.Reverse(buf)
			}
			return k.get(buf)
		})
		method(p, "set"+name, 2, func(t Value, args []Value) Value {
			v := ToNumber(Arg(args, 1))
			_, b, _ := at(t, args)
			buf := make([]byte, k.size)
			k.put(buf, v)
			if !Truthy(Arg(args, 2)) {
				slices.Reverse(buf)
			}
			copy(b, buf)
			return Undefined
		})
	}
}

// canonicalIndex reports whether key is a canonical numeric string, which
// typed arrays treat as an element access even when it is out of range,
// and the integer index if it is one.
func canonicalIndex(key PropKey) (i int, numeric, isIndex bool) {
	if n, ok := arrayIndex(key); ok {
		return n, true, true
	}
	s, ok := key.(string)
	if !ok || s == "" {
		return 0, false, false
	}
	if c := s[0]; c != '-' && c != 'I' && c != 'N' && (c < '0' || c > '9') {
		return 0, false, false
	}
	if s == "-0" {
		return 0, true, false
	}
	n := StringToNumber(s)
	if NumberToString(n) != s {
		return 0, false, false
	}
	return 0, true, false
}

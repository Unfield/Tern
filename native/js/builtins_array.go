package js

import (
	"math"
	"slices"
	"strings"
)

// NewArray makes an array holding elems (which it keeps).
func (rt *Realm) NewArray(elems []Value) *Object {
	if elems == nil {
		elems = []Value{}
	}
	return &Object{proto: rt.ArrayPrototype, array: &elems, Class: "Array"}
}

// IsArray is Array.isArray.
func IsArray(v Value) bool {
	o, ok := v.(*Object)
	return ok && o.array != nil
}

// Elems returns an array's elements, which the caller may read but not keep.
func (o *Object) Elems() []Value {
	if o.array == nil {
		return nil
	}
	if o.holes {
		out := make([]Value, len(*o.array))
		for i, v := range *o.array {
			out[i] = unhole(v)
		}
		return out
	}
	return *o.array
}

// Push appends to an array.
func (o *Object) Push(v ...Value) { *o.array = append(*o.array, v...) }

// elems reads an array or array-like as a slice, for the generic methods.
func (rt *Realm) elems(this Value) []Value {
	o := rt.ToObject(this)
	if o.array != nil {
		if o.holes {
			out := make([]Value, len(*o.array))
			for i, v := range *o.array {
				out[i] = unhole(v)
			}
			return out
		}
		return *o.array
	}
	return ListFromArrayLike(o)
}

// rawElems is elems with holes kept, for the methods that skip missing
// elements.
func (rt *Realm) rawElems(this Value) []Value {
	o := rt.ToObject(this)
	if o.array != nil {
		return *o.array
	}
	return ListFromArrayLike(o)
}

// newHoley makes an array whose elements may include holes.
func (rt *Realm) newHoley(elems []Value) *Object {
	a := rt.NewArray(elems)
	for _, v := range elems {
		if v == hole {
			a.holes = true
			break
		}
	}
	return a
}

// NewArrayWithHoles makes an array literal with elisions, like [1, , 3]:
// Hole marks each missing element.
func (rt *Realm) NewArrayWithHoles(elems []Value) *Object { return rt.newHoley(elems) }

// Hole is the element of an elision in an array literal.
var Hole = hole

func callback(args []Value, name string) *Object {
	f, ok := Arg(args, 0).(*Object)
	if !ok || f.call == nil {
		panic(TypeErrorf("%s: %s is not a function", name, describeForError(Arg(args, 0))))
	}
	return f
}

func relIndex(v Value, length int, def int) int {
	if v == Undefined {
		return def
	}
	n := ToIntegerOrInfinity(v)
	if n < 0 {
		n += float64(length)
		if n < 0 {
			return 0
		}
		return int(n)
	}
	if n > float64(length) {
		return length
	}
	return int(n)
}

func (rt *Realm) setupArray() {
	rt.ArrayCtor = rt.NewConstructor("Array", 1, nil, func(args []Value, nt *Object) Value {
		if len(args) == 1 {
			if n, ok := args[0].(float64); ok {
				l := int(n)
				if float64(l) != n || l < 0 {
					panic(RangeErrorf("Invalid array length"))
				}
				out := make([]Value, l)
				for i := range out {
					out[i] = hole
				}
				a := rt.NewArray(out)
				a.holes = l > 0
				return a
			}
		}
		return rt.NewArray(append([]Value(nil), args...))
	}, rt.ArrayPrototype)
	rt.ArrayCtor.call = func(_ Value, args []Value) Value { return rt.ArrayCtor.construct(args, rt.ArrayCtor) }
	rt.ArrayPrototype.array = &[]Value{}
	rt.method(rt.ArrayCtor, "isArray", 1, func(_ Value, args []Value) Value { return IsArray(Arg(args, 0)) })
	rt.method(rt.ArrayCtor, "of", 0, func(_ Value, args []Value) Value { return rt.NewArray(append([]Value(nil), args...)) })
	rt.method(rt.ArrayCtor, "from", 1, func(_ Value, args []Value) Value {
		src := Arg(args, 0)
		var mapFn *Object
		if f, ok := Arg(args, 1).(*Object); ok && f.call != nil {
			mapFn = f
		} else if Arg(args, 1) != Undefined {
			panic(TypeErrorf("%s is not a function", describeForError(Arg(args, 1))))
		}
		var out []Value
		if IsNullish(src) {
			panic(TypeErrorf("%s is not iterable", ToString(src)))
		}
		if _, iterable := rt.iteratorFor(src); iterable {
			rt.ForOf(src, func(v Value) bool { out = append(out, v); return true })
		} else {
			o := rt.ToObject(src)
			n := int(ToIntegerOrInfinity(o.Get("length")))
			out = make([]Value, n)
			for i := range out {
				out[i] = o.Get(Itoa(i))
			}
		}
		if mapFn != nil {
			for i, v := range out {
				out[i] = mapFn.invoke(Arg(args, 2), v, float64(i))
			}
		}
		return rt.NewArray(out)
	})

	p := rt.ArrayPrototype
	rt.method(p, "at", 1, func(this Value, args []Value) Value {
		a := rt.elems(this)
		i := int(ToIntegerOrInfinity(Arg(args, 0)))
		if i < 0 {
			i += len(a)
		}
		if i < 0 || i >= len(a) {
			return Undefined
		}
		return a[i]
	})
	rt.method(p, "concat", 1, func(this Value, args []Value) Value {
		out := append([]Value(nil), rt.rawElems(this)...)
		for _, a := range args {
			if IsArray(a) {
				out = append(out, *a.(*Object).array...)
			} else {
				out = append(out, a)
			}
		}
		return rt.newHoley(out)
	})
	iterating := func(name string, fn func(f *Object, thisArg Value, a []Value, arr Value) Value) {
		rt.method(p, name, 1, func(this Value, args []Value) Value {
			return fn(callback(args, name), Arg(args, 1), rt.elems(this), this)
		})
	}
	// skipping is iterating for the methods that skip missing elements:
	// their callbacks never see a hole.
	skipping := func(name string, fn func(f *Object, thisArg Value, a []Value, arr Value) Value) {
		rt.method(p, name, 1, func(this Value, args []Value) Value {
			return fn(callback(args, name), Arg(args, 1), rt.rawElems(this), this)
		})
	}
	skipping("every", func(f *Object, t Value, a []Value, arr Value) Value {
		for i := 0; i < len(a); i++ {
			if a[i] == hole {
				continue
			}
			if !Truthy(f.invoke(t, a[i], float64(i), arr)) {
				return false
			}
		}
		return true
	})
	skipping("some", func(f *Object, t Value, a []Value, arr Value) Value {
		for i := 0; i < len(a); i++ {
			if a[i] == hole {
				continue
			}
			if Truthy(f.invoke(t, a[i], float64(i), arr)) {
				return true
			}
		}
		return false
	})
	skipping("filter", func(f *Object, t Value, a []Value, arr Value) Value {
		var out []Value
		for i := 0; i < len(a); i++ {
			v := a[i]
			if v == hole {
				continue
			}
			if Truthy(f.invoke(t, v, float64(i), arr)) {
				out = append(out, v)
			}
		}
		return rt.NewArray(out)
	})
	skipping("map", func(f *Object, t Value, a []Value, arr Value) Value {
		out := make([]Value, len(a))
		for i := 0; i < len(a); i++ {
			if a[i] == hole {
				out[i] = hole // map keeps the holes
				continue
			}
			out[i] = f.invoke(t, a[i], float64(i), arr)
		}
		return rt.newHoley(out)
	})
	skipping("forEach", func(f *Object, t Value, a []Value, arr Value) Value {
		for i := 0; i < len(a); i++ {
			if a[i] != hole {
				f.invoke(t, a[i], float64(i), arr)
			}
		}
		return Undefined
	})
	iterating("find", func(f *Object, t Value, a []Value, arr Value) Value {
		for i := 0; i < len(a); i++ {
			if Truthy(f.invoke(t, a[i], float64(i), arr)) {
				return a[i]
			}
		}
		return Undefined
	})
	iterating("findIndex", func(f *Object, t Value, a []Value, arr Value) Value {
		for i := 0; i < len(a); i++ {
			if Truthy(f.invoke(t, a[i], float64(i), arr)) {
				return float64(i)
			}
		}
		return -1.0
	})
	iterating("findLast", func(f *Object, t Value, a []Value, arr Value) Value {
		for i := len(a) - 1; i >= 0; i-- {
			if Truthy(f.invoke(t, a[i], float64(i), arr)) {
				return a[i]
			}
		}
		return Undefined
	})
	iterating("findLastIndex", func(f *Object, t Value, a []Value, arr Value) Value {
		for i := len(a) - 1; i >= 0; i-- {
			if Truthy(f.invoke(t, a[i], float64(i), arr)) {
				return float64(i)
			}
		}
		return -1.0
	})
	skipping("flatMap", func(f *Object, t Value, a []Value, arr Value) Value {
		var out []Value
		for i := 0; i < len(a); i++ {
			if a[i] == hole {
				continue
			}
			r := f.invoke(t, a[i], float64(i), arr)
			if IsArray(r) {
				out = append(out, rt.elems(r)...)
			} else {
				out = append(out, r)
			}
		}
		return rt.NewArray(out)
	})
	reduce := func(name string, right bool) {
		rt.method(p, name, 1, func(this Value, args []Value) Value {
			f := callback(args, name)
			a := rt.rawElems(this)
			i, end, step := 0, len(a), 1
			if right {
				i, end, step = len(a)-1, -1, -1
			}
			var acc Value
			if len(args) >= 2 {
				acc = args[1]
			} else {
				for i != end && a[i] == hole {
					i += step
				}
				if i == end {
					panic(TypeErrorf("Reduce of empty array with no initial value"))
				}
				acc = a[i]
				i += step
			}
			for ; i != end; i += step {
				if a[i] != hole {
					acc = f.invoke(Undefined, acc, a[i], float64(i), this)
				}
			}
			return acc
		})
	}
	reduce("reduce", false)
	reduce("reduceRight", true)
	rt.method(p, "fill", 1, func(this Value, args []Value) Value {
		o := rt.ToObject(this)
		a := *o.array
		start := relIndex(Arg(args, 1), len(a), 0)
		end := relIndex(Arg(args, 2), len(a), len(a))
		for i := start; i < end; i++ {
			a[i] = Arg(args, 0)
		}
		return o
	})
	var flatten func(out, a []Value, depth float64) []Value
	flatten = func(out, a []Value, depth float64) []Value {
		for _, v := range a {
			if v == hole {
				continue
			}
			if depth >= 1 && IsArray(v) {
				out = flatten(out, *v.(*Object).array, depth-1)
			} else {
				out = append(out, v)
			}
		}
		return out
	}
	rt.method(p, "flat", 0, func(this Value, args []Value) Value {
		depth := 1.0
		if d := Arg(args, 0); d != Undefined {
			depth = ToIntegerOrInfinity(d)
		}
		return rt.NewArray(flatten(nil, rt.rawElems(this), depth))
	})
	rt.method(p, "includes", 1, func(this Value, args []Value) Value {
		a := rt.elems(this)
		for i := relIndex(Arg(args, 1), len(a), 0); i < len(a); i++ {
			if SameValueZero(a[i], Arg(args, 0)) {
				return true
			}
		}
		return false
	})
	rt.method(p, "indexOf", 1, func(this Value, args []Value) Value {
		a := rt.rawElems(this) // a hole matches nothing
		for i := relIndex(Arg(args, 1), len(a), 0); i < len(a); i++ {
			if StrictEquals(a[i], Arg(args, 0)) {
				return float64(i)
			}
		}
		return -1.0
	})
	rt.method(p, "lastIndexOf", 1, func(this Value, args []Value) Value {
		a := rt.rawElems(this) // a hole matches nothing
		from := len(a) - 1
		if len(args) > 1 {
			from = relIndex(args[1], len(a), 0)
			if from >= len(a) {
				from = len(a) - 1
			}
		}
		for i := from; i >= 0; i-- {
			if StrictEquals(a[i], Arg(args, 0)) {
				return float64(i)
			}
		}
		return -1.0
	})
	rt.method(p, "join", 1, func(this Value, args []Value) Value {
		sep := ","
		if s := Arg(args, 0); s != Undefined {
			sep = ToString(s)
		}
		return Join(rt.elems(this), sep)
	})
	rt.method(p, "toString", 0, func(this Value, _ []Value) Value {
		if o, ok := this.(*Object); ok && o.array == nil {
			return rt.ObjectToString(this)
		}
		return Join(rt.elems(this), ",")
	})
	rt.method(p, "toLocaleString", 0, func(this Value, _ []Value) Value { return Join(rt.elems(this), ",") })
	rt.method(p, "push", 1, func(this Value, args []Value) Value {
		o := rt.ToObject(this)
		if o.frozen {
			panic(TypeErrorf("Cannot add property %d, object is not extensible", len(*o.array)))
		}
		*o.array = append(*o.array, args...)
		return float64(len(*o.array))
	})
	rt.method(p, "pop", 0, func(this Value, _ []Value) Value {
		o := rt.ToObject(this)
		a := *o.array
		if len(a) == 0 {
			return Undefined
		}
		v := a[len(a)-1]
		*o.array = a[:len(a)-1]
		return v
	})
	rt.method(p, "shift", 0, func(this Value, _ []Value) Value {
		o := rt.ToObject(this)
		a := *o.array
		if len(a) == 0 {
			return Undefined
		}
		v := a[0]
		*o.array = append([]Value(nil), a[1:]...)
		return v
	})
	rt.method(p, "unshift", 1, func(this Value, args []Value) Value {
		o := rt.ToObject(this)
		*o.array = append(append([]Value(nil), args...), *o.array...)
		return float64(len(*o.array))
	})
	rt.method(p, "slice", 2, func(this Value, args []Value) Value {
		a := rt.rawElems(this)
		start := relIndex(Arg(args, 0), len(a), 0)
		end := relIndex(Arg(args, 1), len(a), len(a))
		if start >= end {
			return rt.NewArray(nil)
		}
		return rt.newHoley(append([]Value(nil), a[start:end]...))
	})
	splice := func(a []Value, args []Value) (kept, removed []Value) {
		start := relIndex(Arg(args, 0), len(a), 0)
		del := len(a) - start
		switch {
		case len(args) == 0:
			del = 0
		case len(args) >= 2:
			del = int(max(0, min(ToIntegerOrInfinity(args[1]), float64(len(a)-start))))
		}
		removed = append([]Value(nil), a[start:start+del]...)
		var items []Value
		if len(args) > 2 {
			items = args[2:]
		}
		kept = append(append(append([]Value(nil), a[:start]...), items...), a[start+del:]...)
		return kept, removed
	}
	rt.method(p, "splice", 2, func(this Value, args []Value) Value {
		o := rt.ToObject(this)
		kept, removed := splice(*o.array, args)
		*o.array = kept
		return rt.NewArray(removed)
	})
	rt.method(p, "toSpliced", 2, func(this Value, args []Value) Value {
		kept, _ := splice(rt.elems(this), args)
		return rt.NewArray(kept)
	})
	rt.method(p, "reverse", 0, func(this Value, _ []Value) Value {
		o := rt.ToObject(this)
		slices.Reverse(*o.array)
		return o
	})
	rt.method(p, "toReversed", 0, func(this Value, _ []Value) Value {
		a := append([]Value(nil), rt.elems(this)...)
		slices.Reverse(a)
		return rt.NewArray(a)
	})
	rt.method(p, "sort", 1, func(this Value, args []Value) Value {
		o := rt.ToObject(this)
		a := *o.array
		if o.holes {
			// Holes go last, after undefined.
			kept := a[:0:0]
			n := 0
			for _, v := range a {
				if v != hole {
					kept = append(kept, v)
				} else {
					n++
				}
			}
			rt.sortValues(kept, Arg(args, 0))
			for range n {
				kept = append(kept, hole)
			}
			copy(a, kept)
			return o
		}
		rt.sortValues(a, Arg(args, 0))
		return o
	})
	rt.method(p, "toSorted", 1, func(this Value, args []Value) Value {
		a := append([]Value(nil), rt.elems(this)...)
		rt.sortValues(a, Arg(args, 0))
		return rt.NewArray(a)
	})
	rt.method(p, "with", 2, func(this Value, args []Value) Value {
		a := append([]Value(nil), rt.elems(this)...)
		i := int(ToIntegerOrInfinity(Arg(args, 0)))
		if i < 0 {
			i += len(a)
		}
		if i < 0 || i >= len(a) {
			panic(RangeErrorf("Invalid index : %d", i))
		}
		a[i] = Arg(args, 1)
		return rt.NewArray(a)
	})
	keys := func(this Value, _ []Value) Value {
		a := rt.elems(this)
		i := 0
		return rt.NewIterator(func() (Value, bool) {
			if i >= len(a) {
				return Undefined, false
			}
			i++
			return float64(i - 1), true
		})
	}
	values := func(this Value, _ []Value) Value {
		o := rt.ToObject(this)
		i := 0
		return rt.NewIterator(func() (Value, bool) {
			a := rt.elems(o)
			if i >= len(a) {
				return Undefined, false
			}
			i++
			return a[i-1], true
		})
	}
	rt.method(p, "keys", 0, keys)
	vf := rt.method(p, "values", 0, values)
	p.DefineHidden(SymbolIterator, vf)
	rt.method(p, "entries", 0, func(this Value, _ []Value) Value {
		a := rt.elems(this)
		i := 0
		return rt.NewIterator(func() (Value, bool) {
			if i >= len(a) {
				return Undefined, false
			}
			i++
			return rt.NewArray([]Value{float64(i - 1), a[i-1]}), true
		})
	})
}

// Join is Array.prototype.join: undefined and null become "".
func Join(a []Value, sep string) string {
	var b strings.Builder
	for i, v := range a {
		if i > 0 {
			b.WriteString(sep)
		}
		if !IsNullish(v) {
			b.WriteString(ToString(v))
		}
	}
	return b.String()
}

// sortValues is the stable sort of Array.prototype.sort: undefined last,
// then by the comparator, or by UTF-16 strings.
func (rt *Realm) sortValues(a []Value, cmp Value) {
	f, hasCmp := cmp.(*Object)
	if cmp != Undefined && (!hasCmp || f.call == nil) {
		panic(TypeErrorf("The comparison function must be either a function or undefined"))
	}
	slices.SortStableFunc(a, func(x, y Value) int {
		switch {
		case x == Undefined && y == Undefined:
			return 0
		case x == Undefined:
			return 1
		case y == Undefined:
			return -1
		}
		if hasCmp {
			n := ToNumber(f.invoke(Undefined, x, y))
			switch {
			case n < 0:
				return -1
			case n > 0:
				return 1
			}
			return 0
		}
		return rt.CompareStrings(ToString(x), ToString(y))
	})
}

// NewIterator makes an iterator object from a next function.
func (rt *Realm) NewIterator(next func() (Value, bool)) *Object {
	if rt.listIteratorPrototype == nil {
		// The built-in iterators' own prototype, as %ArrayIteratorPrototype%
		// is: its prototype is Iterator.prototype.
		rt.listIteratorPrototype = NewObject(rt.IteratorPrototype)
	}
	it := NewObject(rt.listIteratorPrototype)
	rt.method(it, "next", 0, func(Value, []Value) Value {
		v, ok := next()
		r := rt.NewPlainObject()
		r.Define("value", v)
		r.Define("done", !ok)
		return r
	})
	it.Internal = next
	return it
}

func (rt *Realm) setupIteratorPrototype() {
	rt.method(rt.IteratorPrototype, "[Symbol.iterator]", 0, func(this Value, _ []Value) Value { return this })
	f, _ := rt.IteratorPrototype.GetOwn("[Symbol.iterator]")
	rt.iteratorSelf = f
	rt.IteratorPrototype.Delete("[Symbol.iterator]")
	rt.IteratorPrototype.DefineHidden(SymbolIterator, f)
}

// iteratorFor returns a Go iterator for v's for…of, and whether v is iterable.
func (rt *Realm) iteratorFor(v Value) (func() (Value, bool), bool) {
	switch x := v.(type) {
	case string:
		rs := []rune(x)
		i := 0
		return func() (Value, bool) {
			if i >= len(rs) {
				return Undefined, false
			}
			i++
			return string(rs[i-1]), true
		}, true
	case *Object:
		if x.array != nil && x.Get(SymbolIterator) == rt.ArrayValues() {
			i := 0
			return func() (Value, bool) {
				a := *x.array
				if i >= len(a) {
					return Undefined, false
				}
				i++
				return unhole(a[i-1]), true
			}, true
		}
		if next, ok := x.Internal.(func() (Value, bool)); ok && x.proto == rt.IteratorPrototype {
			return next, true
		}
		if g, ok := x.Internal.(*Gen); ok && x.Get("next") == rt.genNext && x.Get(SymbolIterator) == rt.iteratorSelf {
			return genIterator(x, g), true
		}
		m, ok := x.Get(SymbolIterator).(*Object)
		if !ok || m.call == nil {
			return nil, false
		}
		it := m.invoke(x)
		if io, ok := it.(*Object); ok {
			if next, ok := io.Internal.(func() (Value, bool)); ok && io.proto == rt.IteratorPrototype {
				return next, true
			}
		}
		nextFn := rt.Get(it, "next")
		return func() (Value, bool) {
			r := Call(nextFn, it)
			if Truthy(rt.Get(r, "done")) {
				return Undefined, false
			}
			return rt.Get(r, "value"), true
		}, true
	}
	return nil, false
}

// ArrayValues is Array.prototype.values, to tell built-in iteration apart.
func (rt *Realm) ArrayValues() Value {
	if rt.arrayValues == nil {
		rt.arrayValues = rt.ArrayPrototype.Get("values")
	}
	return rt.arrayValues
}

// ForOf runs fn for each value of a for…of over v, until fn returns false.
func (rt *Realm) ForOf(v Value, fn func(Value) bool) {
	next, ok := rt.iteratorFor(v)
	if !ok {
		panic(TypeErrorf("%s is not iterable", describeForError(v)))
	}
	for {
		x, ok := next()
		if !ok || !fn(x) {
			return
		}
	}
}

// Spread collects the values of v, for [...v] and f(...v).
func (rt *Realm) Spread(v Value) []Value {
	if o, ok := v.(*Object); ok && o.array != nil && o.Get(SymbolIterator) == rt.ArrayValues() {
		if o.holes {
			return rt.elems(o)
		}
		return append([]Value(nil), *o.array...)
	}
	var out []Value
	rt.ForOf(v, func(x Value) bool { out = append(out, x); return true })
	return out
}

var _ = math.Inf

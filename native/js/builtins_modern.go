package js

import (
	"strings"
	"unicode/utf16"
)

// The built-ins of ES2022 to ES2025 that QuickJS has: grouping, Set
// methods, iterator helpers, WeakRef and the rest.

func (rt *Realm) setupModern() {
	rt.setupGrouping()
	rt.setupSetMethods()
	rt.setupIteratorHelpers()
	rt.setupWeakRefs()
	rt.setupModernMisc()
}

// groupBy calls cb on each value of items and collects them by its key.
func (rt *Realm) groupBy(items Value, cb *Object, key func(Value) Value, add func(k, v Value)) {
	if IsNullish(items) {
		panic(TypeErrorf("groupBy called on null or undefined"))
	}
	i := 0
	rt.ForOf(items, func(v Value) bool {
		add(key(cb.invoke(Undefined, v, float64(i))), v)
		i++
		return true
	})
}

func (rt *Realm) setupGrouping() {
	rt.method(rt.ObjectCtor, "groupBy", 2, func(_ Value, args []Value) Value {
		cb := callbackAt(args, 1, "Object.groupBy")
		out := NewObject(nil)
		var order []PropKey
		groups := map[PropKey][]Value{}
		rt.groupBy(Arg(args, 0), cb, ToPropertyKey, func(pk, v Value) {
			if _, ok := groups[pk]; !ok {
				order = append(order, pk)
			}
			groups[pk] = append(groups[pk], v)
		})
		for _, k := range order {
			out.Define(k, rt.NewArray(groups[k]))
		}
		return out
	})
	mapCtor := rt.Global.Get("Map").(*Object)
	rt.method(mapCtor, "groupBy", 2, func(_ Value, args []Value) Value {
		cb := callbackAt(args, 1, "Map.groupBy")
		m := Construct(mapCtor).(*Object)
		om := m.Internal.(*ordered)
		rt.groupBy(Arg(args, 0), cb, func(k Value) Value { return k }, func(k, v Value) {
			if g, ok := om.get(k); ok {
				a := g.(*Object)
				*a.array = append(*a.array, v)
				return
			}
			om.set(k, rt.NewArray([]Value{v}))
		})
		return m
	})
}

func callbackAt(args []Value, i int, name string) *Object {
	f, ok := Arg(args, i).(*Object)
	if !ok || f.call == nil {
		panic(TypeErrorf("%s: %s is not a function", name, describeForError(Arg(args, i))))
	}
	return f
}

// setRecord is the other set of a Set method: anything with size, has
// and keys.
type setRecord struct {
	obj       *Object
	size      float64
	has, keys Value
}

func (rt *Realm) getSetRecord(v Value) setRecord {
	o, ok := v.(*Object)
	if !ok {
		panic(TypeErrorf("%s is not an object", describeForError(v)))
	}
	size := ToNumber(o.Get("size"))
	if size != size {
		panic(TypeErrorf("The 'size' property must be a number"))
	}
	r := setRecord{obj: o, size: ToIntegerOrInfinity(size), has: o.Get("has"), keys: o.Get("keys")}
	if f, ok := r.has.(*Object); !ok || f.call == nil {
		panic(TypeErrorf("The 'has' property must be a function"))
	}
	if f, ok := r.keys.(*Object); !ok || f.call == nil {
		panic(TypeErrorf("The 'keys' property must be a function"))
	}
	if r.size < 0 {
		panic(RangeErrorf("The 'size' property must not be negative"))
	}
	return r
}

func (r setRecord) contains(v Value) bool { return Truthy(Call(r.has, r.obj, v)) }

// each runs fn on each of the other set's keys, until it returns false.
func (rt *Realm) eachKey(r setRecord, fn func(Value) bool) {
	it := Call(r.keys, r.obj)
	next := rt.Get(it, "next")
	for {
		res := Call(next, it)
		if _, ok := res.(*Object); !ok {
			panic(TypeErrorf("iterator result is not an object"))
		}
		if Truthy(rt.Get(res, "done")) {
			return
		}
		if !fn(rt.Get(res, "value")) {
			if ret, ok := rt.Get(it, "return").(*Object); ok && ret.call != nil {
				Call(ret, it)
			}
			return
		}
	}
}

func (rt *Realm) setupSetMethods() {
	setCtor := rt.Global.Get("Set").(*Object)
	proto := setCtor.Get("prototype").(*Object)
	this := func(v Value) *ordered {
		if o, ok := v.(*Object); ok && o.Class == "Set" {
			return o.Internal.(*ordered)
		}
		panic(TypeErrorf("Method Set.prototype method called on incompatible receiver"))
	}
	newSet := func() (*Object, *ordered) {
		o := Construct(setCtor).(*Object)
		return o, o.Internal.(*ordered)
	}
	values := func(m *ordered) []Value {
		var out []Value
		next := m.iterate(rt, "keys")
		for {
			v, ok := next()
			if !ok {
				return out
			}
			out = append(out, v)
		}
	}
	rt.method(proto, "union", 1, func(t Value, args []Value) Value {
		m := this(t)
		r := rt.getSetRecord(Arg(args, 0))
		o, out := newSet()
		for _, v := range values(m) {
			out.set(v, v)
		}
		rt.eachKey(r, func(v Value) bool { out.set(v, v); return true })
		return o
	})
	rt.method(proto, "intersection", 1, func(t Value, args []Value) Value {
		m := this(t)
		r := rt.getSetRecord(Arg(args, 0))
		o, out := newSet()
		if float64(m.live) <= r.size {
			for _, v := range values(m) {
				if r.contains(v) {
					out.set(v, v)
				}
			}
		} else {
			rt.eachKey(r, func(v Value) bool {
				if _, ok := m.get(v); ok {
					out.set(v, v)
				}
				return true
			})
		}
		return o
	})
	rt.method(proto, "difference", 1, func(t Value, args []Value) Value {
		m := this(t)
		r := rt.getSetRecord(Arg(args, 0))
		o, out := newSet()
		for _, v := range values(m) {
			out.set(v, v)
		}
		if float64(m.live) <= r.size {
			for _, v := range values(m) {
				if r.contains(v) {
					out.del(v)
				}
			}
		} else {
			rt.eachKey(r, func(v Value) bool { out.del(v); return true })
		}
		return o
	})
	rt.method(proto, "symmetricDifference", 1, func(t Value, args []Value) Value {
		m := this(t)
		r := rt.getSetRecord(Arg(args, 0))
		o, out := newSet()
		for _, v := range values(m) {
			out.set(v, v)
		}
		rt.eachKey(r, func(v Value) bool {
			if _, in := m.get(v); in {
				out.del(v)
			} else {
				out.set(v, v)
			}
			return true
		})
		return o
	})
	rt.method(proto, "isSubsetOf", 1, func(t Value, args []Value) Value {
		m := this(t)
		r := rt.getSetRecord(Arg(args, 0))
		if float64(m.live) > r.size {
			return false
		}
		for _, v := range values(m) {
			if !r.contains(v) {
				return false
			}
		}
		return true
	})
	rt.method(proto, "isSupersetOf", 1, func(t Value, args []Value) Value {
		m := this(t)
		r := rt.getSetRecord(Arg(args, 0))
		if float64(m.live) < r.size {
			return false
		}
		ok := true
		rt.eachKey(r, func(v Value) bool {
			if _, in := m.get(v); !in {
				ok = false
			}
			return ok
		})
		return ok
	})
	rt.method(proto, "isDisjointFrom", 1, func(t Value, args []Value) Value {
		m := this(t)
		r := rt.getSetRecord(Arg(args, 0))
		if float64(m.live) <= r.size {
			for _, v := range values(m) {
				if r.contains(v) {
					return false
				}
			}
			return true
		}
		ok := true
		rt.eachKey(r, func(v Value) bool {
			if _, in := m.get(v); in {
				ok = false
			}
			return ok
		})
		return ok
	})
}

// iterRecord is an iterator and its next method, for iterator helpers.
type iterRecord struct {
	it   Value
	next Value
	done bool
}

func (rt *Realm) iterRecordOf(v Value, name string) *iterRecord {
	if _, ok := v.(*Object); !ok {
		panic(TypeErrorf("%s called on non-object", name))
	}
	return &iterRecord{it: v, next: rt.Get(v, "next")}
}

func (r *iterRecord) step(rt *Realm) (Value, bool) {
	if r.done {
		return Undefined, false
	}
	res := Call(r.next, r.it)
	if _, ok := res.(*Object); !ok {
		panic(TypeErrorf("iterator result is not an object"))
	}
	if Truthy(rt.Get(res, "done")) {
		r.done = true
		return Undefined, false
	}
	return rt.Get(res, "value"), true
}

func (r *iterRecord) close(rt *Realm) {
	if r.done {
		return
	}
	r.done = true
	if ret, ok := rt.Get(r.it, "return").(*Object); ok && ret.call != nil {
		Call(ret, r.it)
	}
}

// closing runs fn, closing the iterator if it throws.
func (r *iterRecord) closing(rt *Realm, fn func()) {
	ok := false
	defer func() {
		if !ok {
			if p := recover(); p != nil {
				func() {
					defer func() { _ = recover() }() // the first error wins
					r.close(rt)
				}()
				panic(p)
			}
		}
	}()
	fn()
	ok = true
}

func (rt *Realm) setupIteratorHelpers() {
	ip := rt.IteratorPrototype
	helperProto := NewObject(ip)
	helperProto.DefineHidden(SymbolToStringTag, "Iterator Helper")
	// helper makes the iterator a helper method returns; next produces its
	// values, and return closes what it reads from.
	helper := func(src *iterRecord, next func() (Value, bool)) *Object {
		o := NewObject(helperProto)
		running, finished := false, false
		rt.method(o, "next", 0, func(Value, []Value) Value {
			if running {
				panic(TypeErrorf("Generator is already running"))
			}
			r := rt.NewPlainObject()
			if finished {
				r.Define("value", Undefined)
				r.Define("done", true)
				return r
			}
			running = true
			var v Value
			var ok bool
			func() {
				defer func() {
					running = false
					if p := recover(); p != nil {
						finished = true
						panic(p)
					}
				}()
				v, ok = next()
			}()
			if !ok {
				finished = true
			}
			r.Define("value", v)
			r.Define("done", !ok)
			return r
		})
		rt.method(o, "return", 0, func(Value, []Value) Value {
			if !finished {
				finished = true
				src.close(rt)
			}
			r := rt.NewPlainObject()
			r.Define("value", Undefined)
			r.Define("done", true)
			return r
		})
		return o
	}
	ctor := rt.NewConstructor("Iterator", 0, func(Value, []Value) Value {
		panic(TypeErrorf("Constructor Iterator requires 'new'"))
	}, func(_ []Value, nt *Object) Value {
		if nt == nil || nt == rt.Global.Get("Iterator") {
			panic(TypeErrorf("Abstract class Iterator not directly constructable"))
		}
		return OrdinaryCreateFromConstructor(nt, ip)
	}, ip)
	rt.Global.DefineHidden("Iterator", ctor)
	ip.DefineHidden("constructor", ctor)
	ip.DefineAccessor(SymbolToStringTag, rt.NewFunction("get [Symbol.toStringTag]", 0, func(Value, []Value) Value { return "Iterator" }), nil, false)

	wrapProto := NewObject(ip)
	rt.method(ctor, "from", 1, func(_ Value, args []Value) Value {
		v := Arg(args, 0)
		if s, ok := v.(string); ok {
			return rt.NewIterator(rt.Iterate(s))
		}
		o, ok := v.(*Object)
		if !ok {
			panic(TypeErrorf("Iterator.from called on non-object"))
		}
		var it Value = o
		if m := o.Get(SymbolIterator); !IsNullish(m) {
			it = Call(m, o)
		}
		if InstanceOfProto(it, ip) {
			return it
		}
		rec := rt.iterRecordOf(it, "Iterator.from")
		w := NewObject(wrapProto)
		rt.method(w, "next", 0, func(Value, []Value) Value { return Call(rec.next, rec.it) })
		rt.method(w, "return", 0, func(Value, []Value) Value {
			if ret, ok := rt.Get(rec.it, "return").(*Object); ok && ret.call != nil {
				return Call(ret, rec.it)
			}
			r := rt.NewPlainObject()
			r.Define("value", Undefined)
			r.Define("done", true)
			return r
		})
		return w
	})

	fnArg := func(r *iterRecord, args []Value, name string) *Object {
		f, ok := Arg(args, 0).(*Object)
		if !ok || f.call == nil {
			r.close(rt)
			panic(TypeErrorf("%s: %s is not a function", name, describeForError(Arg(args, 0))))
		}
		return f
	}
	count := func(r *iterRecord, args []Value, name string) float64 {
		n := ToNumber(Arg(args, 0))
		if n != n {
			r.close(rt)
			panic(RangeErrorf("%s must be a number", name))
		}
		n = ToIntegerOrInfinity(n)
		if n < 0 {
			r.close(rt)
			panic(RangeErrorf("%s must be positive", name))
		}
		return n
	}
	rt.method(ip, "map", 1, func(t Value, args []Value) Value {
		r := rt.iterRecordOf(t, "Iterator.prototype.map")
		f := fnArg(r, args, "Iterator.prototype.map")
		i := 0
		return helper(r, func() (out Value, ok bool) {
			v, more := r.step(rt)
			if !more {
				return Undefined, false
			}
			r.closing(rt, func() { out = f.invoke(Undefined, v, float64(i)) })
			i++
			return out, true
		})
	})
	rt.method(ip, "filter", 1, func(t Value, args []Value) Value {
		r := rt.iterRecordOf(t, "Iterator.prototype.filter")
		f := fnArg(r, args, "Iterator.prototype.filter")
		i := 0
		return helper(r, func() (Value, bool) {
			for {
				v, more := r.step(rt)
				if !more {
					return Undefined, false
				}
				var keep bool
				r.closing(rt, func() { keep = Truthy(f.invoke(Undefined, v, float64(i))) })
				i++
				if keep {
					return v, true
				}
			}
		})
	})
	rt.method(ip, "take", 1, func(t Value, args []Value) Value {
		r := rt.iterRecordOf(t, "Iterator.prototype.take")
		left := count(r, args, "limit")
		return helper(r, func() (Value, bool) {
			if left <= 0 {
				r.close(rt)
				return Undefined, false
			}
			left--
			return r.step(rt)
		})
	})
	rt.method(ip, "drop", 1, func(t Value, args []Value) Value {
		r := rt.iterRecordOf(t, "Iterator.prototype.drop")
		skip := count(r, args, "limit")
		return helper(r, func() (Value, bool) {
			for ; skip > 0; skip-- {
				if _, more := r.step(rt); !more {
					return Undefined, false
				}
			}
			return r.step(rt)
		})
	})
	rt.method(ip, "flatMap", 1, func(t Value, args []Value) Value {
		r := rt.iterRecordOf(t, "Iterator.prototype.flatMap")
		f := fnArg(r, args, "Iterator.prototype.flatMap")
		i := 0
		var inner *iterRecord
		return helper(r, func() (Value, bool) {
			for {
				if inner != nil {
					var v Value
					var more bool
					r.closing(rt, func() { v, more = inner.step(rt) })
					if more {
						return v, true
					}
					inner = nil
				}
				v, more := r.step(rt)
				if !more {
					return Undefined, false
				}
				r.closing(rt, func() {
					mapped := f.invoke(Undefined, v, float64(i))
					o, ok := mapped.(*Object)
					if !ok {
						panic(TypeErrorf("flatMap mapper must return an iterator or iterable"))
					}
					var it Value = o
					if m := o.Get(SymbolIterator); !IsNullish(m) {
						it = Call(m, o)
					}
					inner = rt.iterRecordOf(it, "flatMap")
				})
				i++
			}
		})
	})
	consume := func(name string, fn func(r *iterRecord, f *Object) Value) {
		rt.method(ip, name, 1, func(t Value, args []Value) Value {
			r := rt.iterRecordOf(t, "Iterator.prototype."+name)
			return fn(r, fnArg(r, args, "Iterator.prototype."+name))
		})
	}
	consume("forEach", func(r *iterRecord, f *Object) Value {
		for i := 0; ; i++ {
			v, more := r.step(rt)
			if !more {
				return Undefined
			}
			r.closing(rt, func() { f.invoke(Undefined, v, float64(i)) })
		}
	})
	find := func(name string, want bool, found func(v Value) Value, none Value) {
		consume(name, func(r *iterRecord, f *Object) Value {
			for i := 0; ; i++ {
				v, more := r.step(rt)
				if !more {
					return none
				}
				var hit bool
				r.closing(rt, func() { hit = Truthy(f.invoke(Undefined, v, float64(i))) == want })
				if hit {
					r.close(rt)
					return found(v)
				}
			}
		})
	}
	find("some", true, func(Value) Value { return true }, false)
	find("every", false, func(Value) Value { return false }, true)
	find("find", true, func(v Value) Value { return v }, Undefined)
	rt.method(ip, "reduce", 1, func(t Value, args []Value) Value {
		r := rt.iterRecordOf(t, "Iterator.prototype.reduce")
		f := fnArg(r, args, "Iterator.prototype.reduce")
		var acc Value
		i := 0
		if len(args) > 1 {
			acc = args[1]
		} else {
			v, more := r.step(rt)
			if !more {
				panic(TypeErrorf("Reduce of empty iterator with no initial value"))
			}
			acc, i = v, 1
		}
		for ; ; i++ {
			v, more := r.step(rt)
			if !more {
				return acc
			}
			r.closing(rt, func() { acc = f.invoke(Undefined, acc, v, float64(i)) })
		}
	})
	rt.method(ip, "toArray", 0, func(t Value, _ []Value) Value {
		r := rt.iterRecordOf(t, "Iterator.prototype.toArray")
		var out []Value
		for {
			v, more := r.step(rt)
			if !more {
				return rt.NewArray(out)
			}
			out = append(out, v)
		}
	})
}

// InstanceOfProto reports whether proto is on v's prototype chain.
func InstanceOfProto(v Value, proto *Object) bool {
	o, ok := v.(*Object)
	if !ok {
		return false
	}
	for p := o.proto; p != nil; p = p.proto {
		if p == proto {
			return true
		}
	}
	return false
}

// WeakRef and FinalizationRegistry: a render is short, so a WeakRef holds
// its target for the render and cleanup callbacks never run, which is
// what an engine that doesn't collect in between does too.
func (rt *Realm) setupWeakRefs() {
	canHold := func(v Value) bool {
		switch x := v.(type) {
		case *Object:
			return true
		case *Symbol:
			return !x.registered(rt)
		}
		return false
	}
	wp := NewObject(rt.ObjectPrototype)
	wp.DefineHidden(SymbolToStringTag, "WeakRef")
	wr := rt.NewConstructor("WeakRef", 1, func(Value, []Value) Value {
		panic(TypeErrorf("Constructor WeakRef requires 'new'"))
	}, func(args []Value, nt *Object) Value {
		if !canHold(Arg(args, 0)) {
			panic(TypeErrorf("WeakRef: invalid target"))
		}
		o := OrdinaryCreateFromConstructor(nt, wp)
		o.Class, o.Internal = "WeakRef", Arg(args, 0)
		return o
	}, wp)
	rt.method(wp, "deref", 0, func(t Value, _ []Value) Value {
		if o, ok := t.(*Object); ok && o.Class == "WeakRef" {
			return o.Internal
		}
		panic(TypeErrorf("WeakRef.prototype.deref called on incompatible receiver"))
	})
	rt.Global.DefineHidden("WeakRef", wr)

	fp := NewObject(rt.ObjectPrototype)
	fp.DefineHidden(SymbolToStringTag, "FinalizationRegistry")
	type registry struct{ tokens []Value }
	fr := rt.NewConstructor("FinalizationRegistry", 1, func(Value, []Value) Value {
		panic(TypeErrorf("Constructor FinalizationRegistry requires 'new'"))
	}, func(args []Value, nt *Object) Value {
		callbackAt(args, 0, "FinalizationRegistry")
		o := OrdinaryCreateFromConstructor(nt, fp)
		o.Class, o.Internal = "FinalizationRegistry", &registry{}
		return o
	}, fp)
	this := func(v Value) *registry {
		if o, ok := v.(*Object); ok && o.Class == "FinalizationRegistry" {
			return o.Internal.(*registry)
		}
		panic(TypeErrorf("FinalizationRegistry method called on incompatible receiver"))
	}
	rt.method(fp, "register", 2, func(t Value, args []Value) Value {
		r := this(t)
		target := Arg(args, 0)
		if !canHold(target) {
			panic(TypeErrorf("FinalizationRegistry.prototype.register: invalid target"))
		}
		if SameValue(target, Arg(args, 1)) {
			panic(TypeErrorf("FinalizationRegistry.prototype.register: target and holdings must not be same"))
		}
		if tok := Arg(args, 2); tok != Undefined {
			if !canHold(tok) {
				panic(TypeErrorf("FinalizationRegistry.prototype.register: invalid unregister token"))
			}
			r.tokens = append(r.tokens, tok)
		}
		return Undefined
	})
	rt.method(fp, "unregister", 1, func(t Value, args []Value) Value {
		r := this(t)
		tok := Arg(args, 0)
		if !canHold(tok) {
			panic(TypeErrorf("FinalizationRegistry.prototype.unregister: invalid unregister token"))
		}
		found := false
		kept := r.tokens[:0]
		for _, x := range r.tokens {
			if x == tok {
				found = true
			} else {
				kept = append(kept, x)
			}
		}
		r.tokens = kept
		return found
	})
	rt.Global.DefineHidden("FinalizationRegistry", fr)
}

// registered reports whether s came from Symbol.for.
func (s *Symbol) registered(rt *Realm) bool {
	return rt.SymbolCtor.Get("keyFor") != nil && Call(rt.SymbolCtor.Get("keyFor"), Undefined, s) != Undefined
}

func (rt *Realm) setupModernMisc() {
	c := rt.ObjectCtor
	rt.method(c, "isSealed", 1, func(_ Value, args []Value) Value {
		o, ok := Arg(args, 0).(*Object)
		return !ok || o.IsFrozen()
	})
	if _, ok := c.GetOwn("isExtensible"); !ok {
		rt.method(c, "isExtensible", 1, func(_ Value, args []Value) Value {
			o, ok := Arg(args, 0).(*Object)
			return ok && !o.IsFrozen()
		})
	}

	rt.method(rt.ArrayPrototype, "copyWithin", 2, func(t Value, args []Value) Value {
		o := rt.ToObject(t)
		n := int(max(0, min(ToIntegerOrInfinity(o.Get("length")), 1<<53-1)))
		to := relIndex(Arg(args, 0), n, 0)
		from := relIndex(Arg(args, 1), n, 0)
		end := relIndex(Arg(args, 2), n, n)
		count := min(end-from, n-to)
		if count <= 0 {
			return o
		}
		if o.array != nil && !o.holes && len(*o.array) == n {
			a := *o.array
			copy(a[to:to+count], a[from:from+count])
			return o
		}
		dir := 1
		if from < to && to < from+count {
			dir, from, to = -1, from+count-1, to+count-1
		}
		for ; count > 0; count-- {
			fk, tk := PropKey(ToString(float64(from))), PropKey(ToString(float64(to)))
			if o.Has(fk) {
				o.Set(tk, o.Get(fk))
			} else {
				o.Delete(tk)
			}
			from += dir
			to += dir
		}
		return o
	})

	sp := rt.StringPrototype
	thisStr := func(t Value, name string) string {
		if IsNullish(t) {
			panic(TypeErrorf("String.prototype.%s called on null or undefined", name))
		}
		return ToString(t)
	}
	rt.method(sp, "isWellFormed", 0, func(t Value, _ []Value) Value {
		s := thisStr(t, "isWellFormed")
		if rt.isASCII(s) {
			return true
		}
		u := Units(s)
		for i := 0; i < len(u); i++ {
			switch c := u[i]; {
			case utf16.IsSurrogate(rune(c)) && c < 0xdc00 && i+1 < len(u) && u[i+1] >= 0xdc00 && u[i+1] <= 0xdfff:
				i++
			case utf16.IsSurrogate(rune(c)):
				return false
			}
		}
		return true
	})
	rt.method(sp, "toWellFormed", 0, func(t Value, _ []Value) Value {
		s := thisStr(t, "toWellFormed")
		if rt.isASCII(s) {
			return s
		}
		u := append([]uint16(nil), Units(s)...)
		for i := 0; i < len(u); i++ {
			switch c := u[i]; {
			case utf16.IsSurrogate(rune(c)) && c < 0xdc00 && i+1 < len(u) && u[i+1] >= 0xdc00 && u[i+1] <= 0xdfff:
				i++
			case utf16.IsSurrogate(rune(c)):
				u[i] = 0xfffd
			}
		}
		return FromUnits(u)
	})

	g := rt.Global
	rt.method(g, "escape", 1, func(_ Value, args []Value) Value {
		const keep = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789@*_+-./"
		var b strings.Builder
		for _, c := range Units(ToString(Arg(args, 0))) {
			switch {
			case c < 128 && strings.IndexByte(keep, byte(c)) >= 0:
				b.WriteByte(byte(c))
			case c < 256:
				b.WriteString("%" + hex2(int(c)))
			default:
				b.WriteString("%u" + hex2(int(c>>8)) + hex2(int(c&0xff)))
			}
		}
		return b.String()
	})
	rt.method(g, "unescape", 1, func(_ Value, args []Value) Value {
		u := Units(ToString(Arg(args, 0)))
		out := make([]uint16, 0, len(u))
		for i := 0; i < len(u); i++ {
			c := u[i]
			if c == '%' {
				if i+5 < len(u) && u[i+1] == 'u' {
					if v, ok := hexUnits(u[i+2 : i+6]); ok {
						out = append(out, uint16(v))
						i += 5
						continue
					}
				}
				if i+2 < len(u) {
					if v, ok := hexUnits(u[i+1 : i+3]); ok {
						out = append(out, uint16(v))
						i += 2
						continue
					}
				}
			}
			out = append(out, c)
		}
		return FromUnits(out)
	})

	// BigInt typed arrays: pages that use them render with the engine.
	for _, name := range []string{"BigInt64Array", "BigUint64Array"} {
		f := rt.NewConstructor(name, 3, func(Value, []Value) Value { panic(NotSupported("%s", name)) },
			func([]Value, *Object) Value { panic(NotSupported("%s", name)) }, NewObject(rt.TypedArrayPrototype))
		g.DefineHidden(name, f)
	}
}

func hex2(n int) string {
	const digits = "0123456789ABCDEF"
	return string([]byte{digits[n>>4], digits[n&15]})
}

func hexUnits(u []uint16) (int, bool) {
	v := 0
	for _, c := range u {
		switch {
		case c >= '0' && c <= '9':
			v = v*16 + int(c-'0')
		case c >= 'a' && c <= 'f':
			v = v*16 + int(c-'a'+10)
		case c >= 'A' && c <= 'F':
			v = v*16 + int(c-'A'+10)
		default:
			return 0, false
		}
	}
	return v, true
}

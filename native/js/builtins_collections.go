package js

import "math"

// Map and Set keep insertion order; keys compare with SameValueZero.
type ordered struct {
	keys   []Value
	values []Value
	index  map[any]int
	live   int
}

func mapKey(v Value) any {
	if f, ok := v.(float64); ok {
		if f == 0 {
			return 0.0 // -0 and +0 are one key
		}
		if math.IsNaN(f) {
			return "\x00NaN"
		}
	}
	if s, ok := v.(string); ok {
		return "s" + s
	}
	return v
}

var deleted = &Symbol{"deleted"}

func (m *ordered) get(k Value) (Value, bool) {
	if i, ok := m.index[mapKey(k)]; ok {
		return m.values[i], true
	}
	return Undefined, false
}

func (m *ordered) set(k, v Value) {
	key := mapKey(k)
	if i, ok := m.index[key]; ok {
		m.values[i] = v
		return
	}
	if f, ok := k.(float64); ok && f == 0 {
		k = 0.0
	}
	m.index[key] = len(m.keys)
	m.keys = append(m.keys, k)
	m.values = append(m.values, v)
	m.live++
}

func (m *ordered) del(k Value) bool {
	key := mapKey(k)
	i, ok := m.index[key]
	if !ok {
		return false
	}
	delete(m.index, key)
	m.keys[i] = deleted
	m.values[i] = Undefined
	m.live--
	return true
}

func (m *ordered) clear() {
	for i := range m.keys {
		m.keys[i] = deleted
	}
	m.index = map[any]int{}
	m.live = 0
}

// iterate visits entries added during iteration too, as Map iteration does.
func (m *ordered) iterate(kind string) func() (Value, bool) {
	i := 0
	return func() (Value, bool) {
		for i < len(m.keys) {
			k, v := m.keys[i], m.values[i]
			i++
			if k == deleted {
				continue
			}
			switch kind {
			case "keys":
				return k, true
			case "values":
				return v, true
			}
			return NewArray([]Value{k, v}), true
		}
		return Undefined, false
	}
}

func setupCollections() {
	collection := func(name string, isSet, weak bool) {
		proto := &Object{proto: ObjectPrototype}
		proto.DefineHidden(SymbolToStringTag, name)
		this := func(v Value) *ordered {
			if o, ok := v.(*Object); ok && o.Class == name {
				return o.Internal.(*ordered)
			}
			panic(TypeErrorf("Method %s.prototype called on incompatible receiver", name))
		}
		ctor := NewConstructor(name, 0, nil, func(args []Value, nt *Object) Value {
			o := OrdinaryCreateFromConstructor(nt, proto)
			m := &ordered{index: map[any]int{}}
			o.Class, o.Internal = name, m
			if it := Arg(args, 0); !IsNullish(it) {
				ForOf(it, func(e Value) bool {
					if isSet {
						m.set(e, e)
					} else {
						m.set(Get(e, "0"), Get(e, "1"))
					}
					return true
				})
			}
			return o
		}, proto)
		Global.DefineHidden(name, ctor)
		method(proto, "has", 1, func(t Value, args []Value) Value { _, ok := this(t).get(Arg(args, 0)); return ok })
		method(proto, "delete", 1, func(t Value, args []Value) Value { return this(t).del(Arg(args, 0)) })
		if isSet {
			method(proto, "add", 1, func(t Value, args []Value) Value { this(t).set(Arg(args, 0), Arg(args, 0)); return t })
		} else {
			method(proto, "get", 1, func(t Value, args []Value) Value { v, _ := this(t).get(Arg(args, 0)); return v })
			method(proto, "set", 2, func(t Value, args []Value) Value { this(t).set(Arg(args, 0), Arg(args, 1)); return t })
		}
		if weak {
			return
		}
		method(proto, "clear", 0, func(t Value, _ []Value) Value { this(t).clear(); return Undefined })
		proto.DefineAccessor("size", NewFunction("get size", 0, func(t Value, _ []Value) Value { return float64(this(t).live) }), nil, false)
		method(proto, "forEach", 1, func(t Value, args []Value) Value {
			f := callback(args, "forEach")
			m := this(t)
			next := m.iterate("entries")
			for {
				e, ok := next()
				if !ok {
					return Undefined
				}
				kv := *e.(*Object).array
				f.call(Arg(args, 1), []Value{kv[1], kv[0], t})
			}
		})
		method(proto, "keys", 0, func(t Value, _ []Value) Value { return NewIterator(this(t).iterate("keys")) })
		vals := method(proto, "values", 0, func(t Value, _ []Value) Value { return NewIterator(this(t).iterate("values")) })
		ents := method(proto, "entries", 0, func(t Value, _ []Value) Value {
			if isSet {
				next := this(t).iterate("keys")
				return NewIterator(func() (Value, bool) {
					k, ok := next()
					if !ok {
						return Undefined, false
					}
					return NewArray([]Value{k, k}), true
				})
			}
			return NewIterator(this(t).iterate("entries"))
		})
		if isSet {
			proto.DefineHidden(SymbolIterator, vals)
		} else {
			proto.DefineHidden(SymbolIterator, ents)
		}
	}
	collection("Map", false, false)
	collection("Set", true, false)
	collection("WeakMap", false, true)
	collection("WeakSet", true, true)
}

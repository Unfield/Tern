package js

import (
	"slices"
	"strconv"
)

// PropKey is a property key: a string or a *Symbol.
type PropKey = any

type prop struct {
	key          PropKey
	value        Value
	get, set     *Object // accessors; nil when absent
	accessor     bool
	enumerable   bool
	writable     bool
	configurable bool
}

// Object is a JavaScript object. Functions, arrays and the built-in
// objects with internal slots (Map, Set, RegExp, Date, errors) are objects
// with the matching fields set.
type Object struct {
	props []prop
	index map[string]int // string keys, when there are many props
	proto *Object

	// Functions.
	call      func(this Value, args []Value) Value
	construct func(args []Value, newTarget *Object) Value

	// Arrays keep their elements here, not in props.
	array *[]Value
	// Typed arrays read and write their buffer.
	typed *typedArray
	// The private members of class instances.
	privates []privateSlot

	// Internal slots: Map and Set data, a RegExp, a Date's time, a boxed primitive.
	Internal any
	// Class is the kind of built-in object, such as "Map" or "Error", for
	// Object.prototype.toString and instanceof checks of built-ins.
	Class string

	frozen bool
	// Functions get their name, length and prototype properties when first
	// asked for: most closures never are.
	fnLazy uint8 // lazyNameLength, or lazyPrototype for that and prototype
	fnLen  int32
	fnName string
}

const (
	lazyNameLength = 1
	lazyPrototype  = 2
)

// materialize adds a function's lazy properties.
func (o *Object) materialize() {
	lazy := o.fnLazy
	o.fnLazy = 0
	o.add(prop{key: "length", value: float64(o.fnLen), configurable: true})
	o.add(prop{key: "name", value: o.fnName, configurable: true})
	if lazy == lazyPrototype {
		proto := NewPlainObject()
		proto.DefineHidden("constructor", o)
		o.add(prop{key: "prototype", value: proto, writable: true})
	}
}

const indexAt = 12 // build an index once an object has this many props

// NewObject makes an ordinary object with the given prototype (nil for none).
func NewObject(proto *Object) *Object { return &Object{proto: proto} }

// NewPlainObject makes an object like an object literal does.
func NewPlainObject() *Object { return &Object{proto: ObjectPrototype} }

// Proto is the object's prototype, or nil.
func (o *Object) Proto() *Object { return o.proto }

// SetProto changes the prototype.
func (o *Object) SetProto(p *Object) { o.proto = p }

// globalVersion counts changes to globalThis's own properties, so reads of
// globals can be cached.
var globalVersion uint64 = 1

// touched records a change to o's own properties.
func (o *Object) touched() {
	if o == Global {
		globalVersion++
	}
}

// IsCallable reports whether o is a function.
func (o *Object) IsCallable() bool { return o.call != nil }

func (o *Object) find(key PropKey) int {
	if o.fnLazy != 0 {
		o.materialize()
	}
	if k, ok := key.(string); ok {
		return o.findString(k)
	}
	for i := range o.props { // a symbol: there are few
		if o.props[i].key == key {
			return i
		}
	}
	return -1
}

// findString is find for a string key, the common case: comparing
// strings directly is much cheaper than comparing interfaces.
func (o *Object) findString(k string) int {
	if o.fnLazy != 0 {
		o.materialize()
	}
	if o.index != nil {
		if i, ok := o.index[k]; ok {
			return i
		}
		return -1
	}
	for i := range o.props {
		if s, ok := o.props[i].key.(string); ok && len(s) == len(k) && s == k {
			return i
		}
	}
	return -1
}

func (o *Object) add(p prop) {
	if o.frozen {
		panic(TypeErrorf("Cannot add property %s, object is not extensible", keyName(p.key)))
	}
	if o.fnLazy != 0 {
		o.materialize()
	}
	o.touched()
	if o.props == nil {
		o.props = make([]prop, 0, 4) // most objects have a few properties
	}
	o.props = append(o.props, p)
	if o.index != nil {
		if k, ok := p.key.(string); ok {
			o.index[k] = len(o.props) - 1
		}
	} else if len(o.props) >= indexAt {
		o.reindex()
	}
}

func keyName(k PropKey) string {
	if s, ok := k.(*Symbol); ok {
		return "Symbol(" + s.Description + ")"
	}
	return k.(string)
}

func (o *Object) reindex() {
	o.index = make(map[string]int, len(o.props)*2)
	for i := range o.props {
		if k, ok := o.props[i].key.(string); ok {
			o.index[k] = i
		}
	}
}

// arrayIndex reports whether key is an array index, such as "0" or "12".
func arrayIndex(key PropKey) (int, bool) {
	s, ok := key.(string)
	if !ok || s == "" || len(s) > 10 || (len(s) > 1 && s[0] == '0') {
		return 0, false
	}
	n := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
	}
	return n, n < 1<<32-1
}

// GetOwn returns an own data property's value without running getters.
func (o *Object) GetOwn(key PropKey) (Value, bool) {
	if o.typed != nil {
		if i, num, idx := canonicalIndex(key); num {
			if idx && i < o.typed.length {
				return o.typed.at(i), true
			}
			return Undefined, false
		}
	}
	if o.array != nil {
		if i, ok := arrayIndex(key); ok {
			a := *o.array
			if i < len(a) {
				return a[i], true
			}
			return Undefined, false
		}
		if key == "length" {
			return float64(len(*o.array)), true
		}
	}
	if i := o.find(key); i >= 0 && !o.props[i].accessor {
		return o.props[i].value, true
	}
	return Undefined, false
}

// lookup finds a data property on o or its prototypes, without getters.
func (o *Object) lookup(key PropKey) (Value, bool) {
	for p := o; p != nil; p = p.proto {
		if v, ok := p.GetOwn(key); ok {
			return v, true
		}
	}
	return Undefined, false
}

// Get is [[Get]] with o as the receiver.
func (o *Object) Get(key PropKey) Value { return o.GetFrom(key, o) }

// GetFrom is [[Get]] with another receiver, for getters on prototypes and
// for primitives' methods.
func (o *Object) GetFrom(key PropKey, receiver Value) Value {
	for p := o; p != nil; p = p.proto {
		if p.typed != nil {
			if i, num, idx := canonicalIndex(key); num {
				if idx && i < p.typed.length {
					return p.typed.at(i)
				}
				return Undefined // typed arrays never look up numeric keys on prototypes
			}
		}
		if p.array != nil {
			if i, ok := arrayIndex(key); ok {
				if a := *p.array; i < len(a) {
					return a[i]
				}
				continue
			}
			if key == "length" {
				return Box(float64(len(*p.array)))
			}
		}
		if i := p.find(key); i >= 0 {
			pr := &p.props[i]
			if pr.accessor {
				if pr.get == nil {
					return Undefined
				}
				return pr.get.invoke(receiver)
			}
			return pr.value
		}
	}
	return Undefined
}

// Has is the in operator.
func (o *Object) Has(key PropKey) bool {
	for p := o; p != nil; p = p.proto {
		if p.HasOwn(key) {
			return true
		}
	}
	return false
}

// HasOwn is Object.hasOwn.
func (o *Object) HasOwn(key PropKey) bool {
	if o.typed != nil {
		if i, num, idx := canonicalIndex(key); num {
			return idx && i < o.typed.length
		}
	}
	if o.array != nil {
		if i, ok := arrayIndex(key); ok {
			return i < len(*o.array)
		}
		if key == "length" {
			return true
		}
	}
	return o.find(key) >= 0
}

// Set is [[Set]] in strict mode code: it throws where sloppy code would
// silently fail.
func (o *Object) Set(key PropKey, v Value) { o.SetFrom(key, v, o) }

// SetFrom is [[Set]] with a receiver.
func (o *Object) SetFrom(key PropKey, v Value, receiver Value) {
	if o.typed != nil && receiver == Value(o) {
		if i, num, idx := canonicalIndex(key); num {
			if !idx {
				i = -1
			}
			o.typed.put(i, v)
			return
		}
	}
	if o.array != nil && receiver == Value(o) {
		if i, ok := arrayIndex(key); ok {
			if o.frozen {
				panic(TypeErrorf("Cannot assign to read only property '%d' of object", i))
			}
			a := o.array
			for len(*a) <= i {
				*a = append(*a, Undefined)
			}
			(*a)[i] = v
			return
		}
		if key == "length" {
			setLength(o, v)
			return
		}
	}
	for p := o; p != nil; p = p.proto {
		if i := p.find(key); i >= 0 {
			pr := &p.props[i]
			if pr.accessor {
				if pr.set == nil {
					panic(TypeErrorf("Cannot set property %s of object which has only a getter", keyName(key)))
				}
				pr.set.invoke(receiver, v)
				return
			}
			if !pr.writable || p.frozen {
				panic(TypeErrorf("Cannot assign to read only property '%s' of object", keyName(key)))
			}
			if p == o && receiver == Value(o) {
				pr.value = v
				p.touched()
				return
			}
			break // an inherited writable data property: define an own one
		}
	}
	r, ok := receiver.(*Object)
	if !ok {
		panic(TypeErrorf("Cannot create property '%s' on %s", keyName(key), Typeof(receiver)))
	}
	if i := r.find(key); i >= 0 {
		r.props[i].value = v
		r.touched()
		return
	}
	r.add(prop{key: key, value: v, enumerable: true, writable: true, configurable: true})
}

func setLength(o *Object, v Value) {
	n := ToNumber(v)
	l := int(n)
	if float64(l) != n || l < 0 {
		panic(RangeErrorf("Invalid array length"))
	}
	a := o.array
	switch {
	case l < len(*a):
		*a = (*a)[:l]
	case l > len(*a):
		for len(*a) < l {
			*a = append(*a, Undefined)
		}
	}
}

// Define makes an own, enumerable, writable data property, as an object
// literal does.
func (o *Object) Define(key PropKey, v Value) {
	if o.array != nil {
		if _, ok := arrayIndex(key); ok {
			o.Set(key, v)
			return
		}
	}
	if i := o.find(key); i >= 0 {
		o.props[i] = prop{key: key, value: v, enumerable: true, writable: true, configurable: true}
		o.touched()
		return
	}
	o.add(prop{key: key, value: v, enumerable: true, writable: true, configurable: true})
}

// DefineHidden makes an own, non-enumerable data property, as built-ins and
// class methods have.
func (o *Object) DefineHidden(key PropKey, v Value) {
	if i := o.find(key); i >= 0 {
		o.props[i] = prop{key: key, value: v, writable: true, configurable: true}
		o.touched()
		return
	}
	o.add(prop{key: key, value: v, writable: true, configurable: true})
}

// DefineAccessor makes a getter and/or setter property. Object literals'
// accessors are enumerable; classes' are not.
func (o *Object) DefineAccessor(key PropKey, get, set *Object, enumerable bool) {
	if i := o.find(key); i >= 0 {
		p := &o.props[i]
		if !p.accessor {
			*p = prop{key: key, accessor: true, enumerable: enumerable, configurable: true}
		}
		if get != nil {
			p.get = get
		}
		if set != nil {
			p.set = set
		}
		return
	}
	o.add(prop{key: key, get: get, set: set, accessor: true, enumerable: enumerable, configurable: true})
}

// DefineProperty is Object.defineProperty with a parsed descriptor.
func (o *Object) DefineProperty(key PropKey, desc *Object) {
	get, hasGet := desc.GetOwn("get")
	set, hasSet := desc.GetOwn("set")
	enumerable := Truthy(desc.Get("enumerable"))
	configurable := Truthy(desc.Get("configurable"))
	if hasGet || hasSet || desc.HasOwn("get") || desc.HasOwn("set") {
		g, _ := get.(*Object)
		s, _ := set.(*Object)
		if i := o.find(key); i >= 0 {
			o.props[i] = prop{key: key, get: g, set: s, accessor: true, enumerable: enumerable, configurable: configurable}
			o.touched()
			return
		}
		o.add(prop{key: key, get: g, set: s, accessor: true, enumerable: enumerable, configurable: configurable})
		return
	}
	v := desc.Get("value")
	writable := Truthy(desc.Get("writable"))
	if i := o.find(key); i >= 0 {
		p := &o.props[i]
		if !desc.HasOwn("value") {
			v = p.value
		}
		if !desc.HasOwn("writable") {
			writable = p.writable
		}
		if !desc.HasOwn("enumerable") {
			enumerable = p.enumerable
		}
		*p = prop{key: key, value: v, enumerable: enumerable, writable: writable, configurable: configurable}
		return
	}
	if o.array != nil {
		if _, ok := arrayIndex(key); ok {
			o.Set(key, v)
			return
		}
	}
	o.add(prop{key: key, value: v, enumerable: enumerable, writable: writable, configurable: configurable})
}

// Delete is the delete operator.
func (o *Object) Delete(key PropKey) bool {
	if o.frozen {
		panic(TypeErrorf("Cannot delete property '%s'", keyName(key)))
	}
	if o.typed != nil {
		if i, num, idx := canonicalIndex(key); num {
			if idx && i < o.typed.length {
				panic(TypeErrorf("Cannot delete property '%d' of typed array", i))
			}
			return true
		}
	}
	if o.array != nil {
		if i, ok := arrayIndex(key); ok {
			if a := *o.array; i < len(a) {
				a[i] = Undefined // holes read as undefined
			}
			return true
		}
	}
	i := o.find(key)
	if i < 0 {
		return true
	}
	o.props = slices.Delete(o.props, i, i+1)
	o.touched()
	if o.index != nil {
		o.reindex()
	}
	return true
}

// OwnKeys lists own property keys in JavaScript's order: array indexes
// ascending, then strings in creation order, then symbols.
func (o *Object) OwnKeys(onlyEnumerable, withSymbols bool) []PropKey {
	if o.fnLazy != 0 {
		o.materialize()
	}
	var ints []int
	var strs, syms []PropKey
	if o.array != nil {
		for i := range *o.array {
			ints = append(ints, i)
		}
	}
	if o.typed != nil {
		for i := range o.typed.length {
			ints = append(ints, i)
		}
	}
	for _, p := range o.props {
		if onlyEnumerable && !p.enumerable {
			continue
		}
		switch k := p.key.(type) {
		case *Symbol:
			if withSymbols {
				syms = append(syms, k)
			}
		case string:
			if i, ok := arrayIndex(k); ok {
				ints = append(ints, i)
			} else {
				strs = append(strs, k)
			}
		}
	}
	out := make([]PropKey, 0, len(ints)+len(strs)+len(syms))
	if len(ints) > 0 {
		slices.Sort(ints)
		for _, i := range ints {
			out = append(out, strconv.Itoa(i))
		}
	}
	out = append(out, strs...)
	if o.array != nil && !onlyEnumerable {
		out = append(out, "length")
	}
	return append(out, syms...)
}

// EnumerableOwn calls fn for each enumerable own string-keyed property, in
// order, as Object.keys, for…in over own keys and object spread do.
func (o *Object) EnumerableOwn(fn func(key string, v Value)) {
	for _, k := range o.OwnKeys(true, false) {
		s := k.(string)
		fn(s, o.Get(s))
	}
}

// Freeze is Object.freeze.
func (o *Object) Freeze() {
	if o.fnLazy != 0 {
		o.materialize()
	}
	o.frozen = true
	o.touched()
	for i := range o.props {
		o.props[i].writable = o.props[i].writable && o.props[i].accessor
		o.props[i].configurable = false
	}
}

// IsFrozen is Object.isFrozen.
func (o *Object) IsFrozen() bool { return o.frozen }

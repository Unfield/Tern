package js

import "slices"

// PropKey is a property key: a string or a *Symbol.
type PropKey = any

// prop describes a property: what add takes and propAt returns.
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
	// shape lists the own properties, slots holds their values: accessor
	// properties' slots hold an *accessor. A nil shape is the root shape
	// of the prototype, set when first needed.
	shape *Shape
	slots []Value
	proto *Object
	// childRoot is the root shape of objects whose prototype this is.
	childRoot *Shape
	// realm is a function's realm (and globalThis's), whose argument
	// stack its calls use.
	realm  *Realm
	global bool

	// Functions.
	call      func(this Value, args []Value) Value
	construct func(args []Value, newTarget *Object) Value

	// Arrays keep their elements here, not in properties. holes is set
	// once an element may be missing (see hole).
	array *[]Value
	holes bool
	// Typed arrays read and write their buffer.
	typed *typedArray
	// The private members of class instances.
	privates []privateSlot
	// A proxy's target and handler.
	proxy *proxyData

	// Internal slots: Map and Set data, a RegExp, a Date's time, a boxed primitive.
	Internal any
	// Class is the kind of built-in object, such as "Map" or "Error", for
	// Object.prototype.toString and instanceof checks of built-ins.
	Class string

	frozen bool
	// Functions get their name, length and prototype properties when first
	// asked for: most closures never are.
	fnLazy uint8 // lazyNameLength, or lazyPrototype for that and prototype
	fn     *FnInfo
}

const (
	lazyNameLength = 1
	lazyPrototype  = 2
)

// sh is the object's shape.
func (o *Object) sh() *Shape {
	if o.shape == nil {
		o.shape = rootShape(o.proto)
	}
	if o.fnLazy != 0 {
		o.materialize()
	}
	return o.shape
}

// materialize adds a function's lazy properties.
func (o *Object) materialize() {
	lazy := o.fnLazy
	o.fnLazy = 0
	o.add(prop{key: "length", value: Box(float64(o.fn.Length)), configurable: true})
	o.add(prop{key: "name", value: o.fn.Name, configurable: true})
	if lazy == lazyPrototype {
		proto := o.realm.NewPlainObject()
		proto.DefineHidden("constructor", o)
		o.add(prop{key: "prototype", value: proto, writable: true})
	}
}

// NewObject makes an ordinary object with the given prototype (nil for none).
func NewObject(proto *Object) *Object { return &Object{proto: proto, shape: rootShape(proto)} }

// NewPlainObject makes an object like an object literal does.
func (rt *Realm) NewPlainObject() *Object { return NewObject(rt.ObjectPrototype) }

// Proto is the object's prototype, or nil.
func (o *Object) Proto() *Object { return o.proto }

// SetProto changes the prototype. The shape follows: shapes know their
// prototype.
func (o *Object) SetProto(p *Object) {
	if o.proto == p {
		return
	}
	o.proto = p
	if o.shape == nil {
		return
	}
	old := o.shape
	switch {
	case old.dict:
		old.proto = p
		old.index = nil
	case len(old.keys) == 0:
		o.shape = rootShape(p)
	default:
		s := rootShape(p)
		for i, k := range old.keys {
			s = s.with(k, old.attrs[i])
		}
		o.shape = s
	}
	o.touched()
}

// touched records a change to o's own properties.
func (o *Object) touched() {
	if o.global {
		o.realm.globalVersion++
	}
}

// IsCallable reports whether o is a function.
func (o *Object) IsCallable() bool { return o.call != nil }

func (o *Object) find(key PropKey) int { return o.sh().find(key) }

// findString is find for a string key, the common case.
func (o *Object) findString(k string) int { return o.sh().findString(k) }

// propAt describes the own property in slot i.
func (o *Object) propAt(i int) prop {
	s := o.shape
	a := s.attrs[i]
	p := prop{key: s.keys[i], enumerable: a&attrEnumerable != 0, writable: a&attrWritable != 0, configurable: a&attrConfigurable != 0}
	if a&attrAccessor != 0 {
		acc := o.slots[i].(*accessor)
		p.accessor, p.get, p.set = true, acc.get, acc.set
	} else {
		p.value = o.slots[i]
	}
	return p
}

// setAt replaces the property in slot i, attributes and all.
func (o *Object) setAt(i int, p prop) {
	if a := attrsOf(&p); a != o.shape.attrs[i] {
		o.reshape()
		o.shape.attrs[i] = a
	}
	if p.accessor {
		o.slots[i] = &accessor{get: p.get, set: p.set}
	} else {
		o.slots[i] = p.value
	}
	o.touched()
}

func (o *Object) add(p prop) {
	if o.frozen {
		panic(TypeErrorf("Cannot add property %s, object is not extensible", keyName(p.key)))
	}
	s := o.sh()
	a := attrsOf(&p)
	v := p.value
	if p.accessor {
		v = &accessor{get: p.get, set: p.set}
	}
	switch {
	case s.dict:
		s.keys = append(s.keys, p.key)
		s.attrs = append(s.attrs, a)
		if s.index != nil {
			if k, ok := p.key.(string); ok {
				s.index[k] = len(s.keys) - 1
			}
		}
	case len(s.keys) >= dictAt:
		o.reshape()
		o.shape.keys = append(o.shape.keys, p.key)
		o.shape.attrs = append(o.shape.attrs, a)
	default:
		o.shape = s.with(p.key, a)
	}
	if o.slots == nil {
		o.slots = make([]Value, 0, 4) // most objects have a few properties
	}
	o.slots = append(o.slots, v)
	o.touched()
}

func keyName(k PropKey) string {
	if s, ok := k.(*Symbol); ok {
		return "Symbol(" + s.Description + ")"
	}
	return k.(string)
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
	if o.proxy != nil {
		d, ok := o.proxy.ownDescriptor(key).(*Object)
		if !ok || d.HasOwn("get") || d.HasOwn("set") {
			return Undefined, false
		}
		return d.Get("value"), true
	}
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
			if i < len(a) && a[i] != hole {
				return a[i], true
			}
			return Undefined, false
		}
		if key == "length" {
			return Box(float64(len(*o.array))), true
		}
	}
	if i := o.find(key); i >= 0 && o.shape.attrs[i]&attrAccessor == 0 {
		return o.slots[i], true
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
		if p.proxy != nil {
			return p.proxy.get(key, receiver)
		}
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
				if a := *p.array; i < len(a) && a[i] != hole {
					return a[i]
				}
				continue
			}
			if key == "length" {
				return Box(float64(len(*p.array)))
			}
		}
		if i := p.find(key); i >= 0 {
			if p.shape.attrs[i]&attrAccessor != 0 {
				acc := p.slots[i].(*accessor)
				if acc.get == nil {
					return Undefined
				}
				return acc.get.invoke(receiver)
			}
			return p.slots[i]
		}
	}
	return Undefined
}

// Has is the in operator.
func (o *Object) Has(key PropKey) bool {
	for p := o; p != nil; p = p.proto {
		if p.proxy != nil {
			return p.proxy.has(key)
		}
		if p.HasOwn(key) {
			return true
		}
	}
	return false
}

// HasOwn is Object.hasOwn.
func (o *Object) HasOwn(key PropKey) bool {
	if o.proxy != nil {
		return o.proxy.hasOwn(key)
	}
	if o.typed != nil {
		if i, num, idx := canonicalIndex(key); num {
			return idx && i < o.typed.length
		}
	}
	if o.array != nil {
		if i, ok := arrayIndex(key); ok {
			return i < len(*o.array) && (*o.array)[i] != hole
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
	if o.proxy != nil {
		o.proxy.set(key, v, receiver)
		return
	}
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
			if len(*a) < i {
				o.holes = true
			}
			for len(*a) <= i {
				*a = append(*a, hole)
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
		if p.proxy != nil && p != o {
			p.proxy.set(key, v, receiver)
			return
		}
		if i := p.find(key); i >= 0 {
			a := p.shape.attrs[i]
			if a&attrAccessor != 0 {
				acc := p.slots[i].(*accessor)
				if acc.set == nil {
					panic(TypeErrorf("Cannot set property %s of object which has only a getter", keyName(key)))
				}
				acc.set.invoke(receiver, v)
				return
			}
			if a&attrWritable == 0 || p.frozen {
				panic(TypeErrorf("Cannot assign to read only property '%s' of object", keyName(key)))
			}
			if p == o && receiver == Value(o) {
				p.slots[i] = v
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
		r.slots[i] = v
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
		o.holes = true
		for len(*a) < l {
			*a = append(*a, hole)
		}
	}
}

// Define makes an own, enumerable, writable data property, as an object
// literal does.
func (o *Object) Define(key PropKey, v Value) {
	if o.proxy != nil {
		o.proxy.set(key, v, o)
		return
	}
	if o.array != nil {
		if _, ok := arrayIndex(key); ok {
			o.Set(key, v)
			return
		}
	}
	p := prop{key: key, value: v, enumerable: true, writable: true, configurable: true}
	if i := o.find(key); i >= 0 {
		o.setAt(i, p)
		return
	}
	o.add(p)
}

// DefineHidden makes an own, non-enumerable data property, as built-ins and
// class methods have.
func (o *Object) DefineHidden(key PropKey, v Value) {
	p := prop{key: key, value: v, writable: true, configurable: true}
	if i := o.find(key); i >= 0 {
		o.setAt(i, p)
		return
	}
	o.add(p)
}

// DefineAccessor makes a getter and/or setter property. Object literals'
// accessors are enumerable; classes' are not.
func (o *Object) DefineAccessor(key PropKey, get, set *Object, enumerable bool) {
	if i := o.find(key); i >= 0 {
		p := o.propAt(i)
		if !p.accessor {
			p = prop{key: key, accessor: true, enumerable: enumerable, configurable: true}
		}
		if get != nil {
			p.get = get
		}
		if set != nil {
			p.set = set
		}
		o.setAt(i, p)
		return
	}
	o.add(prop{key: key, get: get, set: set, accessor: true, enumerable: enumerable, configurable: true})
}

// DefineProperty is Object.defineProperty with a parsed descriptor.
func (o *Object) DefineProperty(key PropKey, desc *Object) {
	if o.proxy != nil {
		o.proxy.defineProperty(key, desc)
		return
	}
	get, hasGet := desc.GetOwn("get")
	set, hasSet := desc.GetOwn("set")
	enumerable := Truthy(desc.Get("enumerable"))
	configurable := Truthy(desc.Get("configurable"))
	if hasGet || hasSet || desc.HasOwn("get") || desc.HasOwn("set") {
		g, _ := get.(*Object)
		s, _ := set.(*Object)
		p := prop{key: key, get: g, set: s, accessor: true, enumerable: enumerable, configurable: configurable}
		if i := o.find(key); i >= 0 {
			o.setAt(i, p)
			return
		}
		o.add(p)
		return
	}
	v := desc.Get("value")
	writable := Truthy(desc.Get("writable"))
	if i := o.find(key); i >= 0 {
		old := o.propAt(i)
		if !desc.HasOwn("value") {
			v = old.value
		}
		if !desc.HasOwn("writable") {
			writable = old.writable
		}
		if !desc.HasOwn("enumerable") {
			enumerable = old.enumerable
		}
		o.setAt(i, prop{key: key, value: v, enumerable: enumerable, writable: writable, configurable: configurable})
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
	if o.proxy != nil {
		return o.proxy.deleteProperty(key)
	}
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
				a[i] = hole
				o.holes = true
			}
			return true
		}
	}
	i := o.find(key)
	if i < 0 {
		return true
	}
	o.reshape()
	s := o.shape
	s.keys = slices.Delete(s.keys, i, i+1)
	s.attrs = slices.Delete(s.attrs, i, i+1)
	s.index = nil
	o.slots = slices.Delete(o.slots, i, i+1)
	o.touched()
	return true
}

// OwnKeys lists own property keys in JavaScript's order: array indexes
// ascending, then strings in creation order, then symbols.
func (o *Object) OwnKeys(onlyEnumerable, withSymbols bool) []PropKey {
	if o.proxy != nil {
		return o.proxy.ownKeys(onlyEnumerable, withSymbols)
	}
	s := o.sh()
	n := len(s.keys)
	if o.array != nil {
		n += len(*o.array) + 1
	} else if o.typed != nil {
		n += o.typed.length
	}
	out := make([]PropKey, 0, n)
	elements := 0
	if o.array != nil {
		elements = len(*o.array)
	} else if o.typed != nil {
		elements = o.typed.length
	}
	for i := range elements {
		if o.holes && (*o.array)[i] == hole {
			continue
		}
		out = append(out, indexKey(i))
	}
	// Integer keys among the properties come first, in order; then the
	// rest as they were added.
	var ints []int
	for i, k := range s.keys {
		if onlyEnumerable && s.attrs[i]&attrEnumerable == 0 {
			continue
		}
		if j, ok := arrayIndex(k); ok {
			ints = append(ints, j)
		}
	}
	if len(ints) > 0 {
		slices.Sort(ints)
		for _, j := range ints {
			out = append(out, indexKey(j))
		}
	}
	for i, k := range s.keys {
		if onlyEnumerable && s.attrs[i]&attrEnumerable == 0 {
			continue
		}
		if str, ok := k.(string); ok {
			if _, isInt := arrayIndex(str); !isInt {
				out = append(out, k)
			}
		}
	}
	if o.array != nil && !onlyEnumerable {
		out = append(out, "length")
	}
	if withSymbols {
		for i, k := range s.keys {
			if _, ok := k.(*Symbol); ok && (!onlyEnumerable || s.attrs[i]&attrEnumerable != 0) {
				out = append(out, k)
			}
		}
	}
	return out
}

// indexKey is the key of an array index, as a Value.
func indexKey(i int) PropKey {
	if i < len(intKeys) {
		return intKeys[i]
	}
	return Itoa(i)
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
	s := o.sh()
	o.frozen = true
	o.touched()
	if len(s.keys) == 0 {
		return
	}
	o.reshape()
	for i, a := range o.shape.attrs {
		if a&attrAccessor == 0 {
			a &^= attrWritable
		}
		o.shape.attrs[i] = a &^ attrConfigurable
	}
}

// IsFrozen is Object.isFrozen.
func (o *Object) IsFrozen() bool { return o.frozen }

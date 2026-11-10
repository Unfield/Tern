package js

import (
	"math"
)

// Helpers for compiled code.

// Number constants that have no Go literal.
var (
	Inf     Value = math.Inf(1)
	NegInf  Value = math.Inf(-1)
	NaN     Value = math.NaN()
	NegZero Value = math.Copysign(0, -1)
)

// Control codes: how a try block's closure was left.
const (
	CtlNone = iota
	CtlReturn
	CtlBreak
	CtlContinue
)

// Try runs try…catch…finally. Each part returns how it ended: CtlNone to
// carry on, or a return, break or continue with its value (a break or
// continue carries the target's number).
func (rt *Realm) Try(body func() (int, Value), catch func(Value) (int, Value), finally func() (int, Value)) (ctl int, val Value) {
	run := func() (ctl int, val Value) {
		if catch == nil {
			return body()
		}
		var thrown *Throw
		func() {
			defer func() {
				if r := recover(); r != nil {
					t, ok := r.(*Throw)
					if !ok {
						panic(r) // Go errors and Unsupported aren't JavaScript exceptions
					}
					thrown = t
				}
			}()
			ctl, val = body()
		}()
		if thrown != nil {
			return catch(thrown.In(rt))
		}
		return ctl, val
	}
	if finally == nil {
		return run()
	}
	var panicked any
	func() {
		defer func() { panicked = recover() }()
		ctl, val = run()
	}()
	if isAbort(panicked) {
		panic(panicked) // a dropped generator runs nothing more
	}
	if fc, fv := finally(); fc != CtlNone {
		return fc, fv // the finally block's return, break or continue wins
	}
	if panicked != nil {
		panic(panicked)
	}
	return ctl, val
}

// ThrowValue makes a throw statement's panic.
func ThrowValue(v Value) *Throw { return &Throw{Value: v} }

// Fail is compiled in place of what the compiler can't compile.
func Fail(what string) Value { panic(NotSupported("%s", what)) }

// Iterate returns a for…of's next function.
func (rt *Realm) Iterate(v Value) func() (Value, bool) {
	next, ok := rt.iteratorFor(v)
	if !ok {
		panic(TypeErrorf("%s is not iterable", describeForError(v)))
	}
	return next
}

// Remaining collects an iterator's remaining values, for [...rest].
func (rt *Realm) Remaining(next func() (Value, bool)) *Object {
	var out []Value
	for {
		v, ok := next()
		if !ok {
			return rt.NewArray(out)
		}
		out = append(out, v)
	}
}

// ForInKeys returns a for…in's next function: enumerable string keys of
// the object and its prototypes, each once.
func (rt *Realm) ForInKeys(v Value) func() (Value, bool) {
	if IsNullish(v) {
		return func() (Value, bool) { return Undefined, false }
	}
	o := rt.ToObject(v)
	inherited := false
	for p := o.proto; p != nil; p = p.proto {
		if p.hasEnumerable() {
			inherited = true
			break
		}
	}
	if !inherited {
		// The usual case: only the object's own keys, which need no
		// check for repeats.
		keys := o.OwnKeys(true, false)
		i := 0
		return func() (Value, bool) {
			for i < len(keys) {
				k := keys[i]
				i++
				if o.HasOwn(k) { // skip keys deleted since
					return k, true
				}
			}
			return Undefined, false
		}
	}
	var keys []string
	seen := map[string]bool{}
	for p := o; p != nil; p = p.proto {
		for _, k := range p.OwnKeys(false, false) {
			s := k.(string)
			if seen[s] {
				continue
			}
			seen[s] = true
			if p.array != nil || p.typed != nil {
				if _, ok := arrayIndex(s); ok {
					keys = append(keys, s)
					continue
				}
				if s == "length" && p.array != nil {
					continue
				}
			}
			if i := p.find(s); i >= 0 && p.shape.attrs[i]&attrEnumerable != 0 {
				keys = append(keys, s)
			}
		}
	}
	i := 0
	return func() (Value, bool) {
		for i < len(keys) {
			k := keys[i]
			i++
			if o.Has(k) {
				return k, true
			}
		}
		return Undefined, false
	}
}

// hasEnumerable reports whether an object has an enumerable string-keyed
// property of its own.
func (o *Object) hasEnumerable() bool {
	if o.fnLazy != 0 {
		return false // name, length and prototype aren't enumerable
	}
	if (o.array != nil && len(*o.array) > 0) || (o.typed != nil && o.typed.length > 0) {
		return true
	}
	s := o.sh()
	for i, k := range s.keys {
		if _, ok := k.(string); ok && s.attrs[i]&attrEnumerable != 0 {
			return true
		}
	}
	return false
}

// GlobalRef caches a global's value at one place in compiled code.
type GlobalRef struct {
	version uint64
	value   Value
}

// GlobalGetCached is GlobalGet with a cache, valid until globalThis's own
// properties change. Accessors and inherited properties aren't cached.
func (rt *Realm) GlobalGetCached(c *GlobalRef, name string) Value {
	if c.version == rt.globalVersion {
		return c.value
	}
	if i := rt.Global.findString(name); i >= 0 && rt.Global.shape.attrs[i]&attrAccessor == 0 {
		c.version, c.value = rt.globalVersion, rt.Global.slots[i]
		return c.value
	}
	return rt.GlobalGet(name)
}

// GlobalGet reads an undeclared identifier from globalThis.
func (rt *Realm) GlobalGet(name string) Value {
	if !rt.Global.Has(name) {
		panic(ReferenceErrorf("%s is not defined", name))
	}
	return rt.Global.Get(name)
}

// GlobalSet assigns an undeclared identifier, which strict mode only
// allows when globalThis has it.
func (rt *Realm) GlobalSet(name string, v Value) {
	if !rt.Global.Has(name) {
		panic(ReferenceErrorf("%s is not defined", name))
	}
	rt.Global.Set(name, v)
}

// TypeofGlobal is typeof of an undeclared identifier.
func (rt *Realm) TypeofGlobal(name string) string {
	if !rt.Global.Has(name) {
		return "undefined"
	}
	return Typeof(rt.Global.Get(name))
}

// CallMethodV is obj[key](...args) with a computed key.
func (rt *Realm) CallMethodV(obj Value, key Value, args ...Value) Value {
	return rt.CallMethod(obj, ToPropertyKey(key), args...)
}

// Obj makes an object literal from keys and values.
func (rt *Realm) Obj(kv ...Value) *Object {
	o := &Object{proto: rt.ObjectPrototype, shape: rootShape(rt.ObjectPrototype), slots: make([]Value, 0, len(kv)/2)}
	for i := 0; i+1 < len(kv); i += 2 {
		o.Define(kv[i], kv[i+1])
	}
	return o
}

// CopyDataProperties is object spread: {...src}.
func CopyDataProperties(target *Object, src Value) {
	if IsNullish(src) {
		return
	}
	switch s := src.(type) {
	case string:
		for i, u := range Units(s) {
			target.Define(Itoa(i), FromUnits([]uint16{u}))
		}
		return
	case *Object:
		for _, k := range s.OwnKeys(true, true) {
			target.Define(k, s.Get(k))
		}
	}
}

// CopyDataPropertiesExcluding is an object pattern's rest element.
func (rt *Realm) CopyDataPropertiesExcluding(src Value, excluded []PropKey) *Object {
	out := rt.NewPlainObject()
	s, ok := src.(*Object)
	if !ok {
		return out
	}
	for _, k := range s.OwnKeys(true, true) {
		skip := false
		for _, e := range excluded {
			if e == k {
				skip = true
				break
			}
		}
		if !skip {
			out.Define(k, s.Get(k))
		}
	}
	return out
}

// RequireObjectCoercible throws for destructuring undefined or null.
func RequireObjectCoercible(v Value) {
	if IsNullish(v) {
		panic(TypeErrorf("Cannot destructure '%s' as it is %s.", ToString(v), ToString(v)))
	}
}

// SetProtoFromLiteral is __proto__: v in an object literal.
func SetProtoFromLiteral(o *Object, v Value) {
	switch p := v.(type) {
	case *Object:
		o.SetProto(p)
	case nullType:
		o.SetProto(nil)
	}
}

// RegExpLiteral evaluates a regular expression literal: a new object each
// time, sharing the compiled pattern.
func (rt *Realm) RegExpLiteral(source, flags string) Value {
	key := flags + "/" + source
	if data, ok := rt.regexpCache[key]; ok {
		o := NewObject(rt.RegExpPrototype)
		o.Class, o.Internal = "RegExp", data
		o.add(prop{key: "lastIndex", value: 0.0, writable: true})
		return o
	}
	o := rt.NewRegExp(source, flags)
	if rt.regexpCache == nil {
		rt.regexpCache = map[string]*regexpData{}
	}
	rt.regexpCache[key] = o.Internal.(*regexpData)
	return o
}

// TemplateObject is a tagged template's strings array, the same object
// every time its call site runs.
func (rt *Realm) TemplateObject(site int, cooked []Value, raw []string) Value {
	if t, ok := rt.templates[site]; ok {
		return t
	}
	rv := make([]Value, len(raw))
	for i, r := range raw {
		rv[i] = r
	}
	rawArr := rt.NewArray(rv)
	rawArr.Freeze()
	t := rt.NewArray(cooked)
	t.DefineHidden("raw", rawArr)
	t.Freeze()
	if rt.templates == nil {
		rt.templates = map[int]Value{}
	}
	rt.templates[site] = t
	return t
}

// CheckThis reads this in a derived constructor, which super() sets.
func CheckThis(v Value) Value {
	if v == nil {
		panic(ReferenceErrorf("Must call super constructor in derived class before accessing 'this' or returning from derived constructor"))
	}
	return v
}

// Rest is a rest parameter: the arguments from i on.
func (rt *Realm) Rest(args []Value, i int) *Object {
	if i >= len(args) {
		return rt.NewArray(nil)
	}
	return rt.NewArray(append([]Value(nil), args[i:]...))
}

// NewArguments is a function's arguments object.
func (rt *Realm) NewArguments(args []Value) *Object {
	a := rt.NewArray(append([]Value(nil), args...))
	a.Class = "Arguments"
	return a
}

// NonArrow makes a function declaration or expression constructible, as
// non-arrow functions are.
func (rt *Realm) NonArrow(f *Object) *Object {
	if f.fnLazy == lazyNameLength {
		f.fnLazy = lazyPrototype // made when first asked for
	} else {
		proto := rt.NewPlainObject()
		proto.DefineHidden("constructor", f)
		f.add(prop{key: "prototype", value: proto, writable: true})
	}
	f.construct = func(args []Value, nt *Object) Value {
		this := OrdinaryCreateFromConstructor(nt, rt.ObjectPrototype)
		if r, ok := f.call(this, args).(*Object); ok {
			return r
		}
		return this
	}
	return f
}

// ProtoOf is an object's prototype, for super in object literal methods.
func ProtoOf(v Value) *Object {
	if o, ok := v.(*Object); ok {
		return o.proto
	}
	return nil
}

// AsObject asserts that a compiled function expression is a function object.
func AsObject(v Value) *Object { return v.(*Object) }

// ClassParentProto is the prototype of a class's instances' prototype:
// the superclass's prototype, or null for extends null.
func ClassParentProto(parent Value) *Object {
	if parent == Null {
		return nil
	}
	p, ok := parent.(*Object)
	if !ok || p.construct == nil {
		panic(TypeErrorf("Class extends value %s is not a constructor or null", describeForError(parent)))
	}
	switch pp := p.Get("prototype").(type) {
	case *Object:
		return pp
	case nullType:
		return nil
	}
	panic(TypeErrorf("Class extends value does not have valid prototype property"))
}

// InheritStatics makes a subclass's constructor inherit the parent's statics.
func InheritStatics(ctor *Object, parent Value) {
	if p, ok := parent.(*Object); ok {
		ctor.SetProto(p)
	}
}

// DefineField is a class field: an own, enumerable property.
func DefineField(obj Value, key PropKey, v Value) {
	o, ok := obj.(*Object)
	if !ok {
		return
	}
	p := prop{key: key, value: v, enumerable: true, writable: true, configurable: true}
	if i := o.find(key); i >= 0 {
		o.setAt(i, p)
		return
	}
	o.add(p)
}

// PrivateName is a class's #name; each evaluation of the class makes new ones.
type PrivateName struct{ Description string }

// NewPrivateName makes a private name.
func NewPrivateName(desc string) *PrivateName { return &PrivateName{Description: desc} }

// UnknownPrivate stands for a private name the compiler couldn't find.
var UnknownPrivate = &PrivateName{Description: "#unknown"}

type privateSlot struct {
	name     *PrivateName
	value    Value
	get, set *Object
	accessor bool
}

// private finds an object's slot for name. Objects have few private
// members, so a list beats a map.
func (o *Object) private(name *PrivateName) *privateSlot {
	for i := range o.privates {
		if o.privates[i].name == name {
			return &o.privates[i]
		}
	}
	return nil
}

// PrivateDefine adds a private field or method to an object.
func PrivateDefine(obj Value, name *PrivateName, v Value) {
	o := obj.(*Object)
	if o.private(name) != nil {
		panic(TypeErrorf("Cannot initialize %s twice on the same object", name.Description))
	}
	o.privates = append(o.privates, privateSlot{name: name, value: v})
}

// PrivateAccessor adds a private getter or setter.
func PrivateAccessor(obj Value, name *PrivateName, get, set Value) {
	o := obj.(*Object)
	s := o.private(name)
	if s == nil {
		o.privates = append(o.privates, privateSlot{name: name, accessor: true})
		s = &o.privates[len(o.privates)-1]
	}
	if g, ok := get.(*Object); ok {
		s.get = g
	}
	if st, ok := set.(*Object); ok {
		s.set = st
	}
}

func privateSlotOf(obj Value, name *PrivateName) *privateSlot {
	if o, ok := obj.(*Object); ok {
		if s := o.private(name); s != nil {
			return s
		}
	}
	panic(TypeErrorf("Cannot read private member %s from an object whose class did not declare it", name.Description))
}

// PrivateGet reads obj.#name.
func PrivateGet(obj Value, name *PrivateName) Value {
	s := privateSlotOf(obj, name)
	if s.accessor {
		if s.get == nil {
			panic(TypeErrorf("'%s' was defined without a getter", name.Description))
		}
		return s.get.invoke(obj)
	}
	return s.value
}

// PrivateSet writes obj.#name.
func PrivateSet(obj Value, name *PrivateName, v Value) {
	s := privateSlotOf(obj, name)
	if s.accessor {
		if s.set == nil {
			panic(TypeErrorf("'%s' was defined without a setter", name.Description))
		}
		s.set.invoke(obj, v)
		return
	}
	s.value = v
}

// PrivateIn is #name in obj.
func PrivateIn(name *PrivateName, obj Value) Value {
	o, ok := obj.(*Object)
	if !ok {
		panic(TypeErrorf("Cannot use 'in' operator to search for '%s' in %s", name.Description, ToString(obj)))
	}
	return o.private(name) != nil
}

// SuperGet is super[key] in a method.
func SuperGet(proto *Object, key PropKey, this Value) Value {
	if proto == nil {
		return Undefined
	}
	return proto.GetFrom(key, this)
}

// SuperSet is super[key] = v.
func SuperSet(proto *Object, key PropKey, v Value, this Value) {
	if proto == nil {
		Set(this, key, v)
		return
	}
	proto.SetFrom(key, v, this)
}

// NewNamespace makes a module namespace object.
func NewNamespace() *Object {
	o := NewObject(nil)
	o.DefineHidden(SymbolToStringTag, "Module")
	return o
}

// ExportLive adds a live binding to a namespace.
func (rt *Realm) ExportLive(ns *Object, name string, get func() Value) {
	ns.DefineAccessor(name, rt.NewFunction(name, 0, func(Value, []Value) Value { return get() }), nil, true)
}

// MissingImport stands for a binding imported from a module the IR doesn't
// have; reading it raises Unsupported.
func MissingImport(spec, name string) Value {
	panic(NotSupported("import of %s from %s", name, spec))
}

// MissingExport stands for an import of a name the module doesn't export.
func MissingExport(module, name string) Value {
	panic(NotSupported("%s has no export %s", module, name))
}

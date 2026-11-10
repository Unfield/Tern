package js

// NewFunction makes a function object. length is the number of declared
// parameters, as fn.length reports it.
func NewFunction(name string, length int, fn func(this Value, args []Value) Value) *Object {
	f := &Object{proto: FunctionPrototype, call: fn, Class: "Function"}
	f.add(prop{key: "length", value: float64(length), configurable: true})
	f.add(prop{key: "name", value: name, configurable: true})
	return f
}

// NewConstructor makes a function that new can call, with a prototype
// object whose constructor is the function. construct receives the new
// target, for subclasses.
func NewConstructor(name string, length int, call func(this Value, args []Value) Value, construct func(args []Value, newTarget *Object) Value, proto *Object) *Object {
	if call == nil {
		call = func(Value, []Value) Value {
			panic(TypeErrorf("Class constructor %s cannot be invoked without 'new'", name))
		}
	}
	f := NewFunction(name, length, call)
	f.construct = construct
	if proto == nil {
		proto = NewPlainObject()
	}
	f.add(prop{key: "prototype", value: proto, writable: true})
	proto.DefineHidden("constructor", f)
	return f
}

// SetConstruct makes an existing function a constructor.
func (o *Object) SetConstruct(fn func(args []Value, newTarget *Object) Value) { o.construct = fn }

// IsConstructor reports whether new can call v.
func IsConstructor(v Value) bool {
	o, ok := v.(*Object)
	return ok && o.construct != nil
}

// Arg returns args[i], or undefined.
func Arg(args []Value, i int) Value {
	if i < len(args) {
		return args[i]
	}
	return Undefined
}

// Call calls f with a this value.
func Call(f Value, this Value, args ...Value) Value {
	o, ok := f.(*Object)
	if !ok || o.call == nil {
		panic(TypeErrorf("%s is not a function", describeForError(f)))
	}
	return o.call(this, args)
}

// CallMethod is obj[key](...args), with obj as this.
func CallMethod(obj Value, key PropKey, args ...Value) Value {
	f := Get(obj, key)
	o, ok := f.(*Object)
	if !ok || o.call == nil {
		panic(TypeErrorf("%s.%s is not a function", describeForError(obj), keyName(key)))
	}
	return o.call(obj, args)
}

// Construct is the new operator.
func Construct(f Value, args ...Value) Value {
	o, ok := f.(*Object)
	if !ok || o.construct == nil {
		panic(TypeErrorf("%s is not a constructor", describeForError(f)))
	}
	return o.construct(args, o)
}

// ConstructWith is construct with an explicit new target, for super(...).
func ConstructWith(f Value, newTarget *Object, args ...Value) Value {
	o, ok := f.(*Object)
	if !ok || o.construct == nil {
		panic(TypeErrorf("Super constructor %s is not a constructor", describeForError(f)))
	}
	return o.construct(args, newTarget)
}

// OrdinaryCreateFromConstructor makes the object a constructor builds: its
// prototype is newTarget.prototype, or fallback.
func OrdinaryCreateFromConstructor(newTarget *Object, fallback *Object) *Object {
	if p, ok := newTarget.Get("prototype").(*Object); ok {
		return NewObject(p)
	}
	return NewObject(fallback)
}

func describeForError(v Value) string {
	switch x := v.(type) {
	case undefinedType:
		return "undefined"
	case nullType:
		return "null"
	case string:
		return `"` + x + `"`
	case *Object:
		if x.call != nil {
			if n, ok := x.GetOwn("name"); ok {
				if s, ok := n.(string); ok && s != "" {
					return s
				}
			}
			return "function"
		}
		if x.array != nil {
			return "array"
		}
		return "object"
	case *Symbol:
		return "Symbol(" + x.Description + ")"
	}
	return ToString(v)
}

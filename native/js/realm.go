package js

import "math"

// The intrinsic objects. They're shared by every render: compiled code
// never changes them, as Tern's server bundle never does.
var (
	ObjectPrototype   = &Object{Class: "Object"}
	FunctionPrototype = &Object{proto: ObjectPrototype, Class: "Function"}
	ArrayPrototype    = &Object{proto: ObjectPrototype, Class: "Array"}
	StringPrototype   = &Object{proto: ObjectPrototype, Class: "String"}
	NumberPrototype   = &Object{proto: ObjectPrototype, Class: "Number"}
	BooleanPrototype  = &Object{proto: ObjectPrototype, Class: "Boolean"}
	SymbolPrototype   = &Object{proto: ObjectPrototype, Class: "Symbol"}
	ErrorPrototype    = &Object{proto: ObjectPrototype, Class: "Error"}
	IteratorPrototype = &Object{proto: ObjectPrototype}

	// The constructors of the built-in types.
	ObjectCtor   *Object
	FunctionCtor *Object
	ArrayCtor    *Object
	StringCtor   *Object
	NumberCtor   *Object
	BooleanCtor  *Object
	SymbolCtor   *Object

	// The error constructors.
	Error          *Object
	TypeError      *Object
	RangeError     *Object
	SyntaxError    *Object
	ReferenceError *Object

	// Global is globalThis.
	Global = &Object{proto: ObjectPrototype}
)

// method adds a built-in method: non-enumerable, as in the spec.
func method(o *Object, name string, length int, fn func(this Value, args []Value) Value) *Object {
	f := NewFunction(name, length, fn)
	o.DefineHidden(name, f)
	return f
}

func init() {
	FunctionPrototype.call = func(Value, []Value) Value { return Undefined }
	setupObject()
	setupFunction()
	setupErrors()
	setupArray()
	setupString()
	setupNumber()
	setupBoolean()
	setupSymbol()
	setupMath()
	setupJSON()
	setupCollections()
	setupRegExp()
	setupDate()
	setupTypedArrays()
	setupGenerators()
	setupPromise()
	setupGlobals()
	defaultHasInstance = method(FunctionPrototype, "[Symbol.hasInstance]", 1, func(this Value, args []Value) Value {
		return InstanceOf(Arg(args, 0), this)
	})
	FunctionPrototype.Delete("[Symbol.hasInstance]")
	FunctionPrototype.DefineHidden(symbolHasInstance, defaultHasInstance)
}

func setupBoolean() {
	BooleanCtor = NewConstructor("Boolean", 1, func(_ Value, args []Value) Value { return Truthy(Arg(args, 0)) },
		func(args []Value, nt *Object) Value {
			o := OrdinaryCreateFromConstructor(nt, BooleanPrototype)
			o.Class, o.Internal = "Boolean", Truthy(Arg(args, 0))
			return o
		}, BooleanPrototype)
	thisBool := func(v Value) bool {
		switch x := v.(type) {
		case bool:
			return x
		case *Object:
			if b, ok := x.Internal.(bool); ok && x.Class == "Boolean" {
				return b
			}
		}
		panic(TypeErrorf("Boolean.prototype method called on incompatible receiver"))
	}
	method(BooleanPrototype, "toString", 0, func(this Value, _ []Value) Value { return ToString(thisBool(this)) })
	method(BooleanPrototype, "valueOf", 0, func(this Value, _ []Value) Value { return thisBool(this) })
}

func setupSymbol() {
	SymbolCtor = NewFunction("Symbol", 0, func(_ Value, args []Value) Value {
		d := ""
		if v := Arg(args, 0); v != Undefined {
			d = ToString(v)
		}
		return &Symbol{Description: d}
	})
	SymbolCtor.DefineHidden("prototype", SymbolPrototype)
	SymbolPrototype.DefineHidden("constructor", SymbolCtor)
	for name, s := range map[string]*Symbol{"iterator": SymbolIterator, "toPrimitive": SymbolToPrimitive, "toStringTag": SymbolToStringTag, "hasInstance": symbolHasInstance} {
		SymbolCtor.add(prop{key: name, value: s})
	}
	registry := map[string]*Symbol{}
	method(SymbolCtor, "for", 1, func(_ Value, args []Value) Value {
		k := ToString(Arg(args, 0))
		if s, ok := registry[k]; ok {
			return s
		}
		s := &Symbol{Description: k}
		registry[k] = s
		return s
	})
	thisSym := func(v Value) *Symbol {
		switch x := v.(type) {
		case *Symbol:
			return x
		case *Object:
			if s, ok := x.Internal.(*Symbol); ok {
				return s
			}
		}
		panic(TypeErrorf("Symbol.prototype method called on incompatible receiver"))
	}
	method(SymbolPrototype, "toString", 0, func(this Value, _ []Value) Value { return "Symbol(" + thisSym(this).Description + ")" })
	method(SymbolPrototype, "valueOf", 0, func(this Value, _ []Value) Value { return thisSym(this) })
}

func setupErrors() {
	mk := func(name string, parent *Object, proto *Object) *Object {
		proto.Class = "Error"
		ctor := NewConstructor(name, 1, nil, func(args []Value, nt *Object) Value {
			o := OrdinaryCreateFromConstructor(nt, proto)
			o.Class = "Error"
			if m := Arg(args, 0); m != Undefined {
				o.DefineHidden("message", ToString(m))
			}
			if opts, ok := Arg(args, 1).(*Object); ok && opts.HasOwn("cause") {
				o.DefineHidden("cause", opts.Get("cause"))
			}
			o.DefineHidden("stack", name+": "+ToString(o.Get("message")))
			return o
		}, proto)
		ctor.call = func(_ Value, args []Value) Value { return ctor.construct(args, ctor) }
		if parent != nil {
			ctor.proto = parent
		}
		proto.DefineHidden("name", name)
		proto.DefineHidden("message", "")
		return ctor
	}
	Error = mk("Error", nil, ErrorPrototype)
	method(ErrorPrototype, "toString", 0, func(this Value, _ []Value) Value {
		o, ok := this.(*Object)
		if !ok {
			panic(TypeErrorf("Error.prototype.toString called on non-object"))
		}
		name, msg := "Error", ""
		if n := o.Get("name"); n != Undefined {
			name = ToString(n)
		}
		if m := o.Get("message"); m != Undefined {
			msg = ToString(m)
		}
		switch {
		case name == "":
			return msg
		case msg == "":
			return name
		}
		return name + ": " + msg
	})
	TypeError = mk("TypeError", Error, &Object{proto: ErrorPrototype})
	RangeError = mk("RangeError", Error, &Object{proto: ErrorPrototype})
	SyntaxError = mk("SyntaxError", Error, &Object{proto: ErrorPrototype})
	ReferenceError = mk("ReferenceError", Error, &Object{proto: ErrorPrototype})
	Global.DefineHidden("EvalError", mk("EvalError", Error, &Object{proto: ErrorPrototype}))
	Global.DefineHidden("URIError", mk("URIError", Error, &Object{proto: ErrorPrototype}))
	agg := mk("AggregateError", Error, &Object{proto: ErrorPrototype})
	base := agg.construct
	agg.construct = func(args []Value, nt *Object) Value {
		o := base(args[min(1, len(args)):], nt).(*Object)
		o.DefineHidden("errors", NewArray(Spread(Arg(args, 0))))
		return o
	}
	agg.call = func(_ Value, args []Value) Value { return agg.construct(args, agg) }
	Global.DefineHidden("AggregateError", agg)
}

func setupGlobals() {
	g := Global
	g.DefineHidden("globalThis", g)
	for name, v := range map[string]Value{
		"Object": ObjectCtor, "Function": FunctionCtor, "Array": ArrayCtor, "String": StringCtor, "Number": NumberCtor,
		"Boolean": BooleanCtor, "Symbol": SymbolCtor, "Error": Error, "TypeError": TypeError, "RangeError": RangeError,
		"SyntaxError": SyntaxError, "ReferenceError": ReferenceError,
	} {
		g.DefineHidden(name, v)
	}
	g.add(prop{key: "NaN", value: math.NaN()})
	g.add(prop{key: "Infinity", value: math.Inf(1)})
	g.add(prop{key: "undefined", value: Undefined})
	method(g, "isNaN", 1, func(_ Value, args []Value) Value { return math.IsNaN(ToNumber(Arg(args, 0))) })
	method(g, "isFinite", 1, func(_ Value, args []Value) Value {
		n := ToNumber(Arg(args, 0))
		return !math.IsNaN(n) && !math.IsInf(n, 0)
	})
	method(g, "parseFloat", 1, func(_ Value, args []Value) Value { return parseFloat(ToString(Arg(args, 0))) })
	method(g, "parseInt", 2, func(_ Value, args []Value) Value { return parseInt(ToString(Arg(args, 0)), Arg(args, 1)) })
	setupURIFunctions(g)
}

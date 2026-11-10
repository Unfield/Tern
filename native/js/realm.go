package js

import (
	"math"
	"sync/atomic"
)

// A Realm is a world of JavaScript objects: its own built-ins, globalThis
// and module state. Realms share nothing that changes, so each can run on
// its own goroutine; one realm runs one thing at a time.
type Realm struct {
	// The intrinsic objects.
	ObjectPrototype, FunctionPrototype, ArrayPrototype, StringPrototype *Object
	NumberPrototype, BooleanPrototype, SymbolPrototype, ErrorPrototype  *Object
	IteratorPrototype, DatePrototype, RegExpPrototype, PromisePrototype *Object
	ArrayBufferPrototype, TypedArrayPrototype, DataViewPrototype        *Object
	GeneratorPrototype, GeneratorFunctionPrototype                      *Object
	ObjectCtor, FunctionCtor, ArrayCtor, StringCtor, NumberCtor         *Object
	BooleanCtor, SymbolCtor, promiseCtor                                *Object
	Error, TypeError, RangeError, SyntaxError, ReferenceError           *Object
	arrayValues, iteratorSelf, genNext, defaultHasInstance              Value
	typedKinds                                                          []*typedKind
	stringShape, numberShape, booleanShape                              *Shape
	BigIntPrototype                                                     *Object
	AsyncIteratorPrototype                                              *Object
	AsyncGeneratorPrototype                                             *Object
	bigintShape                                                         *Shape

	// Global is globalThis.
	Global *Object
	// globalVersion counts changes to globalThis's own properties, so
	// reads of globals can be cached.
	globalVersion uint64

	// interrupted stops the realm's code at the next call or loop turn.
	interrupted atomic.Bool
	depth       int     // calls in progress, to stop runaway recursion
	newTarget   *Object // being handed to a function that reads new.target

	argStack    []Value    // see invoke
	lastConcat  []byte     // see Concat
	infoCache   stringInfo // see strInfo
	jobs        []func()   // the Promise job queue
	regexpCache map[string]*regexpData
	templates   map[int]Value
}

// NewRealm makes a realm with the built-ins and an empty globalThis.
func NewRealm() *Realm {
	rt := &Realm{globalVersion: 1, argStack: make([]Value, 0, 1024)}
	rt.ObjectPrototype = &Object{Class: "Object"}
	op := rt.ObjectPrototype
	rt.FunctionPrototype = &Object{proto: op, Class: "Function"}
	rt.FunctionPrototype.realm = rt
	rt.ArrayPrototype = &Object{proto: op, Class: "Array"}
	rt.StringPrototype = &Object{proto: op, Class: "String"}
	rt.NumberPrototype = &Object{proto: op, Class: "Number"}
	rt.BooleanPrototype = &Object{proto: op, Class: "Boolean"}
	rt.SymbolPrototype = &Object{proto: op, Class: "Symbol"}
	rt.ErrorPrototype = &Object{proto: op, Class: "Error"}
	rt.IteratorPrototype = &Object{proto: op}
	rt.DatePrototype = &Object{proto: op}
	rt.RegExpPrototype = &Object{proto: op}
	rt.PromisePrototype = &Object{proto: op}
	rt.ArrayBufferPrototype = &Object{proto: op}
	rt.TypedArrayPrototype = &Object{proto: op}
	rt.DataViewPrototype = &Object{proto: op}
	rt.GeneratorPrototype = &Object{proto: rt.IteratorPrototype}
	rt.GeneratorFunctionPrototype = &Object{proto: rt.FunctionPrototype}
	rt.Global = &Object{proto: op, realm: rt, global: true}
	rt.stringShape = &Shape{proto: rt.StringPrototype}
	rt.numberShape = &Shape{proto: rt.NumberPrototype}
	rt.booleanShape = &Shape{proto: rt.BooleanPrototype}
	for _, k := range typedKindTemplates {
		c := *k
		c.rt = rt
		rt.typedKinds = append(rt.typedKinds, &c)
	}
	rt.setup()
	return rt
}

// method adds a built-in method: non-enumerable, as in the spec.
func (rt *Realm) method(o *Object, name string, length int, fn func(this Value, args []Value) Value) *Object {
	f := rt.NewFunction(name, length, fn)
	o.DefineHidden(name, f)
	return f
}

func (rt *Realm) setup() {
	rt.FunctionPrototype.call = func(Value, []Value) Value { return Undefined }
	rt.setupObject()
	rt.setupFunction()
	rt.setupErrors()
	rt.setupArray()
	rt.setupIteratorPrototype()
	rt.setupString()
	rt.setupNumber()
	rt.setupBoolean()
	rt.setupSymbol()
	rt.setupMath()
	rt.setupJSON()
	rt.setupCollections()
	rt.setupRegExp()
	rt.setupDate()
	rt.setupTypedArrays()
	rt.setupGenerators()
	rt.setupPromise()
	rt.setupProxy()
	rt.setupBigInt()
	rt.setupAsyncGenerators()
	rt.setupGlobals()
	rt.Global.sh()
	rt.Global.reshape() // globals change; reads of them have their own cache
	hi := rt.method(rt.FunctionPrototype, "[Symbol.hasInstance]", 1, func(this Value, args []Value) Value {
		return rt.InstanceOf(Arg(args, 0), this)
	})
	rt.defaultHasInstance = hi
	rt.FunctionPrototype.Delete("[Symbol.hasInstance]")
	rt.FunctionPrototype.DefineHidden(symbolHasInstance, hi)
}

func (rt *Realm) setupBoolean() {
	rt.BooleanCtor = rt.NewConstructor("Boolean", 1, func(_ Value, args []Value) Value { return Truthy(Arg(args, 0)) },
		func(args []Value, nt *Object) Value {
			o := OrdinaryCreateFromConstructor(nt, rt.BooleanPrototype)
			o.Class, o.Internal = "Boolean", Truthy(Arg(args, 0))
			return o
		}, rt.BooleanPrototype)
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
	rt.method(rt.BooleanPrototype, "toString", 0, func(this Value, _ []Value) Value { return ToString(thisBool(this)) })
	rt.method(rt.BooleanPrototype, "valueOf", 0, func(this Value, _ []Value) Value { return thisBool(this) })
}

func (rt *Realm) setupSymbol() {
	rt.SymbolCtor = rt.NewFunction("Symbol", 0, func(_ Value, args []Value) Value {
		d := ""
		if v := Arg(args, 0); v != Undefined {
			d = ToString(v)
		}
		return &Symbol{Description: d}
	})
	rt.SymbolCtor.DefineHidden("prototype", rt.SymbolPrototype)
	rt.SymbolPrototype.DefineHidden("constructor", rt.SymbolCtor)
	for _, w := range []struct {
		name string
		s    *Symbol
	}{{"asyncIterator", SymbolAsyncIterator}, {"hasInstance", symbolHasInstance}, {"iterator", SymbolIterator}, {"toPrimitive", SymbolToPrimitive}, {"toStringTag", SymbolToStringTag}} {
		rt.SymbolCtor.add(prop{key: w.name, value: w.s})
	}
	registry := map[string]*Symbol{}
	rt.method(rt.SymbolCtor, "for", 1, func(_ Value, args []Value) Value {
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
	rt.method(rt.SymbolPrototype, "toString", 0, func(this Value, _ []Value) Value { return "Symbol(" + thisSym(this).Description + ")" })
	rt.method(rt.SymbolPrototype, "valueOf", 0, func(this Value, _ []Value) Value { return thisSym(this) })
}

func (rt *Realm) setupErrors() {
	mk := func(name string, parent *Object, proto *Object) *Object {
		proto.Class = "Error"
		ctor := rt.NewConstructor(name, 1, nil, func(args []Value, nt *Object) Value {
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
			ctor.SetProto(parent)
		}
		proto.DefineHidden("name", name)
		proto.DefineHidden("message", "")
		return ctor
	}
	rt.Error = mk("Error", nil, rt.ErrorPrototype)
	rt.method(rt.ErrorPrototype, "toString", 0, func(this Value, _ []Value) Value {
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
	rt.TypeError = mk("TypeError", rt.Error, &Object{proto: rt.ErrorPrototype})
	rt.RangeError = mk("RangeError", rt.Error, &Object{proto: rt.ErrorPrototype})
	rt.SyntaxError = mk("SyntaxError", rt.Error, &Object{proto: rt.ErrorPrototype})
	rt.ReferenceError = mk("ReferenceError", rt.Error, &Object{proto: rt.ErrorPrototype})
	rt.Global.DefineHidden("EvalError", mk("EvalError", rt.Error, &Object{proto: rt.ErrorPrototype}))
	rt.Global.DefineHidden("URIError", mk("URIError", rt.Error, &Object{proto: rt.ErrorPrototype}))
	agg := mk("AggregateError", rt.Error, &Object{proto: rt.ErrorPrototype})
	base := agg.construct
	agg.construct = func(args []Value, nt *Object) Value {
		o := base(args[min(1, len(args)):], nt).(*Object)
		o.DefineHidden("errors", rt.NewArray(rt.Spread(Arg(args, 0))))
		return o
	}
	agg.call = func(_ Value, args []Value) Value { return agg.construct(args, agg) }
	rt.Global.DefineHidden("AggregateError", agg)
}

func (rt *Realm) setupGlobals() {
	g := rt.Global
	g.DefineHidden("globalThis", g)
	for name, v := range map[string]Value{
		"Object": rt.ObjectCtor, "Function": rt.FunctionCtor, "Array": rt.ArrayCtor, "String": rt.StringCtor, "Number": rt.NumberCtor,
		"Boolean": rt.BooleanCtor, "Symbol": rt.SymbolCtor, "Error": rt.Error, "TypeError": rt.TypeError, "RangeError": rt.RangeError,
		"SyntaxError": rt.SyntaxError, "ReferenceError": rt.ReferenceError,
	} {
		g.DefineHidden(name, v)
	}
	g.add(prop{key: "NaN", value: math.NaN()})
	g.add(prop{key: "Infinity", value: math.Inf(1)})
	g.add(prop{key: "undefined", value: Undefined})
	rt.method(g, "isNaN", 1, func(_ Value, args []Value) Value { return math.IsNaN(ToNumber(Arg(args, 0))) })
	rt.method(g, "isFinite", 1, func(_ Value, args []Value) Value {
		n := ToNumber(Arg(args, 0))
		return !math.IsNaN(n) && !math.IsInf(n, 0)
	})
	rt.method(g, "parseFloat", 1, func(_ Value, args []Value) Value { return parseFloat(ToString(Arg(args, 0))) })
	rt.method(g, "parseInt", 2, func(_ Value, args []Value) Value { return parseInt(ToString(Arg(args, 0)), Arg(args, 1)) })
	rt.setupURIFunctions(g)
}

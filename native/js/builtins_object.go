package js

func toObjectArg(args []Value, name string) *Object {
	v := Arg(args, 0)
	if IsNullish(v) {
		panic(TypeErrorf("%s called on null or undefined", name))
	}
	return ToObject(v)
}

func setupObject() {
	ObjectCtor = NewConstructor("Object", 1, func(_ Value, args []Value) Value {
		v := Arg(args, 0)
		if IsNullish(v) {
			return NewPlainObject()
		}
		return ToObject(v)
	}, func(args []Value, nt *Object) Value {
		v := Arg(args, 0)
		if IsNullish(v) {
			return OrdinaryCreateFromConstructor(nt, ObjectPrototype)
		}
		return ToObject(v)
	}, ObjectPrototype)
	c := ObjectCtor
	method(c, "keys", 1, func(_ Value, args []Value) Value {
		o := toObjectArg(args, "Object.keys")
		return NewArray(o.OwnKeys(true, false))
	})
	method(c, "values", 1, func(_ Value, args []Value) Value {
		o := toObjectArg(args, "Object.values")
		var out []Value
		o.EnumerableOwn(func(_ string, v Value) { out = append(out, v) })
		return NewArray(out)
	})
	method(c, "entries", 1, func(_ Value, args []Value) Value {
		o := toObjectArg(args, "Object.entries")
		var out []Value
		o.EnumerableOwn(func(k string, v Value) { out = append(out, NewArray([]Value{k, v})) })
		return NewArray(out)
	})
	method(c, "assign", 2, func(_ Value, args []Value) Value {
		target := toObjectArg(args, "Object.assign")
		for _, src := range args[1:] {
			if IsNullish(src) {
				continue
			}
			s := ToObject(src)
			for _, k := range s.OwnKeys(true, true) {
				target.Set(k, s.Get(k))
			}
		}
		return target
	})
	method(c, "fromEntries", 1, func(_ Value, args []Value) Value {
		o := NewPlainObject()
		ForOf(Arg(args, 0), func(entry Value) bool {
			o.Define(ToPropertyKey(Get(entry, "0")), Get(entry, "1"))
			return true
		})
		return o
	})
	method(c, "freeze", 1, func(_ Value, args []Value) Value {
		if o, ok := Arg(args, 0).(*Object); ok {
			o.Freeze()
		}
		return Arg(args, 0)
	})
	method(c, "isFrozen", 1, func(_ Value, args []Value) Value {
		o, ok := Arg(args, 0).(*Object)
		return !ok || o.IsFrozen()
	})
	method(c, "seal", 1, func(_ Value, args []Value) Value { return Arg(args, 0) })
	method(c, "preventExtensions", 1, func(_ Value, args []Value) Value { return Arg(args, 0) })
	method(c, "create", 2, func(_ Value, args []Value) Value {
		var proto *Object
		switch p := Arg(args, 0).(type) {
		case *Object:
			proto = p
		case nullType:
		default:
			panic(TypeErrorf("Object prototype may only be an Object or null"))
		}
		o := NewObject(proto)
		if props, ok := Arg(args, 1).(*Object); ok {
			for _, k := range props.OwnKeys(true, true) {
				o.DefineProperty(k, ToObject(props.Get(k)))
			}
		}
		return o
	})
	method(c, "getPrototypeOf", 1, func(_ Value, args []Value) Value {
		o := toObjectArg(args, "Object.getPrototypeOf")
		if o.proto == nil {
			return Null
		}
		return o.proto
	})
	method(c, "setPrototypeOf", 2, func(_ Value, args []Value) Value {
		if o, ok := Arg(args, 0).(*Object); ok {
			switch p := Arg(args, 1).(type) {
			case *Object:
				o.SetProto(p)
			case nullType:
				o.SetProto(nil)
			}
		}
		return Arg(args, 0)
	})
	method(c, "defineProperty", 3, func(_ Value, args []Value) Value {
		o, ok := Arg(args, 0).(*Object)
		if !ok {
			panic(TypeErrorf("Object.defineProperty called on non-object"))
		}
		d, ok := Arg(args, 2).(*Object)
		if !ok {
			panic(TypeErrorf("Property description must be an object"))
		}
		o.DefineProperty(ToPropertyKey(Arg(args, 1)), d)
		return o
	})
	method(c, "defineProperties", 2, func(_ Value, args []Value) Value {
		o, ok := Arg(args, 0).(*Object)
		if !ok {
			panic(TypeErrorf("Object.defineProperties called on non-object"))
		}
		props := ToObject(Arg(args, 1))
		for _, k := range props.OwnKeys(true, true) {
			o.DefineProperty(k, ToObject(props.Get(k)))
		}
		return o
	})
	method(c, "getOwnPropertyNames", 1, func(_ Value, args []Value) Value {
		o := toObjectArg(args, "Object.getOwnPropertyNames")
		return NewArray(keysToValues(o.OwnKeys(false, false)))
	})
	method(c, "getOwnPropertySymbols", 1, func(_ Value, args []Value) Value {
		o := toObjectArg(args, "Object.getOwnPropertySymbols")
		var out []Value
		for _, k := range o.OwnKeys(false, true) {
			if s, ok := k.(*Symbol); ok {
				out = append(out, s)
			}
		}
		return NewArray(out)
	})
	method(c, "getOwnPropertyDescriptor", 2, func(_ Value, args []Value) Value {
		o := toObjectArg(args, "Object.getOwnPropertyDescriptor")
		return o.descriptor(ToPropertyKey(Arg(args, 1)))
	})
	method(c, "getOwnPropertyDescriptors", 1, func(_ Value, args []Value) Value {
		o := toObjectArg(args, "Object.getOwnPropertyDescriptors")
		out := NewPlainObject()
		for _, k := range o.OwnKeys(false, true) {
			out.Define(k, o.descriptor(k))
		}
		return out
	})
	method(c, "is", 2, func(_ Value, args []Value) Value { return SameValue(Arg(args, 0), Arg(args, 1)) })
	method(c, "hasOwn", 2, func(_ Value, args []Value) Value {
		return toObjectArg(args, "Object.hasOwn").HasOwn(ToPropertyKey(Arg(args, 1)))
	})

	p := ObjectPrototype
	method(p, "hasOwnProperty", 1, func(this Value, args []Value) Value {
		return ToObject(this).HasOwn(ToPropertyKey(Arg(args, 0)))
	})
	method(p, "isPrototypeOf", 1, func(this Value, args []Value) Value {
		o, ok := Arg(args, 0).(*Object)
		if !ok {
			return false
		}
		self := ToObject(this)
		for q := o.proto; q != nil; q = q.proto {
			if q == self {
				return true
			}
		}
		return false
	})
	method(p, "propertyIsEnumerable", 1, func(this Value, args []Value) Value {
		o := ToObject(this)
		k := ToPropertyKey(Arg(args, 0))
		if o.array != nil {
			if _, ok := arrayIndex(k); ok {
				return o.HasOwn(k)
			}
		}
		i := o.find(k)
		return i >= 0 && o.shape.attrs[i]&attrEnumerable != 0
	})
	method(p, "toString", 0, func(this Value, _ []Value) Value { return ObjectToString(this) })
	method(p, "toLocaleString", 0, func(this Value, _ []Value) Value { return CallMethod(this, "toString") })
	method(p, "valueOf", 0, func(this Value, _ []Value) Value { return ToObject(this) })
}

// ObjectToString is Object.prototype.toString.
func ObjectToString(v Value) string {
	switch v.(type) {
	case undefinedType:
		return "[object Undefined]"
	case nullType:
		return "[object Null]"
	}
	o := ToObject(v)
	if tag, ok := o.Get(SymbolToStringTag).(string); ok {
		return "[object " + tag + "]"
	}
	switch {
	case o.array != nil:
		return "[object Array]"
	case o.call != nil:
		return "[object Function]"
	}
	switch o.Class {
	case "Error", "Boolean", "Number", "String", "Date", "RegExp", "Arguments":
		return "[object " + o.Class + "]"
	}
	return "[object Object]"
}

func (o *Object) descriptor(k PropKey) Value {
	if o.array != nil {
		if v, ok := o.GetOwn(k); ok {
			d := NewPlainObject()
			d.Define("value", v)
			d.Define("writable", !o.frozen && k != "length" || k == "length")
			d.Define("enumerable", k != "length")
			d.Define("configurable", k != "length" && !o.frozen)
			return d
		}
	}
	i := o.find(k)
	if i < 0 {
		return Undefined
	}
	p := o.propAt(i)
	d := NewPlainObject()
	if p.accessor {
		d.Define("get", orUndefined(p.get))
		d.Define("set", orUndefined(p.set))
	} else {
		d.Define("value", p.value)
		d.Define("writable", p.writable)
	}
	d.Define("enumerable", p.enumerable)
	d.Define("configurable", p.configurable)
	return d
}

func orUndefined(o *Object) Value {
	if o == nil {
		return Undefined
	}
	return o
}

func setupFunction() {
	FunctionCtor = NewConstructor("Function", 1, func(Value, []Value) Value {
		panic(NotSupported("new Function"))
	}, func([]Value, *Object) Value { panic(NotSupported("new Function")) }, FunctionPrototype)
	p := FunctionPrototype
	p.add(prop{key: "length", value: 0.0, configurable: true})
	p.add(prop{key: "name", value: "", configurable: true})
	method(p, "call", 1, func(this Value, args []Value) Value {
		var rest []Value
		if len(args) > 1 {
			rest = args[1:]
		}
		return Call(this, Arg(args, 0), rest...)
	})
	method(p, "apply", 2, func(this Value, args []Value) Value {
		return Call(this, Arg(args, 0), ListFromArrayLike(Arg(args, 1))...)
	})
	method(p, "bind", 1, func(this Value, args []Value) Value {
		target, ok := this.(*Object)
		if !ok || target.call == nil {
			panic(TypeErrorf("Bind must be called on a function"))
		}
		boundThis := Arg(args, 0)
		var bound []Value
		if len(args) > 1 {
			bound = append(bound, args[1:]...)
		}
		name := ""
		if n, ok := target.Get("name").(string); ok {
			name = n
		}
		length := 0.0
		if l, ok := target.Get("length").(float64); ok {
			length = max(0, l-float64(len(bound)))
		}
		f := NewFunction("bound "+name, int(length), func(_ Value, more []Value) Value {
			return target.call(boundThis, append(append([]Value(nil), bound...), more...))
		})
		if target.construct != nil {
			f.construct = func(more []Value, nt *Object) Value {
				if nt == f {
					nt = target
				}
				return target.construct(append(append([]Value(nil), bound...), more...), nt)
			}
		}
		return f
	})
	method(p, "toString", 0, func(Value, []Value) Value { return "function () { [native code] }" })
}

// ListFromArrayLike turns an array or array-like object into arguments.
func ListFromArrayLike(v Value) []Value {
	switch x := v.(type) {
	case undefinedType, nullType:
		return nil
	case *Object:
		if x.array != nil {
			return append([]Value(nil), *x.array...)
		}
		n := int(ToIntegerOrInfinity(x.Get("length")))
		out := make([]Value, n)
		for i := range out {
			out[i] = x.Get(Itoa(i))
		}
		return out
	}
	panic(TypeErrorf("CreateListFromArrayLike called on non-object"))
}

func keysToValues(keys []PropKey) []Value {
	out := make([]Value, len(keys))
	copy(out, keys)
	return out
}

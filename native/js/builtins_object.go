package js

func (rt *Realm) toObjectArg(args []Value, name string) *Object {
	v := Arg(args, 0)
	if IsNullish(v) {
		panic(TypeErrorf("%s called on null or undefined", name))
	}
	return rt.ToObject(v)
}

func (rt *Realm) setupObject() {
	rt.ObjectCtor = rt.NewConstructor("Object", 1, func(_ Value, args []Value) Value {
		v := Arg(args, 0)
		if IsNullish(v) {
			return rt.NewPlainObject()
		}
		return rt.ToObject(v)
	}, func(args []Value, nt *Object) Value {
		v := Arg(args, 0)
		if IsNullish(v) {
			return OrdinaryCreateFromConstructor(nt, rt.ObjectPrototype)
		}
		return rt.ToObject(v)
	}, rt.ObjectPrototype)
	c := rt.ObjectCtor
	rt.method(c, "keys", 1, func(_ Value, args []Value) Value {
		o := rt.toObjectArg(args, "Object.keys")
		return rt.NewArray(o.OwnKeys(true, false))
	})
	rt.method(c, "values", 1, func(_ Value, args []Value) Value {
		o := rt.toObjectArg(args, "Object.values")
		var out []Value
		o.EnumerableOwn(func(_ string, v Value) { out = append(out, v) })
		return rt.NewArray(out)
	})
	rt.method(c, "entries", 1, func(_ Value, args []Value) Value {
		o := rt.toObjectArg(args, "Object.entries")
		var out []Value
		o.EnumerableOwn(func(k string, v Value) { out = append(out, rt.NewArray([]Value{k, v})) })
		return rt.NewArray(out)
	})
	rt.method(c, "assign", 2, func(_ Value, args []Value) Value {
		target := rt.toObjectArg(args, "Object.assign")
		for _, src := range args[1:] {
			if IsNullish(src) {
				continue
			}
			s := rt.ToObject(src)
			for _, k := range s.OwnKeys(true, true) {
				target.Set(k, s.Get(k))
			}
		}
		return target
	})
	rt.method(c, "fromEntries", 1, func(_ Value, args []Value) Value {
		o := rt.NewPlainObject()
		rt.ForOf(Arg(args, 0), func(entry Value) bool {
			o.Define(ToPropertyKey(rt.Get(entry, "0")), rt.Get(entry, "1"))
			return true
		})
		return o
	})
	rt.method(c, "freeze", 1, func(_ Value, args []Value) Value {
		if o, ok := Arg(args, 0).(*Object); ok {
			o.Freeze()
		}
		return Arg(args, 0)
	})
	rt.method(c, "isFrozen", 1, func(_ Value, args []Value) Value {
		o, ok := Arg(args, 0).(*Object)
		return !ok || o.IsFrozen()
	})
	rt.method(c, "seal", 1, func(_ Value, args []Value) Value { return Arg(args, 0) })
	rt.method(c, "preventExtensions", 1, func(_ Value, args []Value) Value { return Arg(args, 0) })
	rt.method(c, "create", 2, func(_ Value, args []Value) Value {
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
				o.DefineProperty(k, rt.ToObject(props.Get(k)))
			}
		}
		return o
	})
	rt.method(c, "getPrototypeOf", 1, func(_ Value, args []Value) Value {
		o := rt.toObjectArg(args, "Object.getPrototypeOf")
		if o.proto == nil {
			return Null
		}
		return o.proto
	})
	rt.method(c, "setPrototypeOf", 2, func(_ Value, args []Value) Value {
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
	rt.method(c, "defineProperty", 3, func(_ Value, args []Value) Value {
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
	rt.method(c, "defineProperties", 2, func(_ Value, args []Value) Value {
		o, ok := Arg(args, 0).(*Object)
		if !ok {
			panic(TypeErrorf("Object.defineProperties called on non-object"))
		}
		props := rt.ToObject(Arg(args, 1))
		for _, k := range props.OwnKeys(true, true) {
			o.DefineProperty(k, rt.ToObject(props.Get(k)))
		}
		return o
	})
	rt.method(c, "getOwnPropertyNames", 1, func(_ Value, args []Value) Value {
		o := rt.toObjectArg(args, "Object.getOwnPropertyNames")
		return rt.NewArray(keysToValues(o.OwnKeys(false, false)))
	})
	rt.method(c, "getOwnPropertySymbols", 1, func(_ Value, args []Value) Value {
		o := rt.toObjectArg(args, "Object.getOwnPropertySymbols")
		var out []Value
		for _, k := range o.OwnKeys(false, true) {
			if s, ok := k.(*Symbol); ok {
				out = append(out, s)
			}
		}
		return rt.NewArray(out)
	})
	rt.method(c, "getOwnPropertyDescriptor", 2, func(_ Value, args []Value) Value {
		o := rt.toObjectArg(args, "Object.getOwnPropertyDescriptor")
		return o.descriptor(rt, ToPropertyKey(Arg(args, 1)))
	})
	rt.method(c, "getOwnPropertyDescriptors", 1, func(_ Value, args []Value) Value {
		o := rt.toObjectArg(args, "Object.getOwnPropertyDescriptors")
		out := rt.NewPlainObject()
		for _, k := range o.OwnKeys(false, true) {
			out.Define(k, o.descriptor(rt, k))
		}
		return out
	})
	rt.method(c, "is", 2, func(_ Value, args []Value) Value { return SameValue(Arg(args, 0), Arg(args, 1)) })
	rt.method(c, "hasOwn", 2, func(_ Value, args []Value) Value {
		return rt.toObjectArg(args, "Object.hasOwn").HasOwn(ToPropertyKey(Arg(args, 1)))
	})

	p := rt.ObjectPrototype
	rt.method(p, "hasOwnProperty", 1, func(this Value, args []Value) Value {
		return rt.ToObject(this).HasOwn(ToPropertyKey(Arg(args, 0)))
	})
	rt.method(p, "isPrototypeOf", 1, func(this Value, args []Value) Value {
		o, ok := Arg(args, 0).(*Object)
		if !ok {
			return false
		}
		self := rt.ToObject(this)
		for q := o.proto; q != nil; q = q.proto {
			if q == self {
				return true
			}
		}
		return false
	})
	rt.method(p, "propertyIsEnumerable", 1, func(this Value, args []Value) Value {
		o := rt.ToObject(this)
		k := ToPropertyKey(Arg(args, 0))
		if o.array != nil {
			if _, ok := arrayIndex(k); ok {
				return o.HasOwn(k)
			}
		}
		i := o.find(k)
		return i >= 0 && o.shape.attrs[i]&attrEnumerable != 0
	})
	rt.method(p, "toString", 0, func(this Value, _ []Value) Value { return rt.ObjectToString(this) })
	rt.method(p, "toLocaleString", 0, func(this Value, _ []Value) Value { return rt.CallMethod(this, "toString") })
	rt.method(p, "valueOf", 0, func(this Value, _ []Value) Value { return rt.ToObject(this) })
}

// ObjectToString is Object.prototype.toString.
func (rt *Realm) ObjectToString(v Value) string {
	switch v.(type) {
	case undefinedType:
		return "[object Undefined]"
	case nullType:
		return "[object Null]"
	}
	o := rt.ToObject(v)
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

func (o *Object) descriptor(rt *Realm, k PropKey) Value {
	if o.array != nil {
		if v, ok := o.GetOwn(k); ok {
			d := rt.NewPlainObject()
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
	d := rt.NewPlainObject()
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

func (rt *Realm) setupFunction() {
	rt.FunctionCtor = rt.NewConstructor("Function", 1, func(Value, []Value) Value {
		panic(NotSupported("new Function"))
	}, func([]Value, *Object) Value { panic(NotSupported("new Function")) }, rt.FunctionPrototype)
	p := rt.FunctionPrototype
	p.add(prop{key: "length", value: 0.0, configurable: true})
	p.add(prop{key: "name", value: "", configurable: true})
	rt.method(p, "call", 1, func(this Value, args []Value) Value {
		var rest []Value
		if len(args) > 1 {
			rest = args[1:]
		}
		return Call(this, Arg(args, 0), rest...)
	})
	rt.method(p, "apply", 2, func(this Value, args []Value) Value {
		return Call(this, Arg(args, 0), ListFromArrayLike(Arg(args, 1))...)
	})
	rt.method(p, "bind", 1, func(this Value, args []Value) Value {
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
		f := rt.NewFunction("bound "+name, int(length), func(_ Value, more []Value) Value {
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
	rt.method(p, "toString", 0, func(this Value, _ []Value) Value {
		f, ok := this.(*Object)
		if !ok || f.call == nil {
			panic(TypeErrorf("Function.prototype.toString requires that 'this' be a Function"))
		}
		if f.fn != nil {
			if src := f.fn.Source(); src != "" {
				return src // a compiled function: its source text
			}
		}
		name := ""
		if n, ok := f.Get("name").(string); ok {
			name = n
		}
		return "function " + name + "() {\n    [native code]\n}"
	})
}

// ListFromArrayLike turns an array or array-like object into arguments.
func ListFromArrayLike(v Value) []Value {
	switch x := v.(type) {
	case undefinedType, nullType:
		return nil
	case *Object:
		if x.array != nil {
			out := append([]Value(nil), *x.array...)
			if x.holes {
				for i, v := range out {
					out[i] = unhole(v)
				}
			}
			return out
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

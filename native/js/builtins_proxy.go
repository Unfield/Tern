package js

// Proxy and Reflect. A proxy is an object whose operations go to its
// handler's traps, or to its target where the handler has none. Proxies
// have a dictionary shape of their own, so inline caches never cache
// them and every access reaches the traps.

type proxyData struct {
	rt      *Realm
	target  *Object
	handler *Object // nil once revoked
}

func (p *proxyData) live() {
	if p.handler == nil {
		panic(TypeErrorf("Cannot perform an operation on a revoked proxy"))
	}
}

// trap returns the handler's trap, or nil to use the target.
func (p *proxyData) trap(name string) *Object {
	p.live()
	t := p.handler.Get(name)
	if IsNullish(t) {
		return nil
	}
	f, ok := t.(*Object)
	if !ok || f.call == nil {
		panic(TypeErrorf("proxy trap %s is not a function", name))
	}
	return f
}

func (p *proxyData) get(key PropKey, receiver Value) Value {
	if t := p.trap("get"); t != nil {
		return t.invoke(p.handler, p.target, key, receiver)
	}
	return p.target.GetFrom(key, receiver)
}

func (p *proxyData) set(key PropKey, v, receiver Value) {
	if t := p.trap("set"); t != nil {
		if !Truthy(t.invoke(p.handler, p.target, key, v, receiver)) {
			panic(TypeErrorf("'set' on proxy: trap returned falsish for property '%s'", keyName(key)))
		}
		return
	}
	if receiver == Value(p.target) {
		p.target.Set(key, v)
		return
	}
	p.target.SetFrom(key, v, receiver)
}

func (p *proxyData) has(key PropKey) bool {
	if t := p.trap("has"); t != nil {
		return Truthy(t.invoke(p.handler, p.target, key))
	}
	return p.target.Has(key)
}

func (p *proxyData) deleteProperty(key PropKey) bool {
	if t := p.trap("deleteProperty"); t != nil {
		return Truthy(t.invoke(p.handler, p.target, key))
	}
	return p.target.Delete(key)
}

// ownDescriptor is the trap's descriptor object, or the target's, or
// undefined.
func (p *proxyData) ownDescriptor(key PropKey) Value {
	if t := p.trap("getOwnPropertyDescriptor"); t != nil {
		return t.invoke(p.handler, p.target, key)
	}
	return p.target.descriptor(p.rt, key)
}

func (p *proxyData) ownKeys(onlyEnumerable, withSymbols bool) []PropKey {
	t := p.trap("ownKeys")
	if t == nil {
		return p.target.OwnKeys(onlyEnumerable, withSymbols)
	}
	var out []PropKey
	for _, k := range ListFromArrayLike(t.invoke(p.handler, p.target)) {
		switch k.(type) {
		case string:
		case *Symbol:
			if !withSymbols {
				continue
			}
		default:
			panic(TypeErrorf("proxy ownKeys trap result must contain only strings and symbols"))
		}
		if onlyEnumerable {
			d, ok := p.ownDescriptor(k).(*Object)
			if !ok || !Truthy(d.Get("enumerable")) {
				continue
			}
		}
		out = append(out, k)
	}
	return out
}

func (p *proxyData) defineProperty(key PropKey, desc *Object) {
	if t := p.trap("defineProperty"); t != nil {
		if !Truthy(t.invoke(p.handler, p.target, key, desc)) {
			panic(TypeErrorf("'defineProperty' on proxy: trap returned falsish for property '%s'", keyName(key)))
		}
		return
	}
	p.target.DefineProperty(key, desc)
}

func (p *proxyData) getPrototypeOf() Value {
	if t := p.trap("getPrototypeOf"); t != nil {
		return t.invoke(p.handler, p.target)
	}
	return orUndefinedNull(p.target.proto)
}

func (p *proxyData) hasOwn(key PropKey) bool {
	if t := p.trap("getOwnPropertyDescriptor"); t != nil {
		return t.invoke(p.handler, p.target, key) != Undefined
	}
	return p.target.HasOwn(key)
}

func orUndefinedNull(o *Object) Value {
	if o == nil {
		return Null
	}
	return o
}

// NewProxy is new Proxy(target, handler).
func (rt *Realm) NewProxy(target, handler *Object) *Object {
	p := &proxyData{rt: rt, target: target, handler: handler}
	o := &Object{proto: target.proto, shape: &Shape{dict: true}, proxy: p, Class: target.Class}
	if target.call != nil {
		o.realm = rt
		o.call = func(this Value, args []Value) Value {
			if t := p.trap("apply"); t != nil {
				return t.invoke(p.handler, p.target, this, rt.NewArray(append([]Value(nil), args...)))
			}
			return p.target.invoke(this, args...)
		}
		if target.construct != nil {
			o.construct = func(args []Value, nt *Object) Value {
				if nt == o {
					nt = p.target
				}
				if t := p.trap("construct"); t != nil {
					r := t.invoke(p.handler, p.target, rt.NewArray(append([]Value(nil), args...)), nt)
					if _, ok := r.(*Object); !ok {
						panic(TypeErrorf("proxy [[Construct]] must return an object"))
					}
					return r
				}
				return p.target.constructArgs(args, nt)
			}
		}
	}
	return o
}

func (rt *Realm) setupProxy() {
	toObject := func(v Value, what string) *Object {
		o, ok := v.(*Object)
		if !ok {
			panic(TypeErrorf("%s must be an object", what))
		}
		return o
	}
	ctor := rt.NewConstructor("Proxy", 2, func(Value, []Value) Value {
		panic(TypeErrorf("Constructor Proxy requires 'new'"))
	}, func(args []Value, _ *Object) Value {
		return rt.NewProxy(toObject(Arg(args, 0), "Proxy target"), toObject(Arg(args, 1), "Proxy handler"))
	}, nil)
	ctor.Delete("prototype")
	rt.method(ctor, "revocable", 2, func(_ Value, args []Value) Value {
		p := rt.NewProxy(toObject(Arg(args, 0), "Proxy target"), toObject(Arg(args, 1), "Proxy handler"))
		revoke := rt.NewFunction("", 0, func(Value, []Value) Value {
			p.proxy.handler = nil
			return Undefined
		})
		return rt.Obj("proxy", p, "revoke", revoke)
	})
	rt.Global.DefineHidden("Proxy", ctor)

	r := rt.NewPlainObject()
	r.DefineHidden(SymbolToStringTag, "Reflect")
	rt.Global.DefineHidden("Reflect", r)
	target := func(args []Value, name string) *Object {
		return toObject(Arg(args, 0), "Reflect."+name+" target")
	}
	rt.method(r, "get", 2, func(_ Value, args []Value) Value {
		o := target(args, "get")
		receiver := Value(o)
		if len(args) > 2 {
			receiver = args[2]
		}
		return o.GetFrom(ToPropertyKey(Arg(args, 1)), receiver)
	})
	rt.method(r, "set", 3, func(_ Value, args []Value) Value {
		o := target(args, "set")
		receiver := Value(o)
		if len(args) > 3 {
			receiver = args[3]
		}
		return succeeds(func() { o.SetFrom(ToPropertyKey(Arg(args, 1)), Arg(args, 2), receiver) })
	})
	rt.method(r, "has", 2, func(_ Value, args []Value) Value { return target(args, "has").Has(ToPropertyKey(Arg(args, 1))) })
	rt.method(r, "deleteProperty", 2, func(_ Value, args []Value) Value {
		o := target(args, "deleteProperty")
		var ok bool
		if !succeeds(func() { ok = o.Delete(ToPropertyKey(Arg(args, 1))) }) {
			return false
		}
		return ok
	})
	rt.method(r, "ownKeys", 1, func(_ Value, args []Value) Value {
		return rt.NewArray(target(args, "ownKeys").OwnKeys(false, true))
	})
	rt.method(r, "getOwnPropertyDescriptor", 2, func(_ Value, args []Value) Value {
		o := target(args, "getOwnPropertyDescriptor")
		if o.proxy != nil {
			return o.proxy.ownDescriptor(ToPropertyKey(Arg(args, 1)))
		}
		return o.descriptor(rt, ToPropertyKey(Arg(args, 1)))
	})
	rt.method(r, "defineProperty", 3, func(_ Value, args []Value) Value {
		o := target(args, "defineProperty")
		d := toObject(Arg(args, 2), "Property description")
		return succeeds(func() { o.DefineProperty(ToPropertyKey(Arg(args, 1)), d) })
	})
	rt.method(r, "getPrototypeOf", 1, func(_ Value, args []Value) Value {
		o := target(args, "getPrototypeOf")
		if o.proxy != nil {
			return o.proxy.getPrototypeOf()
		}
		return orUndefinedNull(o.proto)
	})
	rt.method(r, "setPrototypeOf", 2, func(_ Value, args []Value) Value {
		o := target(args, "setPrototypeOf")
		switch p := Arg(args, 1).(type) {
		case *Object:
			o.SetProto(p)
		case nullType:
			o.SetProto(nil)
		default:
			panic(TypeErrorf("Object prototype may only be an Object or null"))
		}
		return true
	})
	rt.method(r, "isExtensible", 1, func(_ Value, args []Value) Value { return !target(args, "isExtensible").frozen })
	rt.method(r, "preventExtensions", 1, func(_ Value, args []Value) Value {
		target(args, "preventExtensions").Freeze()
		return true
	})
	rt.method(r, "apply", 3, func(_ Value, args []Value) Value {
		f, ok := Arg(args, 0).(*Object)
		if !ok || f.call == nil {
			panic(TypeErrorf("Reflect.apply target is not a function"))
		}
		return f.invoke(Arg(args, 1), ListFromArrayLike(Arg(args, 2))...)
	})
	rt.method(r, "construct", 2, func(_ Value, args []Value) Value {
		f, ok := Arg(args, 0).(*Object)
		if !ok || f.construct == nil {
			panic(TypeErrorf("Reflect.construct target is not a constructor"))
		}
		nt := f
		if len(args) > 2 {
			n, ok := args[2].(*Object)
			if !ok || n.construct == nil {
				panic(TypeErrorf("Reflect.construct newTarget is not a constructor"))
			}
			nt = n
		}
		return f.constructArgs(ListFromArrayLike(Arg(args, 1)), nt)
	})
}

// succeeds runs f and reports whether it finished without a TypeError,
// for the Reflect functions that return false where an operator throws.
func succeeds(f func()) (ok bool) {
	defer func() {
		if r := recover(); r != nil {
			if t, isThrow := r.(*Throw); isThrow && t.Value == nil && t.Kind == "TypeError" {
				ok = false
				return
			}
			panic(r)
		}
	}()
	f()
	return true
}

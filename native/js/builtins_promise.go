package js

// Promise. Reactions are jobs on a queue that RunJobs drains. Tern's
// QuickJS renderer never drains its queue, since server rendering is
// synchronous, so the native renderer doesn't either: a render's output
// can't depend on a job.

const (
	pending = iota
	fulfilled
	rejected
)

type reaction struct {
	onFulfilled, onRejected Value
	resolve, reject         func(Value)
}

type promise struct {
	rt        *Realm
	state     int
	value     Value
	reactions []reaction
}

// RunJobs runs queued jobs, and the jobs they queue, until none are left.
func (rt *Realm) RunJobs() {
	for len(rt.jobs) > 0 {
		j := rt.jobs[0]
		rt.jobs = rt.jobs[1:]
		j()
	}
}

// DropJobs forgets the queued jobs.
func (rt *Realm) DropJobs() { rt.jobs = nil }

func thisPromise(v Value) *promise {
	if o, ok := v.(*Object); ok {
		if p, ok := o.Internal.(*promise); ok {
			return p
		}
	}
	panic(TypeErrorf("not a Promise"))
}

func (rt *Realm) newPromise(proto *Object) (*Object, *promise) {
	o := NewObject(proto)
	p := &promise{rt: rt}
	o.Class, o.Internal = "Promise", p
	return o, p
}

func (p *promise) settle(state int, v Value) {
	if p.state != pending {
		return
	}
	p.state, p.value = state, v
	rs := p.reactions
	p.reactions = nil
	for _, rc := range rs {
		p.schedule(rc)
	}
}

func (p *promise) schedule(rc reaction) {
	state, v := p.state, p.value
	p.rt.jobs = append(p.rt.jobs, func() {
		h := rc.onFulfilled
		if state == rejected {
			h = rc.onRejected
		}
		f, ok := h.(*Object)
		if !ok || f.call == nil {
			if state == rejected {
				rc.reject(v)
			} else {
				rc.resolve(v)
			}
			return
		}
		out, thrown := callCatching(f, Undefined, v)
		if thrown != nil {
			rc.reject(thrown.In(p.rt))
			return
		}
		rc.resolve(out)
	})
}

func callCatching(f *Object, this Value, args ...Value) (out Value, thrown *Throw) {
	defer func() {
		if r := recover(); r != nil {
			t, ok := r.(*Throw)
			if !ok {
				panic(r)
			}
			thrown = t
		}
	}()
	return f.invoke(this, args...), nil
}

// resolvers makes a promise's resolve and reject functions.
func (rt *Realm) resolvers(o *Object, p *promise) (resolve, reject func(Value)) {
	done := false
	reject = func(v Value) {
		if !done {
			done = true
			p.settle(rejected, v)
		}
	}
	resolve = func(v Value) {
		if done {
			return
		}
		done = true
		if v == Value(o) {
			p.settle(rejected, TypeErrorf("Chaining cycle detected for promise").In(rt))
			return
		}
		t, ok := v.(*Object)
		if !ok {
			p.settle(fulfilled, v)
			return
		}
		then, thrown := getCatching(t, "then")
		if thrown != nil {
			p.settle(rejected, thrown.In(rt))
			return
		}
		tf, ok := then.(*Object)
		if !ok || tf.call == nil {
			p.settle(fulfilled, v)
			return
		}
		// Adopt the thenable's state, in a job.
		rt.jobs = append(rt.jobs, func() {
			res, rej := rt.resolvers(o, p)
			if _, th := callCatching(tf, t, rt.fnValue(res), rt.fnValue(rej)); th != nil {
				rej(th.In(rt))
			}
		})
	}
	return resolve, reject
}

func getCatching(o *Object, key PropKey) (v Value, thrown *Throw) {
	defer func() {
		if r := recover(); r != nil {
			t, ok := r.(*Throw)
			if !ok {
				panic(r)
			}
			thrown = t
		}
	}()
	return o.Get(key), nil
}

func (rt *Realm) fnValue(f func(Value)) *Object {
	return rt.NewFunction("", 1, func(_ Value, args []Value) Value { f(Arg(args, 0)); return Undefined })
}

// promiseResolve is Promise.resolve.
func (rt *Realm) promiseResolve(v Value) *Object {
	if o, ok := v.(*Object); ok {
		if _, ok := o.Internal.(*promise); ok && o.Get("constructor") == Value(rt.promiseCtor) {
			return o
		}
	}
	o, p := rt.newPromise(rt.PromisePrototype)
	res, _ := rt.resolvers(o, p)
	res(v)
	return o
}

func (p *promise) then(onF, onR Value) *Object {
	o, np := p.rt.newPromise(p.rt.PromisePrototype)
	res, rej := p.rt.resolvers(o, np)
	rc := reaction{onFulfilled: onF, onRejected: onR, resolve: res, reject: rej}
	if p.state == pending {
		p.reactions = append(p.reactions, rc)
	} else {
		p.schedule(rc)
	}
	return o
}

func (rt *Realm) setupPromise() {
	rt.promiseCtor = rt.NewConstructor("Promise", 1, func(Value, []Value) Value {
		panic(TypeErrorf("Promise constructor cannot be invoked without 'new'"))
	}, func(args []Value, nt *Object) Value {
		exec, ok := Arg(args, 0).(*Object)
		if !ok || exec.call == nil {
			panic(TypeErrorf("Promise resolver %s is not a function", describeForError(Arg(args, 0))))
		}
		o, p := rt.newPromise(OrdinaryCreateFromConstructor(nt, rt.PromisePrototype).proto)
		res, rej := rt.resolvers(o, p)
		if _, th := callCatching(exec, Undefined, rt.fnValue(res), rt.fnValue(rej)); th != nil {
			rej(th.In(rt))
		}
		return o
	}, rt.PromisePrototype)
	rt.Global.DefineHidden("Promise", rt.promiseCtor)
	rt.PromisePrototype.DefineHidden(SymbolToStringTag, "Promise")

	rt.method(rt.PromisePrototype, "then", 2, func(this Value, args []Value) Value {
		return thisPromise(this).then(Arg(args, 0), Arg(args, 1))
	})
	rt.method(rt.PromisePrototype, "catch", 1, func(this Value, args []Value) Value {
		return Call(rt.Get(this, "then"), this, Undefined, Arg(args, 0))
	})
	rt.method(rt.PromisePrototype, "finally", 1, func(this Value, args []Value) Value {
		f, ok := Arg(args, 0).(*Object)
		if !ok || f.call == nil {
			return Call(rt.Get(this, "then"), this, Arg(args, 0), Arg(args, 0))
		}
		onF := rt.NewFunction("", 1, func(_ Value, a []Value) Value {
			v := Arg(a, 0)
			return rt.promiseResolve(f.invoke(Undefined)).Internal.(*promise).then(rt.NewFunction("", 0, func(Value, []Value) Value { return v }), Undefined)
		})
		onR := rt.NewFunction("", 1, func(_ Value, a []Value) Value {
			v := Arg(a, 0)
			return rt.promiseResolve(f.invoke(Undefined)).Internal.(*promise).then(rt.NewFunction("", 0, func(Value, []Value) Value { panic(&Throw{Value: v}) }), Undefined)
		})
		return Call(rt.Get(this, "then"), this, onF, onR)
	})

	rt.method(rt.promiseCtor, "resolve", 1, func(_ Value, args []Value) Value { return rt.promiseResolve(Arg(args, 0)) })
	rt.method(rt.promiseCtor, "reject", 1, func(_ Value, args []Value) Value {
		o, p := rt.newPromise(rt.PromisePrototype)
		p.settle(rejected, Arg(args, 0))
		return o
	})
	rt.method(rt.promiseCtor, "withResolvers", 0, func(Value, []Value) Value {
		o, p := rt.newPromise(rt.PromisePrototype)
		res, rej := rt.resolvers(o, p)
		return rt.Obj("promise", o, "resolve", rt.fnValue(res), "reject", rt.fnValue(rej))
	})

	// combine implements all, allSettled, any and race.
	combine := func(name string, onItem func(i int, item *Object, done func(int, Value, bool), results []Value), empty func(res, rej func(Value), results []Value)) {
		rt.method(rt.promiseCtor, name, 1, func(_ Value, args []Value) Value {
			o, p := rt.newPromise(rt.PromisePrototype)
			res, rej := rt.resolvers(o, p)
			items := rt.Spread(Arg(args, 0))
			results := make([]Value, len(items))
			remaining := len(items)
			done := func(i int, v Value, final bool) {
				switch {
				case final:
					res(v)
				case i < 0:
					rej(v)
				default:
					results[i] = v
					remaining--
					if remaining == 0 {
						empty(res, rej, results)
					}
				}
			}
			if len(items) == 0 {
				empty(res, rej, results)
				return o
			}
			for i, it := range items {
				onItem(i, rt.promiseResolve(it), done, results)
			}
			return o
		})
	}
	thenOf := func(item *Object, onF, onR func(Value)) {
		Call(item.Get("then"), item, rt.fnValue(onF), rt.fnValue(onR))
	}
	combine("all", func(i int, item *Object, done func(int, Value, bool), _ []Value) {
		thenOf(item, func(v Value) { done(i, v, false) }, func(e Value) { done(-1, e, false) })
	}, func(res, _ func(Value), results []Value) { res(rt.NewArray(results)) })
	combine("allSettled", func(i int, item *Object, done func(int, Value, bool), _ []Value) {
		thenOf(item, func(v Value) { done(i, rt.Obj("status", "fulfilled", "value", v), false) },
			func(e Value) { done(i, rt.Obj("status", "rejected", "reason", e), false) })
	}, func(res, _ func(Value), results []Value) { res(rt.NewArray(results)) })
	combine("race", func(_ int, item *Object, done func(int, Value, bool), _ []Value) {
		thenOf(item, func(v Value) { done(0, v, true) }, func(e Value) { done(-1, e, false) })
	}, func(func(Value), func(Value), []Value) {})
	combine("any", func(i int, item *Object, done func(int, Value, bool), _ []Value) {
		thenOf(item, func(v Value) { done(0, v, true) }, func(e Value) { done(i, e, false) })
	}, func(_, rej func(Value), results []Value) {
		e := Construct(rt.Global.Get("AggregateError"), rt.NewArray(results), "All promises were rejected")
		rej(e)
	})
}

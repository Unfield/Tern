package js

// Promise. Reactions are jobs on a queue that RunJobs drains. Tern's
// QuickJS renderer never drains its queue, since server rendering is
// synchronous, so the native renderer doesn't either: a render's output
// can't depend on a job.

// PromisePrototype is Promise.prototype.
var PromisePrototype = &Object{proto: ObjectPrototype}

var promiseCtor *Object

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
	state     int
	value     Value
	reactions []reaction
}

// jobs is the job queue. Native rendering runs one render at a time per
// realm, so a plain slice does.
var jobs []func()

// RunJobs runs queued jobs, and the jobs they queue, until none are left.
func RunJobs() {
	for len(jobs) > 0 {
		j := jobs[0]
		jobs = jobs[1:]
		j()
	}
}

// DropJobs forgets the queued jobs.
func DropJobs() { jobs = nil }

func thisPromise(v Value) *promise {
	if o, ok := v.(*Object); ok {
		if p, ok := o.Internal.(*promise); ok {
			return p
		}
	}
	panic(TypeErrorf("not a Promise"))
}

func newPromise(proto *Object) (*Object, *promise) {
	o := NewObject(proto)
	p := &promise{}
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
	for _, r := range rs {
		p.schedule(r)
	}
}

func (p *promise) schedule(r reaction) {
	state, v := p.state, p.value
	jobs = append(jobs, func() {
		h := r.onFulfilled
		if state == rejected {
			h = r.onRejected
		}
		f, ok := h.(*Object)
		if !ok || f.call == nil {
			if state == rejected {
				r.reject(v)
			} else {
				r.resolve(v)
			}
			return
		}
		out, thrown := callCatching(f, Undefined, v)
		if thrown != nil {
			r.reject(thrown.Value)
			return
		}
		r.resolve(out)
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
func resolvers(o *Object, p *promise) (resolve, reject func(Value)) {
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
			p.settle(rejected, newError(TypeError, "Chaining cycle detected for promise").Value)
			return
		}
		t, ok := v.(*Object)
		if !ok {
			p.settle(fulfilled, v)
			return
		}
		then, thrown := getCatching(t, "then")
		if thrown != nil {
			p.settle(rejected, thrown.Value)
			return
		}
		tf, ok := then.(*Object)
		if !ok || tf.call == nil {
			p.settle(fulfilled, v)
			return
		}
		// Adopt the thenable's state, in a job.
		jobs = append(jobs, func() {
			res, rej := resolvers(o, p)
			if _, th := callCatching(tf, t, fnValue(res), fnValue(rej)); th != nil {
				rej(th.Value)
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

func fnValue(f func(Value)) *Object {
	return NewFunction("", 1, func(_ Value, args []Value) Value { f(Arg(args, 0)); return Undefined })
}

// promiseResolve is Promise.resolve.
func promiseResolve(v Value) *Object {
	if o, ok := v.(*Object); ok {
		if _, ok := o.Internal.(*promise); ok && o.Get("constructor") == Value(promiseCtor) {
			return o
		}
	}
	o, p := newPromise(PromisePrototype)
	res, _ := resolvers(o, p)
	res(v)
	return o
}

func (p *promise) then(onF, onR Value) *Object {
	o, np := newPromise(PromisePrototype)
	res, rej := resolvers(o, np)
	r := reaction{onFulfilled: onF, onRejected: onR, resolve: res, reject: rej}
	if p.state == pending {
		p.reactions = append(p.reactions, r)
	} else {
		p.schedule(r)
	}
	return o
}

func setupPromise() {
	promiseCtor = NewConstructor("Promise", 1, func(Value, []Value) Value {
		panic(TypeErrorf("Promise constructor cannot be invoked without 'new'"))
	}, func(args []Value, nt *Object) Value {
		exec, ok := Arg(args, 0).(*Object)
		if !ok || exec.call == nil {
			panic(TypeErrorf("Promise resolver %s is not a function", describeForError(Arg(args, 0))))
		}
		o, p := newPromise(OrdinaryCreateFromConstructor(nt, PromisePrototype).proto)
		res, rej := resolvers(o, p)
		if _, th := callCatching(exec, Undefined, fnValue(res), fnValue(rej)); th != nil {
			rej(th.Value)
		}
		return o
	}, PromisePrototype)
	Global.DefineHidden("Promise", promiseCtor)
	PromisePrototype.DefineHidden(SymbolToStringTag, "Promise")

	method(PromisePrototype, "then", 2, func(this Value, args []Value) Value {
		return thisPromise(this).then(Arg(args, 0), Arg(args, 1))
	})
	method(PromisePrototype, "catch", 1, func(this Value, args []Value) Value {
		return Call(Get(this, "then"), this, Undefined, Arg(args, 0))
	})
	method(PromisePrototype, "finally", 1, func(this Value, args []Value) Value {
		f, ok := Arg(args, 0).(*Object)
		if !ok || f.call == nil {
			return Call(Get(this, "then"), this, Arg(args, 0), Arg(args, 0))
		}
		onF := NewFunction("", 1, func(_ Value, a []Value) Value {
			v := Arg(a, 0)
			return promiseResolve(f.invoke(Undefined)).Internal.(*promise).then(NewFunction("", 0, func(Value, []Value) Value { return v }), Undefined)
		})
		onR := NewFunction("", 1, func(_ Value, a []Value) Value {
			v := Arg(a, 0)
			return promiseResolve(f.invoke(Undefined)).Internal.(*promise).then(NewFunction("", 0, func(Value, []Value) Value { panic(&Throw{Value: v}) }), Undefined)
		})
		return Call(Get(this, "then"), this, onF, onR)
	})

	method(promiseCtor, "resolve", 1, func(_ Value, args []Value) Value { return promiseResolve(Arg(args, 0)) })
	method(promiseCtor, "reject", 1, func(_ Value, args []Value) Value {
		o, p := newPromise(PromisePrototype)
		p.settle(rejected, Arg(args, 0))
		return o
	})
	method(promiseCtor, "withResolvers", 0, func(Value, []Value) Value {
		o, p := newPromise(PromisePrototype)
		res, rej := resolvers(o, p)
		return Obj("promise", o, "resolve", fnValue(res), "reject", fnValue(rej))
	})

	// combine implements all, allSettled, any and race.
	combine := func(name string, onItem func(i int, item *Object, done func(int, Value, bool), results []Value), empty func(res, rej func(Value), results []Value)) {
		method(promiseCtor, name, 1, func(_ Value, args []Value) Value {
			o, p := newPromise(PromisePrototype)
			res, rej := resolvers(o, p)
			items := Spread(Arg(args, 0))
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
				onItem(i, promiseResolve(it), done, results)
			}
			return o
		})
	}
	thenOf := func(item *Object, onF, onR func(Value)) {
		Call(item.Get("then"), item, fnValue(onF), fnValue(onR))
	}
	combine("all", func(i int, item *Object, done func(int, Value, bool), _ []Value) {
		thenOf(item, func(v Value) { done(i, v, false) }, func(e Value) { done(-1, e, false) })
	}, func(res, _ func(Value), results []Value) { res(NewArray(results)) })
	combine("allSettled", func(i int, item *Object, done func(int, Value, bool), _ []Value) {
		thenOf(item, func(v Value) { done(i, Obj("status", "fulfilled", "value", v), false) },
			func(e Value) { done(i, Obj("status", "rejected", "reason", e), false) })
	}, func(res, _ func(Value), results []Value) { res(NewArray(results)) })
	combine("race", func(_ int, item *Object, done func(int, Value, bool), _ []Value) {
		thenOf(item, func(v Value) { done(0, v, true) }, func(e Value) { done(-1, e, false) })
	}, func(func(Value), func(Value), []Value) {})
	combine("any", func(i int, item *Object, done func(int, Value, bool), _ []Value) {
		thenOf(item, func(v Value) { done(0, v, true) }, func(e Value) { done(i, e, false) })
	}, func(_, rej func(Value), results []Value) {
		e := Construct(Global.Get("AggregateError"), NewArray(results), "All promises were rejected")
		rej(e)
	})
}

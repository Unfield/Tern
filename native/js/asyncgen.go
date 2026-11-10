package js

// Async generators and for await. The body runs on a coroutine; await
// and yield both suspend it, an await marked as one, so the driver knows
// whether to wait for a value or to hand one out. Calls to next, return
// and throw queue up, each with its promise, and are served in order.

// awaiting is what Await hands the driver of an async generator.
type awaiting struct{ v Value }

type agRequest struct {
	mode          int
	value         Value
	resolve, fail func(Value)
}

// asyncGen is an async generator object's state.
type asyncGen struct {
	rt      *Realm
	g       *Gen
	owner   *Object
	queue   []agRequest
	running bool // a request is being served
	done    bool
}

// NewAsyncGeneratorFunction makes an async generator function.
func (rt *Realm) NewAsyncGeneratorFunction(name string, length int, call func(this Value, args []Value) func(*Gen) Value) *Object {
	var f *Object
	f = rt.NewFunction(name, length, func(this Value, args []Value) Value {
		body := call(this, args)
		proto, ok := f.Get("prototype").(*Object)
		if !ok {
			proto = rt.AsyncGeneratorPrototype
		}
		o := NewObject(proto)
		g := &Gen{rt: rt, body: body, async: true}
		o.Class, o.Internal = "AsyncGenerator", &asyncGen{rt: rt, g: g, owner: &Object{}}
		return o
	})
	f.add(prop{key: "prototype", value: NewObject(rt.AsyncGeneratorPrototype), writable: true})
	return f
}

// AsyncYield is yield in an async generator: the value is awaited first.
func (g *Gen) AsyncYield(v Value) Value { return g.Yield(g.Await(v)) }

// request queues a next, return or throw, and serves the queue if idle.
func (a *asyncGen) request(mode int, v Value) Value {
	po, p := a.rt.newPromise(a.rt.PromisePrototype)
	resolve, reject := a.rt.resolvers(po, p)
	a.queue = append(a.queue, agRequest{mode: mode, value: v, resolve: resolve, fail: reject})
	if !a.running {
		a.serve()
	}
	return po
}

// serve takes the next request.
func (a *asyncGen) serve() {
	for len(a.queue) > 0 && !a.running {
		req := a.queue[0]
		if a.done {
			a.queue = a.queue[1:]
			switch req.mode {
			case resumeThrow:
				req.fail(req.value)
			case resumeReturn:
				a.awaitThen(req.value, func(v Value) { req.resolve(iterResultIn(a.rt, v, true)) }, req.fail)
				return
			default:
				req.resolve(iterResultIn(a.rt, Undefined, true))
			}
			continue
		}
		a.running = true
		a.step(req.mode, req.value)
	}
}

// awaitThen waits for v to settle, then calls ok or fail, and serves on.
func (a *asyncGen) awaitThen(v Value, ok, fail func(Value)) {
	a.running = true
	a.rt.promiseResolve(v).Internal.(*promise).then(
		a.rt.NewFunction("", 1, func(_ Value, args []Value) Value {
			a.running = false
			ok(Arg(args, 0))
			a.serve()
			return Undefined
		}),
		a.rt.NewFunction("", 1, func(_ Value, args []Value) Value {
			a.running = false
			fail(Arg(args, 0))
			a.serve()
			return Undefined
		}))
}

// step resumes the body for the head request.
func (a *asyncGen) step(mode int, v Value) {
	req := a.queue[0]
	if !a.g.started && mode != resumeNext {
		// return or throw before the body started: it never does.
		a.done = true
		a.running = false
		a.serve()
		return
	}
	var out Value
	var finished bool
	if th := catchThrow(func() { out, finished = a.g.resume(mode, v, a.owner) }); th != nil {
		a.done, a.running = true, false
		a.queue = a.queue[1:]
		req.fail(th.In(a.rt))
		a.serve()
		return
	}
	if w, ok := out.(awaiting); ok && !finished {
		// An await in the body: resume it when the value settles.
		a.rt.promiseResolve(w.v).Internal.(*promise).then(
			a.rt.NewFunction("", 1, func(_ Value, args []Value) Value { a.step(resumeNext, Arg(args, 0)); return Undefined }),
			a.rt.NewFunction("", 1, func(_ Value, args []Value) Value { a.step(resumeThrow, Arg(args, 0)); return Undefined }))
		return
	}
	a.queue = a.queue[1:]
	a.running = false
	if finished {
		a.done = true
	}
	req.resolve(iterResultIn(a.rt, out, finished))
	a.serve()
}

func iterResultIn(rt *Realm, v Value, done bool) *Object { return rt.Obj("value", v, "done", done) }

func (rt *Realm) setupAsyncGenerators() {
	rt.AsyncIteratorPrototype = rt.NewPlainObject()
	rt.method(rt.AsyncIteratorPrototype, "[Symbol.asyncIterator]", 0, func(this Value, _ []Value) Value { return this })
	f, _ := rt.AsyncIteratorPrototype.GetOwn("[Symbol.asyncIterator]")
	rt.AsyncIteratorPrototype.Delete("[Symbol.asyncIterator]")
	rt.AsyncIteratorPrototype.DefineHidden(SymbolAsyncIterator, f)
	p := NewObject(rt.AsyncIteratorPrototype)
	rt.AsyncGeneratorPrototype = p
	this := func(v Value) *asyncGen {
		if o, ok := v.(*Object); ok {
			if a, ok := o.Internal.(*asyncGen); ok {
				return a
			}
		}
		panic(TypeErrorf("not an async generator"))
	}
	reject := func(err *Throw) Value {
		po, pr := rt.newPromise(rt.PromisePrototype)
		_, rej := rt.resolvers(po, pr)
		rej(err.In(rt))
		return po
	}
	for _, m := range []struct {
		name string
		mode int
	}{{"next", resumeNext}, {"return", resumeReturn}, {"throw", resumeThrow}} {
		rt.method(p, m.name, 1, func(t Value, args []Value) Value {
			var a *asyncGen
			if th := catchThrow(func() { a = this(t) }); th != nil {
				return reject(th)
			}
			return a.request(m.mode, Arg(args, 0))
		})
	}
	p.DefineHidden(SymbolToStringTag, "AsyncGenerator")
}

// AsyncIterate starts a for await: it returns the function that gets the
// next result's promise. A sync iterable is wrapped, its values awaited.
func (rt *Realm) AsyncIterate(v Value) func() Value {
	if m, ok := rt.Get(v, SymbolAsyncIterator).(*Object); ok && m.call != nil {
		it := m.invoke(v)
		next := rt.Get(it, "next")
		return func() Value { return Call(next, it) }
	}
	sync := rt.Iterate(v)
	return func() Value {
		x, ok := sync()
		if !ok {
			return rt.promiseResolve(iterResultIn(rt, Undefined, true))
		}
		start := rt.promiseResolve(x).Internal.(*promise)
		return start.then(rt.NewFunction("", 1, func(_ Value, a []Value) Value { return iterResultIn(rt, Arg(a, 0), false) }), Undefined)
	}
}

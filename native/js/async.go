package js

// Async functions. The body runs on a coroutine, as a generator's does;
// await hands the awaited value to the driver, which resumes the body from
// a promise job once the value settles. Like the QuickJS renderer, a
// render never drains the job queue, so an async function called while
// rendering runs to its first await and no further, in both.

// NewAsyncFunction makes an async function. call binds the parameters and
// returns the body.
func (rt *Realm) NewAsyncFunction(name string, length int, call func(this Value, args []Value) func(*Gen) Value) *Object {
	return rt.NewFunction(name, length, func(this Value, args []Value) Value {
		po, p := rt.newPromise(rt.PromisePrototype)
		resolve, reject := rt.resolvers(po, p)
		var body func(*Gen) Value
		if th := catchThrow(func() { body = call(this, args) }); th != nil {
			reject(th.In(rt)) // a failing default parameter rejects too
			return po
		}
		g := &Gen{rt: rt, body: body}
		owner := &Object{} // drops the coroutine if no job ever resumes it
		var step func(mode int, v Value)
		onFulfilled := rt.NewFunction("", 1, func(_ Value, a []Value) Value { step(resumeNext, Arg(a, 0)); return Undefined })
		onRejected := rt.NewFunction("", 1, func(_ Value, a []Value) Value { step(resumeThrow, Arg(a, 0)); return Undefined })
		step = func(mode int, v Value) {
			var out Value
			var done bool
			if th := catchThrow(func() { out, done = g.resume(mode, v, owner) }); th != nil {
				reject(th.In(rt))
				return
			}
			if done {
				resolve(out)
				return
			}
			rt.promiseResolve(out).Internal.(*promise).then(onFulfilled, onRejected)
		}
		step(resumeNext, Undefined)
		return po
	})
}

// Await is an await expression: the body waits for v to settle, and gets
// its value, or its reason thrown.
func (g *Gen) Await(v Value) Value { return g.Yield(v) }

// catchThrow runs f and returns the JavaScript exception it threw, if
// any; other panics go on.
func catchThrow(f func()) (thrown *Throw) {
	defer func() {
		if r := recover(); r != nil {
			t, ok := r.(*Throw)
			if !ok {
				panic(r)
			}
			thrown = t
		}
	}()
	f()
	return nil
}

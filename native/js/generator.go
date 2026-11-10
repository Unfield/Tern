package js

import (
	"iter"
	"runtime"
)

// Generators. A generator's body is a Go function that runs on a
// coroutine (iter.Pull) and calls Yield. A yield* of a fresh generator
// runs that generator's body on the same coroutine, so a recursive
// generator, like Svelte's walk of its renderer tree, costs one coroutine
// in all.

// GeneratorPrototype is %GeneratorPrototype%, the prototype of generator
// functions' prototype objects.
var GeneratorPrototype = &Object{proto: IteratorPrototype}

// GeneratorFunctionPrototype is %GeneratorFunction.prototype%.
var GeneratorFunctionPrototype = &Object{proto: FunctionPrototype}

const (
	resumeNext = iota
	resumeThrow
	resumeReturn
)

// genIO is what a resumption passes into the running body: shared by
// generators delegated to with yield*.
type genIO struct {
	yield func(Value) bool
	sent  Value
	mode  int
}

// Gen is a generator object's state.
type Gen struct {
	body    func(*Gen) Value
	io      *genIO
	next    func() (Value, bool)
	stop    func()
	result  Value // the body's return value
	started bool
	running bool
	done    bool
}

// genReturn unwinds a body for return(): finally blocks run, catch blocks
// don't.
type genReturn struct{ value Value }

// genAbort unwinds the body of a generator nothing can reach any more:
// nothing runs, as JavaScript would never resume it.
type genAbort struct{}

// IsAbort reports whether a panic is a generator being dropped, which Try
// lets through without running finally blocks.
func isAbort(r any) bool { _, ok := r.(genAbort); return ok }

// NewGeneratorFunction makes a generator function. call runs when it's
// called: it binds the parameters and returns the body.
func NewGeneratorFunction(name string, length int, call func(this Value, args []Value) func(*Gen) Value) *Object {
	var f *Object
	f = NewFunction(name, length, func(this Value, args []Value) Value {
		body := call(this, args)
		proto, ok := f.Get("prototype").(*Object)
		if !ok {
			proto = GeneratorPrototype
		}
		o := NewObject(proto)
		o.Class, o.Internal = "Generator", &Gen{body: body}
		return o
	})
	f.SetProto(GeneratorFunctionPrototype)
	f.add(prop{key: "prototype", value: NewObject(GeneratorPrototype), writable: true})
	return f
}

func thisGen(v Value) *Gen {
	if o, ok := v.(*Object); ok {
		if g, ok := o.Internal.(*Gen); ok {
			return g
		}
	}
	panic(TypeErrorf("not a generator"))
}

// Yield is a yield expression: it hands v to the consumer and returns what
// the next call to next passes.
func (g *Gen) Yield(v Value) Value {
	io := g.io
	if !io.yield(v) {
		panic(genAbort{})
	}
	switch io.mode {
	case resumeThrow:
		panic(&Throw{Value: io.sent})
	case resumeReturn:
		panic(genReturn{io.sent})
	}
	return io.sent
}

// YieldStar is yield* v.
func (g *Gen) YieldStar(v Value) Value {
	if o, ok := v.(*Object); ok {
		if inner, ok := o.Internal.(*Gen); ok && !inner.started && o.Get("next") == genNext {
			// Run the delegate's body right here, with our consumer.
			inner.started, inner.running, inner.io = true, true, g.io
			defer func() { inner.running, inner.done = false, true }()
			return inner.body(inner)
		}
	}
	it := getIterator(v)
	next := Get(it, "next")
	io := g.io
	mode, sent := resumeNext, Undefined
	for {
		var r Value
		switch mode {
		case resumeNext:
			r = Call(next, it, sent)
		case resumeThrow:
			th := Get(it, "throw")
			if IsNullish(th) {
				closeIterator(it)
				panic(TypeErrorf("The iterator does not provide a 'throw' method"))
			}
			r = Call(th, it, sent)
		case resumeReturn:
			ret := Get(it, "return")
			if IsNullish(ret) {
				panic(genReturn{sent})
			}
			r = Call(ret, it, sent)
		}
		if _, ok := r.(*Object); !ok {
			panic(TypeErrorf("iterator result %s is not an object", ToDisplay(r)))
		}
		if Truthy(Get(r, "done")) {
			if mode == resumeReturn {
				panic(genReturn{Get(r, "value")})
			}
			return Get(r, "value")
		}
		if !io.yield(Get(r, "value")) {
			panic(genAbort{})
		}
		mode, sent = io.mode, io.sent
	}
}

func getIterator(v Value) Value {
	m, ok := Get(v, SymbolIterator).(*Object)
	if !ok || m.call == nil {
		panic(TypeErrorf("%s is not iterable", describeForError(v)))
	}
	it := m.invoke(v)
	if _, ok := it.(*Object); !ok {
		panic(TypeErrorf("Result of the Symbol.iterator method is not an object"))
	}
	return it
}

func closeIterator(it Value) {
	if ret := Get(it, "return"); !IsNullish(ret) {
		Call(ret, it)
	}
}

// start makes the coroutine that runs the body.
func (g *Gen) start() {
	g.started = true
	io := &genIO{}
	g.io = io
	seq := func(yield func(Value) bool) {
		io.yield = yield
		defer func() {
			if r := recover(); r != nil {
				switch r := r.(type) {
				case genReturn:
					g.result = r.value
				case genAbort:
				default:
					panic(r)
				}
			}
		}()
		g.result = g.body(g)
	}
	g.next, g.stop = iter.Pull(iter.Seq[Value](seq))
}

// resume runs the body until it yields or ends.
func (g *Gen) resume(mode int, v Value, owner *Object) (Value, bool) {
	if g.running {
		panic(TypeErrorf("Generator is already running"))
	}
	if g.done {
		switch mode {
		case resumeThrow:
			panic(&Throw{Value: v})
		case resumeReturn:
			return v, true
		}
		return Undefined, true
	}
	if !g.started {
		if mode != resumeNext {
			g.done, g.started = true, true
			if mode == resumeThrow {
				panic(&Throw{Value: v})
			}
			return v, true
		}
		g.start()
		// Drop the coroutine with the generator, if it's never finished.
		runtime.AddCleanup(owner, func(stop func()) { stop() }, g.stop)
	}
	g.io.mode, g.io.sent = mode, v
	g.running = true
	finished := true
	defer func() {
		g.running = false
		if finished {
			g.done = true
			g.body, g.io = nil, nil
		}
	}()
	out, ok := g.next()
	if ok {
		finished = false
		return out, false
	}
	return g.result, true
}

func iterResult(v Value, done bool) *Object {
	r := NewPlainObject()
	r.add(prop{key: "value", value: v, enumerable: true, writable: true, configurable: true})
	r.add(prop{key: "done", value: done, enumerable: true, writable: true, configurable: true})
	return r
}

var genNext *Object

func setupGenerators() {
	p := GeneratorPrototype
	genNext = method(p, "next", 1, func(this Value, args []Value) Value {
		return iterResult(thisGen(this).resume(resumeNext, Arg(args, 0), this.(*Object)))
	})
	method(p, "return", 1, func(this Value, args []Value) Value {
		return iterResult(thisGen(this).resume(resumeReturn, Arg(args, 0), this.(*Object)))
	})
	method(p, "throw", 1, func(this Value, args []Value) Value {
		return iterResult(thisGen(this).resume(resumeThrow, Arg(args, 0), this.(*Object)))
	})
	p.DefineHidden(SymbolToStringTag, "Generator")
	GeneratorFunctionPrototype.DefineHidden("prototype", GeneratorPrototype)
	GeneratorFunctionPrototype.DefineHidden(SymbolToStringTag, "GeneratorFunction")
	GeneratorPrototype.DefineHidden("constructor", GeneratorFunctionPrototype)
}

// genIterator is the Go iterator of a for…of over a generator that still
// has the built-in next: no result objects are made.
func genIterator(o *Object, g *Gen) func() (Value, bool) {
	return func() (Value, bool) {
		v, done := g.resume(resumeNext, Undefined, o)
		return v, !done
	}
}

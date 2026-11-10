package goscript

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"runtime/debug"
	"sync"
	"sync/atomic"
)

// Locals returns request values a live component was mounted with, such
// as the signed-in user from ctx.Locals; nil in the browser.
func (c *Component) Locals() map[string]any { return c.locals }

// actor runs a live component's work on one goroutine, in order, so its
// code never runs concurrently with itself while different components run
// in parallel.
type actor struct {
	mu      sync.Mutex
	tasks   []func()
	wake    chan struct{}
	done    chan struct{}
	stopped bool
	panic   func(any)
}

func newActor(onPanic func(any)) *actor {
	a := &actor{wake: make(chan struct{}, 1), done: make(chan struct{}), panic: onPanic}
	go a.loop()
	return a
}

func (a *actor) post(fn func()) bool {
	a.mu.Lock()
	if a.stopped {
		a.mu.Unlock()
		return false
	}
	a.tasks = append(a.tasks, fn)
	a.mu.Unlock()
	select {
	case a.wake <- struct{}{}:
	default:
	}
	return true
}

func (a *actor) next() (func(), bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.tasks) == 0 {
		return nil, a.stopped
	}
	t := a.tasks[0]
	a.tasks[0] = nil
	a.tasks = a.tasks[1:]
	return t, false
}

func (a *actor) loop() {
	defer close(a.done)
	for {
		t, stopped := a.next()
		if stopped {
			return
		}
		if t == nil {
			<-a.wake
			continue
		}
		a.run(t)
	}
}

func (a *actor) run(t func()) {
	defer func() {
		if r := recover(); r != nil && a.panic != nil {
			a.panic(r)
		}
	}()
	t()
}

// drain runs the tasks queued so far, such as updates a Func caused in
// this component through a Global, so they reach the page before the
// Func's result.
func (a *actor) drain() {
	a.mu.Lock()
	n := len(a.tasks)
	a.mu.Unlock()
	for range n {
		t, _ := a.next()
		if t == nil {
			return
		}
		a.run(t)
	}
}

// stop ends the loop after the running task; queued tasks are dropped.
func (a *actor) stop() {
	a.mu.Lock()
	a.stopped = true
	a.tasks = nil
	a.mu.Unlock()
	select {
	case a.wake <- struct{}{}:
	default:
	}
}

// LiveOptions configure [MountLive].
type LiveOptions struct {
	Props   json.RawMessage            // the props, as JSON
	Restore map[string]json.RawMessage // values the page shows, for State declared with Bind or Restore
	Locals  map[string]any             // request values, for Component.Locals

	// Emit receives each value that changed; Commit is called after the
	// changes of one batch, so they can be sent together.
	Emit   func(name string, value []byte)
	Commit func()
	// Event receives what the component passes to Emit.
	Event func(name string, value []byte)
	// Panic receives a panic recovered in the component's code.
	Panic func(v any, stack []byte)
}

// Instance is a mounted live component. It is safe for concurrent use.
type Instance struct {
	c *Component
	a *actor
}

var liveInstances atomic.Int64

// Instances returns how many live components are mounted, for metrics.
func Instances() int64 { return liveInstances.Load() }

// ErrDestroyed is returned by an Instance after it is destroyed.
var ErrDestroyed = errors.New("goscript: the component was destroyed")

// ErrPanic is returned by Call when the Func panicked.
var ErrPanic = errors.New("goscript: the component failed")

// StartLive mounts a component for a live connection without waiting:
// Setup runs on the component's own goroutine, and calls made meanwhile
// wait for it. If Setup fails, failed receives the error and the instance
// refuses calls. Tern's live endpoint calls it.
func StartLive(id string, o LiveOptions, failed func(error)) (*Instance, error) {
	setup, ok := registry[id]
	if !ok {
		return nil, fmt.Errorf("goscript: no component %q", id)
	}
	onPanic := func(r any) {
		if o.Panic != nil {
			o.Panic(r, debug.Stack())
		}
	}
	h := hooks{emit: o.Emit, commit: o.Commit, event: o.Event, panic: onPanic}
	c := newComponent(id, o.Props, o.Locals, h, false)
	c.restore = o.Restore
	a := newActor(onPanic)
	c.exec = a
	inst := &Instance{c: c, a: a}
	liveInstances.Add(1)
	inst.submit(func() (err error) {
		defer func() {
			if r := recover(); r != nil {
				onPanic(r)
				err = ErrPanic
			}
			// A failed Setup takes the instance down before anything queued
			// behind it runs.
			if err != nil {
				c.rt.batch(c.destroy)
				if failed != nil {
					failed(err)
				}
			}
		}()
		return runSetup(c, setup)
	}, func(error) {})
	return inst, nil
}

// MountLive mounts a component and waits for its Setup.
func MountLive(ctx context.Context, id string, o LiveOptions) (*Instance, error) {
	res := make(chan error, 1)
	inst, err := StartLive(id, o, func(err error) { res <- err })
	if err != nil {
		return nil, err
	}
	if err := inst.do(ctx, func() error { return nil }); err != nil {
		inst.Destroy()
		select {
		case setupErr := <-res:
			return nil, setupErr
		default:
			return nil, err
		}
	}
	return inst, nil
}

// submit queues fn on the component's goroutine; done receives its error
// there, after the changes fn caused have been emitted.
func (i *Instance) submit(fn func() error, done func(error)) bool {
	return i.a.post(func() {
		if i.c.closed {
			done(ErrDestroyed)
			return
		}
		err := fn()
		i.a.drain()
		done(err)
	})
}

// do runs fn on the component's goroutine and waits for it.
func (i *Instance) do(ctx context.Context, fn func() error) error {
	res := make(chan error, 1)
	if !i.submit(fn, func(err error) { res <- err }) {
		return ErrDestroyed
	}
	select {
	case err := <-res:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-i.a.done:
		select {
		case err := <-res:
			return err
		default:
			return ErrDestroyed
		}
	}
}

func (i *Instance) callFunc(name string, args []byte) (out []byte, err error) {
	defer func() {
		if r := recover(); r != nil {
			i.c.reportPanic(r)
			err = ErrPanic
		}
	}()
	return i.c.call(name, args)
}

// Call runs one of the component's Funcs with JSON arguments, and returns
// its JSON result once the changes it caused have been emitted.
func (i *Instance) Call(ctx context.Context, name string, args []byte) (out []byte, err error) {
	err = i.do(ctx, func() (err error) {
		out, err = i.callFunc(name, args)
		return err
	})
	return out, err
}

// CallAsync queues a call to a Func and returns at once; done receives the
// result on the component's goroutine. Calls run in the order queued.
func (i *Instance) CallAsync(name string, args []byte, done func(out []byte, err error)) {
	var out []byte
	if !i.submit(func() (err error) {
		out, err = i.callFunc(name, args)
		return err
	}, func(err error) { done(out, err) }) {
		done(nil, ErrDestroyed)
	}
}

// Set sets a State declared with Bind to a JSON value from the page.
func (i *Instance) Set(ctx context.Context, name string, value []byte) error {
	return i.do(ctx, func() error { return i.c.bind(name, value) })
}

// SetAsync is Set without waiting; done receives the error.
func (i *Instance) SetAsync(name string, value []byte, done func(error)) {
	if !i.submit(func() error { return i.c.bind(name, value) }, done) {
		done(ErrDestroyed)
	}
}

// SetProps passes new props from the parent.
func (i *Instance) SetProps(ctx context.Context, props []byte) error {
	return i.do(ctx, func() error { return i.c.setProps(props) })
}

// SetPropsAsync is SetProps without waiting.
func (i *Instance) SetPropsAsync(props []byte) {
	i.submit(func() error { return i.c.setProps(props) }, func(error) {})
}

// Destroy stops the component, runs its OnDestroy functions and ends its
// goroutine. It waits for the component's running work to finish.
func (i *Instance) Destroy() {
	if i.DestroyAsync() {
		<-i.a.done
	}
}

// DestroyAsync is Destroy without waiting. It reports whether the
// instance was still running.
func (i *Instance) DestroyAsync() bool {
	return i.a.post(func() {
		if !i.c.closed {
			i.c.rt.batch(i.c.destroy)
		}
		liveInstances.Add(-1)
		i.a.stop()
	})
}

// InitialState runs a component's Setup with props and returns what it
// published, for server rendering. Effects, Do, Go and Every don't run.
func InitialState(id string, props []byte, locals map[string]any) (map[string]json.RawMessage, error) {
	state := map[string]json.RawMessage{}
	c, err := mountComponent(id, props, locals, hooks{emit: func(name string, v []byte) { state[name] = v }}, true)
	if err != nil {
		return nil, err
	}
	c.destroy()
	return state, nil
}

// GlobalSignal is a value shared by every instance of every component in
// this process, such as a counter all visitors see. A component shows it
// with Show or reads it reactively through Use. It is safe for concurrent
// use. Apps running several server processes use [Shared] instead.
type GlobalSignal[T any] struct {
	mu      sync.Mutex
	value   T
	version uint64
	mirrors map[*mirror[T]]bool
}

// mirror is a component's copy of a shared value. ver is only used on the
// component's goroutine, so updates applied out of order are skipped.
type mirror[T any] struct {
	s   *Signal[T]
	ver uint64
}

// Global declares a value shared within this process.
func Global[T any](initial T) *GlobalSignal[T] {
	return &GlobalSignal[T]{value: initial, mirrors: map[*mirror[T]]bool{}}
}

// Get returns the current value. It doesn't subscribe: inside a Derived or
// Effect, read the signal Use or Show returns.
func (g *GlobalSignal[T]) Get() T {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.value
}

// Set changes the value for everyone.
func (g *GlobalSignal[T]) Set(v T) { g.Update(func(T) T { return v }) }

// Update sets the value to fn of the current one, atomically.
func (g *GlobalSignal[T]) Update(fn func(T) T) {
	g.mu.Lock()
	v := fn(g.value)
	if reflect.DeepEqual(g.value, v) {
		g.mu.Unlock()
		return
	}
	g.value = v
	g.version++
	ver := g.version
	mirrors := make([]*mirror[T], 0, len(g.mirrors))
	for m := range g.mirrors {
		mirrors = append(mirrors, m)
	}
	g.mu.Unlock()
	for _, m := range mirrors {
		m.s.c.post(func() {
			if ver > m.ver {
				m.ver = ver
				m.s.Set(v)
			}
		})
	}
}

// Use returns a signal in c that follows the shared value, for Derived
// values and Effects.
func (g *GlobalSignal[T]) Use(c *Component) *Signal[T] { return g.mirror(c, "") }

// Show publishes the shared value in c's markup under name, keeps it up to
// date there, and returns the signal that follows it.
func (g *GlobalSignal[T]) Show(c *Component, name string) *Signal[T] { return g.mirror(c, name) }

func (g *GlobalSignal[T]) mirror(c *Component, name string) *Signal[T] {
	g.mu.Lock()
	s := &Signal[T]{c: c, name: name, value: g.value}
	m := &mirror[T]{s: s, ver: g.version}
	if !c.rendering {
		g.mirrors[m] = true
	}
	g.mu.Unlock()
	c.releases = append(c.releases, func() {
		g.mu.Lock()
		delete(g.mirrors, m)
		g.mu.Unlock()
	})
	c.publish(name, s.value)
	return s
}

// Package goscript is the Go side of Tern's <script lang="go">: component
// logic written in Go with Svelte-style runes, while the markup stays
// Svelte.
//
//	<script lang="go" live>
//	import "github.com/Unfield/Tern/goscript"
//
//	func Setup(c *goscript.Component) {
//		count := goscript.State(c, "count", 0)
//		goscript.Derived(c, "doubled", func() int { return count.Get() * 2 })
//		c.Func("increment", func() { count.Set(count.Get() + 1) })
//	}
//	</script>
//
//	<button onclick={increment}>{count} × 2 = {doubled}</button>
//
// Names given to State, Derived, Show and Func become variables in the
// component's markup.
//
// With live, the component runs on the server and the page talks to it over
// one WebSocket: this is the supported, production mode. Without live, the
// same code is compiled to WebAssembly and runs in the browser; that mode is
// experimental and needs "experimental": {"goScript": true} in tern.json.
// See docs/live-components.md.
package goscript

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"time"

	"github.com/Unfield/Tern/internal/nilslices"
)

// Component is one mounted instance of a component. Its methods, and the
// signals it owns, must be used from the component's own code: Setup, its
// Funcs, Derived values and Effects, and functions passed to Do. Other
// goroutines go through Do.
type Component struct {
	rt        *reactor
	id        string
	props     json.RawMessage
	propsSubs []func(json.RawMessage) error
	restore   map[string]json.RawMessage
	locals    map[string]any
	hooks     hooks
	dirty     map[string][]byte
	order     []string
	funcs     map[string]reflect.Value
	binds     map[string]func([]byte) error
	comps     []*computation
	releases  []func()
	cleanups  []func()
	rendering bool
	closed    bool
	exec      *actor // nil: runs inline (WebAssembly, rendering, tests)
	ctx       context.Context
	cancel    context.CancelFunc
}

// hooks connect a component to whatever shows it.
type hooks struct {
	emit   func(name string, value []byte) // a value changed
	commit func()                          // after the changes of one batch
	event  func(name string, value []byte) // c.Emit
	panic  func(v any)                     // a recovered panic
}

// OnDestroy runs fn when the component goes away: when its page closes or
// navigates, or its live connection ends.
func (c *Component) OnDestroy(fn func()) { c.cleanups = append(c.cleanups, fn) }

// Rendering reports whether Setup runs only to compute the initial state,
// for server rendering or the build. Skip side effects then, such as
// counting a visitor. Do, Go and Every do nothing while rendering.
func (c *Component) Rendering() bool { return c.rendering }

// Context returns a context that is canceled when the component is
// destroyed. Use it for work a Func or Go starts.
func (c *Component) Context() context.Context { return c.ctx }

// Props decodes the component's props, as they were when it mounted, into
// dst. Use [Props] for props that change while the component is shown.
func (c *Component) Props(dst any) error {
	if len(c.props) == 0 {
		return nil
	}
	return json.Unmarshal(c.props, dst)
}

// Props returns the component's props as a signal of T, decoded from what
// the parent passes. It changes when the parent passes new props, so
// Derived values and Effects that read it rerun.
func Props[T any](c *Component) *Signal[T] {
	var v T
	if len(c.props) > 0 {
		_ = json.Unmarshal(c.props, &v)
	}
	s := &Signal[T]{c: c, value: v}
	c.propsSubs = append(c.propsSubs, func(raw json.RawMessage) error {
		var next T
		if err := json.Unmarshal(raw, &next); err != nil {
			return err
		}
		s.Set(next)
		return nil
	})
	return s
}

// setProps applies new props from the parent.
func (c *Component) setProps(raw json.RawMessage) error {
	c.props = raw
	var err error
	c.rt.batch(func() {
		for _, fn := range c.propsSubs {
			if e := fn(raw); e != nil {
				err = e
			}
		}
	})
	return err
}

// Emit sends an event to the component's parent in the page: Emit("saved",
// v) calls the onsaved prop with v. It does nothing while rendering.
func (c *Component) Emit(name string, v any) {
	if c.rendering || c.closed || c.hooks.event == nil {
		return
	}
	b, err := json.Marshal(nilslices.Remove(v))
	if err != nil {
		b = []byte("null")
	}
	c.hooks.event(name, b)
}

// Do runs fn as the component, safely from any goroutine: changes it makes
// to signals reach the page like changes made by a Func. It does nothing
// while rendering or after the component is destroyed.
func (c *Component) Do(fn func()) {
	if c.rendering {
		return
	}
	c.post(fn)
}

// Go runs fn in a new goroutine with the component's context, which is
// canceled when the component is destroyed. fn changes signals through Do.
// It does nothing while rendering.
func (c *Component) Go(fn func(ctx context.Context)) {
	if c.rendering || c.ctx.Err() != nil {
		return
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				c.reportPanic(r)
			}
		}()
		fn(c.ctx)
	}()
}

// Every runs fn as the component every d until it is destroyed, such as to
// refresh a value. It does nothing while rendering.
func (c *Component) Every(d time.Duration, fn func()) {
	c.Go(func(ctx context.Context) {
		t := time.NewTicker(d)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				c.Do(fn)
			}
		}
	})
}

func (c *Component) reportPanic(r any) {
	if c.hooks.panic != nil {
		c.hooks.panic(r)
	}
}

// post runs fn as the component: on its goroutine for a live component,
// right away otherwise.
func (c *Component) post(fn func()) {
	if c.exec != nil {
		c.exec.post(func() {
			if !c.closed {
				c.rt.batch(fn)
			}
		})
		return
	}
	if !c.closed {
		c.rt.batch(fn)
	}
}

func (c *Component) publish(name string, v any) {
	if c.closed || name == "" {
		return
	}
	b, err := json.Marshal(nilslices.Remove(v))
	if err != nil {
		b = []byte("null")
	}
	if _, ok := c.dirty[name]; !ok {
		c.order = append(c.order, name)
	}
	c.dirty[name] = b
	if c.rt.depth == 0 {
		c.flushDirty()
	}
}

// flushDirty hands the values changed in a batch to the page, each once.
func (c *Component) flushDirty() {
	if len(c.order) == 0 {
		return
	}
	order, dirty := c.order, c.dirty
	c.order, c.dirty = nil, map[string][]byte{}
	if c.closed || c.hooks.emit == nil {
		return
	}
	for _, name := range order {
		c.hooks.emit(name, dirty[name])
	}
	if c.hooks.commit != nil {
		c.hooks.commit()
	}
}

// Func makes fn callable from the markup under name, such as
// onclick={increment}. Its arguments are decoded from the JavaScript values
// passed to it; DOM events arrive as an Event. fn may take a
// context.Context first, canceled when the component goes away, and may
// return a value, an error, or both. The value goes back to the page; an
// error made with Fail is shown to it, other errors are logged and reported
// as a generic failure.
func (c *Component) Func(name string, fn any) {
	v := reflect.ValueOf(fn)
	if v.Kind() != reflect.Func {
		panic(fmt.Sprintf("goscript: Func %q needs a function, got %T", name, fn))
	}
	t := v.Type()
	if t.NumOut() > 2 || (t.NumOut() == 2 && t.Out(1) != errorType) {
		panic(fmt.Sprintf("goscript: Func %q must return nothing, a value, an error, or a value and an error", name))
	}
	c.funcs[name] = v
}

var (
	errorType   = reflect.TypeFor[error]()
	contextType = reflect.TypeFor[context.Context]()
)

// Event is a DOM event passed to a Func.
type Event struct {
	Type    string `json:"type"`
	Value   string `json:"value"`   // the target's value, for inputs
	Checked bool   `json:"checked"` // for checkboxes
	Key     string `json:"key"`     // for keyboard events
}

// Error is an error whose message is shown to the page, made by Fail.
type Error struct{ msg string }

func (e *Error) Error() string { return e.msg }

// Fail returns an error for a Func whose message is sent to the page, which
// can show it: the call's promise rejects with it, and the Func's error is
// set to it. Other errors are not shown, so internal details don't leak.
func Fail(format string, args ...any) error { return &Error{msg: fmt.Sprintf(format, args...)} }

// rejection carries Reject's error out of Setup.
type rejection struct{ err *Error }

// Reject refuses to mount the component: Setup stops, nothing reaches the
// page but the message, which live.error shows. Call it in Setup when the
// visitor may not see the component, after checking Locals: a live
// component is reachable by anyone who can open the page's WebSocket, like
// an endpoint.
func (c *Component) Reject(format string, args ...any) {
	panic(rejection{&Error{msg: fmt.Sprintf(format, args...)}})
}

// runSetup runs setup, turning Reject into an error.
func runSetup(c *Component, setup func(*Component)) (err error) {
	defer func() {
		if r := recover(); r != nil {
			rej, ok := r.(rejection)
			if !ok {
				panic(r)
			}
			err = rej.err
		}
	}()
	c.rt.batch(func() { setup(c) })
	return nil
}

// ErrArguments is returned when a Func's arguments can't be decoded.
var ErrArguments = errors.New("goscript: invalid arguments")

// call runs a Func with JSON arguments and returns its JSON result.
func (c *Component) call(name string, argsJSON []byte) ([]byte, error) {
	fn, ok := c.funcs[name]
	if !ok {
		return nil, fmt.Errorf("goscript: no Func named %q", name)
	}
	var raw []json.RawMessage
	if len(argsJSON) > 0 {
		if err := json.Unmarshal(argsJSON, &raw); err != nil {
			return nil, ErrArguments
		}
	}
	t := fn.Type()
	args := make([]reflect.Value, t.NumIn())
	j := 0
	for i := range args {
		if i == 0 && t.In(0) == contextType {
			args[0] = reflect.ValueOf(c.ctx)
			continue
		}
		p := reflect.New(t.In(i))
		if j < len(raw) {
			if err := json.Unmarshal(raw[j], p.Interface()); err != nil {
				return nil, fmt.Errorf("%w: %s: argument %d: %w", ErrArguments, name, j+1, err)
			}
		}
		j++
		args[i] = p.Elem()
	}
	var out []reflect.Value
	c.rt.batch(func() { out = fn.Call(args) })
	var result any
	for _, o := range out {
		if o.Type() == errorType {
			if !o.IsNil() {
				return nil, o.Interface().(error)
			}
			continue
		}
		result = o.Interface()
	}
	return json.Marshal(nilslices.Remove(result))
}

// bind sets a State declared with Bind from the page.
func (c *Component) bind(name string, value []byte) error {
	set, ok := c.binds[name]
	if !ok {
		return fmt.Errorf("goscript: %q is not bindable; declare it with goscript.Bind", name)
	}
	var err error
	c.rt.batch(func() { err = set(value) })
	return err
}

// destroy stops the component's derived values and effects and runs its
// OnDestroy functions.
func (c *Component) destroy() {
	if c.closed {
		return
	}
	for _, comp := range c.comps {
		comp.dispose()
	}
	if !c.rendering {
		for i := len(c.cleanups) - 1; i >= 0; i-- {
			c.rt.batch(c.cleanups[i])
		}
	}
	c.closed = true
	for _, r := range c.releases {
		r()
	}
	if c.cancel != nil {
		c.cancel()
	}
}

// The reactive core: signals, and computations (derived values and
// effects) that track the signals they read and rerun when one changes.
// Changes made in one Func call or Set are applied together, in a batch.
// Each component has its own reactor, so live components run in parallel.

type reactor struct {
	c        *Component
	current  *computation
	depth    int
	queue    []*computation
	flushing bool
}

func (rt *reactor) batch(fn func()) {
	rt.depth++
	defer func() {
		rt.depth--
		if rt.depth == 0 {
			rt.flush()
			if rt.c != nil {
				rt.c.flushDirty()
			}
		}
	}()
	fn()
}

func (rt *reactor) schedule(c *computation) {
	if c.queued || c.disposed {
		return
	}
	c.queued = true
	rt.queue = append(rt.queue, c)
	if rt.depth == 0 {
		rt.batch(func() {})
	}
}

// flush runs queued computations. It is not re-entrant: computations
// queued while flushing run in the same loop.
func (rt *reactor) flush() {
	if rt.flushing {
		return
	}
	rt.flushing = true
	defer func() {
		rt.flushing = false
		rt.queue = rt.queue[:0]
	}()
	for i := 0; i < len(rt.queue); i++ {
		if i > 10000 {
			panic("goscript: a derived value or effect keeps changing itself")
		}
		c := rt.queue[i]
		c.queued = false
		c.execute()
	}
}

type source struct {
	subs map[*computation]bool
}

func (s *source) track(rt *reactor) {
	cur := rt.current
	if cur != nil && !cur.disposed {
		if s.subs == nil {
			s.subs = map[*computation]bool{}
		}
		s.subs[cur] = true
		cur.sources = append(cur.sources, s)
	}
}

func (s *source) notify(rt *reactor) {
	for comp := range s.subs {
		rt.schedule(comp)
	}
}

type computation struct {
	rt       *reactor
	run      func()
	sources  []*source
	disposed bool
	queued   bool
}

func (c *computation) execute() {
	if c.disposed {
		return
	}
	for _, s := range c.sources {
		delete(s.subs, c)
	}
	c.sources = c.sources[:0]
	prev := c.rt.current
	c.rt.current = c
	defer func() { c.rt.current = prev }()
	c.run()
}

func (c *computation) dispose() {
	c.disposed = true
	for _, s := range c.sources {
		delete(s.subs, c)
	}
	c.sources = nil
}

// Signal is a reactive value; see State.
type Signal[T any] struct {
	source
	c     *Component
	name  string
	value T
}

// Option changes how a State behaves.
type Option interface{ option() }

type optBind struct{}
type optRestore struct{}

func (optBind) option()    {}
func (optRestore) option() {}

var (
	// Bind lets the page set the State, with bind:value={live.bind.name}
	// or live.set("name", v). Treat the value like any other user input.
	// A bindable State is also restored when the page reconnects.
	Bind Option = optBind{}
	// Restore keeps the State's value when the page reconnects to a new
	// server process, such as after a deploy: the page sends back the value
	// it shows. Treat it like user input.
	Restore Option = optRestore{}
)

// State declares a reactive value, shown in the markup under name. Reading
// it with Get inside a Derived or Effect subscribes to it.
func State[T any](c *Component, name string, initial T, opts ...Option) *Signal[T] {
	s := &Signal[T]{c: c, name: name, value: initial}
	bind, restore := false, false
	for _, o := range opts {
		switch o.(type) {
		case optBind:
			bind, restore = true, true
		case optRestore:
			restore = true
		}
	}
	if restore && name != "" {
		if raw, ok := c.restore[name]; ok {
			var v T
			if json.Unmarshal(raw, &v) == nil {
				s.value = v
			}
		}
	}
	if bind && name != "" {
		c.binds[name] = func(raw []byte) error {
			var v T
			if err := json.Unmarshal(raw, &v); err != nil {
				return fmt.Errorf("%w: %s: %w", ErrArguments, name, err)
			}
			s.Set(v)
			// Echo the value even when it didn't change, so the page
			// settles on what the server holds.
			c.publish(name, s.value)
			return nil
		}
	}
	c.publish(name, s.value)
	return s
}

// Get returns the value.
func (s *Signal[T]) Get() T {
	s.track(s.c.rt)
	return s.value
}

// Peek returns the value without subscribing to it.
func (s *Signal[T]) Peek() T { return s.value }

// Set changes the value, updating the markup and everything derived from
// it. Treat values as immutable: set a new slice or map rather than
// changing the one a signal holds.
func (s *Signal[T]) Set(v T) {
	if reflect.DeepEqual(s.value, v) {
		return
	}
	s.value = v
	s.c.publish(s.name, v)
	s.c.rt.batch(func() { s.notify(s.c.rt) })
}

// Update sets the value to fn of the current one.
func (s *Signal[T]) Update(fn func(T) T) { s.Set(fn(s.value)) }

// Derived declares a value computed from others, recomputed when they
// change. With a name, it is shown in the markup too.
func Derived[T any](c *Component, name string, fn func() T) *Signal[T] {
	s := &Signal[T]{c: c, name: name}
	comp := &computation{rt: c.rt}
	first := true
	comp.run = func() {
		v := fn()
		if first || !reflect.DeepEqual(s.value, v) {
			first = false
			s.value = v
			s.c.publish(s.name, v)
			s.notify(c.rt)
		}
	}
	c.comps = append(c.comps, comp)
	c.rt.batch(comp.execute)
	return s
}

// Effect runs fn now and again whenever a value it reads changes. It does
// not run while rendering.
func Effect(c *Component, fn func()) {
	if c.rendering {
		return
	}
	comp := &computation{rt: c.rt, run: fn}
	c.comps = append(c.comps, comp)
	c.rt.batch(comp.execute)
}

var registry = map[string]func(*Component){}

// Register makes a component's Setup available; generated code calls it
// from an init function.
func Register(id string, setup func(*Component)) { registry[id] = setup }

// ComponentID returns the ID of the component in the .svelte file at path,
// relative to the app's root, such as "src/lib/Counter.svelte".
func ComponentID(path string) string {
	sum := sha256.Sum256([]byte(filepath.ToSlash(path)))
	return "c" + hex.EncodeToString(sum[:])[:10]
}

// Registered reports whether a component ID is registered.
func Registered(id string) bool {
	_, ok := registry[id]
	return ok
}

// newComponent creates a component without running Setup.
func newComponent(id string, props []byte, locals map[string]any, h hooks, rendering bool) *Component {
	c := &Component{
		id: id, props: props, locals: locals, hooks: h, rendering: rendering,
		funcs: map[string]reflect.Value{}, binds: map[string]func([]byte) error{}, dirty: map[string][]byte{},
	}
	c.rt = &reactor{c: c}
	c.ctx, c.cancel = context.WithCancel(context.Background())
	return c
}

// mount creates a component instance and runs its Setup inline.
func mount(id string, props []byte, emit func(string, []byte)) (*Component, error) {
	return mountComponent(id, props, nil, hooks{emit: emit}, false)
}

func mountComponent(id string, props []byte, locals map[string]any, h hooks, rendering bool) (*Component, error) {
	setup, ok := registry[id]
	if !ok {
		return nil, fmt.Errorf("goscript: no component %q", id)
	}
	c := newComponent(id, props, locals, h, rendering)
	if err := runSetup(c, setup); err != nil {
		c.destroy()
		return nil, err
	}
	return c, nil
}

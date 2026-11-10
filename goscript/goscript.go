// Package goscript is the Go side of Tern's experimental <script lang="go">:
// component logic written in Go, compiled to WebAssembly for the browser,
// with Svelte-style runes.
//
//	<script lang="go">
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
// Names given to State, Derived and Func become variables in the
// component's markup. Setup also runs natively at build time, so server
// rendering and the first paint show the initial state before the
// WebAssembly module loads. Enable it with "experimental": {"goScript":
// true} in tern.json. See docs/experimental-go-script.md.
package goscript

import (
	"encoding/json"
	"fmt"
	"reflect"
)

// Component is one mounted instance of a component.
type Component struct {
	props     json.RawMessage
	locals    map[string]any
	emit      func(name string, value []byte)
	funcs     map[string]reflect.Value
	comps     []*computation
	globals   []releaser
	cleanups  []func()
	rendering bool
	closed    bool
}

// OnDestroy runs fn when the component goes away: when its page closes or
// navigates, or its live connection ends.
func (c *Component) OnDestroy(fn func()) { c.cleanups = append(c.cleanups, fn) }

// Rendering reports whether Setup runs only to compute the initial state,
// for server rendering or the build. Skip side effects then, such as
// counting a visitor.
func (c *Component) Rendering() bool { return c.rendering }

// Props decodes the component's props into dst. Props are read when the
// component mounts.
func (c *Component) Props(dst any) error {
	if len(c.props) == 0 {
		return nil
	}
	return json.Unmarshal(c.props, dst)
}

func (c *Component) publish(name string, v any) {
	if c.closed || c.emit == nil || name == "" {
		return
	}
	b, err := json.Marshal(v)
	if err != nil {
		b = []byte("null")
	}
	c.emit(name, b)
}

// Func makes fn callable from the markup under name, such as
// onclick={increment}. Its arguments are decoded from the JavaScript
// values passed to it; DOM events arrive as an Event.
func (c *Component) Func(name string, fn any) {
	v := reflect.ValueOf(fn)
	if v.Kind() != reflect.Func {
		panic(fmt.Sprintf("goscript: Func %q needs a function, got %T", name, fn))
	}
	c.funcs[name] = v
}

// Event is a DOM event passed to a Func.
type Event struct {
	Type    string `json:"type"`
	Value   string `json:"value"`   // the target's value, for inputs
	Checked bool   `json:"checked"` // for checkboxes
	Key     string `json:"key"`     // for keyboard events
}

// call runs a Func with JSON arguments and returns its JSON result.
func (c *Component) call(name string, argsJSON []byte) ([]byte, error) {
	fn, ok := c.funcs[name]
	if !ok {
		return nil, fmt.Errorf("goscript: no Func named %q", name)
	}
	var raw []json.RawMessage
	_ = json.Unmarshal(argsJSON, &raw)
	t := fn.Type()
	args := make([]reflect.Value, t.NumIn())
	for i := range args {
		p := reflect.New(t.In(i))
		if i < len(raw) {
			if err := json.Unmarshal(raw[i], p.Interface()); err != nil {
				return nil, fmt.Errorf("goscript: %s: argument %d: %w", name, i+1, err)
			}
		}
		args[i] = p.Elem()
	}
	var out []reflect.Value
	batch(func() { out = fn.Call(args) })
	if len(out) == 0 {
		return []byte("null"), nil
	}
	return json.Marshal(out[0].Interface())
}

// destroy stops the component's derived values and effects.
func (c *Component) destroy() {
	if c.closed {
		return
	}
	c.closed = true
	for _, comp := range c.comps {
		comp.dispose()
	}
	if !c.rendering {
		for i := len(c.cleanups) - 1; i >= 0; i-- {
			batch(c.cleanups[i])
		}
	}
}

// The reactive core: signals, and computations (derived values and
// effects) that track the signals they read and rerun when one changes.
// Changes made in one Func call or Set are applied together, in batch.

type source struct {
	subs map[*computation]bool
}

func (s *source) track() {
	if current != nil && !current.disposed {
		if s.subs == nil {
			s.subs = map[*computation]bool{}
		}
		s.subs[current] = true
		current.sources = append(current.sources, s)
	}
}

func (s *source) notify() {
	for comp := range s.subs {
		schedule(comp)
	}
}

type computation struct {
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
	prev := current
	current = c
	defer func() { current = prev }()
	c.run()
}

func (c *computation) dispose() {
	c.disposed = true
	for _, s := range c.sources {
		delete(s.subs, c)
	}
	c.sources = nil
}

var (
	current  *computation
	depth    int
	queue    []*computation
	flushing bool
)

func schedule(c *computation) {
	if c.queued || c.disposed {
		return
	}
	c.queued = true
	queue = append(queue, c)
	if depth == 0 {
		flush()
	}
}

func batch(fn func()) {
	depth++
	defer func() {
		depth--
		if depth == 0 {
			flush()
		}
	}()
	fn()
}

// flush runs queued computations. It is not re-entrant: computations
// queued while flushing run in the same loop.
func flush() {
	if flushing {
		return
	}
	flushing = true
	defer func() {
		flushing = false
		queue = queue[:0]
	}()
	for i := 0; i < len(queue); i++ {
		if i > 10000 {
			panic("goscript: a derived value or effect keeps changing itself")
		}
		c := queue[i]
		c.queued = false
		c.execute()
	}
}

// Signal is a reactive value; see State.
type Signal[T any] struct {
	source
	c     *Component
	name  string
	value T
}

// State declares a reactive value, shown in the markup under name. Reading
// it with Get inside a Derived or Effect subscribes to it.
func State[T any](c *Component, name string, initial T) *Signal[T] {
	s := &Signal[T]{c: c, name: name, value: initial}
	c.publish(name, initial)
	return s
}

// Get returns the value.
func (s *Signal[T]) Get() T {
	s.track()
	return s.value
}

// Peek returns the value without subscribing to it.
func (s *Signal[T]) Peek() T { return s.value }

// Set changes the value, updating the markup and everything derived from it.
func (s *Signal[T]) Set(v T) {
	if reflect.DeepEqual(s.value, v) {
		return
	}
	s.value = v
	s.c.publish(s.name, v)
	batch(s.notify)
}

// Update sets the value to fn of the current one.
func (s *Signal[T]) Update(fn func(T) T) { s.Set(fn(s.value)) }

// Derived declares a value computed from others, recomputed when they
// change. With a name, it is shown in the markup too.
func Derived[T any](c *Component, name string, fn func() T) *Signal[T] {
	s := &Signal[T]{c: c, name: name}
	comp := &computation{}
	first := true
	comp.run = func() {
		v := fn()
		if first || !reflect.DeepEqual(s.value, v) {
			first = false
			s.value = v
			s.c.publish(s.name, v)
			s.notify()
		}
	}
	c.comps = append(c.comps, comp)
	batch(comp.execute)
	return s
}

// Effect runs fn now and again whenever a value it reads changes.
func Effect(c *Component, fn func()) {
	comp := &computation{run: fn}
	c.comps = append(c.comps, comp)
	batch(comp.execute)
}

var registry = map[string]func(*Component){}

// Register makes a component's Setup available; generated code calls it.
func Register(id string, setup func(*Component)) { registry[id] = setup }

// mount creates a component instance.
func mount(id string, props []byte, emit func(string, []byte)) (*Component, error) {
	return mountWith(id, props, nil, emit)
}

func mountWith(id string, props []byte, locals map[string]any, emit func(string, []byte)) (*Component, error) {
	return mountComponent(id, props, locals, emit, false)
}

func mountComponent(id string, props []byte, locals map[string]any, emit func(string, []byte), rendering bool) (*Component, error) {
	setup, ok := registry[id]
	if !ok {
		return nil, fmt.Errorf("goscript: no component %q", id)
	}
	c := &Component{props: props, locals: locals, emit: emit, funcs: map[string]reflect.Value{}, rendering: rendering}
	batch(func() { setup(c) })
	return c, nil
}

package goscript

import (
	"encoding/json"
	"reflect"
	"sync"
)

// The reactive core keeps its bookkeeping in package variables, so live
// components (many connections at once on the server) run one at a time.
var liveMu sync.Mutex

// Locals returns request values a live component was mounted with, such
// as the signed-in user from ctx.Locals; nil in the browser.
func (c *Component) Locals() map[string]any { return c.locals }

// Instance is a mounted live component.
type Instance struct{ c *Component }

// MountLive mounts a component on the server for a live connection; emit
// receives every value it publishes. Tern's live endpoint calls it.
func MountLive(id string, props []byte, locals map[string]any, emit func(name string, value []byte)) (*Instance, error) {
	liveMu.Lock()
	defer liveMu.Unlock()
	c, err := mountWith(id, props, locals, emit)
	if err != nil {
		return nil, err
	}
	return &Instance{c: c}, nil
}

// Call runs one of the component's Funcs with JSON arguments.
func (i *Instance) Call(name string, args []byte) ([]byte, error) {
	liveMu.Lock()
	defer liveMu.Unlock()
	return i.c.call(name, args)
}

// Destroy stops the component.
func (i *Instance) Destroy() {
	liveMu.Lock()
	defer liveMu.Unlock()
	i.c.destroy()
	for _, g := range i.c.globals {
		g.release(i.c)
	}
}

// InitialState runs a component's Setup with props and returns what it
// published, for server rendering.
func InitialState(id string, props []byte, locals map[string]any) (map[string]json.RawMessage, error) {
	liveMu.Lock()
	defer liveMu.Unlock()
	state := map[string]json.RawMessage{}
	c, err := mountComponent(id, props, locals, func(name string, v []byte) { state[name] = v }, true)
	if err != nil {
		return nil, err
	}
	c.destroy()
	for _, g := range c.globals {
		g.release(c)
	}
	return state, nil
}

// Registered reports whether a component ID is registered.
func Registered(id string) bool {
	_, ok := registry[id]
	return ok
}

// GlobalSignal is a value shared by every instance of every component, such
// as a counter all visitors see. Shown in a component's markup with Show.
// It is meant for live components, which run on the server and so share
// memory; in the browser each page has its own.
type GlobalSignal[T any] struct {
	source
	value T
	shown map[*Component]string // components showing it, by name
}

type releaser interface{ release(c *Component) }

// Global declares a shared value.
func Global[T any](initial T) *GlobalSignal[T] { return &GlobalSignal[T]{value: initial} }

// Get returns the value, subscribing the running Derived or Effect to it.
func (g *GlobalSignal[T]) Get() T {
	g.track()
	return g.value
}

// Set changes the value for everyone.
func (g *GlobalSignal[T]) Set(v T) {
	if reflect.DeepEqual(g.value, v) {
		return
	}
	g.value = v
	for c, name := range g.shown {
		c.publish(name, v)
	}
	batch(g.notify)
}

// Update sets the value to fn of the current one.
func (g *GlobalSignal[T]) Update(fn func(T) T) { g.Set(fn(g.value)) }

// Show publishes the shared value in c's markup under name, and keeps it
// up to date there.
func (g *GlobalSignal[T]) Show(c *Component, name string) {
	if g.shown == nil {
		g.shown = map[*Component]string{}
	}
	g.shown[c] = name
	c.globals = append(c.globals, g)
	c.publish(name, g.value)
}

func (g *GlobalSignal[T]) release(c *Component) { delete(g.shown, c) }

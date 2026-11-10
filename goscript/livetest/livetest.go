// Package livetest runs live components in Go tests, without a browser:
//
//	func TestCounter(t *testing.T) {
//		c := livetest.Mount(t, "src/lib/Counter.svelte", map[string]any{"start": 2})
//		if _, err := c.Call("increment"); err != nil {
//			t.Fatal(err)
//		}
//		if got := livetest.Get[int](c, "count"); got != 3 {
//			t.Errorf("count = %d", got)
//		}
//	}
//
// Write the test in the app's main package, next to tern.gen.go, which
// registers the app's components.
package livetest

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Unfield/Tern/goscript"
)

// Event is a value a component passed to Emit.
type Event struct {
	Name  string
	Value json.RawMessage
}

// Component is a mounted live component.
type Component struct {
	t      testing.TB
	inst   *goscript.Instance
	mu     sync.Mutex
	state  map[string]json.RawMessage
	events []Event
	panics []any
}

// Option configures Mount.
type Option func(*goscript.LiveOptions)

// WithLocals sets the request values the component reads with Locals,
// such as the signed-in user.
func WithLocals(locals map[string]any) Option {
	return func(o *goscript.LiveOptions) { o.Locals = locals }
}

// WithRestore mounts the component as if the page reconnected showing
// these values, for State declared with Bind or Restore.
func WithRestore(values map[string]any) Option {
	return func(o *goscript.LiveOptions) {
		o.Restore = map[string]json.RawMessage{}
		for k, v := range values {
			b, _ := json.Marshal(v)
			o.Restore[k] = b
		}
	}
}

// Mount mounts a component, named by its .svelte path relative to the app
// root (or its ID), with props, and destroys it when the test ends. A
// panic in the component fails the test.
func Mount(t testing.TB, component string, props any, opts ...Option) *Component {
	t.Helper()
	id := component
	if strings.HasSuffix(component, ".svelte") {
		id = goscript.ComponentID(component)
	}
	raw, err := json.Marshal(props)
	if err != nil {
		t.Fatalf("livetest: props: %v", err)
	}
	c := &Component{t: t, state: map[string]json.RawMessage{}}
	o := goscript.LiveOptions{
		Props: raw,
		Emit: func(name string, v []byte) {
			c.mu.Lock()
			c.state[name] = v
			c.mu.Unlock()
		},
		Event: func(name string, v []byte) {
			c.mu.Lock()
			c.events = append(c.events, Event{name, v})
			c.mu.Unlock()
		},
		Panic: func(v any, stack []byte) {
			c.mu.Lock()
			c.panics = append(c.panics, v)
			c.mu.Unlock()
			t.Errorf("livetest: %s panicked: %v\n%s", component, v, stack)
		},
	}
	for _, opt := range opts {
		opt(&o)
	}
	if !goscript.Registered(id) {
		t.Fatalf("livetest: no live component %s; is the test in the app's main package, and was tern gen run?", component)
	}
	inst, err := goscript.MountLive(context.Background(), id, o)
	if err != nil {
		t.Fatalf("livetest: mounting %s: %v", component, err)
	}
	c.inst = inst
	t.Cleanup(inst.Destroy)
	return c
}

func encodeArgs(args []any) []byte {
	if args == nil {
		args = []any{}
	}
	b, _ := json.Marshal(args)
	return b
}

// Call runs a Func with args and returns its JSON result. By the time it
// returns, the state the call changed is visible to State and Get.
func (c *Component) Call(name string, args ...any) (json.RawMessage, error) {
	return c.inst.Call(context.Background(), name, encodeArgs(args))
}

// Set sets a State declared with Bind, as bind:value does.
func (c *Component) Set(name string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return c.inst.Set(context.Background(), name, b)
}

// SetProps passes new props, as a parent re-rendering does.
func (c *Component) SetProps(props any) error {
	b, err := json.Marshal(props)
	if err != nil {
		return err
	}
	return c.inst.SetProps(context.Background(), b)
}

// State returns the JSON value the page shows for name, or nil.
func (c *Component) State(name string) json.RawMessage {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state[name]
}

// Events returns the events the component emitted so far.
func (c *Component) Events() []Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]Event(nil), c.events...)
}

// Eventually waits up to a second for cond, for changes that arrive from
// other goroutines, such as Every, Go or another component's Global. It
// fails the test on timeout.
func (c *Component) Eventually(cond func() bool) {
	c.t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	c.t.Fatal("livetest: condition not met within a second")
}

// Destroy destroys the component now, running its OnDestroy functions.
func (c *Component) Destroy() { c.inst.Destroy() }

// Get decodes the value the page shows for name into T.
func Get[T any](c *Component, name string) T {
	c.t.Helper()
	var v T
	raw := c.State(name)
	if raw == nil {
		c.t.Fatalf("livetest: no state named %q", name)
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		c.t.Fatalf("livetest: state %q: %v", name, err)
	}
	return v
}

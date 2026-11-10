package kit

import (
	"context"
	"encoding/json"
	"maps"
	"sync"

	"github.com/Unfield/Tern/goscript"
)

// LivePath is the WebSocket that live Go components (<script lang="go"
// live>) talk to.
const LivePath = "/_tern/live"

type liveLocalsKey struct{}

// liveMessage is one message on the live socket, in either direction.
type liveMessage struct {
	Op    string          `json:"op"`              // mount, call, unmount; state, result, error
	Ref   int             `json:"ref"`             // the component instance, chosen by the browser
	ID    string          `json:"id,omitempty"`    // the component, for mount
	Props json.RawMessage `json:"props,omitempty"` // for mount
	Name  string          `json:"name,omitempty"`  // the Func or the state's name
	Args  json.RawMessage `json:"args,omitempty"`  // for call
	Seq   int             `json:"seq,omitempty"`   // matches a call to its result
	Value json.RawMessage `json:"value,omitempty"` // state values and results
	Error string          `json:"error,omitempty"`
}

// serveLive runs the live components of one page over a WebSocket. Every
// instance runs in this process; their state changes are sent as they
// happen, and calls are answered in order.
func (a *App) serveLive(c *Ctx) error {
	locals := maps.Clone(c.Locals)
	return c.WebSocket(nil, func(ctx context.Context, ws *WebSocket) error {
		out := make(chan liveMessage, 512)
		done := make(chan struct{})
		var wg sync.WaitGroup
		wg.Go(func() {
			for {
				select {
				case <-done:
					return
				case m := <-out:
					if ws.Send(m) != nil {
						return
					}
				}
			}
		})
		send := func(m liveMessage) {
			select {
			case out <- m:
			case <-done:
			}
		}
		instances := map[int]*goscript.Instance{}
		defer func() {
			for _, inst := range instances {
				inst.Destroy()
			}
			close(done)
			wg.Wait()
		}()
		for {
			var m liveMessage
			if err := ws.Receive(&m); err != nil {
				return err
			}
			switch m.Op {
			case "mount":
				if old := instances[m.Ref]; old != nil {
					old.Destroy()
				}
				ref := m.Ref
				inst, err := goscript.MountLive(m.ID, m.Props, locals, func(name string, v []byte) {
					send(liveMessage{Op: "state", Ref: ref, Name: name, Value: v})
				})
				if err != nil {
					send(liveMessage{Op: "error", Ref: ref, Error: err.Error()})
					continue
				}
				instances[ref] = inst
			case "call":
				inst := instances[m.Ref]
				if inst == nil {
					send(liveMessage{Op: "result", Ref: m.Ref, Seq: m.Seq, Error: "not mounted"})
					continue
				}
				v, err := a.callLive(inst, m.Name, m.Args)
				res := liveMessage{Op: "result", Ref: m.Ref, Seq: m.Seq, Value: v}
				if err != nil {
					res.Error, res.Value = err.Error(), nil
				}
				send(res)
			case "unmount":
				if inst := instances[m.Ref]; inst != nil {
					inst.Destroy()
					delete(instances, m.Ref)
				}
			}
		}
	})
}

// callLive runs a Func, turning a panic into an error so one component
// can't take the connection down.
func (a *App) callLive(inst *goscript.Instance, name string, args json.RawMessage) (out json.RawMessage, err error) {
	defer func() {
		if r := recover(); r != nil {
			a.logger.Error("kit: a live component panicked", "func", name, "panic", r)
			err = ErrLivePanic
		}
	}()
	return inst.Call(name, args)
}

// ErrLivePanic is reported to the browser when a live Func panics.
var ErrLivePanic = liveError("the component failed")

type liveError string

func (e liveError) Error() string { return string(e) }

// liveInitial renders a live component's initial state during server
// rendering, called from the bundle as __tern_live_initial(id, props).
func liveInitial(ctx context.Context, args []any) (any, error) {
	if len(args) < 2 {
		return "{}", nil
	}
	id, _ := args[0].(string)
	props, _ := args[1].(string)
	locals, _ := ctx.Value(liveLocalsKey{}).(map[string]any)
	state, err := goscript.InitialState(id, []byte(props), locals)
	if err != nil {
		return "{}", nil //nolint:nilerr // render with empty state; the socket reports the error
	}
	b, _ := json.Marshal(state)
	return string(b), nil
}

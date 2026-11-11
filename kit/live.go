package kit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"sync"
	"time"

	"github.com/Unfield/Tern/report"

	"github.com/coder/websocket"

	"github.com/Unfield/Tern/goscript"
	"github.com/Unfield/Tern/internal/jsondiff"
	"github.com/Unfield/Tern/internal/jsonenc"
)

// LivePath is the WebSocket that live Go components (<script lang="go"
// live>) talk to.
const LivePath = "/_tern/live"

// LiveOptions tune live components; see [WithLive].
type LiveOptions struct {
	// MaxInstances caps the components one page may mount at once. The
	// default is 256.
	MaxInstances int
	// Rate is how many calls and bound values per second one page may
	// send, with bursts of twice that. The default is 50.
	Rate float64
	// SendBuffer is how many messages may wait for a slow browser before
	// the connection is closed; the page reconnects and starts fresh. The
	// default is 256.
	SendBuffer int
	// PingInterval keeps idle connections open through proxies. The
	// default is 25 seconds.
	PingInterval time.Duration
	// Origins lists other origins allowed to connect, as for
	// [WebSocketOptions].
	Origins []string
}

// WithLive tunes live components.
func WithLive(o LiveOptions) Option { return func(opts *options) { opts.live = o } }

// WithLiveBroker connects live components in every process of the app
// through b, so goscript.Shared values, goscript.Presence counts and
// goscript.Publish reach all of them. Any realtime.Broker works, such as
// the PostgreSQL one in realtime/pgbroker.
func WithLiveBroker(b goscript.Broker) Option {
	return func(opts *options) { opts.liveBroker = b }
}

func (o LiveOptions) withDefaults() LiveOptions {
	if o.MaxInstances <= 0 {
		o.MaxInstances = 256
	}
	if o.Rate <= 0 {
		o.Rate = 50
	}
	if o.SendBuffer <= 0 {
		o.SendBuffer = 256
	}
	if o.PingInterval <= 0 {
		o.PingInterval = 25 * time.Second
	}
	return o
}

type liveLocalsKey struct{}

// liveIn is a message from the page.
type liveIn struct {
	Op      string                     `json:"op"`  // mount, props, call, set, unmount
	Ref     int                        `json:"ref"` // the component instance, chosen by the page
	ID      string                     `json:"id,omitempty"`
	Props   json.RawMessage            `json:"props,omitempty"`
	Restore map[string]json.RawMessage `json:"restore,omitempty"`
	Name    string                     `json:"name,omitempty"`
	Args    json.RawMessage            `json:"args,omitempty"`
	Value   json.RawMessage            `json:"value,omitempty"`
	Seq     int                        `json:"seq,omitempty"`
}

// liveOut is a message to the page.
type liveOut struct {
	Op    string                     `json:"op"` // state, result, event, error
	Ref   int                        `json:"ref"`
	Set   map[string]json.RawMessage `json:"set,omitempty"`
	Patch map[string]json.RawMessage `json:"patch,omitempty"`
	Seq   int                        `json:"seq,omitempty"`
	Name  string                     `json:"name,omitempty"`
	Value json.RawMessage            `json:"value,omitempty"`
	Error string                     `json:"error,omitempty"`
}

// liveView tracks what one instance's page has, to send patches. It is
// only used on the instance's goroutine.
type liveView struct {
	ref     int
	sent    map[string]any // decoded values the page has
	pending map[string][]byte
	order   []string
}

func (v *liveView) emit(name string, value []byte) {
	if _, ok := v.pending[name]; !ok {
		v.order = append(v.order, name)
	}
	v.pending[name] = value
}

// commit builds one state message from the values changed in a batch,
// sending a patch instead of the value when it is smaller.
func (v *liveView) commit() *liveOut {
	if len(v.order) == 0 {
		return nil
	}
	m := &liveOut{Op: "state", Ref: v.ref}
	for _, name := range v.order {
		raw := v.pending[name]
		next, err := jsondiff.Decode(raw)
		prev, had := v.sent[name]
		if err == nil && had {
			if p, err := json.Marshal(jsondiff.Diff(prev, next)); err == nil && len(p) < len(raw) {
				if m.Patch == nil {
					m.Patch = map[string]json.RawMessage{}
				}
				m.Patch[name] = p
				v.sent[name] = next
				continue
			}
		}
		if m.Set == nil {
			m.Set = map[string]json.RawMessage{}
		}
		m.Set[name] = raw
		if err == nil {
			v.sent[name] = next
		}
	}
	v.pending, v.order = map[string][]byte{}, nil
	return m
}

// liveConn is one page's connection.
type liveConn struct {
	a      *App
	o      LiveOptions
	out    chan *liveOut
	ctx    context.Context
	cancel context.CancelFunc

	mu        sync.Mutex
	instances map[int]*goscript.Instance

	tokens float64
	last   time.Time
}

// send queues m for the page. When the page reads too slowly, the
// connection is closed; it reconnects and remounts with fresh state.
func (lc *liveConn) send(m *liveOut) {
	select {
	case lc.out <- m:
	case <-lc.ctx.Done():
	default:
		lc.a.logger.Warn("kit: closing a live connection that reads too slowly")
		lc.cancel()
	}
}

// allow takes a token for a call or a bound value.
func (lc *liveConn) allow() bool {
	now := time.Now()
	if lc.last.IsZero() {
		lc.tokens = 2 * lc.o.Rate
	} else {
		lc.tokens = min(2*lc.o.Rate, lc.tokens+now.Sub(lc.last).Seconds()*lc.o.Rate)
	}
	lc.last = now
	if lc.tokens < 1 {
		return false
	}
	lc.tokens--
	return true
}

// errorMessage is what the page sees for a failed call: Fail messages as
// they are, other errors generically, so internal details don't leak.
func (lc *liveConn) errorMessage(name string, err error) string {
	var fe *goscript.Error
	switch {
	case errors.As(err, &fe):
		return fe.Error()
	case errors.Is(err, goscript.ErrArguments):
		return "invalid arguments"
	case errors.Is(err, goscript.ErrDestroyed):
		return "not mounted"
	case errors.Is(err, goscript.ErrPanic):
		return goscript.ErrPanic.Error()
	default:
		lc.a.logger.Error("kit: a live component's Func failed", "func", name, "err", err)
		return "the component failed"
	}
}

// serveLive runs the live components of one page over a WebSocket. Every
// instance runs on its own goroutine; state changes are sent as they
// happen, and calls to one instance are answered in order.
func (a *App) serveLive(c *Ctx) error {
	locals := maps.Clone(c.Locals)
	o := a.opts.live.withDefaults()
	return c.WebSocket(&WebSocketOptions{Origins: o.Origins}, func(ctx context.Context, ws *WebSocket) error {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		lc := &liveConn{a: a, o: o, out: make(chan *liveOut, o.SendBuffer), ctx: ctx, cancel: cancel, instances: map[int]*goscript.Instance{}}

		var wg sync.WaitGroup
		wg.Go(func() {
			for {
				select {
				case <-ctx.Done():
					return
				case m := <-lc.out:
					if ws.Send(m) != nil {
						cancel()
						return
					}
				}
			}
		})
		wg.Go(func() {
			t := time.NewTicker(o.PingInterval)
			defer t.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-t.C:
					pctx, done := context.WithTimeout(ctx, o.PingInterval)
					err := ws.Conn().Ping(pctx)
					done()
					if err != nil {
						cancel()
						return
					}
				}
			}
		})
		// A canceled context (slow reader, failed ping) ends the read too.
		stop := context.AfterFunc(ctx, func() { _ = ws.Conn().Close(websocket.StatusGoingAway, "") })
		defer func() {
			stop()
			lc.mu.Lock()
			insts := maps.Clone(lc.instances)
			lc.instances = nil
			lc.mu.Unlock()
			for _, inst := range insts {
				inst.Destroy()
			}
			cancel()
			wg.Wait()
		}()

		for {
			var m liveIn
			if err := ws.Receive(&m); err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return err
			}
			lc.handle(m, locals)
		}
	})
}

func (lc *liveConn) instance(ref int) *goscript.Instance {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	return lc.instances[ref]
}

func (lc *liveConn) handle(m liveIn, locals map[string]any) {
	switch m.Op {
	case "mount":
		lc.mount(m, locals)
	case "props":
		if inst := lc.instance(m.Ref); inst != nil {
			inst.SetPropsAsync(m.Props)
		}
	case "call":
		inst := lc.instance(m.Ref)
		switch {
		case inst == nil:
			lc.send(&liveOut{Op: "result", Ref: m.Ref, Seq: m.Seq, Error: "not mounted"})
		case !lc.allow():
			lc.send(&liveOut{Op: "result", Ref: m.Ref, Seq: m.Seq, Error: "too many calls; slow down"})
		default:
			inst.CallAsync(m.Name, m.Args, func(v []byte, err error) {
				res := &liveOut{Op: "result", Ref: m.Ref, Seq: m.Seq, Value: v}
				if err != nil {
					res.Value, res.Error = nil, lc.errorMessage(m.Name, err)
				}
				lc.send(res)
			})
		}
	case "set":
		inst := lc.instance(m.Ref)
		switch {
		case inst == nil:
			lc.send(&liveOut{Op: "result", Ref: m.Ref, Seq: m.Seq, Error: "not mounted"})
		case !lc.allow():
			lc.send(&liveOut{Op: "result", Ref: m.Ref, Seq: m.Seq, Error: "too many updates; slow down"})
		default:
			inst.SetAsync(m.Name, m.Value, func(err error) {
				res := &liveOut{Op: "result", Ref: m.Ref, Seq: m.Seq}
				if err != nil {
					res.Error = err.Error()
				}
				lc.send(res)
			})
		}
	case "unmount":
		lc.mu.Lock()
		inst := lc.instances[m.Ref]
		delete(lc.instances, m.Ref)
		lc.mu.Unlock()
		if inst != nil {
			inst.DestroyAsync()
		}
	}
}

func (lc *liveConn) mount(m liveIn, locals map[string]any) {
	lc.mu.Lock()
	old := lc.instances[m.Ref]
	count := len(lc.instances)
	lc.mu.Unlock()
	if old != nil {
		old.DestroyAsync()
	} else if count >= lc.o.MaxInstances {
		lc.send(&liveOut{Op: "error", Ref: m.Ref, Error: "too many live components on this page"})
		return
	}
	ref := m.Ref
	view := &liveView{ref: ref, sent: map[string]any{}, pending: map[string][]byte{}}
	inst, err := goscript.StartLive(m.ID, goscript.LiveOptions{
		Props:   m.Props,
		Restore: m.Restore,
		Locals:  locals,
		Emit:    view.emit,
		Commit: func() {
			if out := view.commit(); out != nil {
				lc.send(out)
			}
		},
		Event: func(name string, v []byte) {
			lc.send(&liveOut{Op: "event", Ref: ref, Name: name, Value: v})
		},
		Panic: func(v any, stack []byte) {
			lc.a.logger.Error("kit: a live component panicked", "component", m.ID, "panic", v, "stack", string(stack))
			if r := lc.a.reporter(); r != nil {
				r.Send(report.Report{Source: "server", Type: "panic", Panic: true, Message: fmt.Sprint(v), Route: "live " + m.ID})
			}
		},
	}, func(err error) {
		lc.send(&liveOut{Op: "error", Ref: ref, Error: lc.errorMessage("Setup", err)})
	})
	if err != nil {
		lc.send(&liveOut{Op: "error", Ref: ref, Error: "unknown component"})
		return
	}
	lc.mu.Lock()
	if lc.instances == nil { // the connection is closing
		lc.mu.Unlock()
		inst.Destroy()
		return
	}
	lc.instances[ref] = inst
	lc.mu.Unlock()
}

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
		// The page shows live.error instead of the component.
		msg := "the component failed"
		var fe *goscript.Error
		if errors.As(err, &fe) {
			msg = fe.Error()
		}
		b, _ := json.Marshal(map[string]string{"__tern_error": msg})
		return string(b), nil
	}
	b, _ := jsonenc.Marshal(state)
	return string(b), nil
}

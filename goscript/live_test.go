package goscript

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// recorder collects what a live instance emits.
type recorder struct {
	mu      sync.Mutex
	state   map[string]string
	commits int
	events  []string
	panics  []string
}

func newRecorder() *recorder { return &recorder{state: map[string]string{}} }

func (r *recorder) opts(props string, locals map[string]any) LiveOptions {
	return LiveOptions{
		Props:  json.RawMessage(props),
		Locals: locals,
		Emit: func(name string, v []byte) {
			r.mu.Lock()
			r.state[name] = string(v)
			r.mu.Unlock()
		},
		Commit: func() { r.mu.Lock(); r.commits++; r.mu.Unlock() },
		Event: func(name string, v []byte) {
			r.mu.Lock()
			r.events = append(r.events, name+"="+string(v))
			r.mu.Unlock()
		},
		Panic: func(v any, _ []byte) { r.mu.Lock(); r.panics = append(r.panics, fmt.Sprint(v)); r.mu.Unlock() },
	}
}

func (r *recorder) get(name string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.state[name]
}

// eventually waits for cond, for values that arrive from other goroutines.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for range 200 {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func mustMount(t *testing.T, id string, o LiveOptions) *Instance {
	t.Helper()
	inst, err := MountLive(context.Background(), id, o)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(inst.Destroy)
	return inst
}

func TestLiveAndGlobals(t *testing.T) {
	visits := Global(0)
	Register("shared", func(c *Component) {
		v := visits.Show(c, "visits")
		user, _ := c.Locals()["user"].(string)
		Derived(c, "greeting", func() string { return user + " sees " + jsonString(v.Get()) })
		c.Func("visit", func() { visits.Update(func(n int) int { return n + 1 }) })
	})
	a, b := newRecorder(), newRecorder()
	ia := mustMount(t, "shared", a.opts("", map[string]any{"user": "ada"}))
	ib, err := MountLive(context.Background(), "shared", b.opts("", map[string]any{"user": "bob"}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ia.Call(context.Background(), "visit", []byte("[]")); err != nil {
		t.Fatal(err)
	}
	// The caller's own update arrives before the call returns.
	if a.get("visits") != "1" || a.get("greeting") != `"ada sees 1"` {
		t.Errorf("ada after her visit: %v", a.state)
	}
	eventually(t, "bob to see the visit", func() bool { return b.get("greeting") == `"bob sees 1"` })
	ib.Destroy()
	if _, err := ib.Call(context.Background(), "visit", nil); !errors.Is(err, ErrDestroyed) {
		t.Errorf("call after destroy = %v", err)
	}
	_, _ = ia.Call(context.Background(), "visit", []byte("[]"))
	if a.get("visits") != "2" || b.get("visits") != "1" {
		t.Errorf("after bob left: a=%v b=%v", a.state, b.state)
	}
	state, err := InitialState("shared", nil, map[string]any{"user": "cy"})
	if err != nil || string(state["greeting"]) != `"cy sees 2"` {
		t.Errorf("InitialState = %v, %v", state, err)
	}
	if Registered("nope") || !Registered("shared") {
		t.Error("Registered")
	}
}

func TestLiveFuncs(t *testing.T) {
	Register("funcs", func(c *Component) {
		n := State(c, "n", 0)
		c.Func("add", func(ctx context.Context, by int) (int, error) {
			if ctx.Err() != nil {
				return 0, ctx.Err()
			}
			if by < 0 {
				return 0, Fail("can't add %d", by)
			}
			n.Update(func(v int) int { return v + by })
			return n.Peek(), nil
		})
		c.Func("secret", func() error { return errors.New("db password wrong") })
		c.Func("boom", func() { panic("kaboom") })
	})
	r := newRecorder()
	inst := mustMount(t, "funcs", r.opts("", nil))
	ctx := context.Background()
	if out, err := inst.Call(ctx, "add", []byte("[2]")); err != nil || string(out) != "2" || r.get("n") != "2" {
		t.Errorf("add = %s, %v, n=%s", out, err, r.get("n"))
	}
	var fe *Error
	if _, err := inst.Call(ctx, "add", []byte("[-1]")); !errors.As(err, &fe) || fe.Error() != "can't add -1" {
		t.Errorf("Fail = %v", err)
	}
	if _, err := inst.Call(ctx, "secret", nil); err == nil || errors.As(err, &fe) {
		t.Errorf("plain errors are not Fail errors: %v", err)
	}
	if _, err := inst.Call(ctx, "add", []byte(`["x"]`)); !errors.Is(err, ErrArguments) {
		t.Errorf("bad args = %v", err)
	}
	if _, err := inst.Call(ctx, "boom", nil); !errors.Is(err, ErrPanic) || len(r.panics) != 1 {
		t.Errorf("panic = %v, %v", err, r.panics)
	}
	// The instance keeps working after a panic.
	if out, _ := inst.Call(ctx, "add", []byte("[1]")); string(out) != "3" {
		t.Errorf("after panic: %s", out)
	}
	if _, err := MountLive(ctx, "missing", r.opts("", nil)); err == nil {
		t.Error("mounting an unknown component should fail")
	}
	Register("private", func(c *Component) {
		if c.Locals()["user"] == nil {
			c.Reject("sign in first")
		}
		State(c, "secret", 42)
	})
	var fe2 *Error
	if _, err := MountLive(ctx, "private", r.opts("", nil)); !errors.As(err, &fe2) || fe2.Error() != "sign in first" {
		t.Errorf("Reject = %v", err)
	}
	if _, err := InitialState("private", nil, nil); err == nil {
		t.Error("Reject while rendering should fail")
	}
	if st, err := InitialState("private", nil, map[string]any{"user": 1}); err != nil || string(st["secret"]) != "42" {
		t.Errorf("allowed: %v %v", st, err)
	}
	Register("badsetup", func(*Component) { panic("setup") })
	if _, err := MountLive(ctx, "badsetup", r.opts("", nil)); !errors.Is(err, ErrPanic) {
		t.Errorf("panicking Setup = %v", err)
	}
}

func TestLiveBindRestorePropsEmit(t *testing.T) {
	Register("form", func(c *Component) {
		type P struct {
			Label string `json:"label"`
		}
		props := Props[P](c)
		q := State(c, "query", "", Bind)
		draft := State(c, "draft", "", Restore)
		State(c, "plain", "server")
		Derived(c, "title", func() string { return props.Get().Label + ": " + q.Get() })
		c.Func("submit", func() { c.Emit("submitted", map[string]string{"query": q.Peek(), "draft": draft.Peek()}) })
	})
	r := newRecorder()
	o := r.opts(`{"label":"Search"}`, nil)
	o.Restore = map[string]json.RawMessage{"query": json.RawMessage(`"go"`), "draft": json.RawMessage(`"hi"`), "plain": json.RawMessage(`"forged"`)}
	inst := mustMount(t, "form", o)
	ctx := context.Background()
	if r.get("title") != `"Search: go"` || r.get("draft") != `"hi"` || r.get("plain") != `"server"` {
		t.Errorf("restored = %v", r.state)
	}
	if err := inst.Set(ctx, "query", []byte(`"svelte"`)); err != nil || r.get("title") != `"Search: svelte"` {
		t.Errorf("bind = %v, %v", err, r.state)
	}
	if err := inst.Set(ctx, "plain", []byte(`"x"`)); err == nil {
		t.Error("setting a State without Bind should fail")
	}
	if err := inst.SetProps(ctx, []byte(`{"label":"Find"}`)); err != nil || r.get("title") != `"Find: svelte"` {
		t.Errorf("props = %v, %v", err, r.state)
	}
	if _, err := inst.Call(ctx, "submit", nil); err != nil || len(r.events) != 1 || !strings.Contains(r.events[0], `"query":"svelte"`) {
		t.Errorf("emit = %v, %v", err, r.events)
	}
}

func TestLiveBackgroundWork(t *testing.T) {
	destroyed := make(chan struct{})
	Register("ticker", func(c *Component) {
		ticks := State(c, "ticks", 0)
		c.Every(time.Millisecond, func() { ticks.Update(func(n int) int { return n + 1 }) })
		c.Go(func(ctx context.Context) {
			<-ctx.Done()
			close(destroyed)
		})
		Subscribe(c, "news", func(s string) { ticks.Set(-len(s)) })
	})
	r := newRecorder()
	inst := mustMount(t, "ticker", r.opts("", nil))
	eventually(t, "ticks", func() bool { n, _ := strconv.Atoi(r.get("ticks")); return n >= 3 })
	inst.Destroy()
	select {
	case <-destroyed:
	case <-time.After(time.Second):
		t.Fatal("the component's context was not canceled")
	}
	// Rendering runs no background work.
	if _, err := InitialState("ticker", nil, nil); err != nil {
		t.Fatal(err)
	}
}

func TestLiveConcurrency(t *testing.T) {
	total := Global(0)
	Register("counter-many", func(c *Component) {
		mine := State(c, "mine", 0)
		total.Show(c, "total")
		c.Func("inc", func() {
			mine.Update(func(n int) int { return n + 1 })
			total.Update(func(n int) int { return n + 1 })
		})
	})
	const n, calls = 8, 50
	recs := make([]*recorder, n)
	insts := make([]*Instance, n)
	for i := range n {
		recs[i] = newRecorder()
		insts[i] = mustMount(t, "counter-many", recs[i].opts("", nil))
	}
	var wg sync.WaitGroup
	for _, inst := range insts {
		wg.Go(func() {
			for range calls {
				if _, err := inst.Call(context.Background(), "inc", nil); err != nil {
					t.Error(err)
				}
			}
		})
	}
	wg.Wait()
	want := fmt.Sprint(n * calls)
	for i, r := range recs {
		eventually(t, "every instance to see the total", func() bool { return r.get("total") == want })
		if r.get("mine") != fmt.Sprint(calls) {
			t.Errorf("instance %d: mine = %s", i, r.get("mine"))
		}
	}
}

// fakeBroker is an in-process broker that also lets a test speak for
// another process.
type fakeBroker struct {
	mu   sync.Mutex
	subs map[string][]func([]byte)
	sent []string
}

func (b *fakeBroker) Publish(_ context.Context, topic string, msg []byte) error {
	b.mu.Lock()
	subs := append([]func([]byte){}, b.subs[topic]...)
	b.sent = append(b.sent, topic+" "+string(msg))
	b.mu.Unlock()
	for _, fn := range subs {
		fn(msg)
	}
	return nil
}

func (b *fakeBroker) Subscribe(_ context.Context, topic string, fn func([]byte)) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.subs == nil {
		b.subs = map[string][]func([]byte){}
	}
	b.subs[topic] = append(b.subs[topic], fn)
	return nil
}

// remote publishes as another process.
func (b *fakeBroker) remote(topic string, e envelope) {
	e.Origin = "zzzz-other"
	raw, _ := json.Marshal(e)
	_ = b.Publish(context.Background(), topic, raw)
}

func TestSharedPresenceAndTopics(t *testing.T) {
	b := &fakeBroker{}
	color := Shared("color", "red")
	room := Presence("room")
	SetBroker(b)
	t.Cleanup(func() { SetBroker(nil) })

	Register("room", func(c *Component) {
		color.Show(c, "color")
		room.Show(c, "online")
		room.Join(c)
		got := State(c, "got", "")
		Subscribe(c, "chat", func(s string) { got.Set(s) })
		c.Func("paint", func(v string) { color.Set(v) })
	})
	r := newRecorder()
	inst := mustMount(t, "room", r.opts("", nil))
	eventually(t, "online = 1", func() bool { return r.get("online") == "1" })

	// Another process reports two visitors and a newer color.
	b.remote(topicPrefix+"presence.room", envelope{Count: 2})
	eventually(t, "online = 3", func() bool { return r.get("online") == "3" })
	b.remote(topicPrefix+"shared.color", envelope{Clock: 5, Data: json.RawMessage(`"blue"`)})
	eventually(t, "remote color", func() bool { return r.get("color") == `"blue"` })
	// An older write loses.
	b.remote(topicPrefix+"shared.color", envelope{Clock: 3, Data: json.RawMessage(`"green"`)})
	if _, err := inst.Call(context.Background(), "paint", []byte(`["pink"]`)); err != nil {
		t.Fatal(err)
	}
	if r.get("color") != `"pink"` || color.Get() != "pink" {
		t.Errorf("local write: %v", r.state)
	}
	eventually(t, "the write to be published with a newer clock", func() bool {
		b.mu.Lock()
		defer b.mu.Unlock()
		for _, m := range b.sent {
			if strings.Contains(m, `"c":6`) && strings.Contains(m, "pink") {
				return true
			}
		}
		return false
	})
	// Topics reach local and remote subscribers.
	if err := Publish(context.Background(), "chat", "hello"); err != nil {
		t.Fatal(err)
	}
	eventually(t, "local message", func() bool { return r.get("got") == `"hello"` })
	b.remote(topicPrefix+"topic.chat", envelope{Data: json.RawMessage(`"from afar"`)})
	eventually(t, "remote message", func() bool { return r.get("got") == `"from afar"` })

	inst.Destroy()
	if room.Count() != 2 {
		t.Errorf("after leaving, count = %d, want the other process's 2", room.Count())
	}
	b.remote(topicPrefix+"presence.room", envelope{Count: 0})
	eventually(t, "count = 0", func() bool { return room.Count() == 0 })
}

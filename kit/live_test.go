package kit

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/Unfield/Tern/goscript"
)

// liveClient speaks the live protocol in tests.
type liveClient struct {
	t    *testing.T
	ctx  context.Context
	conn *websocket.Conn
}

func dialLive(t *testing.T, a *App) *liveClient {
	t.Helper()
	srv := httptest.NewServer(a)
	t.Cleanup(srv.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+LivePath, nil) //nolint:bodyclose // the connection owns it
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close(websocket.StatusNormalClosure, "") })
	return &liveClient{t: t, ctx: ctx, conn: conn}
}

func (c *liveClient) send(m liveIn) {
	c.t.Helper()
	if err := wsjson.Write(c.ctx, c.conn, m); err != nil {
		c.t.Fatal(err)
	}
}

func (c *liveClient) recv() liveOut {
	c.t.Helper()
	var m liveOut
	if err := wsjson.Read(c.ctx, c.conn, &m); err != nil {
		c.t.Fatal(err)
	}
	return m
}

func TestLiveSocket(t *testing.T) {
	goscript.Register("kit-counter", func(c *goscript.Component) {
		user, _ := c.Locals()["user"].(string)
		n := goscript.State(c, "n", 0)
		goscript.State(c, "user", user)
		q := goscript.State(c, "q", "", goscript.Bind)
		items := goscript.State(c, "items", []string{"a", "b", "c", "d", "e", "f", "g", "h"})
		goscript.Derived(c, "echo", func() string { return "q=" + q.Get() })
		c.Func("add", func(k int) int { n.Set(n.Get() + k); return n.Get() })
		c.Func("push", func(s string) { items.Update(func(v []string) []string { return append(append([]string{}, v...), s) }) })
		c.Func("boom", func() { panic("boom") })
		c.Func("secret", func() error { return errors.New("password=hunter2") })
		c.Func("refuse", func() error { return goscript.Fail("no way") })
		c.Func("notify", func() { c.Emit("saved", 42) })
	})
	m := testManifest()
	m.Live = true
	a, _ := newTestApp(t, m, WithHook(func(c *Ctx) error { c.Locals["user"] = "ada"; return nil }))
	c := dialLive(t, a)

	c.send(liveIn{Op: "mount", Ref: 1, ID: "kit-counter", Props: json.RawMessage(`{}`), Restore: map[string]json.RawMessage{"q": json.RawMessage(`"kept"`)}})
	st := c.recv()
	if st.Op != "state" || string(st.Set["n"]) != "0" || string(st.Set["user"]) != `"ada"` || string(st.Set["echo"]) != `"q=kept"` {
		t.Fatalf("mount state = %+v (locals come from hooks, q is restored)", st)
	}
	c.send(liveIn{Op: "call", Ref: 1, Seq: 7, Name: "add", Args: json.RawMessage(`[5]`)})
	state, result := c.recv(), c.recv()
	if state.Op != "state" || string(state.Set["n"]) != "5" || result.Op != "result" || result.Seq != 7 || string(result.Value) != "5" {
		t.Errorf("call: %+v %+v", state, result)
	}
	// Appending to a list sends a patch.
	c.send(liveIn{Op: "call", Ref: 1, Seq: 8, Name: "push", Args: json.RawMessage(`["i"]`)})
	state, _ = c.recv(), c.recv()
	if string(state.Patch["items"]) != `[["a",["items"],8,0,["i"]]]` && string(state.Patch["items"]) != `[["a",[],8,0,["i"]]]` {
		t.Errorf("patch = %+v", state)
	}
	c.send(liveIn{Op: "set", Ref: 1, Seq: 9, Name: "q", Value: json.RawMessage(`"go"`)})
	state, result = c.recv(), c.recv()
	if string(state.Set["echo"]) != `"q=go"` || result.Seq != 9 || result.Error != "" {
		t.Errorf("set: %+v %+v", state, result)
	}
	c.send(liveIn{Op: "set", Ref: 1, Seq: 10, Name: "n", Value: json.RawMessage(`1`)})
	if r := c.recv(); r.Error == "" {
		t.Errorf("setting a State without Bind should fail: %+v", r)
	}
	for seq, want := range map[int]string{11: "the component failed", 12: "the component failed", 13: "no way"} {
		name := map[int]string{11: "boom", 12: "secret", 13: "refuse"}[seq]
		c.send(liveIn{Op: "call", Ref: 1, Seq: seq, Name: name, Args: json.RawMessage(`[]`)})
		if r := c.recv(); r.Error != want && (seq != 11 || !strings.Contains(r.Error, "failed")) {
			t.Errorf("%s: %+v", name, r)
		}
	}
	c.send(liveIn{Op: "call", Ref: 1, Seq: 14, Name: "notify"})
	if ev := c.recv(); ev.Op != "event" || ev.Name != "saved" || string(ev.Value) != "42" {
		t.Errorf("event: %+v", ev)
	}
	c.recv() // the result
	c.send(liveIn{Op: "mount", Ref: 2, ID: "nope"})
	if r := c.recv(); r.Op != "error" || r.Ref != 2 {
		t.Errorf("unknown component: %+v", r)
	}
	c.send(liveIn{Op: "call", Ref: 3, Seq: 1, Name: "add"})
	if r := c.recv(); r.Error != "not mounted" {
		t.Errorf("unmounted: %+v", r)
	}
}

func TestLiveLimits(t *testing.T) {
	goscript.Register("kit-tiny", func(c *goscript.Component) {
		c.Func("noop", func() {})
	})
	m := testManifest()
	m.Live = true
	a, _ := newTestApp(t, m, WithLive(LiveOptions{MaxInstances: 2, Rate: 1}))
	c := dialLive(t, a)
	for ref := 1; ref <= 3; ref++ {
		c.send(liveIn{Op: "mount", Ref: ref, ID: "kit-tiny"})
	}
	if r := c.recv(); r.Op != "error" || r.Ref != 3 {
		t.Errorf("third mount: %+v", r)
	}
	limited := false
	for seq := 1; seq <= 4; seq++ {
		c.send(liveIn{Op: "call", Ref: 1, Seq: seq, Name: "noop"})
	}
	for range 4 {
		if r := c.recv(); strings.Contains(r.Error, "slow down") {
			limited = true
		}
	}
	if !limited {
		t.Error("calls over the rate should be refused")
	}
}

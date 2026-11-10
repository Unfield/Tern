package kit

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/Unfield/Tern/goscript"
)

func TestLiveSocket(t *testing.T) {
	goscript.Register("kit-counter", func(c *goscript.Component) {
		user, _ := c.Locals()["user"].(string)
		n := goscript.State(c, "n", 0)
		goscript.State(c, "user", user)
		c.Func("add", func(k int) int { n.Set(n.Get() + k); return n.Get() })
		c.Func("boom", func() { panic("boom") })
	})
	m := testManifest()
	m.Live = true
	a, _ := newTestApp(t, m, WithHook(func(c *Ctx) error { c.Locals["user"] = "ada"; return nil }))
	srv := httptest.NewServer(a)
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+LivePath, nil) //nolint:bodyclose // the connection owns it
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()
	send := func(m liveMessage) {
		if err := wsjson.Write(ctx, conn, m); err != nil {
			t.Fatal(err)
		}
	}
	recv := func() liveMessage {
		var m liveMessage
		if err := wsjson.Read(ctx, conn, &m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	send(liveMessage{Op: "mount", Ref: 1, ID: "kit-counter", Props: json.RawMessage(`{}`)})
	got := map[string]string{}
	for range 2 {
		m := recv()
		got[m.Name] = string(m.Value)
	}
	if got["n"] != "0" || got["user"] != `"ada"` {
		t.Fatalf("mount state = %v (locals come from hooks)", got)
	}
	send(liveMessage{Op: "call", Ref: 1, Seq: 7, Name: "add", Args: json.RawMessage(`[5]`)})
	state, result := recv(), recv()
	if state.Op != "state" || string(state.Value) != "5" || result.Op != "result" || result.Seq != 7 || string(result.Value) != "5" {
		t.Errorf("call: %+v %+v", state, result)
	}
	send(liveMessage{Op: "call", Ref: 1, Seq: 8, Name: "boom", Args: json.RawMessage(`[]`)})
	if r := recv(); r.Error == "" {
		t.Errorf("a panic should come back as an error: %+v", r)
	}
	send(liveMessage{Op: "mount", Ref: 2, ID: "nope"})
	if r := recv(); r.Op != "error" || r.Ref != 2 {
		t.Errorf("unknown component: %+v", r)
	}
}

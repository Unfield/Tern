package livetest_test

import (
	"testing"

	"github.com/Unfield/Tern/goscript"
	"github.com/Unfield/Tern/goscript/livetest"
)

func init() {
	goscript.Register(goscript.ComponentID("src/lib/Counter.svelte"), func(c *goscript.Component) {
		var p struct {
			Start int `json:"start"`
		}
		_ = c.Props(&p)
		count := goscript.State(c, "count", p.Start)
		user, _ := c.Locals()["user"].(string)
		goscript.State(c, "user", user)
		goscript.State(c, "note", "", goscript.Bind)
		c.Func("increment", func(by int) int {
			count.Update(func(n int) int { return n + by })
			c.Emit("changed", count.Peek())
			return count.Peek()
		})
	})
}

func TestHarness(t *testing.T) {
	c := livetest.Mount(t, "src/lib/Counter.svelte", map[string]any{"start": 2},
		livetest.WithLocals(map[string]any{"user": "ada"}), livetest.WithRestore(map[string]any{"note": "kept"}))
	if livetest.Get[int](c, "count") != 2 || livetest.Get[string](c, "user") != "ada" || livetest.Get[string](c, "note") != "kept" {
		t.Fatalf("initial state")
	}
	out, err := c.Call("increment", 3)
	if err != nil || string(out) != "5" || livetest.Get[int](c, "count") != 5 {
		t.Errorf("increment: %s, %v", out, err)
	}
	if ev := c.Events(); len(ev) != 1 || ev[0].Name != "changed" || string(ev[0].Value) != "5" {
		t.Errorf("events = %v", ev)
	}
	if err := c.Set("note", "new"); err != nil || livetest.Get[string](c, "note") != "new" {
		t.Errorf("set: %v", err)
	}
}

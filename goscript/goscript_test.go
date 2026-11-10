package goscript

import (
	"encoding/json"
	"testing"
)

func TestRunes(t *testing.T) {
	emitted := map[string]string{}
	var calls int
	Register("counter", func(c *Component) {
		var props struct {
			Start int `json:"start"`
		}
		_ = c.Props(&props)
		count := State(c, "count", props.Start)
		step := State(c, "step", 1)
		doubled := Derived(c, "doubled", func() int { return count.Get() * 2 })
		Derived(c, "label", func() string {
			calls++
			return "count is " + jsonString(doubled.Get())
		})
		c.Func("add", func(n int) { count.Set(count.Get() + n*step.Get()) })
		c.Func("both", func() int {
			count.Set(count.Get() + 1)
			step.Set(10)
			return count.Peek()
		})
		c.Func("type", func(e Event) string { return e.Value })
	})
	c, err := mount("counter", []byte(`{"start":2}`), func(name string, v []byte) { emitted[name] = string(v) })
	if err != nil {
		t.Fatal(err)
	}
	if emitted["count"] != "2" || emitted["doubled"] != "4" || emitted["label"] != `"count is 4"` {
		t.Fatalf("initial = %v", emitted)
	}
	// A derived value equal to its type's zero value is published too.
	zero := map[string]string{}
	Register("zero", func(c *Component) { Derived(c, "z", func() int { return 0 }) })
	if _, err := mount("zero", nil, func(n string, v []byte) { zero[n] = string(v) }); err != nil || zero["z"] != "0" {
		t.Errorf("zero derived = %v, %v", zero, err)
	}
	if _, err := c.call("add", []byte(`[3]`)); err != nil {
		t.Fatal(err)
	}
	if emitted["count"] != "5" || emitted["doubled"] != "10" || emitted["label"] != `"count is 10"` {
		t.Errorf("after add = %v", emitted)
	}
	before := calls
	out, _ := c.call("both", []byte(`[]`))
	if string(out) != "6" || calls-before != 1 {
		t.Errorf("both: %s, label recomputed %d times (want once per batch)", out, calls-before)
	}
	if out, _ := c.call("type", []byte(`[{"type":"input","value":"hi"}]`)); string(out) != `"hi"` {
		t.Errorf("event: %s", out)
	}
	if _, err := c.call("nope", nil); err == nil {
		t.Error("calling a missing Func should fail")
	}
	c.destroy()
	emitted = map[string]string{}
	_, _ = c.call("add", []byte(`[1]`))
	if len(emitted) != 0 {
		t.Errorf("a destroyed component still publishes: %v", emitted)
	}
}

func jsonString(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func TestEffectsAndCycles(t *testing.T) {
	c := &Component{funcs: nil, emit: func(string, []byte) {}}
	a := State(c, "", 1)
	var seen []int
	Effect(c, func() { seen = append(seen, a.Get()) })
	a.Set(2)
	a.Set(2) // unchanged: no rerun
	a.Update(func(v int) int { return v + 1 })
	if len(seen) != 3 || seen[2] != 3 {
		t.Errorf("effect runs = %v", seen)
	}
	defer func() {
		if recover() == nil {
			t.Error("an effect that changes its own input forever should panic")
		}
	}()
	Effect(c, func() { a.Set(a.Get() + 1) })
}

func TestLiveAndGlobals(t *testing.T) {
	visits := Global(0)
	Register("shared", func(c *Component) {
		visits.Show(c, "visits")
		user, _ := c.Locals()["user"].(string)
		Derived(c, "greeting", func() string { return user + " sees " + jsonString(visits.Get()) })
		c.Func("visit", func() { visits.Update(func(n int) int { return n + 1 }) })
	})
	a, b := map[string]string{}, map[string]string{}
	ia, err := MountLive("shared", nil, map[string]any{"user": "ada"}, func(n string, v []byte) { a[n] = string(v) })
	if err != nil {
		t.Fatal(err)
	}
	ib, _ := MountLive("shared", nil, map[string]any{"user": "bob"}, func(n string, v []byte) { b[n] = string(v) })
	if _, err := ia.Call("visit", []byte("[]")); err != nil {
		t.Fatal(err)
	}
	if a["visits"] != "1" || b["visits"] != "1" || b["greeting"] != `"bob sees 1"` {
		t.Errorf("after ada's visit: a=%v b=%v", a, b)
	}
	ib.Destroy()
	b = map[string]string{}
	_, _ = ia.Call("visit", []byte("[]"))
	if a["visits"] != "2" || len(b) != 0 {
		t.Errorf("after bob left: a=%v b=%v", a, b)
	}
	state, err := InitialState("shared", nil, map[string]any{"user": "cy"})
	if err != nil || string(state["greeting"]) != `"cy sees 2"` {
		t.Errorf("InitialState = %v, %v", state, err)
	}
	if Registered("nope") || !Registered("shared") {
		t.Error("Registered")
	}
}

package js

import "testing"

func TestProxy(t *testing.T) {
	rt := NewRealm()
	var log []string
	target := rt.Obj("a", 1.0)
	handler := rt.Obj(
		"get", rt.NewFunction("get", 3, func(_ Value, args []Value) Value {
			log = append(log, "get "+ToString(args[1]))
			if args[1] == "magic" {
				return 42.0
			}
			return args[0].(*Object).Get(args[1])
		}),
		"set", rt.NewFunction("set", 4, func(_ Value, args []Value) Value {
			log = append(log, "set "+ToString(args[1]))
			args[0].(*Object).Set(args[1], Mul(args[2], 2.0))
			return true
		}),
		"has", rt.NewFunction("has", 2, func(_ Value, args []Value) Value { return args[1] == "virtual" }),
	)
	p := Construct(rt.Global.Get("Proxy"), target, handler)
	var c IC
	// Inline caches must never cache a proxy: each read reaches the trap.
	for range 3 {
		if v := rt.GetIC(&c, p, "magic"); v != 42.0 {
			t.Fatalf("magic: %v", v)
		}
	}
	var s IC
	rt.SetIC(&s, p, "b", 5.0)
	rt.SetIC(&s, p, "b", 6.0)
	if target.Get("b") != 12.0 {
		t.Fatalf("set trap: target.b is %v", target.Get("b"))
	}
	if !In("virtual", p) || In("a", p) {
		t.Fatal("has trap")
	}
	if len(log) != 5 {
		t.Fatalf("traps ran %d times: %v", len(log), log)
	}
	// Reflect.ownKeys orders integer keys first, as Node does.
	o := rt.Obj("b", 1.0, "2", 1.0, "a", 1.0, "1", 1.0)
	keys := rt.CallMethod(rt.Global.Get("Reflect"), "ownKeys", o)
	if got := ToString(keys); got != "1,2,b,a" {
		t.Fatalf("Reflect.ownKeys: %s", got)
	}
	// A proxy without traps forwards everything; JSON sees the target.
	plain := Construct(rt.Global.Get("Proxy"), rt.Obj("x", 1.0), rt.NewPlainObject())
	if s, _ := rt.Stringify(plain, Undefined, Undefined); s != `{"x":1}` {
		t.Fatalf("JSON of a forwarding proxy: %s", s)
	}
}

package js

import (
	"testing"
	"time"
)

func TestRunawayRecursion(t *testing.T) {
	var f *Object
	f = tr.NewFunction("f", 0, func(Value, []Value) Value { return Call(f, Undefined) })
	err := catch(func() { Call(f, Undefined) })
	if err == nil || err.Kind != "RangeError" {
		t.Fatalf("got %v, want a RangeError", err)
	}
	if tr.depth != 0 {
		t.Fatalf("depth is %d after the error", tr.depth)
	}
}

func TestInterrupt(t *testing.T) {
	rt := NewRealm()
	loop := rt.NewFunction("loop", 0, func(Value, []Value) Value {
		for {
			rt.Tick()
		}
	})
	done := make(chan any)
	go func() {
		defer func() { done <- recover() }()
		Call(loop, Undefined)
	}()
	time.Sleep(10 * time.Millisecond)
	rt.Interrupt()
	select {
	case r := <-done:
		if r != any(ErrInterrupted) {
			t.Fatalf("got %v, want ErrInterrupted", r)
		}
	case <-time.After(time.Second):
		t.Fatal("the loop didn't stop")
	}
}

func TestHostFunc(t *testing.T) {
	rt := NewRealm()
	var got []any
	rt.HostFunc("host", func(args []any) (any, error) {
		got = args
		return "ok", nil
	})
	out := Call(rt.Global.Get("host"), Undefined, "a", 2.0, 2.5, true, Null)
	if out != "ok" || len(got) != 5 || got[0] != "a" || got[1] != 2 || got[2] != 2.5 || got[3] != true || got[4] != nil {
		t.Fatalf("got %v → %v", got, out)
	}
}

package js

import (
	"fmt"
	"math"
	"runtime"
	"testing"
)

func TestBoxer(t *testing.T) {
	var b Boxer
	strs := make([]Value, 0, 1000)
	nums := make([]Value, 0, 1000)
	for i := range 1000 {
		strs = append(strs, b.Str(fmt.Sprint("s", i)))
		nums = append(nums, b.Num(float64(i)+0.5))
	}
	// Slabs stay alive while their values do, through collections.
	runtime.GC()
	runtime.GC()
	for i := range 1000 {
		if s, ok := strs[i].(string); !ok || s != fmt.Sprint("s", i) {
			t.Fatalf("strs[%d] = %#v", i, strs[i])
		}
		if f, ok := nums[i].(float64); !ok || f != float64(i)+0.5 {
			t.Fatalf("nums[%d] = %#v", i, nums[i])
		}
	}
	// They're ordinary values: equal to others, usable as map keys, typed.
	if strs[7] != Value("s7") || nums[3] != Value(3.5) {
		t.Error("a boxed value isn't equal to the same value boxed by Go")
	}
	m := map[Value]int{strs[1]: 1}
	if m["s1"] != 1 {
		t.Error("a boxed string isn't the same map key")
	}
	switch v := strs[2].(type) {
	case string:
		if v != "s2" {
			t.Error(v)
		}
	default:
		t.Errorf("type switch: %T", v)
	}
	if b.Str("") != Value("") || b.Num(5) != boxedInts[5] || !math.Signbit(b.Num(math.Copysign(0, -1)).(float64)) {
		t.Error("edge values")
	}
	if Typeof(b.Str("x")) != "string" || Typeof(b.Num(1.5)) != "number" || ToString(b.Num(1.5)) != "1.5" {
		t.Error("the runtime doesn't see boxed values as their types")
	}
}

func TestBoxerAllocations(t *testing.T) {
	var b Boxer
	s := "already a string"
	allocs := testing.AllocsPerRun(1000, func() {
		for range 64 {
			_ = b.Str(s)
			_ = b.Num(1234.5)
		}
	})
	// Two slabs of 128 per 128 values each, at most: under one allocation a run.
	if allocs > 1 {
		t.Errorf("%v allocations for 128 values", allocs)
	}
}

func TestPlainEach(t *testing.T) {
	rt := NewRealm()
	o := AsObject(rt.Obj("b", 1.0, "a", true))
	var got []string
	if !o.PlainEach(func(k string, v Value) { got = append(got, k+"="+ToString(v)) }) || fmt.Sprint(got) != "[b=1 a=true]" {
		t.Errorf("entries = %v", got)
	}
	// Integer keys come first in property order: left to the full algorithm.
	withInt := AsObject(rt.Obj("b", 1.0, "1", 2.0))
	if withInt.PlainEach(func(string, Value) {}) {
		t.Error("an object with an integer key was walked in insertion order")
	}
	arr := AsObject(rt.NewArray([]Value{1.0}))
	if arr.PlainEach(func(string, Value) {}) {
		t.Error("an array was walked as a plain object")
	}
}

func TestIDProperties(t *testing.T) {
	rt := NewRealm()
	re := rt.NewRegExp(`^(?:\p{ID_Start}[$\u200c\u200d\p{ID_Continue}]*)$`, "u")
	test := func(s string) bool { return Truthy(Call(rt.Get(re, "test"), re, s)) }
	for s, want := range map[string]bool{"foo": true, "ÉtéX9_": true, "名前": true, "℘x": true, "a\u200db": true, "9x": false, "a-b": false, "": false, "x·y": true} {
		if got := test(s); got != want {
			t.Errorf("%q: %v, want %v", s, got, want)
		}
	}
	not := rt.NewRegExp(`^\P{ID_Start}$`, "u")
	if !Truthy(Call(rt.Get(not, "test"), not, "9")) || Truthy(Call(rt.Get(not, "test"), not, "a")) {
		t.Error("\\P{ID_Start}")
	}
}

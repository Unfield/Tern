package js

import (
	"errors"
	"math"
)

// ErrInterrupted is what a realm's code panics with once Interrupt is called.
var ErrInterrupted = errors.New("js: interrupted")

// Interrupt stops the realm's running code at its next call or loop turn;
// it may be called from any goroutine. The realm can't be used after.
func (rt *Realm) Interrupt() { rt.interrupted.Store(true) }

// Tick is compiled into every loop: it stops interrupted code.
func (rt *Realm) Tick() {
	if rt.interrupted.Load() {
		panic(ErrInterrupted)
	}
}

// Reset clears what an interrupted or failed call left behind, for the
// next call: the argument stack, the call depth and queued jobs.
func (rt *Realm) Reset() {
	clear(rt.argStack)
	rt.argStack = rt.argStack[:0]
	rt.depth = 0
	rt.jobs = nil
}

// HostFunc makes a global function that calls Go, as the QuickJS
// renderer's host functions do: arguments arrive as strings, numbers (int
// when whole), bools or nil, and the result goes back the same way. An
// error becomes a thrown Error.
func (rt *Realm) HostFunc(name string, fn func(args []any) (any, error)) {
	f := rt.NewFunction(name, 0, func(_ Value, args []Value) Value {
		in := make([]any, len(args))
		for i, a := range args {
			in[i] = ToGo(a)
		}
		out, err := fn(in)
		if err != nil {
			panic(errorf("Error", "%s", []any{err.Error()}))
		}
		return FromGo(out)
	})
	rt.Global.DefineHidden(name, f)
}

// ToGo converts a primitive to Go: nil, bool, int (for whole numbers),
// float64 or string. Objects become their string conversion's result.
func ToGo(v Value) any {
	switch x := v.(type) {
	case undefinedType, nullType:
		return nil
	case bool:
		return x
	case float64:
		if i := int(x); float64(i) == x && math.Abs(x) < 1<<53 && !(x == 0 && math.Signbit(x)) {
			return i
		}
		return x
	case string:
		return x
	}
	return ToString(v)
}

// FromGo converts a Go value to a primitive Value.
func FromGo(v any) Value {
	switch x := v.(type) {
	case nil:
		return Undefined
	case bool:
		return x
	case string:
		return x
	case []byte:
		return string(x)
	case int:
		return Box(float64(x))
	case int64:
		return Box(float64(x))
	case int32:
		return Box(float64(x))
	case float64:
		return Box(x)
	case float32:
		return Box(float64(x))
	case Value:
		return x
	}
	return Undefined
}

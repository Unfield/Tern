// Package js is a Go model of JavaScript values with the language's exact
// semantics, for components that tern build compiles to Go (ADR 0011).
// Generated code calls it; it is not covered by the stability promise.
//
// A Value is one of:
//
//	Undefined, Null       the two sentinels
//	bool                  a boolean
//	float64               a number
//	string                a string, UTF-8 encoded; lengths and indexes are UTF-16
//	*Object               an object, including functions and arrays
//	*Symbol               a symbol
package js

import (
	"math"
	"math/big"
	"unsafe"
)

// Value is a JavaScript value.
type Value = any

type undefinedType struct{}
type nullType struct{}

// hole marks a missing element of an array, as new Array(3) has: it reads
// as undefined, but the element isn't there, so in, Object.keys and the
// array methods that skip missing elements see that. It never leaves the
// array's storage.
type holeType struct{}

var hole Value = holeType{}

// unhole is v as JavaScript sees an element: undefined for a hole.
func unhole(v Value) Value {
	if v == hole {
		return Undefined
	}
	return v
}

// The two values that aren't objects or primitives of a Go type.
var (
	Undefined Value = undefinedType{}
	Null      Value = nullType{}
)

// Symbol is a JavaScript symbol. Each one is distinct.
type Symbol struct{ Description string }

// Well-known symbols.
var (
	SymbolIterator    = &Symbol{"Symbol.iterator"}
	SymbolToPrimitive = &Symbol{"Symbol.toPrimitive"}
	// SymbolAsyncIterator is Symbol.asyncIterator.
	SymbolAsyncIterator = &Symbol{"Symbol.asyncIterator"}
	SymbolToStringTag   = &Symbol{"Symbol.toStringTag"}
)

// IsNullish reports whether v is undefined or null.
func IsNullish(v Value) bool { return v == Undefined || v == Null }

// Typeof is the typeof operator.
func Typeof(v Value) string {
	switch x := v.(type) {
	case undefinedType:
		return "undefined"
	case nullType:
		return "object"
	case bool:
		return "boolean"
	case float64:
		return "number"
	case string:
		return "string"
	case *Symbol:
		return "symbol"
	case *big.Int:
		return "bigint"
	case *Object:
		if x.call != nil {
			return "function"
		}
		return "object"
	}
	panic(TypeErrorf("unknown value of Go type %T", v))
}

// Truthy is ToBoolean.
func Truthy(v Value) bool {
	switch x := v.(type) {
	case bool:
		return x
	case float64:
		return x != 0 && !math.IsNaN(x)
	case string:
		return x != ""
	case undefinedType, nullType:
		return false
	case *big.Int:
		return x.Sign() != 0
	}
	return true // objects and symbols
}

// Num makes a number from any Go number, for generated code.
func Num[T ~int | ~int8 | ~int16 | ~int32 | ~int64 | ~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~float32 | ~float64](n T) Value {
	return Box(float64(n))
}

// boxedInts holds the small integers as Values. Putting a float64 in an
// interface allocates; small integers, the most common numbers, needn't.
var boxedInts = func() []Value {
	out := make([]Value, 1024)
	for i := range out {
		out[i] = float64(i)
	}
	return out
}()

// Box makes a number a Value, without allocating for small integers.
func Box(f float64) Value {
	if i := int(f); float64(i) == f && i >= 0 && i < len(boxedInts) && (i != 0 || !math.Signbit(f)) {
		return boxedInts[i]
	}
	return f
}

// SetUndefined sets n Values in a row, from first, to Undefined: the
// fields of a compiled module's variables, which are js.Values one after
// another, so they're laid out without gaps.
func SetUndefined(first *Value, n int) {
	vals := unsafe.Slice(first, n)
	for i := range vals {
		vals[i] = Undefined
	}
}

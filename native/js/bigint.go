package js

import (
	"math"
	"math/big"
	"strings"
)

// BigInt. A BigInt is a *big.Int, never changed once made.

// BigIntLit is a BigInt literal's value, from its digits.
func BigIntLit(digits string) Value {
	b, ok := new(big.Int).SetString(strings.ReplaceAll(digits, "_", ""), 0)
	if !ok {
		panic(SyntaxErrorf("invalid BigInt literal %s", digits))
	}
	return b
}

var errMix = "Cannot mix BigInt and other types, use explicit conversions"

// operands is ToNumeric of both: numbers, or BigInts; mixing is a TypeError.
func operands(a, b Value) (x, y float64, bx, by *big.Int, isBig bool) {
	if fa, ok := a.(float64); ok {
		if fb, ok := b.(float64); ok {
			return fa, fb, nil, nil, false
		}
	}
	pa, pb := ToPrimitive(a, "number"), ToPrimitive(b, "number")
	ba, aBig := pa.(*big.Int)
	bb, bBig := pb.(*big.Int)
	switch {
	case aBig && bBig:
		return 0, 0, ba, bb, true
	case aBig || bBig:
		panic(TypeErrorf("%s", errMix))
	}
	return ToNumber(pa), ToNumber(pb), nil, nil, false
}

// bigints returns two BigInt operands, from primitives; mixing them with
// numbers is a TypeError.
func bigints(pa, pb Value) (x, y *big.Int, ok bool) {
	ba, aBig := pa.(*big.Int)
	bb, bBig := pb.(*big.Int)
	switch {
	case aBig && bBig:
		return ba, bb, true
	case aBig || bBig:
		panic(TypeErrorf("%s", errMix))
	}
	return nil, nil, false
}

// bigintOperands is for the bitwise operators: BigInts, or ok false for
// numbers (converted by ToInt32 then).
func bigintOperands(a, b Value) (x, y *big.Int, ok bool) {
	if _, isNum := a.(float64); isNum {
		if _, isNum := b.(float64); isNum {
			return nil, nil, false
		}
	}
	_, _, bx, by, isBig := operands(a, b)
	return bx, by, isBig
}

func shiftBig(x, n *big.Int) *big.Int {
	if !n.IsInt64() || n.Int64() > 1<<30 || n.Int64() < -(1<<30) {
		panic(RangeErrorf("Maximum BigInt size exceeded"))
	}
	s := n.Int64()
	if s >= 0 {
		return new(big.Int).Lsh(x, uint(s))
	}
	return new(big.Int).Rsh(x, uint(-s)) // floors, as >> on BigInts does
}

// looseBigInt is == when a BigInt is involved.
func looseBigInt(a, b Value) (eq, handled bool) {
	ba, aBig := a.(*big.Int)
	bb, bBig := b.(*big.Int)
	if !aBig && !bBig {
		return false, false
	}
	if aBig && bBig {
		return ba.Cmp(bb) == 0, true
	}
	if !aBig {
		b = a
		ba = bb
	}
	switch y := b.(type) {
	case float64:
		return cmpBigFloat(ba, y) == 0, true
	case string:
		n, ok := stringToBigInt(y)
		return ok && ba.Cmp(n) == 0, true
	case bool:
		return ba.Cmp(big.NewInt(int64(boolNum(y)))) == 0, true
	case *Object:
		return LooseEquals(ba, ToPrimitive(y, "default")), true
	}
	return false, true
}

// lessBigInt is IsLessThan when a BigInt is involved.
func lessBigInt(pa, pb Value) (r int, handled bool) {
	ba, aBig := pa.(*big.Int)
	bb, bBig := pb.(*big.Int)
	if !aBig && !bBig {
		return 0, false
	}
	toBig := func(v Value) (*big.Int, float64, bool) { // big, or float when not
		switch x := v.(type) {
		case *big.Int:
			return x, 0, true
		case string:
			if n, ok := stringToBigInt(x); ok {
				return n, 0, true
			}
			return nil, math.NaN(), false
		}
		return nil, ToNumber(v), false
	}
	if aBig && bBig {
		return boolInt(ba.Cmp(bb) < 0), true
	}
	if aBig {
		y, f, isBig := toBig(pb)
		if isBig {
			return boolInt(ba.Cmp(y) < 0), true
		}
		if math.IsNaN(f) {
			return -1, true
		}
		return boolInt(cmpBigFloat(ba, f) < 0), true
	}
	x, f, isBig := toBig(pa)
	if isBig {
		return boolInt(x.Cmp(bb) < 0), true
	}
	if math.IsNaN(f) {
		return -1, true
	}
	return boolInt(cmpBigFloat(bb, f) > 0), true
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// cmpBigFloat compares a BigInt with a number by mathematical value.
func cmpBigFloat(b *big.Int, f float64) int {
	switch {
	case math.IsNaN(f):
		return 2 // unordered: neither equal nor less
	case math.IsInf(f, 1):
		return -1
	case math.IsInf(f, -1):
		return 1
	}
	return new(big.Float).SetInt(b).Cmp(big.NewFloat(f))
}

func stringToBigInt(s string) (*big.Int, bool) {
	s = TrimJSSpace(s)
	if s == "" {
		return new(big.Int), true
	}
	base := 10
	switch {
	case len(s) > 2 && (s[:2] == "0x" || s[:2] == "0X"):
		base, s = 16, s[2:]
	case len(s) > 2 && (s[:2] == "0o" || s[:2] == "0O"):
		base, s = 8, s[2:]
	case len(s) > 2 && (s[:2] == "0b" || s[:2] == "0B"):
		base, s = 2, s[2:]
	}
	if base != 10 && (s[0] == '+' || s[0] == '-') {
		return nil, false
	}
	n, ok := new(big.Int).SetString(s, base)
	return n, ok
}

func (rt *Realm) setupBigInt() {
	rt.BigIntPrototype = &Object{proto: rt.ObjectPrototype}
	ctor := rt.NewFunction("BigInt", 1, func(_ Value, args []Value) Value {
		v := ToPrimitive(Arg(args, 0), "number")
		switch x := v.(type) {
		case float64:
			if x != math.Trunc(x) || math.IsInf(x, 0) || math.IsNaN(x) {
				panic(RangeErrorf("The number %s cannot be converted to a BigInt because it is not an integer", NumberToString(x)))
			}
			b, _ := new(big.Float).SetFloat64(x).Int(nil)
			return b
		case *big.Int:
			return x
		case bool:
			return big.NewInt(int64(boolNum(x)))
		case string:
			if n, ok := stringToBigInt(x); ok {
				return n
			}
			panic(SyntaxErrorf("Cannot convert %s to a BigInt", x))
		}
		panic(TypeErrorf("Cannot convert %s to a BigInt", ToString(v)))
	})
	ctor.add(prop{key: "prototype", value: rt.BigIntPrototype})
	rt.BigIntPrototype.DefineHidden("constructor", ctor)
	rt.BigIntPrototype.DefineHidden(SymbolToStringTag, "BigInt")
	rt.Global.DefineHidden("BigInt", ctor)
	thisBig := func(v Value) *big.Int {
		switch x := v.(type) {
		case *big.Int:
			return x
		case *Object:
			if b, ok := x.Internal.(*big.Int); ok && x.Class == "BigInt" {
				return b
			}
		}
		panic(TypeErrorf("not a BigInt"))
	}
	rt.method(rt.BigIntPrototype, "toString", 0, func(this Value, args []Value) Value {
		radix := 10
		if r := Arg(args, 0); r != Undefined {
			radix = int(ToIntegerOrInfinity(r))
			if radix < 2 || radix > 36 {
				panic(RangeErrorf("toString() radix must be between 2 and 36"))
			}
		}
		return thisBig(this).Text(radix)
	})
	rt.method(rt.BigIntPrototype, "toLocaleString", 0, func(this Value, _ []Value) Value { return thisBig(this).String() })
	rt.method(rt.BigIntPrototype, "valueOf", 0, func(this Value, _ []Value) Value { return thisBig(this) })
	asN := func(name string, signed bool) {
		rt.method(ctor, name, 2, func(_ Value, args []Value) Value {
			bits := int(ToIntegerOrInfinity(Arg(args, 0)))
			b, ok := Arg(args, 1).(*big.Int)
			if !ok {
				panic(TypeErrorf("Cannot convert %s to a BigInt", ToString(Arg(args, 1))))
			}
			mod := new(big.Int).Lsh(big.NewInt(1), uint(bits))
			r := new(big.Int).Mod(b, mod) // non-negative
			if signed && bits > 0 && r.Cmp(new(big.Int).Rsh(mod, 1)) >= 0 {
				r.Sub(r, mod)
			}
			return r
		})
	}
	asN("asIntN", true)
	asN("asUintN", false)
	rt.bigintShape = &Shape{proto: rt.BigIntPrototype}
}

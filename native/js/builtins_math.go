package js

import (
	"math"
	"math/rand/v2"
)

func setupMath() {
	m := NewPlainObject()
	Global.DefineHidden("Math", m)
	m.DefineHidden(SymbolToStringTag, "Math")
	for name, v := range map[string]float64{"PI": math.Pi, "E": math.E, "LN2": math.Ln2, "LN10": math.Ln10, "LOG2E": math.Log2E, "LOG10E": math.Log10E, "SQRT2": math.Sqrt2, "SQRT1_2": math.Sqrt2 / 2} {
		m.add(prop{key: name, value: v})
	}
	one := func(name string, fn func(float64) float64) {
		method(m, name, 1, func(_ Value, args []Value) Value { return fn(ToNumber(Arg(args, 0))) })
	}
	one("abs", math.Abs)
	one("acos", math.Acos)
	one("acosh", math.Acosh)
	one("asin", math.Asin)
	one("asinh", math.Asinh)
	one("atan", math.Atan)
	one("atanh", math.Atanh)
	one("cbrt", math.Cbrt)
	one("ceil", math.Ceil)
	one("cos", math.Cos)
	one("cosh", math.Cosh)
	one("exp", math.Exp)
	one("expm1", math.Expm1)
	one("floor", math.Floor)
	one("fround", func(x float64) float64 { return float64(float32(x)) })
	one("log", math.Log)
	one("log1p", math.Log1p)
	one("log10", math.Log10)
	one("log2", math.Log2)
	one("sin", math.Sin)
	one("sinh", math.Sinh)
	one("sqrt", math.Sqrt)
	one("tan", math.Tan)
	one("tanh", math.Tanh)
	one("trunc", math.Trunc)
	one("sign", func(x float64) float64 {
		switch {
		case x > 0:
			return 1
		case x < 0:
			return -1
		}
		return x // ±0 and NaN
	})
	// Math.round rounds half toward +∞: -2.5 is -2, unlike math.Round, and
	// x - floor(x) avoids floor(x + 0.5)'s error at 0.49999999999999994.
	one("round", func(x float64) float64 {
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return x
		}
		f := math.Floor(x)
		r := f
		if x-f >= 0.5 {
			r = f + 1
		}
		if r == 0 && (x < 0 || math.Signbit(x)) {
			return math.Copysign(0, -1)
		}
		return r
	})
	method(m, "atan2", 2, func(_ Value, args []Value) Value { return math.Atan2(ToNumber(Arg(args, 0)), ToNumber(Arg(args, 1))) })
	method(m, "pow", 2, func(_ Value, args []Value) Value { return Pow(Arg(args, 0), Arg(args, 1)) })
	method(m, "imul", 2, func(_ Value, args []Value) Value { return float64(ToInt32(Arg(args, 0)) * ToInt32(Arg(args, 1))) })
	method(m, "clz32", 1, func(_ Value, args []Value) Value {
		n := ToUint32(Arg(args, 0))
		c := 0
		for i := 31; i >= 0 && n&(1<<uint(i)) == 0; i-- {
			c++
		}
		return float64(c)
	})
	method(m, "hypot", 2, func(_ Value, args []Value) Value {
		s := 0.0
		for _, a := range args {
			n := ToNumber(a)
			if math.IsInf(n, 0) {
				return math.Inf(1)
			}
			s += n * n
		}
		return math.Sqrt(s)
	})
	method(m, "max", 2, func(_ Value, args []Value) Value {
		r := math.Inf(-1)
		for _, a := range args {
			n := ToNumber(a)
			switch {
			case math.IsNaN(n):
				r = n
			case math.IsNaN(r):
			case n > r || (n == 0 && r == 0 && !math.Signbit(n)):
				r = n
			}
		}
		return r
	})
	method(m, "min", 2, func(_ Value, args []Value) Value {
		r := math.Inf(1)
		for _, a := range args {
			n := ToNumber(a)
			switch {
			case math.IsNaN(n):
				r = n
			case math.IsNaN(r):
			case n < r || (n == 0 && r == 0 && math.Signbit(n)):
				r = n
			}
		}
		return r
	})
	// Rendering output that depends on Math.random can't hydrate; it still
	// works as it does in the engine.
	method(m, "random", 0, func(Value, []Value) Value { return rand.Float64() })
}

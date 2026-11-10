package js

import (
	"math"
	"math/bits"
	"strconv"
	"strings"
)

var nan = math.NaN()

func thisNumber(this Value, name string) float64 {
	switch x := this.(type) {
	case float64:
		return x
	case *Object:
		if n, ok := x.Internal.(float64); ok && x.Class == "Number" {
			return n
		}
	}
	panic(TypeErrorf("Number.prototype.%s requires that 'this' be a Number", name))
}

func (rt *Realm) setupNumber() {
	rt.NumberCtor = rt.NewConstructor("Number", 1, func(_ Value, args []Value) Value {
		if len(args) == 0 {
			return 0.0
		}
		return ToNumber(args[0])
	}, func(args []Value, nt *Object) Value {
		o := OrdinaryCreateFromConstructor(nt, rt.NumberPrototype)
		n := 0.0
		if len(args) > 0 {
			n = ToNumber(args[0])
		}
		o.Class, o.Internal = "Number", n
		return o
	}, rt.NumberPrototype)
	rt.NumberPrototype.Internal = 0.0
	c := rt.NumberCtor
	for name, v := range map[string]float64{
		"EPSILON": math.Pow(2, -52), "MAX_SAFE_INTEGER": 1<<53 - 1, "MIN_SAFE_INTEGER": -(1<<53 - 1),
		"MAX_VALUE": math.MaxFloat64, "MIN_VALUE": 5e-324, "POSITIVE_INFINITY": math.Inf(1),
		"NEGATIVE_INFINITY": math.Inf(-1), "NaN": nan,
	} {
		c.add(prop{key: name, value: v})
	}
	isInt := func(n float64) bool { return !math.IsInf(n, 0) && !math.IsNaN(n) && n == math.Trunc(n) }
	rt.method(c, "isInteger", 1, func(_ Value, args []Value) Value {
		n, ok := Arg(args, 0).(float64)
		return ok && isInt(n)
	})
	rt.method(c, "isSafeInteger", 1, func(_ Value, args []Value) Value {
		n, ok := Arg(args, 0).(float64)
		return ok && isInt(n) && math.Abs(n) <= 1<<53-1
	})
	rt.method(c, "isFinite", 1, func(_ Value, args []Value) Value {
		n, ok := Arg(args, 0).(float64)
		return ok && !math.IsInf(n, 0) && !math.IsNaN(n)
	})
	rt.method(c, "isNaN", 1, func(_ Value, args []Value) Value {
		n, ok := Arg(args, 0).(float64)
		return ok && math.IsNaN(n)
	})
	rt.method(c, "parseFloat", 1, func(_ Value, args []Value) Value { return parseFloat(ToString(Arg(args, 0))) })
	rt.method(c, "parseInt", 2, func(_ Value, args []Value) Value { return parseInt(ToString(Arg(args, 0)), Arg(args, 1)) })

	p := rt.NumberPrototype
	rt.method(p, "valueOf", 0, func(this Value, _ []Value) Value { return thisNumber(this, "valueOf") })
	rt.method(p, "toString", 1, func(this Value, args []Value) Value {
		x := thisNumber(this, "toString")
		radix := 10.0
		if r := Arg(args, 0); r != Undefined {
			radix = ToIntegerOrInfinity(r)
		}
		if radix < 2 || radix > 36 {
			panic(RangeErrorf("toString() radix must be between 2 and 36"))
		}
		if radix == 10 {
			return NumberToString(x)
		}
		return toRadix(x, int(radix))
	})
	// QuickJS's own toLocaleString is toString; Tern's polyfill replaces it.
	rt.method(p, "toLocaleString", 0, func(this Value, _ []Value) Value { return NumberToString(thisNumber(this, "toLocaleString")) })
	rt.method(p, "toFixed", 1, func(this Value, args []Value) Value {
		x := thisNumber(this, "toFixed")
		d := ToIntegerOrInfinity(Arg(args, 0))
		if d < 0 || d > 100 {
			panic(RangeErrorf("toFixed() digits argument must be between 0 and 100"))
		}
		return ToFixed(x, int(d))
	})
	rt.method(p, "toPrecision", 1, func(this Value, args []Value) Value {
		x := thisNumber(this, "toPrecision")
		if Arg(args, 0) == Undefined {
			return NumberToString(x)
		}
		pr := ToIntegerOrInfinity(Arg(args, 0))
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return NumberToString(x)
		}
		if pr < 1 || pr > 100 {
			panic(RangeErrorf("toPrecision() argument must be between 1 and 100"))
		}
		return toPrecision(x, int(pr))
	})
	rt.method(p, "toExponential", 1, func(this Value, args []Value) Value {
		x := thisNumber(this, "toExponential")
		f := ToIntegerOrInfinity(Arg(args, 0))
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return NumberToString(x)
		}
		if f < 0 || f > 100 {
			panic(RangeErrorf("toExponential() argument must be between 0 and 100"))
		}
		return toExponential(x, int(f), Arg(args, 0) == Undefined)
	})
}

// exactDigits returns the decimal digits of |x| rounded to the nearest
// value with n significant digits, ties away from zero as the spec's "if
// there are two such sets, choose the larger", and the decimal exponent
// of the first digit.
func exactDigits(x float64, n int) (string, int) {
	x = math.Abs(x)
	if x == 0 {
		return strings.Repeat("0", n), 0
	}
	// 'e' with plenty of digits is exact enough to decide every tie.
	s := strconv.FormatFloat(x, 'e', 767, 64)
	mant, expPart, _ := strings.Cut(s, "e")
	e, _ := strconv.Atoi(expPart)
	ds := []byte(strings.Replace(mant, ".", "", 1))
	keep := append([]byte(nil), ds[:n]...)
	if ds[n] >= '5' {
		i := n - 1
		for ; i >= 0 && keep[i] == '9'; i-- {
			keep[i] = '0'
		}
		if i < 0 {
			keep = append([]byte{'1'}, keep[:n-1]...)
			e++
		} else {
			keep[i]++
		}
	}
	return string(keep), e
}

// fixed formats x ≥ 0 with d fraction digits, rounding as toFixed does: to
// the nearest, and up on an exact tie. Go rounds to the nearest too, but
// to even on a tie. A tie is only possible when x's exact decimal
// expansion, which has as many fraction digits as x has fraction bits,
// ends right after the first dropped digit.
func fixed(x float64, d int) string {
	if fractionBits(x) != d+1 {
		return strconv.FormatFloat(x, 'f', d, 64)
	}
	exact := []byte(strconv.FormatFloat(x, 'f', d+1, 64)) // exact: it has d+1 digits
	keep := exact[:len(exact)-1]
	if d == 0 {
		keep = keep[:len(keep)-1] // the point
	}
	i := len(keep) - 1
	for ; i >= 0; i-- {
		if keep[i] == '.' {
			continue
		}
		if keep[i] != '9' {
			keep[i]++
			break
		}
		keep[i] = '0'
	}
	if i < 0 {
		return "1" + string(keep)
	}
	return string(keep)
}

// fractionBits is the number of binary digits after the point of x.
func fractionBits(x float64) int {
	b := math.Float64bits(x)
	exp := int(b >> 52 & 0x7ff)
	mant := b & (1<<52 - 1)
	e := -1074
	if exp != 0 {
		mant |= 1 << 52
		e = exp - 1075
	}
	if mant == 0 {
		return 0
	}
	e += bits.TrailingZeros64(mant)
	return max(0, -e)
}

// ToFixed is Number.prototype.toFixed.
func ToFixed(x float64, d int) string {
	if math.IsNaN(x) {
		return "NaN"
	}
	if math.Abs(x) >= 1e21 {
		return NumberToString(x)
	}
	neg := x < 0
	var s string
	if x == 0 {
		s = "0"
		if d > 0 {
			s += "." + strings.Repeat("0", d)
		}
	} else {
		s = fixed(math.Abs(x), d)
	}
	if neg {
		return "-" + s
	}
	return s
}

func toPrecision(x float64, p int) string {
	neg := x < 0
	var s string
	if x == 0 {
		s = "0"
		if p > 1 {
			s += "." + strings.Repeat("0", p-1)
		}
	} else {
		ds, e := exactDigits(x, p)
		switch {
		case e < -6 || e >= p:
			s = ds[:1]
			if p > 1 {
				s += "." + ds[1:]
			}
			sign := "+"
			if e < 0 {
				sign, e = "-", -e
			}
			s += "e" + sign + strconv.Itoa(e)
		case e == p-1:
			s = ds
		case e >= 0:
			s = ds[:e+1] + "." + ds[e+1:]
		default:
			s = "0." + strings.Repeat("0", -(e+1)) + ds
		}
	}
	if neg && x != 0 {
		return "-" + s
	}
	return s
}

func toExponential(x float64, f int, auto bool) string {
	neg := x < 0
	var ds string
	var e int
	switch {
	case x == 0:
		ds, e = strings.Repeat("0", f+1), 0
	case auto:
		sh := strconv.FormatFloat(math.Abs(x), 'e', -1, 64)
		mant, expPart, _ := strings.Cut(sh, "e")
		ds = strings.Replace(mant, ".", "", 1)
		e, _ = strconv.Atoi(expPart)
	default:
		ds, e = exactDigits(x, f+1)
	}
	s := ds[:1]
	if len(ds) > 1 {
		s += "." + ds[1:]
	}
	sign := "+"
	if e < 0 {
		sign, e = "-", -e
	}
	s += "e" + sign + strconv.Itoa(e)
	if neg {
		return "-" + s
	}
	return s
}

// toRadix prints a number in another base, as V8 and QuickJS do: the
// integer part exactly, and up to 52 bits of fraction.
func toRadix(x float64, radix int) string {
	if math.IsNaN(x) {
		return "NaN"
	}
	if math.IsInf(x, 0) {
		if x > 0 {
			return "Infinity"
		}
		return "-Infinity"
	}
	neg := x < 0
	x = math.Abs(x)
	ip := math.Floor(x)
	fp := x - ip
	const digits = "0123456789abcdefghijklmnopqrstuvwxyz"
	var ib []byte
	if ip == 0 {
		ib = []byte{'0'}
	}
	for ip > 0 {
		d := math.Mod(ip, float64(radix))
		ib = append(ib, digits[int(d)])
		ip = math.Floor(ip / float64(radix))
	}
	for i, j := 0, len(ib)-1; i < j; i, j = i+1, j-1 {
		ib[i], ib[j] = ib[j], ib[i]
	}
	s := string(ib)
	if fp > 0 {
		var fb []byte
		// Enough digits to tell this double from its neighbors.
		delta := math.Max(math.Nextafter(x, math.Inf(1))-x, 5e-324) / 2
		for fp >= delta && len(fb) < 1100 {
			fp *= float64(radix)
			delta *= float64(radix)
			d := int(fp)
			fb = append(fb, digits[d])
			fp -= float64(d)
			if fp > 0.5 || (fp == 0.5 && d&1 == 1) {
				if fp+delta > 1 {
					// Round up, carrying.
					for i := len(fb) - 1; ; i-- {
						if i < 0 {
							s = toRadix(math.Floor(x)+1, radix)
							break
						}
						k := strings.IndexByte(digits, fb[i]) + 1
						if k < radix {
							fb[i] = digits[k]
							break
						}
						fb = fb[:i]
					}
					break
				}
			}
		}
		if len(fb) > 0 {
			s += "." + string(fb)
		}
	}
	if neg {
		return "-" + s
	}
	return s
}

func parseFloat(s string) Value {
	s = strings.TrimLeftFunc(s, isJSSpace)
	// The longest prefix that is a StrDecimalLiteral.
	end, sawDigit := 0, false
	i := 0
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		i++
	}
	if strings.HasPrefix(s[i:], "Infinity") {
		if s[0] == '-' {
			return math.Inf(-1)
		}
		return math.Inf(1)
	}
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
		sawDigit = true
		end = i
	}
	if i < len(s) && s[i] == '.' {
		i++
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
			sawDigit = true
			end = i
		}
	}
	if !sawDigit {
		return nan
	}
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		j := i + 1
		if j < len(s) && (s[j] == '+' || s[j] == '-') {
			j++
		}
		k := j
		for k < len(s) && s[k] >= '0' && s[k] <= '9' {
			k++
		}
		if k > j {
			end = k
		}
	}
	f, _ := strconv.ParseFloat(s[:end], 64) // the scan stopped before a lone "."
	return f
}

func parseInt(s string, radixArg Value) Value {
	s = TrimJSSpace(s)
	neg := false
	if s != "" && (s[0] == '+' || s[0] == '-') {
		neg = s[0] == '-'
		s = s[1:]
	}
	radix := int(ToInt32(radixArg))
	strip := true
	if radix != 0 {
		if radix < 2 || radix > 36 {
			return nan
		}
		if radix != 16 {
			strip = false
		}
	} else {
		radix = 10
	}
	if strip && len(s) >= 2 && s[0] == '0' && (s[1] == 'x' || s[1] == 'X') {
		s = s[2:]
		radix = 16
	}
	end := 0
	for end < len(s) {
		d := digitVal(rune(s[end]))
		if d < 0 || d >= radix {
			break
		}
		end++
	}
	if end == 0 {
		return nan
	}
	var n float64
	if radix == 10 {
		n, _ = strconv.ParseFloat(s[:end], 64) // correctly rounded, however many digits
	} else {
		for _, c := range s[:end] {
			n = n*float64(radix) + float64(digitVal(c))
		}
	}
	if neg {
		return -n
	}
	return n
}

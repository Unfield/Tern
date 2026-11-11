package js

import (
	"errors"
	"math"
	"math/big"
	"strconv"
	"strings"
	"unicode/utf8"
)

// ToPrimitive with a hint of "default", "number" or "string".
func ToPrimitive(v Value, hint string) Value {
	o, ok := v.(*Object)
	if !ok {
		return v
	}
	if ex, ok := o.Get(SymbolToPrimitive).(*Object); ok && ex.call != nil {
		r := ex.invoke(o, hint)
		if _, isObj := r.(*Object); isObj {
			panic(TypeErrorf("Cannot convert object to primitive value"))
		}
		return r
	}
	order := [2]string{"valueOf", "toString"}
	if hint == "string" {
		order = [2]string{"toString", "valueOf"}
	}
	for _, name := range order {
		if f, ok := o.Get(name).(*Object); ok && f.call != nil {
			r := f.invoke(o)
			if _, isObj := r.(*Object); !isObj {
				return r
			}
		}
	}
	panic(TypeErrorf("Cannot convert object to primitive value"))
}

// ToString is the abstract operation ToString.
func ToString(v Value) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return NumberToString(x)
	case bool:
		if x {
			return "true"
		}
		return "false"
	case undefinedType:
		return "undefined"
	case nullType:
		return "null"
	case *Symbol:
		panic(TypeErrorf("Cannot convert a Symbol value to a string"))
	case *big.Int:
		return x.String()
	case *Object:
		return ToString(ToPrimitive(x, "string"))
	}
	panic(TypeErrorf("unknown value of Go type %T", v))
}

// intKeys holds the small integers' strings as Values, for indexing.
var intKeys = func() []Value {
	out := make([]Value, 1024)
	for i := range out {
		out[i] = strconv.Itoa(i)
	}
	return out
}()

var smallInts = func() []string {
	out := make([]string, 1024)
	for i := range out {
		out[i] = strconv.Itoa(i)
	}
	return out
}()

// NumberToString is Number::toString(x) with radix 10.
func NumberToString(x float64) string {
	switch {
	case math.IsNaN(x):
		return "NaN"
	case math.IsInf(x, 1):
		return "Infinity"
	case math.IsInf(x, -1):
		return "-Infinity"
	case x == 0:
		return "0"
	}
	if x == math.Trunc(x) && math.Abs(x) < 1e21 {
		if math.Abs(x) < 1<<53 {
			if x >= 0 && x < float64(len(smallInts)) {
				return smallInts[int(x)]
			}
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	}
	digits := strconv.FormatFloat(math.Abs(x), 'e', -1, 64)
	mant, expPart, _ := strings.Cut(digits, "e")
	exp, _ := strconv.Atoi(expPart)
	ds := strings.Replace(mant, ".", "", 1)
	k := len(ds)
	n := exp + 1
	var out string
	switch {
	case k <= n && n <= 21:
		out = ds + strings.Repeat("0", n-k)
	case 0 < n && n <= 21:
		out = ds[:n] + "." + ds[n:]
	case -6 < n && n <= 0:
		out = "0." + strings.Repeat("0", -n) + ds
	default:
		e := n - 1
		sign := "+"
		if e < 0 {
			sign, e = "-", -e
		}
		if k == 1 {
			out = ds + "e" + sign + strconv.Itoa(e)
		} else {
			out = ds[:1] + "." + ds[1:] + "e" + sign + strconv.Itoa(e)
		}
	}
	if x < 0 {
		return "-" + out
	}
	return out
}

// ToNumber is the abstract operation ToNumber.
func ToNumber(v Value) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case bool:
		if x {
			return 1
		}
		return 0
	case string:
		return StringToNumber(x)
	case *big.Int:
		panic(TypeErrorf("Cannot convert a BigInt value to a number"))
	case undefinedType:
		return math.NaN()
	case nullType:
		return 0
	case *Symbol:
		panic(TypeErrorf("Cannot convert a Symbol value to a number"))
	case *Object:
		return ToNumber(ToPrimitive(x, "number"))
	}
	panic(TypeErrorf("unknown value of Go type %T", v))
}

// isJSSpace reports StrWhiteSpaceChar: WhiteSpace and LineTerminator.
func isJSSpace(r rune) bool {
	switch r {
	case '\t', '\n', '\v', '\f', '\r', ' ', 0xa0, 0x1680, 0x2028, 0x2029, 0x202f, 0x205f, 0x3000, 0xfeff:
		return true
	}
	return r >= 0x2000 && r <= 0x200a
}

// TrimJSSpace trims what String.prototype.trim trims.
func TrimJSSpace(s string) string { return strings.TrimFunc(s, isJSSpace) }

// StringToNumber is StringToNumber: decimal, 0x/0o/0b integers, Infinity,
// and the empty string as 0.
func StringToNumber(s string) float64 {
	s = TrimJSSpace(s)
	if s == "" {
		return 0
	}
	if len(s) > 2 && s[0] == '0' {
		base := 0
		switch s[1] {
		case 'x', 'X':
			base = 16
		case 'o', 'O':
			base = 8
		case 'b', 'B':
			base = 2
		}
		if base != 0 {
			n := 0.0
			for _, c := range s[2:] {
				d := digitVal(c)
				if d < 0 || d >= base {
					return math.NaN()
				}
				n = n*float64(base) + float64(d)
			}
			return n
		}
	}
	switch s {
	case "Infinity", "+Infinity":
		return math.Inf(1)
	case "-Infinity":
		return math.Inf(-1)
	}
	// The StrDecimalLiteral grammar: digits, one dot, an exponent. Go's
	// ParseFloat also takes "inf", hex floats and underscores, so check first.
	if !isDecimalLiteral(s) {
		return math.NaN()
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		if errors.Is(err, strconv.ErrRange) {
			return f // ±Inf or 0, as JavaScript rounds
		}
		return math.NaN()
	}
	return f
}

func isDecimalLiteral(s string) bool {
	i := 0
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		i++
	}
	digits := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
		digits++
	}
	if i < len(s) && s[i] == '.' {
		i++
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
			digits++
		}
	}
	if digits == 0 {
		return false
	}
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		i++
		if i < len(s) && (s[i] == '+' || s[i] == '-') {
			i++
		}
		start := i
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
		if i == start {
			return false
		}
	}
	return i == len(s)
}

func digitVal(c rune) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'z':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'Z':
		return int(c-'A') + 10
	}
	return -1
}

// ToIntegerOrInfinity is ToIntegerOrInfinity.
func ToIntegerOrInfinity(v Value) float64 {
	n := ToNumber(v)
	if math.IsNaN(n) {
		return 0
	}
	return math.Trunc(n) + 0 // + 0 turns -0 into 0
}

// ToInt32 is ToInt32, for the bitwise operators.
func ToInt32(v Value) int32 {
	if _, ok := v.(*big.Int); ok {
		panic(TypeErrorf("Cannot mix BigInt and other types, use explicit conversions"))
	}
	n := ToNumber(v)
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return 0
	}
	return int32(uint32(int64(math.Mod(math.Trunc(n), 4294967296))))
}

// ToUint32 is ToUint32.
func ToUint32(v Value) uint32 { return uint32(ToInt32(v)) }

// ToPropertyKey is ToPropertyKey.
func ToPropertyKey(v Value) PropKey {
	switch x := v.(type) {
	case string, *Symbol:
		return v // as it is: boxing it again would allocate
	case float64:
		if i := int(x); float64(i) == x && i >= 0 && i < len(intKeys) {
			return intKeys[i]
		}
		return NumberToString(x)
	}
	p := ToPrimitive(v, "string")
	if s, ok := p.(*Symbol); ok {
		return s
	}
	return ToString(p)
}

// ToStringValue is ToString as a Value: a string comes back as it is,
// without the allocation of boxing it again.
func ToStringValue(v Value) Value {
	if _, ok := v.(string); ok {
		return v
	}
	return ToString(v)
}

// ToObject boxes primitives, for property access and Object(...).
func (rt *Realm) ToObject(v Value) *Object {
	switch x := v.(type) {
	case *Object:
		return x
	case undefinedType, nullType:
		panic(TypeErrorf("Cannot convert undefined or null to object"))
	case string:
		o := NewObject(rt.StringPrototype)
		rt.stringData(o, x)
		return o
	case float64:
		o := NewObject(rt.NumberPrototype)
		o.Class, o.Internal = "Number", x
		return o
	case bool:
		o := NewObject(rt.BooleanPrototype)
		o.Class, o.Internal = "Boolean", x
		return o
	case *Symbol:
		o := NewObject(rt.SymbolPrototype)
		o.Class, o.Internal = "Symbol", x
		return o
	case *big.Int:
		o := NewObject(rt.BigIntPrototype)
		o.Class, o.Internal = "BigInt", x
		return o
	}
	panic(TypeErrorf("unknown value of Go type %T", v))
}

// isASCII reports whether s has only ASCII, where UTF-16 and UTF-8 indexes agree.
func (rt *Realm) isASCII(s string) bool {
	if len(s) >= 32 {
		return rt.strInfo(s).ascii
	}
	return scanASCII(s)
}

func scanASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

// stringData makes o a String object of s: its index properties, one per
// UTF-16 unit, enumerable and read-only, and its length, as a String
// exotic object has them.
func (rt *Realm) stringData(o *Object, s string) {
	o.Class, o.Internal = "String", s
	n := rt.Len16(s)
	for i := range n {
		o.add(prop{key: strconv.Itoa(i), value: rt.charAt(s, i), enumerable: true})
	}
	o.add(prop{key: "length", value: float64(n)})
}

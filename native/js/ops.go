package js

import (
	"math"
	"strconv"
)

// Get is v[key] for any value: objects, and primitives through their
// prototypes.
func Get(v Value, key PropKey) Value {
	switch x := v.(type) {
	case *Object:
		return x.Get(key)
	case string:
		if key == "length" {
			return Box(float64(Len16(x)))
		}
		if i, ok := arrayIndex(key); ok {
			if u := UnitAt(x, i); u >= 0 {
				return charAt(x, i)
			}
			return Undefined
		}
		return StringPrototype.GetFrom(key, v)
	case float64:
		return NumberPrototype.GetFrom(key, v)
	case bool:
		return BooleanPrototype.GetFrom(key, v)
	case *Symbol:
		if key == "description" {
			return x.Description
		}
		return SymbolPrototype.GetFrom(key, v)
	case undefinedType, nullType:
		panic(TypeErrorf("Cannot read properties of %s (reading '%s')", ToString(v), keyName(key)))
	}
	panic(TypeErrorf("unknown value of Go type %T", v))
}

// asciiChars holds the one-character ASCII strings as Values.
var asciiChars = func() (out [128]Value) {
	for i := range out {
		out[i] = string(rune(i))
	}
	return
}()

func charAt(s string, i int) Value {
	if isASCII(s) {
		return asciiChars[s[i]]
	}
	u := units16(s)[i]
	if u < 128 {
		return asciiChars[u]
	}
	return FromUnits([]uint16{u})
}

// GetV is v[k] where k is any value, with a fast path for array indexes.
func GetV(v Value, k Value) Value {
	if str, ok := v.(string); ok {
		if n, ok := k.(float64); ok {
			if i := int(n); float64(i) == n && i >= 0 {
				if UnitAt(str, i) >= 0 {
					return charAt(str, i)
				}
				return Undefined
			}
		}
	}
	if o, ok := v.(*Object); ok && o.typed != nil {
		if n, ok := k.(float64); ok {
			if i := int(n); float64(i) == n && i >= 0 && i < o.typed.length {
				return o.typed.at(i)
			}
		}
	}
	if o, ok := v.(*Object); ok && o.array != nil {
		if n, ok := k.(float64); ok {
			if i := int(n); float64(i) == n && i >= 0 {
				if a := *o.array; i < len(a) {
					return a[i]
				}
				return o.proto.Get(NumberToString(n))
			}
		}
	}
	return Get(v, ToPropertyKey(k))
}

// GetOptional is v?.[key]: undefined when v is nullish.
func GetOptional(v Value, key PropKey) Value {
	if IsNullish(v) {
		return Undefined
	}
	return Get(v, key)
}

// Set is v[key] = val in strict mode code.
func Set(v Value, key PropKey, val Value) {
	o, ok := v.(*Object)
	if !ok {
		if IsNullish(v) {
			panic(TypeErrorf("Cannot set properties of %s (setting '%s')", ToString(v), keyName(key)))
		}
		panic(TypeErrorf("Cannot create property '%s' on %s '%s'", keyName(key), Typeof(v), ToString(v)))
	}
	o.Set(key, val)
}

// SetV is v[k] = val where k is any value.
func SetV(v Value, k Value, val Value) {
	if o, ok := v.(*Object); ok && o.typed != nil {
		if n, ok := k.(float64); ok {
			if i := int(n); float64(i) == n && i >= 0 && i < o.typed.length {
				o.typed.put(i, val)
				return
			}
		}
	}
	if o, ok := v.(*Object); ok && o.array != nil && !o.frozen {
		if n, ok := k.(float64); ok {
			if i := int(n); float64(i) == n && i >= 0 && i < len(*o.array) {
				(*o.array)[i] = val
				return
			}
		}
	}
	Set(v, ToPropertyKey(k), val)
}

// Add is the + operator.
func Add(a, b Value) Value {
	if x, ok := a.(float64); ok {
		if y, ok := b.(float64); ok {
			return Box(x + y)
		}
	}
	if x, ok := a.(string); ok {
		if y, ok := b.(string); ok {
			return Concat(x, y)
		}
	}
	pa, pb := ToPrimitive(a, "default"), ToPrimitive(b, "default")
	if sa, ok := pa.(string); ok {
		return Concat(sa, ToString(pb))
	}
	if sb, ok := pb.(string); ok {
		return Concat(ToString(pa), sb)
	}
	return Box(ToNumber(pa) + ToNumber(pb))
}

// Sub is the - operator.
func Sub(a, b Value) Value { return Box(ToNumber(a) - ToNumber(b)) }

// Mul is the * operator.
func Mul(a, b Value) Value { return Box(ToNumber(a) * ToNumber(b)) }

// Div is the / operator.
func Div(a, b Value) Value { return Box(ToNumber(a) / ToNumber(b)) }

// Mod is the % operator: the result has the dividend's sign.
func Mod(a, b Value) Value {
	x, y := ToNumber(a), ToNumber(b)
	if math.IsInf(y, 0) && !math.IsInf(x, 0) {
		return x
	}
	return math.Mod(x, y)
}

// Pow is **, where JavaScript differs from math.Pow: 1 ** NaN and
// (-1) ** ±Infinity are NaN.
func Pow(a, b Value) Value {
	x, y := ToNumber(a), ToNumber(b)
	if math.IsNaN(y) {
		return math.NaN()
	}
	if math.Abs(x) == 1 && math.IsInf(y, 0) {
		return math.NaN()
	}
	return math.Pow(x, y)
}

// Neg is unary -.
func Neg(a Value) Value { return -ToNumber(a) }

// Plus is unary +.
func Plus(a Value) Value { return Box(ToNumber(a)) }

// BitAnd is &.
func BitAnd(a, b Value) Value { return Box(float64(ToInt32(a) & ToInt32(b))) }

// BitOr is |.
func BitOr(a, b Value) Value { return Box(float64(ToInt32(a) | ToInt32(b))) }

// BitXor is ^.
func BitXor(a, b Value) Value { return Box(float64(ToInt32(a) ^ ToInt32(b))) }

// BitNot is ~.
func BitNot(a Value) Value { return Box(float64(^ToInt32(a))) }

// Shl is <<.
func Shl(a, b Value) Value { return Box(float64(ToInt32(a) << (ToUint32(b) & 31))) }

// Shr is >>.
func Shr(a, b Value) Value { return Box(float64(ToInt32(a) >> (ToUint32(b) & 31))) }

// UShr is >>>.
func UShr(a, b Value) Value { return Box(float64(ToUint32(a) >> (ToUint32(b) & 31))) }

// StrictEquals is ===.
func StrictEquals(a, b Value) bool {
	switch x := a.(type) {
	case float64:
		y, ok := b.(float64)
		return ok && x == y // NaN != NaN, and 0 == -0
	case string:
		y, ok := b.(string)
		return ok && x == y
	}
	return a == b
}

// SameValueZero is SameValueZero, for includes and Map keys.
func SameValueZero(a, b Value) bool {
	if x, ok := a.(float64); ok {
		if y, ok := b.(float64); ok {
			return x == y || (math.IsNaN(x) && math.IsNaN(y))
		}
		return false
	}
	return StrictEquals(a, b)
}

// SameValue is Object.is.
func SameValue(a, b Value) bool {
	if x, ok := a.(float64); ok {
		if y, ok := b.(float64); ok {
			if x == 0 && y == 0 {
				return math.Signbit(x) == math.Signbit(y)
			}
			return x == y || (math.IsNaN(x) && math.IsNaN(y))
		}
		return false
	}
	return StrictEquals(a, b)
}

// LooseEquals is ==.
func LooseEquals(a, b Value) bool {
	switch {
	case sameType(a, b):
		return StrictEquals(a, b)
	case IsNullish(a) && IsNullish(b):
		return true
	case IsNullish(a) || IsNullish(b):
		return false
	}
	switch x := a.(type) {
	case float64:
		if s, ok := b.(string); ok {
			return x == StringToNumber(s)
		}
	case string:
		if y, ok := b.(float64); ok {
			return StringToNumber(x) == y
		}
	case bool:
		return LooseEquals(boolNum(x), b)
	}
	if y, ok := b.(bool); ok {
		return LooseEquals(a, boolNum(y))
	}
	_, aObj := a.(*Object)
	_, bObj := b.(*Object)
	if bObj && !aObj { // a is a string, number or symbol here
		return LooseEquals(a, ToPrimitive(b, "default"))
	}
	if aObj && !bObj {
		return LooseEquals(ToPrimitive(a, "default"), b)
	}
	return false
}

func boolNum(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

func sameType(a, b Value) bool {
	switch a.(type) {
	case float64:
		_, ok := b.(float64)
		return ok
	case string:
		_, ok := b.(string)
		return ok
	case bool:
		_, ok := b.(bool)
		return ok
	case undefinedType:
		return b == Undefined
	case nullType:
		return b == Null
	case *Object:
		_, ok := b.(*Object)
		return ok
	case *Symbol:
		_, ok := b.(*Symbol)
		return ok
	}
	return false
}

// lessThan is IsLessThan: it returns -1 for undefined (a NaN was involved),
// 0 for false and 1 for true.
func lessThan(a, b Value, leftFirst bool) int {
	var pa, pb Value
	if leftFirst {
		pa, pb = ToPrimitive(a, "number"), ToPrimitive(b, "number")
	} else {
		pb, pa = ToPrimitive(b, "number"), ToPrimitive(a, "number")
	}
	if sa, ok := pa.(string); ok {
		if sb, ok := pb.(string); ok {
			if CompareStrings(sa, sb) < 0 {
				return 1
			}
			return 0
		}
	}
	x, y := ToNumber(pa), ToNumber(pb)
	if math.IsNaN(x) || math.IsNaN(y) {
		return -1
	}
	if x < y {
		return 1
	}
	return 0
}

// Less is <.
func Less(a, b Value) bool {
	if x, ok := a.(float64); ok {
		if y, ok := b.(float64); ok {
			return x < y
		}
	}
	return lessThan(a, b, true) == 1
}

// Greater is >.
func Greater(a, b Value) bool {
	if x, ok := a.(float64); ok {
		if y, ok := b.(float64); ok {
			return x > y
		}
	}
	return lessThan(b, a, false) == 1
}

// LessEq is <=.
func LessEq(a, b Value) bool {
	if x, ok := a.(float64); ok {
		if y, ok := b.(float64); ok {
			return x <= y
		}
	}
	return lessThan(b, a, false) == 0
}

// GreaterEq is >=.
func GreaterEq(a, b Value) bool {
	if x, ok := a.(float64); ok {
		if y, ok := b.(float64); ok {
			return x >= y
		}
	}
	return lessThan(a, b, true) == 0
}

// InstanceOf is instanceof.
func InstanceOf(v Value, ctor Value) bool {
	c, ok := ctor.(*Object)
	if !ok || c.call == nil {
		panic(TypeErrorf("Right-hand side of 'instanceof' is not callable"))
	}
	if h, ok := c.Get(symbolHasInstance).(*Object); ok && h.call != nil && h != defaultHasInstance {
		return Truthy(h.invoke(c, v))
	}
	o, ok := v.(*Object)
	if !ok {
		return false
	}
	proto, ok := c.Get("prototype").(*Object)
	if !ok {
		panic(TypeErrorf("Function has non-object prototype in instanceof check"))
	}
	for p := o.proto; p != nil; p = p.proto {
		if p == proto {
			return true
		}
	}
	return false
}

var symbolHasInstance = &Symbol{"Symbol.hasInstance"}
var defaultHasInstance *Object

// In is the in operator.
func In(key Value, obj Value) bool {
	o, ok := obj.(*Object)
	if !ok {
		panic(TypeErrorf("Cannot use 'in' operator to search for '%s' in %s", ToString(key), ToString(obj)))
	}
	return o.Has(ToPropertyKey(key))
}

// Delete is the delete operator on v[key].
func Delete(v Value, key PropKey) bool {
	o, ok := v.(*Object)
	if !ok {
		if IsNullish(v) {
			panic(TypeErrorf("Cannot convert undefined or null to object"))
		}
		return true
	}
	return o.Delete(key)
}

// Nullish is a ?? b's test.
func Nullish(v Value) bool { return IsNullish(v) }

// Itoa formats an integer index as a property key.
func Itoa(i int) string {
	if i >= 0 && i < len(smallInts) {
		return smallInts[i]
	}
	return strconv.Itoa(i)
}

// Inc is x + 1 for x++ and ++x.
func Inc(v Value) Value {
	if x, ok := v.(float64); ok {
		return Box(x + 1)
	}
	return Box(ToNumber(v) + 1)
}

// Dec is x - 1 for x-- and --x.
func Dec(v Value) Value {
	if x, ok := v.(float64); ok {
		return Box(x - 1)
	}
	return Box(ToNumber(v) - 1)
}

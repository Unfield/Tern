package js

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

func thisString(this Value, name string) string {
	switch x := this.(type) {
	case string:
		return x
	case undefinedType, nullType:
		panic(TypeErrorf("String.prototype.%s called on null or undefined", name))
	case *Object:
		if s, ok := x.Internal.(string); ok && x.Class == "String" {
			return s
		}
	}
	return ToString(this)
}

func (rt *Realm) setupString() {
	rt.StringCtor = rt.NewConstructor("String", 1, func(_ Value, args []Value) Value {
		if len(args) == 0 {
			return ""
		}
		if s, ok := args[0].(*Symbol); ok {
			return "Symbol(" + s.Description + ")"
		}
		return ToStringValue(args[0])
	}, func(args []Value, nt *Object) Value {
		o := OrdinaryCreateFromConstructor(nt, rt.StringPrototype)
		s := ""
		if len(args) > 0 {
			s = ToString(args[0])
		}
		rt.stringData(o, s)
		return o
	}, rt.StringPrototype)
	rt.StringPrototype.Internal = ""
	rt.method(rt.StringCtor, "fromCharCode", 1, func(_ Value, args []Value) Value {
		u := make([]uint16, len(args))
		for i, a := range args {
			u[i] = uint16(ToUint32(a))
		}
		return FromUnits(u)
	})
	rt.method(rt.StringCtor, "fromCodePoint", 1, func(_ Value, args []Value) Value {
		var b strings.Builder
		for _, a := range args {
			n := ToNumber(a)
			if n < 0 || n > 0x10FFFF || n != float64(int(n)) {
				panic(RangeErrorf("Invalid code point %s", ToString(a)))
			}
			b.WriteString(FromUnits(Units(string(rune(int(n))))))
		}
		return b.String()
	})
	rt.method(rt.StringCtor, "raw", 1, func(_ Value, args []Value) Value {
		strs := ListFromArrayLike(rt.Get(Arg(args, 0), "raw"))
		var b strings.Builder
		for i, s := range strs {
			b.WriteString(ToString(s))
			if i+1 < len(strs) && i+1 < len(args) {
				b.WriteString(ToString(args[i+1]))
			}
		}
		return b.String()
	})

	p := rt.StringPrototype
	str := func(name string, length int, fn func(s string, args []Value) Value) {
		rt.method(p, name, length, func(this Value, args []Value) Value { return fn(thisString(this, name), args) })
	}
	str("toString", 0, func(s string, _ []Value) Value { return s })
	str("valueOf", 0, func(s string, _ []Value) Value { return s })
	str("at", 1, func(s string, args []Value) Value {
		n := rt.Len16(s)
		i := int(ToIntegerOrInfinity(Arg(args, 0)))
		if i < 0 {
			i += n
		}
		if i < 0 || i >= n {
			return Undefined
		}
		return rt.charAt(s, i)
	})
	str("charAt", 1, func(s string, args []Value) Value {
		i := ToIntegerOrInfinity(Arg(args, 0))
		if i < 0 || int(i) >= rt.Len16(s) {
			return ""
		}
		return rt.charAt(s, int(i))
	})
	str("charCodeAt", 1, func(s string, args []Value) Value {
		u := rt.UnitAt(s, int(ToIntegerOrInfinity(Arg(args, 0))))
		if u < 0 {
			return nan
		}
		return float64(u)
	})
	str("codePointAt", 1, func(s string, args []Value) Value {
		u := Units(s)
		i := int(ToIntegerOrInfinity(Arg(args, 0)))
		if i < 0 || i >= len(u) {
			return Undefined
		}
		c := u[i]
		if c >= 0xD800 && c <= 0xDBFF && i+1 < len(u) && u[i+1] >= 0xDC00 && u[i+1] <= 0xDFFF {
			return float64((rune(c)-0xD800)<<10 + (rune(u[i+1]) - 0xDC00) + 0x10000)
		}
		return float64(c)
	})
	str("concat", 1, func(s string, args []Value) Value {
		for _, a := range args {
			s += ToString(a)
		}
		return s
	})
	str("endsWith", 1, func(s string, args []Value) Value {
		sub := searchString(Arg(args, 0), "endsWith")
		n := rt.Len16(s)
		end := n
		if e := Arg(args, 1); e != Undefined {
			end = int(max(0, min(ToIntegerOrInfinity(e), float64(n))))
		}
		start := end - rt.Len16(sub)
		return start >= 0 && rt.Slice16(s, start, end) == sub
	})
	str("startsWith", 1, func(s string, args []Value) Value {
		sub := searchString(Arg(args, 0), "startsWith")
		n := rt.Len16(s)
		start := int(max(0, min(ToIntegerOrInfinity(Arg(args, 1)), float64(n))))
		end := start + rt.Len16(sub)
		return end <= n && rt.Slice16(s, start, end) == sub
	})
	str("includes", 1, func(s string, args []Value) Value {
		sub := searchString(Arg(args, 0), "includes")
		return rt.Index16(s, sub, int(max(0, ToIntegerOrInfinity(Arg(args, 1))))) >= 0
	})
	str("indexOf", 1, func(s string, args []Value) Value {
		return float64(rt.Index16(s, ToString(Arg(args, 0)), int(max(0, ToIntegerOrInfinity(Arg(args, 1))))))
	})
	str("lastIndexOf", 1, func(s string, args []Value) Value {
		u, v := Units(s), Units(ToString(Arg(args, 0)))
		from := len(u)
		if n := ToNumber(Arg(args, 1)); n == n {
			from = int(max(0, min(ToIntegerOrInfinity(n), float64(len(u)))))
		}
		for i := min(from, len(u)-len(v)); i >= 0; i-- {
			match := true
			for j := range v {
				if u[i+j] != v[j] {
					match = false
					break
				}
			}
			if match {
				return float64(i)
			}
		}
		return -1.0
	})
	str("slice", 2, func(s string, args []Value) Value {
		n := rt.Len16(s)
		start := relIndex(Arg(args, 0), n, 0)
		end := relIndex(Arg(args, 1), n, n)
		return rt.Slice16(s, start, end)
	})
	str("substring", 2, func(s string, args []Value) Value {
		n := rt.Len16(s)
		clamp := func(v Value, def int) int {
			if v == Undefined {
				return def
			}
			return int(max(0, min(ToIntegerOrInfinity(v), float64(n))))
		}
		a, b := clamp(Arg(args, 0), 0), clamp(Arg(args, 1), n)
		if a > b {
			a, b = b, a
		}
		return rt.Slice16(s, a, b)
	})
	str("substr", 2, func(s string, args []Value) Value {
		n := rt.Len16(s)
		start := relIndex(Arg(args, 0), n, 0)
		l := n - start
		if v := Arg(args, 1); v != Undefined {
			l = int(max(0, min(ToIntegerOrInfinity(v), float64(n-start))))
		}
		return rt.Slice16(s, start, start+l)
	})
	str("toUpperCase", 0, func(s string, _ []Value) Value { return rt.jsUpper(s) })
	str("toLowerCase", 0, func(s string, _ []Value) Value { return rt.jsLower(s) })
	str("toLocaleUpperCase", 0, func(s string, _ []Value) Value { return rt.jsUpper(s) })
	str("toLocaleLowerCase", 0, func(s string, _ []Value) Value { return rt.jsLower(s) })
	str("trim", 0, func(s string, _ []Value) Value { return TrimJSSpace(s) })
	str("trimStart", 0, func(s string, _ []Value) Value { return strings.TrimLeftFunc(s, isJSSpace) })
	str("trimEnd", 0, func(s string, _ []Value) Value { return strings.TrimRightFunc(s, isJSSpace) })
	str("padStart", 2, func(s string, args []Value) Value { return rt.pad(s, args, true) })
	str("padEnd", 2, func(s string, args []Value) Value { return rt.pad(s, args, false) })
	str("repeat", 1, func(s string, args []Value) Value {
		n := ToIntegerOrInfinity(Arg(args, 0))
		if n < 0 || n > 1<<28 {
			panic(RangeErrorf("Invalid count value: %s", ToString(Arg(args, 0))))
		}
		return strings.Repeat(s, int(n))
	})
	str("normalize", 0, func(s string, args []Value) Value {
		form := "NFC"
		if f := Arg(args, 0); f != Undefined {
			form = ToString(f)
		}
		switch form {
		case "NFC":
			return norm.NFC.String(s)
		case "NFD":
			return norm.NFD.String(s)
		case "NFKC":
			return norm.NFKC.String(s)
		case "NFKD":
			return norm.NFKD.String(s)
		}
		panic(RangeErrorf("The normalization form should be one of NFC, NFD, NFKC, NFKD."))
	})
	str("localeCompare", 1, func(s string, args []Value) Value {
		return float64(rt.localeCompare(s, ToString(Arg(args, 0))))
	})
	str("split", 2, func(s string, args []Value) Value {
		sep := Arg(args, 0)
		if r, ok := sep.(*Object); ok && r.Class == "RegExp" {
			return rt.regexpSplit(r, s, Arg(args, 1))
		}
		limit := 1<<32 - 1
		if l := Arg(args, 1); l != Undefined {
			limit = int(ToUint32(l))
		}
		if sep == Undefined {
			return rt.NewArray([]Value{s})
		}
		ss := ToString(sep)
		var parts []string
		if ss == "" {
			u := Units(s)
			for i := range u {
				parts = append(parts, FromUnits(u[i:i+1]))
			}
		} else {
			parts = strings.Split(s, ss)
		}
		out := make([]Value, 0, len(parts))
		for _, part := range parts {
			if len(out) >= limit {
				break
			}
			out = append(out, part)
		}
		return rt.NewArray(out)
	})
	str("replace", 2, func(s string, args []Value) Value { return rt.replace(s, Arg(args, 0), Arg(args, 1), false) })
	str("replaceAll", 2, func(s string, args []Value) Value { return rt.replace(s, Arg(args, 0), Arg(args, 1), true) })
	str("match", 1, func(s string, args []Value) Value { return rt.regexpMatch(rt.toRegExp(Arg(args, 0), ""), s) })
	str("matchAll", 1, func(s string, args []Value) Value {
		r := rt.toRegExp(Arg(args, 0), "g")
		if !strings.Contains(r.regexpFlags(), "g") {
			panic(TypeErrorf("String.prototype.matchAll called with a non-global RegExp argument"))
		}
		return rt.regexpMatchAll(r, s)
	})
	str("search", 1, func(s string, args []Value) Value { return rt.regexpSearch(rt.toRegExp(Arg(args, 0), ""), s) })
	si := rt.method(p, "[Symbol.iterator]", 0, func(this Value, _ []Value) Value {
		next, _ := rt.iteratorFor(thisString(this, "[Symbol.iterator]"))
		return rt.NewIterator(next)
	})
	p.Delete("[Symbol.iterator]")
	p.DefineHidden(SymbolIterator, si)
}

func searchString(v Value, name string) string {
	if o, ok := v.(*Object); ok && o.Class == "RegExp" {
		panic(TypeErrorf("First argument to String.prototype.%s must not be a regular expression", name))
	}
	return ToString(v)
}

func (rt *Realm) pad(s string, args []Value, start bool) string {
	n := int(ToIntegerOrInfinity(Arg(args, 0)))
	fill := " "
	if f := Arg(args, 1); f != Undefined {
		fill = ToString(f)
	}
	l := rt.Len16(s)
	if n <= l || fill == "" {
		return s
	}
	fu := Units(fill)
	need := n - l
	pu := make([]uint16, 0, need)
	for len(pu) < need {
		pu = append(pu, fu...)
	}
	padding := FromUnits(pu[:need])
	if start {
		return padding + s
	}
	return s + padding
}

// jsUpper and jsLower follow Unicode's full case mappings, as JavaScript
// does: "ß".toUpperCase() is "SS".
func (rt *Realm) jsUpper(s string) string {
	if rt.isASCII(s) {
		return strings.ToUpper(s)
	}
	var b strings.Builder
	for _, r := range s {
		switch r {
		case 'ß':
			b.WriteString("SS")
		case 'ŉ':
			b.WriteString("ʼN")
		default:
			b.WriteRune(unicode.ToUpper(r))
		}
	}
	return b.String()
}

func (rt *Realm) jsLower(s string) string {
	if rt.isASCII(s) {
		return strings.ToLower(s)
	}
	var b strings.Builder
	rs := []rune(s)
	for i, r := range rs {
		if r == 'Σ' {
			// Final sigma: lowercase ς at the end of a word.
			end := i+1 == len(rs) || !unicode.IsLetter(rs[i+1])
			if end && i > 0 && unicode.IsLetter(rs[i-1]) {
				b.WriteRune('ς')
				continue
			}
		}
		if r == 'İ' {
			b.WriteString("i̇")
			continue
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

// localeCompare is QuickJS's String.prototype.localeCompare, which the
// server renderer uses: both strings normalized to NFC, then compared by
// UTF-16 code units.
func (rt *Realm) localeCompare(a, b string) int {
	return rt.CompareStrings(norm.NFC.String(a), norm.NFC.String(b))
}

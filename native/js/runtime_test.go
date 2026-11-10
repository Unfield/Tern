package js

import (
	"encoding/json"
	"math"
	"os"
	"testing"
)

func m(v Value, name string, args ...Value) Value { return CallMethod(v, name, args...) }
func g(name string) Value                         { return Global.Get(name) }
func arr(vs ...Value) Value                       { return NewArray(vs) }
func str(v Value) Value                           { return Call(g("String"), Undefined, v) }
func re(src, flags string) Value                  { return NewRegExp(src, flags) }

// TestAgainstNode checks the runtime against what Node computes for the
// same expressions: testdata/node-cases.json holds each one's
// JSON.stringify, written by the script in testdata/node-cases.js.
func TestAgainstNode(t *testing.T) {
	raw, err := os.ReadFile("testdata/node-cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var want map[string]string
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatal(err)
	}
	obj := func(kv ...Value) *Object {
		o := NewPlainObject()
		for i := 0; i < len(kv); i += 2 {
			o.Define(kv[i].(string), kv[i+1])
		}
		return o
	}
	n := func(x float64) Value { return x }
	fn := NewFunction("f", 0, func(Value, []Value) Value { return 1.0 })
	offset := NewFunction("", 2, func(_ Value, a []Value) Value { return Arg(a, 1) }) // (m, i) => i
	sum := 0.1
	sum += 0.2
	got := map[string]Value{
		"add1": Add(n(1), "2"), "add2": Add(arr(n(1), n(2)), n(3)), "add3": Add(NewPlainObject(), ""), "add4": Add(Null, n(1)), "add5": Add(Undefined, n(1)), "add6": Add(true, true),
		"eq1": LooseEquals(Null, Undefined), "eq2": LooseEquals("1", n(1)), "eq3": LooseEquals(n(0), ""), "eq4": LooseEquals("0", false), "eq5": LooseEquals(nan, nan), "eq6": LooseEquals(arr(n(1)), n(1)),
		"lt1": Less("10", "9"), "lt2": Less("10", n(9)), "lt3": Less(Null, n(1)), "lt4": Less(Undefined, n(1)), "lt5": Less("a", "B"),
		"num1": str(sum), "num2": str(n(1e21)), "num3": str(n(-1e-7)), "num4": str(n(123e-20)), "num5": m(n(255), "toString", n(16)), "num6": m(n(0.5), "toString", n(2)), "num7": m(n(-255), "toString", n(36)),
		"fix1": m(n(1.005), "toFixed", n(2)), "fix2": m(n(1e21), "toFixed", n(2)), "fix3": m(n(0.000001), "toFixed", n(7)), "fix4": m(n(-1.5), "toFixed", n(0)), "fix5": m(n(2.5), "toFixed", n(0)),
		"prec1": m(n(123.456), "toPrecision", n(4)), "prec2": m(n(0.00001), "toPrecision", n(1)), "prec3": m(n(123456), "toPrecision", n(2)), "prec4": m(n(1.45), "toPrecision", n(2)),
		"exp1": m(n(123456), "toExponential", n(2)), "exp2": m(n(0), "toExponential"), "exp3": m(n(1.5e-7), "toExponential"),
		"pf1": Call(g("parseFloat"), Undefined, "3.14abc"), "pf2": Call(g("parseFloat"), Undefined, ".5"), "pf3": Call(g("parseFloat"), Undefined, "-Infinityx"), "pf4": Call(g("parseFloat"), Undefined, "1e"),
		"pi1": Call(g("parseInt"), Undefined, "0x1F"), "pi2": Call(g("parseInt"), Undefined, "08"), "pi3": Call(g("parseInt"), Undefined, "123456789012345678901234567890"), "pi4": Call(g("parseInt"), Undefined, "z", n(36)),
		"sn1": ToNumber(""), "sn2": ToNumber(" 12 "), "sn3": ToNumber("0b101"), "sn4": ToNumber("1_000"), "sn5": ToNumber("Infinity"), "sn6": ToNumber("inf"), "sn7": ToNumber("12px"),
		"mod1": Mod(n(-5), n(3)), "mod2": Mod(n(5.5), n(2)), "pow1": Pow(n(1), nan), "pow2": Pow(n(-1), math.Inf(1)), "pow3": Pow(n(2), n(10)),
		"round1": m(g("Math"), "round", n(-2.5)), "round2": m(g("Math"), "round", n(2.5)), "round3": m(g("Math"), "round", n(0.49999999999999994)), "round4": math.Signbit(m(g("Math"), "round", n(-0.4)).(float64)),
		"str1": Get("héllo", "length"), "str2": Get("😀a", "length"), "str3": m("😀a", "slice", n(1)), "str4": m("abc", "at", n(-1)), "str5": m("Straße", "toUpperCase"), "str6": m("ΣΑΣ", "toLowerCase"),
		"str7": m("a,b,,c", "split", ","), "str8": m("abc", "split", ""), "str9": m("x", "padStart", n(4), "ab"),
		"re1": m("a1b22c333", "replace", re(`\d+`, "g"), "#"), "re2": m("John Smith", "replace", re(`(\w+)\s(\w+)`, ""), "$2, $1"), "re3": m("aaa", "replace", re("a", "g"), offset),
		"re4": Get(Get(m("2026-11-10", "match", re(`(?<y>\d+)-(?<m>\d+)`, "")), "groups"), "m"), "re5": m("a1b2", "split", re(`\d`, "")), "re6": m("abc", "search", re("c", "")), "re7": m(re("^[a-z]+$", "i"), "test", "AbC"),
		"json1": jsonStr(obj("b", n(1), "a", arr(n(1), "x", Null, Undefined, fn), "c", Undefined, "d", nan, "2", n(0), "1", n(1)), Undefined), "json2": jsonStr(obj("a", obj("b", n(1))), n(2)),
		"json3": jsonStr(FromUnits([]uint16{0x2028, 0xd800}), Undefined), "json4": jsonStr(must(ParseJSON(`{"x":[1,2,{"y":"z"}]}`)), Undefined),
		"obj1": m(g("Object"), "keys", obj("b", n(1), "2", n(1), "a", n(1), "1", n(1))), "sort1": m(arr(n(10), n(9), n(1), Undefined, n(2)), "sort"), "sort2": m(arr("b", "a", "C"), "sort"),
		"ta1": Construct(g("Int8Array"), arr(n(127), n(128), n(-129), n(1.9))), "ta2": Construct(g("Uint8ClampedArray"), arr(n(1.5), n(2.5), n(300), n(-3), nan)),
		"ta3": GetV(Construct(g("Float32Array"), arr(n(0.1))), n(0)), "ta4": m(Get(g("Object"), "prototype").(*Object).Get("toString"), "call", Construct(g("Uint16Array"), n(2))),
		"ta5": m(m(Construct(g("Uint8Array"), arr(n(3), n(1), n(2))), "sort"), "join", "-"), "ta6": m(Construct(g("Uint8Array"), Get(Construct(g("Uint16Array"), arr(n(258), n(3))), "buffer")), "join"),
		"ta7": m(m(Construct(g("Uint8Array"), arr(n(1), n(2), n(3), n(4))), "subarray", n(1), n(3)), "map", NewFunction("", 1, func(_ Value, a []Value) Value { return Mul(a[0], n(100)) })),
		"ta8": m(NewArray(Spread(m(Construct(g("Float64Array"), arr(math.Copysign(0, -1), nan, n(1))), "sort"))), "map", g("String")),
		"ta9": func() Value {
			a := Construct(g("Uint8Array"), n(2))
			Set(a, "5", n(1))
			Set(a, "-0", n(1))
			Set(a, "x", n(1))
			return m(g("Object"), "keys", a)
		}(),
		"ta10": m(Construct(g("DataView"), Get(Construct(g("Uint8Array"), arr(n(1), n(2))), "buffer")), "getUint16", n(0)),
		"re8":  m("a<b&\"😀c<", "replace", re(`[&"<]`, "g"), "#"),
		"re9": func() Value {
			r := re("[<]", "y")
			Set(r, "lastIndex", n(1))
			a := m(r, "test", "a<")
			b := Get(r, "lastIndex")
			c := m(r, "test", "a<")
			return arr(a, b, c, Get(r, "lastIndex"))
		}(),
		"re10": m("x😀<y", "search", re("[<]", "")),
		"fix6": func() Value {
			var out []Value
			for _, c := range [][2]float64{{0.125, 2}, {2.5, 0}, {1.45, 1}, {0.5, 0}, {9.995, 2}, {99.5, 0}, {4.69, 2}, {-0.125, 2}, {1e-10, 3}, {123.456, 10}, {0.000001, 20}} {
				out = append(out, m(n(c[0]), "toFixed", n(c[1])))
			}
			return NewArray(out)
		}(),
		"uri1": Call(g("encodeURIComponent"), Undefined, "a b&ü/😀"), "uri2": Call(g("decodeURIComponent"), Undefined, "%E2%82%AC%20x"),
	}
	for name, w := range want {
		gv, ok := got[name]
		if !ok {
			t.Errorf("%s: no Go case", name)
			continue
		}
		gj, ok := Stringify(gv, Undefined, Undefined)
		if !ok {
			gj = "undefined"
		}
		if gj != w {
			t.Errorf("%s: got %s, want %s", name, gj, w)
		}
	}
	for name := range got {
		if _, ok := want[name]; !ok {
			t.Errorf("%s: no Node answer", name)
		}
	}
}

func jsonStr(v, space Value) Value {
	s, _ := Stringify(v, Undefined, space)
	return s
}

func must(v Value, err error) Value {
	if err != nil {
		panic(err)
	}
	return v
}

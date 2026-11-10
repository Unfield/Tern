// Writes node-cases.json: each case's JSON.stringify, as Node computes it.
// Run it with `node testdata/node-cases.js > testdata/node-cases.json`.
const cases = {
 add1: () => 1 + "2", add2: () => [1,2] + 3, add3: () => ({}) + "", add4: () => null + 1, add5: () => undefined + 1, add6: () => true + true,
 eq1: () => null == undefined, eq2: () => "1" == 1, eq3: () => 0 == "", eq4: () => "0" == false, eq5: () => NaN == NaN, eq6: () => [1] == 1,
 lt1: () => "10" < "9", lt2: () => "10" < 9, lt3: () => null < 1, lt4: () => undefined < 1, lt5: () => "a" < "B",
 num1: () => String(0.1+0.2), num2: () => String(1e21), num3: () => String(-1e-7), num4: () => String(123e-20), num5: () => (255).toString(16), num6: () => (0.5).toString(2), num7: () => (-255).toString(36),
 fix1: () => (1.005).toFixed(2), fix2: () => (1e21).toFixed(2), fix3: () => (0.000001).toFixed(7), fix4: () => (-1.5).toFixed(0), fix5: () => (2.5).toFixed(0),
 prec1: () => (123.456).toPrecision(4), prec2: () => (0.00001).toPrecision(1), prec3: () => (123456).toPrecision(2), prec4: () => (1.45).toPrecision(2),
 exp1: () => (123456).toExponential(2), exp2: () => (0).toExponential(), exp3: () => (1.5e-7).toExponential(),
 pf1: () => parseFloat("3.14abc"), pf2: () => parseFloat(".5"), pf3: () => parseFloat("-Infinityx"), pf4: () => parseFloat("1e"), pi1: () => parseInt("0x1F"), pi2: () => parseInt("08"), pi3: () => parseInt("123456789012345678901234567890"), pi4: () => parseInt("z", 36),
 sn1: () => Number(""), sn2: () => Number(" 12 "), sn3: () => Number("0b101"), sn4: () => Number("1_000"), sn5: () => Number("Infinity"), sn6: () => Number("inf"), sn7: () => Number("12px"),
 mod1: () => -5 % 3, mod2: () => 5.5 % 2, pow1: () => 1 ** NaN, pow2: () => (-1) ** Infinity, pow3: () => 2 ** 10,
 round1: () => Math.round(-2.5), round2: () => Math.round(2.5), round3: () => Math.round(0.49999999999999994), round4: () => Object.is(Math.round(-0.4), -0),
 str1: () => "héllo".length, str2: () => "😀a".length, str3: () => "😀a".slice(1), str4: () => "abc".at(-1), str5: () => "Straße".toUpperCase(), str6: () => "ΣΑΣ".toLowerCase(), str7: () => "a,b,,c".split(","), str8: () => "abc".split(""), str9: () => "x".padStart(4, "ab"),
 re1: () => "a1b22c333".replace(/\d+/g, "#"), re2: () => "John Smith".replace(/(\w+)\s(\w+)/, "$2, $1"), re3: () => "aaa".replace(/a/g, (m, i) => i), re4: () => "2026-11-10".match(/(?<y>\d+)-(?<m>\d+)/).groups.m, re5: () => "a1b2".split(/\d/), re6: () => "abc".search(/c/), re7: () => /^[a-z]+$/i.test("AbC"),
 json1: () => JSON.stringify({b: 1, a: [1, "x", null, undefined, () => 1], c: undefined, d: NaN, "2": 0, "1": 1}), json2: () => JSON.stringify({a: {b: 1}}, null, 2), json3: () => JSON.stringify(" \ud800"), json4: () => JSON.stringify(JSON.parse("{\"x\":[1,2,{\"y\":\"z\"}]}")),
 obj1: () => Object.keys({b: 1, 2: 1, a: 1, 1: 1}), sort1: () => [10, 9, 1, undefined, 2].sort(), sort2: () => ["b", "a", "C"].sort(), uri1: () => encodeURIComponent("a b&ü/😀"), uri2: () => decodeURIComponent("%E2%82%AC%20x"),
 ta1: () => new Int8Array([127, 128, -129, 1.9]), ta2: () => new Uint8ClampedArray([1.5, 2.5, 300, -3, NaN]), ta3: () => new Float32Array([0.1])[0], ta4: () => Object.prototype.toString.call(new Uint16Array(2)),
 ta5: () => new Uint8Array([3, 1, 2]).sort().join("-"), ta6: () => new Uint8Array(new Uint16Array([258, 3]).buffer).join(), ta7: () => new Uint8Array([1, 2, 3, 4]).subarray(1, 3).map((x) => x * 100), ta8: () => [...new Float64Array([-0, NaN, 1]).sort()].map(String),
 ta9: () => { const a = new Uint8Array(2); a[5] = 1; a["-0"] = 1; a.x = 1; return Object.keys(a); }, ta10: () => new DataView(new Uint8Array([1, 2]).buffer).getUint16(0),
 re8: () => "a<b&\"😀c<".replace(/[&"<]/g, "#"), re9: () => { const r = /[<]/y; r.lastIndex = 1; return [r.test("a<"), r.lastIndex, r.test("a<"), r.lastIndex]; }, re10: () => "x😀<y".search(/[<]/),
 fix6: () => [0.125.toFixed(2), 2.5.toFixed(0), 1.45.toFixed(1), 0.5.toFixed(0), 9.995.toFixed(2), 99.5.toFixed(0), 4.69.toFixed(2), (-0.125).toFixed(2), 1e-10.toFixed(3), 123.456.toFixed(10), 0.000001.toFixed(20)],
};
const out = {};
for (const [k, f] of Object.entries(cases)) { const v = f(); out[k] = v === undefined ? "undefined" : JSON.stringify(v); }
console.log(JSON.stringify(out, null, 1));

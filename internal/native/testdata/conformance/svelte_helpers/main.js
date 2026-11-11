// Svelte's attribute helpers, whose Go versions native rendering uses:
// they must give what the JavaScript gives, for every kind of input.
import { attr, to_class } from './node_modules/svelte/src/internal/shared/attributes.js';
import { attr_class, stringify, ensure_array_like } from './node_modules/svelte/src/internal/server/index.js';
import { escape_html } from './node_modules/svelte/src/escaping.js';

const out = [];
const show = (v) => (typeof v === 'string' ? JSON.stringify(v) : Array.isArray(v) ? `[${v.map(show)}]` : String(v));
const run = (label, f) => {
	try {
		out.push(label + ' = ' + show(f()));
	} catch (e) {
		out.push(label + ' threw ' + e.constructor.name);
	}
};

const counted = { n: 0, toString() { this.n++; return 'obj<' + this.n + '>'; } };
const values = ['', 'x', 'a "b" & <c>', 'héllo', 0, 1, -0, 1.5, NaN, true, false, null, undefined, 10n, counted, [1, 2], { a: 1 }];

for (const [i, v] of values.entries()) {
	run(`stringify(${i})`, () => stringify(v));
	run(`attr(title, ${i})`, () => attr('title', v));
	run(`attr(title, ${i}, true)`, () => attr('title', v, true));
	run(`attr(hidden, ${i})`, () => attr('hidden', v));
	run(`attr(translate, ${i})`, () => attr('translate', v));
	run(`to_class(${i})`, () => to_class(v));
	run(`attr_class(${i})`, () => attr_class(v));
	run(`escape(${i})`, () => escape_html(v, true));
}
run('attr(hidden, until-found)', () => attr('hidden', 'until-found'));
run('attr(name not a string)', () => attr(5, 'x'));
run('attr(empty-bool)', () => attr('disabled', '', true));
run('attr(zero-bool)', () => attr('disabled', 0, true));
run('attr(is_boolean truthy)', () => attr('disabled', 1, 'yes'));
run('attr(symbol)', () => attr('title', Symbol('s')));

const classCases = [
	['btn', undefined, undefined],
	['btn', 'svelte-xyz', undefined],
	['', 'svelte-xyz', { on: true }],
	['btn primary', undefined, { primary: false, active: true }],
	['a b a', undefined, { a: false }],
	['xa a ax', undefined, { a: false }],
	['a\tb\na', undefined, { a: false }],
	['primary', undefined, { primary: false }],
	[null, undefined, { out: true }],
	[null, undefined, { out: false }],
	[undefined, undefined, {}],
	['', undefined, undefined],
	[42, undefined, { x: true }],
	[true, 'h', { y: 0 }],
	['naïve', undefined, { x: true }],
	['btn', 'hash', { 'naïve': false }],
	['btn', 5, undefined],
	['btn', undefined, 'directive string'],
	['btn', undefined, [true]],
	['btn  btn', undefined, { btn: false }],
	['a', undefined, Object.create({ inherited: true })],
	['a', undefined, Object.defineProperty({}, 'x', { get() { out.push('getter ran'); return true; }, enumerable: true })],
	['a', undefined, { 2: true, 1: false, b: true }],
	[counted, undefined, { z: true }]
];
for (const [i, [v, h, d]] of classCases.entries()) {
	run(`to_class#${i}`, () => to_class(v, h, d));
	run(`attr_class#${i}`, () => attr_class(v, h, d));
}

run('ensure_array_like(array)', () => { const a = [1, 2]; return ensure_array_like(a) === a; });
run('ensure_array_like(string)', () => ensure_array_like('abc'));
run('ensure_array_like(set)', () => ensure_array_like(new Set([1, 2])));
run('ensure_array_like(null)', () => ensure_array_like(null));
run('ensure_array_like(0)', () => ensure_array_like(0));
run('ensure_array_like(5)', () => ensure_array_like(5));
run('ensure_array_like(arraylike)', () => ensure_array_like({ length: 2, 0: 'a', 1: 'b' }).length);
run('ensure_array_like(holes)', () => ensure_array_like([1, , 3]).length);
run('ensure_array_like(fresh [])', () => ensure_array_like(undefined) !== ensure_array_like(undefined));
let reads = 0;
run('ensure_array_like(getter)', () => ensure_array_like({ get length() { reads++; return undefined; }, [Symbol.iterator]: function* () { yield 'g'; } }));
run('length reads', () => reads);
run('counted calls', () => counted.n);

globalThis.out = out;

const out = [];
const log = (...a) => out.push(a.map((x) => (typeof x === 'string' ? x : JSON.stringify(x))).join(' '));
const tryLog = (name, f) => {
	try {
		log(name, f());
	} catch (e) {
		log(name, 'threw', e.constructor.name);
	}
};

// Grouping
const people = [{ n: 'a', age: 30 }, { n: 'b', age: 20 }, { n: 'c', age: 30 }];
const byAge = Object.groupBy(people, (p) => p.age);
log(Object.getPrototypeOf(byAge) === null, Object.keys(byAge), byAge[30].map((p) => p.n));
log(Object.keys(Object.groupBy('hello', (c, i) => (i % 2 ? 'odd' : 'even'))));
const m = Map.groupBy([1, 2, 3, 4, 5], (x) => (x % 2 ? 'odd' : 'even'));
log(m instanceof Map, [...m.keys()], m.get('odd'));
tryLog('groupBy null', () => Object.groupBy(null, (x) => x));
tryLog('groupBy no fn', () => Object.groupBy([1]));

// Set methods
const a = new Set([1, 2, 3]);
const b = new Set([3, 4]);
log([...a.union(b)], [...a.intersection(b)], [...a.difference(b)], [...a.symmetricDifference(b)]);
log(a.isSubsetOf(new Set([1, 2, 3, 4])), a.isSupersetOf(new Set([1])), a.isDisjointFrom(new Set([9])), a.isDisjointFrom(b));
log([...a.union(new Map([[7, 'x']]))]);
const setLike = { size: 2, has: (x) => x === 1 || x === 9, keys: () => [1, 9][Symbol.iterator]() };
log([...a.intersection(setLike)], [...a.difference(setLike)], [...a.union(setLike)]);
tryLog('union bad', () => a.union([1, 2]));
tryLog('union NaN size', () => a.union({ size: NaN, has() {}, keys() {} }));

// Iterator helpers
function* nat() {
	let i = 0;
	while (true) yield i++;
}
log(nat().filter((x) => x % 2).map((x) => x * 10).take(3).toArray());
log(nat().drop(5).take(2).toArray(), [1, 2, 3].values().reduce((s, x) => s + x), [1, 2, 3].values().reduce((s, x) => s + x, 10));
log([1, 2].values().flatMap((x) => [x, x * 100]).toArray());
log([1, 2, 3].values().some((x) => x > 2), [1, 2, 3].values().every((x) => x > 2), [1, 2, 3].values().find((x) => x > 1));
const seen = [];
[4, 5].values().forEach((x, i) => seen.push([x, i]));
log(seen);
log(typeof Iterator, Iterator.prototype === Object.getPrototypeOf(Object.getPrototypeOf([].values())));
log(Iterator.from({ next: (() => { let i = 0; return () => ({ done: i > 2, value: i++ }); })() }).toArray());
log(Iterator.from([7, 8]).toArray(), Iterator.from('hé').toArray());
class Count extends Iterator {
	#i = 0;
	next() {
		return this.#i < 3 ? { value: this.#i++, done: false } : { value: undefined, done: true };
	}
}
log(new Count().map((x) => x + 1).toArray(), new Count() instanceof Iterator);
tryLog('new Iterator', () => new Iterator());
tryLog('take -1', () => nat().take(-1));
let closed = 0;
const closable = { i: 0, next() { return { value: this.i++, done: false }; }, return() { closed++; return {}; }, __proto__: Iterator.prototype };
closable.take(1).toArray();
closable.find((x) => x > 3);
const helper = closable.map((x) => x);
helper.next();
helper.return();
log('closed', closed);
tryLog('reduce empty', () => [].values().reduce((a, b) => a + b));
log(Object.prototype.toString.call(nat().map((x) => x)));

// Arrays and strings
log([1, 2, 3, 4, 5].copyWithin(0, 3), [1, 2, 3, 4, 5].copyWithin(1, 0, 3), [1, 2, 3, 4, 5].copyWithin(-2, -4, -3));
log(Array.prototype.copyWithin.call({ length: 5, 3: 'x' }, 0, 3));
log('ab'.isWellFormed(), 'a\uD800b'.isWellFormed(), '😀'.isWellFormed(), 'a\uDC00'.toWellFormed() === 'a�');
log(escape('ä b+c/é€'), unescape('%E4%20b%u20AC%zz'), unescape(escape('😀')) === '😀');

// Symbols, weak refs
const reg = Symbol.for('app');
log(Symbol.keyFor(reg), Symbol.keyFor(Symbol('x')));
tryLog('keyFor string', () => Symbol.keyFor('x'));
const target = { v: 1 };
const ref = new WeakRef(target);
log(ref.deref() === target, Object.prototype.toString.call(ref));
tryLog('WeakRef prim', () => new WeakRef(1));
const fr = new FinalizationRegistry(() => {});
const token = {};
fr.register(target, 'held', token);
log(fr.unregister(token), fr.unregister(token));
tryLog('register same', () => fr.register(target, target));
log(Object.isSealed(Object.freeze({})), Object.isSealed({}), Object.isSealed(1));

// RegExp d flag
const md = /(?<y>\d{4})-(\d\d)?/d.exec('on 2026- ok');
log(md.indices[0], md.indices[1], md.indices[2], md.indices.groups.y);
log(/b/d.exec('abc').indices, /a/.exec('a').indices, /x/dg.flags, /x/d.hasIndices);
log(/[\p{L}]+/v.exec('héllo1')[0], /x/v.unicodeSets, /x/v.flags);
tryLog('uv', () => new RegExp('x', 'uv'));

globalThis.out = out;

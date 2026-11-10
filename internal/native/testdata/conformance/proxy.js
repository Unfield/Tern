const log = [];
const p = new Proxy({ a: 1 }, {
	get: (t, k) => (log.push(String(k)), k in t ? t[k] : 'missing'),
	has: (t, k) => k === 'virtual',
	ownKeys: () => ['a', 'b'],
	getOwnPropertyDescriptor: (t, k) => ({ value: k, enumerable: true, configurable: true }),
});
globalThis.out = [p.a, p.zzz, 'virtual' in p, 'a' in p, Object.keys(p), Reflect.ownKeys({ b: 1, 1: 2 }), Reflect.has({ q: 1 }, 'q'), log];

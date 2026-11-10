const s = '😀aé';
globalThis.out = [
	s.length, s.slice(1), [...s].length, s.codePointAt(0), s.at(-1),
	0.1 + 0.2, (1234.5678).toFixed(2), (255).toString(16), 1 / 3, -0 === 0, Object.is(-0, 0),
	'10' < '9', 10 < 9, null == undefined, NaN !== NaN, [] + {}, [1, [2, [3]]].flat(Infinity),
	typeof null, typeof (() => 1), typeof Symbol(), void 0,
	JSON.stringify({ b: 1, a: [1, 'x', null, undefined], c: undefined, d: new Date(0) }),
	Object.entries({ z: 1, 2: 'two', 1: 'one' }), [3, 10, 1].sort(), [3, 10, 1].sort((a, b) => a - b),
	'a-b-c'.split('-', 2), 'Hello'.replace(/l/g, (m, i) => i), /(?<y>\d{4})/.exec('in 2026').groups.y,
	encodeURIComponent('a b&ü'), parseInt('0x1f'), Number('  12  '), Math.max(), [1, 2, 3].includes(NaN),
];
const holey = new Array(3).concat([1, , 2]);
const seen = [];
holey.forEach((v, i) => seen.push(i));
globalThis.out.push(
	holey.length, seen, holey.map((v) => v * 2).length, 1 in holey, Object.keys(holey), holey.indexOf(undefined),
	holey.includes(undefined), JSON.stringify(holey), [...holey].length, holey.filter(() => true).length,
	[3, , 1].sort(), [, 'a'].join('-'), [, 1].reduce((a, b) => a + b), (() => { const a = [1, 2]; delete a[0]; return [0 in a, a.length]; })(),
);

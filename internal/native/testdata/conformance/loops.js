const fns = [];
for (let i = 0, n = 3, sq = n * n; i < n; i++) fns.push(() => i * sq);
const labels = [];
outer: for (const a of [1, 2, 3]) {
	for (const b of [1, 2, 3]) {
		if (b === 2) continue outer;
		if (a === 3) break outer;
		labels.push(`${a}${b}`);
	}
}
let w = 0;
do { w++; } while (w < 5);
const keys = [];
for (const k in { x: 1, y: 2, [Symbol('s')]: 3 }) keys.push(k);
let sw = '';
for (const v of [1, 2, 3, 4]) {
	switch (v) {
		case 1: sw += 'a';
		case 2: sw += 'b'; break;
		default: sw += 'd';
	}
}
globalThis.out = [fns.map((f) => f()), labels, w, keys, sw];

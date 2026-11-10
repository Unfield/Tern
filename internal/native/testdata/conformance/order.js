// Operands are evaluated left to right, even when later ones change
// earlier variables.
let f = (x) => 'first ' + x;
const r1 = f((f = (x) => 'second ' + x, 1));
let a = 1;
const r2 = a + (a = 10, a);
let o = { k: 'old' };
const r3 = [o.k, (o = { k: 'new' }).k];
let n = 0;
const r4 = { first: n, second: ++n, third: n };
let e = () => 'callee';
const r5 = e && e((e = 0));
let arr = [1, 2];
const r6 = arr[(arr = [3, 4], 0)];
globalThis.out = [r1, r2, r3, r4, r5, r6];

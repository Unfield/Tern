function* gen(n) { for (let i = 0; i < n; i++) yield i; return 'done'; }
function* deleg() { const r = yield* gen(2); yield r; }
const it = gen(3);
const steps = [it.next(), it.next(), it.return(9), it.next()];
function rest(a, ...more) { return [a, more.length, arguments.length]; }
const { x, y: [first, ...others] = [], ...remaining } = { x: 1, y: [2, 3, 4], z: 5, w: 6 };
const bound = function (p) { return this.v + p; }.bind({ v: 10 });
let closures = 0;
const counter = () => ++closures;
counter(); counter();
let caught;
try { null.prop; } catch (e) { caught = e instanceof TypeError; } finally { closures += 10; }
const tagged = ((s, ...v) => s.raw.join('|') + v.join(','))`a${1}b${2}c`;
globalThis.out = [[...deleg()], steps, rest(1, 2, 3), x, first, others, remaining, bound(5), closures, caught, tagged, gen.name, rest.length];
function shown(a, b) { return a + b; }
class Shown { m() { return 1; } }
globalThis.out.push(String(shown), shown.toString().length, String(Shown), String((x) => x * 2));

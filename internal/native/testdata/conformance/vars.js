// var in a module's loops and blocks belongs to the module.
var total = 0;
for (var i = 0; i < 3; i++) total += i;
if (total > 1) {
	var seen = 'yes';
}
for (var k in { a: 1, b: 2 }) var last = k;
try {
	throw new Error('x');
} catch (e) {
	var caught = e.message;
}
function readLater() {
	return [i, seen, k, last, caught, typeof never];
}
if (false) {
	var never = 1;
}
globalThis.out = [total, readLater()];

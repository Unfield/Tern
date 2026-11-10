// Async functions and promise job order. Node drains the job queue after
// the module runs; the harness does too (RunJobs), so the order of every
// step is compared.
const log = [];
async function inner(x) {
	log.push('inner start ' + x);
	const v = await x;
	log.push('inner resumed ' + v);
	return v * 2;
}
const outer = async () => {
	log.push('outer start');
	try {
		const a = await inner(1);
		const b = await Promise.resolve(10);
		await Promise.reject(new Error('nope'));
		log.push('unreachable');
		return a + b;
	} catch (e) {
		log.push('caught ' + e.message);
		return 'recovered';
	} finally {
		log.push('finally');
	}
};
outer().then((v) => log.push('outer done ' + v));
Promise.resolve().then(() => log.push('tick 1')).then(() => log.push('tick 2'));
(async function thrower() { throw new TypeError('bad'); })().catch((e) => log.push('rejected ' + e.name));
log.push('sync end');
const thenable = { then(res) { log.push('thenable then'); res('from thenable'); } };
(async () => log.push('await thenable: ' + (await thenable)))();
Promise.all([1, Promise.resolve(2), inner(3)]).then((v) => log.push('all ' + v));
globalThis.out = log;

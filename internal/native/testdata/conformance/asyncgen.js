const log = [];
async function* count(n) {
	for (let i = 0; i < n; i++) {
		await null;
		yield i;
	}
	return 'end';
}
async function* echo() {
	const got = yield 'first';
	log.push('echo got ' + got);
	try { yield 'second'; } finally { log.push('echo finally'); }
}
(async () => {
	for await (const x of count(3)) log.push('count ' + x);
	for await (const y of [Promise.resolve('a'), 'b']) log.push('sync ' + y);
	const it = echo();
	log.push(JSON.stringify(await it.next()));
	log.push(JSON.stringify(await it.next('hi')));
	log.push(JSON.stringify(await it.return('stop')));
	log.push(JSON.stringify(await it.next()));
	const q = count(2);
	const [a, b, c] = await Promise.all([q.next(), q.next(), q.next()]);
	log.push([a.value, b.value, c.value, c.done].join());
	log.push(Object.prototype.toString.call(q), typeof q[Symbol.asyncIterator]);
})();
globalThis.out = log;

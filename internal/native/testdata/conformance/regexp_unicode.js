// Regexps after non-ASCII text: lastIndex and index are UTF-16 positions.
const out = [];
for (const pre of ['·x · ', 'a'.repeat(40) + 'é ', '😀 é ', 'a\uD800b']) {
	const s = pre + '<b>c';
	for (const flags of ['y', 'g']) {
		const re = new RegExp('<b>', flags);
		re.lastIndex = pre.length;
		const m = re.exec(s);
		out.push([flags, m && m.index, re.lastIndex]);
	}
	out.push(s.match(/c/g), s.search(/</), s.replace(/<b>/, '[$&]'), [...s.matchAll(/./gu)].length);
}
const id = /^(?:\p{ID_Start}[$‌‍\p{ID_Continue}]*)$/u;
out.push(['foo', 'é9_x', '名前', '9a', 'a-b', 'x·y'].map((s) => id.test(s)));
globalThis.out = out;

// Message formatting for i18n. This mirrors internal/i18n in Go exactly:
// both are tested against internal/i18n/testdata/cases.json, so the server
// and the browser produce the same text and hydration matches.

type Node =
	| { type: 'text'; text: string }
	| { type: 'hash' }
	| { type: 'arg'; name: string }
	| { type: 'plural' | 'select'; name: string; options: Record<string, Node[]> };

const cache = new Map<string, Node[] | null>();

/** Formats one message in the given locale. */
export function format(locale: string, message: string, params: Record<string, unknown> = {}): string {
	let nodes = cache.get(message);
	if (nodes === undefined) {
		try {
			nodes = parse(message);
		} catch {
			nodes = null;
		}
		cache.set(message, nodes);
	}
	if (!nodes) return message;
	return render(nodes, locale, params, undefined);
}

function render(nodes: Node[], locale: string, params: Record<string, unknown>, hash: number | undefined): string {
	let out = '';
	for (const n of nodes) {
		switch (n.type) {
			case 'text':
				out += n.text;
				break;
			case 'hash':
				out += hash === undefined ? '#' : formatNumber(hash);
				break;
			case 'arg':
				out += n.name in params ? String(params[n.name]) : `{${n.name}}`;
				break;
			case 'plural': {
				const num = Number(params[n.name] ?? 0) || 0;
				const body = n.options[`=${formatNumber(num)}`] ?? n.options[pluralCategory(locale, num)] ?? n.options.other!;
				out += render(body, locale, params, num);
				break;
			}
			case 'select': {
				const body = n.options[String(params[n.name])] ?? n.options.other!;
				out += render(body, locale, params, hash);
			}
		}
	}
	return out;
}

function formatNumber(n: number): string {
	return String(n);
}

export function parse(message: string): Node[] {
	const p = { s: [...message], i: 0 };
	const nodes = parseNodes(p, false, false);
	if (p.i < p.s.length) throw new Error(`unexpected '}' at ${p.i}`);
	return nodes;
}

function parseNodes(p: { s: string[]; i: number }, inBranch: boolean, plural: boolean): Node[] {
	const out: Node[] = [];
	let text = '';
	const flush = () => {
		if (text) out.push({ type: 'text', text });
		text = '';
	};
	while (p.i < p.s.length) {
		const c = p.s[p.i]!;
		if (c === "'") {
			const next = p.s[p.i + 1];
			if (next === "'") {
				text += "'";
				p.i += 2;
				continue;
			}
			if (next !== undefined && '{}#'.includes(next)) {
				let end = p.i + 1;
				while (end < p.s.length && p.s[end] !== "'") end++;
				text += p.s.slice(p.i + 1, end).join('');
				p.i = end + 1;
				continue;
			}
			text += c;
			p.i++;
		} else if (c === '{') {
			flush();
			out.push(parseArgument(p));
		} else if (c === '}') {
			if (!inBranch) throw new Error(`unexpected '}' at ${p.i}`);
			flush();
			return out;
		} else if (c === '#' && plural) {
			flush();
			out.push({ type: 'hash' });
			p.i++;
		} else {
			text += c;
			p.i++;
		}
	}
	if (inBranch) throw new Error("unclosed '{'");
	flush();
	return out;
}

function until(p: { s: string[]; i: number }, stops: string): string {
	const start = p.i;
	while (p.i < p.s.length && !stops.includes(p.s[p.i]!)) p.i++;
	return p.s.slice(start, p.i).join('');
}

function parseArgument(p: { s: string[]; i: number }): Node {
	p.i++; // {
	const name = until(p, ',}').trim();
	if (!name) throw new Error(`empty argument at ${p.i}`);
	if (p.i >= p.s.length) throw new Error("unclosed '{'");
	if (p.s[p.i] === '}') {
		p.i++;
		return { type: 'arg', name };
	}
	p.i++; // ,
	const kind = until(p, ',}').trim();
	if (kind !== 'plural' && kind !== 'select') throw new Error(`unknown argument type "${kind}"`);
	if (p.s[p.i] !== ',') throw new Error(`${kind} argument "${name}" needs options`);
	p.i++;
	const options: Record<string, Node[]> = {};
	for (;;) {
		while (p.i < p.s.length && ' \n\t'.includes(p.s[p.i]!)) p.i++;
		if (p.i >= p.s.length) throw new Error("unclosed '{'");
		if (p.s[p.i] === '}') {
			p.i++;
			break;
		}
		const key = until(p, '{').trim();
		if (!key || p.i >= p.s.length) throw new Error(`expected an option in "${name}"`);
		p.i++; // {
		options[key] = parseNodes(p, true, kind === 'plural');
		p.i++; // }
	}
	if (!options.other) throw new Error(`${kind} argument "${name}" needs an 'other' option`);
	return { type: kind, name, options };
}

/** The CLDR cardinal plural category of n in locale (see the Go version). */
export function pluralCategory(locale: string, n: number): string {
	const lang = locale.toLowerCase().split(/[-_]/)[0]!;
	const i = Math.abs(Math.trunc(n));
	const isInt = n === Math.trunc(n);
	const mod10 = i % 10;
	const mod100 = i % 100;
	switch (lang) {
		case 'ja': case 'zh': case 'ko': case 'vi': case 'th': case 'id': case 'ms': case 'lo': case 'my': case 'km':
			return 'other';
		case 'fr': case 'pt': case 'hy': case 'ff': case 'kab':
			if (i === 0 || i === 1) return 'one';
			break;
		case 'ru': case 'uk': case 'be': case 'sr': case 'hr': case 'bs':
			if (!isInt) return 'other';
			if (mod10 === 1 && mod100 !== 11) return 'one';
			if (mod10 >= 2 && mod10 <= 4 && (mod100 < 12 || mod100 > 14)) return 'few';
			if (lang === 'ru' || lang === 'uk' || lang === 'be') return 'many';
			break;
		case 'pl':
			if (!isInt) return 'other';
			if (i === 1) return 'one';
			if (mod10 >= 2 && mod10 <= 4 && (mod100 < 12 || mod100 > 14)) return 'few';
			return 'many';
		case 'cs': case 'sk':
			if (!isInt) return 'many';
			if (i === 1) return 'one';
			if (i >= 2 && i <= 4) return 'few';
			break;
		case 'ro':
			if (i === 1 && isInt) return 'one';
			if (!isInt || i === 0 || (mod100 >= 2 && mod100 <= 19)) return 'few';
			break;
		case 'lt':
			if (mod10 === 1 && (mod100 < 11 || mod100 > 19)) return 'one';
			if (mod10 >= 2 && (mod100 < 11 || mod100 > 19)) return 'few';
			break;
		case 'lv':
			if (mod10 === 0 || (mod100 >= 11 && mod100 <= 19)) return 'zero';
			if (mod10 === 1 && mod100 !== 11) return 'one';
			break;
		case 'ar':
			if (n === 0) return 'zero';
			if (n === 1) return 'one';
			if (n === 2) return 'two';
			if (mod100 >= 3 && mod100 <= 10) return 'few';
			if (mod100 >= 11 && mod100 <= 99) return 'many';
			break;
		case 'he': case 'iw':
			if (i === 1 && isInt) return 'one';
			if (i === 2 && isInt) return 'two';
			break;
		case 'ga':
			if (n === 1) return 'one';
			if (n === 2) return 'two';
			if (n >= 3 && n <= 6) return 'few';
			if (n >= 7 && n <= 10) return 'many';
			break;
		case 'cy':
			if (n === 0) return 'zero';
			if (n === 1) return 'one';
			if (n === 2) return 'two';
			if (n === 3) return 'few';
			if (n === 6) return 'many';
			break;
		default:
			if (n === 1 && isInt) return 'one';
	}
	return 'other';
}

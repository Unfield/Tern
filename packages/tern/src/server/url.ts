// URL and URLSearchParams for the server renderer.
//
// QuickJS has no URL. core-js's polyfill keeps each URL's state in a WeakMap
// whose values point back at their keys, and QuickJS never frees such
// entries, so every URL that outlives a render leaked. This implementation
// keeps state in ordinary private fields instead. It covers the WHATWG URL
// standard for what pages use: special and opaque schemes, relative
// resolution, dot segments, default ports and percent-encoding. It does not
// convert international domain names to punycode or parse IPv6 hosts beyond
// keeping them as written.

const SPECIAL: Record<string, string> = { 'http:': '80', 'https:': '443', 'ws:': '80', 'wss:': '443', 'ftp:': '21', 'file:': '' };

// Percent-encode sets from the URL standard.
const C0 = (c: number) => c < 0x20 || c > 0x7e;
const FRAGMENT = (c: number) => C0(c) || c === 0x20 || c === 0x22 || c === 0x3c || c === 0x3e || c === 0x60;
const QUERY = (c: number) => C0(c) || c === 0x20 || c === 0x22 || c === 0x23 || c === 0x3c || c === 0x3e;
const SPECIAL_QUERY = (c: number) => QUERY(c) || c === 0x27;
const PATH = (c: number) => QUERY(c) || c === 0x3f || c === 0x60 || c === 0x7b || c === 0x7d || c === 0x5e;
const USERINFO = (c: number) => PATH(c) || c === 0x2f || c === 0x3a || c === 0x3b || c === 0x3d || c === 0x40 || (c >= 0x5b && c <= 0x5d) || c === 0x7c;
// application/x-www-form-urlencoded: everything but *-._ and alphanumerics.
const FORM = (c: number) => !((c >= 0x30 && c <= 0x39) || (c >= 0x41 && c <= 0x5a) || (c >= 0x61 && c <= 0x7a) || c === 0x2a || c === 0x2d || c === 0x2e || c === 0x5f);

const encoder = typeof TextEncoder === 'function' ? new TextEncoder() : null;

function utf8(s: string): number[] {
	if (encoder) return [...encoder.encode(s)];
	const out: number[] = [];
	for (const ch of s) {
		let c = ch.codePointAt(0)!;
		if (c >= 0xd800 && c <= 0xdfff) c = 0xfffd;
		if (c < 0x80) out.push(c);
		else if (c < 0x800) out.push(0xc0 | (c >> 6), 0x80 | (c & 63));
		else if (c < 0x10000) out.push(0xe0 | (c >> 12), 0x80 | ((c >> 6) & 63), 0x80 | (c & 63));
		else out.push(0xf0 | (c >> 18), 0x80 | ((c >> 12) & 63), 0x80 | ((c >> 6) & 63), 0x80 | (c & 63));
	}
	return out;
}

const HEX = '0123456789ABCDEF';

function encode(s: string, set: (c: number) => boolean, spaceAsPlus = false): string {
	let out = '';
	for (const ch of s) {
		const c = ch.codePointAt(0)!;
		if (c < 0x80 && !set(c)) {
			out += ch;
			continue;
		}
		if (spaceAsPlus && c === 0x20) {
			out += '+';
			continue;
		}
		for (const b of utf8(ch)) out += '%' + HEX[b >> 4] + HEX[b & 15];
	}
	return out;
}

function decode(s: string): string {
	const bytes: number[] = [];
	for (let i = 0; i < s.length; i++) {
		const c = s.charCodeAt(i);
		if (c === 0x25 && /^[0-9a-fA-F]{2}$/.test(s.slice(i + 1, i + 3))) {
			bytes.push(parseInt(s.slice(i + 1, i + 3), 16));
			i += 2;
		} else if (c < 0x80) {
			bytes.push(c);
		} else {
			bytes.push(...utf8(s[i]! + (c >= 0xd800 && c <= 0xdbff ? (s[++i] ?? '') : '')));
		}
	}
	return decodeUTF8(bytes);
}

function decodeUTF8(bytes: number[]): string {
	let out = '';
	for (let i = 0; i < bytes.length; ) {
		const b = bytes[i]!;
		let n = 0;
		let c = 0;
		if (b < 0x80) [n, c] = [0, b];
		else if (b >= 0xc2 && b < 0xe0) [n, c] = [1, b & 31];
		else if (b >= 0xe0 && b < 0xf0) [n, c] = [2, b & 15];
		else if (b >= 0xf0 && b < 0xf5) [n, c] = [3, b & 7];
		else {
			out += '�';
			i++;
			continue;
		}
		let ok = i + n < bytes.length + 0 || n === 0;
		for (let k = 1; k <= n && ok; k++) {
			const cb = bytes[i + k];
			if (cb === undefined || (cb & 0xc0) !== 0x80) ok = false;
			else c = (c << 6) | (cb & 63);
		}
		if (!ok) {
			out += '�';
			i++;
			continue;
		}
		out += String.fromCodePoint(c);
		i += n + 1;
	}
	return out;
}

function invalid(input: string): TypeError {
	return new TypeError(`Invalid URL: ${input}`);
}

interface Parts {
	protocol: string; // "https:"
	username: string;
	password: string;
	hostname: string; // null host is "" with hasHost false
	hasHost: boolean;
	port: string;
	path: string[]; // segments for hierarchical URLs
	opaque: string | null; // path of opaque URLs such as mailto:
	query: string | null; // without "?"
	fragment: string | null; // without "#"
}

function normalizePath(segments: string[]): string[] {
	const out: string[] = [];
	for (let i = 0; i < segments.length; i++) {
		const seg = segments[i]!;
		const lower = seg.toLowerCase();
		const last = i === segments.length - 1;
		if (lower === '..' || lower === '.%2e' || lower === '%2e.' || lower === '%2e%2e') {
			out.pop();
			if (last) out.push('');
		} else if (lower === '.' || lower === '%2e') {
			if (last) out.push('');
		} else {
			out.push(encode(seg, PATH));
		}
	}
	return out;
}

function parseHost(host: string, special: boolean, input: string): string {
	if (host.startsWith('[')) {
		if (!host.endsWith(']')) throw invalid(input);
		return host.toLowerCase();
	}
	if (!special) return encode(host, C0);
	const h = decode(host).toLowerCase();
	if (h === '' || /[\0\t\n\r #/:<>?@[\\\]^|%]/.test(h)) throw invalid(input);
	return ipv4(h) ?? h;
}

/** Normalizes IPv4 hosts written in decimal, hex or octal parts, as browsers do. */
function ipv4(h: string): string | null {
	const parts = h.split('.');
	if (parts[parts.length - 1] === '') parts.pop();
	if (parts.length === 0 || parts.length > 4) return null;
	const last = parts[parts.length - 1]!;
	if (!/^(0x[0-9a-f]*|[0-9]+)$/.test(last)) return null;
	const nums: number[] = [];
	for (const p of parts) {
		let n: number;
		if (/^0x[0-9a-f]*$/.test(p)) n = p.length === 2 ? 0 : parseInt(p.slice(2), 16);
		else if (/^0[0-7]+$/.test(p)) n = parseInt(p.slice(1), 8);
		else if (/^[0-9]+$/.test(p)) n = parseInt(p, 10);
		else return null;
		nums.push(n);
	}
	let value = nums.pop()!;
	if (nums.some((n) => n > 255) || value >= 256 ** (4 - nums.length)) return null;
	nums.forEach((n, i) => (value += n * 256 ** (3 - i)));
	return [value >>> 24, (value >>> 16) & 255, (value >>> 8) & 255, value & 255].join('.');
}

function parse(input: string, base?: Parts): Parts {
	// Strip leading and trailing C0 controls and spaces, and all tabs and newlines.
	const raw = input.replace(/^[\u0000- ]+|[\u0000- ]+$/g, '').replace(/[\t\n\r]/g, '');
	let rest = raw;
	let fragment: string | null = null;
	const hashAt = rest.indexOf('#');
	if (hashAt >= 0) {
		fragment = encode(rest.slice(hashAt + 1), FRAGMENT);
		rest = rest.slice(0, hashAt);
	}

	const scheme = /^([a-zA-Z][a-zA-Z0-9+.-]*):/.exec(rest);
	if (!scheme) {
		if (!base) throw invalid(input);
		return resolve(rest, fragment, base, input);
	}
	const protocol = scheme[1]!.toLowerCase() + ':';
	rest = rest.slice(scheme[0].length);
	const special = protocol in SPECIAL;

	if (special && base && base.protocol === protocol && !rest.startsWith('/') && !rest.startsWith('\\')) {
		// "http:foo" relative to an http: base.
		return resolve(rest, fragment, base, input);
	}

	if (protocol === 'file:') {
		if (/^[/\\]{2}/.test(rest)) return withAuthority(protocol, rest.slice(2), fragment, input, true);
		return withAuthority(protocol, rest.replace(/^[/\\]?/, '/'), fragment, input, true);
	}
	if (special) return withAuthority(protocol, rest.replace(/^[/\\]*/, ''), fragment, input, false);
	if (rest.startsWith('//')) return withAuthority(protocol, rest.slice(2), fragment, input, false);
	if (rest.startsWith('/')) {
		const { path, query } = splitQuery(rest, false);
		return { protocol, username: '', password: '', hostname: '', hasHost: false, port: '', path: normalizePath(path.slice(1).split('/')), opaque: null, query, fragment };
	}
	const { path, query } = splitQuery(rest, false);
	return { protocol, username: '', password: '', hostname: '', hasHost: false, port: '', path: [], opaque: encode(path, C0), query, fragment };
}

function splitQuery(s: string, special: boolean): { path: string; query: string | null } {
	const q = s.indexOf('?');
	if (q < 0) return { path: s, query: null };
	return { path: s.slice(0, q), query: encode(s.slice(q + 1), special ? SPECIAL_QUERY : QUERY) };
}

function withAuthority(protocol: string, rest: string, fragment: string | null, input: string, file: boolean): Parts {
	const special = protocol in SPECIAL;
	const end = rest.search(special ? /[/\\?]/ : /[/?]/);
	const authority = end < 0 ? rest : rest.slice(0, end);
	let tail = end < 0 ? '' : rest.slice(end);
	if (special) tail = tail.replace(/^[^?]*/, (m) => m.replace(/\\/g, '/'));

	let username = '';
	let password = '';
	let hostport = authority;
	const at = authority.lastIndexOf('@');
	if (at >= 0) {
		const userinfo = authority.slice(0, at);
		hostport = authority.slice(at + 1);
		const colon = userinfo.indexOf(':');
		username = encode(colon < 0 ? userinfo : userinfo.slice(0, colon), USERINFO);
		password = colon < 0 ? '' : encode(userinfo.slice(colon + 1), USERINFO);
	}
	let host = hostport;
	let port = '';
	const portMatch = /:([0-9]*)$/.exec(hostport);
	if (portMatch && !hostport.endsWith(']')) {
		host = hostport.slice(0, portMatch.index);
		if (portMatch[1]) {
			const n = parseInt(portMatch[1], 10);
			if (n > 65535) throw invalid(input);
			port = String(n);
		}
	} else if (/:[^\]]*$/.test(hostport) && !hostport.startsWith('[')) {
		throw invalid(input);
	}
	if (SPECIAL[protocol] === port) port = '';
	if (file && host.toLowerCase() === 'localhost') host = '';
	if (special && !file && host === '') throw invalid(input);
	const hostname = host === '' ? '' : parseHost(host, special, input);

	const { path, query } = splitQuery(tail, special);
	const segments = path === '' ? (special ? [''] : []) : normalizePath(path.slice(1).split('/'));
	return { protocol, username, password, hostname, hasHost: true, port, path: segments, opaque: null, query, fragment };
}

function resolve(rest: string, fragment: string | null, base: Parts, input: string): Parts {
	const special = base.protocol in SPECIAL;
	if (special) rest = rest.replace(/\\/g, '/');
	if (base.opaque !== null) {
		if (rest !== '' || fragment === null) throw invalid(input);
		return { ...base, fragment };
	}
	if (rest.startsWith('//')) return withAuthority(base.protocol, rest.slice(2), fragment, input, base.protocol === 'file:');
	if (rest === '') return { ...base, path: [...base.path], fragment };
	if (rest.startsWith('?')) {
		return { ...base, path: [...base.path], query: encode(rest.slice(1), special ? SPECIAL_QUERY : QUERY), fragment };
	}
	const { path, query } = splitQuery(rest, special);
	let segments: string[];
	if (path.startsWith('/')) {
		segments = path.slice(1).split('/');
	} else {
		segments = [...base.path.slice(0, -1), ...path.split('/')];
	}
	return { ...base, path: normalizePath(segments), query, fragment };
}

function serialize(p: Parts, excludeFragment = false): string {
	let s = p.protocol;
	if (p.hasHost) {
		s += '//';
		if (p.username || p.password) s += p.username + (p.password ? ':' + p.password : '') + '@';
		s += p.hostname + (p.port ? ':' + p.port : '');
	} else if (p.opaque === null && p.path.length > 1 && p.path[0] === '') {
		s += '/.';
	}
	s += p.opaque ?? (p.path.length || p.hasHost ? '/' + p.path.join('/') : '');
	if (p.query !== null) s += '?' + p.query;
	if (!excludeFragment && p.fragment !== null) s += '#' + p.fragment;
	return s;
}

export class URLSearchParams {
	#list: [string, string][] = [];
	#url: URL | null = null;

	constructor(init?: string | Record<string, string> | Iterable<[string, string]> | URLSearchParams) {
		if (init == null) return;
		if (typeof init === 'string') {
			this.#list = parseQuery(init.startsWith('?') ? init.slice(1) : init);
		} else if (typeof (init as Iterable<[string, string]>)[Symbol.iterator] === 'function') {
			for (const pair of init as Iterable<[string, string]>) {
				const [k, v] = [...pair];
				if (pair.length !== 2 && [...pair].length !== 2) throw new TypeError('Each pair needs a name and a value');
				this.#list.push([String(k), String(v)]);
			}
		} else {
			for (const [k, v] of Object.entries(init)) this.#list.push([k, String(v)]);
		}
	}

	/** @internal Links the params to their URL, so changes update it. */
	static _link(params: URLSearchParams, url: URL, query: string | null) {
		params.#url = url;
		params.#list = query ? parseQuery(query) : [];
	}

	/** @internal */
	static _reset(params: URLSearchParams, query: string | null) {
		params.#list = query ? parseQuery(query) : [];
	}

	#update() {
		if (this.#url) URL._setQuery(this.#url, this.toString());
	}

	get size(): number {
		return this.#list.length;
	}
	append(name: string, value: string) {
		this.#list.push([String(name), String(value)]);
		this.#update();
	}
	delete(name: string, value?: string) {
		this.#list = this.#list.filter(([k, v]) => !(k === name && (value === undefined || v === String(value))));
		this.#update();
	}
	get(name: string): string | null {
		return this.#list.find(([k]) => k === name)?.[1] ?? null;
	}
	getAll(name: string): string[] {
		return this.#list.filter(([k]) => k === name).map(([, v]) => v);
	}
	has(name: string, value?: string): boolean {
		return this.#list.some(([k, v]) => k === name && (value === undefined || v === String(value)));
	}
	set(name: string, value: string) {
		const i = this.#list.findIndex(([k]) => k === name);
		if (i < 0) this.#list.push([String(name), String(value)]);
		else {
			this.#list[i] = [String(name), String(value)];
			this.#list = this.#list.filter(([k], j) => j <= i || k !== name);
		}
		this.#update();
	}
	sort() {
		this.#list = this.#list
			.map((e, i) => [e, i] as const)
			.sort(([a, i], [b, j]) => (a[0] < b[0] ? -1 : a[0] > b[0] ? 1 : i - j))
			.map(([e]) => e);
		this.#update();
	}
	forEach(fn: (value: string, name: string, params: URLSearchParams) => void, thisArg?: unknown) {
		for (const [k, v] of this.#list) fn.call(thisArg, v, k, this);
	}
	*entries(): IterableIterator<[string, string]> {
		for (const [k, v] of this.#list) yield [k, v];
	}
	*keys(): IterableIterator<string> {
		for (const [k] of this.#list) yield k;
	}
	*values(): IterableIterator<string> {
		for (const [, v] of this.#list) yield v;
	}
	[Symbol.iterator]() {
		return this.entries();
	}
	toString(): string {
		return this.#list.map(([k, v]) => encode(k, FORM, true) + '=' + encode(v, FORM, true)).join('&');
	}
	get [Symbol.toStringTag]() {
		return 'URLSearchParams';
	}
}

function parseQuery(q: string): [string, string][] {
	const out: [string, string][] = [];
	for (const part of q.split('&')) {
		if (!part) continue;
		const eq = part.indexOf('=');
		const k = eq < 0 ? part : part.slice(0, eq);
		const v = eq < 0 ? '' : part.slice(eq + 1);
		out.push([decode(k.replace(/\+/g, ' ')), decode(v.replace(/\+/g, ' '))]);
	}
	return out;
}

export class URL {
	#p: Parts;
	#params: URLSearchParams | null = null;

	constructor(url: string | URL, base?: string | URL) {
		const b = base === undefined ? undefined : parse(String(base));
		this.#p = parse(String(url), b);
	}

	static canParse(url: string, base?: string): boolean {
		try {
			new URL(url, base);
			return true;
		} catch {
			return false;
		}
	}

	static parse(url: string, base?: string): URL | null {
		try {
			return new URL(url, base);
		} catch {
			return null;
		}
	}

	/** @internal */
	static _setQuery(url: URL, query: string) {
		url.#p.query = query === '' ? null : query;
	}

	get href(): string {
		return serialize(this.#p);
	}
	set href(v: string) {
		this.#p = parse(v);
		if (this.#params) URLSearchParams._reset(this.#params, this.#p.query);
	}
	get origin(): string {
		const p = this.#p;
		if (p.protocol in SPECIAL && p.protocol !== 'file:') return p.protocol + '//' + p.hostname + (p.port ? ':' + p.port : '');
		if (p.protocol === 'blob:') {
			try {
				return new URL(p.opaque ?? '').origin;
			} catch {
				return 'null';
			}
		}
		return 'null';
	}
	get protocol(): string {
		return this.#p.protocol;
	}
	set protocol(v: string) {
		const m = /^([a-zA-Z][a-zA-Z0-9+.-]*)/.exec(v);
		if (!m) return;
		const next = m[1]!.toLowerCase() + ':';
		if (next in SPECIAL !== this.#p.protocol in SPECIAL) return;
		this.#p.protocol = next;
		if (SPECIAL[next] === this.#p.port) this.#p.port = '';
	}
	get username(): string {
		return this.#p.username;
	}
	set username(v: string) {
		if (this.#p.hasHost && this.#p.hostname) this.#p.username = encode(v, USERINFO);
	}
	get password(): string {
		return this.#p.password;
	}
	set password(v: string) {
		if (this.#p.hasHost && this.#p.hostname) this.#p.password = encode(v, USERINFO);
	}
	get host(): string {
		return this.#p.hostname + (this.#p.port ? ':' + this.#p.port : '');
	}
	set host(v: string) {
		if (this.#p.opaque !== null) return;
		try {
			const next = parse(this.#p.protocol + '//' + v);
			this.#p.hostname = next.hostname;
			this.#p.port = next.port;
			this.#p.hasHost = true;
		} catch {
			// invalid hosts are ignored, as in browsers
		}
	}
	get hostname(): string {
		return this.#p.hostname;
	}
	set hostname(v: string) {
		if (this.#p.opaque !== null) return;
		try {
			this.#p.hostname = parse(this.#p.protocol + '//' + v).hostname;
			this.#p.hasHost = true;
		} catch {
			// ignored
		}
	}
	get port(): string {
		return this.#p.port;
	}
	set port(v: string) {
		if (!this.#p.hostname || this.#p.protocol === 'file:') return;
		const m = /^[0-9]*/.exec(String(v))![0];
		if (m === '') {
			if (String(v) === '') this.#p.port = '';
			return;
		}
		const n = parseInt(m, 10);
		if (n > 65535) return;
		this.#p.port = SPECIAL[this.#p.protocol] === String(n) ? '' : String(n);
	}
	get pathname(): string {
		const p = this.#p;
		return p.opaque ?? (p.path.length || p.hasHost ? '/' + p.path.join('/') : '');
	}
	set pathname(v: string) {
		if (this.#p.opaque !== null) return;
		const special = this.#p.protocol in SPECIAL;
		let s = String(v);
		if (special) s = s.replace(/\\/g, '/');
		s = s.replace(/[?#]/g, (c) => (c === '?' ? '%3F' : '%23'));
		this.#p.path = normalizePath((s.startsWith('/') ? s.slice(1) : s).split('/'));
	}
	get search(): string {
		return this.#p.query ? '?' + this.#p.query : '';
	}
	set search(v: string) {
		const s = String(v);
		const q = s.startsWith('?') ? s.slice(1) : s;
		this.#p.query = s === '' ? null : encode(q, this.#p.protocol in SPECIAL ? SPECIAL_QUERY : QUERY);
		if (this.#params) URLSearchParams._reset(this.#params, this.#p.query);
	}
	get searchParams(): URLSearchParams {
		if (!this.#params) {
			this.#params = new URLSearchParams();
			URLSearchParams._link(this.#params, this, this.#p.query);
		}
		return this.#params;
	}
	get hash(): string {
		return this.#p.fragment ? '#' + this.#p.fragment : '';
	}
	set hash(v: string) {
		const s = String(v);
		this.#p.fragment = s === '' ? null : encode(s.startsWith('#') ? s.slice(1) : s, FRAGMENT);
	}
	toString(): string {
		return this.href;
	}
	toJSON(): string {
		return this.href;
	}
	get [Symbol.toStringTag]() {
		return 'URL';
	}
}

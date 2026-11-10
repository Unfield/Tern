import { describe, expect, it } from 'vitest';
import { URL as TernURL, URLSearchParams as TernParams } from './url.js';

const fields = ['href', 'origin', 'protocol', 'username', 'password', 'host', 'hostname', 'port', 'pathname', 'search', 'hash'] as const;

function same(input: string, base?: string) {
	let want: globalThis.URL | null = null;
	try {
		want = new globalThis.URL(input, base);
	} catch {
		// invalid
	}
	if (!want) {
		expect(() => new TernURL(input, base), `${input} should be invalid`).toThrow(TypeError);
		return;
	}
	const got = new TernURL(input, base);
	for (const f of fields) expect(got[f], `${f} of ${JSON.stringify(input)} against ${base}`).toBe(want[f]);
	expect([...got.searchParams]).toEqual([...want.searchParams]);
}

describe('URL', () => {
	it.each([
		'https://example.com',
		'https://example.com/',
		'HTTPS://Example.COM:443/a/b?x=1#frag',
		'http://example.com:8080/path',
		'http://user:pa%20ss@example.com/',
		'http://user@example.com/',
		'https://example.com/a/./b/../c/',
		'https://example.com/a/b/..',
		'https://example.com/a/%2e%2E/b',
		'https://example.com/über?q=ä ö#ü',
		'https://example.com/a b?c d#e f',
		'https://example.com/?a=1&b=2&a=3',
		'https://example.com/?q=a+b%20c&empty=&novalue',
		'https://example.com/?',
		'https://example.com/#',
		'https://example.com/path?x=\'quote"',
		'http://127.0.0.1:3000/',
		'http://0x7f.1/',
		'http://[::1]:8080/x',
		'http://localhost',
		'http:\\\\example.com\\a\\b',
		'  https://example.com/trim  ',
		'https://exa\tmple.com/a\nb',
		'file:///etc/passwd',
		'file://localhost/etc',
		'mailto:someone@example.com?subject=hi',
		'data:text/plain,hello world',
		'javascript:alert(1)',
		'tel:+1-555',
		'git+ssh://git@github.com/x/y.git',
		'custom://host/path?q#f',
		'custom:/path',
		'blob:https://example.com/uuid',
		'ws://example.com:80/socket',
		'wss://example.com/socket',
		'https://example.com:65535/',
		'https://example.com:65536/',
		'https://',
		'http://exa mple.com/',
		'not a url',
		'',
		'https://example.com:abc/'
	])('parses %j like Node', (input) => same(input));

	it.each([
		['/a/b', 'https://example.com/x/y'],
		['b', 'https://example.com/x/y'],
		['./b', 'https://example.com/x/y/'],
		['../b', 'https://example.com/x/y/z'],
		['../../../../b', 'https://example.com/x'],
		['?q=1', 'https://example.com/x/y?old#h'],
		['#frag', 'https://example.com/x?q'],
		['', 'https://example.com/x?q#h'],
		['//other.com/p', 'https://example.com/x'],
		['http:foo', 'http://example.com/a/'],
		['https:foo', 'http://example.com/a/'],
		['https://abs.com', 'http://example.com/a/'],
		['x', 'mailto:a@b.com'],
		['', 'mailto:a@b.com'],
		['#x', 'mailto:a@b.com'],
		['\\a\\b', 'http://example.com/x'],
		['b?c#d', 'custom://h/a/']
	])('resolves %j against %j like Node', (input, base) => same(input, base));

	it('updates through setters like Node', () => {
		const ops: [string, string][] = [
			['pathname', '/new path/x'],
			['search', '?a=1 2'],
			['search', ''],
			['hash', 'top'],
			['hash', ''],
			['port', '8080'],
			['port', '443'],
			['port', ''],
			['hostname', 'Other.Example'],
			['host', 'h.com:81'],
			['protocol', 'http'],
			['username', 'me'],
			['password', 's e'],
			['href', 'https://reset.com/a?b=c']
		];
		const want = new globalThis.URL('https://example.com/a?x=1#h');
		const got = new TernURL('https://example.com/a?x=1#h');
		for (const [k, v] of ops) {
			(want as any)[k] = v;
			(got as any)[k] = v;
			for (const f of fields) expect(got[f], `${f} after setting ${k}=${v}`).toBe(want[f]);
		}
	});

	it('keeps searchParams and the URL in sync', () => {
		const want = new globalThis.URL('https://example.com/?a=1&b=2');
		const got = new TernURL('https://example.com/?a=1&b=2');
		for (const u of [want, got]) {
			u.searchParams.set('a', 'x y');
			u.searchParams.append('c', 'ä&=');
			u.searchParams.delete('b');
		}
		expect(got.href).toBe(want.href);
		got.search = '?z=9';
		want.search = '?z=9';
		expect(got.searchParams.get('z')).toBe('9');
		expect(got.searchParams.get('a')).toBe(null);
		got.searchParams.delete('z');
		want.searchParams.delete('z');
		expect(got.href).toBe(want.href);
		expect(String(got)).toBe(want.toString());
		expect(JSON.stringify({ u: got })).toBe(JSON.stringify({ u: want }));
	});

	it('has the static helpers', () => {
		expect(TernURL.canParse('/x', 'https://a.com')).toBe(true);
		expect(TernURL.canParse('/x')).toBe(false);
		expect(TernURL.parse('nope')).toBe(null);
		expect(TernURL.parse('https://a.com/b')?.pathname).toBe('/b');
	});
});

describe('URLSearchParams', () => {
	it.each(['a=1&b=2&a=3', '?x=%20y+z', 'k', '=v', 'a=1&&b', 'emoji=%F0%9F%98%80', 'bad=%zz%', 'ä=ö', ''])('parses %j like Node', (input) => {
		const want = new globalThis.URLSearchParams(input);
		const got = new TernParams(input);
		expect([...got]).toEqual([...want]);
		expect(got.toString()).toBe(want.toString());
		expect(got.size).toBe(want.size);
	});

	it('supports the whole API like Node', () => {
		const init = { b: '2', a: '1 1', c: '*-._~!' };
		const want = new globalThis.URLSearchParams(init);
		const got = new TernParams(init);
		for (const p of [want, got]) {
			p.append('a', '0');
			p.sort();
			p.set('b', 'new');
		}
		expect(got.toString()).toBe(want.toString());
		expect(got.getAll('a')).toEqual(want.getAll('a'));
		expect(got.has('a', '0')).toBe(want.has('a', '0'));
		expect([...got.keys()]).toEqual([...want.keys()]);
		expect([...got.values()]).toEqual([...want.values()]);
		const seen: string[] = [];
		got.forEach((v, k) => seen.push(`${k}=${v}`));
		expect(seen).toEqual([...want].map(([k, v]) => `${k}=${v}`));
		expect(new TernParams([['x', '1'], ['y', '2']]).toString()).toBe('x=1&y=2');
		expect(new TernParams(got).toString()).toBe(got.toString());
	});
});

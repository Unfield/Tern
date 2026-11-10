import { describe, expect, it } from 'vitest';
import { install } from './web.js';

// Install into an empty object, so Node's own implementations don't hide ours.
const g: Record<string, any> = {};
install(g);

describe('server web APIs', () => {
	it('does not replace existing globals', () => {
		const existing = { setTimeout: 1 };
		install(existing);
		expect(existing.setTimeout).toBe(1);
	});

	it('encodes and decodes UTF-8 like Node', () => {
		for (const s of ['', 'hello', 'grüße', '日本語', '😀 emoji', 'a\u0000b', '\ud800 lone']) {
			const ours = new g.TextEncoder().encode(s);
			expect([...ours]).toEqual([...new TextEncoder().encode(s)]);
			expect(new g.TextDecoder().decode(ours)).toBe(new TextDecoder().decode(ours));
		}
		const bad = new Uint8Array([0x61, 0xff, 0xc3, 0x28, 0xe2, 0x82]);
		expect(new g.TextDecoder().decode(bad)).toBe(new TextDecoder().decode(bad));
		expect(() => new g.TextDecoder('utf-8', { fatal: true }).decode(bad)).toThrow(TypeError);
		const bom = new Uint8Array([0xef, 0xbb, 0xbf, 0x41]);
		expect(new g.TextDecoder().decode(bom)).toBe('A');
		expect(new g.TextDecoder().decode(bom.buffer)).toBe('A');
		expect(() => new g.TextDecoder('latin1')).toThrow(RangeError);
	});

	it('has atob and btoa like Node', () => {
		for (const s of ['', 'f', 'fo', 'foo', 'foob', 'hello world', 'ÿ\u0000\u0080']) {
			expect(g.btoa(s)).toBe(btoa(s));
			expect(g.atob(btoa(s))).toBe(s);
		}
		expect(g.atob(' Zm9v\nYmFy ')).toBe('foobar');
		expect(g.atob('Zm9vYg')).toBe('foob');
		expect(() => g.btoa('€')).toThrow();
		expect(() => g.atob('a')).toThrow();
	});

	it('generates random values and UUIDs', () => {
		const a = g.crypto.getRandomValues(new Uint32Array(8));
		expect(a).toHaveLength(8);
		expect(new Set(a).size).toBeGreaterThan(1);
		expect(g.crypto.randomUUID()).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/);
		expect(() => g.crypto.getRandomValues(new Uint8Array(70000))).toThrow();
	});

	it('clones structured data with cycles', () => {
		const src: any = { d: new Date(5), m: new Map([[1, { x: 1 }]]), s: new Set([1, 2]), r: /a/gi, u: new Uint8Array([1, 2]), arr: [1, { y: 2 }] };
		src.self = src;
		const c = g.structuredClone(src);
		expect(c).not.toBe(src);
		expect(c.self).toBe(c);
		expect(c.d).toEqual(new Date(5));
		expect(c.m.get(1)).toEqual({ x: 1 });
		expect([...c.s]).toEqual([1, 2]);
		expect(c.r.flags).toBe('gi');
		expect([...c.u]).toEqual([1, 2]);
		expect(c.arr[1]).not.toBe(src.arr[1]);
		expect(() => g.structuredClone({ f() {} })).toThrow();
	});

	it('dispatches events and aborts', () => {
		const t = new g.EventTarget();
		const seen: string[] = [];
		const fn = (e: any) => seen.push(e.type + ':' + (e.detail ?? ''));
		t.addEventListener('ping', fn);
		t.addEventListener('ping', fn); // duplicates are ignored
		t.addEventListener('ping', { handleEvent: () => seen.push('object') }, { once: true });
		expect(t.dispatchEvent(new g.CustomEvent('ping', { detail: 1 }))).toBe(true);
		t.dispatchEvent(new g.Event('ping'));
		t.removeEventListener('ping', fn);
		t.dispatchEvent(new g.Event('ping'));
		expect(seen).toEqual(['ping:1', 'object', 'ping:']);

		const e = new g.Event('x', { cancelable: true });
		t.addEventListener('x', (ev: any) => ev.preventDefault());
		expect(t.dispatchEvent(e)).toBe(false);

		const c = new g.AbortController();
		const reasons: unknown[] = [];
		c.signal.addEventListener('abort', () => reasons.push(c.signal.reason));
		const child = g.AbortSignal.any([c.signal]);
		c.abort('stop');
		c.abort('again');
		expect(reasons).toEqual(['stop']);
		expect(child.aborted).toBe(true);
		expect(() => c.signal.throwIfAborted()).toThrow();
		expect(g.AbortSignal.abort().reason.name).toBe('AbortError');
	});

	it('drops timers and explains fetch', async () => {
		let ran = false;
		const id = g.setTimeout(() => (ran = true), 0);
		expect(typeof id).toBe('number');
		g.clearTimeout(id);
		expect(ran).toBe(false);
		expect(g.performance.now()).toBeGreaterThanOrEqual(0);
		await expect(g.fetch('/x')).rejects.toThrow(/Go Load function/);
	});
});

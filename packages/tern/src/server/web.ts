// Web APIs that Svelte libraries commonly touch while rendering, for the
// server renderer. QuickJS implements only the language; Node, which runs
// SvelteKit's renderer, has all of these. Each is installed only where it is
// missing.
//
// Browser-only objects (window, document, navigator, location) are left
// undefined on purpose: libraries check for them to decide whether they are
// running in a browser.

declare const __tern_random: ((n: number) => string) | undefined;

// TextEncoder and TextDecoder (UTF-8).
export class TextEncoderPolyfill {
	readonly encoding = 'utf-8';
	encode(input = ''): Uint8Array {
		const out: number[] = [];
		for (const ch of String(input)) {
			let c = ch.codePointAt(0)!;
			if (c >= 0xd800 && c <= 0xdfff) c = 0xfffd;
			if (c < 0x80) out.push(c);
			else if (c < 0x800) out.push(0xc0 | (c >> 6), 0x80 | (c & 63));
			else if (c < 0x10000) out.push(0xe0 | (c >> 12), 0x80 | ((c >> 6) & 63), 0x80 | (c & 63));
			else out.push(0xf0 | (c >> 18), 0x80 | ((c >> 12) & 63), 0x80 | ((c >> 6) & 63), 0x80 | (c & 63));
		}
		return new Uint8Array(out);
	}
	encodeInto(input: string, dest: Uint8Array): { read: number; written: number } {
		const bytes = this.encode(input);
		const n = Math.min(bytes.length, dest.length);
		dest.set(bytes.subarray(0, n));
		return {
			read: n === bytes.length ? input.length : new TextDecoderPolyfill().decode(bytes.subarray(0, n)).length,
			written: n
		};
	}
}

export class TextDecoderPolyfill {
	readonly encoding: string;
	readonly fatal: boolean;
	readonly ignoreBOM: boolean;
	constructor(label = 'utf-8', options: { fatal?: boolean; ignoreBOM?: boolean } = {}) {
		const l = String(label).toLowerCase();
		if (l !== 'utf-8' && l !== 'utf8' && l !== 'unicode-1-1-utf-8') throw new RangeError(`TextDecoder: unsupported encoding "${label}"`);
		this.encoding = 'utf-8';
		this.fatal = !!options.fatal;
		this.ignoreBOM = !!options.ignoreBOM;
	}
	decode(input?: ArrayBuffer | ArrayBufferView): string {
		if (input === undefined) return '';
		const b = input instanceof ArrayBuffer ? new Uint8Array(input) : new Uint8Array(input.buffer, input.byteOffset, input.byteLength);
		let out = '';
		let i = !this.ignoreBOM && b[0] === 0xef && b[1] === 0xbb && b[2] === 0xbf ? 3 : 0;
		const bad = () => {
			if (this.fatal) throw new TypeError('TextDecoder: invalid UTF-8');
			out += '�';
		};
		while (i < b.length) {
			const x = b[i]!;
			let n: number;
			let c: number;
			if (x < 0x80) [n, c] = [0, x];
			else if (x >= 0xc2 && x < 0xe0) [n, c] = [1, x & 31];
			else if (x >= 0xe0 && x < 0xf0) [n, c] = [2, x & 15];
			else if (x >= 0xf0 && x < 0xf5) [n, c] = [3, x & 7];
			else {
				bad();
				i++;
				continue;
			}
			// The WHATWG decoder: a malformed sequence becomes one U+FFFD
			// covering its lead byte and the continuation bytes that were valid.
			let k = 1;
			for (; k <= n; k++) {
				const y = b[i + k];
				const lo = k === 1 ? (x === 0xe0 ? 0xa0 : x === 0xf0 ? 0x90 : 0x80) : 0x80;
				const hi = k === 1 ? (x === 0xed ? 0x9f : x === 0xf4 ? 0x8f : 0xbf) : 0xbf;
				if (y === undefined || y < lo || y > hi) break;
				c = (c << 6) | (y & 63);
			}
			if (k <= n) {
				bad();
				i += k;
				continue;
			}
			out += String.fromCodePoint(c);
			i += n + 1;
		}
		return out;
	}
}

// Event, CustomEvent and EventTarget.
export class EventPolyfill {
	readonly type: string;
	readonly bubbles: boolean;
	readonly cancelable: boolean;
	readonly composed: boolean;
	readonly timeStamp = Date.now();
	defaultPrevented = false;
	target: unknown = null;
	currentTarget: unknown = null;
	/** @internal */
	_stop = false;
	constructor(type: string, init: { bubbles?: boolean; cancelable?: boolean; composed?: boolean } = {}) {
		this.type = String(type);
		this.bubbles = !!init.bubbles;
		this.cancelable = !!init.cancelable;
		this.composed = !!init.composed;
	}
	preventDefault() {
		if (this.cancelable) this.defaultPrevented = true;
	}
	stopPropagation() {}
	stopImmediatePropagation() {
		this._stop = true;
	}
}

export class CustomEventPolyfill<T = unknown> extends EventPolyfill {
	readonly detail: T | null;
	constructor(type: string, init: { detail?: T; bubbles?: boolean; cancelable?: boolean } = {}) {
		super(type, init);
		this.detail = init.detail ?? null;
	}
}

type Listener = ((e: EventPolyfill) => void) | { handleEvent(e: EventPolyfill): void };

export class EventTargetPolyfill {
	#listeners = new Map<string, { fn: Listener; once: boolean }[]>();
	addEventListener(type: string, fn: Listener | null, options?: boolean | { once?: boolean; signal?: AbortSignalPolyfill }) {
		if (!fn) return;
		const opts = typeof options === 'object' ? options : {};
		if (opts.signal?.aborted) return;
		const list = this.#listeners.get(type) ?? [];
		if (list.some((l) => l.fn === fn)) return;
		list.push({ fn, once: !!opts.once });
		this.#listeners.set(type, list);
		opts.signal?.addEventListener('abort', () => this.removeEventListener(type, fn));
	}
	removeEventListener(type: string, fn: Listener | null) {
		const list = this.#listeners.get(type);
		if (list)
			this.#listeners.set(
				type,
				list.filter((l) => l.fn !== fn)
			);
	}
	dispatchEvent(event: EventPolyfill): boolean {
		event.target = this;
		event.currentTarget = this;
		for (const l of [...(this.#listeners.get(event.type) ?? [])]) {
			if (l.once) this.removeEventListener(event.type, l.fn);
			if (typeof l.fn === 'function') l.fn.call(this, event);
			else l.fn.handleEvent(event);
			if (event._stop) break;
		}
		event.currentTarget = null;
		return !event.defaultPrevented;
	}
}

export class AbortSignalPolyfill extends EventTargetPolyfill {
	aborted = false;
	reason: unknown = undefined;
	onabort: ((e: EventPolyfill) => void) | null = null;
	throwIfAborted() {
		if (this.aborted) throw this.reason;
	}
	static abort(reason?: unknown) {
		const c = new AbortControllerPolyfill();
		c.abort(reason);
		return c.signal;
	}
	static timeout() {
		return new AbortSignalPolyfill(); // timers never fire during a render
	}
	static any(signals: AbortSignalPolyfill[]) {
		const c = new AbortControllerPolyfill();
		for (const s of signals) {
			if (s.aborted) {
				c.abort(s.reason);
				break;
			}
			s.addEventListener('abort', () => c.abort(s.reason), { once: true });
		}
		return c.signal;
	}
}

export class AbortControllerPolyfill {
	readonly signal = new AbortSignalPolyfill();
	abort(reason?: unknown) {
		const s = this.signal;
		if (s.aborted) return;
		s.aborted = true;
		s.reason =
			reason ??
			Object.assign(new Error('This operation was aborted'), {
				name: 'AbortError'
			});
		const e = new EventPolyfill('abort');
		s.onabort?.(e);
		s.dispatchEvent(e);
	}
}

/** Installs every missing API on g (globalThis in the renderer). */
export function install(g: Record<string, unknown>) {
	const define = (name: string, value: unknown) => {
		if (typeof g[name] === 'undefined') {
			Object.defineProperty(g, name, {
				value,
				writable: true,
				configurable: true,
				enumerable: false
			});
		}
	};

	// Timers. A render is synchronous and its HTML is sent when it returns, so a
	// timer set during rendering could not change the output; in Node it would
	// fire after the response was sent. Callbacks are dropped, not kept, so they
	// cannot hold memory across renders.
	{
		let next = 1;
		const set = () => next++;
		const clear = () => {};
		define('setTimeout', set);
		define('setInterval', set);
		define('setImmediate', set);
		define('clearTimeout', clear);
		define('clearInterval', clear);
		define('clearImmediate', clear);
		define('requestAnimationFrame', undefined);
	}

	// performance.now(), for libraries that measure or seed with it.
	{
		const origin = Date.now();
		define('performance', {
			timeOrigin: origin,
			now: () => Date.now() - origin,
			mark: () => undefined,
			measure: () => undefined,
			getEntriesByName: () => [],
			getEntriesByType: () => [],
			clearMarks: () => {},
			clearMeasures: () => {}
		});
	}

	// atob and btoa.
	{
		const chars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/';
		define('btoa', (input: string) => {
			const s = String(input);
			let out = '';
			for (let i = 0; i < s.length; i += 3) {
				const codes = [s.charCodeAt(i), s.charCodeAt(i + 1), s.charCodeAt(i + 2)];
				for (const c of codes) if (c > 255) throw new Error('btoa: the string contains characters outside of Latin1');
				const [a, b, c] = codes as [number, number, number];
				const n = (a << 16) | ((b || 0) << 8) | (c || 0);
				out +=
					chars[n >> 18]! + chars[(n >> 12) & 63]! + (i + 1 < s.length ? chars[(n >> 6) & 63]! : '=') + (i + 2 < s.length ? chars[n & 63]! : '=');
			}
			return out;
		});
		define('atob', (input: string) => {
			let s = String(input).replace(/[\t\n\f\r ]/g, '');
			if (s.length % 4 === 0) s = s.replace(/==?$/, '');
			if (s.length % 4 === 1 || /[^A-Za-z0-9+/]/.test(s)) throw new Error('atob: the string is not correctly encoded');
			let out = '';
			let bits = 0;
			let acc = 0;
			for (const ch of s) {
				acc = (acc << 6) | chars.indexOf(ch);
				bits += 6;
				if (bits >= 8) {
					bits -= 8;
					out += String.fromCharCode((acc >> bits) & 255);
				}
			}
			return out;
		});
	}

	// crypto.getRandomValues and crypto.randomUUID, from Go's crypto/rand.
	{
		const getRandomValues = <T extends ArrayBufferView | null>(array: T): T => {
			if (!array) return array;
			const bytes = new Uint8Array(array.buffer, array.byteOffset, array.byteLength);
			if (bytes.length > 65536) throw new Error('getRandomValues: at most 65536 bytes');
			if (typeof __tern_random === 'function') {
				const hex = __tern_random(bytes.length);
				for (let i = 0; i < bytes.length; i++) bytes[i] = parseInt(hex.slice(i * 2, i * 2 + 2), 16);
			} else {
				for (let i = 0; i < bytes.length; i++) bytes[i] = Math.floor(Math.random() * 256);
			}
			return array;
		};
		const randomUUID = () => {
			const b = getRandomValues(new Uint8Array(16));
			b[6] = (b[6]! & 0x0f) | 0x40;
			b[8] = (b[8]! & 0x3f) | 0x80;
			const h = [...b].map((x) => x.toString(16).padStart(2, '0')).join('');
			return `${h.slice(0, 8)}-${h.slice(8, 12)}-${h.slice(12, 16)}-${h.slice(16, 20)}-${h.slice(20)}`;
		};
		define('crypto', { getRandomValues, randomUUID });
	}

	// structuredClone.
	{
		const clone = (value: unknown, seen: Map<unknown, unknown>): unknown => {
			if (typeof value !== 'object' || value === null) {
				if (typeof value === 'function' || typeof value === 'symbol') throw new Error(`structuredClone: ${typeof value} could not be cloned`);
				return value;
			}
			if (seen.has(value)) return seen.get(value);
			let out: unknown;
			if (value instanceof Date) out = new Date(value.getTime());
			else if (value instanceof RegExp) out = new RegExp(value.source, value.flags);
			else if (value instanceof ArrayBuffer) out = value.slice(0);
			else if (ArrayBuffer.isView(value)) {
				const C = value.constructor as new (b: ArrayBuffer, o: number, l: number) => unknown;
				const buf = value.buffer.slice(value.byteOffset, value.byteOffset + value.byteLength);
				out = value instanceof DataView ? new DataView(buf as ArrayBuffer) : new C(buf as ArrayBuffer, 0, (value as unknown as { length: number }).length);
			} else if (value instanceof Map) {
				const m = new Map();
				seen.set(value, m);
				for (const [k, v] of value) m.set(clone(k, seen), clone(v, seen));
				return m;
			} else if (value instanceof Set) {
				const s = new Set();
				seen.set(value, s);
				for (const v of value) s.add(clone(v, seen));
				return s;
			} else if (value instanceof Error) {
				const e = new (value.constructor as ErrorConstructor)(value.message);
				if (value.stack) e.stack = value.stack;
				out = e;
			} else if (Array.isArray(value)) {
				const a: unknown[] = [];
				seen.set(value, a);
				value.forEach((v, i) => (a[i] = clone(v, seen)));
				return a;
			} else {
				const o: Record<string, unknown> = {};
				seen.set(value, o);
				for (const k of Object.keys(value)) o[k] = clone((value as Record<string, unknown>)[k], seen);
				return o;
			}
			seen.set(value, out);
			return out;
		};
		define('structuredClone', (value: unknown) => clone(value, new Map()));
	}

	define('TextEncoder', TextEncoderPolyfill);
	define('TextDecoder', TextDecoderPolyfill);
	define('Event', EventPolyfill);
	define('CustomEvent', CustomEventPolyfill);
	define('EventTarget', EventTargetPolyfill);
	define('AbortSignal', AbortSignalPolyfill);
	define('AbortController', AbortControllerPolyfill);

	// fetch is not available while rendering: in Tern, data comes from Go load
	// functions. Fail with an explanation instead of "fetch is not defined".
	define('fetch', () =>
		Promise.reject(new Error('fetch is not available during server rendering. Load data in a Go Load function, or fetch in onMount.'))
	);
}

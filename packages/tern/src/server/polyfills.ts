// Web APIs the server-side renderer needs. Tern renders in an embedded
// JavaScript engine (QuickJS) that only implements the language itself.
// Each polyfill is skipped where the API already exists, such as in Node
// during development.
import { IntlShim } from './intl.js';
import { URL, URLSearchParams } from './url.js';
import { install } from './web.js';

declare const __tern_log: ((level: string, message: string) => void) | undefined;

const g = globalThis as Record<string, unknown>;

install(g);

// Some libraries read process.env.SOMETHING at runtime. Give them an empty
// environment instead of a ReferenceError.
if (typeof g.process === 'undefined') {
	g.process = { env: { NODE_ENV: 'production' }, browser: false, version: '', versions: {} };
}

if (typeof g.URL === 'undefined') {
	g.URL = URL;
	g.URLSearchParams = URLSearchParams;
}

if (typeof g.Intl === 'undefined') {
	g.Intl = IntlShim;
	// QuickJS's own toLocaleString ignores locales: (5e6).toLocaleString()
	// is "5000000" where browsers print "5,000,000". Route them through the
	// shim so server-rendered text matches the browser's en-US output.
	const NF = IntlShim.NumberFormat as unknown as new (l?: unknown, o?: object) => { format(n: number): string };
	const DF = IntlShim.DateTimeFormat as unknown as new (l?: unknown, o?: object) => { format(d: Date): string };
	Object.defineProperty(Number.prototype, 'toLocaleString', {
		value(this: number, locales?: unknown, options?: object) {
			return new NF(locales, options).format(Number(this));
		},
		configurable: true,
		writable: true
	});
	const dateMethod = (defaults: object) =>
		function (this: Date, locales?: unknown, options?: object) {
			return new DF(locales, { ...defaults, ...options }).format(this);
		};
	for (const [name, defaults] of [
		['toLocaleString', { dateStyle: 'short', timeStyle: 'medium' }],
		['toLocaleDateString', { dateStyle: 'short' }],
		['toLocaleTimeString', { timeStyle: 'medium' }]
	] as const) {
		Object.defineProperty(Date.prototype, name, { value: dateMethod(defaults), configurable: true, writable: true });
	}
}

if (typeof g.queueMicrotask !== 'function') {
	g.queueMicrotask = (fn: () => void) => void Promise.resolve().then(fn);
}

if (typeof g.console === 'undefined') {
	const format = (args: unknown[]) =>
		args
			.map((a) => {
				if (typeof a === 'string') return a;
				if (a instanceof Error) return a.stack ?? a.message;
				try {
					return JSON.stringify(a);
				} catch {
					return String(a);
				}
			})
			.join(' ');
	const log = (level: string) => (...args: unknown[]) => {
		if (typeof __tern_log === 'function') __tern_log(level, format(args));
	};
	g.console = {
		log: log('info'),
		info: log('info'),
		debug: log('debug'),
		warn: log('warn'),
		error: log('error'),
		trace: log('debug')
	};
}

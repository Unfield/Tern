// Web APIs the server-side renderer needs. Tern renders in an embedded
// JavaScript engine (QuickJS) that only implements the language itself.
// Each polyfill is skipped where the API already exists, such as in Node
// during development.
import { IntlShim } from './intl.js';
import { URL, URLSearchParams } from './url.js';

declare const __tern_log: ((level: string, message: string) => void) | undefined;

const g = globalThis as Record<string, unknown>;

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

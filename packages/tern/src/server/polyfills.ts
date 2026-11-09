// Web APIs the server-side renderer needs. Tern renders in an embedded
// JavaScript engine (QuickJS) that only implements the language itself.
// Each polyfill is skipped where the API already exists, such as in Node
// during development.
import 'core-js/actual/url';
import 'core-js/actual/url-search-params';

declare const __tern_log: ((level: string, message: string) => void) | undefined;

const g = globalThis as Record<string, unknown>;

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

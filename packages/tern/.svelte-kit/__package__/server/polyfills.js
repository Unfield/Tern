// Web APIs the server-side renderer needs. Tern renders in an embedded
// JavaScript engine (QuickJS) that only implements the language itself.
// Each polyfill is skipped where the API already exists, such as in Node
// during development.
import 'core-js/actual/url';
import 'core-js/actual/url-search-params';
const g = globalThis;
if (typeof g.queueMicrotask !== 'function') {
    g.queueMicrotask = (fn) => void Promise.resolve().then(fn);
}
if (typeof g.console === 'undefined') {
    const format = (args) => args
        .map((a) => {
        if (typeof a === 'string')
            return a;
        if (a instanceof Error)
            return a.stack ?? a.message;
        try {
            return JSON.stringify(a);
        }
        catch {
            return String(a);
        }
    })
        .join(' ');
    const log = (level) => (...args) => {
        if (typeof __tern_log === 'function')
            __tern_log(level, format(args));
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

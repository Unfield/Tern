
// tern dev's overlay. A button in the corner of every page opens a panel
// with the page's route, files and data, and each request's spans and
// logs; errors, from the server or the browser, open over the page with
// their stacks. The router and forms announce what they did with
// "tern:dev" events (packages/tern/src/runtime/dev.ts), and the rest comes
// from tern dev's API.
(() => {
	if (window.__ternOverlay) return;
	window.__ternOverlay = true;

	const HIDDEN = 'tern-dev-overlay-hidden';
	const state = {
		pages: [], // tern:dev page events, oldest first
		requests: new Map(), // id -> { id, label, kind, status, items: Map(seq -> item), traces: Set }
		order: [], // request ids, oldest first
		errors: [],
		reports: new Set(),
		seq: 0,
		stream: null,
		files: new Map(), // component -> route file info
		open: false,
		tab: 'page',
		selected: null,
		overlay: false,
		errorIndex: 0,
		hidden: (() => {
			try {
				return localStorage.getItem(HIDDEN) === '1';
			} catch {
				return false;
			}
		})()
	};

	// ---- data ----

	const api = (path) => fetch('/__tern/dev/api/' + path, { credentials: 'same-origin' }).then((r) => (r.ok ? r.json() : Promise.reject(new Error(r.statusText))));

	function request(id, label, kind, status) {
		if (!id) return null;
		let r = state.requests.get(id);
		if (!r) {
			r = { id, label, kind, status, items: new Map(), traces: new Set() };
			state.requests.set(id, r);
			state.order.push(id);
			if (state.order.length > 30) state.requests.delete(state.order.shift());
			api('request?id=' + encodeURIComponent(id))
				.then((res) => {
					for (const it of res.items) take(it);
					connect(res.seq);
					render();
				})
				.catch(() => {});
		}
		return r;
	}

	function connect(seq) {
		if (state.stream) return;
		state.stream = new EventSource('/__tern/dev/logs/stream?after=' + seq);
		state.stream.onmessage = (e) => {
			let items;
			try {
				items = JSON.parse(e.data);
			} catch {
				return;
			}
			let changed = false;
			for (const it of items) changed = take(it) || changed;
			if (changed) render();
		};
	}

	// Spans end, and arrive, before the request's own span, which is the
	// one that names the request: they wait here until their trace is known.
	const strays = new Map(); // trace -> items

	// take files an item under its request; it reports whether it was one of ours.
	function take(it) {
		let r = it.request_id && state.requests.get(it.request_id);
		if (!r && it.span) {
			for (const id of state.order) {
				const c = state.requests.get(id);
				if (c && c.traces.has(it.span.trace)) r = c;
			}
			if (!r) {
				if (!strays.has(it.span.trace)) strays.set(it.span.trace, []);
				strays.get(it.span.trace).push(it);
				if (strays.size > 200) strays.delete(strays.keys().next().value);
				return false;
			}
		}
		if (!r) return false;
		r.items.set(it.seq, it);
		if (it.span && !r.traces.has(it.span.trace)) {
			r.traces.add(it.span.trace);
			for (const s of strays.get(it.span.trace) || []) r.items.set(s.seq, s);
			strays.delete(it.span.trace);
		}
		if (it.kind === 'report' && it.report && !state.reports.has(it.report.id)) {
			state.reports.add(it.report.id);
			const rep = it.report;
			// A server error the page already showed without its stack gets it now.
			const pending = state.errors.find((e) => e.requestId === rep.requestId && e.pending);
			const err = { source: rep.source || 'server', type: rep.type, message: rep.message, frames: rep.stack || [], route: rep.route, url: rep.url, requestId: rep.requestId, time: rep.time };
			if (pending) Object.assign(pending, err, { pending: false });
			else addError(err);
		}
		return true;
	}

	function addError(err) {
		state.errors.push(err);
		if (state.errors.length > 50) state.errors.shift();
		state.errorIndex = state.errors.length - 1;
		state.overlay = true;
		render();
	}

	function onDev(ev) {
		if (ev.kind === 'page') {
			state.pages.push(ev);
			if (state.pages.length > 20) state.pages.shift();
			const r = request(ev.requestId, 'GET ' + ev.url, 'page', ev.status);
			if (r) state.selected = r.id;
			for (const c of ev.components) {
				if (!state.files.has(c)) state.files.set(c, null);
			}
			const missing = ev.components.filter((c) => state.files.get(c) === null);
			if (missing.length) {
				api('files?' + missing.map((c) => 'c=' + encodeURIComponent(c)).join('&'))
					.then((list) => {
						for (const f of list) state.files.set(f.component, f);
						render();
					})
					.catch(() => {});
			}
			if (ev.status >= 500) {
				const load = ev.failed != null ? ev.components[ev.failed] : null;
				addError({ source: 'server', type: 'Error', message: ev.error || 'Internal Error', frames: [], route: ev.route, url: ev.url, requestId: ev.requestId, pending: true, load });
			}
		} else if (ev.kind === 'action') {
			request(ev.requestId, 'POST ' + ev.url, 'action', ev.status);
		}
		render();
	}

	// ---- browser errors ----

	function browserError(name, message, stack) {
		if (String(stack).includes('/__tern/dev/client.js')) return; // ours
		if (state.errors.some((e) => e.source === 'browser' && e.message === message && e.stackText === stack)) return;
		addError({ source: 'browser', type: name, message, stackText: stack || '', frames: parseStack(stack), url: location.pathname + location.search, route: current()?.route });
	}

	addEventListener('error', (e) => {
		if (!e.error && !e.message) return; // a resource that failed to load
		const err = e.error;
		browserError(err?.name ?? 'Error', err?.message ?? e.message, err?.stack ?? `at ${e.filename}:${e.lineno}:${e.colno}`);
	});
	addEventListener('unhandledrejection', (e) => {
		const r = e.reason;
		if (r instanceof Error) browserError(r.name, r.message, r.stack ?? '');
		else browserError('UnhandledRejection', String(r), '');
	});

	// Frames of a JavaScript stack; those served from the project's src/ are the app's.
	function parseStack(stack) {
		const out = [];
		for (const line of String(stack || '').split('\n')) {
			const m = line.match(/^\s*at (?:(.*?) \()?(.*?):(\d+):(\d+)\)?\s*$/) || line.match(/^(.*?)@(.*?):(\d+):(\d+)$/);
			if (!m) continue;
			let file = m[2];
			let inApp = false;
			try {
				const u = new URL(file, location.href);
				if (u.origin === location.origin && u.pathname.startsWith('/src/')) {
					file = decodeURIComponent(u.pathname.slice(1));
					inApp = true;
				}
			} catch {}
			out.push({ function: m[1] || '(anonymous)', file, line: +m[3], inApp, browser: true });
		}
		return out;
	}

	// ---- view helpers ----

	function h(tag, attrs, ...kids) {
		const el = document.createElement(tag);
		for (const [k, v] of Object.entries(attrs || {})) {
			if (v == null || v === false) continue;
			if (k.startsWith('on')) el.addEventListener(k.slice(2), v);
			else if (k === 'class') el.className = v;
			else el.setAttribute(k, v === true ? '' : v);
		}
		for (const k of kids.flat()) if (k != null && k !== false) el.append(k instanceof Node ? k : String(k));
		return el;
	}

	function openFile(file, line) {
		return fetch('/__tern/dev/api/open', { method: 'POST', credentials: 'same-origin', headers: { 'content-type': 'application/json' }, body: JSON.stringify({ file, line }) }).then(async (r) => {
			if (!r.ok) toast((await r.text()) || 'Couldn’t open the editor.');
		});
	}

	let toastTimer;
	function toast(msg) {
		const t = root.querySelector('.toast');
		t.textContent = msg;
		t.hidden = false;
		clearTimeout(toastTimer);
		toastTimer = setTimeout(() => (t.hidden = true), 5000);
	}

	function fileButton(label, file, line, title) {
		return h('button', { class: 'file', type: 'button', title: title || 'Open in your editor', onclick: () => openFile(file, line) }, label);
	}

	const short = (c) => c.replace(/^src\/routes\/?/, '').replace(/\/?\+(page|layout|error)\.svelte$/, '') || '/';
	const kindOf = (c) => (c.endsWith('+layout.svelte') ? 'Layout' : c.endsWith('+error.svelte') ? 'Error page' : 'Page');

	// A JSON value as nested, collapsed details.
	function tree(v, depth = 0) {
		if (v === null) return h('span', { class: 'j-null' }, 'null');
		if (typeof v === 'string') return h('span', { class: 'j-str' }, JSON.stringify(v.length > 300 ? v.slice(0, 300) + '…' : v));
		if (typeof v === 'number') return h('span', { class: 'j-num' }, String(v));
		if (typeof v === 'boolean') return h('span', { class: 'j-bool' }, String(v));
		const arr = Array.isArray(v);
		const keys = arr ? v.map((_, i) => i) : Object.keys(v);
		if (!keys.length) return h('span', { class: 'j-punct' }, arr ? '[]' : '{}');
		const d = h('details', depth < 1 ? { open: true } : {}, h('summary', null, h('span', { class: 'j-punct' }, arr ? `Array(${keys.length})` : `{${keys.length > 3 ? keys.slice(0, 3).join(', ') + ', …' : keys.join(', ')}}`)));
		const list = h('div', { class: 'j-kids' });
		for (const k of keys.slice(0, 200)) list.append(h('div', { class: 'j-row' }, h('span', { class: 'j-key' }, String(k)), ': ', tree(v[k], depth + 1)));
		if (keys.length > 200) list.append(h('div', { class: 'j-more' }, `… ${keys.length - 200} more`));
		d.append(list);
		return d;
	}

	const current = () => state.pages[state.pages.length - 1];
	const ms = (us) => (us >= 1000 ? (us / 1000).toFixed(us >= 100000 ? 0 : 1) + ' ms' : us + ' µs');

	// ---- views ----

	function pageView() {
		const p = current();
		if (!p) return h('p', { class: 'muted' }, 'The page hasn’t told the overlay about itself yet.');
		const params = Object.entries(p.params);
		const out = [
			h('dl', { class: 'facts' },
				h('dt', null, 'Route'), h('dd', null, h('code', null, p.route ?? '(none)')),
				h('dt', null, 'Address'), h('dd', null, h('code', null, p.url)),
				h('dt', null, 'Status'), h('dd', null, h('span', { class: 'status s' + String(p.status)[0] }, String(p.status)), p.error ? h('span', { class: 'muted' }, ' ' + p.error) : null),
				params.length ? [h('dt', null, 'Params'), h('dd', null, params.map(([k, v]) => h('code', { class: 'param' }, `${k}=${v}`)))] : null
			)
		];
		out.push(h('h3', null, 'Files'));
		const files = h('ul', { class: 'files' });
		for (const c of p.components) {
			const f = state.files.get(c);
			files.append(h('li', null,
				h('span', { class: 'kind' }, kindOf(c)),
				fileButton(c.replace(/^src\/routes\//, ''), c, 1),
				f?.server ? fileButton(f.server.split('/').pop() + ':' + f.line, f.server, f.line, 'Open its load in your editor') : null
			));
		}
		out.push(files);
		const flags = Object.entries(p.flags || {}).sort(([a], [b]) => a.localeCompare(b));
		if (flags.length) {
			out.push(h('h3', null, 'Feature flags'));
			out.push(h('ul', { class: 'flags' }, flags.map(([name, on]) => h('li', null, h('span', { class: 'flag ' + (on ? 'on' : 'off') }, on ? 'on' : 'off'), h('code', null, name)))));
		}
		out.push(h('h3', null, 'Data'));
		p.components.forEach((c, i) => {
			const node = p.nodes[i];
			out.push(h('div', { class: 'node' }, h('div', { class: 'node-head' }, h('span', { class: 'kind' }, kindOf(c)), h('code', null, short(c))), node == null ? h('span', { class: 'muted' }, 'No load, or no data') : tree(node)));
		});
		return out;
	}

	function requestView() {
		const ids = [...state.order].reverse();
		if (!ids.length) return h('p', { class: 'muted' }, 'No requests yet.');
		const sel = state.requests.get(state.selected) || state.requests.get(ids[0]);
		const picker = h('div', { class: 'reqs' });
		for (const id of ids) {
			const r = state.requests.get(id);
			const reqItem = [...r.items.values()].find((i) => i.kind === 'request');
			picker.append(h('button', { type: 'button', class: 'req' + (r === sel ? ' on' : ''), onclick: () => { state.selected = id; render(); } },
				h('span', { class: 'status s' + String(reqItem?.status ?? r.status)[0] }, String(reqItem?.status ?? r.status)),
				h('span', { class: 'req-label' }, r.label),
				reqItem ? h('span', { class: 'muted' }, reqItem.ms.toFixed(1) + ' ms') : null
			));
		}
		const items = [...sel.items.values()].sort((a, b) => a.seq - b.seq);
		const spans = items.filter((i) => i.span).map((i) => i.span);
		const logs = items.filter((i) => i.kind === 'log' || i.kind === 'report');
		const out = [picker];
		out.push(h('h3', null, 'Timing'));
		if (!spans.length) out.push(h('p', { class: 'muted' }, 'No spans: tracing records them when the app has it on.'));
		else {
			const t0 = Math.min(...spans.map((s) => Date.parse(s.start)));
			const end = Math.max(...spans.map((s) => Date.parse(s.start) + s.us / 1000));
			const total = Math.max(end - t0, 0.001);
			// Children under their parents.
			const kids = new Map();
			for (const s of spans) {
				const k = spans.some((p) => p.span === s.parent) ? s.parent : '';
				if (!kids.has(k)) kids.set(k, []);
				kids.get(k).push(s);
			}
			const fall = h('div', { class: 'fall' });
			const walk = (parent, depth) => {
				for (const s of (kids.get(parent) || []).sort((a, b) => Date.parse(a.start) - Date.parse(b.start))) {
					const left = ((Date.parse(s.start) - t0) / total) * 100;
					const width = Math.max((s.us / 1000 / total) * 100, 0.4);
					fall.append(h('div', { class: 'span' + (s.error ? ' err' : ''), title: s.error || '' },
						h('span', { class: 'span-name', style: `padding-left:${depth * 12}px` }, s.name),
						h('span', { class: 'span-track' }, h('span', { class: 'span-bar', style: `left:${left}%;width:${width}%` })),
						h('span', { class: 'span-ms' }, ms(s.us))
					));
					walk(s.span, depth + 1);
				}
			};
			walk('', 0);
			out.push(fall);
		}
		out.push(h('h3', null, 'Logs'));
		if (!logs.length) out.push(h('p', { class: 'muted' }, 'Nothing was logged.'));
		else {
			const list = h('div', { class: 'logs' });
			for (const l of logs) {
				const attrs = Object.entries(l.attrs || {}).filter(([k]) => k !== 'request_id');
				list.append(h('div', { class: 'log ' + (l.kind === 'report' ? 'error' : String(l.level || '').toLowerCase()) },
					h('span', { class: 'lvl' }, l.kind === 'report' ? 'ERROR' : l.level),
					h('span', { class: 'msg' }, l.msg, attrs.map(([k, v]) => h('span', { class: 'attr' }, ` ${k}=`, typeof v === 'string' ? v : JSON.stringify(v))))
				));
			}
			out.push(list);
		}
		out.push(h('p', null, h('a', { href: '/__tern/dev/logs', target: '_blank', class: 'link' }, 'All logs ↗')));
		return out;
	}

	function errorsView() {
		if (!state.errors.length) return h('p', { class: 'muted' }, 'No errors on this page.');
		return h('div', { class: 'errlist' }, [...state.errors].reverse().map((e) => {
			const i = state.errors.indexOf(e);
			return h('button', { type: 'button', class: 'erritem', onclick: () => { state.errorIndex = i; state.overlay = true; render(); } },
				h('span', { class: 'badge ' + e.source }, e.source === 'browser' ? 'Browser' : 'Server'),
				h('span', { class: 'errmsg' }, (e.type ? e.type + ': ' : '') + e.message)
			);
		}));
	}

	// The source around a frame, loaded once.
	const sources = new Map();
	function codeFrame(f, find) {
		const key = f.file + ':' + f.line + ':' + (find || '');
		const box = h('div', { class: 'code' });
		const fill = (src) => {
			box.replaceChildren(...src.lines.map((text, i) => {
				const n = src.start + i;
				return h('div', { class: 'cl' + (n === src.line ? ' hit' : '') }, h('span', { class: 'ln' }, String(n)), h('span', { class: 'ct' }, text || ' '));
			}));
		};
		const have = sources.get(key);
		if (have) {
			if (have.lines) fill(have);
			else have.then?.((s) => s && fill(s));
			return box;
		}
		const p = api(`source?file=${encodeURIComponent(f.file)}&line=${f.line}${find ? '&find=' + encodeURIComponent(find) : ''}`).then((s) => (sources.set(key, s), s)).catch(() => (sources.set(key, {}), null));
		sources.set(key, p);
		p.then((s) => s && fill(s));
		return box;
	}

	function overlayView() {
		const e = state.errors[state.errorIndex];
		if (!e) return null;
		const n = state.errors.length;
		const app = e.frames.filter((f) => f.inApp);
		const rest = e.frames.filter((f) => !f.inApp);
		const frame = (f, withCode) =>
			h('div', { class: 'frame' + (f.inApp ? ' app' : '') },
				h('div', { class: 'frame-head' },
					h('span', { class: 'fn' }, f.function),
					// A .svelte file's line in the browser is the compiled module's, not the source's.
					f.inApp ? (f.browser && f.file.endsWith('.svelte') ? fileButton(f.file.split('/').slice(-2).join('/'), f.file, 0) : fileButton(`${f.file.split('/').slice(-2).join('/')}:${f.line}`, f.file, f.line)) : h('span', { class: 'loc' }, `${f.file}:${f.line}`)
				),
				withCode && !f.browser ? codeFrame(f) : null
			);
		const stack = [];
		if (e.pending) stack.push(h('p', { class: 'muted' }, 'Waiting for the server’s error report…'));
		// An error without the app's frames, such as one from errors.New, came
		// out of a load: show that load.
		const lf = !app.length && e.load && state.files.get(e.load);
		if (lf?.server) {
			stack.push(h('div', { class: 'frame app' },
				h('div', { class: 'frame-head' }, h('span', { class: 'fn' }, `Returned by the load of ${e.load.replace(/^src\/routes\/?/, '/').replace(/\/?\+(page|layout)\.svelte$/, '') || '/'}`), fileButton(`${lf.server}:${lf.line}`, lf.server, lf.line)),
				codeFrame({ file: lf.server, line: lf.line }, e.message)
			));
		}
		app.forEach((f, i) => stack.push(frame(f, i < (e.source === 'build' ? 5 : 3))));
		if (rest.length) {
			stack.push(h('details', { class: 'more' }, h('summary', null, `${rest.length} more ${rest.length === 1 ? 'frame' : 'frames'} from ${e.source === 'browser' ? 'Svelte and other packages' : 'Go and other packages'}`), rest.map((f) => frame(f, false))));
		}
		if (!e.frames.length && e.stackText) stack.push(h('pre', { class: 'raw' }, e.stackText));
		return h('div', { class: 'backdrop', onclick: (ev) => { if (ev.target === ev.currentTarget) close(); } },
			h('div', { class: 'dialog', role: 'dialog', 'aria-modal': 'true', 'aria-labelledby': 'tern-err-title' },
				h('header', { class: 'dialog-head' },
					h('span', { class: 'badge ' + e.source }, { browser: 'Browser error', job: 'Job error', build: 'Build error' }[e.source] || 'Server error'),
					n > 1 ? h('span', { class: 'pager' },
						h('button', { type: 'button', 'aria-label': 'Previous error', disabled: state.errorIndex === 0, onclick: () => { state.errorIndex--; render(); } }, '‹'),
						`${state.errorIndex + 1} of ${n}`,
						h('button', { type: 'button', 'aria-label': 'Next error', disabled: state.errorIndex === n - 1, onclick: () => { state.errorIndex++; render(); } }, '›')
					) : null,
					h('button', { type: 'button', class: 'x', 'aria-label': 'Close', onclick: close }, '×')
				),
				h('h2', { id: 'tern-err-title' }, e.message),
				h('p', { class: 'meta' }, [/errorString|^Error$/.test(e.type || '') ? null : e.type, e.route && 'route ' + e.route, e.url, e.requestId && 'request #' + e.requestId.slice(0, 8)].filter(Boolean).join(' · ')),
				h('div', { class: 'stack' }, stack),
				h('footer', { class: 'dialog-foot' },
					h('span', { class: 'muted' }, e.source === 'build' ? 'Fix it and save: the page reloads by itself.' : 'Esc to close. Errors stay in the Tern button’s Errors tab.'),
					e.requestId ? h('button', { type: 'button', class: 'btn', onclick: () => { state.overlay = false; state.open = true; state.tab = 'request'; state.selected = e.requestId; render(); } }, 'Show the request') : null
				)
			)
		);
		function close() {
			state.overlay = false;
			render();
		}
	}

	// ---- shell ----

	const host = document.createElement('tern-dev-overlay');
	const root = host.attachShadow({ mode: 'open' });
	root.innerHTML = `<style>${CSS()}</style><div class="ui"></div><div class="toast" role="status" hidden></div>`;
	const ui = root.querySelector('.ui');

	function render() {
		const errs = state.errors.length;
		const p = current();
		const parts = [];
		if (!state.hidden || errs) {
			parts.push(h('button', { type: 'button', class: 'pill' + (errs ? ' has-errors' : '') + (state.open ? ' on' : ''), 'aria-expanded': String(state.open), 'aria-label': 'Tern dev tools', onclick: () => { state.open = !state.open; render(); } },
				h('span', { class: 'mark', 'aria-hidden': 'true' }, 'T'),
				p ? h('span', { class: 'pill-route' }, p.route ?? p.url) : null,
				errs ? h('span', { class: 'count', title: `${errs} ${errs === 1 ? 'error' : 'errors'}` }, String(errs)) : null
			));
		}
		if (state.open) {
			const tabs = [['page', 'Page'], ['request', 'Requests'], ['errors', errs ? `Errors (${errs})` : 'Errors']];
			const body = state.tab === 'page' ? pageView() : state.tab === 'request' ? requestView() : errorsView();
			parts.push(h('section', { class: 'panel', 'aria-label': 'Tern dev tools' },
				h('header', { class: 'panel-head' },
					h('nav', { class: 'tabs', role: 'tablist' }, tabs.map(([id, label]) => h('button', { type: 'button', role: 'tab', 'aria-selected': String(state.tab === id), class: 'tab', onclick: () => { state.tab = id; render(); } }, label))),
					h('button', { type: 'button', class: 'hide', title: 'Hide the button until an error happens. ternDev.show() brings it back.', onclick: () => setHidden(true) }, 'Hide'),
					h('button', { type: 'button', class: 'x', 'aria-label': 'Close', onclick: () => { state.open = false; render(); } }, '×')
				),
				h('div', { class: 'panel-body' }, body)
			));
		}
		if (state.overlay) parts.push(overlayView());
		// Keep the panel's scroll position across renders.
		const scroller = ui.querySelector('.panel-body');
		const top = scroller?.scrollTop ?? 0;
		ui.replaceChildren(...parts.filter(Boolean));
		const again = ui.querySelector('.panel-body');
		if (again) again.scrollTop = top;
	}

	function setHidden(v) {
		state.hidden = v;
		state.open = false;
		try {
			if (v) localStorage.setItem(HIDDEN, '1');
			else localStorage.removeItem(HIDDEN);
		} catch {}
		if (v) console.info('[tern] The dev tools button is hidden; run ternDev.show() to bring it back. Errors still open the overlay.');
		render();
	}
	window.ternDev = { show: () => setHidden(false), hide: () => setHidden(true) };

	addEventListener('keydown', (e) => {
		if (e.key !== 'Escape') return;
		if (state.overlay) state.overlay = false;
		else if (state.open) state.open = false;
		else return;
		render();
	});

	// tern dev's page for a build that failed: each compiler error, with its source.
	function buildProblem() {
		const text = document.querySelector('pre')?.textContent || '';
		const frames = [];
		for (const m of text.matchAll(/^(?:\.\/)?([^\s:]+\.(?:go|svelte|ts|js|json|sql)):(\d+)(?::\d+)?: (.*)$/gm)) {
			frames.push({ function: m[3], file: m[1], line: +m[2], inApp: true });
		}
		state.errors.push({ source: 'build', type: '', message: frames.length ? `The app doesn’t build: ${frames.length} ${frames.length === 1 ? 'error' : 'errors'}` : 'The app could not be built or started', frames, stackText: frames.length ? '' : text });
		state.overlay = true;
	}

	function mountUI() {
		if (document.body.hasAttribute('data-tern-problem')) buildProblem();
		document.body.append(host);
		for (const ev of window.__ternDev || []) onDev(ev);
		addEventListener('tern:dev', (e) => onDev(e.detail));
		render();
	}
	if (document.body) mountUI();
	else addEventListener('DOMContentLoaded', mountUI, { once: true });

	function CSS() {
		return `
:host { all: initial; }
* { box-sizing: border-box; }
.ui { --bg: #0d1214; --bg2: #141b1e; --line: #243034; --fg: #e3ebed; --muted: #8a9a9f; --accent: #5ec4b6; --danger: #f87171; --warn: #fbbf24; --ok: #4ade80;
	font: 13px/1.45 ui-sans-serif, system-ui, -apple-system, 'Segoe UI', sans-serif; color: var(--fg); }
code, .code, .loc, .raw, .j-row, .span-name, .log { font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace; font-size: 12px; }
button { font: inherit; color: inherit; cursor: pointer; }
.pill { position: fixed; left: 12px; bottom: 12px; z-index: 2147483000; display: flex; align-items: center; gap: 8px; height: 32px; max-width: min(360px, calc(100vw - 24px));
	padding: 0 10px 0 4px; border-radius: 999px; border: 1px solid var(--line); background: rgba(13, 18, 20, 0.92); backdrop-filter: blur(8px); box-shadow: 0 4px 16px rgba(0,0,0,.35); }
.pill:hover, .pill.on { border-color: #3a4a4f; }
.pill:focus-visible, .tab:focus-visible, .file:focus-visible, .btn:focus-visible, .x:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
.mark { display: grid; place-items: center; width: 24px; height: 24px; border-radius: 50%; background: var(--accent); color: #062421; font-weight: 800; font-size: 13px; }
.pill-route { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--muted); font-family: ui-monospace, Menlo, monospace; font-size: 12px; }
.count { min-width: 20px; height: 20px; padding: 0 6px; border-radius: 999px; background: var(--danger); color: #2a0b0b; font-weight: 700; font-size: 11px; display: grid; place-items: center; }
.pill.has-errors { border-color: rgba(248,113,113,.5); }
.panel { position: fixed; left: 12px; bottom: 52px; z-index: 2147483000; width: min(760px, calc(100vw - 24px)); height: min(70vh, 620px); display: flex; flex-direction: column;
	background: var(--bg); border: 1px solid var(--line); border-radius: 12px; box-shadow: 0 16px 48px rgba(0,0,0,.5); overflow: hidden; }
.panel-head { display: flex; align-items: center; gap: 4px; padding: 6px 6px 0 8px; border-bottom: 1px solid var(--line); background: var(--bg2); }
.tabs { display: flex; gap: 2px; margin-right: auto; overflow-x: auto; }
.tab { padding: 8px 10px; border: 0; background: none; color: var(--muted); border-bottom: 2px solid transparent; white-space: nowrap; }
.tab[aria-selected="true"] { color: var(--fg); border-bottom-color: var(--accent); }
.hide { border: 0; background: none; color: var(--muted); font-size: 12px; padding: 6px 8px; }
.hide:hover { color: var(--fg); }
.x { width: 28px; height: 28px; border: 0; border-radius: 6px; background: none; color: var(--muted); font-size: 18px; line-height: 1; }
.x:hover { background: #1d272a; color: var(--fg); }
.panel-body { flex: 1; overflow: auto; padding: 12px 14px 16px; }
h3 { margin: 18px 0 8px; font-size: 11px; text-transform: uppercase; letter-spacing: .06em; color: var(--muted); font-weight: 600; }
.muted { color: var(--muted); }
p { margin: 0 0 8px; }
.facts { display: grid; grid-template-columns: max-content 1fr; gap: 6px 16px; margin: 0; }
.facts dt { color: var(--muted); }
.facts dd { margin: 0; min-width: 0; overflow-wrap: anywhere; }
.param { margin-right: 8px; }
.status { display: inline-block; padding: 0 6px; border-radius: 4px; font-family: ui-monospace, Menlo, monospace; font-size: 11px; font-weight: 600; background: #1d272a; }
.s2 { color: var(--ok); } .s3 { color: var(--accent); } .s4 { color: var(--warn); } .s5 { color: var(--danger); }
.files { list-style: none; margin: 0; padding: 0; display: grid; gap: 4px; }
.files li { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.kind { display: inline-block; min-width: 72px; color: var(--muted); font-size: 12px; }
.file { border: 1px solid var(--line); background: var(--bg2); border-radius: 6px; padding: 2px 8px; font-family: ui-monospace, Menlo, monospace; font-size: 12px; color: var(--accent); }
.file:hover { border-color: var(--accent); }
.flags { list-style: none; margin: 0; padding: 0; display: flex; flex-wrap: wrap; gap: 6px 14px; }
.flags li { display: flex; align-items: center; gap: 6px; }
.flag { padding: 0 6px; border-radius: 4px; font-size: 11px; font-weight: 600; background: #1d272a; }
.flag.on { color: var(--ok); } .flag.off { color: var(--muted); }
.node { border: 1px solid var(--line); border-radius: 8px; padding: 8px 10px; margin-bottom: 8px; background: var(--bg2); }
.node-head { display: flex; gap: 8px; align-items: baseline; margin-bottom: 4px; }
details > summary { cursor: pointer; list-style: none; }
details > summary::before { content: '▸ '; color: var(--muted); }
details[open] > summary::before { content: '▾ '; }
.j-row > details { display: inline-block; vertical-align: top; }
.j-kids { padding-left: 16px; border-left: 1px solid var(--line); margin-left: 4px; }
.j-key { color: #9cc9ff; } .j-str { color: #a5e3b4; } .j-num { color: #f9c97c; } .j-bool, .j-null { color: #d2a8ff; } .j-punct, .j-more { color: var(--muted); }
.reqs { display: grid; gap: 2px; max-height: 160px; overflow: auto; border: 1px solid var(--line); border-radius: 8px; padding: 4px; }
.req { display: flex; gap: 8px; align-items: center; padding: 4px 8px; border: 0; border-radius: 6px; background: none; text-align: left; }
.req.on { background: #1a2427; }
.req:hover { background: #172023; }
.req-label { flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-family: ui-monospace, Menlo, monospace; font-size: 12px; }
.fall { display: grid; gap: 2px; }
.span { display: grid; grid-template-columns: minmax(120px, 36%) 1fr 64px; gap: 8px; align-items: center; }
.span-name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.span-track { position: relative; height: 14px; background: #111819; border-radius: 3px; }
.span-bar { position: absolute; top: 2px; bottom: 2px; border-radius: 2px; background: var(--accent); }
.span.err .span-bar { background: var(--danger); }
.span-ms { text-align: right; color: var(--muted); font-size: 11px; }
.logs { display: grid; gap: 2px; }
.log { display: grid; grid-template-columns: 48px 1fr; gap: 8px; padding: 2px 0; }
.lvl { color: var(--muted); font-size: 11px; }
.log.warn .lvl { color: var(--warn); } .log.error .lvl { color: var(--danger); }
.msg { overflow-wrap: anywhere; } .attr { color: var(--muted); }
.link { color: var(--accent); }
.errlist { display: grid; gap: 4px; }
.erritem { display: flex; gap: 8px; align-items: center; padding: 8px 10px; border: 1px solid var(--line); border-radius: 8px; background: var(--bg2); text-align: left; }
.erritem:hover { border-color: #3a4a4f; }
.errmsg { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.badge { flex: none; padding: 1px 8px; border-radius: 999px; font-size: 11px; font-weight: 600; background: rgba(248,113,113,.15); color: var(--danger); }
.badge.browser { background: rgba(251,191,36,.14); color: var(--warn); }
.backdrop { position: fixed; inset: 0; z-index: 2147483001; background: rgba(5, 8, 9, .62); backdrop-filter: blur(3px); display: grid; place-items: start center; padding: 6vh 16px 16px; overflow: auto; }
.dialog { width: min(960px, 100%); background: var(--bg); border: 1px solid var(--line); border-top: 3px solid var(--danger); border-radius: 12px; box-shadow: 0 24px 64px rgba(0,0,0,.6); padding: 16px 20px 14px; }
.dialog-head { display: flex; align-items: center; gap: 12px; }
.dialog-head .x { margin-left: auto; }
.pager { display: flex; align-items: center; gap: 6px; color: var(--muted); font-size: 12px; }
.pager button { width: 24px; height: 24px; border: 1px solid var(--line); border-radius: 6px; background: var(--bg2); }
.pager button:disabled { opacity: .4; cursor: default; }
.dialog h2 { margin: 14px 0 4px; font-size: 17px; line-height: 1.4; color: #ffd5d5; font-weight: 600; overflow-wrap: anywhere; white-space: pre-wrap; }
.meta { color: var(--muted); font-size: 12px; overflow-wrap: anywhere; }
.stack { display: grid; gap: 8px; margin-top: 12px; }
.frame { border: 1px solid var(--line); border-radius: 8px; overflow: hidden; }
.frame.app { border-color: #34474c; }
.frame-head { display: flex; justify-content: space-between; align-items: center; gap: 12px; padding: 6px 10px; background: var(--bg2); flex-wrap: wrap; }
.fn { font-family: ui-monospace, Menlo, monospace; font-size: 12px; overflow-wrap: anywhere; }
.loc { color: var(--muted); overflow-wrap: anywhere; }
.code { background: #090d0e; padding: 6px 0; overflow-x: auto; }
.cl { display: grid; grid-template-columns: 48px 1fr; white-space: pre; }
.cl.hit { background: rgba(248,113,113,.14); }
.cl.hit .ln { color: var(--danger); }
.ln { text-align: right; padding-right: 12px; color: #4d5c60; user-select: none; }
.more > summary { color: var(--muted); padding: 4px 0; }
.more .frame { margin-top: 6px; }
.raw { white-space: pre-wrap; background: #090d0e; padding: 10px; border-radius: 8px; margin: 0; color: #c8d3d6; }
.dialog-foot { display: flex; justify-content: space-between; align-items: center; gap: 12px; margin-top: 14px; flex-wrap: wrap; font-size: 12px; }
.btn { border: 1px solid var(--line); background: var(--bg2); border-radius: 6px; padding: 5px 10px; }
.btn:hover { border-color: var(--accent); }
.toast { position: fixed; left: 50%; bottom: 16px; transform: translateX(-50%); z-index: 2147483002; max-width: calc(100vw - 32px); padding: 8px 14px; border-radius: 8px; background: #1d272a; color: #e3ebed; border: 1px solid #2f3d41;
	font: 13px/1.4 ui-sans-serif, system-ui, sans-serif; box-shadow: 0 8px 24px rgba(0,0,0,.4); }
.toast[hidden] { display: none; }
@media (max-width: 640px) {
	.panel { left: 0; right: 0; bottom: 0; width: 100%; height: 75vh; border-radius: 12px 12px 0 0; }
	.pill.on { display: none; }
	.span { grid-template-columns: 40% 1fr 56px; }
	.dialog { padding: 14px; }
}
`;
	}
})();

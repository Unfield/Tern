package main

// devLogsPage is the log view tern dev serves at /__tern/dev/logs.
const devLogsPage = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Logs · tern dev</title>
<style>
:root {
	--bg: #fafaf9; --panel: #ffffff; --line: #e7e5e4; --text: #1c1917; --muted: #78716c;
	--debug: #a8a29e; --info: #0369a1; --warn: #b45309; --error: #b91c1c; --ok: #15803d; --event: #6d28d9;
	--hover: #f5f5f4; --chip: #e7e5e4;
}
@media (prefers-color-scheme: dark) {
	:root {
		--bg: #0c0a09; --panel: #1c1917; --line: #292524; --text: #f5f5f4; --muted: #a8a29e;
		--debug: #78716c; --info: #7dd3fc; --warn: #fcd34d; --error: #fca5a5; --ok: #86efac; --event: #c4b5fd;
		--hover: #232120; --chip: #292524;
	}
}
* { box-sizing: border-box; }
body { margin: 0; background: var(--bg); color: var(--text); font: 14px/1.45 system-ui, sans-serif; }
header { position: sticky; top: 0; z-index: 1; display: flex; flex-wrap: wrap; gap: 8px; align-items: center; padding: 10px 16px; background: var(--panel); border-bottom: 1px solid var(--line); }
header h1 { font-size: 15px; margin: 0 8px 0 0; }
header h1 span { color: var(--muted); font-weight: 400; }
input[type=search], select, button { font: inherit; color: inherit; background: var(--bg); border: 1px solid var(--line); border-radius: 6px; padding: 4px 8px; }
input[type=search] { flex: 1 1 200px; min-width: 0; }
button { cursor: pointer; }
button[aria-pressed=true] { background: var(--chip); }
label { display: inline-flex; gap: 4px; align-items: center; color: var(--muted); }
.chip { display: none; align-items: center; gap: 4px; background: var(--chip); border-radius: 999px; padding: 2px 4px 2px 10px; font: 12px ui-monospace, monospace; }
.chip.on { display: inline-flex; }
.chip button { border: 0; background: none; padding: 0 6px; }
#status { margin-left: auto; color: var(--muted); font-size: 12px; }
main { font: 12.5px/1.5 ui-monospace, SFMono-Regular, Menlo, monospace; }
.row { display: grid; grid-template-columns: 92px 56px 1fr; gap: 10px; padding: 3px 16px; border-bottom: 1px solid var(--line); cursor: default; }
.row:hover { background: var(--hover); }
.time { color: var(--muted); }
.lvl { font-weight: 600; }
.DEBUG .lvl { color: var(--debug); } .INFO .lvl { color: var(--info); } .WARN .lvl { color: var(--warn); } .ERROR .lvl { color: var(--error); }
.ERROR { background: color-mix(in srgb, var(--error) 7%, transparent); }
.event .lvl, .event .msg { color: var(--event); }
.msg { min-width: 0; overflow-wrap: anywhere; }
.attr { color: var(--muted); margin-left: 8px; }
.attr b { font-weight: 400; color: var(--text); }
.rid { cursor: pointer; text-decoration: underline dotted; }
.s2 { color: var(--ok); } .s3 { color: var(--info); } .s4 { color: var(--warn); } .s5 { color: var(--error); }
.ms { color: var(--muted); margin-left: 8px; }
pre.detail { grid-column: 3; margin: 4px 0 6px; padding: 8px 10px; background: var(--bg); border: 1px solid var(--line); border-radius: 6px; white-space: pre-wrap; overflow-wrap: anywhere; }
#empty { padding: 48px 16px; text-align: center; color: var(--muted); font: 14px/1.5 system-ui, sans-serif; }
@media (max-width: 640px) { .row { grid-template-columns: 64px 44px 1fr; gap: 6px; padding: 3px 10px; } .time { font-size: 11px; } }
</style>
</head>
<body>
<header>
	<h1>Logs <span>tern dev</span></h1>
	<input type="search" id="q" placeholder="Filter messages, attributes, paths" aria-label="Filter">
	<select id="level" aria-label="Lowest level">
		<option value="0">All levels</option>
		<option value="1" selected>Info and up</option>
		<option value="2">Warnings and errors</option>
		<option value="3">Errors</option>
	</select>
	<label><input type="checkbox" id="requests" checked> Requests</label>
	<label><input type="checkbox" id="assets"> Assets</label>
	<span class="chip" id="chip">request <span id="chipid"></span><button id="unchip" aria-label="Show all requests">×</button></span>
	<button id="pause" aria-pressed="false">Pause</button>
	<button id="clear">Clear</button>
	<span id="status">connecting…</span>
</header>
<main id="rows"></main>
<div id="empty">No records yet. Log with the app's logger, or with slog and a logger from logging.New, and they show up here.</div>
<script>
const rows = document.getElementById('rows'), empty = document.getElementById('empty'), status = document.getElementById('status');
const q = document.getElementById('q'), level = document.getElementById('level'), showReq = document.getElementById('requests'), showAssets = document.getElementById('assets');
const pauseBtn = document.getElementById('pause'), chip = document.getElementById('chip'), chipId = document.getElementById('chipid');
const items = []; let paused = false, held = [], onlyRequest = '';
const levels = { DEBUG: 0, INFO: 1, WARN: 2, ERROR: 3 };
function rank(l) { const base = String(l || 'INFO').replace(/[+-]\d+$/, ''); return levels[base] ?? 1; }
function isAsset(p) {
	p = (p || '').split('?')[0];
	if (/^\/(@|node_modules\/|src\/|\.tern\/|__vite|_app\/|__tern\/)/.test(p)) return true;
	const last = p.slice(p.lastIndexOf('/') + 1);
	return last.includes('.') && !last.endsWith('.json');
}
function text(v) { return typeof v === 'string' ? v : JSON.stringify(v); }
function flat(obj, prefix, out) {
	for (const [k, v] of Object.entries(obj || {})) {
		const key = prefix ? prefix + '.' + k : k;
		if (v && typeof v === 'object' && !Array.isArray(v)) flat(v, key, out); else out.push([key, v]);
	}
	return out;
}
function matches(it) {
	if (it.kind === 'request' && (!showReq.checked || (!showAssets.checked && isAsset(it.path)))) return false;
	if (it.kind === 'log' && rank(it.level) < Number(level.value)) return false;
	if (onlyRequest && it.request_id !== onlyRequest) return false;
	const s = q.value.trim().toLowerCase();
	if (s && !(it.msg + ' ' + JSON.stringify(it.attrs || {})).toLowerCase().includes(s)) return false;
	return true;
}
function el(tag, cls, txt) { const e = document.createElement(tag); if (cls) e.className = cls; if (txt != null) e.textContent = txt; return e; }
function render(it) {
	const row = el('div', 'row ' + (it.kind === 'log' ? String(it.level).replace(/[+-]\d+$/, '') : it.kind));
	const t = new Date(it.time);
	row.append(el('span', 'time', t.toLocaleTimeString([], { hour12: false }) + '.' + String(t.getMilliseconds()).padStart(3, '0')));
	const msg = el('span', 'msg');
	if (it.kind === 'request') {
		row.append(el('span', 'lvl s' + String(it.status)[0], String(it.status)));
		msg.append(el('span', null, it.method + ' ' + it.path), el('span', 'ms', it.ms.toFixed(1) + ' ms'));
	} else {
		row.append(el('span', 'lvl', it.kind === 'event' ? 'tern' : it.level));
		msg.append(el('span', null, it.msg));
		for (const [k, v] of flat(it.attrs, '', [])) {
			if (k === 'request_id') continue;
			const a = el('span', 'attr', k + '=');
			a.append(el('b', null, text(v)));
			msg.append(a);
		}
	}
	if (it.request_id) {
		const r = el('span', 'attr rid', '#' + it.request_id.slice(0, 8));
		r.title = 'Show only request ' + it.request_id;
		r.onclick = (e) => { e.stopPropagation(); filterRequest(it.request_id); };
		msg.append(r);
	}
	row.append(msg);
	row.onclick = () => {
		const open = row.querySelector('pre.detail');
		if (open) { open.remove(); return; }
		const d = Object.assign({}, it); delete d.seq;
		row.append(el('pre', 'detail', JSON.stringify(d, null, 2)));
	};
	return row;
}
function atBottom() { return window.innerHeight + window.scrollY >= document.body.scrollHeight - 40; }
function append(list) {
	const stick = atBottom();
	const frag = document.createDocumentFragment();
	for (const it of list) { items.push(it); if (matches(it)) frag.append(render(it)); }
	while (items.length > 5000) items.shift();
	rows.append(frag);
	while (rows.childElementCount > 5000) rows.firstElementChild.remove();
	empty.style.display = items.length ? 'none' : '';
	if (stick) window.scrollTo(0, document.body.scrollHeight);
}
function redraw() {
	rows.replaceChildren(...items.filter(matches).map(render));
	empty.style.display = items.length ? 'none' : '';
	window.scrollTo(0, document.body.scrollHeight);
}
function filterRequest(id) { onlyRequest = id; chipId.textContent = id; chip.classList.toggle('on', !!id); redraw(); }
document.getElementById('unchip').onclick = () => filterRequest('');
for (const c of [q, level, showReq, showAssets]) c.addEventListener('input', redraw);
pauseBtn.onclick = () => {
	paused = !paused; pauseBtn.setAttribute('aria-pressed', paused); pauseBtn.textContent = paused ? 'Resume' : 'Pause';
	if (!paused) { append(held); held = []; }
};
document.getElementById('clear').onclick = async () => {
	await fetch('/__tern/dev/logs/clear', { method: 'POST' });
	items.length = 0; held = []; redraw();
};
const es = new EventSource('/__tern/dev/logs/stream');
es.onopen = () => { status.textContent = 'live'; };
es.onerror = () => { status.textContent = 'reconnecting…'; };
es.onmessage = (e) => { const list = JSON.parse(e.data); if (paused) held.push(...list); else append(list); };
</script>
</body>
</html>
`

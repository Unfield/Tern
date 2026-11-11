import { hydrate, mount, tick, type Component } from 'svelte';
import DefaultError from './DefaultError.svelte';
import Passthrough from './Passthrough.svelte';
import Root from './Root.svelte';
import { DeferredScope, readStreamed, revive, type Chunk } from './deferred.js';
import { setBase } from './base.js';
import { configureI18n, useLocale } from './i18n.svelte.js';
import { runAfter, runBefore, runOn } from './navigation.js';
import { plan } from './select.js';
import { applyPayloadToPage, page, setNavigating } from './state.svelte.js';
import type {
	ActionResult,
	ClientManifest,
	GotoOptions,
	Navigation,
	NavigationTarget,
	NavigationType,
	NodeUses,
	Payload
} from './types.js';

const MAX_REDIRECTS = 20;
const PRELOAD_TTL = 10_000;
const SCROLL_KEY = 'tern:scroll';

let manifest: ClientManifest;
let started = false;
let token = 0;
let historyIndex = 0;
let nodes: (Record<string, unknown> | null)[] = [];
let layoutCount = 0;
let scrollPositions: Record<number, { x: number; y: number }> = {};
let announcer: HTMLElement | null = null;

const props = $state<{ components: Component<any>[]; data: Record<string, unknown>[]; form: unknown }>({
	components: [],
	data: [],
	form: null
});

const preloads = new Map<string, { at: number; promise: Promise<Payload | null> }>();

/** Layout data the client holds, by layout index, for reuse on navigation. */
interface Held {
	data: Record<string, unknown> | null;
	uses: NodeUses | null;
	params: Record<string, string>;
	url: string;
}
const held = new Map<number, Held>();
const invalidated = new Set<string>();
let ignoreNextPop = false;

/** Marks invalidate() reloads, which are not navigations: no hooks, no navigating state. */
const RELOAD = Symbol('reload');
type NavigateOptions = GotoOptions & { [RELOAD]?: boolean };

/** Starts the client. Called by the generated entry module. */
export async function start(m: ClientManifest): Promise<void> {
	manifest = m;
	setBase(m.base ?? '');
	configureI18n(m.i18n ?? null);
	const el = document.getElementById('__tern_data');
	const target = document.getElementById('tern-root');
	if (!el || !target) {
		throw new Error('Tern: the page is missing its data or mount point; check that src/app.html contains %tern.head% and %tern.body%');
	}

	try {
		scrollPositions = JSON.parse(sessionStorage.getItem(SCROLL_KEY) ?? '{}');
	} catch {
		scrollPositions = {};
	}

	historyIndex = history.state?.ternIndex ?? 0;
	if (history.state?.ternIndex == null) {
		history.replaceState({ ...history.state, ternIndex: historyIndex, ternState: {} }, '');
	}
	history.scrollRestoration = 'manual';

	const url = new URL(location.href);
	const payload = JSON.parse(el.textContent ?? '{}') as Payload;

	// Deferred values stream in as inline scripts calling __tern_resolve.
	const w = window as unknown as { __tern_d?: Record<number, Chunk>; __tern_on?: (c: Chunk) => void };
	const scope = new DeferredScope();
	if (payload.deferred) payload.nodes = revive(payload.nodes, (id) => scope.promise(id));
	for (const chunk of Object.values(w.__tern_d ?? {})) scope.settle(chunk);
	w.__tern_on = (chunk) => scope.settle(chunk);
	await render(payload, url, history.state?.ternState ?? {}, token);
	if (target.hasAttribute('data-tern-ssr')) {
		hydrate(Root, { target, props });
	} else {
		mount(Root, { target, props });
	}
	started = true;
	await tick();
	runAfter({ from: null, to: current(), type: 'enter' });
	registerServiceWorker();

	const saved = scrollPositions[historyIndex];
	if (saved) {
		scrollTo(saved.x, saved.y);
	} else if (url.hash) {
		scrollToHash(url.hash);
	}
	listen();
}

function registerServiceWorker() {
	// Not in development: a caching worker would serve stale modules.
	if (!manifest.serviceWorker || import.meta.env?.DEV || !('serviceWorker' in navigator)) return;
	const base = manifest.base ?? '';
	navigator.serviceWorker.register(`${base}/service-worker.js`, { scope: `${base}/` }).catch((err) => {
		console.error('Tern: registering the service worker failed', err);
	});
}

/** Navigates to href, which may be relative to the current URL. */
export async function goto(href: string | URL, opts: GotoOptions = {}): Promise<void> {
	if (!started) {
		throw new Error('Tern: goto() was called before the app started');
	}
	const url = new URL(href, location.href);
	if (!inApp(url)) {
		location.href = url.href;
		return;
	}
	await navigate(url, opts, 'goto', 0);
}

/**
 * Reruns the loads that depend on key, as declared with ctx.Depends(key)
 * in Go, plus the current page's load.
 */
export function invalidate(key: string): Promise<void> {
	invalidated.add(key);
	preloads.clear();
	return navigate(new URL(location.href), {
		replaceState: true,
		noScroll: true,
		keepFocus: true,
		state: page.state,
		[RELOAD]: true
	}, 'goto', 0);
}

/** Reruns every load for the current page. */
export function invalidateAll(): Promise<void> {
	return navigate(new URL(location.href), {
		replaceState: true,
		noScroll: true,
		keepFocus: true,
		invalidateAll: true,
		state: page.state,
		[RELOAD]: true
	}, 'goto', 0);
}

/**
 * Loads the data and code for a URL so a later navigation to it is instant.
 * Tern does this automatically for links; see data-tern-preload.
 */
export function preloadData(href: string | URL): Promise<Payload | null> {
	const url = new URL(href, location.href);
	const key = url.pathname + url.search;
	const hit = preloads.get(key);
	if (hit && Date.now() - hit.at < PRELOAD_TTL) {
		return hit.promise;
	}
	const promise = fetchPayload(url).then(async (p) => {
		if (p?.type === 'data' && p.route) {
			await loadRoute(p.route).catch(() => {});
		}
		return p;
	});
	preloads.set(key, { at: Date.now(), promise });
	return promise;
}

/** Applies a form action result to the page, as `use:enhance` does. */
export async function applyAction(result: ActionResult): Promise<void> {
	switch (result.type) {
		case 'success':
		case 'failure':
			page.status = result.status;
			setForm(result.data ?? null);
			return;
		case 'redirect':
			await goto(result.location, { invalidateAll: true });
			return;
		case 'error':
			await render(
				{
					type: 'error',
					route: page.route.id ?? undefined,
					params: page.params,
					status: result.status,
					error: result.error,
					nodes: nodes.slice(0, layoutCount),
					failed: layoutCount
				},
				page.url,
				page.state,
				token
			);
	}
}

function setForm(form: unknown) {
	page.form = form;
	props.form = form;
}

function dataURL(url: URL, reuse: boolean): string {
	const path = url.pathname.replace(/\/$/, '');
	// The base path is part of the pathname; the server strips it.
	let target = `${path}/__data.json${url.search}`;
	if (reuse) {
		const here = url.pathname + url.search;
		const entries: { i: number; p: Record<string, string> }[] = [];
		for (const [i, h] of held) {
			if (!h.uses) continue; // no load: nothing to reuse
			if (h.uses.url && h.url !== here) continue;
			if (h.uses.deps?.some((d) => invalidated.has(d))) continue;
			const p: Record<string, string> = {};
			for (const name of h.uses.params ?? []) p[name] = h.params[name] ?? '';
			entries.push({ i, p });
		}
		if (entries.length) {
			target += (url.search ? '&' : '?') + 'x-tern-reuse=' + encodeURIComponent(JSON.stringify(entries));
		}
	}
	return target;
}

async function fetchPayload(url: URL, reuse = true): Promise<Payload | null> {
	try {
		const res = await fetch(dataURL(url, reuse), {
			headers: { accept: 'application/json' },
			credentials: 'same-origin'
		});
		if (!res.ok || !res.headers.get('content-type')?.includes('application/json')) {
			return null;
		}
		return await readStreamed<Payload>(res);
	} catch {
		return null;
	}
}

function takePreload(url: URL): Promise<Payload | null> | undefined {
	const key = url.pathname + url.search;
	const hit = preloads.get(key);
	preloads.delete(key);
	if (hit && Date.now() - hit.at < PRELOAD_TTL) {
		return hit.promise;
	}
	return undefined;
}

async function navigate(
	url: URL,
	opts: NavigateOptions,
	type: NavigationType,
	redirects: number,
	popIndex?: number,
	fromIndex?: number
): Promise<void> {
	const id = ++token;

	// A change of hash alone is a scroll, not a navigation.
	if (type !== 'popstate' && !opts.invalidateAll && url.hash && samePage(url)) {
		saveScroll();
		history.pushState({ ternIndex: (historyIndex = historyIndex + 1), ternState: $state.snapshot(opts.state ?? {}) }, '', url);
		scrollToHash(url.hash);
		return;
	}

	const from = current();
	const nav: Navigation = { from, to: { url, params: null, route: { id: null } }, type };
	const reload = opts[RELOAD] === true;
	if (!reload && !runBefore(nav, false)) {
		if (type === 'popstate' && popIndex != null && fromIndex != null) {
			// The browser already moved; move back to where we were.
			ignoreNextPop = true;
			historyIndex = fromIndex;
			history.go(fromIndex - popIndex);
		}
		return;
	}
	if (!reload) setNavigating(nav);

	const payload = await (opts.invalidateAll ? undefined : takePreload(url)) ?? (await fetchPayload(url, !opts.invalidateAll));
	if (id !== token) return;
	if (!payload) {
		location.href = url.href; // not a page, or the server is unreachable
		return;
	}

	if (payload.type === 'redirect' && payload.location) {
		const next = new URL(payload.location, url);
		if (redirects >= MAX_REDIRECTS || !inApp(next)) {
			location.href = next.href;
			return;
		}
		return navigate(next, { ...opts, replaceState: true, [RELOAD]: false }, type, redirects + 1);
	}

	if (type !== 'popstate') saveScroll(); // popstate saved before historyIndex changed
	const state = opts.state ?? (type === 'popstate' ? history.state?.ternState ?? {} : {});

	nav.to = { url, params: payload.params ?? {}, route: { id: payload.route ?? null } };
	let finish!: () => void;
	const complete = new Promise<void>((resolve) => (finish = resolve));
	const cleanups = reload ? [] : await runOn(nav, complete);
	if (id !== token) return;

	const ok = await render(payload, url, state, id);
	if (id !== token) return;
	if (!ok) {
		location.href = url.href;
		return;
	}

	if (type !== 'popstate') {
		// History entries must be structured-cloneable: no state proxies.
		const entry = { ternIndex: historyIndex, ternState: $state.snapshot(state) };
		if (opts.replaceState) {
			history.replaceState(entry, '', url);
		} else {
			entry.ternIndex = historyIndex = historyIndex + 1;
			history.pushState(entry, '', url);
		}
	}

	setNavigating(null);
	await tick();

	if (type === 'popstate') {
		const pos = popIndex != null ? scrollPositions[popIndex] : undefined;
		scrollTo(pos?.x ?? 0, pos?.y ?? 0);
	} else if (!opts.noScroll) {
		if (url.hash) scrollToHash(url.hash);
		else scrollTo(0, 0);
	}
	if (!opts.keepFocus) {
		resetFocus();
	}
	invalidated.clear();
	finish();
	if (!reload) runAfter(nav);
	for (const fn of cleanups) fn();
}

function samePage(url: URL): boolean {
	return url.pathname === location.pathname && url.search === location.search;
}

function current(): NavigationTarget | null {
	if (!started) return null;
	return { url: page.url, params: page.params, route: { id: page.route.id } };
}

async function loadLayout(i: number): Promise<Component<any>> {
	const entry = manifest.layouts[i];
	if (!entry?.component) return Passthrough;
	return (await entry.component()).default;
}

function loadRoute(id: string): Promise<Component<any>[]> {
	const route = manifest.routes[id];
	if (!route) return Promise.reject(new Error(`unknown route ${id}`));
	return Promise.all([...route.layouts.map(loadLayout), route.page().then((m) => m.default)]);
}

/** Renders a payload. It reports false if the payload is not a page. */
async function render(p: Payload, url: URL, state: Record<string, unknown>, id: number): Promise<boolean> {
	const route = p.route ? manifest.routes[p.route] : undefined;
	const layouts = route ? route.layouts : manifest.fallback.layouts;
	// Fill in the layout data the server skipped because we already have it.
	if (p.reused?.length) {
		p.nodes = [...(p.nodes ?? [])];
		for (const k of p.reused) {
			const li = layouts[k];
			p.nodes[k] = li === undefined ? null : (held.get(li)?.data ?? null);
		}
	}
	const what = plan(manifest, p);
	const ready = useLocale(p.locale); // messages load alongside the components
	let components: Component<any>[];
	switch (what.kind) {
		case 'not-a-page':
			return false;
		case 'page':
			components = await loadRoute(what.route);
			layoutCount = what.layouts.length;
			break;
		case 'error': {
			const errors = what.route ? manifest.routes[what.route]!.errors : manifest.fallback.errors;
			components = [
				...(await Promise.all(what.layouts.map(loadLayout))),
				(await errors[what.errorIndex]!.component()).default
			];
			layoutCount = what.layouts.length;
			break;
		}
		case 'default-error':
			components = [DefaultError];
			layoutCount = 0;
	}
	nodes = p.nodes ?? [];
	await ready;

	if (id !== token) return true; // superseded; the newer navigation renders

	if (p.type === 'data') {
		const here = url.pathname + url.search;
		layouts.forEach((li, k) => {
			if (p.reused?.includes(k)) return;
			held.set(li, { data: p.nodes?.[k] ?? null, uses: p.uses?.[k] ?? null, params: p.params ?? {}, url: here });
		});
	}

	applyPayloadToPage(p, url, state, what.data);
	props.components = components;
	props.data = what.data;
	props.form = page.form;
	return true;
}

function saveScroll() {
	scrollPositions[historyIndex] = { x: scrollX, y: scrollY };
	try {
		sessionStorage.setItem(SCROLL_KEY, JSON.stringify(scrollPositions));
	} catch {
		// Storage can be full or disabled; scroll restoration is best effort.
	}
}

function scrollToHash(hash: string) {
	const id = decodeURIComponent(hash.slice(1));
	const el = document.getElementById(id) ?? document.getElementsByName(id)[0];
	if (el) el.scrollIntoView();
	else scrollTo(0, 0);
}

/** Moves focus to the document and announces the new page to screen readers. */
function resetFocus() {
	const body = document.body;
	const tabindex = body.getAttribute('tabindex');
	body.tabIndex = -1;
	body.focus({ preventScroll: true });
	if (tabindex === null) body.removeAttribute('tabindex');
	else body.setAttribute('tabindex', tabindex);

	if (!announcer) {
		announcer = document.createElement('div');
		announcer.setAttribute('aria-live', 'assertive');
		announcer.setAttribute('aria-atomic', 'true');
		announcer.style.cssText =
			'position:absolute;left:0;top:0;clip:rect(0 0 0 0);clip-path:inset(50%);overflow:hidden;white-space:nowrap;width:1px;height:1px';
		document.body.append(announcer);
	}
	announcer.textContent = document.title || page.url.pathname;
}

function findAnchor(event: Event): HTMLAnchorElement | SVGAElement | null {
	for (const el of event.composedPath()) {
		if (el instanceof HTMLAnchorElement || el instanceof SVGAElement) return el;
	}
	return null;
}

/** Reports whether url belongs to this app (its origin and base path). */
function inApp(url: URL): boolean {
	if (url.origin !== location.origin) return false;
	const base = manifest.base ?? '';
	return !base || url.pathname === base || url.pathname.startsWith(base + '/');
}

function anchorURL(a: HTMLAnchorElement | SVGAElement): URL | null {
	const href = a instanceof SVGAElement ? a.href.baseVal : a.href;
	if (!href) return null;
	const url = new URL(href, document.baseURI);
	if (!inApp(url) || !/^https?:$/.test(url.protocol)) return null;
	const target = a instanceof SVGAElement ? a.target.baseVal : a.target;
	if (target && target !== '_self') return null;
	if (a.hasAttribute('download') || (a.getAttribute('rel') ?? '').split(/\s+/).includes('external')) return null;
	if (a.closest('[data-tern-reload]:not([data-tern-reload="off"])')) return null;
	return url;
}

function preloadMode(a: Element): string {
	return a.closest('[data-tern-preload]')?.getAttribute('data-tern-preload') ?? 'tap';
}

function listen() {
	document.addEventListener('click', (event) => {
		if (event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) {
			return;
		}
		const a = findAnchor(event);
		const url = a && anchorURL(a);
		if (!a || !url) return;
		event.preventDefault();
		void navigate(url, {
			noScroll: a.closest('[data-tern-noscroll]') !== null,
			replaceState: a.closest('[data-tern-replacestate]') !== null || url.href === location.href
		}, 'link', 0);
	});

	let hoverTimer: ReturnType<typeof setTimeout> | undefined;
	document.addEventListener('mouseover', (event) => {
		clearTimeout(hoverTimer);
		const a = findAnchor(event);
		const url = a && anchorURL(a);
		if (!a || !url || preloadMode(a) !== 'hover' || samePage(url)) return;
		hoverTimer = setTimeout(() => void preloadData(url), 20);
	});

	const onTap = (event: Event) => {
		const a = findAnchor(event);
		const url = a && anchorURL(a);
		if (!a || !url || preloadMode(a) === 'off' || samePage(url)) return;
		void preloadData(url);
	};
	document.addEventListener('mousedown', onTap);
	document.addEventListener('touchstart', onTap, { passive: true });

	// GET forms navigate on the client, like links. POST forms are handled
	// by use:enhance or submit normally.
	document.addEventListener('submit', (event) => {
		if (event.defaultPrevented) return;
		const form = event.target as HTMLFormElement;
		const submitter = event.submitter as HTMLButtonElement | HTMLInputElement | null;
		const own = HTMLFormElement.prototype.cloneNode.call(form) as HTMLFormElement;
		const method = submitter?.hasAttribute('formmethod') ? submitter.formMethod : own.method;
		const target = submitter?.getAttribute('formtarget') ?? own.target;
		if (method !== 'get' || (target && target !== '_self') || form.closest('[data-tern-reload]:not([data-tern-reload="off"])')) {
			return;
		}
		const url = new URL(submitter?.hasAttribute('formaction') ? submitter.formAction : own.action);
		if (!inApp(url)) return;
		event.preventDefault();
		url.search = new URLSearchParams(new FormData(form, submitter) as unknown as Record<string, string>).toString();
		const filtering = url.pathname === location.pathname;
		void navigate(url, { keepFocus: filtering, noScroll: filtering }, 'form', 0);
	});

	addEventListener('beforeunload', (event) => {
		if (!runBefore({ from: current(), to: null, type: 'leave' }, true)) {
			event.preventDefault();
			event.returnValue = '';
		}
	});

	addEventListener('popstate', (event) => {
		if (ignoreNextPop) {
			ignoreNextPop = false;
			return;
		}
		const index = event.state?.ternIndex;
		if (index == null) return;
		saveScroll();
		const fromIndex = historyIndex;
		historyIndex = index;
		void navigate(new URL(location.href), { state: event.state?.ternState ?? {} }, 'popstate', 0, index, fromIndex);
	});

	addEventListener('pagehide', saveScroll);
}

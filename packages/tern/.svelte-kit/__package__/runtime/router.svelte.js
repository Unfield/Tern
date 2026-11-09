import { mount, tick } from 'svelte';
import DefaultError from './DefaultError.svelte';
import Passthrough from './Passthrough.svelte';
import Root from './Root.svelte';
import { page, setNavigating } from './state.svelte.js';
const MAX_REDIRECTS = 20;
const PRELOAD_TTL = 10_000;
const SCROLL_KEY = 'tern:scroll';
let manifest;
let started = false;
let token = 0;
let historyIndex = 0;
let nodes = [];
let layoutCount = 0;
let scrollPositions = {};
let announcer = null;
const props = $state({
    components: [],
    data: [],
    form: null
});
const preloads = new Map();
/** Starts the client. Called by the generated entry module. */
export async function start(m) {
    manifest = m;
    const el = document.getElementById('__tern_data');
    const target = document.getElementById('tern-root');
    if (!el || !target) {
        throw new Error('Tern: the page is missing its data or mount point; check that src/app.html contains %tern.head% and %tern.body%');
    }
    try {
        scrollPositions = JSON.parse(sessionStorage.getItem(SCROLL_KEY) ?? '{}');
    }
    catch {
        scrollPositions = {};
    }
    historyIndex = history.state?.ternIndex ?? Date.now();
    if (history.state?.ternIndex == null) {
        history.replaceState({ ...history.state, ternIndex: historyIndex, ternState: {} }, '');
    }
    history.scrollRestoration = 'manual';
    const url = new URL(location.href);
    const payload = JSON.parse(el.textContent ?? '{}');
    await render(payload, url, history.state?.ternState ?? {}, token);
    mount(Root, { target, props });
    started = true;
    const saved = scrollPositions[historyIndex];
    if (saved) {
        scrollTo(saved.x, saved.y);
    }
    else if (url.hash) {
        scrollToHash(url.hash);
    }
    listen();
}
/** Navigates to href, which may be relative to the current URL. */
export async function goto(href, opts = {}) {
    if (!started) {
        throw new Error('Tern: goto() was called before the app started');
    }
    const url = new URL(href, location.href);
    if (url.origin !== location.origin) {
        location.href = url.href;
        return;
    }
    await navigate(url, opts, 'goto', 0);
}
/** Reruns every load for the current page. */
export function invalidateAll() {
    return navigate(new URL(location.href), {
        replaceState: true,
        noScroll: true,
        keepFocus: true,
        invalidateAll: true,
        state: page.state
    }, 'goto', 0);
}
/**
 * Loads the data and code for a URL so a later navigation to it is instant.
 * Tern does this automatically for links; see data-tern-preload.
 */
export function preloadData(href) {
    const url = new URL(href, location.href);
    const key = url.pathname + url.search;
    const hit = preloads.get(key);
    if (hit && Date.now() - hit.at < PRELOAD_TTL) {
        return hit.promise;
    }
    const promise = fetchPayload(url).then(async (p) => {
        if (p?.type === 'data' && p.route) {
            await loadRoute(p.route).catch(() => { });
        }
        return p;
    });
    preloads.set(key, { at: Date.now(), promise });
    return promise;
}
/** Applies a form action result to the page, as `use:enhance` does. */
export async function applyAction(result) {
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
            await render({
                type: 'error',
                route: page.route.id ?? undefined,
                params: page.params,
                status: result.status,
                error: result.error,
                nodes: nodes.slice(0, layoutCount),
                failed: layoutCount
            }, page.url, page.state, token);
    }
}
function setForm(form) {
    page.form = form;
    props.form = form;
}
function dataURL(url) {
    const path = url.pathname.replace(/\/$/, '');
    return `${path}/__data.json${url.search}`;
}
async function fetchPayload(url) {
    try {
        const res = await fetch(dataURL(url), {
            headers: { accept: 'application/json' },
            credentials: 'same-origin'
        });
        if (!res.ok || !res.headers.get('content-type')?.includes('application/json')) {
            return null;
        }
        return (await res.json());
    }
    catch {
        return null;
    }
}
function takePreload(url) {
    const key = url.pathname + url.search;
    const hit = preloads.get(key);
    preloads.delete(key);
    if (hit && Date.now() - hit.at < PRELOAD_TTL) {
        return hit.promise;
    }
    return undefined;
}
async function navigate(url, opts, type, redirects, popIndex) {
    const id = ++token;
    // A change of hash alone is a scroll, not a navigation.
    if (type !== 'popstate' && !opts.invalidateAll && url.hash && samePage(url)) {
        saveScroll();
        history.pushState({ ternIndex: (historyIndex = Date.now()), ternState: opts.state ?? {} }, '', url);
        scrollToHash(url.hash);
        return;
    }
    const from = current();
    setNavigating({ from, to: { url, params: null, route: { id: null } }, type });
    const payload = await (opts.invalidateAll ? undefined : takePreload(url)) ?? (await fetchPayload(url));
    if (id !== token)
        return;
    if (!payload) {
        location.href = url.href; // not a page, or the server is unreachable
        return;
    }
    if (payload.type === 'redirect' && payload.location) {
        const next = new URL(payload.location, url);
        if (redirects >= MAX_REDIRECTS || next.origin !== location.origin) {
            location.href = next.href;
            return;
        }
        return navigate(next, { ...opts, replaceState: true }, type, redirects + 1);
    }
    if (type !== 'popstate')
        saveScroll(); // popstate saved before historyIndex changed
    const state = opts.state ?? (type === 'popstate' ? history.state?.ternState ?? {} : {});
    const ok = await render(payload, url, state, id);
    if (id !== token)
        return;
    if (!ok) {
        location.href = url.href;
        return;
    }
    if (type !== 'popstate') {
        if (opts.replaceState) {
            history.replaceState({ ternIndex: historyIndex, ternState: state }, '', url);
        }
        else {
            historyIndex = Date.now();
            history.pushState({ ternIndex: historyIndex, ternState: state }, '', url);
        }
    }
    setNavigating(null);
    await tick();
    if (type === 'popstate') {
        const pos = popIndex != null ? scrollPositions[popIndex] : undefined;
        scrollTo(pos?.x ?? 0, pos?.y ?? 0);
    }
    else if (!opts.noScroll) {
        if (url.hash)
            scrollToHash(url.hash);
        else
            scrollTo(0, 0);
    }
    if (!opts.keepFocus) {
        resetFocus();
    }
}
function samePage(url) {
    return url.pathname === location.pathname && url.search === location.search;
}
function current() {
    if (!started)
        return null;
    return { url: page.url, params: page.params, route: { id: page.route.id } };
}
async function loadLayout(i) {
    const entry = manifest.layouts[i];
    if (!entry?.component)
        return Passthrough;
    return (await entry.component()).default;
}
function loadRoute(id) {
    const route = manifest.routes[id];
    if (!route)
        return Promise.reject(new Error(`unknown route ${id}`));
    return Promise.all([...route.layouts.map(loadLayout), route.page().then((m) => m.default)]);
}
function mergeData(list) {
    const out = [];
    let acc = {};
    for (const node of list) {
        acc = { ...acc, ...(node ?? {}) };
        out.push(acc);
    }
    return out;
}
/** Renders a payload. It reports false if the payload is not a page. */
async function render(p, url, state, id) {
    let components;
    let data;
    const received = p.nodes ?? [];
    if (p.type === 'data') {
        const route = p.route ? manifest.routes[p.route] : undefined;
        if (!route || !p.route)
            return false;
        components = await loadRoute(p.route);
        data = mergeData(received);
        nodes = received;
        layoutCount = route.layouts.length;
    }
    else if (p.type === 'error') {
        const route = p.route ? manifest.routes[p.route] : undefined;
        if (p.route && !route)
            return false; // an endpoint, not a page
        const layouts = route ? route.layouts : manifest.fallback.layouts;
        const failed = p.failed ?? 0;
        const boundary = pickErrorPage(route ? route.errors : manifest.fallback.errors, failed);
        if (boundary) {
            components = [
                ...(await Promise.all(layouts.slice(0, boundary.layouts).map(loadLayout))),
                (await boundary.component()).default
            ];
            const merged = mergeData(received.slice(0, boundary.layouts));
            data = [...merged, merged[merged.length - 1] ?? {}];
        }
        else {
            components = [DefaultError];
            data = [{}];
        }
        nodes = received;
        layoutCount = boundary?.layouts ?? 0;
    }
    else {
        return false;
    }
    if (id !== token)
        return true; // superseded; the newer navigation renders
    page.url = url;
    page.params = p.params ?? {};
    page.route = { id: p.route ?? null };
    page.status = p.status;
    page.error = p.type === 'error' ? (p.error ?? { message: 'Error' }) : null;
    page.data = data[data.length - 1] ?? {};
    page.state = state;
    page.form = p.hasForm ? (p.form ?? null) : null;
    props.components = components;
    props.data = data;
    props.form = page.form;
    return true;
}
/** The innermost error page whose layouts all loaded successfully. */
function pickErrorPage(errors, failed) {
    let pick;
    for (const e of errors) {
        if (e.layouts <= failed)
            pick = e;
    }
    return pick;
}
function saveScroll() {
    scrollPositions[historyIndex] = { x: scrollX, y: scrollY };
    try {
        sessionStorage.setItem(SCROLL_KEY, JSON.stringify(scrollPositions));
    }
    catch {
        // Storage can be full or disabled; scroll restoration is best effort.
    }
}
function scrollToHash(hash) {
    const id = decodeURIComponent(hash.slice(1));
    const el = document.getElementById(id) ?? document.getElementsByName(id)[0];
    if (el)
        el.scrollIntoView();
    else
        scrollTo(0, 0);
}
/** Moves focus to the document and announces the new page to screen readers. */
function resetFocus() {
    const body = document.body;
    const tabindex = body.getAttribute('tabindex');
    body.tabIndex = -1;
    body.focus({ preventScroll: true });
    if (tabindex === null)
        body.removeAttribute('tabindex');
    else
        body.setAttribute('tabindex', tabindex);
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
function findAnchor(event) {
    for (const el of event.composedPath()) {
        if (el instanceof HTMLAnchorElement || el instanceof SVGAElement)
            return el;
    }
    return null;
}
function anchorURL(a) {
    const href = a instanceof SVGAElement ? a.href.baseVal : a.href;
    if (!href)
        return null;
    const url = new URL(href, document.baseURI);
    if (url.origin !== location.origin || !/^https?:$/.test(url.protocol))
        return null;
    const target = a instanceof SVGAElement ? a.target.baseVal : a.target;
    if (target && target !== '_self')
        return null;
    if (a.hasAttribute('download') || (a.getAttribute('rel') ?? '').split(/\s+/).includes('external'))
        return null;
    if (a.closest('[data-tern-reload]:not([data-tern-reload="off"])'))
        return null;
    return url;
}
function preloadMode(a) {
    return a.closest('[data-tern-preload]')?.getAttribute('data-tern-preload') ?? 'tap';
}
function listen() {
    document.addEventListener('click', (event) => {
        if (event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) {
            return;
        }
        const a = findAnchor(event);
        const url = a && anchorURL(a);
        if (!a || !url)
            return;
        event.preventDefault();
        void navigate(url, {
            noScroll: a.closest('[data-tern-noscroll]') !== null,
            replaceState: a.closest('[data-tern-replacestate]') !== null || url.href === location.href
        }, 'link', 0);
    });
    let hoverTimer;
    document.addEventListener('mouseover', (event) => {
        clearTimeout(hoverTimer);
        const a = findAnchor(event);
        const url = a && anchorURL(a);
        if (!a || !url || preloadMode(a) !== 'hover' || samePage(url))
            return;
        hoverTimer = setTimeout(() => void preloadData(url), 20);
    });
    const onTap = (event) => {
        const a = findAnchor(event);
        const url = a && anchorURL(a);
        if (!a || !url || preloadMode(a) === 'off' || samePage(url))
            return;
        void preloadData(url);
    };
    document.addEventListener('mousedown', onTap);
    document.addEventListener('touchstart', onTap, { passive: true });
    addEventListener('popstate', (event) => {
        const index = event.state?.ternIndex;
        if (index == null)
            return;
        saveScroll();
        historyIndex = index;
        void navigate(new URL(location.href), { state: event.state?.ternState ?? {} }, 'popstate', 0, index);
    });
    addEventListener('pagehide', saveScroll);
}

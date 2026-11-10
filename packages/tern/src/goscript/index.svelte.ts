// The browser side of the experimental <script lang="go">: loads the app's
// Go WebAssembly module once, mounts a Go instance per component, and keeps
// the values Go publishes in Svelte state. See docs/experimental-go-script.md.

import { getBase } from '../runtime/base.js';

type Instance = { call(name: string, args: string): string | null; destroy(): void };
type GoGlobal = { mount(id: string, props: string, emit: (name: string, value: string) => void): Instance | Error };

declare global {
	// eslint-disable-next-line no-var
	var __tern_go: GoGlobal | undefined;
	// eslint-disable-next-line no-var
	var __tern_go_ready: (() => void) | undefined;
	// eslint-disable-next-line no-var
	var Go: (new () => { importObject: WebAssembly.Imports; run(instance: WebAssembly.Instance): Promise<void> }) | undefined;
}

let loading: Promise<GoGlobal> | null = null;

function load(runtime: () => Promise<unknown>, wasm: string): Promise<GoGlobal> {
	loading ??= (async () => {
		await runtime(); // defines globalThis.Go
		const go = new globalThis.Go!();
		const ready = new Promise<void>((resolve) => (globalThis.__tern_go_ready = resolve));
		const res = await WebAssembly.instantiateStreaming(fetch(wasm), go.importObject);
		void go.run(res.instance);
		await ready;
		return globalThis.__tern_go!;
	})();
	return loading;
}

function toArg(a: unknown): unknown {
	if (typeof Event !== 'undefined' && a instanceof Event) {
		const t = a.target as HTMLInputElement | null;
		return { type: a.type, value: t?.value ?? '', checked: !!t?.checked, key: (a as KeyboardEvent).key ?? '' };
	}
	return a;
}

export interface GoComponent {
	/** The values Go published with State and Derived. */
	readonly state: Record<string, unknown>;
	/** Whether the Go instance is running; until then the state is the build-time initial state. */
	readonly ready: boolean;
	/** Calls a Func; returns its result, or undefined before the module loads. */
	call(name: string, ...args: unknown[]): unknown;
}

/**
 * Mounts a Go component; code generated from <script lang="go"> calls it.
 * initial is the state Setup produced at build time, so server rendering and
 * the first paint match what Go will compute.
 */
export function useGo(id: string, initial: Record<string, unknown>, runtime: () => Promise<unknown>, wasm: string, props: () => Record<string, unknown>): GoComponent {
	const state = $state<Record<string, unknown>>({ ...initial });
	let instance: Instance | null = null;
	let ready = $state(false);
	if (typeof window !== 'undefined') {
		$effect(() => {
			let alive = true;
			const json = JSON.stringify(props());
			void load(runtime, wasm).then((go) => {
				if (!alive) return;
				const m = go.mount(id, json, (name, value) => (state[name] = JSON.parse(value)));
				if (m instanceof Error) throw m;
				instance = m;
				ready = true;
			});
			return () => {
				alive = false;
				instance?.destroy();
				instance = null;
				ready = false;
			};
		});
	}
	return {
		state,
		get ready() {
			return ready;
		},
		call(name: string, ...args: unknown[]) {
			if (!instance) return undefined;
			const out = instance.call(name, JSON.stringify(args.map(toArg)));
			return out === null ? undefined : JSON.parse(out);
		}
	};
}

// Live components: the same Go API, running on the server. One WebSocket
// per page carries every instance's state and calls.

declare global {
	// eslint-disable-next-line no-var
	var __tern_live_initial: ((id: string, props: string) => string) | undefined;
}

type LiveMessage = { op: string; ref: number; id?: string; props?: unknown; name?: string; args?: unknown[]; seq?: number; value?: unknown; error?: string };
type LiveRef = { id: string; props: () => Record<string, unknown>; state: Record<string, unknown>; pending: Map<number, (v: unknown) => void> };

class LiveConnection {
	#ws: WebSocket | null = null;
	#refs = new Map<number, LiveRef>();
	#queue: string[] = [];
	#nextRef = 1;
	#seq = 1;
	#attempt = 0;
	#url: string;

	constructor(url: string) {
		this.#url = url;
	}

	#connect() {
		const u = new URL(this.#url, location.href);
		u.protocol = u.protocol === 'https:' ? 'wss:' : 'ws:';
		const ws = new WebSocket(u);
		this.#ws = ws;
		ws.onopen = () => {
			this.#attempt = 0;
			// After a reconnect, the server has forgotten every instance.
			for (const [ref, r] of this.#refs) ws.send(JSON.stringify({ op: 'mount', ref, id: r.id, props: r.props() }));
			for (const m of this.#queue.splice(0)) ws.send(m);
		};
		ws.onmessage = (e) => {
			const m = JSON.parse(e.data as string) as LiveMessage;
			const r = this.#refs.get(m.ref);
			if (!r) return;
			if (m.op === 'state' && m.name) r.state[m.name] = m.value;
			else if (m.op === 'result') {
				r.pending.get(m.seq!)?.(m.error ? Promise.reject(new Error(m.error)) : m.value);
				r.pending.delete(m.seq!);
			} else if (m.op === 'error') console.error(`[tern] live component ${r.id}: ${m.error}`);
		};
		ws.onclose = () => {
			this.#ws = null;
			if (this.#refs.size === 0) return;
			const delay = Math.min(10_000, 300 * 2 ** this.#attempt++);
			setTimeout(() => this.#refs.size && this.#connect(), delay);
		};
	}

	#send(m: object) {
		const data = JSON.stringify(m);
		if (this.#ws?.readyState === WebSocket.OPEN) this.#ws.send(data);
		else {
			this.#queue.push(data);
			if (!this.#ws) this.#connect();
		}
	}

	mount(r: LiveRef): number {
		const ref = this.#nextRef++;
		this.#refs.set(ref, r);
		this.#send({ op: 'mount', ref, id: r.id, props: r.props() });
		return ref;
	}

	unmount(ref: number) {
		this.#refs.delete(ref);
		if (this.#ws?.readyState === WebSocket.OPEN) this.#ws.send(JSON.stringify({ op: 'unmount', ref }));
		if (this.#refs.size === 0) {
			this.#ws?.close(1000);
			this.#ws = null;
		}
	}

	call(ref: number, name: string, args: unknown[]): Promise<unknown> {
		const r = this.#refs.get(ref);
		if (!r) return Promise.resolve(undefined);
		const seq = this.#seq++;
		return new Promise((resolve) => {
			r.pending.set(seq, resolve);
			this.#send({ op: 'call', ref, seq, name, args });
		});
	}
}

let connection: LiveConnection | null = null;

function escapeJSON(s: string) {
	return s.replace(/</g, '\\u003c');
}

export interface LiveComponent extends GoComponent {
	/** The server-rendered state, embedded in the page for hydration. */
	readonly stateScript: string;
}

/**
 * Mounts a live Go component; code generated from <script lang="go" live>
 * calls it. On the server, Setup runs in Go for the render, and the state
 * goes into the page; in the browser, the component connects to
 * /_tern/live, where its Go instance runs.
 */
export function useLive(id: string, uid: string, props: () => Record<string, unknown>): LiveComponent {
	const elementID = `tern-live-${uid}`;
	let initial: Record<string, unknown> = {};
	let stateScript = '';
	if (typeof window === 'undefined') {
		const raw = globalThis.__tern_live_initial?.(id, JSON.stringify(props())) ?? '{}';
		initial = JSON.parse(raw) as Record<string, unknown>;
		stateScript = `<script type="application/json" id="${elementID}">${escapeJSON(raw)}</script>`;
	} else {
		const el = document.getElementById(elementID);
		if (el) {
			initial = JSON.parse(el.textContent || '{}') as Record<string, unknown>;
			stateScript = el.outerHTML;
		}
	}
	const state = $state<Record<string, unknown>>(initial);
	let ref = 0;
	let ready = $state(false);
	if (typeof window !== 'undefined') {
		$effect(() => {
			connection ??= new LiveConnection(`${getBase()}/_tern/live`);
			const live: LiveRef = { id, props, state, pending: new Map() };
			ref = connection.mount(live);
			ready = true;
			return () => {
				connection?.unmount(ref);
				ref = 0;
				ready = false;
			};
		});
	}
	return {
		state,
		stateScript,
		get ready() {
			return ready;
		},
		call(name: string, ...args: unknown[]) {
			if (!ref || !connection) return undefined;
			return connection.call(ref, name, args.map(toArg));
		}
	};
}

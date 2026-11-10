// The browser side of the experimental <script lang="go">: loads the app's
// Go WebAssembly module once, mounts a Go instance per component, and keeps
// the values Go publishes in Svelte state. See docs/experimental-go-script.md.

import { untrack } from 'svelte';
import { getBase } from '../runtime/base.js';
import { applyPatch, type PatchOp } from './patch.js';

export { applyPatch, type PatchOp };

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
// per page carries every instance's state, calls and events.

declare global {
	// eslint-disable-next-line no-var
	var __tern_live_initial: ((id: string, props: string) => string) | undefined;
}

type LiveIn = {
	op: 'state' | 'result' | 'event' | 'error';
	ref: number;
	set?: Record<string, unknown>;
	patch?: Record<string, PatchOp[]>;
	seq?: number;
	name?: string;
	value?: unknown;
	error?: string;
};

function clone<T>(v: T): T {
	return v === undefined ? v : (JSON.parse(JSON.stringify(v)) as T);
}

type Call = { resolve: (v: unknown) => void; reject: (e: Error) => void; name: string; holds: string[]; sent: boolean };

/** The browser side of one live component instance. */
class LiveInstance {
	/** What the markup shows: the server's values, or optimistic ones while a call is pending. */
	state: Record<string, unknown>;
	/** The server's values, as last sent. */
	#server: Record<string, unknown>;
	#holds = new Map<string, number>();
	calls = new Map<number, Call>();
	pendingCalls = $state<Record<string, number>>({});
	errors = $state<Record<string, string | null>>({});
	error = $state<string | null>(null);
	/** Whether the component has its state: from the server render, or the first message. */
	loaded = $state(false);
	mounted = false;

	constructor(
		readonly id: string,
		readonly props: () => Record<string, unknown>,
		initial: Record<string, unknown> | null
	) {
		const { __tern_error: error, ...values } = initial ?? {};
		this.state = $state(clone(values));
		this.#server = clone(values);
		this.loaded = initial !== null;
		if (typeof error === 'string') this.error = error;
	}

	apply(m: LiveIn) {
		this.loaded = true;
		for (const [name, v] of Object.entries(m.set ?? {})) {
			this.#server[name] = clone(v);
			if (!this.#holds.has(name)) this.state[name] = v;
		}
		for (const [name, ops] of Object.entries(m.patch ?? {})) {
			this.#server[name] = applyPatch(this.#server[name], clone(ops));
			if (!this.#holds.has(name)) this.state[name] = applyPatch(this.state[name], ops);
		}
	}

	/** Shows value for name until the operation holding it settles. */
	hold(name: string, value: unknown) {
		this.#holds.set(name, (this.#holds.get(name) ?? 0) + 1);
		this.state[name] = value;
	}

	release(names: string[]) {
		for (const name of names) {
			const n = (this.#holds.get(name) ?? 1) - 1;
			if (n > 0) {
				this.#holds.set(name, n);
				continue;
			}
			this.#holds.delete(name);
			this.state[name] = clone(this.#server[name]);
		}
	}

	begin(seq: number, call: Call) {
		this.calls.set(seq, call);
		this.pendingCalls[call.name] = (this.pendingCalls[call.name] ?? 0) + 1;
	}

	settle(seq: number, value: unknown, error?: string) {
		const call = this.calls.get(seq);
		if (!call) return;
		this.calls.delete(seq);
		this.pendingCalls[call.name] = (this.pendingCalls[call.name] ?? 1) - 1;
		this.release(call.holds);
		this.errors[call.name] = error ?? null;
		if (error) call.reject(new Error(error));
		else call.resolve(value);
	}

	/** The values to restore after reconnecting to a new server process. */
	snapshot(): Record<string, unknown> {
		return $state.snapshot(this.state) as Record<string, unknown>;
	}
}

const status = $state({ connected: false });

class LiveConnection {
	#ws: WebSocket | null = null;
	#refs = new Map<number, LiveInstance>();
	#queue: { data: string; seq?: number; ref?: number }[] = [];
	#nextRef = 1;
	#seq = 1;
	#attempt = 0;
	#url: string;

	constructor(url: string) {
		this.#url = url;
	}

	#mountMessage(ref: number, r: LiveInstance) {
		const props = untrack(() => r.props());
		// After a reconnect, the server has forgotten the instance: values
		// declared with Bind or Restore come back from what the page shows.
		return JSON.stringify({ op: 'mount', ref, id: r.id, props, restore: r.mounted ? r.snapshot() : undefined });
	}

	#connect() {
		const u = new URL(this.#url, location.href);
		u.protocol = u.protocol === 'https:' ? 'wss:' : 'ws:';
		const ws = new WebSocket(u);
		this.#ws = ws;
		ws.onopen = () => {
			this.#attempt = 0;
			status.connected = true;
			for (const [ref, r] of this.#refs) {
				ws.send(this.#mountMessage(ref, r));
				r.mounted = true;
			}
			for (const m of this.#queue.splice(0)) {
				if (m.ref !== undefined && !this.#refs.has(m.ref)) continue;
				ws.send(m.data);
				if (m.seq !== undefined) this.#markSent(m.ref!, m.seq);
			}
		};
		ws.onmessage = (e) => {
			const m = JSON.parse(e.data as string) as LiveIn;
			const r = this.#refs.get(m.ref);
			if (!r) return;
			if (m.op === 'state') r.apply(m);
			else if (m.op === 'result') r.settle(m.seq!, m.value, m.error);
			else if (m.op === 'event') {
				const handler = untrack(() => r.props())['on' + m.name];
				if (typeof handler === 'function') handler(m.value);
			} else if (m.op === 'error') {
				r.error = m.error ?? 'error';
				r.loaded = true;
				console.error(`[tern] live component ${r.id}: ${m.error}`);
			}
		};
		ws.onclose = () => {
			if (this.#ws === ws) this.#ws = null;
			status.connected = false;
			// Calls that reached the old connection have no answer coming.
			for (const r of this.#refs.values()) {
				for (const [seq, call] of r.calls) if (call.sent) r.settle(seq, undefined, 'connection lost');
			}
			if (this.#refs.size === 0) return;
			const delay = Math.min(10_000, 250 * 2 ** this.#attempt++) * (0.75 + Math.random() / 2);
			setTimeout(() => this.#refs.size && !this.#ws && this.#connect(), delay);
		};
	}

	#markSent(ref: number, seq: number) {
		const call = this.#refs.get(ref)?.calls.get(seq);
		if (call) call.sent = true;
	}

	#send(m: Record<string, unknown>) {
		const data = JSON.stringify(m);
		const seq = m.seq as number | undefined;
		const ref = m.ref as number | undefined;
		if (this.#ws?.readyState === WebSocket.OPEN) {
			this.#ws.send(data);
			if (seq !== undefined) this.#markSent(ref!, seq);
		} else {
			this.#queue.push({ data, seq, ref });
			if (!this.#ws) this.#connect();
		}
	}

	mount(r: LiveInstance): number {
		const ref = this.#nextRef++;
		this.#refs.set(ref, r);
		if (this.#ws?.readyState === WebSocket.OPEN) {
			this.#ws.send(this.#mountMessage(ref, r));
			r.mounted = true;
		} else if (!this.#ws) this.#connect();
		return ref;
	}

	unmount(ref: number) {
		this.#refs.delete(ref);
		if (this.#ws?.readyState === WebSocket.OPEN) this.#ws.send(JSON.stringify({ op: 'unmount', ref }));
		if (this.#refs.size === 0) {
			const ws = this.#ws;
			this.#ws = null;
			this.#queue = [];
			ws?.close(1000);
		}
	}

	props(ref: number, props: Record<string, unknown>) {
		this.#send({ op: 'props', ref, props });
	}

	call(ref: number, name: string, args: unknown[], optimistic?: Record<string, unknown>): Promise<unknown> {
		const r = this.#refs.get(ref);
		if (!r) return Promise.reject(new Error('not mounted'));
		const seq = this.#seq++;
		const holds = Object.keys(optimistic ?? {});
		for (const k of holds) r.hold(k, optimistic![k]);
		const p = new Promise<unknown>((resolve, reject) => {
			r.begin(seq, { resolve, reject, name, holds, sent: false });
		});
		this.#send({ op: 'call', ref, seq, name, args });
		// The error is also in fn.error; don't report unawaited calls as
		// unhandled rejections.
		p.catch(() => {});
		return p;
	}

	set(ref: number, name: string, value: unknown): Promise<unknown> {
		const r = this.#refs.get(ref);
		if (!r) return Promise.reject(new Error('not mounted'));
		const seq = this.#seq++;
		r.hold(name, value);
		const p = new Promise<unknown>((resolve, reject) => {
			r.begin(seq, { resolve, reject, name: '$set:' + name, holds: [name], sent: false });
		});
		this.#send({ op: 'set', ref, seq, name, value: $state.snapshot(value) });
		p.catch(() => {});
		return p;
	}
}

let connection: LiveConnection | null = null;

function escapeJSON(s: string) {
	return s.replace(/</g, '\\u003c');
}

/** A Func of a live component, as the markup sees it. */
export interface LiveFunc<A extends unknown[] = unknown[], R = unknown> {
	(...args: A): Promise<R>;
	/** Whether a call is waiting for the server. */
	readonly pending: boolean;
	/** The last call's error message (from goscript.Fail), or null. */
	readonly error: string | null;
	/** Calls the Func, showing values for these state names until it returns. */
	optimistic(values: Record<string, unknown>, ...args: A): Promise<R>;
	/** A version that waits until calls stop for ms (default 200), for input events. */
	debounce(ms?: number): (...args: A) => void;
}

export interface LiveComponent<S extends Record<string, unknown> = Record<string, unknown>> extends GoComponent {
	readonly state: S;
	/** The server-rendered state, embedded in the page for hydration. */
	readonly stateScript: string;
	/** Whether the page is connected to the server. */
	readonly connected: boolean;
	/** Whether any call is waiting for the server. */
	readonly pending: boolean;
	/** Why the component failed to mount (such as goscript Reject's message), or null. */
	readonly error: string | null;
	/** Whether the state has arrived; the markup renders once it has. */
	readonly loaded: boolean;
	/** Everything the parent passed, such as children snippets and onevent handlers. */
	readonly props: Record<string, any>;
	/** Bindable state: bind:value={live.bind.name} for a State declared with goscript.Bind. */
	readonly bind: S;
	/** Sets a State declared with goscript.Bind; the page shows value at once. */
	set<K extends keyof S & string>(name: K, value: S[K]): Promise<void>;
	/** The Func named name. */
	fn<A extends unknown[] = unknown[], R = unknown>(name: string): LiveFunc<A, R>;
}

/**
 * Mounts a live Go component; code generated from <script lang="go" live>
 * calls it. On the server, Setup runs in Go for the render, and the state
 * goes into the page; in the browser, the component connects to
 * /_tern/live, where its Go instance runs.
 */
export function useLive(id: string, uid: string, props: () => Record<string, unknown>): LiveComponent {
	const elementID = `tern-live-${uid}`;
	// After a client-side navigation there is no server-rendered state: the
	// markup waits for the first message (see the preprocessor).
	let initial: Record<string, unknown> | null = null;
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
	const inst = new LiveInstance(id, props, initial);
	let ref = 0;
	let ready = $state(false);
	if (typeof window !== 'undefined') {
		$effect(() => {
			connection ??= new LiveConnection(`${getBase()}/_tern/live`);
			ref = connection.mount(inst);
			ready = true;
			return () => {
				connection?.unmount(ref);
				ref = 0;
				ready = false;
			};
		});
		// New props from the parent go to the server without remounting.
		let sent = JSON.stringify(untrack(props));
		$effect(() => {
			const next = JSON.stringify(props());
			if (next !== sent && ref) {
				sent = next;
				connection!.props(ref, JSON.parse(next) as Record<string, unknown>);
			}
		});
	}
	const call = (name: string, args: unknown[], optimistic?: Record<string, unknown>) => {
		if (!ref || !connection) return Promise.reject(new Error('not mounted'));
		return connection.call(ref, name, args.map(toArg), optimistic);
	};
	const funcs = new Map<string, LiveFunc>();
	const fn = (name: string): LiveFunc => {
		let f = funcs.get(name);
		if (f) return f;
		const debounced = new Map<number, (...args: unknown[]) => void>();
		f = Object.assign((...args: unknown[]) => call(name, args), {
			optimistic: (values: Record<string, unknown>, ...args: unknown[]) => call(name, args, values),
			debounce: (ms = 200) => {
				let d = debounced.get(ms);
				if (!d) {
					let timer: ReturnType<typeof setTimeout> | undefined;
					d = (...args: unknown[]) => {
						const converted = args.map(toArg); // read input values now
						clearTimeout(timer);
						timer = setTimeout(() => void call(name, converted), ms);
					};
					debounced.set(ms, d);
				}
				return d;
			}
		}) as unknown as LiveFunc;
		Object.defineProperties(f, {
			pending: { get: () => (inst.pendingCalls[name] ?? 0) > 0 },
			error: { get: () => inst.errors[name] ?? null }
		});
		funcs.set(name, f);
		return f;
	};
	const set = (name: string, value: unknown) => {
		if (!ref || !connection) {
			inst.state[name] = value;
			return Promise.resolve();
		}
		return connection.set(ref, name, value).then(() => undefined);
	};
	const bind = new Proxy({} as Record<string, unknown>, {
		get: (_, name) => (typeof name === 'string' ? inst.state[name] : undefined),
		set: (_, name, value) => {
			if (typeof name === 'string') void set(name, value);
			return true;
		}
	});
	return {
		state: inst.state,
		stateScript,
		bind,
		fn: fn as LiveComponent['fn'],
		set: set as LiveComponent['set'],
		get ready() {
			return ready;
		},
		get props() {
			return props();
		},
		get connected() {
			return status.connected;
		},
		get pending() {
			return Object.values(inst.pendingCalls).some((n) => n > 0);
		},
		get error() {
			return inst.error;
		},
		get loaded() {
			return inst.loaded;
		},
		call(name: string, ...args: unknown[]) {
			return call(name, args);
		}
	};
}

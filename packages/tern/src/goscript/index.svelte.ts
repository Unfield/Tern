// The browser side of the experimental <script lang="go">: loads the app's
// Go WebAssembly module once, mounts a Go instance per component, and keeps
// the values Go publishes in Svelte state. See docs/experimental-go-script.md.

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

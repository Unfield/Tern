/** A resolved or failed deferred value, as streamed by the server. */
export interface Chunk {
	id: number;
	data?: unknown;
	error?: { message: string };
}

/**
 * Connects the deferred placeholders of one response to the chunks that
 * resolve them. IDs are only unique within a response.
 */
export class DeferredScope {
	#pending = new Map<number, { resolve: (v: unknown) => void; reject: (e: Error) => void }>();
	#early = new Map<number, Chunk>();

	promise(id: number): Promise<unknown> {
		const early = this.#early.get(id);
		if (early) {
			this.#early.delete(id);
			return early.error ? Promise.reject(new Error(early.error.message)) : Promise.resolve(early.data);
		}
		return new Promise((resolve, reject) => this.#pending.set(id, { resolve, reject }));
	}

	settle(chunk: Chunk) {
		const p = this.#pending.get(chunk.id);
		if (!p) {
			this.#early.set(chunk.id, chunk);
			return;
		}
		this.#pending.delete(chunk.id);
		if (chunk.error) p.reject(new Error(chunk.error.message));
		else p.resolve(chunk.data);
	}

	/** Rejects every value still pending, e.g. when the stream ends early. */
	close(reason: string) {
		for (const p of this.#pending.values()) p.reject(new Error(reason));
		this.#pending.clear();
	}
}

/**
 * Replaces deferred placeholders in data with promises from scope. The data
 * must be fresh from JSON.parse: it is changed in place, which saves
 * copying every page's data on every render.
 */
export function revive<T>(value: T, promise: (id: number) => Promise<unknown>): T {
	if (value === null || typeof value !== 'object') return value;
	if (Array.isArray(value)) {
		for (let i = 0; i < value.length; i++) {
			const v = value[i];
			if (v !== null && typeof v === 'object') {
				const r = revive(v, promise);
				if (r !== v) value[i] = r;
			}
		}
		return value;
	}
	const obj = value as Record<string, unknown>;
	if (typeof obj.__tern_deferred === 'number' && Object.keys(obj).length === 1) {
		return promise(obj.__tern_deferred) as T;
	}
	for (const k in obj) {
		const v = obj[k];
		if (v !== null && typeof v === 'object') {
			const r = revive(v, promise);
			// defineProperty, so a key named __proto__ stays a plain property.
			if (r !== v) Object.defineProperty(obj, k, { value: r, writable: true, enumerable: true, configurable: true });
		}
	}
	return value;
}

/**
 * Reads a response that holds one JSON payload line, optionally followed
 * by one line per resolved deferred value. Resolves once the payload is
 * read; deferred values keep streaming in the background.
 */
export async function readStreamed<P extends { nodes?: unknown }>(res: Response): Promise<P> {
	if (!res.body) {
		const [first, ...rest] = (await res.text()).split('\n');
		const scope = new DeferredScope();
		const payload = JSON.parse(first!) as P;
		payload.nodes = revive(payload.nodes, (id) => scope.promise(id));
		for (const line of rest) if (line.trim()) scope.settle(JSON.parse(line));
		scope.close('The server did not send this value');
		return payload;
	}

	const reader = res.body.getReader();
	const decoder = new TextDecoder();
	let buffer = '';
	let done = false;
	while (!buffer.includes('\n') && !done) {
		const r = await reader.read();
		done = r.done;
		if (r.value) buffer += decoder.decode(r.value, { stream: true });
	}
	const newline = buffer.indexOf('\n');
	const first = newline < 0 ? buffer : buffer.slice(0, newline);
	buffer = newline < 0 ? '' : buffer.slice(newline + 1);

	const scope = new DeferredScope();
	const payload = JSON.parse(first) as P;
	payload.nodes = revive(payload.nodes, (id) => scope.promise(id));

	void (async () => {
		try {
			for (;;) {
				let i;
				while ((i = buffer.indexOf('\n')) >= 0) {
					const line = buffer.slice(0, i);
					buffer = buffer.slice(i + 1);
					if (line.trim()) scope.settle(JSON.parse(line));
				}
				if (done) break;
				const r = await reader.read();
				done = r.done;
				if (r.value) buffer += decoder.decode(r.value, { stream: true });
				if (done && buffer.trim()) buffer += '\n';
			}
		} finally {
			scope.close('The connection closed before this value arrived');
		}
	})();
	return payload;
}

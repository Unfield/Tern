/** An error answer from a typed endpoint. */
export class ApiError extends Error {
	/** The HTTP status. */
	readonly status: number;
	/** The decoded response body, if it was JSON. */
	readonly body: unknown;
	constructor(status: number, message: string, body: unknown) {
		super(message);
		this.name = 'ApiError';
		this.status = status;
		this.body = body;
	}
}

type Options = {
	params?: Record<string, unknown>;
	query?: Record<string, unknown>;
	body?: unknown;
	signal?: AbortSignal;
	fetch?: typeof fetch;
};

type Route = (pattern: string, params?: Record<string, unknown>) => string;

function request(route: Route, endpoint: string, options: Options, accept: string): { url: string; init: RequestInit } {
	const space = endpoint.indexOf(' ');
	const method = endpoint.slice(0, space);
	let url = route(endpoint.slice(space + 1), options.params ?? {});
	const headers: Record<string, string> = { Accept: accept };
	const init: RequestInit = { method, headers, credentials: 'same-origin', signal: options.signal };
	if (method === 'GET' || method === 'HEAD' || method === 'DELETE') {
		const q = new URLSearchParams();
		for (const [k, v] of Object.entries(options.query ?? {})) {
			if (v === undefined || v === null) continue;
			for (const item of Array.isArray(v) ? v : [v]) q.append(k, item instanceof Date ? item.toISOString() : String(item));
		}
		const qs = q.toString();
		if (qs) url += '?' + qs;
	} else {
		headers['Content-Type'] = 'application/json';
		init.body = JSON.stringify(options.body ?? {});
	}
	return { url, init };
}

async function failure(res: Response): Promise<ApiError> {
	const text = await res.text();
	let data: unknown = text || null;
	try {
		data = text ? JSON.parse(text) : null;
	} catch {
		// not JSON: keep the text
	}
	const message = (data as { message?: string } | null)?.message ?? res.statusText;
	return new ApiError(res.status, message, data);
}

/** Creates the $api client from $routes' route(); generated code calls it. */
export function createApi(route: Route) {
	return async function api(endpoint: string, options: Options = {}): Promise<unknown> {
		const { url, init } = request(route, endpoint, options, 'application/json');
		const res = await (options.fetch ?? fetch)(url, init);
		if (!res.ok) throw await failure(res);
		const text = await res.text();
		if (!text) return null;
		try {
			return JSON.parse(text);
		} catch {
			return text;
		}
	};
}

/**
 * A streaming endpoint's answer: iterate it with for await to get each
 * value as the server sends it.
 */
export interface ApiStream<T> extends AsyncIterable<T> {
	/** Stops the stream: the loop ends, and the server's request is cancelled. */
	abort(): void;
}

/** Creates $api's stream from $routes' route(); generated code calls it. */
export function createStream(route: Route) {
	return function stream(endpoint: string, options: Options = {}): ApiStream<unknown> {
		const controller = new AbortController();
		options.signal?.addEventListener('abort', () => controller.abort(options.signal?.reason), { once: true });
		let started = false;
		async function* read(): AsyncGenerator<unknown> {
			const { url, init } = request(route, endpoint, options, 'text/event-stream');
			init.signal = controller.signal;
			let res: Response;
			try {
				res = await (options.fetch ?? fetch)(url, init);
			} catch (err) {
				if (controller.signal.aborted) return;
				throw err;
			}
			if (!res.ok) throw await failure(res);
			if (!res.body || !res.headers.get('content-type')?.includes('text/event-stream')) {
				throw new ApiError(res.status, `${endpoint} didn’t answer with a stream`, null);
			}
			const reader = res.body.pipeThrough(new TextDecoderStream()).getReader();
			let buf = '';
			try {
				for (;;) {
					let chunk: ReadableStreamReadResult<string>;
					try {
						chunk = await reader.read();
					} catch (err) {
						if (controller.signal.aborted) return;
						throw err;
					}
					if (chunk.done) {
						// The body ended without the server's done: it was cut off.
						if (controller.signal.aborted) return;
						throw new ApiError(0, 'The stream ended before it was done', null);
					}
					buf += chunk.value.replace(/\r\n/g, '\n');
					let end: number;
					while ((end = buf.indexOf('\n\n')) >= 0) {
						if (controller.signal.aborted) return;
						const raw = buf.slice(0, end);
						buf = buf.slice(end + 2);
						let event = 'message';
						const data: string[] = [];
						for (const line of raw.split('\n')) {
							if (line.startsWith('event:')) event = line.slice(6).trim();
							else if (line.startsWith('data:')) data.push(line.slice(5).replace(/^ /, ''));
						}
						if (!data.length) continue;
						const value = JSON.parse(data.join('\n'));
						if (event === 'done') return;
						if (event === 'error') {
							const e = value as { status?: number; message?: string };
							throw new ApiError(e.status ?? 500, e.message ?? 'The stream failed', value);
						}
						yield value;
					}
				}
			} finally {
				reader.releaseLock();
				// Leaving the loop early, with break or an error, stops the request.
				controller.abort();
			}
		}
		return {
			[Symbol.asyncIterator]() {
				if (started) throw new Error('A stream can be read only once');
				started = true;
				return read();
			},
			abort: () => controller.abort()
		};
	};
}

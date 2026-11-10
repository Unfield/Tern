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

/** Creates the $api client from $routes' route(); generated code calls it. */
export function createApi(route: (pattern: string, params?: Record<string, unknown>) => string) {
	return async function api(endpoint: string, options: Options = {}): Promise<unknown> {
		const space = endpoint.indexOf(' ');
		const method = endpoint.slice(0, space);
		let url = route(endpoint.slice(space + 1), options.params ?? {});
		const headers: Record<string, string> = { Accept: 'application/json' };
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
		const res = await (options.fetch ?? fetch)(url, init);
		const text = await res.text();
		let data: unknown = null;
		if (text) {
			try {
				data = JSON.parse(text);
			} catch {
				data = text;
			}
		}
		if (!res.ok) {
			const message = (data as { message?: string } | null)?.message ?? res.statusText;
			throw new ApiError(res.status, message, data);
		}
		return data;
	};
}

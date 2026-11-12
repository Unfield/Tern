// Under tern dev, the router and forms tell the dev overlay what each
// page and action did, with a "tern:dev" event on window. The overlay
// (tern dev's /__tern/dev/client.js) looks up the request's logs, spans
// and errors by its ID. Builds leave this out: callers check
// import.meta.env.DEV first.

import type { Payload } from './types.js';

/** What the server tells the overlay about a response, in dev only. */
export type DevInfo = { requestId: string; components: string[] };

export type DevEvent =
	| {
			kind: 'page';
			url: string;
			route: string | null;
			params: Record<string, string>;
			status: number;
			error: string | null;
			/** The node whose load failed, for an error. */
			failed: number | null;
			nodes: unknown[];
			requestId: string;
			components: string[];
	  }
	| { kind: 'action'; url: string; type: string; status: number; requestId: string; data: unknown };

function emit(detail: DevEvent) {
	const w = window as unknown as { __ternDev?: DevEvent[] };
	// The overlay may load after the first page renders: it reads what it missed here.
	(w.__ternDev ??= []).push(detail);
	if (w.__ternDev.length > 50) w.__ternDev.shift();
	dispatchEvent(new CustomEvent('tern:dev', { detail }));
}

export function devPage(p: Payload, url: URL) {
	const dev = (p as Payload & { dev?: DevInfo }).dev;
	emit({
		kind: 'page',
		url: url.pathname + url.search,
		route: p.route ?? null,
		params: p.params ?? {},
		status: p.status,
		error: p.error?.message ?? null,
		failed: p.failed ?? null,
		nodes: p.nodes ?? [],
		requestId: dev?.requestId ?? '',
		components: dev?.components ?? []
	});
}

export function devAction(action: URL, result: { type: string; status?: number; data?: unknown }, requestId: string) {
	emit({ kind: 'action', url: action.pathname + action.search, type: result.type, status: result.status ?? 200, requestId, data: result.data ?? null });
}

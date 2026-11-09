import type { ErrorEntry, Payload } from './types.js';

/** The parts of a manifest that decide what renders. */
export interface Tree<C> {
	routes: Record<string, { layouts: number[]; errors: { layouts: number }[] } & C>;
	fallback: { layouts: number[]; errors: { layouts: number }[] };
}

/** What to render for a payload. Shared by the server and the client so hydration matches. */
export type Plan =
	| { kind: 'page'; route: string; layouts: number[]; data: Record<string, unknown>[] }
	| { kind: 'error'; route: string | null; layouts: number[]; errorIndex: number; data: Record<string, unknown>[] }
	| { kind: 'default-error'; data: Record<string, unknown>[] }
	| { kind: 'not-a-page' };

export function mergeData(list: (Record<string, unknown> | null | undefined)[]): Record<string, unknown>[] {
	const out: Record<string, unknown>[] = [];
	let acc: Record<string, unknown> = {};
	for (const node of list) {
		acc = { ...acc, ...(node ?? {}) };
		out.push(acc);
	}
	return out;
}

/** The innermost error page whose layouts all loaded successfully. */
export function pickErrorPage(errors: Pick<ErrorEntry, 'layouts'>[], failed: number): number {
	let pick = -1;
	errors.forEach((e, i) => {
		if (e.layouts <= failed) pick = i;
	});
	return pick;
}

export function plan<C>(tree: Tree<C>, p: Payload): Plan {
	const received = p.nodes ?? [];
	const route = p.route ? tree.routes[p.route] : undefined;

	if (p.type === 'data') {
		if (!route || !p.route) return { kind: 'not-a-page' };
		return { kind: 'page', route: p.route, layouts: route.layouts, data: mergeData(received) };
	}
	if (p.type !== 'error' || (p.route && !route)) {
		return { kind: 'not-a-page' }; // an endpoint, or a non-render result
	}

	const layouts = route ? route.layouts : tree.fallback.layouts;
	const errors = route ? route.errors : tree.fallback.errors;
	const index = pickErrorPage(errors, p.failed ?? 0);
	if (index < 0) {
		return { kind: 'default-error', data: [{}] };
	}
	const above = errors[index]!.layouts;
	const merged = mergeData(received.slice(0, above));
	return {
		kind: 'error',
		route: p.route ?? null,
		layouts: layouts.slice(0, above),
		errorIndex: index,
		data: [...merged, merged[merged.length - 1] ?? {}]
	};
}

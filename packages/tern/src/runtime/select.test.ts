import { describe, expect, it } from 'vitest';
import { mergeData, pickErrorPage, plan, type Tree } from './select.js';

const tree: Tree<object> = {
	routes: {
		'/blog/slug.param': { layouts: [0, 1], errors: [{ layouts: 1 }, { layouts: 2 }] },
		'/': { layouts: [0], errors: [{ layouts: 1 }] }
	},
	fallback: { layouts: [0], errors: [{ layouts: 1 }] }
};

describe('mergeData', () => {
	it('merges each level with the ones above it', () => {
		expect(mergeData([{ a: 1 }, null, { b: 2, a: 3 }])).toEqual([{ a: 1 }, { a: 1 }, { a: 3, b: 2 }]);
	});
});

describe('pickErrorPage', () => {
	it('picks the innermost boundary whose layouts loaded', () => {
		const errors = [{ layouts: 1 }, { layouts: 2 }];
		expect(pickErrorPage(errors, 2)).toBe(1);
		expect(pickErrorPage(errors, 1)).toBe(0);
		expect(pickErrorPage(errors, 0)).toBe(-1);
	});
});

describe('plan', () => {
	it('plans a page with merged data', () => {
		const p = plan(tree, { type: 'data', route: '/blog/slug.param', status: 200, nodes: [{ user: 'a' }, null, { title: 't' }] });
		expect(p).toEqual({ kind: 'page', route: '/blog/slug.param', layouts: [0, 1], data: [{ user: 'a' }, { user: 'a' }, { user: 'a', title: 't' }] });
	});

	it('renders an error inside the layouts that loaded', () => {
		const p = plan(tree, { type: 'error', route: '/blog/slug.param', status: 404, nodes: [{ user: 'a' }, { x: 1 }], failed: 2 });
		expect(p).toMatchObject({ kind: 'error', layouts: [0, 1], errorIndex: 1 });
	});

	it('falls back when a layout itself failed', () => {
		const p = plan(tree, { type: 'error', route: '/', status: 500, nodes: [], failed: 0 });
		expect(p.kind).toBe('default-error');
	});

	it('uses the fallback tree for unmatched URLs', () => {
		const p = plan(tree, { type: 'error', status: 404, nodes: [{ user: 'a' }], failed: 1 });
		expect(p).toMatchObject({ kind: 'error', route: null, layouts: [0], errorIndex: 0 });
	});

	it('recognizes endpoints and other non-page payloads', () => {
		expect(plan(tree, { type: 'error', route: '/api/thing', status: 404 }).kind).toBe('not-a-page');
		expect(plan(tree, { type: 'redirect', status: 303, location: '/' }).kind).toBe('not-a-page');
	});
});

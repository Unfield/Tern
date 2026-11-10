import { describe, expect, it } from 'vitest';
import { applyPatch } from './patch.js';

describe('applyPatch', () => {
	it('applies the operations internal/jsondiff produces', () => {
		const root = { items: [{ id: 1, done: false }, { id: 2, done: false }], tags: { a: 1, b: 2 } };
		const out = applyPatch(root, [
			['s', ['items', 1, 'done'], true],
			['a', ['items'], 2, 0, [{ id: 3, done: false }]],
			['d', ['tags', 'a']],
			['s', ['tags', 'c'], 3]
		]);
		expect(out).toEqual({ items: [{ id: 1, done: false }, { id: 2, done: true }, { id: 3, done: false }], tags: { b: 2, c: 3 } });
	});
	it('replaces or splices the root', () => {
		expect(applyPatch(1, [['s', [], 2]])).toBe(2);
		expect(applyPatch([1, 2, 3], [['a', [], 1, 1, []]])).toEqual([1, 3]);
	});
});

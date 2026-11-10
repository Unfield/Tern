import { describe, expect, it } from 'vitest';
import { compute } from './floating.js';

const vp = { width: 800, height: 600 };
const size = { width: 200, height: 100 };

describe('compute', () => {
	it('places below, aligned to the start', () => {
		expect(compute({ x: 100, y: 100, width: 80, height: 30 }, size, vp)).toEqual({ x: 100, y: 136, side: 'bottom' });
	});
	it('aligns to the end and the center', () => {
		const a = { x: 300, y: 100, width: 80, height: 30 };
		expect(compute(a, size, vp, { align: 'end' }).x).toBe(180);
		expect(compute(a, size, vp, { align: 'center' }).x).toBe(240);
	});
	it('flips to the top when there is no room below', () => {
		expect(compute({ x: 100, y: 540, width: 80, height: 30 }, size, vp)).toEqual({ x: 100, y: 434, side: 'top' });
	});
	it('stays put when neither side fits, shifted on screen', () => {
		const r = compute({ x: 100, y: 250, width: 80, height: 30 }, { width: 200, height: 580 }, vp);
		expect(r.side).toBe('bottom');
		expect(r.y).toBe(12); // 600 - 580 - 8
	});
	it('shifts along the side at the viewport edge', () => {
		expect(compute({ x: 750, y: 100, width: 40, height: 30 }, size, vp).x).toBe(592);
		expect(compute({ x: -50, y: 100, width: 40, height: 30 }, size, vp).x).toBe(8);
	});
	it('places left and right, and flips between them', () => {
		const a = { x: 300, y: 200, width: 80, height: 30 };
		expect(compute(a, size, vp, { side: 'right' })).toMatchObject({ x: 386, side: 'right' });
		expect(compute({ x: 700, y: 200, width: 80, height: 30 }, size, vp, { side: 'right' })).toMatchObject({ x: 494, side: 'left' });
	});
});

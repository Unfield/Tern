import { describe, expect, it } from 'vitest';
import { formatNumber, niceTicks, numericX, timeTicks } from './chart.js';

describe('chart helpers', () => {
	it('makes round ticks', () => {
		expect(niceTicks(0, 87)).toEqual([0, 20, 40, 60, 80, 100]);
		expect(niceTicks(3, 7, 4)).toEqual([3, 4, 5, 6, 7]);
		expect(niceTicks(0, 0)).toEqual([0, 1]);
		expect(niceTicks(-12, 30)).toEqual([-20, -10, 0, 10, 20, 30]);
		expect(niceTicks(0, 1_240_000)).toEqual([0, 250_000, 500_000, 750_000, 1_000_000, 1_250_000]);
	});
	it('reads ISO dates as time', () => {
		expect(numericX(['2026-11-01', '2026-11-02'])).toEqual([Date.UTC(2026, 10, 1), Date.UTC(2026, 10, 2)]);
		expect(numericX(['Mon', 'Tue'])).toBeNull();
	});
	it('spaces time ticks to fit the span', () => {
		const days = timeTicks(Date.UTC(2026, 10, 1), Date.UTC(2026, 10, 30));
		expect(days.ticks.length).toBeLessThanOrEqual(6);
		const hours = timeTicks(Date.UTC(2026, 10, 1), Date.UTC(2026, 10, 1, 12));
		expect(hours.ticks.length).toBeGreaterThan(2);
	});
	it('compacts big numbers only', () => {
		expect(formatNumber(1284)).toBe('1,284');
		expect(formatNumber(12_900)).toBe('12.9K');
	});
});

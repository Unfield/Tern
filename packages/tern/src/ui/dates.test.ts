import { describe, expect, it } from 'vitest';
import { addDays, addMonths, monthGrid } from './dates.js';

describe('dates', () => {
	it('adds days across months and years', () => {
		expect(addDays('2026-12-31', 1)).toBe('2027-01-01');
		expect(addDays('2026-03-01', -1)).toBe('2026-02-28');
		expect(addDays('2028-03-01', -1)).toBe('2028-02-29');
	});
	it('adds months, clamping the day', () => {
		expect(addMonths('2026-01-31', 1)).toBe('2026-02-28');
		expect(addMonths('2026-03-31', -1)).toBe('2026-02-28');
		expect(addMonths('2026-11-10', 12)).toBe('2027-11-10');
	});
	it('lays out a month starting on the given weekday', () => {
		// November 2026 starts on a Sunday.
		const sunday = monthGrid('2026-11-10', 0);
		expect(sunday[0]![0]).toBe('2026-11-01');
		expect(sunday.length).toBe(5);
		const monday = monthGrid('2026-11-10', 1);
		expect(monday[0]![0]).toBe('2026-10-26');
		expect(monday.at(-1)!.at(-1)).toBe('2026-12-06');
		expect(monday.length).toBe(6);
	});
});

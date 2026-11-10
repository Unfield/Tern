import { describe, expect, it } from 'vitest';
import { IntlShim } from './intl.js';

const shim = IntlShim as unknown as typeof Intl;

const values = [0, -0, 1, -1, 0.5, -0.5, 0.0004, -0.0004, 1.5, 12.345, 999, 1000, 1234, -1234, 9999, 12345, 99999, 123456, 999999, 1_000_000, 1_500_000, -2_340_000, 999_999_999, 12_000_000_000, 3.4e12, 0.12345];

// Loosely typed: TypeScript's lib doesn't know signDisplay "negative" yet.
function same(options: Record<string, unknown>) {
	const want = new Intl.NumberFormat('en-US', options as Intl.NumberFormatOptions);
	const got = new shim.NumberFormat('en-US', options as Intl.NumberFormatOptions);
	for (const v of values) {
		expect(got.format(v), `${v} with ${JSON.stringify(options)}`).toBe(want.format(v));
	}
}

describe('NumberFormat', () => {
	it('formats plain numbers', () => same({}));
	it('formats percentages', () => same({ style: 'percent' }));
	it('formats currency', () => same({ style: 'currency', currency: 'USD' }));
	for (const signDisplay of ['auto', 'always', 'exceptZero', 'never', 'negative'] as const) {
		it(`signDisplay ${signDisplay}`, () => {
			same({ signDisplay });
			same({ signDisplay, style: 'percent' });
			same({ signDisplay, style: 'currency', currency: 'USD' });
			same({ signDisplay, notation: 'compact' });
		});
	}
	it('compact notation', () => {
		same({ notation: 'compact' });
		same({ notation: 'compact', compactDisplay: 'short' });
		same({ notation: 'compact', compactDisplay: 'long' });
		same({ notation: 'compact', maximumFractionDigits: 2 });
	});
	it('infinity', () => {
		for (const signDisplay of ['auto', 'always', 'exceptZero', 'never'] as const) {
			const want = new Intl.NumberFormat('en-US', { signDisplay });
			const got = new shim.NumberFormat('en-US', { signDisplay });
			expect(got.format(Infinity)).toBe(want.format(Infinity));
			expect(got.format(-Infinity)).toBe(want.format(-Infinity));
		}
	});
});

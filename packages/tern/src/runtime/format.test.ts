import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';
import { format, parse } from './format.js';

// The same cases as the Go formatter in internal/i18n.
const cases = JSON.parse(
	readFileSync(fileURLToPath(new URL('../../../../internal/i18n/testdata/cases.json', import.meta.url)), 'utf8')
) as { locale: string; message: string; params: Record<string, unknown>; want: string }[];

describe('format', () => {
	for (const c of cases) {
		it(`${c.locale}: ${c.message}`, () => {
			expect(format(c.locale, c.message, c.params)).toBe(c.want);
		});
	}

	it('rejects malformed messages', () => {
		for (const msg of ['{', '{name', 'a}b', '{n, plural, one {x}}', '{n, number}', '{, plural, other {x}}', '{n, plural, other {x}']) {
			expect(() => parse(msg), msg).toThrow();
		}
	});
});

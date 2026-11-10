import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import { checkField, validate, type FormField } from './validate.js';

// The same cases internal/formrules checks in Go.
const data = JSON.parse(readFileSync(new URL('../../../../internal/formrules/testdata/cases.json', import.meta.url), 'utf8')) as {
	fields: Record<string, FormField>;
	cases: { field: string; values: string[]; other?: Record<string, string>; error: string }[];
};

describe('validate matches Go', () => {
	for (const c of data.cases) {
		it(`${c.field} ${JSON.stringify(c.values)}`, () => {
			const f = data.fields[c.field]!;
			expect(checkField(f, c.values, (n) => c.other?.[n] ?? '', (n) => data.fields[n]?.label ?? n)).toBe(c.error);
		});
	}
	it('reads FormData and objects', () => {
		const fields = [data.fields.email!, data.fields.tags!, data.fields.terms!];
		const fd = new FormData();
		fd.set('email', 'nope');
		fd.append('tags', 'go');
		expect(validate(fields, fd)).toEqual({ email: 'Email must be an email address.', terms: 'Terms must be checked.' });
		expect(validate(fields, { email: 'a@b.co', tags: ['go'], terms: true })).toEqual({});
	});
});

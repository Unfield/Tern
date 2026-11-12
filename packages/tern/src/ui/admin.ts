import type { FormField } from '../runtime/validate.js';

/** A field on an admin screen, from the admin package's Field. */
export type AdminField = {
	name: string;
	label: string;
	description: string;
	/** text, textarea, number, checkbox, date, datetime-local, email, url, password or select. */
	input: string;
	options: string[] | null;
	list: boolean;
	filter: boolean;
	readonly: boolean;
	/** The resource this field holds a key of. */
	ref: string;
	/** The field's rules, for the browser to check: a FormField. */
	rules: Omit<FormField, 'kind'> & { kind: string };
};

/** What the admin package's Load returns: admin.Page. */
export type AdminPage = {
	title: string;
	base: string;
	resources: { name: string; label: string; href: string }[];
	/** home, list or form. */
	view: string;
	resource: {
		name: string;
		label: string;
		singular: string;
		href: string;
		key: string;
		fields: AdminField[];
		search: boolean;
		create: boolean;
		edit: boolean;
		delete: boolean;
	} | null;
	list: {
		rows: { key: string; values: Record<string, string> }[];
		total: number;
		page: number;
		pages: number;
		perPage: number;
		search: string;
		sort: string;
		desc: boolean;
		filters: Record<string, string>;
	} | null;
	record: { key: string; values: Record<string, string> } | null;
	counts: number[];
	notice: string;
};

/** A value as a list shows it: dates without seconds, checkboxes as words. */
export function shown(f: AdminField, v: string | undefined, yes: string, no: string): string {
	if (f.input === 'checkbox') return v ? yes : no;
	if (!v) return '';
	if (f.input === 'datetime-local') return v.replace('T', ' ');
	return v;
}

/** The list's address with some of its query changed; page goes back to 1 unless it's set. */
export function listHref(base: string, list: NonNullable<AdminPage['list']>, change: Record<string, string | number | null>): string {
	const q = new URLSearchParams();
	if (list.search) q.set('q', list.search);
	if (list.sort) q.set('sort', (list.desc ? '-' : '') + list.sort);
	for (const [k, v] of Object.entries(list.filters ?? {})) q.set(k, v);
	for (const [k, v] of Object.entries(change)) {
		if (v === null || v === '') q.delete(k);
		else q.set(k, String(v));
	}
	if (!('page' in change)) q.delete('page');
	if (q.get('page') === '1') q.delete('page');
	const s = q.toString();
	return s ? `${base}?${s}` : base;
}

// Checks form values against the rules tern gen writes to ./$forms from a
// kit.FormAction's validate tags. The checks and messages match
// internal/formrules in Go, which checks the same rules on the server;
// internal/formrules/testdata/cases.json keeps the two in step.

/** A form field's rules, as tern gen writes them. */
export interface FormField<Name extends string = string> {
	name: Name;
	label: string;
	kind: 'string' | 'int' | 'float' | 'bool' | 'date' | 'datetime' | 'strings';
	required?: boolean;
	rules?: { name: string; arg?: string }[];
	message?: string;
	secret?: boolean;
}

/** Errors by field name; empty when the form is valid. */
export type FormErrors<Name extends string = string> = Partial<Record<Name, string>>;

const emailRe = /^[^\s@<>()",;:]+@[^\s@<>()",;:]+\.[^\s@<>()",;:]+$/;
const intRe = /^[+-]?\d+$/;
const floatRe = /^[+-]?(\d+\.?\d*|\.\d+)$/;
const patterns = new Map<string, RegExp>();

function truthy(s: string) {
	return !['', 'off', 'false', '0', 'no'].includes(s.trim().toLowerCase());
}

/** Reads a date or a date and time as Go does, or null. Times without a zone are UTC. */
export function parseFormTime(kind: 'date' | 'datetime', s: string): number | null {
	if (kind === 'date') {
		const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(s);
		if (!m) return null;
		const [y, mo, d] = [Number(m[1]), Number(m[2]), Number(m[3])];
		const t = Date.UTC(y, mo - 1, d);
		const back = new Date(t);
		return back.getUTCFullYear() === y && back.getUTCMonth() === mo - 1 && back.getUTCDate() === d ? t : null;
	}
	const m = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})(?::(\d{2}))?$/.exec(s);
	if (m) {
		const [y, mo, d, h, mi, se] = m.slice(1).map((x) => Number(x ?? 0)) as number[];
		const t = Date.UTC(y!, mo! - 1, d!, h!, mi!, se!);
		const b = new Date(t);
		return b.getUTCDate() === d && b.getUTCHours() === h && b.getUTCMinutes() === mi ? t : null;
	}
	if (/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})$/.test(s)) {
		const t = Date.parse(s);
		return Number.isNaN(t) ? null : t;
	}
	return null;
}

function joinOr(list: string[]) {
	return list.length === 1 ? list[0]! : `${list.slice(0, -1).join(', ')} or ${list[list.length - 1]}`;
}

const bound = { min: 'at least', max: 'at most', len: 'exactly' } as Record<string, string>;

/**
 * Returns why a field's values are invalid, or "". values are its posted
 * values; other reads another field's first value, for same=.
 */
export function checkField(f: FormField, values: string[], other: (name: string) => string = () => '', labelOf: (name: string) => string = (n) => n): string {
	const msg = (m: string) => f.message || m;
	const list = values.filter((v) => v.trim() !== '');
	const v = values[0] ?? '';
	const empty = f.kind === 'strings' ? list.length === 0 : f.kind === 'bool' ? !truthy(v) : v.trim() === '';
	if (empty) {
		if (!f.required) return '';
		return msg(f.kind === 'bool' ? `${f.label} must be checked.` : `${f.label} is required.`);
	}
	let num = 0;
	let when = 0;
	if (f.kind === 'int') {
		if (!intRe.test(v.trim())) return msg(`${f.label} must be a whole number.`);
		num = Number(v.trim());
	} else if (f.kind === 'float') {
		if (!floatRe.test(v.trim())) return msg(`${f.label} must be a number.`);
		num = Number(v.trim());
	} else if (f.kind === 'date' || f.kind === 'datetime') {
		const t = parseFormTime(f.kind, v.trim());
		if (t === null) return msg(f.kind === 'date' ? `${f.label} must be a date.` : `${f.label} must be a date and time.`);
		when = t;
	}
	for (const r of f.rules ?? []) {
		const arg = r.arg ?? '';
		const n = Number(arg);
		switch (r.name) {
			case 'min':
			case 'max':
			case 'len': {
				const low = r.name === 'min';
				const fails = (c: number) => (r.name === 'len' && c !== n) || (low && c < n) || (r.name === 'max' && c > n);
				if (f.kind === 'string') {
					if (fails([...v].length)) return msg(`${f.label} must be ${bound[r.name]} ${arg} ${n === 1 ? 'character' : 'characters'}.`);
				} else if (f.kind === 'int' || f.kind === 'float') {
					if (fails(num)) return msg(`${f.label} must be ${bound[r.name]} ${arg}.`);
				} else if (f.kind === 'date' || f.kind === 'datetime') {
					const b = parseFormTime(f.kind, arg) ?? 0;
					if ((low && when < b) || (r.name === 'max' && when > b)) return msg(`${f.label} must be on or ${low ? 'after' : 'before'} ${arg}.`);
				} else if (f.kind === 'strings') {
					if (fails(list.length)) return msg(`Choose ${bound[r.name]} ${arg} for ${f.label.toLowerCase()}.`);
				}
				break;
			}
			case 'email':
				if (!emailRe.test(v)) return msg(`${f.label} must be an email address.`);
				break;
			case 'url': {
				let ok = false;
				try {
					const u = new URL(v);
					ok = (u.protocol === 'http:' || u.protocol === 'https:') && u.host !== '' && !/[ \t\n]/.test(v);
				} catch {
					ok = false;
				}
				if (!ok) return msg(`${f.label} must be a web address starting with http:// or https://.`);
				break;
			}
			case 'pattern': {
				let re = patterns.get(arg);
				if (!re) patterns.set(arg, (re = new RegExp(`^(?:${arg})$`, 'u')));
				for (const s of f.kind === 'strings' ? list : [v]) if (!re.test(s)) return msg(`${f.label} isn't in the expected format.`);
				break;
			}
			case 'oneof': {
				const opts = arg.split('|');
				for (const s of f.kind === 'strings' ? list : [v]) if (!opts.includes(s)) return msg(`${f.label} must be one of ${joinOr(opts)}.`);
				break;
			}
			case 'same':
				if (other(arg) !== v) return msg(`${f.label} must match ${labelOf(arg).toLowerCase()}.`);
				break;
		}
	}
	return '';
}

/**
 * Checks a form against its rules, as kit.FormAction will on the server:
 *
 * ```ts
 * import { forms } from './$forms';
 * const errors = validate(forms.create, new FormData(formElement));
 * ```
 */
export function validate<Name extends string>(fields: FormField<Name>[], data: FormData | Record<string, unknown>): FormErrors<Name> {
	const read = (name: string): string[] => {
		if (data instanceof FormData) return data.getAll(name).map((v) => (typeof v === 'string' ? v : v.name));
		const v = data[name];
		if (v === undefined || v === null || v === false) return [];
		if (v === true) return ['on'];
		return Array.isArray(v) ? v.map(String) : [String(v)];
	};
	const labels = new Map(fields.map((f) => [f.name, f.label]));
	const errors: FormErrors<Name> = {};
	for (const f of fields) {
		const m = checkField(f, read(f.name), (n) => read(n)[0] ?? '', (n) => labels.get(n as Name) ?? n);
		if (m) errors[f.name] = m;
	}
	return errors;
}

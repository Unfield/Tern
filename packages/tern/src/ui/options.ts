/** An option in a Combobox or SelectMenu. */
export type Option = {
	value: string;
	label: string;
	/** A second line, such as an email under a name. */
	description?: string;
	disabled?: boolean;
	/** Groups options under this heading. */
	group?: string;
};

/** Matches when every word of the query starts a word of the text, or, from three letters, appears anywhere in it. Ignores case and accents. */
export function matches(text: string, query: string): boolean {
	const norm = (s: string) => s.normalize('NFD').replace(/\p{Diacritic}/gu, '').toLowerCase();
	const words = norm(text).split(/[\s\-_/.@]+/);
	const hay = norm(text);
	return norm(query)
		.split(/\s+/)
		.filter(Boolean)
		.every((q) => words.some((w) => w.startsWith(q)) || (q.length >= 3 && hay.includes(q)));
}

/** Splits options into groups, keeping their order. */
export function grouped(options: Option[]): { group?: string; options: Option[] }[] {
	const out: { group?: string; options: Option[] }[] = [];
	for (const o of options) {
		const last = out[out.length - 1];
		if (last && last.group === o.group) last.options.push(o);
		else out.push({ group: o.group, options: [o] });
	}
	return out;
}

// Dates on the account screens, in the user's locale.
import { i18n } from '../runtime/i18n.svelte.js';

export function when(iso: string | null | undefined): string {
	if (!iso) return '';
	const d = new Date(iso);
	if (Number.isNaN(d.getTime())) return '';
	try {
		return new Intl.DateTimeFormat(i18n.locale || undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(d);
	} catch {
		return d.toISOString().slice(0, 16).replace('T', ' ');
	}
}

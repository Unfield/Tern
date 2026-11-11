// Shared by the billing screens. Internal: not exported.
import { i18n, tb } from '../runtime/i18n.svelte.js';

export type Interval = 'month' | 'year';

/** A plan, as billing.Plan marshals it. */
export type BillingPlan = {
	id: string;
	name: string;
	description: string;
	paid: boolean;
	intervals: string[];
	amounts?: Record<string, number> | null;
	currency?: string;
	limits?: Record<string, number> | null;
	features?: string[] | null;
	trialDays?: number;
	perSeat?: boolean;
	highlight?: boolean;
};

/** A subscription, as billing.Subscription marshals it. */
export type BillingSubscription = {
	plan: string;
	status: string;
	interval: string;
	seats: number;
	currentPeriodEnd: string;
	cancelAtPeriodEnd: boolean;
	trialEnd: string | null;
};

/** An amount in the currency's smallest unit, as the user's locale writes it. */
export function money(amount: number, currency = 'usd'): string {
	const code = currency.toUpperCase();
	try {
		const f = new Intl.NumberFormat(i18n.locale || undefined, { style: 'currency', currency: code });
		const digits = f.resolvedOptions().maximumFractionDigits ?? 2;
		const value = amount / 10 ** digits;
		// Whole amounts without the cents: $19, not $19.00.
		return new Intl.NumberFormat(i18n.locale || undefined, { style: 'currency', currency: code, maximumFractionDigits: Number.isInteger(value) ? 0 : digits }).format(value);
	} catch {
		return `${(amount / 100).toFixed(2)} ${code}`;
	}
}

/** A date, as the user's locale writes it. */
export function day(iso: string | null | undefined): string {
	if (!iso) return '';
	const d = new Date(iso);
	if (Number.isNaN(d.getTime())) return '';
	try {
		return new Intl.DateTimeFormat(i18n.locale || undefined, { dateStyle: 'medium' }).format(d);
	} catch {
		return d.toISOString().slice(0, 10);
	}
}

export function per(interval: string): string {
	return tb(interval === 'year' ? 'tern.ui.billing.perYear' : 'tern.ui.billing.perMonth');
}

// Calendar dates as ISO strings ("2026-11-10"), done in UTC so time zones
// and daylight saving never move a day.

export type ISODate = string;

export function parse(iso: ISODate): Date {
	const [y, m, d] = iso.split('-').map(Number);
	return new Date(Date.UTC(y!, (m ?? 1) - 1, d ?? 1));
}

export function iso(d: Date): ISODate {
	return d.toISOString().slice(0, 10);
}

export function today(): ISODate {
	const n = new Date();
	return iso(new Date(Date.UTC(n.getFullYear(), n.getMonth(), n.getDate())));
}

export function addDays(date: ISODate, n: number): ISODate {
	const d = parse(date);
	d.setUTCDate(d.getUTCDate() + n);
	return iso(d);
}

/** Adds months, keeping the day where the month has it, else its last day. */
export function addMonths(date: ISODate, n: number): ISODate {
	const d = parse(date);
	const day = d.getUTCDate();
	d.setUTCDate(1);
	d.setUTCMonth(d.getUTCMonth() + n);
	const last = new Date(Date.UTC(d.getUTCFullYear(), d.getUTCMonth() + 1, 0)).getUTCDate();
	d.setUTCDate(Math.min(day, last));
	return iso(d);
}

export function startOfMonth(date: ISODate): ISODate {
	return date.slice(0, 8) + '01';
}

/** The weeks shown for date's month, each 7 ISO dates, starting on weekStart (0 = Sunday). */
export function monthGrid(date: ISODate, weekStart: number): ISODate[][] {
	const first = parse(startOfMonth(date));
	const lead = (first.getUTCDay() - weekStart + 7) % 7;
	let day = addDays(iso(first), -lead);
	const weeks: ISODate[][] = [];
	const month = date.slice(0, 7);
	do {
		const week: ISODate[] = [];
		for (let i = 0; i < 7; i++) {
			week.push(day);
			day = addDays(day, 1);
		}
		weeks.push(week);
	} while (day.slice(0, 7) === month);
	return weeks;
}

/** The first day of the week for a locale, from Intl where the browser knows it. */
export function localeWeekStart(locale?: string): number {
	try {
		const loc = new Intl.Locale(locale ?? navigator.language) as Intl.Locale & { getWeekInfo?: () => { firstDay: number }; weekInfo?: { firstDay: number } };
		const info = loc.getWeekInfo?.() ?? loc.weekInfo;
		if (info) return info.firstDay % 7;
		return ['US', 'CA', 'JP', 'BR', 'IL', 'MX', 'PH', 'KR', 'TW', 'IN'].includes(loc.maximize().region ?? '') ? 0 : 1;
	} catch {
		return 1;
	}
}

export function format(date: ISODate, locale: string | undefined, options: Intl.DateTimeFormatOptions): string {
	return new Intl.DateTimeFormat(locale, { timeZone: 'UTC', ...options }).format(parse(date));
}

// Scales, ticks and formatting shared by the charts.

export type XValue = number | string | Date;
export type Series<Row> = {
	/** The field holding this series' numbers. */
	key: keyof Row & string;
	label: string;
};

/** The series colors, in their fixed order. A ninth series folds into "Other". */
export const seriesColor = (i: number) => `var(--tern-chart-${(i % 8) + 1})`;

const isoDate = /^\d{4}-\d{2}-\d{2}([T ][\d:.]+(Z|[+-]\d{2}:?\d{2})?)?$/;

/** Turns x values into numbers for time and number axes, or null for categories. */
export function numericX(values: XValue[]): number[] | null {
	if (values.every((v) => typeof v === 'number')) return values as number[];
	if (values.every((v) => v instanceof Date)) return (values as Date[]).map((d) => d.getTime());
	if (values.every((v) => typeof v === 'string' && isoDate.test(v))) return (values as string[]).map((s) => Date.parse(s.length === 10 ? s + 'T00:00:00Z' : s));
	return null;
}

export const isTime = (values: XValue[]) => values.length > 0 && !values.every((v) => typeof v === 'number') && numericX(values) !== null;

/** Round ticks covering [min, max], about count of them. */
export function niceTicks(min: number, max: number, count = 5): number[] {
	if (min === max) {
		if (min === 0) return [0, 1];
		[min, max] = min > 0 ? [0, min] : [min, 0];
	}
	const span = max - min;
	const raw = span / Math.max(1, count);
	const mag = 10 ** Math.floor(Math.log10(raw));
	const step = [1, 2, 2.5, 5, 10].map((m) => m * mag).find((s) => span / s <= count) ?? 10 * mag;
	const start = Math.floor(min / step) * step;
	const end = Math.ceil(max / step) * step;
	const out: number[] = [];
	for (let v = start; v <= end + step / 2; v += step) out.push(Number(v.toFixed(10)));
	return out;
}

/** 1,284 in full under 10,000; 12K, 4.2M above. */
export function formatNumber(n: number, compact = Math.abs(n) >= 10_000): string {
	return new Intl.NumberFormat(undefined, compact ? { notation: 'compact', maximumFractionDigits: 1 } : { maximumFractionDigits: 2 }).format(n);
}

const DAY = 86_400_000;

/** Evenly spaced time ticks with a format that fits their spacing. */
export function timeTicks(min: number, max: number, count = 6): { ticks: number[]; format: (t: number) => string } {
	const span = max - min;
	const fmt = (o: Intl.DateTimeFormatOptions) => {
		const f = new Intl.DateTimeFormat(undefined, { timeZone: 'UTC', ...o });
		return (t: number) => f.format(new Date(t));
	};
	if (span <= 2 * DAY) {
		const step = [1, 2, 3, 6, 12].map((h) => h * 3_600_000).find((s) => span / s <= count) ?? 24 * 3_600_000;
		return { ticks: stepTicks(min, max, step), format: fmt({ hour: '2-digit', minute: '2-digit' }) };
	}
	if (span <= 120 * DAY) {
		const step = [1, 2, 7, 14].map((d) => d * DAY).find((s) => span / s <= count) ?? 28 * DAY;
		return { ticks: stepTicks(min, max, step), format: fmt({ month: 'short', day: 'numeric' }) };
	}
	const months: number[] = [];
	const d = new Date(min);
	d.setUTCDate(1);
	d.setUTCHours(0, 0, 0, 0);
	const every = Math.max(1, Math.ceil(span / (30 * DAY) / count));
	while (d.getTime() <= max) {
		if (d.getTime() >= min) months.push(d.getTime());
		d.setUTCMonth(d.getUTCMonth() + every);
	}
	return { ticks: months, format: fmt(span > 400 * DAY ? { month: 'short', year: 'numeric' } : { month: 'short' }) };
}

function stepTicks(min: number, max: number, step: number) {
	const out: number[] = [];
	for (let t = Math.ceil(min / step) * step; t <= max; t += step) out.push(t);
	return out;
}

/** A readable label for an x value in tooltips and tables. */
export function formatX(v: XValue, time: boolean): string {
	if (!time) return v instanceof Date ? v.toISOString() : String(v);
	const t = v instanceof Date ? v.getTime() : typeof v === 'number' ? v : Date.parse(v.length === 10 ? v + 'T00:00:00Z' : v);
	const d = new Date(t);
	const hasTime = d.getUTCHours() || d.getUTCMinutes();
	return new Intl.DateTimeFormat(undefined, { timeZone: 'UTC', dateStyle: 'medium', ...(hasTime ? { timeStyle: 'short' } : {}) }).format(d);
}

/** An estimate of how wide a label is, for margins; the charts' text is 12px. */
export const textWidth = (s: string) => s.length * 6.6;

/** Array.prototype.findLastIndex, for older targets. */
export function lastIndex<T>(a: T[], ok: (v: T) => boolean): number {
	for (let i = a.length - 1; i >= 0; i--) if (ok(a[i]!)) return i;
	return -1;
}

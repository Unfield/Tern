// A small Intl for the server renderer. The embedded engine (QuickJS) has
// no Intl, and many libraries create formatters when they load. This shim
// formats like en-US, which is good enough for server-rendered HTML: the
// browser hydrates with the user's real Intl and locale.

type Opts = Record<string, unknown>;

const MONTHS = ['January', 'February', 'March', 'April', 'May', 'June', 'July', 'August', 'September', 'October', 'November', 'December'];
const DAYS = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'];

function group(int: string): string {
	return int.replace(/\B(?=(\d{3})+(?!\d))/g, ',');
}

class NumberFormat {
	#o: Opts;
	constructor(_locale?: unknown, options: Opts = {}) {
		this.#o = options;
	}
	format(value: number | bigint): string {
		let n = Number(value);
		const o = this.#o;
		const sign = String(o.signDisplay ?? 'auto');
		if (!Number.isFinite(n)) {
			if (Number.isNaN(n)) return 'NaN';
			return (n < 0 ? (sign === 'never' ? '' : '-') : sign === 'always' || sign === 'exceptZero' ? '+' : '') + '∞';
		}
		const negative = n < 0 || Object.is(n, -0);
		let suffix = '';
		if (o.style === 'percent') {
			n *= 100;
			suffix = '%';
		}
		const isCurrency = o.style === 'currency';
		const explicit = o.maximumFractionDigits !== undefined || o.minimumFractionDigits !== undefined;
		let max = (o.maximumFractionDigits as number | undefined) ?? (isCurrency ? 2 : o.style === 'percent' ? 0 : 3);
		const min = (o.minimumFractionDigits as number | undefined) ?? (isCurrency ? 2 : 0);
		let abs = Math.abs(n);
		if (o.notation === 'compact') {
			const long = o.compactDisplay === 'long';
			const units = [
				[1e12, 'T', ' trillion'],
				[1e9, 'B', ' billion'],
				[1e6, 'M', ' million'],
				[1e3, 'K', ' thousand'],
			] as const;
			// Like en-US: two significant digits, but never fewer than the
			// integer part needs.
			const digits = (x: number) => (explicit ? Math.max(min, Math.min(max, 20)) : x >= 10 || x === 0 ? 0 : Math.max(0, 1 - Math.floor(Math.log10(x))));
			const rounded = (x: number) => Number(x.toFixed(digits(x)));
			let unit = '';
			for (let i = 0; i < units.length; i++) {
				const [limit, short, name] = units[i]!;
				if (rounded(abs) >= limit) {
					abs /= limit;
					unit = long ? name : short;
					// 999,999 rounds to 1000K, which is 1M.
					if (rounded(abs) >= 1000 && i > 0) {
						abs /= 1000;
						unit = long ? units[i - 1]![2] : units[i - 1]![1];
					}
					break;
				}
			}
			suffix = unit + suffix;
			max = digits(abs);
		}
		let s = abs.toFixed(o.notation === 'compact' && !explicit ? max : Math.max(min, Math.min(max, 20)));
		if (s.includes('.')) {
			let [int, frac = ''] = s.split('.');
			while (frac.length > min && frac.endsWith('0')) frac = frac.slice(0, -1);
			s = frac ? `${int}.${frac}` : int!;
		}
		const zero = !/[1-9]/.test(s);
		const [int, frac] = s.split('.');
		s = (o.useGrouping === false ? int! : group(int!)) + (frac ? `.${frac}` : '');
		if (isCurrency) {
			const code = String(o.currency ?? 'USD').toUpperCase();
			s = (code === 'USD' ? '$' : code === 'EUR' ? '€' : code === 'GBP' ? '£' : `${code} `) + s;
		}
		let prefix = '';
		switch (sign) {
			case 'never':
				break;
			case 'always':
				prefix = negative ? '-' : '+';
				break;
			case 'exceptZero':
				prefix = zero ? '' : negative ? '-' : '+';
				break;
			case 'negative':
				prefix = negative && !zero ? '-' : '';
				break;
			default:
				prefix = negative ? '-' : '';
		}
		return prefix + s + suffix;
	}
	formatToParts(value: number) {
		return [{ type: 'literal', value: this.format(value) }];
	}
	resolvedOptions() {
		return { locale: 'en-US', numberingSystem: 'latn', ...this.#o };
	}
	static supportedLocalesOf() {
		return ['en-US'];
	}
}

class DateTimeFormat {
	#o: Opts;
	constructor(_locale?: unknown, options: Opts = {}) {
		this.#o = options;
	}
	format(value?: Date | number): string {
		const d = value === undefined ? new Date() : new Date(value);
		if (Number.isNaN(d.getTime())) return 'Invalid Date';
		const o = this.#o;
		const utc = o.timeZone === 'UTC';
		const get = {
			y: utc ? d.getUTCFullYear() : d.getFullYear(),
			m: utc ? d.getUTCMonth() : d.getMonth(),
			d: utc ? d.getUTCDate() : d.getDate(),
			w: utc ? d.getUTCDay() : d.getDay(),
			h: utc ? d.getUTCHours() : d.getHours(),
			min: utc ? d.getUTCMinutes() : d.getMinutes()
		};
		const hasDate = o.year || o.month || o.day || o.weekday || o.dateStyle;
		const hasTime = o.hour || o.minute || o.timeStyle;
		const parts: string[] = [];
		if (hasDate || !hasTime) {
			const month = o.month ?? (o.dateStyle ? 'short' : 'numeric');
			if (o.weekday) parts.push((o.weekday === 'long' ? DAYS[get.w] : DAYS[get.w]!.slice(0, 3))! + ',');
			if (month === 'numeric' || month === '2-digit') {
				parts.push(`${get.m + 1}/${get.d}/${get.y}`);
			} else {
				const name = month === 'long' ? MONTHS[get.m]! : MONTHS[get.m]!.slice(0, 3);
				const day = o.day !== undefined || o.dateStyle ? ` ${get.d}` : '';
				const year = o.year !== undefined || o.dateStyle ? `, ${get.y}` : '';
				parts.push(name + day + (day ? year : year.replace(', ', ' ')));
			}
		}
		if (hasTime) {
			const h12 = o.hour12 !== false;
			const hour = h12 ? get.h % 12 || 12 : get.h;
			const mm = String(get.min).padStart(2, '0');
			parts.push(`${hour}:${mm}${h12 ? (get.h < 12 ? ' AM' : ' PM') : ''}`);
		}
		return parts.join(' ');
	}
	formatToParts(value?: Date | number) {
		return [{ type: 'literal', value: this.format(value) }];
	}
	formatRange(a: Date | number, b: Date | number) {
		return `${this.format(a)} – ${this.format(b)}`;
	}
	resolvedOptions() {
		return { locale: 'en-US', calendar: 'gregory', numberingSystem: 'latn', timeZone: 'UTC', ...this.#o };
	}
	static supportedLocalesOf() {
		return ['en-US'];
	}
}

class PluralRules {
	select(n: number) {
		return n === 1 ? 'one' : 'other';
	}
	resolvedOptions() {
		return { locale: 'en-US', pluralCategories: ['one', 'other'] };
	}
	static supportedLocalesOf() {
		return ['en-US'];
	}
}

class Collator {
	#numeric: boolean;
	constructor(_locale?: unknown, options: Opts = {}) {
		this.#numeric = options.numeric === true;
	}
	compare = (a: string, b: string): number => {
		if (this.#numeric) {
			const na = Number(a);
			const nb = Number(b);
			if (!Number.isNaN(na) && !Number.isNaN(nb)) return na - nb;
		}
		const x = String(a).toLowerCase();
		const y = String(b).toLowerCase();
		return x < y ? -1 : x > y ? 1 : 0;
	};
	resolvedOptions() {
		return { locale: 'en-US', usage: 'sort', sensitivity: 'variant' };
	}
	static supportedLocalesOf() {
		return ['en-US'];
	}
}

class RelativeTimeFormat {
	format(value: number, unit: string) {
		const u = unit.replace(/s$/, '');
		const n = Math.abs(value);
		const label = `${n} ${u}${n === 1 ? '' : 's'}`;
		return value < 0 ? `${label} ago` : `in ${label}`;
	}
	static supportedLocalesOf() {
		return ['en-US'];
	}
}

class ListFormat {
	#type: string;
	constructor(_locale?: unknown, options: Opts = {}) {
		this.#type = options.type === 'disjunction' ? 'or' : 'and';
	}
	format(list: Iterable<string>) {
		const items = [...list];
		if (items.length < 3) return items.join(` ${this.#type} `);
		return `${items.slice(0, -1).join(', ')}, ${this.#type} ${items[items.length - 1]}`;
	}
	static supportedLocalesOf() {
		return ['en-US'];
	}
}

class Locale {
	baseName: string;
	language: string;
	region?: string;
	constructor(tag: string = 'en-US') {
		this.baseName = String(tag);
		const [language, region] = this.baseName.split(/[-_]/);
		this.language = language || 'en';
		this.region = region;
	}
	get weekInfo() {
		return this.getWeekInfo();
	}
	getWeekInfo() {
		return { firstDay: 7, weekend: [6, 7], minimalDays: 1 };
	}
	maximize() {
		return this;
	}
	minimize() {
		return this;
	}
	toString() {
		return this.baseName;
	}
}

class Segmenter {
	segment(input: string) {
		return [...input].map((segment, index) => ({ segment, index, input }));
	}
}

export const IntlShim = {
	NumberFormat,
	DateTimeFormat,
	PluralRules,
	Collator,
	RelativeTimeFormat,
	ListFormat,
	Locale,
	Segmenter,
	getCanonicalLocales: (l?: string | string[]) => (l === undefined ? [] : Array.isArray(l) ? l : [l]),
	supportedValuesOf: () => []
};

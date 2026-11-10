export type Status = 'up' | 'down' | 'pending' | 'paused';

export const statusLabel: Record<string, string> = {
	up: 'Up',
	down: 'Down',
	pending: 'Pending',
	paused: 'Paused'
};

/** Formats an uptime ratio (0 to 1, or -1 for no data) as a percentage. */
export function percent(ratio: number | undefined, digits = 2): string {
	if (ratio === undefined || ratio < 0) return '–';
	if (ratio === 1) return '100%';
	return `${(Math.floor(ratio * 10 ** (digits + 2)) / 10 ** digits).toFixed(digits)}%`;
}

export function ms(v: number | null | undefined): string {
	return v === null || v === undefined ? '–' : `${v} ms`;
}

/**
 * A time relative to now, such as "3m ago". now is passed in so the server
 * render and hydration agree; pages tick it forward in the browser.
 */
export function ago(at: number | null | undefined, now: number): string {
	if (!at) return 'never';
	const s = Math.max(0, Math.round((now - at) / 1000));
	if (s < 60) return `${s}s ago`;
	if (s < 3600) return `${Math.floor(s / 60)}m ago`;
	if (s < 86400) return `${Math.floor(s / 3600)}h ago`;
	return `${Math.floor(s / 86400)}d ago`;
}

/** Formats a timestamp in UTC, so server and browser render the same text. */
export function datetime(at: number): string {
	const d = new Date(at);
	return `${d.toISOString().slice(0, 10)} ${d.toISOString().slice(11, 16)} UTC`;
}

export function date(at: number): string {
	return new Date(at).toISOString().slice(0, 10);
}

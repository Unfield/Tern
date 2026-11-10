// Toasts: short messages that confirm what happened, shown by <Toaster />.

export type Tone = 'neutral' | 'success' | 'danger' | 'warning';

export interface Toast {
	id: number;
	message: string;
	description?: string;
	tone: Tone;
	/** How long it stays, in milliseconds; 0 keeps it until dismissed. */
	duration: number;
}

let next = 1;
export const toasts = $state<Toast[]>([]);

/** Shows a toast and returns its id. */
export function toast(message: string, opts: Partial<Omit<Toast, 'id' | 'message'>> = {}): number {
	const t: Toast = { id: next++, message, tone: 'neutral', duration: opts.tone === 'danger' ? 8000 : 5000, ...opts };
	toasts.push(t);
	return t.id;
}

/** Removes a toast. */
export function dismiss(id: number) {
	const i = toasts.findIndex((t) => t.id === id);
	if (i >= 0) toasts.splice(i, 1);
}

toast.success = (message: string, opts?: Partial<Omit<Toast, 'id' | 'message' | 'tone'>>) => toast(message, { ...opts, tone: 'success' });
toast.error = (message: string, opts?: Partial<Omit<Toast, 'id' | 'message' | 'tone'>>) => toast(message, { ...opts, tone: 'danger' });
toast.warning = (message: string, opts?: Partial<Omit<Toast, 'id' | 'message' | 'tone'>>) => toast(message, { ...opts, tone: 'warning' });

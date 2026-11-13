// The app's native features, when it runs in the phone app (mobile/ios);
// in a browser they do nothing, so the same pages work in both.

type Message = { method: string; [key: string]: unknown };

interface Bridge {
	postMessage(message: Message): Promise<unknown>;
}

function bridge(): Bridge | undefined {
	if (typeof window === 'undefined') return undefined;
	return (window as unknown as { webkit?: { messageHandlers?: { tern?: Bridge } } }).webkit?.messageHandlers?.tern;
}

/** Whether the page is in the phone app. */
export function available(): boolean {
	return bridge() !== undefined;
}

export type Haptic = 'light' | 'medium' | 'heavy' | 'success' | 'warning' | 'error';

/** Taps the phone: a light tap, or the feel of a success, warning or error. */
export async function haptic(style: Haptic = 'light'): Promise<void> {
	await bridge()?.postMessage({ method: 'haptic', style });
}

/** Small secrets, such as tokens, in the phone's Keychain. */
export const store = {
	async get(key: string): Promise<string | null> {
		const b = bridge();
		return b ? ((await b.postMessage({ method: 'store.get', key })) as string | null) : null;
	},
	async set(key: string, value: string | null): Promise<void> {
		await bridge()?.postMessage({ method: 'store.set', key, value });
	}
};

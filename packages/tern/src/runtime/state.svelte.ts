import type { Navigation, Payload } from './types.js';

class PageState {
	/** The current URL. */
	url = $state(new URL('http://localhost/'));
	/** The current route's parameters. */
	params = $state<Record<string, string>>({});
	/** The current route. `id` is null when no route matched. */
	route = $state<{ id: string | null }>({ id: null });
	/** The HTTP status of the current page. */
	status = $state(200);
	/** The error being shown, or null. */
	error = $state<{ message: string } | null>(null);
	/** The merged data of every layout and the page. */
	data = $state<Record<string, unknown>>({});
	/** The result of the last form action, or null. */
	form = $state<unknown>(null);
	/** State stored with the current history entry (see goto's `state`). */
	state = $state<Record<string, unknown>>({});
}

/**
 * Reactive information about the current page. Read it in components or
 * in `$derived`/`$effect`; it updates on every navigation.
 *
 * ```svelte
 * <script>
 *   import { page } from '@ternkit/tern';
 * </script>
 *
 * <a href="/" aria-current={page.url.pathname === '/' ? 'page' : undefined}>Home</a>
 * ```
 */
export const page = new PageState();

class NavigatingState {
	current = $state<Navigation | null>(null);
}

const navigatingState = new NavigatingState();

/**
 * The navigation in progress, or `null`. Use it to show a loading
 * indicator:
 *
 * ```svelte
 * {#if navigating.current}<Spinner />{/if}
 * ```
 */
export const navigating = {
	get current(): Navigation | null {
		return navigatingState.current;
	}
};

export function setNavigating(n: Navigation | null) {
	navigatingState.current = n;
}

/**
 * The server's PUBLIC_ environment variables, read when it started:
 *
 * ```js
 * import { env } from '@ternkit/tern';
 * fetch(`${env.PUBLIC_API_URL}/status`);
 * ```
 *
 * Only variables whose names start with PUBLIC_ are sent to the browser.
 */
export const env: Record<string, string | undefined> = {};

/** Updates page state for a rendered payload. */
export function applyPayloadToPage(p: Payload, url: URL, state: Record<string, unknown>, data: Record<string, unknown>[]) {
	if (p.env) {
		for (const k of Object.keys(env)) delete env[k];
		Object.assign(env, p.env);
	}
	page.url = url;
	page.params = p.params ?? {};
	page.route = { id: p.route ?? null };
	page.status = p.status;
	page.error = p.type === 'error' ? (p.error ?? { message: 'Error' }) : null;
	page.data = data[data.length - 1] ?? {};
	page.state = state;
	page.form = p.hasForm ? (p.form ?? null) : null;
}

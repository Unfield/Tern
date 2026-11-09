class PageState {
    /** The current URL. */
    url = $state(new URL('http://localhost/'));
    /** The current route's parameters. */
    params = $state({});
    /** The current route. `id` is null when no route matched. */
    route = $state({ id: null });
    /** The HTTP status of the current page. */
    status = $state(200);
    /** The error being shown, or null. */
    error = $state(null);
    /** The merged data of every layout and the page. */
    data = $state({});
    /** The result of the last form action, or null. */
    form = $state(null);
    /** State stored with the current history entry (see goto's `state`). */
    state = $state({});
}
/**
 * Reactive information about the current page. Read it in components or
 * in `$derived`/`$effect`; it updates on every navigation.
 *
 * ```svelte
 * <script>
 *   import { page } from '@unfield/tern';
 * </script>
 *
 * <a href="/" aria-current={page.url.pathname === '/' ? 'page' : undefined}>Home</a>
 * ```
 */
export const page = new PageState();
class NavigatingState {
    current = $state(null);
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
    get current() {
        return navigatingState.current;
    }
};
export function setNavigating(n) {
    navigatingState.current = n;
}
/** Updates page state for a rendered payload. */
export function applyPayloadToPage(p, url, state, data) {
    page.url = url;
    page.params = p.params ?? {};
    page.route = { id: p.route ?? null };
    page.status = p.status;
    page.error = p.type === 'error' ? (p.error ?? { message: 'Error' }) : null;
    page.data = data[data.length - 1] ?? {};
    page.state = state;
    page.form = p.hasForm ? (p.form ?? null) : null;
}

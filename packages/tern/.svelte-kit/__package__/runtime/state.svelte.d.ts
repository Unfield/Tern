import type { Navigation, Payload } from './types.js';
declare class PageState {
    /** The current URL. */
    url: URL;
    /** The current route's parameters. */
    params: Record<string, string>;
    /** The current route. `id` is null when no route matched. */
    route: {
        id: string | null;
    };
    /** The HTTP status of the current page. */
    status: number;
    /** The error being shown, or null. */
    error: {
        message: string;
    } | null;
    /** The merged data of every layout and the page. */
    data: Record<string, unknown>;
    /** The result of the last form action, or null. */
    form: unknown;
    /** State stored with the current history entry (see goto's `state`). */
    state: Record<string, unknown>;
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
export declare const page: PageState;
/**
 * The navigation in progress, or `null`. Use it to show a loading
 * indicator:
 *
 * ```svelte
 * {#if navigating.current}<Spinner />{/if}
 * ```
 */
export declare const navigating: {
    readonly current: Navigation | null;
};
export declare function setNavigating(n: Navigation | null): void;
/** Updates page state for a rendered payload. */
export declare function applyPayloadToPage(p: Payload, url: URL, state: Record<string, unknown>, data: Record<string, unknown>[]): void;
export {};

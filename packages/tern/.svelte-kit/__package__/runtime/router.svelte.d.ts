import type { ActionResult, ClientManifest, GotoOptions, Payload } from './types.js';
/** Starts the client. Called by the generated entry module. */
export declare function start(m: ClientManifest): Promise<void>;
/** Navigates to href, which may be relative to the current URL. */
export declare function goto(href: string | URL, opts?: GotoOptions): Promise<void>;
/** Reruns every load for the current page. */
export declare function invalidateAll(): Promise<void>;
/**
 * Loads the data and code for a URL so a later navigation to it is instant.
 * Tern does this automatically for links; see data-tern-preload.
 */
export declare function preloadData(href: string | URL): Promise<Payload | null>;
/** Applies a form action result to the page, as `use:enhance` does. */
export declare function applyAction(result: ActionResult): Promise<void>;

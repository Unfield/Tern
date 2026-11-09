/**
 * Client API for Tern apps.
 *
 * ```svelte
 * <script>
 *   import { page, goto, enhance } from '@unfield/tern';
 * </script>
 * ```
 */
export { page, navigating } from './runtime/state.svelte.js';
export { goto, invalidateAll, preloadData, applyAction } from './runtime/router.svelte.js';
export { enhance } from './runtime/forms.js';
export type { SubmitFunction, SubmitInput, SubmitResult } from './runtime/forms.js';
export type { ActionResult, GotoOptions, Navigation, NavigationTarget, NavigationType } from './runtime/types.js';
/** The version of the Tern client runtime. */
export declare const VERSION = "0.0.0";

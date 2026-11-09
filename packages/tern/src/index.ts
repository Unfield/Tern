/**
 * Client API for Tern apps.
 *
 * ```svelte
 * <script>
 *   import { page, goto, enhance } from '@ternkit/tern';
 * </script>
 * ```
 */
export { page, navigating, env } from './runtime/state.svelte.js';
export { goto, invalidate, invalidateAll, preloadData, applyAction } from './runtime/router.svelte.js';
export { beforeNavigate, onNavigate, afterNavigate } from './runtime/navigation.js';
export type { BeforeNavigateEvent, OnNavigateEvent } from './runtime/navigation.js';
export { enhance } from './runtime/forms.js';
export type { SubmitFunction, SubmitInput, SubmitResult } from './runtime/forms.js';
export type { ActionResult, GotoOptions, Navigation, NavigationTarget, NavigationType } from './runtime/types.js';

/** The version of the Tern client runtime. */
export const VERSION = '0.0.0';

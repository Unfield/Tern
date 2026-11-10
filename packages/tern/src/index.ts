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

export { stream, socket, EventStreamState, SocketState } from './runtime/realtime.svelte.js';
export type { ConnectionStatus, StreamOptions, SocketOptions } from './runtime/realtime.svelte.js';
export { default as Image } from './image/Image.svelte';
export type { Picture } from './image/types.js';
export { t, i18n, setLocale } from './runtime/i18n.svelte.js';
export type { MessageKey } from './runtime/i18n.svelte.js';

/**
 * The app's message keys. `tern gen` fills this in from src/locales, so
 * t() only accepts keys that exist.
 */
// eslint-disable-next-line @typescript-eslint/no-empty-interface
export interface Messages {}

/** The version of the Tern client runtime. */
export const VERSION = '0.0.0';

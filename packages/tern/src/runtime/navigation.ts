import { onMount } from 'svelte';
import type { Navigation } from './types.js';

/** Passed to beforeNavigate callbacks. */
export interface BeforeNavigateEvent extends Navigation {
	/** True when the browser is about to leave the app (a reload, closing the tab, or another site). */
	willUnload: boolean;
	/** Stops the navigation. When leaving the page, the browser asks the user to confirm. */
	cancel(): void;
}

/** Passed to onNavigate callbacks. */
export interface OnNavigateEvent extends Navigation {
	/** Resolves when the new page has rendered. */
	complete: Promise<void>;
}

type BeforeFn = (nav: BeforeNavigateEvent) => void;
type OnFn = (nav: OnNavigateEvent) => void | Promise<void> | (() => void) | Promise<() => void>;
type AfterFn = (nav: Navigation) => void;

const before = new Set<BeforeFn>();
const on = new Set<OnFn>();
const after = new Set<AfterFn>();

/**
 * Runs before every navigation, including leaving the app. Call
 * `nav.cancel()` to stop it, for example to keep unsaved changes:
 *
 * ```js
 * beforeNavigate((nav) => {
 *   if (dirty && !confirm('Discard your changes?')) nav.cancel();
 * });
 * ```
 *
 * Call it while a component initializes; it is removed when the component is destroyed.
 */
export function beforeNavigate(fn: BeforeFn): void {
	onMount(() => {
		before.add(fn);
		return () => before.delete(fn);
	});
}

/**
 * Runs when a client-side navigation has loaded its data, right before the
 * new page renders. Return a promise to delay rendering, for example to
 * start a view transition; return a function to run after the navigation.
 *
 * ```js
 * onNavigate((nav) => {
 *   if (!document.startViewTransition) return;
 *   return new Promise((resolve) => {
 *     document.startViewTransition(async () => {
 *       resolve();
 *       await nav.complete;
 *     });
 *   });
 * });
 * ```
 */
export function onNavigate(fn: OnFn): void {
	onMount(() => {
		on.add(fn);
		return () => on.delete(fn);
	});
}

/** Runs after every navigation, including the first page load. */
export function afterNavigate(fn: AfterFn): void {
	onMount(() => {
		after.add(fn);
		return () => after.delete(fn);
	});
}

/** Runs beforeNavigate callbacks; reports whether the navigation may go ahead. */
export function runBefore(nav: Navigation, willUnload: boolean): boolean {
	let cancelled = false;
	const event: BeforeNavigateEvent = { ...nav, willUnload, cancel: () => (cancelled = true) };
	for (const fn of before) fn(event);
	return !cancelled;
}

/** Runs onNavigate callbacks and returns the cleanups to call afterwards. */
export async function runOn(nav: Navigation, complete: Promise<void>): Promise<(() => void)[]> {
	const results = await Promise.all([...on].map((fn) => fn({ ...nav, complete })));
	return results.filter((r): r is () => void => typeof r === 'function');
}

export function runAfter(nav: Navigation): void {
	for (const fn of after) fn(nav);
}

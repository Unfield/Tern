import { type Options as SvelteOptions } from '@sveltejs/vite-plugin-svelte';
import type { Plugin } from 'vite';
export interface TernOptions {
    /** Options passed to @sveltejs/vite-plugin-svelte. */
    svelte?: Partial<SvelteOptions>;
}
/**
 * The Vite plugin for Tern apps. It includes the Svelte plugin, so it is
 * the only plugin a Tern app needs:
 *
 * ```js
 * // vite.config.js
 * import { tern } from '@unfield/tern/vite';
 * import { defineConfig } from 'vite';
 *
 * export default defineConfig({ plugins: [tern()] });
 * ```
 */
export declare function tern(options?: TernOptions): Plugin[];
export default tern;

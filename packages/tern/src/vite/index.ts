import { svelte, type Options as SvelteOptions } from '@sveltejs/vite-plugin-svelte';
import path from 'node:path';
import type { Plugin, UserConfig } from 'vite';

export interface TernOptions {
	/** Options passed to @sveltejs/vite-plugin-svelte. */
	svelte?: Partial<SvelteOptions>;
}

/** Paths shared with the Go code generator; keep in sync with internal/codegen. */
const CLIENT_ENTRY = '.tern/generated/client.js';
const CLIENT_OUT = '.tern/output/client';

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
export function tern(options: TernOptions = {}): Plugin[] {
	const plugin: Plugin = {
		name: 'tern',
		config(config): UserConfig {
			const root = path.resolve(config.root ?? process.cwd());
			const vitePort = Number(process.env.TERN_VITE_PORT) || undefined;
			const appPort = Number(process.env.TERN_APP_PORT) || undefined;
			return {
				appType: 'custom',
				publicDir: 'static',
				resolve: {
					alias: { $lib: path.join(root, 'src/lib') }
				},
				build: {
					outDir: CLIENT_OUT,
					emptyOutDir: true,
					manifest: true,
					assetsDir: '_app/immutable',
					rolldownOptions: { input: path.join(root, CLIENT_ENTRY) }
				},
				server: {
					// Bind to IPv4 explicitly: the Go app proxies to 127.0.0.1, and
					// Node may resolve "localhost" to ::1 only.
					host: vitePort ? '127.0.0.1' : undefined,
					port: vitePort,
					strictPort: vitePort !== undefined,
					// The browser talks to the Go app, which proxies HMR to Vite.
					hmr: appPort ? { clientPort: appPort } : undefined,
					fs: { allow: [root] }
				},
				optimizeDeps: {
					entries: [CLIENT_ENTRY, 'src/**/*.svelte'],
					exclude: ['@unfield/tern']
				}
			};
		}
	};
	return [...svelte(options.svelte), plugin];
}

export default tern;

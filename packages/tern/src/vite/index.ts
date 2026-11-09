import { svelte, type Options as SvelteOptions } from '@sveltejs/vite-plugin-svelte';
import type { IncomingMessage } from 'node:http';
import fs from 'node:fs';
import path from 'node:path';
import type { Plugin, UserConfig, ViteDevServer } from 'vite';

export interface TernOptions {
	/** Options passed to @sveltejs/vite-plugin-svelte. */
	svelte?: Partial<SvelteOptions>;
}

/** Paths shared with the Go code generator; keep in sync with internal/codegen. */
const CLIENT_ENTRY = '.tern/generated/client.js';
const SERVER_ENTRY = '.tern/generated/server.js';
const CLIENT_OUT = '.tern/output/client';
const SERVER_OUT = '.tern/output/server';

/** The dev endpoint the Go app calls to server-render a page. */
const RENDER_PATH = '/__tern/render';

/**
 * The Vite plugin for Tern apps. It includes the Svelte plugin, so it is
 * the only plugin a Tern app needs:
 *
 * ```js
 * // vite.config.js
 * import { tern } from '@ternkit/tern/vite';
 * import { defineConfig } from 'vite';
 *
 * export default defineConfig({ plugins: [tern()] });
 * ```
 *
 * `vite build` builds the client; `vite build --ssr` builds the server
 * renderer that `tern build` embeds in the Go binary.
 */
export function tern(options: TernOptions = {}): Plugin[] {
	let root = process.cwd();

	const plugin: Plugin = {
		name: 'tern',
		config(config, env): UserConfig {
			root = path.resolve(config.root ?? process.cwd());
			const basePath = readBasePath(root);
			const shared: UserConfig = {
				appType: 'custom',
				base: basePath + '/',
				resolve: {
					alias: {
						$lib: path.join(root, 'src/lib'),
						$routes: path.join(root, '.tern/generated/routes.js')
					}
				}
			};

			if (env.isSsrBuild) {
				return {
					...shared,
					publicDir: false,
					build: {
						ssr: true,
						outDir: SERVER_OUT,
						emptyOutDir: true,
						minify: true,
						target: 'es2023',
						rolldownOptions: {
							input: path.join(root, SERVER_ENTRY),
							output: { format: 'iife', name: '__tern_server', entryFileNames: 'server.js' }
						}
					},
					// One self-contained file: the renderer runs in an embedded engine
					// without a module loader or Node built-ins.
					ssr: { noExternal: true },
					// Libraries often read process.env.NODE_ENV; the engine has no
					// process object, so replace it at build time.
					define: { 'process.env.NODE_ENV': JSON.stringify('production') }
				};
			}

			const vitePort = Number(process.env.TERN_VITE_PORT) || undefined;
			const appPort = Number(process.env.TERN_APP_PORT) || undefined;
			return {
				...shared,
				publicDir: 'static',
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
					ws: appPort ? { clientPort: appPort } : undefined,
					fs: { allow: [root] }
				},
				optimizeDeps: {
					entries: [CLIENT_ENTRY, 'src/**/*.svelte'],
					exclude: ['@ternkit/tern']
				}
			};
		},
		configureServer(server) {
			server.middlewares.use(RENDER_PATH, (req, res) => {
				void renderInDev(server, req).then(
					(body) => {
						res.setHeader('Content-Type', 'application/json');
						res.end(body);
					},
					(err: unknown) => {
						const e = err instanceof Error ? err : new Error(String(err));
						server.ssrFixStacktrace(e);
						res.statusCode = 500;
						res.setHeader('Content-Type', 'text/plain; charset=utf-8');
						res.end(e.stack ?? e.message);
					}
				);
			});
		}
	};

	async function renderInDev(server: ViteDevServer, req: IncomingMessage): Promise<string> {
		if (req.method !== 'POST') throw new Error('POST required');
		const chunks: Buffer[] = [];
		for await (const chunk of req) chunks.push(chunk as Buffer);
		const { payload, href } = JSON.parse(Buffer.concat(chunks).toString('utf8')) as { payload: string; href: string };
		const mod = (await server.ssrLoadModule(path.join(root, SERVER_ENTRY))) as {
			render: (payload: string, href: string) => string;
		};
		return mod.render(payload, href);
	}

	return [...svelte(options.svelte), plugin];
}

/** Reads basePath from tern.json; the Go side validates it. */
function readBasePath(root: string): string {
	try {
		const config = JSON.parse(fs.readFileSync(path.join(root, 'tern.json'), 'utf8')) as { basePath?: string };
		return config.basePath ?? '';
	} catch {
		return '';
	}
}

export default tern;

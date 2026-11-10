import { svelte, type Options as SvelteOptions } from '@sveltejs/vite-plugin-svelte';
import type { IncomingMessage } from 'node:http';
import fs from 'node:fs';
import path from 'node:path';
import type { Plugin, UserConfig, ViteDevServer } from 'vite';
import { imagePlugin } from './image.js';

export interface TernOptions {
	/** Options passed to @sveltejs/vite-plugin-svelte. */
	svelte?: Partial<SvelteOptions>;
}

/** Paths shared with the Go code generator; keep in sync with internal/codegen. */
const CLIENT_ENTRY = '.tern/generated/client.js';
const SERVER_ENTRY = '.tern/generated/server.js';
const CLIENT_OUT = '.tern/output/client';
const SW_MODULE = '$service-worker';
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

			if (process.env.TERN_SW_BUILD === '1') {
				const source = ['src/service-worker.ts', 'src/service-worker.js'].map((f) => path.join(root, f)).find((f) => fs.existsSync(f));
				if (!source) throw new Error('tern: no src/service-worker.ts to build');
				return {
					...shared,
					publicDir: false,
					build: {
						outDir: CLIENT_OUT,
						emptyOutDir: false,
						manifest: false,
						minify: true,
						target: 'es2022',
						rolldownOptions: {
							input: source,
							output: { format: 'iife', entryFileNames: 'service-worker.js' }
						}
					}
				};
			}

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
		resolveId(id) {
			return id === SW_MODULE ? '\0' + SW_MODULE : undefined;
		},
		load(id) {
			if (id !== '\0' + SW_MODULE) return undefined;
			return serviceWorkerModule(root, readBasePath(root));
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

	return [imagePlugin(), ...svelte(options.svelte), plugin];
}

/** The $service-worker module: what this build produced, for caching. */
function serviceWorkerModule(root: string, base: string): string {
	const out = path.join(root, CLIENT_OUT);
	const build: string[] = [];
	try {
		const manifest = JSON.parse(fs.readFileSync(path.join(out, '.vite/manifest.json'), 'utf8')) as Record<string, { file: string; css?: string[]; assets?: string[] }>;
		const seen = new Set<string>();
		for (const chunk of Object.values(manifest)) {
			for (const f of [chunk.file, ...(chunk.css ?? []), ...(chunk.assets ?? [])]) {
				if (!seen.has(f)) {
					seen.add(f);
					build.push(`${base}/${f}`);
				}
			}
		}
	} catch {
		// Development: nothing is built yet.
	}
	const files = walk(path.join(root, 'static')).map((f) => `${base}/${f}`);
	const prerendered = walk(out)
		.filter((f) => f === 'index.html' || f.endsWith('/index.html'))
		.map((f) => `${base}/${f.replace(/\/?index\.html$/, '')}` || '/');
	const version = Date.now().toString(36);
	return [
		`export const base = ${JSON.stringify(base)};`,
		`export const build = ${JSON.stringify(build.sort())};`,
		`export const files = ${JSON.stringify(files.sort())};`,
		`export const prerendered = ${JSON.stringify(prerendered.sort())};`,
		`export const version = ${JSON.stringify(version)};`
	].join('\n');
}

/** Lists files below dir, relative to it, with forward slashes. */
function walk(dir: string, prefix = ''): string[] {
	let entries: fs.Dirent[];
	try {
		entries = fs.readdirSync(dir, { withFileTypes: true });
	} catch {
		return [];
	}
	return entries.flatMap((e) => {
		if (e.name.startsWith('.') || e.name.endsWith('.gz')) return [];
		const rel = prefix ? `${prefix}/${e.name}` : e.name;
		return e.isDirectory() ? walk(path.join(dir, e.name), rel) : [rel];
	});
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

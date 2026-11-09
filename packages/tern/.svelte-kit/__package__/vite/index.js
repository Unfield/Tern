import { svelte } from '@sveltejs/vite-plugin-svelte';
import path from 'node:path';
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
 * import { tern } from '@unfield/tern/vite';
 * import { defineConfig } from 'vite';
 *
 * export default defineConfig({ plugins: [tern()] });
 * ```
 *
 * `vite build` builds the client; `vite build --ssr` builds the server
 * renderer that `tern build` embeds in the Go binary.
 */
export function tern(options = {}) {
    let root = process.cwd();
    const plugin = {
        name: 'tern',
        config(config, env) {
            root = path.resolve(config.root ?? process.cwd());
            const shared = {
                appType: 'custom',
                resolve: { alias: { $lib: path.join(root, 'src/lib') } }
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
                    ssr: { noExternal: true }
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
                    hmr: appPort ? { clientPort: appPort } : undefined,
                    fs: { allow: [root] }
                },
                optimizeDeps: {
                    entries: [CLIENT_ENTRY, 'src/**/*.svelte'],
                    exclude: ['@unfield/tern']
                }
            };
        },
        configureServer(server) {
            server.middlewares.use(RENDER_PATH, (req, res) => {
                void renderInDev(server, req).then((body) => {
                    res.setHeader('Content-Type', 'application/json');
                    res.end(body);
                }, (err) => {
                    const e = err instanceof Error ? err : new Error(String(err));
                    server.ssrFixStacktrace(e);
                    res.statusCode = 500;
                    res.setHeader('Content-Type', 'text/plain; charset=utf-8');
                    res.end(e.stack ?? e.message);
                });
            });
        }
    };
    async function renderInDev(server, req) {
        if (req.method !== 'POST')
            throw new Error('POST required');
        const chunks = [];
        for await (const chunk of req)
            chunks.push(chunk);
        const { payload, href } = JSON.parse(Buffer.concat(chunks).toString('utf8'));
        const mod = (await server.ssrLoadModule(path.join(root, SERVER_ENTRY)));
        return mod.render(payload, href);
    }
    return [...svelte(options.svelte), plugin];
}
export default tern;

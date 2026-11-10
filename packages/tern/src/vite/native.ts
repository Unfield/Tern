import { parse, type Program } from 'acorn';
import fs from 'node:fs';
import path from 'node:path';
import type { Plugin } from 'vite';

// Native rendering (ADR 0011): during tern build's server build, records
// each module as the bundler receives it, so `tern gen` can compile the
// components to Go. tern build sets TERN_NATIVE=1; nothing changes otherwise.

/** One module of the server build. */
interface NativeModule {
	/** The module's file, relative to the app, with / separators. */
	id: string;
	/** Each import's specifier, mapped to the module it resolves to, or null when it isn't recorded. */
	imports: Record<string, string | null>;
	ast: Program;
	code: string;
}

export const NATIVE_IR = '.tern/native/ir.json';

function svelteVersion(root: string): string {
	try {
		const pkg = path.join(path.dirname(require_resolve('svelte/package.json', root)));
		return JSON.parse(fs.readFileSync(path.join(pkg, 'package.json'), 'utf8')).version as string;
	} catch {
		return '';
	}
}

function require_resolve(spec: string, root: string): string {
	// Find svelte's package.json from the app's node_modules, as Vite does.
	let dir = root;
	for (;;) {
		const candidate = path.join(dir, 'node_modules', spec);
		if (fs.existsSync(candidate)) return candidate;
		const up = path.dirname(dir);
		if (up === dir) throw new Error(`cannot find ${spec}`);
		dir = up;
	}
}

export function nativePlugin(getRoot: () => string, entry: string): Plugin {
	const modules = new Map<string, NativeModule>();
	let active = false;
	let env: Record<string, unknown> = {};
	let base = '/';
	// Asset URLs are placeholders in a module (import.meta.ROLLDOWN_FILE_URL_…)
	// until the bundle is written: their final URLs go in the IR.
	const fileRefs = new Set<string>();
	const fileUrls: Record<string, string> = {};
	const bundled: Record<string, boolean> = {};
	const rel = (file: string) => path.relative(getRoot(), file).split(path.sep).join('/');
	// Every module is recorded, Svelte's own runtime included: compiling its
	// code, rather than rewriting it, keeps the output exactly Svelte's.
	const native = (_file: string) => false;

	return {
		name: 'tern:native-ir',
		enforce: 'post',
		configResolved(config) {
			active = !!config.build.ssr && process.env.TERN_NATIVE === '1';
			env = JSON.parse(JSON.stringify({ ...config.env, SSR: true }));
			base = config.base || '/';
		},
		async transform(code, id) {
			if (!active || id.startsWith('\0')) return null;
			// Modules with a query, such as an asset's ?url, are recorded under
			// their whole id; what isn't JavaScript fails to parse and is left out.
			const file = id;
			const query = id.includes('?');
			if ((!query && !/\.(svelte|[cm]?[jt]s|[jt]sx|json)$/.test(file)) || native(file)) return null;
			try {
				const ast = parse(code, { ecmaVersion: 'latest', sourceType: 'module' });
				const imports: Record<string, string | null> = {};
				const sources: string[] = [];
				for (const node of ast.body) {
					if (node.type === 'ImportDeclaration' || node.type === 'ExportAllDeclaration' || (node.type === 'ExportNamedDeclaration' && node.source)) {
						sources.push(node.source!.value as string);
					}
				}
				// import('./x') with a literal resolves like a static import.
				const walk = (n: unknown): void => {
					if (!n || typeof n !== 'object') return;
					if (Array.isArray(n)) return n.forEach(walk);
					const node = n as { type?: string; source?: { type: string; value: unknown } };
					if (node.type === 'ImportExpression' && node.source?.type === 'Literal' && typeof node.source.value === 'string') sources.push(node.source.value);
					for (const v of Object.values(node)) if (v && typeof v === 'object') walk(v);
				};
				walk(ast.body);
				for (const source of sources) {
					if (source in imports) continue;
					const resolved = await this.resolve(source, id, { ssr: true } as never);
					const target = resolved && !resolved.external && !resolved.id.startsWith('\0') ? resolved.id : null;
					imports[source] = target && !native(target) ? rel(target) : null;
				}
				modules.set(file, { id: rel(file), imports, ast, code });
				for (const m of code.matchAll(/import\.meta\.((?:ROLLDOWN|ROLLUP|VITE)_FILE_URL_([\w$]+))/g)) fileRefs.add(m[1]!);
			} catch {
				// Not JavaScript we can read: the module stays unrecorded, so
				// whatever imports it renders with the JavaScript engine.
			}
			return null;
		},
		generateBundle(_opts, bundle) {
			if (!active) return;
			// What the bundler kept code of: modules it tree-shook away aren't
			// compiled.
			for (const out of Object.values(bundle)) {
				if (out.type !== 'chunk') continue;
				for (const [id, m] of Object.entries(out.modules)) if (m.renderedLength > 0) bundled[rel(id)] = true;
			}
			for (const ref of fileRefs) {
				try {
					fileUrls[ref] = path.posix.join(base, this.getFileName(ref.replace(/^(?:ROLLDOWN|ROLLUP|VITE)_FILE_URL_/, '')));
				} catch {
					// Not a file of this build: the module falls back.
				}
			}
		},
		closeBundle() {
			if (!active) return;
			const root = getRoot();
			const out = path.join(root, NATIVE_IR);
			fs.mkdirSync(path.dirname(out), { recursive: true });
			const ir = { version: 1, svelte: svelteVersion(root), entry, env, fileUrls, bundled, modules: [...modules.values()] };
			try {
				// BigInt literals keep their digits in the node's bigint field;
				// their value, which JSON can't hold, is left out.
				fs.writeFileSync(out, JSON.stringify(ir, (_k, v) => (typeof v === 'bigint' ? null : v)));
			} catch (err) {
				// Native rendering is an optimisation: without the IR, tern build
				// skips it and pages render with the JavaScript engine.
				fs.rmSync(out, { force: true });
				this.warn(`native rendering skipped: ${(err as Error).message}`);
			}
		}
	};
}

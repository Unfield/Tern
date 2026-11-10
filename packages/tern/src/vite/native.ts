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
	const rel = (file: string) => path.relative(getRoot(), file).split(path.sep).join('/');
	// Every module is recorded, Svelte's own runtime included: compiling its
	// code, rather than rewriting it, keeps the output exactly Svelte's.
	const native = (_file: string) => false;

	return {
		name: 'tern:native-ir',
		enforce: 'post',
		configResolved(config) {
			active = !!config.build.ssr && process.env.TERN_NATIVE === '1';
		},
		async transform(code, id) {
			if (!active || id.startsWith('\0')) return null;
			const file = id.split('?')[0]!;
			if (id.includes('?') || !/\.(svelte|[cm]?[jt]s|[jt]sx)$/.test(file) || native(file)) return null;
			let ast: Program;
			try {
				ast = parse(code, { ecmaVersion: 'latest', sourceType: 'module' });
			} catch {
				return null; // not JavaScript we can read: the module stays unrecorded, so it isn't native
			}
			const imports: Record<string, string | null> = {};
			for (const node of ast.body) {
				const source =
					node.type === 'ImportDeclaration' || node.type === 'ExportAllDeclaration' || (node.type === 'ExportNamedDeclaration' && node.source)
						? (node.source!.value as string)
						: null;
				if (source === null || source in imports) continue;
				const resolved = await this.resolve(source, id, { ssr: true } as never);
				const target = resolved && !resolved.external && !resolved.id.startsWith('\0') ? resolved.id.split('?')[0]! : null;
				imports[source] = target && !native(target) ? rel(target) : null;
			}
			modules.set(file, { id: rel(file), imports, ast });
			return null;
		},
		closeBundle() {
			if (!active) return;
			const root = getRoot();
			const out = path.join(root, NATIVE_IR);
			fs.mkdirSync(path.dirname(out), { recursive: true });
			const ir = { version: 1, svelte: svelteVersion(root), entry, modules: [...modules.values()] };
			fs.writeFileSync(out, JSON.stringify(ir));
		}
	};
}

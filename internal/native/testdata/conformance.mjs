// Writes conformance.json: each case in conformance/ as IR (what the Vite
// plugin records) with what Node computes for globalThis.out.
// Run it with `node testdata/conformance.mjs` from internal/native.
import fs from 'node:fs';
import path from 'node:path';
import { createRequire } from 'node:module';
import { pathToFileURL } from 'node:url';

const require = createRequire(path.resolve('../../packages/tern/package.json'));
const { parse } = require('acorn');
const dir = 'testdata/conformance';
const cases = [];
// A directory is a case of several modules, entered at main.js.
for (const name of fs.readdirSync(dir).filter((f) => fs.statSync(path.join(dir, f)).isDirectory()).sort()) {
	const sub = path.join(dir, name);
	const modules = [];
	for (const file of fs.readdirSync(sub).filter((f) => f.endsWith('.js')).sort()) {
		const code = fs.readFileSync(path.join(sub, file), 'utf8');
		const ast = parse(code, { ecmaVersion: 'latest', sourceType: 'module' });
		const imports = {};
		const add = (spec) => (imports[spec] = path.posix.join(path.posix.dirname(file), spec));
		JSON.stringify(ast, (k, v) => {
			if (v && (v.type === 'ImportDeclaration' || v.type === 'ImportExpression' || v.type === 'ExportAllDeclaration' || (v.type === 'ExportNamedDeclaration' && v.source))) {
				if (v.source?.type === 'Literal') add(v.source.value);
			}
			return v;
		});
		modules.push({ id: file, imports, ast, code });
	}
	delete globalThis.out;
	await import(pathToFileURL(path.resolve(sub, 'main.js')).href);
	await new Promise((r) => setTimeout(r, 10));
	cases.push({ name, want: JSON.stringify(globalThis.out), ir: { version: 1, svelte: '', entry: 'main.js', env: {}, fileUrls: {}, modules } });
}
for (const file of fs.readdirSync(dir).filter((f) => f.endsWith('.js')).sort()) {
	const code = fs.readFileSync(path.join(dir, file), 'utf8');
	const ast = parse(code, { ecmaVersion: 'latest', sourceType: 'module' });
	delete globalThis.out;
	await import(pathToFileURL(path.resolve(dir, file)).href);
	await new Promise((r) => setTimeout(r, 10)); // let every job run
	cases.push({
		name: file.replace(/\.js$/, ''),
		want: JSON.stringify(globalThis.out),
		ir: { version: 1, svelte: '', entry: file, env: {}, fileUrls: {}, modules: [{ id: file, imports: {}, ast, code }] },
	});
}
fs.writeFileSync('testdata/conformance.json', JSON.stringify({ cases }, (_k, v) => (typeof v === 'bigint' ? null : v)));
console.log(`${cases.length} cases`);

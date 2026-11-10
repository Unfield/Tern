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
for (const file of fs.readdirSync(dir).filter((f) => f.endsWith('.js')).sort()) {
	const code = fs.readFileSync(path.join(dir, file), 'utf8');
	const ast = parse(code, { ecmaVersion: 'latest', sourceType: 'module' });
	delete globalThis.out;
	await import(pathToFileURL(path.resolve(dir, file)).href);
	cases.push({
		name: file.replace(/\.js$/, ''),
		want: JSON.stringify(globalThis.out),
		ir: { version: 1, svelte: '', entry: file, env: {}, fileUrls: {}, modules: [{ id: file, imports: {}, ast, code }] },
	});
}
fs.writeFileSync('testdata/conformance.json', JSON.stringify({ cases }, (_k, v) => (typeof v === 'bigint' ? null : v)));
console.log(`${cases.length} cases`);

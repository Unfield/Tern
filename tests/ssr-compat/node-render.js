// Renders every case of the built bundle in Node, for TestCompat to compare
// with the output of Tern's renderer. Node has real web APIs, so the
// polyfills stay out of the way: differences point at gaps in them.
import fs from 'node:fs';
import vm from 'node:vm';

const bundle = '.tern/output/server/server.js';
vm.runInThisContext(fs.readFileSync(bundle, 'utf8'), { filename: bundle });
const render = globalThis.__tern_render;
const cases = JSON.parse(JSON.parse(render(JSON.stringify({ list: true }))).body);
const out = {};
for (const { name } of cases) out[name] = JSON.parse(render(JSON.stringify({ case: name }))).body;
fs.writeFileSync('.tern/output/node.json', JSON.stringify(out, null, '\t'));
console.log(`rendered ${cases.length} cases in Node`);

#!/usr/bin/env node
// npm create tern@latest [directory] [-- flags]
//
// Runs `tern new` from the Tern release matching this package's version, so
// the prompts, templates and flags are the CLI's own. Tern needs Go.

import { spawnSync } from 'node:child_process';
import { readFileSync } from 'node:fs';

const { version } = JSON.parse(readFileSync(new URL('./package.json', import.meta.url), 'utf8'));
const ref = `github.com/Unfield/Tern/cmd/tern@${version === '0.0.0' ? 'latest' : `v${version}`}`;

const go = spawnSync('go', ['version'], { encoding: 'utf8' });
if (go.error || go.status !== 0) {
	console.error(
		'\n  Tern needs Go, which wasn\'t found on your PATH.\n' +
			'  Install it from https://go.dev/dl/ and run this again.\n'
	);
	process.exit(1);
}

const args = process.argv.slice(2);
const result = spawnSync('go', ['run', ref, 'new', ...args], {
	stdio: 'inherit',
	// An older Go downloads the toolchain Tern needs.
	env: { GOTOOLCHAIN: 'auto', ...process.env }
});
if (result.error) {
	console.error(result.error.message);
	process.exit(1);
}
process.exit(result.status ?? 1);

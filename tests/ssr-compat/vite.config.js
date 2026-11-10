import { tern } from '@ternkit/tern/vite';
import { defineConfig } from 'vite';

// Tern's own SSR build settings, with the compatibility suite's entry.
export default defineConfig({
	plugins: [
		tern(),
		{ name: 'compat-entry', config: () => ({ build: { rolldownOptions: { input: 'src/entry.js' } } }) }
	]
});

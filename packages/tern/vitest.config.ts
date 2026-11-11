import { svelte } from '@sveltejs/vite-plugin-svelte';
import { defineConfig } from 'vitest/config';

export default defineConfig({
	// Compiles .svelte.ts modules, whose runes the runtime uses.
	plugins: [svelte()],
	test: {
		include: ['src/**/*.test.ts']
	}
});

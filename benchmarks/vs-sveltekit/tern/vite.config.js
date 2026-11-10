import { tern } from '@ternkit/tern/vite';
import { defineConfig } from 'vite';

export default defineConfig({
	plugins: [tern()],
	resolve: { dedupe: ['svelte'] },
	server: { fs: { allow: ['..'] } }
});

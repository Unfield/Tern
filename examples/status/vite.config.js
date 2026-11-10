import tailwindcss from '@tailwindcss/vite';
import { tern } from '@ternkit/tern/vite';
import { defineConfig } from 'vite';

export default defineConfig({
	plugins: [tailwindcss(), tern()]
});

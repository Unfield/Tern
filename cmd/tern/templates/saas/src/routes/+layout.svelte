<script lang="ts">
	import '@ternkit/tern/ui/theme.css';
	import { Toaster } from '@ternkit/tern/ui';
	import type { LayoutProps } from './$types';

	let { children }: LayoutProps = $props();
</script>

{@render children()}

<Toaster />

<style>
	:global(body) {
		margin: 0;
		font-family: var(--tern-font);
		color: var(--tern-fg);
		background: var(--tern-bg);
	}
</style>

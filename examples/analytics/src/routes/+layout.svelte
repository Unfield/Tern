<script lang="ts">
	import '@ternkit/tern/ui/theme.css';
	import { Toaster } from '@ternkit/tern/ui';
	import type { LayoutProps } from './$types';

	let { children }: LayoutProps = $props();
</script>

<div class="tern">{@render children()}</div>
<Toaster />

<style>
	:global(body) {
		margin: 0;
		background: var(--tern-bg);
	}
</style>

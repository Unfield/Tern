<script lang="ts">
	import type { LayoutProps } from './$types';
	let { children }: LayoutProps = $props();
</script>

{@render children()}

<style>
	:global(body) {
		font: 16px/1.6 system-ui, sans-serif;
		max-width: 46rem;
		margin: 2.5rem auto;
		padding: 0 1.25rem;
		color: #1c1917;
	}
	:global(.card) {
		border: 1px solid #e7e5e4;
		border-radius: 10px;
		padding: 1rem 1.25rem;
		margin: 1rem 0;
	}
	:global(button, input) {
		font: inherit;
		padding: 0.35rem 0.75rem;
	}
</style>

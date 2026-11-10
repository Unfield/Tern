<script lang="ts">
	import type { Snippet } from 'svelte';
	let { children }: { children: Snippet } = $props();
</script>

<kbd class="tern-kbd">{@render children()}</kbd>

<style>
	.tern-kbd {
		display: inline-flex;
		align-items: center;
		min-width: 1.4em;
		height: 1.4em;
		padding: 0 0.35em;
		justify-content: center;
		border: 1px solid var(--tern-border-strong);
		border-bottom-width: 2px;
		border-radius: 5px;
		background: var(--tern-surface);
		font-family: var(--tern-font-mono);
		font-size: 0.75em;
		color: var(--tern-fg);
	}
</style>

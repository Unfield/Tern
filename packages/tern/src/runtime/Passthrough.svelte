<script lang="ts">
	import type { Snippet } from 'svelte';

	// Used for layout directories that have a LayoutLoad but no +layout.svelte.
	let { children }: { children?: Snippet; data?: unknown } = $props();
</script>

{@render children?.()}

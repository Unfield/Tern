<script lang="ts">
	import type { Snippet } from 'svelte';

	type Props = {
		/** Width over height, such as 16 / 9. */
		ratio?: number;
		children: Snippet;
		class?: string;
	};
	let { ratio = 16 / 9, children, class: klass = '' }: Props = $props();
</script>

<!-- Keeps its content, such as an image or video, at a ratio as it resizes. -->
<div class="tern-aspect {klass}" style:aspect-ratio={ratio}>{@render children()}</div>

<style>
	.tern-aspect {
		position: relative;
		width: 100%;
		overflow: hidden;
		border-radius: var(--tern-radius);
	}
	.tern-aspect > :global(img),
	.tern-aspect > :global(video),
	.tern-aspect > :global(iframe) {
		position: absolute;
		inset: 0;
		width: 100%;
		height: 100%;
		object-fit: cover;
		border: 0;
	}
</style>

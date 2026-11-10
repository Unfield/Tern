<script lang="ts">
	import type { Snippet } from 'svelte';

	type Props = {
		/** Names the group for screen readers, such as "Text alignment". */
		label: string;
		orientation?: 'horizontal' | 'vertical';
		class?: string;
		/** Buttons, Menus or Inputs, drawn as one joined control. */
		children: Snippet;
	};
	let { label, orientation = 'horizontal', class: klass = '', children }: Props = $props();
</script>

<div class="tern-button-group {orientation} {klass}" role="group" aria-label={label}>
	{@render children()}
</div>

<style>
	.tern-button-group {
		display: inline-flex;
		isolation: isolate;
	}
	.vertical {
		flex-direction: column;
	}
	.tern-button-group > :global(*) {
		position: relative;
	}
	.tern-button-group > :global(*:focus-visible),
	.tern-button-group > :global(*:has(:focus-visible)) {
		z-index: 1;
	}
	.horizontal > :global(* + *) {
		margin-inline-start: -1px;
	}
	.vertical > :global(* + *) {
		margin-block-start: -1px;
	}
	.horizontal > :global(:not(:first-child)),
	.horizontal > :global(:not(:first-child) .trigger) {
		border-start-start-radius: 0;
		border-end-start-radius: 0;
	}
	.horizontal > :global(:not(:last-child)),
	.horizontal > :global(:not(:last-child) .trigger) {
		border-start-end-radius: 0;
		border-end-end-radius: 0;
	}
	.vertical > :global(:not(:first-child)) {
		border-start-start-radius: 0;
		border-start-end-radius: 0;
	}
	.vertical > :global(:not(:last-child)) {
		border-end-start-radius: 0;
		border-end-end-radius: 0;
	}
	.tern-button-group > :global(.tern-btn) {
		box-shadow: none;
	}
</style>

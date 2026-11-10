<script lang="ts">
	import type { Snippet } from 'svelte';
	import Dialog from './Dialog.svelte';

	type Props = {
		/** Whether it's open; bind it. */
		open?: boolean;
		title: string;
		description?: string;
		/** The edge it slides in from. */
		side?: 'left' | 'right' | 'top' | 'bottom';
		class?: string;
		children?: Snippet;
		footer?: Snippet;
		onclose?: () => void;
	};
	let { open = $bindable(false), side = 'right', ...rest }: Props = $props();
</script>

<!-- A modal panel against an edge of the screen, for navigation, filters or details. -->
<Dialog bind:open {side} {...rest} />

<script lang="ts">
	import type { Snippet } from 'svelte';
	import Dialog from './Dialog.svelte';

	type Props = {
		/** Whether it's open; bind it. */
		open?: boolean;
		title: string;
		description?: string;
		class?: string;
		children?: Snippet;
		footer?: Snippet;
		onclose?: () => void;
	};
	let { open = $bindable(false), ...rest }: Props = $props();
</script>

<!-- A bottom sheet: slides up, and closes when dragged down, tapped outside or on Escape. -->
<Dialog bind:open side="bottom" draggable {...rest} />

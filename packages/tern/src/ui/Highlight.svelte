<script lang="ts">
	import type { Segment } from './search.js';

	type Props = {
		/** The text, as the search package's segments, or plain. */
		text: string | Segment[] | null | undefined;
		class?: string;
	};
	let { text, class: klass = '' }: Props = $props();
	const parts = $derived(typeof text === 'string' ? [{ text }] : (text ?? []));
</script>

<span class="tern-highlight {klass}">{#each parts as p, i (i)}{#if p.match}<mark>{p.text}</mark>{:else}{p.text}{/if}{/each}</span>

<style>
	mark {
		background: var(--tern-accent-soft);
		color: inherit;
		font-weight: 600;
		border-radius: 2px;
		padding: 0 1px;
	}
</style>

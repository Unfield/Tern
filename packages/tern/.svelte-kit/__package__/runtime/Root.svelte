<script lang="ts">
	import type { Component } from 'svelte';

	interface Props {
		/** Layout components, outermost first, then the page or error component. */
		components: Component<any>[];
		/** Data for each component, already merged with its parents' data. */
		data: Record<string, unknown>[];
		form: unknown;
	}

	let { components, data, form }: Props = $props();
</script>

{#snippet level(i: number)}
	{@const Current = components[i]}
	{#if Current}
		{#if i === components.length - 1}
			<Current data={data[i]} {form} />
		{:else}
			<Current data={data[i]}>
				{@render level(i + 1)}
			</Current>
		{/if}
	{/if}
{/snippet}

{@render level(0)}

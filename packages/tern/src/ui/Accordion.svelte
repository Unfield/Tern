<script lang="ts">
	import type { Snippet } from 'svelte';

	type Item = { id: string; title: string };
	type Props = {
		items: Item[];
		/** The open items' ids; bind it. */
		open?: string[];
		/** Let several items be open at once. */
		multiple?: boolean;
		/** The heading level of the item titles, to fit the page's outline. */
		level?: 2 | 3 | 4 | 5 | 6;
		class?: string;
		/** Renders an item's content. */
		content: Snippet<[string]>;
	};
	let { items, open = $bindable([]), multiple = false, level = 3, class: klass = '', content }: Props = $props();
	const uid = $props.id();

	function toggle(id: string) {
		if (open.includes(id)) open = open.filter((x) => x !== id);
		else open = multiple ? [...open, id] : [id];
	}
</script>

<div class="tern-accordion {klass}">
	{#each items as item (item.id)}
		{@const expanded = open.includes(item.id)}
		<div class="item">
			<div role="heading" aria-level={level}>
				<button type="button" id="tern-acc-{uid}-{item.id}" aria-expanded={expanded} aria-controls="tern-acc-{uid}-{item.id}-panel" onclick={() => toggle(item.id)}>
					{item.title}
					<svg aria-hidden="true" viewBox="0 0 16 16" width="16" height="16"><path d="M4 6l4 4 4-4" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" /></svg>
				</button>
			</div>
			<div class="panel" role="region" id="tern-acc-{uid}-{item.id}-panel" aria-labelledby="tern-acc-{uid}-{item.id}" hidden={!expanded}>
				{#if expanded}{@render content(item.id)}{/if}
			</div>
		</div>
	{/each}
</div>

<style>
	.tern-accordion {
		border-top: 1px solid var(--tern-border);
	}
	.item {
		border-bottom: 1px solid var(--tern-border);
	}
	button {
		display: flex;
		width: 100%;
		align-items: center;
		justify-content: space-between;
		gap: 1rem;
		padding: 0.85rem 0.25rem;
		border: 0;
		background: transparent;
		font: inherit;
		font-size: 0.9375rem;
		font-weight: 550;
		color: var(--tern-fg);
		text-align: start;
		cursor: pointer;
	}
	button:focus-visible {
		outline: 2px solid var(--tern-ring);
		outline-offset: -2px;
		border-radius: var(--tern-radius-sm);
	}
	svg {
		flex: none;
		color: var(--tern-muted);
		transition: rotate var(--tern-duration);
	}
	[aria-expanded='true'] svg {
		rotate: 180deg;
	}
	.panel {
		padding: 0 0.25rem 1rem;
		color: var(--tern-muted);
		font-size: 0.875rem;
	}
</style>

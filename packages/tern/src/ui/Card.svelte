<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';

	type Props = HTMLAttributes<HTMLElement> & {
		/** A heading for the card. */
		title?: string;
		description?: string;
		/** The heading level, to fit the page's outline. */
		level?: 2 | 3 | 4;
		/** The element to render: section for standalone content, article for items in a feed. */
		as?: 'div' | 'section' | 'article';
		children?: Snippet;
		/** Actions in the header's corner. */
		actions?: Snippet;
		footer?: Snippet;
	};
	let { title, description, level = 3, as = 'div', class: klass = '', children, actions, footer, ...rest }: Props = $props();
</script>

<svelte:element this={as} class="tern-card {klass}" {...rest}>
	{#if title || actions}
		<header>
			<div>
				{#if title}<svelte:element this={`h${level}`} class="title">{title}</svelte:element>{/if}
				{#if description}<p class="description">{description}</p>{/if}
			</div>
			{#if actions}<div class="actions">{@render actions()}</div>{/if}
		</header>
	{/if}
	{#if children}<div class="body">{@render children()}</div>{/if}
	{#if footer}<footer>{@render footer()}</footer>{/if}
</svelte:element>

<style>
	.tern-card {
		display: grid;
		gap: 1rem;
		padding: 1.25rem;
		background: var(--tern-surface);
		color: var(--tern-fg);
		border: 1px solid var(--tern-border);
		border-radius: calc(var(--tern-radius) + 2px);
		box-shadow: var(--tern-shadow);
	}
	header {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: 1rem;
	}
	.title {
		margin: 0;
		font-size: 1rem;
		font-weight: 650;
		letter-spacing: -0.01em;
	}
	.description {
		margin: 0.2rem 0 0;
		font-size: 0.875rem;
		color: var(--tern-muted);
	}
	.actions {
		display: flex;
		gap: 0.5rem;
	}
	footer {
		display: flex;
		justify-content: flex-end;
		gap: 0.5rem;
		padding-top: 1rem;
		border-top: 1px solid var(--tern-border);
	}
</style>

<script lang="ts">
	import type { Snippet } from 'svelte';

	type Props = {
		title: string;
		description?: string;
		/** Makes the whole item a link; actions stay separate buttons. */
		href?: string;
		/** An avatar, icon or thumbnail at the start. */
		media?: Snippet;
		/** Buttons or a badge at the end. */
		actions?: Snippet;
		/** li inside a ul or ol, div elsewhere. */
		as?: 'div' | 'li';
		class?: string;
	};
	let { title, description, href, media, actions, as = 'div', class: klass = '' }: Props = $props();
</script>

<!-- A row with media, a title, a description and actions, for lists of people, files or settings. -->
<svelte:element this={as} class="tern-item {klass}" class:link={!!href}>
	{#if media}<div class="media">{@render media()}</div>{/if}
	<div class="text">
		{#if href}<a {href} class="title">{title}</a>{:else}<span class="title">{title}</span>{/if}
		{#if description}<span class="description">{description}</span>{/if}
	</div>
	{#if actions}<div class="actions">{@render actions()}</div>{/if}
</svelte:element>

<style>
	.tern-item {
		position: relative;
		display: flex;
		align-items: center;
		gap: 0.75rem;
		padding: 0.75rem;
		border-radius: var(--tern-radius-sm);
		list-style: none;
	}
	.link:hover {
		background: var(--tern-surface-2);
	}
	.text {
		display: grid;
		flex: 1;
		min-width: 0;
	}
	.title {
		font-weight: 550;
		color: var(--tern-fg);
		text-decoration: none;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	/* The link covers the item, so it's one big target. */
	a.title::after {
		content: '';
		position: absolute;
		inset: 0;
		border-radius: inherit;
	}
	a.title:focus-visible::after {
		outline: 2px solid var(--tern-ring);
		outline-offset: -2px;
	}
	a.title:focus-visible {
		outline: none;
	}
	.description {
		font-size: 0.8125rem;
		color: var(--tern-muted);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.actions {
		position: relative;
		z-index: 1;
		display: flex;
		gap: 0.375rem;
	}
	.media {
		flex: none;
	}
</style>

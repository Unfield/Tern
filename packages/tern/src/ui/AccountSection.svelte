<script lang="ts">
	// The section the account settings lists share. Internal: not exported.
	import type { Snippet } from 'svelte';

	type Props = { id: string; title: string; description?: string; level?: 2 | 3 | 4; class?: string; children: Snippet };
	let { id, title, description, level = 2, class: klass = '', children }: Props = $props();
</script>

<section class="tern-account {klass}" aria-labelledby={id}>
	<header>
		<svelte:element this={`h${level}`} {id}>{title}</svelte:element>
		{#if description}<p>{description}</p>{/if}
	</header>
	{@render children()}
</section>

<style>
	.tern-account {
		display: grid;
		gap: 1rem;
		color: var(--tern-fg);
	}
	header {
		display: grid;
		gap: 0.25rem;
	}
	:is(h2, h3, h4) {
		margin: 0;
		font-size: 1.0625rem;
		font-weight: 650;
	}
	header p,
	.tern-account :global(.tern-account-empty) {
		margin: 0;
		color: var(--tern-muted);
		font-size: 0.875rem;
	}
	.tern-account :global(ul) {
		margin: 0;
		padding: 0;
		list-style: none;
		border: 1px solid var(--tern-border);
		border-radius: var(--tern-radius);
		background: var(--tern-surface);
	}
	/* The text takes the row; when it would get narrower than 12rem, as
	   on a phone, the buttons move to a line of their own instead of
	   squeezing it. */
	.tern-account :global(li) {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: space-between;
		gap: 0.6rem 1rem;
		padding: 0.75rem 1rem;
	}
	/* In a Card, the card is the frame: rows line up with its content. */
	:global(.tern-card) .tern-account :global(ul) {
		border: 0;
		border-radius: 0;
		background: none;
	}
	:global(.tern-card) .tern-account :global(li) {
		padding-inline: 0;
	}
	.tern-account :global(li + li) {
		border-top: 1px solid var(--tern-border);
	}
	.tern-account :global(.tern-account-text) {
		display: grid;
		flex: 1 1 12rem;
		gap: 0.15rem;
		min-width: 0;
	}
	/* A badge after the name goes under it when they don't fit side by
	   side (flex lines break by the name's whole width), and only a word
	   too long for the line, such as an email address, breaks. */
	.tern-account :global(.tern-account-name) {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.25rem 0.5rem;
		font-weight: 550;
		overflow-wrap: anywhere;
		min-width: 0;
	}
	.tern-account :global(.tern-account-meta) {
		color: var(--tern-muted);
		font-size: 0.8125rem;
	}
	.tern-account :global(.tern-form) {
		display: contents;
	}
</style>

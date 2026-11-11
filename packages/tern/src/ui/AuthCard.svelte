<script lang="ts">
	// The card the account screens share. Internal: not exported.
	import type { Snippet } from 'svelte';

	type Props = { id: string; title: string; description?: string; level?: 1 | 2 | 3; class?: string; children: Snippet };
	let { id, title, description, level = 1, class: klass = '', children }: Props = $props();
</script>

<section class="tern-auth {klass}" aria-labelledby={id}>
	<header>
		<svelte:element this={`h${level}`} {id}>{title}</svelte:element>
		{#if description}<p>{description}</p>{/if}
	</header>
	{@render children()}
</section>

<style>
	.tern-auth {
		display: grid;
		gap: 1.25rem;
		width: 100%;
		max-width: 24rem;
		box-sizing: border-box;
		padding: 2rem;
		color: var(--tern-fg);
		background: var(--tern-surface);
		border: 1px solid var(--tern-border);
		border-radius: calc(var(--tern-radius) + 4px);
		box-shadow: var(--tern-shadow);
	}
	header {
		display: grid;
		gap: 0.35rem;
	}
	:is(h1, h2, h3) {
		margin: 0;
		font-size: 1.375rem;
		font-weight: 650;
		letter-spacing: -0.02em;
	}
	header p {
		margin: 0;
		color: var(--tern-muted);
		font-size: 0.875rem;
	}
	/* Links in the text, not link buttons, which keep their own colors. */
	.tern-auth :global(a:not(.tern-btn)) {
		color: var(--tern-accent);
		font-weight: 550;
	}
	.tern-auth :global(a:not(.tern-btn):focus-visible) {
		outline: 2px solid var(--tern-ring);
		outline-offset: 2px;
		border-radius: 2px;
	}
	.tern-auth :global(.tern-form > .tern-btn) {
		width: 100%;
	}
	.tern-auth :global(.tern-auth-note) {
		margin: 0;
		text-align: center;
		font-size: 0.875rem;
		color: var(--tern-muted);
	}
</style>

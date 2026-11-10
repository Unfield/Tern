<script lang="ts">
	import type { Snippet } from 'svelte';

	type Props = {
		title: string;
		description?: string;
		/** The heading level, to fit the page's outline. */
		level?: 2 | 3 | 4;
		/** An illustration or icon above the title. */
		icon?: Snippet;
		/** What to do next, such as a Create button. */
		actions?: Snippet;
		class?: string;
	};
	let { title, description, level = 3, icon, actions, class: klass = '' }: Props = $props();
</script>

<!-- What a list or page shows when it has nothing yet: what it is for and what to do. -->
<div class="tern-empty {klass}">
	{#if icon}<div class="icon" aria-hidden="true">{@render icon()}</div>{/if}
	<svelte:element this={`h${level}`} class="title">{title}</svelte:element>
	{#if description}<p>{description}</p>{/if}
	{#if actions}<div class="actions">{@render actions()}</div>{/if}
</div>

<style>
	.tern-empty {
		display: grid;
		justify-items: center;
		gap: 0.5rem;
		padding: 2.5rem 1.5rem;
		text-align: center;
		border: 1px dashed var(--tern-border-strong);
		border-radius: var(--tern-radius);
	}
	.icon {
		display: grid;
		place-items: center;
		width: 2.75rem;
		height: 2.75rem;
		margin-bottom: 0.25rem;
		color: var(--tern-accent);
		background: var(--tern-accent-soft);
		border-radius: 50%;
	}
	.title {
		margin: 0;
		font-size: 1rem;
		font-weight: 600;
	}
	p {
		max-width: 28rem;
		margin: 0;
		color: var(--tern-muted);
		font-size: 0.875rem;
	}
	.actions {
		display: flex;
		gap: 0.5rem;
		margin-top: 0.5rem;
	}
</style>

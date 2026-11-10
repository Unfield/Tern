<script lang="ts">
	import type { Snippet } from 'svelte';

	type Props = {
		/** Whether it's open; bind it. */
		open?: boolean;
		/** The button's text. */
		label: string;
		children: Snippet;
		class?: string;
	};
	let { open = $bindable(false), label, children, class: klass = '' }: Props = $props();
	const uid = $props.id();
</script>

<!-- A button that shows and hides a region: the disclosure pattern. -->
<div class="tern-collapsible {klass}">
	<button type="button" aria-expanded={open} aria-controls="tern-collapsible-{uid}" onclick={() => (open = !open)}>
		<svg aria-hidden="true" viewBox="0 0 16 16" width="14" height="14"><path d="M6 4l4 4-4 4" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" /></svg>
		{label}
	</button>
	<div id="tern-collapsible-{uid}" class="content" hidden={!open}>
		{@render children()}
	</div>
</div>

<style>
	button {
		display: inline-flex;
		align-items: center;
		gap: 0.375rem;
		padding: 0.3rem 0.4rem 0.3rem 0.2rem;
		font: inherit;
		font-size: 0.875rem;
		font-weight: 550;
		color: var(--tern-fg);
		background: transparent;
		border: 0;
		border-radius: var(--tern-radius-sm);
		cursor: pointer;
	}
	button:hover {
		background: var(--tern-surface-2);
	}
	button:focus-visible {
		outline: 2px solid var(--tern-ring);
		outline-offset: 1px;
	}
	svg {
		color: var(--tern-muted);
		transition: rotate var(--tern-duration);
	}
	[aria-expanded='true'] svg {
		rotate: 90deg;
	}
	.content {
		padding: 0.5rem 0 0 1.4rem;
	}
</style>

<script lang="ts">
	import type { Snippet } from 'svelte';

	type Props = {
		/** Before the input: text such as "https://", an icon, or a button. */
		start?: Snippet;
		/** After the input: text such as ".com", a unit, or a button. */
		end?: Snippet;
		class?: string;
		/** The Input. */
		children: Snippet;
	};
	let { start, end, class: klass = '', children }: Props = $props();
</script>

<!-- An Input with text, icons or buttons inside its border. Text in start and end is read with the label when you add it to the Field's description, not on its own. -->
<div class="tern-input-group {klass}">
	{#if start}<span class="addon start">{@render start()}</span>{/if}
	{@render children()}
	{#if end}<span class="addon end">{@render end()}</span>{/if}
</div>

<style>
	.tern-input-group {
		display: flex;
		align-items: center;
		height: var(--tern-control-height);
		background: var(--tern-surface);
		border: 1px solid var(--tern-border-strong);
		border-radius: var(--tern-radius-sm);
		transition:
			border-color var(--tern-duration),
			box-shadow var(--tern-duration);
	}
	.tern-input-group:hover {
		border-color: var(--tern-muted);
	}
	.tern-input-group:has(:global(input:focus-visible)) {
		border-color: var(--tern-accent);
		box-shadow: 0 0 0 3px color-mix(in oklch, var(--tern-ring) 30%, transparent);
	}
	.tern-input-group:has(:global([aria-invalid='true'])) {
		border-color: var(--tern-danger);
	}
	.tern-input-group > :global(.tern-input) {
		flex: 1;
		min-width: 0;
		height: 100%;
		border: 0;
		box-shadow: none;
		background: transparent;
	}
	.tern-input-group > :global(.tern-input:focus-visible) {
		box-shadow: none;
	}
	.addon {
		display: inline-flex;
		align-items: center;
		gap: 0.25rem;
		height: 100%;
		color: var(--tern-muted);
		font-size: 0.875rem;
		white-space: nowrap;
	}
	.start {
		padding-inline-start: 0.75rem;
	}
	.end {
		padding-inline-end: 0.75rem;
	}
	.start + :global(.tern-input) {
		padding-inline-start: 0.4rem;
	}
	.addon :global(.tern-btn) {
		height: calc(var(--tern-control-height) - 0.6rem);
	}
	.end:has(:global(.tern-btn)) {
		padding-inline-end: 0.25rem;
	}
</style>

<script lang="ts">
	import type { HTMLTextareaAttributes } from 'svelte/elements';
	import { getField } from './field.js';

	type Props = HTMLTextareaAttributes & { value?: unknown };
	const field = getField();
	let { value = $bindable(field?.value), class: klass = '', rows = 4, ...rest }: Props = $props();
</script>

<textarea
	class="tern-textarea {klass}"
	id={field?.id}
	aria-describedby={field?.describedBy}
	aria-invalid={field?.invalid || undefined}
	required={field?.required}
	disabled={field?.disabled}
	{rows}
	name={field?.name}
	{...rest}
	bind:value
></textarea>

<style>
	.tern-textarea {
		width: 100%;
		box-sizing: border-box;
		padding: 0.5rem 0.75rem;
		font: inherit;
		font-size: 0.875rem;
		line-height: 1.5;
		color: var(--tern-fg);
		background: var(--tern-surface);
		border: 1px solid var(--tern-border-strong);
		border-radius: var(--tern-radius-sm);
		resize: vertical;
		transition:
			border-color var(--tern-duration),
			box-shadow var(--tern-duration);
	}
	.tern-textarea:focus-visible {
		outline: none;
		border-color: var(--tern-accent);
		box-shadow: 0 0 0 3px color-mix(in oklch, var(--tern-ring) 30%, transparent);
	}
	.tern-textarea[aria-invalid='true'] {
		border-color: var(--tern-danger);
	}
	.tern-textarea:disabled {
		opacity: 0.6;
		cursor: not-allowed;
	}
</style>

<script lang="ts">
	import type { HTMLInputAttributes } from 'svelte/elements';
	import { getField } from './field.js';

	type Props = HTMLInputAttributes & { value?: unknown };
	let { value = $bindable(), class: klass = '', ...rest }: Props = $props();
	const field = getField();
</script>

<input
	class="tern-input {klass}"
	id={field?.id}
	aria-describedby={field?.describedBy}
	aria-invalid={field?.invalid || undefined}
	required={field?.required}
	disabled={field?.disabled}
	{...rest}
	bind:value
/>

<style>
	.tern-input {
		width: 100%;
		box-sizing: border-box;
		height: var(--tern-control-height);
		padding: 0 0.75rem;
		font: inherit;
		font-size: 0.875rem;
		color: var(--tern-fg);
		background: var(--tern-surface);
		border: 1px solid var(--tern-border-strong);
		border-radius: var(--tern-radius-sm);
		box-shadow: inset 0 1px 1px oklch(0.24 0.02 240 / 0.04);
		transition:
			border-color var(--tern-duration),
			box-shadow var(--tern-duration);
	}
	.tern-input::placeholder {
		color: var(--tern-muted);
		opacity: 0.8;
	}
	.tern-input:hover:not(:disabled) {
		border-color: var(--tern-muted);
	}
	.tern-input:focus-visible {
		outline: none;
		border-color: var(--tern-accent);
		box-shadow: 0 0 0 3px color-mix(in oklch, var(--tern-ring) 30%, transparent);
	}
	.tern-input[aria-invalid='true'] {
		border-color: var(--tern-danger);
	}
	.tern-input[aria-invalid='true']:focus-visible {
		box-shadow: 0 0 0 3px color-mix(in oklch, var(--tern-danger) 25%, transparent);
	}
	.tern-input:disabled {
		opacity: 0.6;
		cursor: not-allowed;
		background: var(--tern-surface-2);
	}
</style>

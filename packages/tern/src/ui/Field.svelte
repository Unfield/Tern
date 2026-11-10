<script lang="ts">
	import type { Snippet } from 'svelte';
	import { setField } from './field.js';

	type Props = {
		/** The visible label. */
		label: string;
		/** Help text under the control, read by screen readers with it. */
		description?: string;
		/** An error message; marks the control invalid and announces the message. */
		error?: string | null;
		required?: boolean;
		disabled?: boolean;
		class?: string;
		/** The control, such as an Input, Textarea or Select. */
		children: Snippet;
	};
	let { label, description, error = null, required = false, disabled = false, class: klass = '', children }: Props = $props();

	const uid = $props.id();
	const id = `tern-field-${uid}`;
	setField({
		id,
		get describedBy() {
			return [description ? `${id}-description` : '', error ? `${id}-error` : ''].filter(Boolean).join(' ') || undefined;
		},
		get invalid() {
			return !!error;
		},
		get required() {
			return required;
		},
		get disabled() {
			return disabled;
		}
	});
</script>

<div class="tern-field {klass}" class:disabled>
	<label for={id}>
		{label}
		{#if required}<span class="required" aria-hidden="true">*</span>{/if}
	</label>
	{@render children()}
	{#if description}<p class="description" id="{id}-description">{description}</p>{/if}
	{#if error}<p class="error" id="{id}-error" role="alert">{error}</p>{/if}
</div>

<style>
	.tern-field {
		display: grid;
		gap: 0.375rem;
	}
	label {
		font-size: 0.875rem;
		font-weight: 550;
		color: var(--tern-fg);
	}
	.required {
		color: var(--tern-danger);
		margin-inline-start: 0.125rem;
	}
	.description {
		margin: 0;
		font-size: 0.8125rem;
		color: var(--tern-muted);
	}
	.error {
		margin: 0;
		font-size: 0.8125rem;
		color: var(--tern-danger);
		font-weight: 500;
	}
	.disabled label {
		opacity: 0.6;
	}
</style>

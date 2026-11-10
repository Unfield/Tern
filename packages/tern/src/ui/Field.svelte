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
	let custom = $state(false);
	const id = `tern-field-${uid}`;
	setField({
		id,
		labelId: `${id}-label`,
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
		},
		custom() {
			custom = true;
		}
	});
</script>

<div class="tern-field {klass}" class:disabled>
	<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_noninteractive_element_interactions -->
	<label for={custom ? undefined : id} id="{id}-label" onclick={custom ? () => document.getElementById(id)?.focus() : undefined}>
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
		align-content: start;
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

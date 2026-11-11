<script lang="ts">
	import type { Snippet } from 'svelte';
	import { setField } from './field.js';
	import { getForm } from './form.js';

	type Props = {
		/** The visible label. */
		label: string;
		/** Help text under the control, read by screen readers with it. */
		description?: string;
		/** An error message; marks the control invalid and announces the message. */
		error?: string | null;
		required?: boolean;
		disabled?: boolean;
		/** The control's name. Inside a Form, the Field shows that field's error and knows if it's required, and the control posts under it. */
		name?: string;
		class?: string;
		/** The control, such as an Input, Textarea or Select. */
		children: Snippet;
	};
	let { label, description, error: errorProp = null, required: requiredProp = false, disabled = false, name, class: klass = '', children }: Props = $props();

	const form = getForm();
	const error = $derived(errorProp ?? (name ? form?.error(name) : undefined) ?? null);
	const required = $derived(requiredProp || (name ? !!form?.field(name)?.required : false));

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
		},
		get name() {
			return name;
		},
		get value() {
			return name && !form?.field(name)?.secret ? form?.value(name) : undefined;
		}
	});
</script>

<div class="tern-field {klass}" class:disabled data-tern-field={name}>
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

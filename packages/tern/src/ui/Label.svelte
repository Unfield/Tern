<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLLabelAttributes } from 'svelte/elements';

	type Props = HTMLLabelAttributes & {
		/** Marks the control as required, with an asterisk screen readers skip (the control says it's required). */
		required?: boolean;
		children: Snippet;
	};
	let { required = false, class: klass = '', children, ...rest }: Props = $props();
</script>

<!-- A label on its own, for layouts Field doesn't fit. Point it at the control with for. -->
<label class="tern-label {klass}" {...rest}>
	{@render children()}
	{#if required}<span class="required" aria-hidden="true">*</span>{/if}
</label>

<style>
	.tern-label {
		font-size: 0.875rem;
		font-weight: 550;
		color: var(--tern-fg);
	}
	.required {
		color: var(--tern-danger);
		margin-inline-start: 0.125rem;
	}
</style>

<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLSelectAttributes } from 'svelte/elements';
	import { getField } from './field.js';

	type Option = { value: string; label: string; disabled?: boolean };
	type Props = HTMLSelectAttributes & {
		value?: unknown;
		/** The options, or pass <option> elements as children. */
		options?: Option[];
		/** Shown, and not selectable, while nothing is chosen. */
		placeholder?: string;
		children?: Snippet;
	};
	const field = getField();
	let { value = $bindable(field?.value), options, placeholder, class: klass = '', children, ...rest }: Props = $props();
</script>

<span class="tern-select {klass}">
	<select
		id={field?.id}
		aria-describedby={field?.describedBy}
		aria-invalid={field?.invalid || undefined}
		required={field?.required}
		disabled={field?.disabled}
		name={field?.name}
	{...rest}
		bind:value
	>
		{#if placeholder}<option value="" disabled selected={value === undefined || value === ''}>{placeholder}</option>{/if}
		{#each options ?? [] as o (o.value)}
			<option value={o.value} disabled={o.disabled}>{o.label}</option>
		{/each}
		{@render children?.()}
	</select>
	<svg aria-hidden="true" viewBox="0 0 16 16" width="16" height="16"><path d="M4 6l4 4 4-4" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" /></svg>
</span>

<style>
	.tern-select {
		position: relative;
		display: block;
	}
	select {
		appearance: none;
		width: 100%;
		height: var(--tern-control-height);
		padding: 0 2.25rem 0 0.75rem;
		font: inherit;
		font-size: 0.875rem;
		color: var(--tern-fg);
		background: var(--tern-surface);
		border: 1px solid var(--tern-border-strong);
		border-radius: var(--tern-radius-sm);
		cursor: pointer;
	}
	select:focus-visible {
		outline: none;
		border-color: var(--tern-accent);
		box-shadow: 0 0 0 3px color-mix(in oklch, var(--tern-ring) 30%, transparent);
	}
	select[aria-invalid='true'] {
		border-color: var(--tern-danger);
	}
	select:disabled {
		opacity: 0.6;
		cursor: not-allowed;
	}
	svg {
		position: absolute;
		right: 0.7rem;
		top: 50%;
		translate: 0 -50%;
		pointer-events: none;
		color: var(--tern-muted);
	}
</style>

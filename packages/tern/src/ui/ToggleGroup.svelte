<script lang="ts">
	import type { Snippet } from 'svelte';
	import { getField } from './field.js';

	type Option = { value: string; label: string; disabled?: boolean; icon?: Snippet };
	type Props = {
		options: Option[];
		/** single: one at a time, like radio buttons; multiple: any number, like checkboxes. */
		type?: 'single' | 'multiple';
		/** The chosen value (single) or values (multiple); bind it. */
		value?: string | string[] | null;
		/** Labels the group when it isn't in a Field. */
		label?: string;
		/** Show only the icons; the labels stay for screen readers. */
		iconsOnly?: boolean;
		/** Posts the chosen values under this name. */
		name?: string;
		size?: 'sm' | 'md';
		disabled?: boolean;
		class?: string;
		onchange?: (value: string | string[] | null) => void;
	};
	const field = getField();
	let { options, type = 'single', value = $bindable(type === 'multiple' ? [] : null), label, iconsOnly = false, name: nameProp, size = 'md', disabled = false, class: klass = '', onchange }: Props = $props();
	const name = $derived(nameProp ?? field?.name);

	field?.custom();
	const buttons: HTMLButtonElement[] = [];
	const chosen = (v: string) => (Array.isArray(value) ? value.includes(v) : value === v);
	const enabled = $derived(options.map((o, i) => ({ o, i })).filter(({ o }) => !o.disabled && !disabled));
	// Roving tabindex: the chosen item, else the first enabled one.
	const tabStop = $derived(enabled.find(({ o }) => chosen(o.value))?.i ?? enabled[0]?.i ?? -1);

	function pick(v: string) {
		if (type === 'multiple') {
			const list = Array.isArray(value) ? value : [];
			value = list.includes(v) ? list.filter((x) => x !== v) : [...list, v];
		} else value = value === v ? null : v;
		onchange?.(value);
	}

	function onkeydown(e: KeyboardEvent, i: number) {
		const pos = enabled.findIndex((x) => x.i === i);
		let next: number | undefined;
		if (e.key === 'ArrowRight' || e.key === 'ArrowDown') next = enabled[(pos + 1) % enabled.length]?.i;
		else if (e.key === 'ArrowLeft' || e.key === 'ArrowUp') next = enabled[(pos - 1 + enabled.length) % enabled.length]?.i;
		else if (e.key === 'Home') next = enabled[0]?.i;
		else if (e.key === 'End') next = enabled[enabled.length - 1]?.i;
		if (next === undefined) return;
		e.preventDefault();
		buttons[next]?.focus();
		// Like radio buttons, moving selects in single mode.
		if (type === 'single') {
			value = options[next]!.value;
			onchange?.(value);
		}
	}
</script>

<div
	class="tern-toggle-group {size} {klass}"
	role={type === 'single' ? 'radiogroup' : 'group'}
	aria-label={field ? undefined : label}
	aria-labelledby={field?.labelId}
	aria-describedby={field?.describedBy}
>
	{#each options as option, i (option.value)}
		<button
			bind:this={buttons[i]}
			id={i === tabStop ? field?.id : undefined}
			type="button"
			role={type === 'single' ? 'radio' : undefined}
			aria-checked={type === 'single' ? chosen(option.value) : undefined}
			aria-pressed={type === 'multiple' ? chosen(option.value) : undefined}
			aria-label={iconsOnly ? option.label : undefined}
			title={iconsOnly ? option.label : undefined}
			tabindex={i === tabStop ? 0 : -1}
			disabled={option.disabled || disabled || field?.disabled}
			onclick={() => pick(option.value)}
			onkeydown={(e) => onkeydown(e, i)}
		>
			{#if option.icon}{@render option.icon()}{/if}
			{#if !iconsOnly}{option.label}{/if}
		</button>
	{/each}
</div>
{#if name}
	{#each options.filter((o) => chosen(o.value)) as o (o.value)}<input type="hidden" {name} value={o.value} />{/each}
{/if}

<style>
	.tern-toggle-group {
		display: inline-flex;
		gap: 0.125rem;
		padding: 0.2rem;
		background: var(--tern-surface-2);
		border: 1px solid var(--tern-border);
		border-radius: var(--tern-radius);
		width: fit-content;
	}
	button {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		gap: 0.4rem;
		height: calc(var(--tern-control-height) - 0.4rem);
		min-width: calc(var(--tern-control-height) - 0.4rem);
		padding: 0 0.7rem;
		font: inherit;
		font-size: 0.875rem;
		font-weight: 550;
		color: var(--tern-muted);
		background: transparent;
		border: 0;
		border-radius: var(--tern-radius-sm);
		cursor: pointer;
		transition:
			background-color var(--tern-duration),
			color var(--tern-duration);
	}
	.sm button {
		height: calc(var(--tern-control-height) - 0.8rem);
		padding: 0 0.5rem;
		font-size: 0.8125rem;
	}
	button:hover:not(:disabled) {
		color: var(--tern-fg);
	}
	button[aria-checked='true'],
	button[aria-pressed='true'] {
		color: var(--tern-fg);
		background: var(--tern-surface);
		box-shadow: var(--tern-shadow);
	}
	button:focus-visible {
		outline: 2px solid var(--tern-ring);
		outline-offset: 1px;
	}
	button:disabled {
		opacity: 0.45;
		cursor: not-allowed;
	}
</style>

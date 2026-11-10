<script lang="ts">
	import Calendar, { type DateRange } from './Calendar.svelte';
	import Popover from './Popover.svelte';
	import { format, type ISODate } from './dates.js';
	import { getField } from './field.js';

	type Props = {
		mode?: 'single' | 'range';
		/** The chosen day (single) or { start, end } (range), as ISO dates; bind it. */
		value?: ISODate | null | DateRange;
		placeholder?: string;
		/** Labels it when it isn't in a Field. */
		label?: string;
		/** Posts the date as YYYY-MM-DD under this name; a range posts name and name + "_end". */
		name?: string;
		min?: ISODate;
		max?: ISODate;
		isDisabled?: (date: ISODate) => boolean;
		locale?: string;
		weekStart?: number;
		/** How the chosen date is shown. */
		display?: Intl.DateTimeFormatOptions;
		disabled?: boolean;
		class?: string;
		onchange?: (value: ISODate | DateRange) => void;
	};
	const field = getField();
	let {
		mode = 'single',
		value = $bindable(mode === 'range' ? { start: null, end: null } : null),
		placeholder = mode === 'range' ? 'Pick dates' : 'Pick a date',
		label,
		name: nameProp,
		min,
		max,
		isDisabled,
		locale,
		weekStart,
		display = { year: 'numeric', month: 'short', day: 'numeric' },
		disabled = false,
		class: klass = '',
		onchange
	}: Props = $props();
	const name = $derived(nameProp ?? field?.name);

	const uid = $props.id();
	field?.custom();
	let open = $state(false);
	const triggerId = field?.id ?? `tern-datepicker-${uid}-trigger`;

	const text = $derived.by(() => {
		if (mode === 'single') return value ? format(value as ISODate, locale, display) : '';
		const r = value as DateRange | null;
		if (!r?.start) return '';
		return `${format(r.start, locale, display)} – ${r.end ? format(r.end, locale, display) : '…'}`;
	});
</script>

<Popover bind:open label={label ?? 'Choose a date'} align="start" {triggerId}>
	{#snippet trigger(props)}
		<button
			type="button"
			class="tern-datepicker {klass}"
			{...props}
			aria-labelledby="{field?.labelId ?? `tern-datepicker-${uid}-label`} tern-datepicker-{uid}-value"
			aria-describedby={field?.describedBy}
			disabled={disabled || field?.disabled}
		>
			<svg aria-hidden="true" viewBox="0 0 16 16" width="16" height="16" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round"><rect x="2.5" y="3.5" width="11" height="10" rx="2" /><path d="M2.5 6.5h11M5.5 2v3M10.5 2v3" /></svg>
			<span class:placeholder={!text} id="tern-datepicker-{uid}-value">{text || placeholder}</span>
		</button>
		{#if !field}<span class="tern-sr-only" id="tern-datepicker-{uid}-label">{label ?? 'Date'}</span>{/if}
	{/snippet}
	<Calendar
		bind:value
		{mode}
		{min}
		{max}
		{isDisabled}
		{locale}
		{weekStart}
		label={label ?? 'Choose a date'}
		onselect={(v) => {
			onchange?.(v);
			open = false;
			document.getElementById(triggerId)?.focus();
		}}
	/>
</Popover>
{#if name}
	{#if mode === 'single'}
		<input type="hidden" {name} value={(value as ISODate | null) ?? ''} />
	{:else}
		<input type="hidden" {name} value={(value as DateRange | null)?.start ?? ''} />
		<input type="hidden" name="{name}_end" value={(value as DateRange | null)?.end ?? ''} />
	{/if}
{/if}

<style>
	.tern-datepicker {
		display: inline-flex;
		align-items: center;
		gap: 0.5rem;
		width: 100%;
		height: var(--tern-control-height);
		padding: 0 0.75rem;
		font: inherit;
		font-size: 0.875rem;
		color: var(--tern-fg);
		text-align: start;
		background: var(--tern-surface);
		border: 1px solid var(--tern-border-strong);
		border-radius: var(--tern-radius-sm);
		cursor: pointer;
	}
	.tern-datepicker svg {
		flex: none;
		color: var(--tern-muted);
	}
	.tern-datepicker:hover:not(:disabled) {
		border-color: var(--tern-muted);
	}
	.tern-datepicker:focus-visible {
		outline: none;
		border-color: var(--tern-accent);
		box-shadow: 0 0 0 3px color-mix(in oklch, var(--tern-ring) 30%, transparent);
	}
	.tern-datepicker[aria-invalid='true'] {
		border-color: var(--tern-danger);
	}
	.tern-datepicker:disabled {
		opacity: 0.6;
		cursor: not-allowed;
	}
	.placeholder {
		color: var(--tern-muted);
	}
</style>

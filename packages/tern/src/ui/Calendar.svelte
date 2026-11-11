<script lang="ts">
	import { tb } from '../runtime/i18n.svelte.js';
	import { tick } from 'svelte';
	import { addDays, addMonths, format, localeWeekStart, monthGrid, startOfMonth, today, type ISODate } from './dates.js';

	export type DateRange = { start: ISODate | null; end: ISODate | null };

	type Props = {
		/** single picks a day; range picks a start and an end. */
		mode?: 'single' | 'range';
		/** The chosen day (single) or { start, end } (range); bind it. */
		value?: ISODate | null | DateRange;
		/** The earliest and latest days that can be picked. */
		min?: ISODate;
		max?: ISODate;
		/** Other days that can't be picked, such as weekends. */
		isDisabled?: (date: ISODate) => boolean;
		/** The locale for month and day names; the browser's by default. */
		locale?: string;
		/** The first day of the week, 0 for Sunday; the locale's by default. */
		weekStart?: number;
		/** Names the calendar for screen readers. */
		label?: string;
		class?: string;
		/** Runs when a day (single) or a whole range is picked. */
		onselect?: (value: ISODate | DateRange) => void;
	};
	let { mode = 'single', value = $bindable(mode === 'range' ? { start: null, end: null } : null), min, max, isDisabled, locale, weekStart, label, class: klass = '', onselect }: Props = $props();

	const uid = $props.id();
	const id = `tern-calendar-${uid}`;
	const first = $derived(weekStart ?? localeWeekStart(locale));
	const range = $derived(mode === 'range' ? ((value as DateRange | null) ?? { start: null, end: null }) : null);
	const single = $derived(mode === 'single' ? (value as ISODate | null) : null);
	const now = today();
	let focused = $state<ISODate>(today());
	let hovered = $state<ISODate | null>(null);
	let grid = $state<HTMLTableElement>();

	// Start where the value is.
	$effect.pre(() => {
		const start = single ?? range?.start;
		if (start) focused = start;
	});

	const weeks = $derived(monthGrid(focused, first));
	const month = $derived(focused.slice(0, 7));
	const weekdays = $derived(weeks[0]!.map((d) => ({ short: format(d, locale, { weekday: 'short' }), long: format(d, locale, { weekday: 'long' }) })));

	const blocked = (d: ISODate) => (min !== undefined && d < min) || (max !== undefined && d > max) || !!isDisabled?.(d);
	const selected = (d: ISODate) => (single ? d === single : !!range && (d === range.start || d === range.end));
	const between = (d: ISODate) => {
		if (!range?.start) return false;
		const end = range.end ?? hovered;
		if (!end) return false;
		const [a, b] = range.start <= end ? [range.start, end] : [end, range.start];
		return d > a && d < b;
	};

	async function moveTo(d: ISODate) {
		if (min && d < min) d = min;
		if (max && d > max) d = max;
		focused = d;
		await tick();
		grid?.querySelector<HTMLElement>(`[data-date="${d}"]`)?.focus();
	}

	function pick(d: ISODate) {
		if (blocked(d)) return;
		focused = d;
		if (mode === 'single') {
			value = d;
			onselect?.(d);
			return;
		}
		const r = range!;
		if (!r.start || r.end) value = { start: d, end: null };
		else {
			const next = d < r.start ? { start: d, end: r.start } : { start: r.start, end: d };
			value = next;
			onselect?.(next);
		}
	}

	function onkeydown(e: KeyboardEvent) {
		const dow = (new Date(`${focused}T00:00:00Z`).getUTCDay() - first + 7) % 7;
		const moves: Record<string, () => ISODate> = {
			ArrowRight: () => addDays(focused, 1),
			ArrowLeft: () => addDays(focused, -1),
			ArrowDown: () => addDays(focused, 7),
			ArrowUp: () => addDays(focused, -7),
			Home: () => addDays(focused, -dow),
			End: () => addDays(focused, 6 - dow),
			PageDown: () => addMonths(focused, e.shiftKey ? 12 : 1),
			PageUp: () => addMonths(focused, e.shiftKey ? -12 : -1)
		};
		if (e.key in moves) {
			e.preventDefault();
			moveTo(moves[e.key]!());
		} else if (e.key === 'Enter' || e.key === ' ') {
			e.preventDefault();
			pick(focused);
		}
	}

	const canPrev = $derived(!min || startOfMonth(focused) > min);
	const canNext = $derived(!max || addMonths(startOfMonth(focused), 1) <= max);
</script>

<div class="tern-calendar {klass}" role="group" aria-label={label}>
	<div class="header">
		<button type="button" class="nav" aria-label={tb('tern.ui.calendar.previousMonth')} disabled={!canPrev} onclick={() => (focused = addMonths(focused, -1))}>
			<svg aria-hidden="true" viewBox="0 0 16 16" width="16" height="16"><path d="M10 4l-4 4 4 4" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" /></svg>
		</button>
		<div class="title" id="{id}-title" aria-live="polite">{format(focused, locale, { month: 'long', year: 'numeric' })}</div>
		<button type="button" class="nav" aria-label={tb('tern.ui.calendar.nextMonth')} disabled={!canNext} onclick={() => (focused = addMonths(focused, 1))}>
			<svg aria-hidden="true" viewBox="0 0 16 16" width="16" height="16"><path d="M6 4l4 4-4 4" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" /></svg>
		</button>
	</div>
	<table bind:this={grid} role="grid" aria-labelledby="{id}-title" aria-multiselectable={mode === 'range' || undefined}>
		<thead>
			<tr>
				{#each weekdays as w (w.long)}<th scope="col" abbr={w.long}>{w.short}</th>{/each}
			</tr>
		</thead>
		<tbody>
			{#each weeks as week (week[0])}
				<tr>
					{#each week as d (d)}
						{@const outside = d.slice(0, 7) !== month}
						<td
							aria-selected={selected(d)}
							class:start={range?.start === d && !!(range.end || hovered)}
							class:end={range?.end === d || (!range?.end && hovered === d && range?.start && range.start < d)}
							class:between={between(d)}
						>
							<button
								type="button"
								data-date={d}
								tabindex={d === focused ? 0 : -1}
								class="day"
								class:outside
								class:selected={selected(d)}
								aria-label={format(d, locale, { weekday: 'long', year: 'numeric', month: 'long', day: 'numeric' })}
								aria-current={d === now ? 'date' : undefined}
								aria-disabled={blocked(d) || undefined}
								onclick={() => pick(d)}
								onfocus={() => (focused = d)}
								onpointerenter={() => (hovered = d)}
								onpointerleave={() => (hovered = null)}
								{onkeydown}
							>
								{Number(d.slice(8))}
							</button>
						</td>
					{/each}
				</tr>
			{/each}
		</tbody>
	</table>
</div>

<style>
	.tern-calendar {
		display: inline-grid;
		gap: 0.5rem;
		color: var(--tern-fg);
		font-size: 0.875rem;
	}
	.header {
		display: flex;
		align-items: center;
		justify-content: space-between;
	}
	.title {
		font-weight: 600;
	}
	.nav {
		display: grid;
		place-items: center;
		width: 2rem;
		height: 2rem;
		color: var(--tern-muted);
		background: transparent;
		border: 1px solid var(--tern-border);
		border-radius: var(--tern-radius-sm);
		cursor: pointer;
	}
	.nav:hover:not(:disabled) {
		color: var(--tern-fg);
		background: var(--tern-surface-2);
	}
	.nav:disabled {
		opacity: 0.4;
		cursor: not-allowed;
	}
	.nav:focus-visible {
		outline: 2px solid var(--tern-ring);
		outline-offset: 1px;
	}
	table {
		border-collapse: collapse;
	}
	th {
		width: 2.25rem;
		padding-bottom: 0.25rem;
		font-size: 0.75rem;
		font-weight: 500;
		color: var(--tern-muted);
	}
	td {
		padding: 1px 0;
	}
	td.between,
	td.start,
	td.end {
		background: var(--tern-accent-soft);
	}
	td.start {
		border-radius: 50% 0 0 50%;
	}
	td.end {
		border-radius: 0 50% 50% 0;
	}
	td.start.end {
		border-radius: 50%;
	}
	.day {
		display: grid;
		place-items: center;
		width: 2.25rem;
		height: 2.25rem;
		padding: 0;
		font: inherit;
		font-variant-numeric: tabular-nums;
		color: var(--tern-fg);
		background: transparent;
		border: 0;
		border-radius: 50%;
		cursor: pointer;
	}
	.day:hover:not([aria-disabled='true']):not(.selected) {
		background: var(--tern-surface-2);
	}
	.day.outside {
		color: var(--tern-muted);
	}
	.day[aria-current='date'] {
		font-weight: 700;
		box-shadow: inset 0 0 0 1px var(--tern-border-strong);
	}
	.day.selected {
		color: var(--tern-accent-fg);
		background: var(--tern-accent);
	}
	.day[aria-disabled='true'] {
		color: var(--tern-muted);
		opacity: 0.45;
		text-decoration: line-through;
		cursor: not-allowed;
	}
	.day:focus-visible {
		outline: 2px solid var(--tern-ring);
		outline-offset: 1px;
	}
</style>

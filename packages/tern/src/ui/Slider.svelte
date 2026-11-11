<script lang="ts">
	import { announce } from './announce.js';
	import { getField } from './field.js';

	type Props = {
		/** A number for one thumb, or [low, high] for a range; bind it. */
		value?: number | [number, number];
		min?: number;
		max?: number;
		step?: number;
		/** Labels it when it isn't in a Field. */
		label?: string;
		/** Formats values for screen readers and the tooltip, such as (v) => `${v}%`. */
		format?: (v: number) => string;
		/** Posts the value (or both values) under this name. */
		name?: string;
		disabled?: boolean;
		class?: string;
		onchange?: (value: number | [number, number]) => void;
	};
	const field = getField();
	let { value = $bindable(0), min = 0, max = 100, step = 1, label, format = String, name: nameProp, disabled = false, class: klass = '', onchange }: Props = $props();
	const name = $derived(nameProp ?? field?.name);

	const uid = $props.id();
	field?.custom();
	let track = $state<HTMLDivElement>();
	const thumbs: HTMLElement[] = [];
	let dragging = $state<number | null>(null);

	const values = $derived<number[]>(Array.isArray(value) ? value : [value]);
	const range = $derived(Array.isArray(value));
	const off = $derived(disabled || !!field?.disabled);
	const pct = (v: number) => ((v - min) / (max - min || 1)) * 100;

	function snap(v: number) {
		const s = Math.round((v - min) / step) * step + min;
		const decimals = (String(step).split('.')[1] ?? '').length;
		return Math.min(max, Math.max(min, Number(s.toFixed(decimals))));
	}

	function set(i: number, v: number) {
		v = snap(v);
		if (range) {
			const [lo, hi] = value as [number, number];
			const next: [number, number] = i === 0 ? [Math.min(v, hi), hi] : [lo, Math.max(v, lo)];
			if (next[0] === lo && next[1] === hi) return;
			value = next;
		} else {
			if (v === value) return;
			value = v;
		}
		onchange?.(value);
	}

	function fromPointer(e: PointerEvent) {
		const r = track!.getBoundingClientRect();
		return min + ((e.clientX - r.left) / r.width) * (max - min);
	}

	function down(e: PointerEvent) {
		if (off || !track) return;
		e.preventDefault();
		const v = fromPointer(e);
		// The nearest thumb moves; on a tie, the one that can go that way.
		const i = range ? (Math.abs(v - values[0]!) < Math.abs(v - values[1]!) || (v < values[0]! && values[0] === values[1]) ? 0 : 1) : 0;
		dragging = i;
		set(i, v);
		thumbs[i]?.focus();
		track.setPointerCapture(e.pointerId);
	}

	function onkeydown(e: KeyboardEvent, i: number) {
		const big = Math.max(step, (max - min) / 10);
		const v = values[i]!;
		const moves: Record<string, number> = {
			ArrowRight: v + step,
			ArrowUp: v + step,
			ArrowLeft: v - step,
			ArrowDown: v - step,
			PageUp: v + big,
			PageDown: v - big,
			Home: min,
			End: max
		};
		if (!(e.key in moves)) return;
		e.preventDefault();
		set(i, moves[e.key]!);
	}
</script>

<div class="tern-slider {klass}" class:disabled={off} use:announce={value}>
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		bind:this={track}
		class="track"
		onpointerdown={down}
		onpointermove={(e) => dragging !== null && set(dragging, fromPointer(e))}
		onpointerup={() => (dragging = null)}
		onpointercancel={() => (dragging = null)}
	>
		<div class="fill" style:left="{range ? pct(values[0]!) : 0}%" style:right="{100 - pct(values[values.length - 1]!)}%"></div>
		{#each values as v, i (i)}
			<span
				bind:this={thumbs[i]}
				class="thumb"
				class:active={dragging === i}
				id={i === 0 ? field?.id : undefined}
				role="slider"
				tabindex={off ? -1 : 0}
				style:left="{pct(v)}%"
				aria-valuemin={range && i === 1 ? values[0] : min}
				aria-valuemax={range && i === 0 ? values[1] : max}
				aria-valuenow={v}
				aria-valuetext={format(v)}
				aria-disabled={off || undefined}
				aria-label={field || !label ? undefined : range ? `${label}, ${i === 0 ? 'minimum' : 'maximum'}` : label}
				aria-labelledby={field ? (range ? `${field.labelId} tern-slider-${uid}-${i}` : field.labelId) : undefined}
				aria-describedby={field?.describedBy}
				onkeydown={(e) => onkeydown(e, i)}
			>
				<span class="value" aria-hidden="true">{format(v)}</span>
			</span>
			{#if range && field}<span class="tern-sr-only" id="tern-slider-{uid}-{i}">{i === 0 ? 'minimum' : 'maximum'}</span>{/if}
		{/each}
	</div>
	{#if name}
		{#each values as v, i (i)}<input type="hidden" {name} value={v} />{/each}
	{/if}
</div>

<style>
	.tern-slider {
		padding: 0.6rem 0.5rem;
		touch-action: none;
	}
	.track {
		position: relative;
		height: 0.375rem;
		border-radius: 1rem;
		background: var(--tern-surface-2);
		box-shadow: inset 0 0 0 1px var(--tern-border);
		cursor: pointer;
	}
	.fill {
		position: absolute;
		top: 0;
		bottom: 0;
		border-radius: inherit;
		background: var(--tern-accent);
	}
	.thumb {
		position: absolute;
		top: 50%;
		width: 1.125rem;
		height: 1.125rem;
		translate: -50% -50%;
		border-radius: 50%;
		background: var(--tern-surface);
		border: 2px solid var(--tern-accent);
		box-shadow: var(--tern-shadow);
		cursor: grab;
		outline: none;
		transition: box-shadow var(--tern-duration);
	}
	.thumb:focus-visible,
	.thumb.active {
		box-shadow: 0 0 0 4px color-mix(in oklch, var(--tern-ring) 35%, transparent);
	}
	.value {
		position: absolute;
		bottom: calc(100% + 0.4rem);
		left: 50%;
		translate: -50% 0;
		padding: 0.15rem 0.4rem;
		border-radius: var(--tern-radius-sm);
		background: var(--tern-fg);
		color: var(--tern-bg);
		font-size: 0.75rem;
		white-space: nowrap;
		opacity: 0;
		pointer-events: none;
		transition: opacity var(--tern-duration);
	}
	.thumb:focus-visible .value,
	.thumb.active .value,
	.thumb:hover .value {
		opacity: 1;
	}
	.disabled {
		opacity: 0.5;
	}
	.disabled .track,
	.disabled .thumb {
		cursor: not-allowed;
	}
</style>

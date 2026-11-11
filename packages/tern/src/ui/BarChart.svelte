<script lang="ts" generics="Row extends object">
	import { tb } from '../runtime/i18n.svelte.js';
	import ChartFrame from './ChartFrame.svelte';
	import { lastIndex, formatNumber, niceTicks, seriesColor, textWidth, type Series } from './chart.js';

	type Props = {
		data: Row[];
		/** The field naming each bar's category. */
		x: keyof Row & string;
		series: Series<Row>[];
		title: string;
		description?: string;
		level?: 2 | 3 | 4;
		/** Stack the series instead of grouping them side by side. */
		stacked?: boolean;
		/** Bars grow sideways, for long category names or many categories. */
		horizontal?: boolean;
		height?: number;
		format?: (v: number) => string;
		class?: string;
	};
	let { data, x, series, title, description, level, stacked = false, horizontal = false, height, format = (v) => formatNumber(v), class: klass = '' }: Props = $props();

	let width = $state(640);
	let active = $state<number | null>(null);

	const field = (r: Row, k: string) => (r as Record<string, unknown>)[k];
	const cats = $derived(data.map((r) => String(field(r, x) ?? '')));
	const values = $derived(
		data.map((r) =>
			series.map((s) => {
				const v = field(r, s.key);
				return typeof v === 'number' && Number.isFinite(v) ? Math.max(0, v) : 0;
			})
		)
	);
	const totals = $derived(values.map((v) => (stacked ? v.reduce((a, b) => a + b, 0) : Math.max(0, ...v))));
	const h = $derived(height ?? (horizontal ? Math.max(120, data.length * (stacked || series.length === 1 ? 32 : 18 * series.length + 14) + 30) : 240));
	const ticks = $derived(niceTicks(0, Math.max(0, ...totals), horizontal ? Math.max(2, Math.floor(width / 110)) : Math.max(3, Math.round(h / 60))));
	const max = $derived(ticks[ticks.length - 1]!);

	const top = 10;
	const labelW = $derived(horizontal ? Math.min(180, Math.max(...cats.map(textWidth)) + 12) : Math.max(...ticks.map((t) => textWidth(format(t)))) + 10);
	const left = $derived(labelW);
	const right = $derived(horizontal && series.length === 1 ? Math.max(...totals.map((t) => textWidth(format(t)))) + 10 : 12);
	const bottom = $derived(horizontal ? 22 : 24);
	const plotW = $derived(Math.max(10, width - left - right));
	const plotH = $derived(h - top - bottom);
	const band = $derived((horizontal ? plotH : plotW) / Math.max(1, data.length));
	const groups = $derived(stacked ? 1 : series.length);
	// Bars are at most 24px thick; the rest of the band is air.
	const thick = $derived(Math.max(2, Math.min(24, (band * 0.7 - (groups - 1) * 2) / groups)));
	const scale = (v: number) => (v / (max || 1)) * (horizontal ? plotW : plotH);

	type Rect = { s: number; x: number; y: number; w: number; h: number; end: boolean };
	const rects = $derived(
		values.map((v, i) => {
			const out: Rect[] = [];
			const groupW = groups * thick + (groups - 1) * 2;
			const start = (horizontal ? top : left) + i * band + (band - groupW) / 2;
			let acc = 0;
			const topSeries = lastIndex(v, (n) => n > 0);
			v.forEach((n, s) => {
				if (n <= 0) return;
				const len = scale(n);
				const pos = stacked ? start : start + s * (thick + 2);
				const from = scale(acc) + (stacked && acc > 0 ? 2 : 0);
				const l = Math.max(0, len - (stacked && acc > 0 ? 2 : 0));
				if (horizontal) out.push({ s, x: left + from, y: pos, w: l, h: thick, end: !stacked || s === topSeries });
				else out.push({ s, x: pos, y: top + plotH - from - l, w: thick, h: l, end: !stacked || s === topSeries });
				if (stacked) acc += n;
			});
			return out;
		})
	);

	// A bar with a 4px rounded data end and a square baseline end.
	function barPath(r: Rect) {
		const rad = r.end ? Math.min(4, horizontal ? r.h / 2 : r.w / 2, horizontal ? r.w : r.h) : 0;
		if (horizontal) return `M${r.x},${r.y}H${r.x + r.w - rad}Q${r.x + r.w},${r.y} ${r.x + r.w},${r.y + rad}V${r.y + r.h - rad}Q${r.x + r.w},${r.y + r.h} ${r.x + r.w - rad},${r.y + r.h}H${r.x}Z`;
		return `M${r.x},${r.y + r.h}V${r.y + rad}Q${r.x},${r.y} ${r.x + rad},${r.y}H${r.x + r.w - rad}Q${r.x + r.w},${r.y} ${r.x + r.w},${r.y + rad}V${r.y + r.h}Z`;
	}

	function onkeydown(e: KeyboardEvent) {
		const last = data.length - 1;
		const next = horizontal ? 'ArrowDown' : 'ArrowRight';
		const prev = horizontal ? 'ArrowUp' : 'ArrowLeft';
		const moves: Record<string, number> = { [next]: Math.min(last, (active ?? -1) + 1), [prev]: Math.max(0, (active ?? last + 1) - 1), Home: 0, End: last };
		if (e.key in moves) {
			e.preventDefault();
			active = moves[e.key]!;
		} else if (e.key === 'Escape') active = null;
	}
	function hit(e: PointerEvent, el: Element) {
		const r = el.getBoundingClientRect();
		const at = horizontal ? ((e.clientY - r.top) / r.height) * h - top : ((e.clientX - r.left) / r.width) * width - left;
		const i = Math.floor(at / band);
		active = i >= 0 && i < data.length ? i : null;
	}

	const describe = (i: number) => `${cats[i]}: ` + series.map((s, k) => `${s.label} ${format(values[i]![k]!)}`).join(', ') + (stacked && series.length > 1 ? `, total ${format(totals[i]!)}` : '');
	const table = $derived({
		headers: [x, ...series.map((s) => s.label), ...(stacked && series.length > 1 ? ['Total'] : [])],
		rows: data.map((_, i) => [cats[i]!, ...values[i]!.map(format), ...(stacked && series.length > 1 ? [format(totals[i]!)] : [])])
	});
	const labelEvery = $derived(horizontal ? 1 : Math.max(1, Math.ceil(data.length / Math.max(2, Math.floor(plotW / 64)))));
</script>

<ChartFrame {title} {description} {level} class={klass} legend={series.map((s, i) => ({ label: s.label, color: seriesColor(i), kind: 'rect' }))} {table} readout={active === null ? '' : describe(active)}>
	<!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions -->
	<div
		class="tern-plot"
		bind:clientWidth={width}
		role="group"
		aria-roledescription="chart"
		aria-label="{title}. Use the arrow keys to read values."
		tabindex="0"
		onpointermove={(e) => hit(e, e.currentTarget)}
		onpointerleave={() => (active = null)}
		onfocus={() => active === null && (active = 0)}
		onblur={() => (active = null)}
		{onkeydown}
	>
		<svg {width} height={h} viewBox="0 0 {width} {h}" aria-hidden="true">
			{#each ticks as t (t)}
				{#if horizontal}
					<line class="grid" x1={left + scale(t)} x2={left + scale(t)} y1={top} y2={top + plotH} />
					<text class="tick" x={left + scale(t)} y={h - 6} text-anchor="middle">{format(t)}</text>
				{:else}
					<line class="grid" x1={left} x2={left + plotW} y1={top + plotH - scale(t)} y2={top + plotH - scale(t)} />
					<text class="tick" x={left - 8} y={top + plotH - scale(t)} dy="0.32em" text-anchor="end">{format(t)}</text>
				{/if}
			{/each}
			{#each cats as c, i (i)}
				{#if active === i}
					<rect x={horizontal ? left : left + i * band} y={horizontal ? top + i * band : top} width={horizontal ? plotW : band} height={horizontal ? band : plotH} fill="var(--tern-surface-2)" />
				{/if}
				{#if horizontal}
					<text class="tick" x={left - 8} y={top + i * band + band / 2} dy="0.32em" text-anchor="end">{c.length > 26 ? c.slice(0, 25) + '…' : c}</text>
				{:else if i % labelEvery === 0}
					<text class="tick" x={left + i * band + band / 2} y={h - 6} text-anchor="middle">{c}</text>
				{/if}
			{/each}
			{#each rects as group, i (i)}
				{#each group as r (r.s)}
					<path d={barPath(r)} fill={seriesColor(r.s)} opacity={active === null || active === i ? 1 : 0.55} />
				{/each}
				{#if series.length === 1 && group[0]}
					{#if horizontal}
						<text class="direct" x={group[0].x + group[0].w + 6} y={group[0].y + group[0].h / 2} dy="0.32em">{format(totals[i]!)}</text>
					{:else if data.length <= 12 && band > textWidth(format(totals[i]!))}
						<text class="direct" x={group[0].x + group[0].w / 2} y={group[0].y - 6} text-anchor="middle">{format(totals[i]!)}</text>
					{/if}
				{/if}
			{/each}
		</svg>
		{#if active !== null}
			<div
				class="tooltip"
				style:left="{horizontal ? Math.min(left + scale(totals[active]!) + 12, width - 170) : Math.min(left + (active + 1) * band, width - 170)}px"
				style:top="{horizontal ? top + active * band : top}px"
			>
				<div class="head">{cats[active]}</div>
				{#each series as s, k (s.key)}
					<div class="row"><span class="swatch" style:background={seriesColor(k)}></span><strong>{format(values[active]![k]!)}</strong><span>{s.label}</span></div>
				{/each}
				{#if stacked && series.length > 1}<div class="row"><strong>{format(totals[active]!)}</strong><span>{tb('tern.ui.barChart.total')}</span></div>{/if}
			</div>
		{/if}
	</div>
</ChartFrame>

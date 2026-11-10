<script lang="ts" generics="Row extends Record<string, unknown>">
	import ChartFrame from './ChartFrame.svelte';
	import { lastIndex, formatNumber, formatX, isTime, niceTicks, numericX, seriesColor, textWidth, timeTicks, type Series, type XValue } from './chart.js';

	type Props = {
		data: Row[];
		/** The field on the x axis: dates (Date or ISO strings), numbers or categories. */
		x: keyof Row & string;
		/** Up to eight series; each is a field of numbers. */
		series: Series<Row>[];
		title: string;
		description?: string;
		level?: 2 | 3 | 4;
		/** Fill under the lines. */
		area?: boolean;
		/** Start the y axis at zero. Turn it off for values far from zero, such as a temperature. */
		zero?: boolean;
		height?: number;
		/** Formats y values on the axis, in tooltips and in the table. */
		format?: (v: number) => string;
		class?: string;
	};
	let { data, x, series, title, description, level, area = false, zero = true, height = 240, format = (v) => formatNumber(v), class: klass = '' }: Props = $props();

	let width = $state(640);
	let active = $state<number | null>(null);

	const xs = $derived(data.map((r) => r[x] as XValue));
	const time = $derived(isTime(xs));
	const nx = $derived(numericX(xs));
	const values = $derived(series.map((s) => data.map((r) => (typeof r[s.key] === 'number' && Number.isFinite(r[s.key]) ? (r[s.key] as number) : null))));
	const all = $derived(values.flat().filter((v): v is number => v !== null));
	const yTicks = $derived(niceTicks(zero || area ? Math.min(0, ...all) : Math.min(...all), Math.max(zero || area ? 0 : -Infinity, ...all), Math.max(3, Math.round(height / 60))));
	const yMin = $derived(yTicks[0]!);
	const yMax = $derived(yTicks[yTicks.length - 1]!);

	// Direct labels at the line ends for up to four series, unless they'd collide.
	const top = 10;
	const bottom = 24;
	const left = $derived(Math.max(...yTicks.map((t) => textWidth(format(t)))) + 10);
	const ends = $derived(values.map((v) => [...v].reverse().find((n) => n !== null) ?? null));
	const endY = (v: number) => top + (1 - (v - yMin) / (yMax - yMin || 1)) * (height - top - bottom);
	const direct = $derived.by(() => {
		if (series.length > 4) return false;
		const ys = ends.filter((e): e is number => e !== null).map(endY).sort((a, b) => a - b);
		return ys.every((y, i) => i === 0 || y - ys[i - 1]! >= 14);
	});
	const endLabels = $derived(series.map((s, i) => (series.length === 1 ? (ends[i] === null ? '' : format(ends[i]!)) : s.label)));
	const right = $derived(direct ? Math.max(...endLabels.map(textWidth)) + 14 : 8);

	const plotW = $derived(Math.max(10, width - left - right));
	const plotH = $derived(height - top - bottom);
	const xMin = $derived(nx ? Math.min(...nx) : 0);
	const xMax = $derived(nx ? Math.max(...nx) : Math.max(1, data.length - 1));
	const px = (i: number) => left + (((nx ? nx[i]! : i) - xMin) / (xMax - xMin || 1)) * plotW;
	const py = (v: number) => top + (1 - (v - yMin) / (yMax - yMin || 1)) * plotH;

	const xTicks = $derived.by(() => {
		if (time && nx) {
			const t = timeTicks(xMin, xMax, Math.max(2, Math.floor(plotW / 90)));
			return t.ticks.map((v) => ({ at: left + ((v - xMin) / (xMax - xMin || 1)) * plotW, label: t.format(v) }));
		}
		if (nx) return niceTicks(xMin, xMax, Math.max(2, Math.floor(plotW / 80))).filter((v) => v >= xMin && v <= xMax).map((v) => ({ at: left + ((v - xMin) / (xMax - xMin || 1)) * plotW, label: formatNumber(v) }));
		const every = Math.max(1, Math.ceil(data.length / Math.max(2, Math.floor(plotW / 70))));
		return xs.map((v, i) => ({ at: px(i), label: String(v), i })).filter((t) => t.i % every === 0);
	});

	function path(v: (number | null)[]) {
		let d = '';
		let pen = false;
		v.forEach((n, i) => {
			if (n === null) return void (pen = false);
			d += `${pen ? 'L' : 'M'}${px(i).toFixed(1)},${py(n).toFixed(1)}`;
			pen = true;
		});
		return d;
	}
	function areaPath(v: (number | null)[]) {
		const base = py(Math.max(yMin, Math.min(0, yMax)));
		const runs: string[] = [];
		let run: number[] = [];
		const flush = () => {
			if (run.length) runs.push(`M${px(run[0]!)},${base}` + run.map((i) => `L${px(i).toFixed(1)},${py(v[i]!).toFixed(1)}`).join('') + `L${px(run[run.length - 1]!)},${base}Z`);
			run = [];
		};
		v.forEach((n, i) => (n === null ? flush() : run.push(i)));
		flush();
		return runs.join('');
	}

	function nearest(clientX: number, el: Element) {
		const r = el.getBoundingClientRect();
		const at = ((clientX - r.left) / r.width) * width;
		let best = 0;
		for (let i = 1; i < data.length; i++) if (Math.abs(px(i) - at) < Math.abs(px(best) - at)) best = i;
		return best;
	}
	function onkeydown(e: KeyboardEvent) {
		const last = data.length - 1;
		const moves: Record<string, number> = { ArrowRight: Math.min(last, (active ?? -1) + 1), ArrowLeft: Math.max(0, (active ?? last + 1) - 1), Home: 0, End: last };
		if (e.key in moves) {
			e.preventDefault();
			active = moves[e.key]!;
		} else if (e.key === 'Escape') active = null;
	}

	const describe = (i: number) => `${formatX(xs[i]!, time)}: ` + series.map((s, k) => `${s.label} ${values[k]![i] === null ? 'no data' : format(values[k]![i]!)}`).join(', ');
	const table = $derived({
		headers: [x, ...series.map((s) => s.label)],
		rows: data.map((_, i) => [formatX(xs[i]!, time), ...values.map((v) => (v[i] === null ? '–' : format(v[i]!)))])
	});
</script>

<ChartFrame {title} {description} {level} class={klass} legend={series.map((s, i) => ({ label: s.label, color: seriesColor(i), kind: area ? 'rect' : 'line' }))} {table} readout={active === null ? '' : describe(active)}>
	<!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions -->
	<div
		class="tern-plot"
		bind:clientWidth={width}
		role="group"
		aria-roledescription="chart"
		aria-label="{title}. Use the arrow keys to read values."
		tabindex="0"
		onpointermove={(e) => (active = nearest(e.clientX, e.currentTarget))}
		onpointerleave={() => (active = null)}
		onfocus={() => active === null && (active = data.length - 1)}
		onblur={() => (active = null)}
		{onkeydown}
	>
		<svg {width} {height} viewBox="0 0 {width} {height}" aria-hidden="true">
			{#each yTicks as t (t)}
				<line class="grid" x1={left} x2={left + plotW} y1={py(t)} y2={py(t)} />
				<text class="tick" x={left - 8} y={py(t)} dy="0.32em" text-anchor="end">{format(t)}</text>
			{/each}
			{#each xTicks as t, i (i)}
				<text class="tick" x={t.at} y={height - 6} text-anchor={i === 0 && t.at - left < 20 ? 'start' : 'middle'}>{t.label}</text>
			{/each}
			{#each values as v, k (series[k]!.key)}
				{#if area}<path d={areaPath(v)} fill={seriesColor(k)} fill-opacity="0.1" />{/if}
				<path d={path(v)} fill="none" stroke={seriesColor(k)} stroke-width="2" stroke-linejoin="round" stroke-linecap="round" />
			{/each}
			{#if active !== null}
				<line class="grid" x1={px(active)} x2={px(active)} y1={top} y2={top + plotH} style="stroke: var(--tern-border-strong)" />
			{/if}
			{#each values as v, k (series[k]!.key)}
				{@const i = active ?? lastIndex(v, (n) => n !== null)}
				{#if i >= 0 && v[i] !== null}
					<circle cx={px(i)} cy={py(v[i]!)} r="4" fill={seriesColor(k)} stroke="var(--tern-surface)" stroke-width="2" />
				{/if}
				{#if direct && ends[k] !== null}
					<text class="direct" x={left + plotW + 10} y={py(ends[k]!)} dy="0.32em">{endLabels[k]}</text>
				{/if}
			{/each}
		</svg>
		{#if active !== null}
			<div class="tooltip" style:left="{Math.min(px(active) + 12, width - 170)}px" style:top="{top}px">
				<div class="head">{formatX(xs[active]!, time)}</div>
				{#each series as s, k (s.key)}
					<div class="row"><span class="swatch" style:background={seriesColor(k)}></span><strong>{values[k]![active] === null ? '–' : format(values[k]![active]!)}</strong><span>{s.label}</span></div>
				{/each}
			</div>
		{/if}
	</div>
</ChartFrame>

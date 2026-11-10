<script lang="ts" generics="Row extends object">
	import ChartFrame from './ChartFrame.svelte';
	import { formatNumber, seriesColor } from './chart.js';

	type Props = {
		data: Row[];
		/** The field naming each part. */
		label: keyof Row & string;
		/** The field with each part's amount. */
		value: keyof Row & string;
		title: string;
		description?: string;
		level?: 2 | 3 | 4;
		/** Parts after this many fold into "Other". */
		max?: number;
		/** The total's caption in the middle, such as "visits". */
		totalLabel?: string;
		size?: number;
		format?: (v: number) => string;
		class?: string;
	};
	let { data, label, value, title, description, level, max = 6, totalLabel = 'total', size = 180, format = (v) => formatNumber(v), class: klass = '' }: Props = $props();

	let active = $state<number | null>(null);

	// Biggest first; the tail folds into Other so colors stay distinct.
	const parts = $derived.by(() => {
		const rows = data
			.map((r) => {
				const o = r as Record<string, unknown>;
				return { label: String(o[label] ?? ''), value: Math.max(0, Number(o[value]) || 0) };
			})
			.sort((a, b) => b.value - a.value);
		if (rows.length <= max) return rows;
		const keep = rows.slice(0, max - 1);
		return [...keep, { label: 'Other', value: rows.slice(max - 1).reduce((a, r) => a + r.value, 0) }];
	});
	const total = $derived(parts.reduce((a, p) => a + p.value, 0));
	const r = $derived(size / 2);
	const inner = $derived(r * 0.62);

	const arcs = $derived.by(() => {
		let a = -Math.PI / 2;
		const gap = parts.length > 1 ? 2 / r : 0; // a 2px surface gap between parts
		return parts.map((p, i) => {
			const sweep = total ? (p.value / total) * Math.PI * 2 : 0;
			const from = a + gap / 2;
			const to = a + sweep - gap / 2;
			a += sweep;
			return { i, d: sweep > gap ? arc(from, to) : '' };
		});
	});
	function arc(from: number, to: number) {
		const large = to - from > Math.PI ? 1 : 0;
		const p = (rad: number, a: number) => `${(r + rad * Math.cos(a)).toFixed(2)},${(r + rad * Math.sin(a)).toFixed(2)}`;
		if (to - from >= Math.PI * 2 - 0.001) return `M${p(r, from)}A${r},${r} 0 1 1 ${p(r, from + Math.PI)}A${r},${r} 0 1 1 ${p(r, from)}M${p(inner, from)}A${inner},${inner} 0 1 0 ${p(inner, from + Math.PI)}A${inner},${inner} 0 1 0 ${p(inner, from)}Z`;
		return `M${p(r, from)}A${r},${r} 0 ${large} 1 ${p(r, to)}L${p(inner, to)}A${inner},${inner} 0 ${large} 0 ${p(inner, from)}Z`;
	}
	const pct = (v: number) => (total ? Math.round((v / total) * 1000) / 10 : 0);

	function onkeydown(e: KeyboardEvent) {
		const last = parts.length - 1;
		const moves: Record<string, number> = { ArrowRight: Math.min(last, (active ?? -1) + 1), ArrowDown: Math.min(last, (active ?? -1) + 1), ArrowLeft: Math.max(0, (active ?? 1) - 1), ArrowUp: Math.max(0, (active ?? 1) - 1), Home: 0, End: last };
		if (e.key in moves) {
			e.preventDefault();
			active = moves[e.key]!;
		} else if (e.key === 'Escape') active = null;
	}
	const table = $derived({ headers: [label, value, 'Share'], rows: parts.map((p) => [p.label, format(p.value), `${pct(p.value)}%`]) });
</script>

<ChartFrame
	{title}
	{description}
	{level}
	class={klass}
	legend={parts.map((p, i) => ({ label: `${p.label} · ${pct(p.value)}%`, color: seriesColor(i), kind: 'rect' }))}
	{table}
	readout={active === null ? '' : `${parts[active]!.label}: ${format(parts[active]!.value)}, ${pct(parts[active]!.value)}%`}
>
	<!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions -->
	<div
		class="tern-plot donut"
		role="group"
		aria-roledescription="chart"
		aria-label="{title}. Use the arrow keys to read values."
		tabindex="0"
		style:width="{size}px"
		onpointerleave={() => (active = null)}
		onfocus={() => active === null && (active = 0)}
		onblur={() => (active = null)}
		{onkeydown}
	>
		<svg width={size} height={size} viewBox="0 0 {size} {size}" aria-hidden="true">
			{#each arcs as a (a.i)}
				<path d={a.d} fill={seriesColor(a.i)} fill-rule="evenodd" opacity={active === null || active === a.i ? 1 : 0.5} onpointerenter={() => (active = a.i)} role="presentation" />
			{/each}
			<text x={r} y={r - 4} text-anchor="middle" class="total">{active === null ? format(total) : format(parts[active]!.value)}</text>
			<text x={r} y={r + 14} text-anchor="middle" class="tick">{active === null ? totalLabel : parts[active]!.label}</text>
		</svg>
	</div>
</ChartFrame>

<style>
	.donut {
		justify-self: start;
		max-width: 100%;
	}
	.total {
		fill: var(--tern-fg);
		font-size: 20px;
		font-weight: 650;
		font-variant-numeric: tabular-nums;
	}
</style>

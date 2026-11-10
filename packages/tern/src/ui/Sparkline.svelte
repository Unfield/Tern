<script lang="ts">
	type Props = {
		values: number[];
		/** Describes the trend for screen readers, such as "Visits rose 12% this week". */
		label: string;
		width?: number;
		height?: number;
		class?: string;
	};
	let { values, label, width = 96, height = 28, class: klass = '' }: Props = $props();

	const min = $derived(Math.min(...values));
	const max = $derived(Math.max(...values));
	const pts = $derived(values.map((v, i) => [2 + (i / Math.max(1, values.length - 1)) * (width - 6), 2 + (1 - (v - min) / (max - min || 1)) * (height - 4)] as const));
	const d = $derived(pts.map(([x, y], i) => `${i ? 'L' : 'M'}${x.toFixed(1)},${y.toFixed(1)}`).join(''));
	const last = $derived(pts[pts.length - 1]);
</script>

<!-- A trend at a glance: the history in a muted line, the latest point in the accent. -->
<svg class="tern-sparkline {klass}" {width} {height} viewBox="0 0 {width} {height}" role="img" aria-label={label}>
	<path {d} fill="none" stroke="var(--tern-border-strong)" stroke-width="1.5" stroke-linejoin="round" stroke-linecap="round" />
	{#if last}<circle cx={last[0]} cy={last[1]} r="2.5" fill="var(--tern-accent)" />{/if}
</svg>

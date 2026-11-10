<script lang="ts">
	import Sparkline from './Sparkline.svelte';
	import { formatNumber } from './chart.js';

	type Props = {
		/** What it counts, in sentence case, such as "Unique visitors". */
		label: string;
		value: number;
		/** The change, in percent, against the period named in period. */
		delta?: number;
		/** Such as "vs last week". */
		period?: string;
		/** Whether up is good (visits) or bad (errors), which picks the delta's color. */
		upIsGood?: boolean;
		/** Recent values, drawn as a sparkline. */
		trend?: number[];
		format?: (v: number) => string;
		class?: string;
	};
	let { label, value, delta, period = 'vs the previous period', upIsGood = true, trend, format = (v) => formatNumber(v), class: klass = '' }: Props = $props();

	const good = $derived(delta === undefined || delta === 0 ? null : delta > 0 === upIsGood);
	const deltaText = $derived(delta === undefined ? '' : `${delta > 0 ? '+' : delta < 0 ? '−' : ''}${Math.abs(Math.round(delta * 10) / 10)}%`);
</script>

<div class="tern-stat {klass}">
	<div class="label">{label}</div>
	<div class="value">{format(value)}</div>
	<div class="foot">
		{#if delta !== undefined}
			<span class="delta" class:good={good === true} class:bad={good === false}>
				<span aria-hidden="true">{delta > 0 ? '↑' : delta < 0 ? '↓' : '→'}</span>
				<span class="tern-sr-only">{delta > 0 ? 'Up' : delta < 0 ? 'Down' : 'No change'}</span>
				{deltaText}
			</span>
			<span class="period">{period}</span>
		{/if}
		{#if trend && trend.length > 1}
			<Sparkline values={trend} label="{label}, trend over the last {trend.length} periods" class="spark" />
		{/if}
	</div>
</div>

<style>
	.tern-stat {
		display: grid;
		gap: 0.25rem;
		padding: 1rem 1.125rem;
		background: var(--tern-surface);
		border: 1px solid var(--tern-border);
		border-radius: var(--tern-radius);
	}
	.label {
		font-size: 0.8125rem;
		font-weight: 550;
		color: var(--tern-muted);
	}
	.value {
		font-size: 1.75rem;
		font-weight: 650;
		letter-spacing: -0.02em;
		font-variant-numeric: tabular-nums;
		color: var(--tern-fg);
	}
	.foot {
		display: flex;
		align-items: center;
		gap: 0.4rem;
		min-height: 1.75rem;
		font-size: 0.8125rem;
	}
	.delta {
		display: inline-flex;
		gap: 0.15rem;
		font-weight: 600;
		color: var(--tern-muted);
		font-variant-numeric: tabular-nums;
	}
	.delta.good {
		color: var(--tern-success);
	}
	.delta.bad {
		color: var(--tern-danger);
	}
	.period {
		color: var(--tern-muted);
	}
	.foot :global(.spark) {
		margin-inline-start: auto;
	}
</style>

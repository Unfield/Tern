<script lang="ts">
	import { tb } from '../runtime/i18n.svelte.js';
	import type { Snippet } from 'svelte';
	import Collapsible from './Collapsible.svelte';
	import ScrollArea from './ScrollArea.svelte';

	type Props = {
		title: string;
		description?: string;
		/** The heading level, to fit the page's outline. */
		level?: 2 | 3 | 4;
		legend: { label: string; color: string; kind: 'line' | 'rect' }[];
		/** The table view: every value, reachable without hovering. */
		table: { headers: string[]; rows: string[][] };
		/** What the live readout says for the focused point, for keyboard and screen reader users. */
		readout: string;
		children: Snippet;
		class?: string;
	};
	let { title, description, level = 3, legend, table, readout, children, class: klass = '' }: Props = $props();
	const uid = $props.id();
</script>

<figure class="tern-chart {klass}" aria-labelledby="tern-chart-{uid}-title">
	<figcaption>
		<svelte:element this={`h${level}`} class="title" id="tern-chart-{uid}-title">{title}</svelte:element>
		{#if description}<p class="description">{description}</p>{/if}
	</figcaption>
	{#if legend.length >= 2}
		<ul class="legend" aria-label={tb('tern.ui.chart.legend')}>
			{#each legend as item (item.label)}
				<li><span class="key {item.kind}" style:background={item.color} aria-hidden="true"></span>{item.label}</li>
			{/each}
		</ul>
	{/if}
	{@render children()}
	<p class="tern-sr-only" aria-live="polite">{readout}</p>
	<Collapsible label="Show data">
		<ScrollArea label="{title} data" maxHeight="16rem" class="table">
			<table class="tern-table">
				<caption class="tern-sr-only">{title}</caption>
				<thead><tr>{#each table.headers as h, i (i)}<th scope="col" style:text-align={i ? 'end' : undefined}>{h}</th>{/each}</tr></thead>
				<tbody>
					{#each table.rows as row, r (r)}
						<tr>{#each row as cell, i (i)}{#if i === 0}<th scope="row">{cell}</th>{:else}<td style:text-align="end">{cell}</td>{/if}{/each}</tr>
					{/each}
				</tbody>
			</table>
		</ScrollArea>
	</Collapsible>
</figure>

<style>
	.tern-chart {
		display: grid;
		grid-template-columns: minmax(0, 1fr);
		gap: 0.5rem;
		min-width: 0;
		margin: 0;
		color: var(--tern-fg);
	}
	.title {
		margin: 0;
		font-size: 0.9375rem;
		font-weight: 600;
	}
	.description {
		margin: 0.15rem 0 0;
		font-size: 0.8125rem;
		color: var(--tern-muted);
	}
	.legend {
		display: flex;
		flex-wrap: wrap;
		gap: 0.25rem 1rem;
		margin: 0;
		padding: 0;
		list-style: none;
		font-size: 0.8125rem;
		color: var(--tern-muted);
	}
	.legend li {
		display: inline-flex;
		align-items: center;
		gap: 0.4rem;
	}
	.key.line {
		width: 0.875rem;
		height: 2px;
		border-radius: 1px;
	}
	.key.rect {
		width: 0.625rem;
		height: 0.625rem;
		border-radius: 2px;
	}
	.tern-chart :global(.table th[scope='row']) {
		font-weight: 500;
		text-transform: none;
		letter-spacing: 0;
		font-size: 0.875rem;
		color: var(--tern-fg);
	}
	.tern-chart :global(.table td) {
		font-variant-numeric: tabular-nums;
	}
	/* Shared by the charts' SVG. */
	.tern-chart :global(.tern-plot) {
		position: relative;
		outline: none;
		border-radius: var(--tern-radius-sm);
	}
	.tern-chart :global(.tern-plot:focus-visible) {
		box-shadow: 0 0 0 2px var(--tern-ring);
	}
	.tern-chart :global(.tern-plot svg) {
		display: block;
		width: 100%;
		overflow: visible;
	}
	.tern-chart :global(.grid) {
		stroke: var(--tern-border);
		stroke-width: 1;
		shape-rendering: crispEdges;
	}
	.tern-chart :global(.tick) {
		fill: var(--tern-muted);
		font-size: 12px;
		font-variant-numeric: tabular-nums;
	}
	.tern-chart :global(.direct) {
		fill: var(--tern-fg);
		font-size: 12px;
		font-weight: 500;
	}
	.tern-chart :global(.tooltip) {
		position: absolute;
		top: 0;
		z-index: 2;
		display: grid;
		gap: 0.2rem;
		min-width: 8rem;
		padding: 0.5rem 0.65rem;
		font-size: 0.8125rem;
		color: var(--tern-fg);
		background: var(--tern-surface);
		border: 1px solid var(--tern-border);
		border-radius: var(--tern-radius-sm);
		box-shadow: var(--tern-shadow-lg);
		pointer-events: none;
		white-space: nowrap;
	}
	.tern-chart :global(.tooltip .head) {
		color: var(--tern-muted);
		font-size: 0.75rem;
	}
	.tern-chart :global(.tooltip .row) {
		display: flex;
		align-items: center;
		gap: 0.45rem;
	}
	.tern-chart :global(.tooltip .row strong) {
		font-variant-numeric: tabular-nums;
		font-weight: 650;
	}
	.tern-chart :global(.tooltip .row span) {
		color: var(--tern-muted);
	}
	.tern-chart :global(.tooltip .swatch) {
		width: 0.75rem;
		height: 2px;
		border-radius: 1px;
		flex: none;
	}
</style>

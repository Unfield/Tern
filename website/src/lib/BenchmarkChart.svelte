<script lang="ts">
	// Requests per second for each server, one small multiple per scenario,
	// from docs/benchmarks.md. Tern's native renderer is the series the
	// story is about, in the brand teal; the rest are context, in gray, and
	// every bar is labeled, so nothing rests on color.
	//
	// Colors (--chart-mark, --chart-other) were run through the dataviz
	// palette validator in both themes: the teal passes every check, the
	// pair separates under all color-vision simulations, and the grays sit
	// under 3:1 against the surface by design, so the value labels and the
	// table view carry them.
	type Scenario = { name: string; servers: string[]; values: number[] };
	let { scenarios }: { scenarios: Scenario[] } = $props();

	let table = $state(false);
	let tip = $state<{ x: number; y: number; text: string } | null>(null);
	let wrap = $state<HTMLElement>();

	const fmt = new Intl.NumberFormat('en-US');

	function best(sc: Scenario) {
		return Math.max(...sc.values);
	}
	function comparison(sc: Scenario, i: number) {
		const v = sc.values[i];
		const native = sc.values[0];
		if (i === 0) {
			const sk = sc.values[sc.values.length - 1];
			return `${fmt.format(v)} requests a second, ${(v / sk).toFixed(1)}× ${sc.servers[sc.servers.length - 1]}`;
		}
		return `${fmt.format(v)} requests a second; Tern, native does ${(native / v).toFixed(1)}× that`;
	}
	function show(e: MouseEvent, sc: Scenario, i: number) {
		const bar = e.currentTarget as HTMLElement;
		if (!wrap) return;
		const a = bar.getBoundingClientRect();
		const b = wrap.getBoundingClientRect();
		tip = { x: Math.min(a.right - b.left + 8, b.width - 240), y: a.top - b.top + a.height / 2, text: `${sc.servers[i]}: ${comparison(sc, i)}` };
	}
</script>

<div class="chart" bind:this={wrap}>
	<div class="head">
		<p class="caption">Requests per second, more is better. Each panel has its own scale.</p>
		<button type="button" class="toggle" onclick={() => (table = !table)} aria-pressed={table}>{table ? 'Show the chart' : 'Show as a table'}</button>
	</div>

	{#if table}
		<div class="table-wrap">
			<table>
				<thead>
					<tr>
						<th scope="col">Scenario</th>
						{#each scenarios[0]?.servers ?? [] as s (s)}<th scope="col">{s}</th>{/each}
					</tr>
				</thead>
				<tbody>
					{#each scenarios as sc (sc.name)}
						<tr>
							<th scope="row">{sc.name}</th>
							{#each sc.values as v, i (i)}<td>{fmt.format(v)}</td>{/each}
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{:else}
		<div class="panels">
			{#each scenarios as sc (sc.name)}
				<figure class="panel">
					<figcaption>{sc.name}</figcaption>
					<div class="rows">
						{#each sc.servers as server, i (server)}
							<div class="row" class:lead={i === 0}>
								<span class="label">{server}</span>
								<span class="track">
									<span
										class="bar"
										style:width="{(sc.values[i] / best(sc)) * 100}%"
										role="img"
										aria-label="{server}: {comparison(sc, i)}"
										onmouseenter={(e) => show(e, sc, i)}
										onmouseleave={() => (tip = null)}
									></span>
									<span class="value">{fmt.format(sc.values[i])}</span>
								</span>
							</div>
						{/each}
					</div>
				</figure>
			{/each}
		</div>
		{#if tip}
			<div class="tip" style:left="{tip.x}px" style:top="{tip.y}px" role="tooltip">{tip.text}</div>
		{/if}
	{/if}
</div>

<style>
	.chart {
		position: relative;
	}
	.head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: var(--s-4);
		margin-bottom: var(--s-6);
	}
	.caption {
		color: var(--text-3);
		font-size: 14px;
	}
	.toggle {
		flex: none;
		height: 32px;
		padding: 0 var(--s-3);
		border: 1px solid var(--border);
		border-radius: var(--radius-s);
		background: var(--surface);
		color: var(--text-2);
		font-size: 13px;
		cursor: pointer;
	}
	.toggle:hover {
		color: var(--text);
		border-color: var(--border-strong);
	}
	.panels {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(340px, 1fr));
		gap: var(--s-6);
	}
	.panel {
		margin: 0;
		padding: var(--s-6);
		border: 1px solid var(--border);
		border-radius: var(--radius-l);
		background: var(--surface);
	}
	figcaption {
		/* two lines, so the rows line up across panels */
		min-height: 2.7em;
		margin-bottom: var(--s-4);
		color: var(--text);
		font-size: 15px;
		font-weight: 600;
		line-height: 1.35;
	}
	.rows {
		display: flex;
		flex-direction: column;
		gap: var(--s-3);
	}
	.row {
		display: grid;
		grid-template-columns: 156px 1fr;
		align-items: center;
		gap: var(--s-3);
	}
	.label {
		color: var(--text-3);
		font-size: 13px;
		line-height: 1.3;
	}
	.lead .label {
		color: var(--text);
		font-weight: 600;
	}
	.track {
		display: flex;
		align-items: center;
		gap: var(--s-2);
		min-width: 0;
		/* room for the longest value label at the tip */
		padding-right: 56px;
		border-left: 1px solid var(--border-strong);
	}
	.bar {
		display: block;
		flex: none;
		height: 20px;
		min-width: 3px;
		border-radius: 0 4px 4px 0;
		background: var(--chart-other);
		transition: filter 120ms;
	}
	.lead .bar {
		background: var(--chart-mark);
	}
	.bar:hover {
		filter: brightness(1.12);
	}
	.value {
		margin-right: -56px;
		color: var(--text-2);
		font-size: 12.5px;
		font-variant-numeric: tabular-nums;
		white-space: nowrap;
	}
	.lead .value {
		color: var(--text);
		font-weight: 600;
	}
	.tip {
		position: absolute;
		z-index: 5;
		max-width: 240px;
		padding: var(--s-2) var(--s-3);
		border: 1px solid var(--border-strong);
		border-radius: var(--radius-s);
		background: var(--surface);
		box-shadow: var(--shadow);
		color: var(--text);
		font-size: 13px;
		line-height: 1.4;
		pointer-events: none;
		transform: translateY(-50%);
	}
	.table-wrap {
		overflow-x: auto;
		border: 1px solid var(--border);
		border-radius: var(--radius-l);
		background: var(--surface);
	}
	table {
		width: 100%;
		border-collapse: collapse;
		font-size: 14px;
	}
	th,
	td {
		padding: var(--s-3) var(--s-4);
		border-bottom: 1px solid var(--border);
		text-align: right;
		white-space: nowrap;
	}
	th[scope='row'],
	thead th:first-child {
		text-align: left;
	}
	thead th {
		color: var(--text-3);
		font-size: 13px;
		font-weight: 550;
	}
	tbody tr:last-child > * {
		border-bottom: 0;
	}
	td {
		font-variant-numeric: tabular-nums;
	}
	@media (max-width: 640px) {
		.head {
			flex-direction: column;
			align-items: flex-start;
		}
		.panel {
			padding: var(--s-5);
		}
		.row {
			grid-template-columns: 132px 1fr;
		}
		figcaption {
			min-height: 0;
		}
	}
</style>

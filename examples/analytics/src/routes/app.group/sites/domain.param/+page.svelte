<script lang="ts">
	import { goto } from '@ternkit/tern';
	import { AlertDialog, AreaChart, BarChart, Button, DataTable, DonutChart, Empty, Stat, ToggleGroup } from '@ternkit/tern/ui';
	import LiveVisitors from '$lib/LiveVisitors.svelte';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	let deleting = $state(false);
	// Go's nil slices arrive as null.
	const series = $derived(data.series ?? []);
	const pages = $derived(data.pages ?? []);
	const sources = $derived(data.sources ?? []);
	const browsers = $derived(data.browsers ?? []);
	const devices = $derived(data.devices ?? []);

	const change = (now: number, before: number) => (before ? ((now - before) / before) * 100 : undefined);
	const period = $derived(String(data.days));
	const snippet = $derived(`<script defer data-domain="${data.site.domain}" src="${typeof location === 'undefined' ? '' : location.origin}/script.js"><\/script>`);
	const empty = $derived(data.summary.pageviews === 0);

	async function copy() {
		await navigator.clipboard?.writeText(snippet);
	}
</script>

<svelte:head><title>{data.site.name} · Analytics</title></svelte:head>

<header class="head">
	<div>
		<h1>{data.site.name}</h1>
		<p class="domain">{data.site.domain}</p>
	</div>
	<div class="tools">
		<LiveVisitors domain={data.site.domain} />
		<ToggleGroup
			label="Period"
			value={period}
			options={[
				{ value: '7', label: '7 days' },
				{ value: '30', label: '30 days' },
				{ value: '90', label: '90 days' }
			]}
			onchange={(v) => v && goto(`?days=${v}`, { replaceState: true, noScroll: true, keepFocus: true })}
		/>
	</div>
</header>

{#if empty}
	<Empty title="No page views yet" description="Add this script to every page of {data.site.domain}. Views show up here within seconds." level={2}>
		{#snippet actions()}<Button variant="secondary" onclick={copy}>Copy the script tag</Button>{/snippet}
	</Empty>
	<pre class="snippet"><code>{snippet}</code></pre>
{:else}
	<section class="stats" aria-label="Summary">
		<Stat label="Unique visitors" value={data.summary.visitors} delta={change(data.summary.visitors, data.previous.visitors)} period="vs the {data.days} days before" trend={series.map((d) => d.visitors)} />
		<Stat label="Page views" value={data.summary.pageviews} delta={change(data.summary.pageviews, data.previous.pageviews)} period="vs the {data.days} days before" />
		<Stat label="Bounce rate" value={Math.round(data.summary.bounceRate)} format={(v) => `${v}%`} delta={change(data.summary.bounceRate, data.previous.bounceRate)} upIsGood={false} period="vs the {data.days} days before" />
		<Stat label="Views per visit" value={data.summary.visitors ? Math.round((data.summary.pageviews / data.summary.visitors) * 10) / 10 : 0} format={(v) => String(v)} />
	</section>

	<section class="panel">
		<AreaChart title="Traffic" description="Unique visitors and page views per day" level={2} data={series} x="day" series={[{ key: 'pageviews', label: 'Page views' }, { key: 'visitors', label: 'Visitors' }]} height={260} />
	</section>

	<div class="grid">
		<section class="panel">
			<DataTable
				caption="Top pages"
				rows={pages}
				key={(p) => p.value}
				pageSize={8}
				searchable
				columns={[
					{ key: 'value', header: 'Page', sortable: true },
					{ key: 'visitors', header: 'Visitors', sortable: true, align: 'end' },
					{ key: 'pageviews', header: 'Views', sortable: true, align: 'end' }
				]}
				empty="No pages in this period"
			/>
		</section>
		<section class="panel">
			{#if sources.length}
				<BarChart title="Sources" description="Where visitors came from" level={2} data={sources} x="value" series={[{ key: 'visitors', label: 'Visitors' }]} horizontal />
			{:else}
				<Empty title="No sources yet" description="Visitors came straight to the site, or their browser didn't say from where." level={2} />
			{/if}
		</section>
		<section class="panel">
			<DonutChart title="Browsers" level={2} data={browsers} label="value" value="visitors" totalLabel="visitors" />
		</section>
		<section class="panel">
			<DonutChart title="Devices" level={2} data={devices} label="value" value="visitors" totalLabel="visitors" />
		</section>
	</div>

	<details class="setup">
		<summary>Script tag</summary>
		<pre class="snippet"><code>{snippet}</code></pre>
	</details>
{/if}

<form method="POST" action="?/delete" class="danger" id="delete-site"></form>
<Button variant="ghost" size="sm" onclick={() => (deleting = true)}>Delete this site</Button>
<AlertDialog
	bind:open={deleting}
	title="Delete {data.site.name}?"
	description="Its page views and totals are deleted too. This can't be undone."
	confirmLabel="Delete site"
	danger
	onconfirm={() => (document.getElementById('delete-site') as HTMLFormElement).requestSubmit()}
/>

<style>
	.head {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: space-between;
		gap: 1rem;
		margin-bottom: 1.5rem;
	}
	h1 {
		margin: 0;
		font-size: 1.5rem;
		letter-spacing: -0.02em;
	}
	.domain {
		margin: 0.15rem 0 0;
		color: var(--tern-muted);
		font-size: 0.875rem;
	}
	.tools {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.75rem;
	}
	.stats {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(13rem, 1fr));
		gap: 1rem;
		margin-bottom: 1rem;
	}
	.panel {
		min-width: 0;
		padding: 1.25rem;
		background: var(--tern-surface);
		border: 1px solid var(--tern-border);
		border-radius: var(--tern-radius);
	}
	.grid {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(22rem, 1fr));
		gap: 1rem;
		margin: 1rem 0;
	}
	@media (max-width: 30rem) {
		.grid {
			grid-template-columns: minmax(0, 1fr);
		}
	}
	.snippet {
		overflow-x: auto;
		padding: 0.75rem 1rem;
		font-size: 0.8125rem;
		background: var(--tern-surface-2);
		border: 1px solid var(--tern-border);
		border-radius: var(--tern-radius-sm);
	}
	.setup {
		margin: 1.5rem 0;
		font-size: 0.875rem;
	}
	.setup summary {
		cursor: pointer;
		font-weight: 550;
	}
	.danger {
		display: none;
	}
</style>

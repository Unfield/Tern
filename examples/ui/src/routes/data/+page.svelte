<script lang="ts">
	import { AreaChart, Badge, BarChart, Button, DataTable, DonutChart, LineChart, Stat, toast } from '@ternkit/tern/ui';

	const days = Array.from({ length: 30 }, (_, i) => {
		const d = new Date(Date.UTC(2026, 9, 12 + i));
		const wave = Math.sin(i / 3) * 120;
		return { day: d.toISOString().slice(0, 10), visitors: Math.round(900 + i * 18 + wave), pageviews: Math.round(2100 + i * 41 + wave * 2.1) };
	});
	const sources = [
		{ source: 'Search', visits: 4210, signups: 120 },
		{ source: 'Direct', visits: 3100, signups: 210 },
		{ source: 'Social', visits: 1680, signups: 44 },
		{ source: 'Newsletter', visits: 980, signups: 96 },
		{ source: 'Referral', visits: 640, signups: 31 }
	];
	const browsers = [
		{ name: 'Chrome', share: 61 },
		{ name: 'Safari', share: 21 },
		{ name: 'Firefox', share: 8 },
		{ name: 'Edge', share: 6 },
		{ name: 'Samsung Internet', share: 2 },
		{ name: 'Opera', share: 1 },
		{ name: 'Other browsers', share: 1 }
	];

	type Page = { path: string; views: number; visitors: number; bounce: number };
	const pages: Page[] = ['/', '/pricing', '/docs', '/docs/guide', '/blog', '/blog/tern-1-4', '/about', '/careers', '/changelog', '/docs/live', '/docs/ui', '/login', '/signup'].map((path, i) => ({
		path,
		views: 9800 - i * 610 + (i % 3) * 140,
		visitors: 6100 - i * 390 + (i % 4) * 90,
		bounce: 32 + ((i * 7) % 30)
	}));
	let selected = $state<string[]>([]);
</script>

<svelte:head><title>Data · Tern UI</title></svelte:head>

<h1>Data</h1>
<p class="lead">Charts read in light and dark mode, with colors checked for color blindness. Hover or use the arrow keys for values; every chart also has its data as a table.</p>

<div class="stats">
	<Stat label="Unique visitors" value={days.at(-1)!.visitors * 30} delta={12.4} period="vs last month" trend={days.map((d) => d.visitors)} />
	<Stat label="Bounce rate" value={41} format={(v) => `${v}%`} delta={-3.1} upIsGood={false} period="vs last month" />
	<Stat label="Errors" value={14} delta={40} upIsGood={false} period="vs last week" />
</div>

<div class="charts">
	<LineChart level={2} title="Traffic" description="The last 30 days" data={days} x="day" series={[{ key: 'pageviews', label: 'Pageviews' }, { key: 'visitors', label: 'Visitors' }]} />
	<AreaChart level={2} title="Visitors" data={days} x="day" series={[{ key: 'visitors', label: 'Visitors' }]} height={180} />
	<BarChart level={2} title="Sources" description="Visits and sign-ups by where people came from" data={sources} x="source" series={[{ key: 'visits', label: 'Visits' }, { key: 'signups', label: 'Sign-ups' }]} />
	<BarChart level={2} title="Visits by source" data={sources} x="source" series={[{ key: 'visits', label: 'Visits' }]} horizontal />
	<BarChart level={2} title="Stacked" data={sources} x="source" series={[{ key: 'visits', label: 'Visits' }, { key: 'signups', label: 'Sign-ups' }]} stacked />
	<DonutChart level={2} title="Browsers" data={browsers} label="name" value="share" totalLabel="percent" format={(v) => String(v)} />
</div>

<DataTable
	caption="Top pages"
	rows={pages}
	key={(p) => p.path}
	pageSize={5}
	searchable
	selectable
	bind:selected
	columns={[
		{ key: 'path', header: 'Page', sortable: true },
		{ key: 'views', header: 'Views', sortable: true, align: 'end' },
		{ key: 'visitors', header: 'Visitors', sortable: true, align: 'end' },
		{ key: 'bounce', header: 'Bounce', sortable: true, align: 'end', cell: bounce }
	]}
>
	{#snippet toolbar(keys)}
		<Button size="sm" variant="secondary" disabled={!keys.length} onclick={() => toast(`Exported ${keys.length} pages`)}>Export selected</Button>
	{/snippet}
</DataTable>
<p data-testid="selected">Selected: {selected.join(', ') || 'none'}</p>

{#snippet bounce(p: Page)}<Badge tone={p.bounce > 50 ? 'warning' : 'neutral'}>{p.bounce}%</Badge>{/snippet}

<style>
	.lead {
		color: var(--tern-muted);
	}
	.stats {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(14rem, 1fr));
		gap: 1rem;
		margin: 1.5rem 0;
	}
	.charts {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(22rem, 1fr));
		gap: 2rem 1.5rem;
		margin-bottom: 2.5rem;
	}
</style>

<script module lang="ts">
	// Props for the preview at /__tern/emails/WeeklyReport under tern dev.
	export const preview = {
		name: 'Ada',
		url: 'https://analytics.example.com',
		sites: [
			{ name: 'Acme', domain: 'acme.example', visitors: 1284, pageviews: 3920, change: 12.5 },
			{ name: 'Docs', domain: 'docs.acme.example', visitors: 802, pageviews: 2410, change: -4 }
		]
	};
</script>

<script lang="ts">
	type Row = { name: string; domain: string; visitors: number; pageviews: number; change: number };
	let { name, url, sites }: { name: string; url: string; sites: Row[] } = $props();
	const fmt = (n: number) => n.toLocaleString('en-US');
</script>

<svelte:head><title>Your week in visits</title></svelte:head>

<div style="font-family: system-ui, sans-serif; max-width: 32rem; margin: 0 auto; color: #1b2430">
	<h1 style="font-size: 20px">Hi {name}, here's your week</h1>
	<table style="width: 100%; border-collapse: collapse; font-size: 14px">
		<thead>
			<tr>
				<th style="text-align: left; padding: 8px 0; color: #5b6573">Site</th>
				<th style="text-align: right; padding: 8px 0; color: #5b6573">Visitors</th>
				<th style="text-align: right; padding: 8px 0; color: #5b6573">Views</th>
			</tr>
		</thead>
		<tbody>
			{#each sites as s (s.domain)}
				<tr>
					<td style="padding: 8px 0; border-top: 1px solid #e3e7ec"><a href="{url}/sites/{s.domain}" style="color: #0b6f86">{s.name}</a></td>
					<td style="padding: 8px 0; border-top: 1px solid #e3e7ec; text-align: right">
						{fmt(s.visitors)}
						<span style="color: {s.change >= 0 ? '#1d7a46' : '#b4232c'}">({s.change >= 0 ? '+' : ''}{Math.round(s.change)}%)</span>
					</td>
					<td style="padding: 8px 0; border-top: 1px solid #e3e7ec; text-align: right">{fmt(s.pageviews)}</td>
				</tr>
			{/each}
		</tbody>
	</table>
	<p style="color: #5b6573; font-size: 13px">Change is against the week before. Turn these emails off in your settings.</p>
</div>

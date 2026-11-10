<script lang="ts">
	import { enhance, invalidate, page, stream } from '@ternkit/tern';
	import { AreaChart } from 'layerchart';
	import Pause from '@lucide/svelte/icons/pause';
	import Pencil from '@lucide/svelte/icons/pencil';
	import Play from '@lucide/svelte/icons/play';
	import RefreshCw from '@lucide/svelte/icons/refresh-cw';
	import Trash from '@lucide/svelte/icons/trash-2';
	import Beats from '$lib/Beats.svelte';
	import StatusDot from '$lib/StatusDot.svelte';
	import { Badge } from '$lib/components/ui/badge/index.js';
	import { Button } from '$lib/components/ui/button/index.js';
	import * as Card from '$lib/components/ui/card/index.js';
	import * as Chart from '$lib/components/ui/chart/index.js';
	import * as Dialog from '$lib/components/ui/dialog/index.js';
	import * as Table from '$lib/components/ui/table/index.js';
	import { ago, date, datetime, ms, percent, statusLabel } from '$lib/format.js';
	import type { Event } from '$lib/live.js';
	import { clock } from '$lib/now.svelte.js';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	// svelte-ignore state_referenced_locally
	const time = clock(data.now);
	let checking = $state(false);
	let confirmDelete = $state(false);

	let liveBeats = $state<Event['beat'][]>([]);
	let liveMonitor = $state<Event['monitor'] | null>(null);
	stream<Event>('/api/events', {
		events: ['monitors'],
		onMessage: (e) => {
			if (e.monitor.id !== data.monitor.id) return;
			const before = liveMonitor?.status ?? data.monitor.status;
			liveMonitor = e.monitor;
			// A change of status adds an event and moves the uptime: reload them.
			if (before !== e.monitor.status && e.monitor.status !== 'pending') void invalidate(`app:monitor:${data.monitor.id}`);
			if (e.beat.status !== 'paused') liveBeats = [...liveBeats, e.beat].slice(-40);
		}
	});

	const m = $derived({
		...data.monitor,
		status: liveMonitor?.status ?? data.monitor.status,
		lastCheck: liveMonitor?.lastCheck ?? data.monitor.lastCheck,
		lastLatency: liveMonitor?.lastLatency ?? data.monitor.lastLatency
	});
	const beats = $derived([...(data.beats ?? []), ...liveBeats.filter((b) => b.at > (data.beats?.at(-1)?.at ?? 0))].slice(-40));
	const points = $derived((data.points ?? []).map((p) => ({ date: new Date(p.at), latency: p.latency })));
	const chartConfig = { latency: { label: 'Response time (ms)', color: 'var(--chart-2)' } } satisfies Chart.ChartConfig;
	const certDays = $derived(m.certExpires ? Math.floor((m.certExpires - time.now) / 86400000) : null);
	const periods = ['24h', '7d', '30d'];
</script>

<svelte:head><title>{m.name} · Status</title></svelte:head>

<div class="flex flex-wrap items-start justify-between gap-4">
	<div class="grid gap-1">
		<p class="text-muted-foreground text-sm"><a href="/admin" class="hover:underline">Monitors</a> / {m.group}</p>
		<h1 class="flex items-center gap-3 text-2xl font-semibold tracking-tight">
			<StatusDot status={m.status} pulse />{m.name}
			<Badge variant={m.status === 'down' ? 'destructive' : 'secondary'} data-testid="detail-status">{statusLabel[m.status]}</Badge>
		</h1>
		<p class="text-muted-foreground text-sm"><span class="uppercase">{m.type}</span> · {m.target} · every {m.intervalSeconds}s · checked {ago(m.lastCheck, time.now)}</p>
	</div>
	<div class="flex flex-wrap gap-2">
		<form
			method="POST"
			action="?/check"
			use:enhance={() => {
				checking = true;
				return async ({ update }) => {
					await update();
					checking = false;
				};
			}}
		>
			<Button variant="outline" type="submit" disabled={checking || m.paused}><RefreshCw class={checking ? 'animate-spin' : ''} /> Check now</Button>
		</form>
		<form method="POST" action={m.paused ? '?/resume' : '?/pause'} use:enhance>
			<Button variant="outline" type="submit">{#if m.paused}<Play /> Resume{:else}<Pause /> Pause{/if}</Button>
		</form>
		<Button variant="outline" href="/admin/monitors/{m.id}/edit"><Pencil /> Edit</Button>
		<Button variant="destructive" onclick={() => (confirmDelete = true)}><Trash /> Delete</Button>
	</div>
</div>

<div class="grid grid-cols-2 gap-4 md:grid-cols-5">
	<Card.Root>
		<Card.Header class="pb-2"><Card.Description>Response</Card.Description></Card.Header>
		<Card.Content><span class="text-2xl font-semibold">{ms(m.lastLatency)}</span></Card.Content>
	</Card.Root>
	<Card.Root>
		<Card.Header class="pb-2"><Card.Description>Average (24h)</Card.Description></Card.Header>
		<Card.Content><span class="text-2xl font-semibold">{ms(data.uptime?.['24h']?.avgLatency || null)}</span></Card.Content>
	</Card.Root>
	{#each ['24h', '30d'] as p (p)}
		<Card.Root>
			<Card.Header class="pb-2"><Card.Description>Uptime ({p})</Card.Description></Card.Header>
			<Card.Content><span class="text-2xl font-semibold" data-testid="uptime-{p}">{percent(data.uptime?.[p]?.ratio)}</span></Card.Content>
		</Card.Root>
	{/each}
	<Card.Root>
		<Card.Header class="pb-2"><Card.Description>Certificate</Card.Description></Card.Header>
		<Card.Content>
			<span class="text-2xl font-semibold {certDays !== null && certDays < m.certExpiryDays ? 'text-red-600' : ''}">{certDays === null ? '–' : `${certDays} days`}</span>
		</Card.Content>
	</Card.Root>
</div>

<Card.Root>
	<Card.Header class="flex flex-row items-center justify-between">
		<div>
			<Card.Title>Response time</Card.Title>
			<Card.Description>Average of successful checks</Card.Description>
		</div>
		<div class="flex gap-1">
			{#each periods as p (p)}
				<Button size="sm" variant={data.period === p ? 'secondary' : 'ghost'} href="?period={p}" data-tern-noscroll>{p}</Button>
			{/each}
		</div>
	</Card.Header>
	<Card.Content>
		{#if points.length > 1}
			<Chart.Container config={chartConfig} class="aspect-auto h-56 w-full" data-testid="latency-chart">
				<AreaChart data={points} x="date" yDomain={[0, null]} series={[{ key: 'latency', label: chartConfig.latency.label, color: chartConfig.latency.color }]}>
					{#snippet tooltip()}<Chart.Tooltip />{/snippet}
				</AreaChart>
			</Chart.Container>
		{:else}
			<p class="text-muted-foreground py-16 text-center text-sm">Not enough checks yet.</p>
		{/if}
		<div class="mt-4"><Beats {beats} /></div>
	</Card.Content>
</Card.Root>

<div class="grid gap-6 md:grid-cols-[2fr_1fr]">
	<Card.Root>
		<Card.Header><Card.Title>Events</Card.Title><Card.Description>Status changes in the last {data.period}</Card.Description></Card.Header>
		<Card.Content class="p-0">
			<Table.Root>
				<Table.Body data-testid="events">
					{#each data.changes ?? [] as c (c.at)}
						<Table.Row>
							<Table.Cell class="w-28"><span class="flex items-center gap-2"><StatusDot status={c.status} />{statusLabel[c.status]}</span></Table.Cell>
							<Table.Cell class="text-muted-foreground">{c.message}</Table.Cell>
							<Table.Cell class="text-muted-foreground text-right whitespace-nowrap">{datetime(c.at)}</Table.Cell>
						</Table.Row>
					{:else}
						<Table.Row><Table.Cell class="text-muted-foreground h-16 text-center">No changes.</Table.Cell></Table.Row>
					{/each}
				</Table.Body>
			</Table.Root>
		</Card.Content>
	</Card.Root>
	<Card.Root>
		<Card.Header><Card.Title>Badge</Card.Title><Card.Description>For a README or website</Card.Description></Card.Header>
		<Card.Content class="grid gap-3">
			{#if m.public}
				<img src={data.badgeUrl} alt="{m.name} status" class="h-5 self-start" />
				<img src="{data.badgeUrl}?style=uptime" alt="{m.name} uptime" class="h-5 self-start" />
				<code class="bg-muted rounded p-2 text-xs break-all">![{m.name}]({page.url.origin}{data.badgeUrl})</code>
			{:else}
				<p class="text-muted-foreground text-sm">Badges are available for monitors shown on the status page.</p>
			{/if}
			<p class="text-muted-foreground text-xs">Added {date(m.createdAt ? new Date(m.createdAt).getTime() : 0)}</p>
		</Card.Content>
	</Card.Root>
</div>

<Dialog.Root bind:open={confirmDelete}>
	<Dialog.Content>
		<Dialog.Header>
			<Dialog.Title>Delete {m.name}?</Dialog.Title>
			<Dialog.Description>Its checks and history are deleted too. This can't be undone.</Dialog.Description>
		</Dialog.Header>
		<Dialog.Footer>
			<Button variant="outline" onclick={() => (confirmDelete = false)}>Cancel</Button>
			<form method="POST" action="?/delete"><Button variant="destructive" type="submit">Delete monitor</Button></form>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>

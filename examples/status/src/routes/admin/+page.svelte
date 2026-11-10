<script lang="ts">
	import { stream } from '@ternkit/tern';
	import Plus from '@lucide/svelte/icons/plus';
	import Beats from '$lib/Beats.svelte';
	import StatusDot from '$lib/StatusDot.svelte';
	import { Badge } from '$lib/components/ui/badge/index.js';
	import { Button } from '$lib/components/ui/button/index.js';
	import * as Card from '$lib/components/ui/card/index.js';
	import { ago, ms, percent, statusLabel } from '$lib/format.js';
	import { clock } from '$lib/now.svelte.js';
	import type { Event } from '$lib/live.js';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	// svelte-ignore state_referenced_locally
	const time = clock(data.now);

	// Checks arriving over server-sent events, layered over the loaded data.
	let live = $state<Record<number, { status: string; lastCheck: number | null; lastLatency: number | null; message: string; beats: Event['beat'][] }>>({});
	const events = stream<Event>('/api/events', {
		events: ['monitors'],
		onMessage: ({ monitor, beat }) => {
			const prev = live[monitor.id];
			const beats = prev?.beats ?? [];
			live[monitor.id] = {
				status: monitor.status,
				lastCheck: monitor.lastCheck,
				lastLatency: monitor.lastLatency,
				message: beat.message,
				beats: beat.status === 'paused' ? beats : [...beats, beat].slice(-40)
			};
		}
	});

	const rows = $derived(
		(data.monitors ?? []).map((m) => {
			const l = live[m.id];
			return l
				? { ...m, status: l.status, lastCheck: l.lastCheck, lastLatency: l.lastLatency, lastMessage: l.message, beats: [...(m.beats ?? []), ...l.beats].slice(-40) }
				: { ...m, beats: m.beats ?? [] };
		})
	);
	const count = (s: string) => rows.filter((m) => m.status === s).length;
	const summary = $derived([
		{ label: 'Up', value: count('up'), tone: 'text-emerald-600' },
		{ label: 'Down', value: count('down'), tone: 'text-red-600' },
		{ label: 'Pending', value: count('pending'), tone: 'text-amber-600' },
		{ label: 'Paused', value: count('paused'), tone: 'text-muted-foreground' }
	]);
</script>

<svelte:head><title>Monitors · Status</title></svelte:head>

<div class="flex flex-wrap items-center justify-between gap-3">
	<div>
		<h1 class="text-2xl font-semibold tracking-tight">Monitors</h1>
		<p class="text-muted-foreground text-sm">
			Live: <span data-testid="live-status">{events.status === 'open' ? 'connected' : events.status}</span>
		</p>
	</div>
	<Button href="/admin/monitors/new"><Plus /> New monitor</Button>
</div>

<div class="grid grid-cols-2 gap-4 md:grid-cols-4">
	{#each summary as s (s.label)}
		<Card.Root>
			<Card.Header class="pb-2"><Card.Description>{s.label}</Card.Description></Card.Header>
			<Card.Content><span class="text-3xl font-semibold {s.tone}" data-testid="count-{s.label.toLowerCase()}">{s.value}</span></Card.Content>
		</Card.Root>
	{/each}
</div>

{#if rows.length === 0}
	<Card.Root>
		<Card.Content class="grid justify-items-center gap-3 py-12 text-center">
			<p class="font-medium">No monitors yet</p>
			<p class="text-muted-foreground text-sm">Add a website, API, port or domain to start checking it.</p>
			<Button href="/admin/monitors/new"><Plus /> Add your first monitor</Button>
		</Card.Content>
	</Card.Root>
{:else}
	<Card.Root>
		<Card.Content class="divide-y p-0" data-testid="monitors">
			{#each rows as m (m.id)}
				<a href="/admin/monitors/{m.id}" class="hover:bg-muted/50 grid gap-3 p-4 md:grid-cols-[1fr_auto_auto] md:items-center" data-testid="monitor-row" data-name={m.name}>
					<div class="min-w-0">
						<div class="flex items-center gap-2 font-medium">
							<StatusDot status={m.status} pulse />
							<span class="truncate">{m.name}</span>
							<Badge variant="outline" class="uppercase">{m.type}</Badge>
						</div>
						<p class="text-muted-foreground mt-1 truncate text-xs">{m.target} · {m.lastMessage || 'not checked yet'}</p>
					</div>
					<Beats beats={m.beats} />
					<div class="flex items-center gap-4 text-sm md:w-56 md:justify-end">
						<span class="text-muted-foreground">{ms(m.lastLatency)}</span>
						<span>{percent(m.uptime)}</span>
						<Badge variant={m.status === 'down' ? 'destructive' : 'secondary'} data-testid="row-status">{statusLabel[m.status]}</Badge>
					</div>
					<span class="sr-only">checked {ago(m.lastCheck, time.now)}</span>
				</a>
			{/each}
		</Card.Content>
	</Card.Root>
{/if}

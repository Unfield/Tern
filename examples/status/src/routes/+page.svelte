<script lang="ts">
	import { invalidateAll, stream } from '@ternkit/tern';
	import CircleAlert from '@lucide/svelte/icons/circle-alert';
	import CircleCheck from '@lucide/svelte/icons/circle-check';
	import Wrench from '@lucide/svelte/icons/wrench';
	import DayBars from '$lib/DayBars.svelte';
	import Incident from '$lib/Incident.svelte';
	import Logo from '$lib/Logo.svelte';
	import StatusDot from '$lib/StatusDot.svelte';
	import { Badge } from '$lib/components/ui/badge/index.js';
	import * as Card from '$lib/components/ui/card/index.js';
	import { ago, percent, statusLabel } from '$lib/format.js';
	import { clock } from '$lib/now.svelte.js';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	// svelte-ignore state_referenced_locally
	const time = clock(data.now);

	// Live status from the server, layered over the loaded data.
	type Live = { status: string; lastCheck: number | null };
	let live = $state<Record<number, Live>>({});
	stream<{ monitor: { id: number; status: string; lastCheck: number | null } }>('/api/public/events', {
		events: ['public'],
		onMessage: ({ monitor }) => {
			const before = live[monitor.id]?.status ?? data.groups?.flatMap((g) => g.monitors ?? []).find((m) => m.id === monitor.id)?.status;
			live[monitor.id] = { status: monitor.status, lastCheck: monitor.lastCheck };
			// Going down or coming back opens or resolves an incident: reload them.
			if (before !== monitor.status && (before === 'down' || monitor.status === 'down')) void invalidateAll();
		}
	});

	const groups = $derived(
		(data.groups ?? []).map((g) => ({
			...g,
			monitors: (g.monitors ?? []).map((m) => ({ ...m, status: live[m.id]?.status ?? m.status, lastCheck: live[m.id]?.lastCheck ?? m.lastCheck }))
		}))
	);
	const all = $derived(groups.flatMap((g) => g.monitors));
	const down = $derived(all.filter((m) => m.status === 'down').length);
	const open = $derived((data.incidents ?? []).filter((i) => !i.resolvedAt));
	const maintenance = $derived(open.some((i) => i.impact === 'maintenance'));
	const overall = $derived(
		down === 0
			? maintenance
				? { label: 'Scheduled maintenance in progress', tone: 'maintenance' }
				: { label: 'All systems operational', tone: 'ok' }
			: down === all.length
				? { label: 'Major outage', tone: 'down' }
				: { label: down === 1 ? 'Partial outage: 1 service is down' : `Partial outage: ${down} services are down`, tone: 'down' }
	);
	const resolved = $derived((data.incidents ?? []).filter((i) => i.resolvedAt));
</script>

<svelte:head>
	<title>{data.title}</title>
	<meta name="description" content={data.description || `Live status and uptime of ${data.title}.`} />
</svelte:head>

<div class="mx-auto flex min-h-svh max-w-3xl flex-col gap-8 px-4 py-10">
	<header class="flex items-center justify-between">
		<Logo name={data.title} />
		<a href={data.admin ? '/admin' : '/login'} class="text-muted-foreground text-sm hover:underline">{data.admin ? 'Admin' : 'Sign in'}</a>
	</header>

	{#if data.description}<p class="text-muted-foreground -mt-4">{data.description}</p>{/if}

	<div
		class="flex items-center gap-3 rounded-xl p-5 text-lg font-semibold text-white {overall.tone === 'ok'
			? 'bg-emerald-600'
			: overall.tone === 'maintenance'
				? 'bg-sky-600'
				: 'bg-red-600'}"
		data-testid="overall"
		role="status"
	>
		{#if overall.tone === 'ok'}<CircleCheck />{:else if overall.tone === 'maintenance'}<Wrench />{:else}<CircleAlert />{/if}
		{overall.label}
	</div>

	{#each open as incident (incident.id)}
		<Incident {incident} />
	{/each}

	{#if all.length === 0}
		<Card.Root><Card.Content class="text-muted-foreground py-10 text-center">No services are listed yet.</Card.Content></Card.Root>
	{/if}

	{#each groups as group (group.name)}
		<section class="grid gap-3">
			<h2 class="text-muted-foreground text-sm font-medium tracking-wide uppercase">{group.name}</h2>
			<Card.Root>
				<Card.Content class="divide-y p-0">
					{#each group.monitors as m (m.id)}
						<div class="grid gap-3 p-5" data-testid="public-monitor" data-name={m.name}>
							<div class="flex items-center justify-between gap-3">
								<div class="flex items-center gap-2 font-medium">
									<StatusDot status={m.status} pulse />
									{m.name}
								</div>
								<div class="flex items-center gap-3 text-sm">
									<span class="text-muted-foreground hidden sm:inline">{percent(m.uptime)} uptime</span>
									<Badge variant={m.status === 'down' ? 'destructive' : 'secondary'} data-testid="public-status">{statusLabel[m.status]}</Badge>
								</div>
							</div>
							<DayBars days={m.days ?? []} />
							<p class="text-muted-foreground text-xs">Checked {ago(m.lastCheck, time.now)}</p>
						</div>
					{/each}
				</Card.Content>
			</Card.Root>
		</section>
	{/each}

	{#if resolved.length}
		<section class="grid gap-3">
			<h2 class="text-muted-foreground text-sm font-medium tracking-wide uppercase">Past incidents</h2>
			{#each resolved as incident (incident.id)}<Incident {incident} />{/each}
		</section>
	{/if}

	<footer class="text-muted-foreground mt-auto text-center text-xs">Powered by <a href="https://github.com/Unfield/Tern" class="hover:underline">Tern</a></footer>
</div>

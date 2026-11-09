<script lang="ts">
	import { BarChart } from 'layerchart';
	import CircleCheck from '@lucide/svelte/icons/circle-check';
	import Circle from '@lucide/svelte/icons/circle';
	import * as Card from '$lib/components/ui/card/index.js';
	import * as Chart from '$lib/components/ui/chart/index.js';
	import { Skeleton } from '$lib/components/ui/skeleton/index.js';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	const stats = $derived([
		{ label: 'Projects', value: data.stats.projects, hint: `${data.stats.activeProjects} active`, id: 'projects' },
		{ label: 'Open tasks', value: data.stats.openTasks, hint: 'across all projects', id: 'open' },
		{ label: 'Completed', value: data.stats.doneTasks, hint: 'tasks done', id: 'done' },
		{
			label: 'Completion',
			value: `${Math.round((data.stats.doneTasks / Math.max(1, data.stats.doneTasks + data.stats.openTasks)) * 100)}%`,
			hint: 'of all tasks',
			id: 'rate'
		}
	]);

	const chartConfig = {
		created: { label: 'Tasks created', color: 'var(--chart-1)' }
	} satisfies Chart.ChartConfig;

	const perDay = $derived(
		(data.stats.perDay ?? []).map((d) => ({ ...d, label: new Date(d.day + 'T00:00:00').toLocaleDateString(undefined, { month: 'short', day: 'numeric' }) }))
	);

	const timeAgo = (iso: string) => {
		const minutes = Math.round((Date.now() - new Date(iso).getTime()) / 60000);
		if (minutes < 60) return `${Math.max(minutes, 1)}m ago`;
		if (minutes < 60 * 24) return `${Math.round(minutes / 60)}h ago`;
		return `${Math.round(minutes / 60 / 24)}d ago`;
	};
</script>

<svelte:head>
	<title>Overview · Acme Dashboard</title>
</svelte:head>

<div>
	<h1 class="text-2xl font-semibold tracking-tight">Welcome back, {data.displayName}</h1>
	<p class="text-muted-foreground">Here's what's happening with your projects.</p>
</div>

<div class="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
	{#each stats as s (s.id)}
		<Card.Root>
			<Card.Header>
				<Card.Description>{s.label}</Card.Description>
				<Card.Title class="text-3xl tabular-nums" data-testid="stat-{s.id}">{s.value}</Card.Title>
			</Card.Header>
			<Card.Footer class="text-muted-foreground text-sm">{s.hint}</Card.Footer>
		</Card.Root>
	{/each}
</div>

<div class="grid gap-4 lg:grid-cols-7">
	<Card.Root class="lg:col-span-4">
		<Card.Header>
			<Card.Title>Tasks created</Card.Title>
			<Card.Description>Last 14 days</Card.Description>
		</Card.Header>
		<Card.Content>
			<Chart.Container config={chartConfig} class="aspect-auto h-64 w-full" data-testid="chart">
				<BarChart data={perDay} x="label" axis="x" series={[{ key: 'created', label: chartConfig.created.label, color: chartConfig.created.color }]}>
					{#snippet tooltip()}<Chart.Tooltip />{/snippet}
				</BarChart>
			</Chart.Container>
		</Card.Content>
	</Card.Root>

	<Card.Root class="lg:col-span-3">
		<Card.Header>
			<Card.Title>Recent activity</Card.Title>
			<Card.Description>Latest task changes</Card.Description>
		</Card.Header>
		<Card.Content>
			{#await data.activity}
				<div class="grid gap-4" data-testid="activity-loading">
					{#each [1, 2, 3, 4] as i (i)}
						<div class="flex items-center gap-3">
							<Skeleton class="size-5 rounded-full" />
							<div class="grid flex-1 gap-1.5"><Skeleton class="h-4 w-3/4" /><Skeleton class="h-3 w-1/3" /></div>
						</div>
					{/each}
				</div>
			{:then activity}
				<ul class="grid gap-4" data-testid="activity">
					{#each activity ?? [] as a, i (i)}
						<li class="flex items-start gap-3">
							{#if a.done}<CircleCheck class="mt-0.5 size-5 text-emerald-600" />{:else}<Circle class="text-muted-foreground mt-0.5 size-5" />{/if}
							<div class="grid gap-0.5 text-sm">
								<span class="font-medium">{a.task}</span>
								<span class="text-muted-foreground">{a.project} · {timeAgo(a.at)}</span>
							</div>
						</li>
					{:else}
						<li class="text-muted-foreground text-sm">No activity yet.</li>
					{/each}
				</ul>
			{:catch error}
				<p class="text-destructive text-sm">Could not load activity: {error.message}</p>
			{/await}
		</Card.Content>
	</Card.Root>
</div>

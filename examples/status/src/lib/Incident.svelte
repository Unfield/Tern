<script lang="ts">
	import { Badge } from '$lib/components/ui/badge/index.js';
	import * as Card from '$lib/components/ui/card/index.js';
	import { datetime } from './format.js';

	type Update = { id: number; status: string; html: string; createdAt: number };
	type Incident = { id: number; title: string; status: string; impact: string; createdAt: number; resolvedAt: number | null; updates: Update[] | null };
	let { incident, href }: { incident: Incident; href?: string } = $props();

	const tone = $derived(incident.resolvedAt ? 'border-l-emerald-500' : incident.impact === 'maintenance' ? 'border-l-sky-500' : incident.impact === 'major' ? 'border-l-red-500' : 'border-l-amber-500');
</script>

<Card.Root class="border-l-4 {tone}" data-testid="incident">
	<Card.Header>
		<div class="flex items-start justify-between gap-3">
			<Card.Title class="text-base">
				{#if href}<a {href} class="hover:underline">{incident.title}</a>{:else}{incident.title}{/if}
			</Card.Title>
			<Badge variant="outline" class="capitalize">{incident.status}</Badge>
		</div>
		<Card.Description>{datetime(incident.createdAt)}{incident.impact === 'maintenance' ? ' · Maintenance' : ''}</Card.Description>
	</Card.Header>
	<Card.Content class="grid gap-4">
		{#each incident.updates ?? [] as u (u.id)}
			<div class="grid gap-1">
				<p class="text-sm"><span class="font-medium capitalize">{u.status}</span> <span class="text-muted-foreground">· {datetime(u.createdAt)}</span></p>
				<div class="prose-sm text-sm [&_a]:underline [&_code]:bg-muted [&_code]:rounded [&_code]:px-1">{@html u.html}</div>
			</div>
		{/each}
	</Card.Content>
</Card.Root>

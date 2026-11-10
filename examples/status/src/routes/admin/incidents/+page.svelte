<script lang="ts">
	import { enhance } from '@ternkit/tern';
	import Incident from '$lib/Incident.svelte';
	import Field from '$lib/Field.svelte';
	import { Button } from '$lib/components/ui/button/index.js';
	import * as Card from '$lib/components/ui/card/index.js';
	import { Label } from '$lib/components/ui/label/index.js';
	import { Textarea } from '$lib/components/ui/textarea/index.js';
	import type { PageProps } from './$types';

	let { data, form }: PageProps = $props();
	const open = $derived((data.incidents ?? []).filter((i) => !i.resolvedAt));
	const resolved = $derived((data.incidents ?? []).filter((i) => i.resolvedAt));
</script>

<svelte:head><title>Incidents · Status</title></svelte:head>

<h1 class="text-2xl font-semibold tracking-tight">Incidents</h1>

<div class="grid gap-6 lg:grid-cols-[1fr_380px]">
	<div class="grid content-start gap-4">
		<h2 class="text-muted-foreground text-sm font-medium uppercase">Open</h2>
		{#each open as incident (incident.id)}
			<Incident {incident} href="/admin/incidents/{incident.id}" />
		{:else}
			<p class="text-muted-foreground text-sm">No open incidents. Monitors open one when they go down.</p>
		{/each}
		{#if resolved.length}
			<h2 class="text-muted-foreground mt-4 text-sm font-medium uppercase">Resolved (90 days)</h2>
			{#each resolved as incident (incident.id)}<Incident {incident} href="/admin/incidents/{incident.id}" />{/each}
		{/if}
	</div>

	<Card.Root class="self-start">
		<Card.Header>
			<Card.Title>New incident</Card.Title>
			<Card.Description>Shown on the status page at once. Markdown works.</Card.Description>
		</Card.Header>
		<Card.Content>
			<form method="POST" use:enhance class="grid gap-4">
				<Field label="Title" name="title" value={form?.title ?? ''} error={form?.errors?.title} placeholder="Slow responses from the API" />
				<div class="grid gap-2">
					<Label for="impact">Impact</Label>
					<select id="impact" name="impact" class="border-input bg-background h-9 rounded-md border px-3 text-sm">
						<option value="minor">Minor: degraded performance</option>
						<option value="major">Major: outage</option>
						<option value="maintenance">Maintenance</option>
					</select>
				</div>
				<div class="grid gap-2">
					<Label for="monitor">Affected monitor</Label>
					<select id="monitor" name="monitor" class="border-input bg-background h-9 rounded-md border px-3 text-sm">
						<option value="">None</option>
						{#each data.monitors ?? [] as m (m.id)}<option value={m.id}>{m.name}</option>{/each}
					</select>
				</div>
				<div class="grid gap-2">
					<Label for="body">Message</Label>
					<Textarea id="body" name="body" rows={5} value={form?.body ?? ''} placeholder="We are investigating reports of…" />
					{#if form?.errors?.body}<p class="text-destructive text-sm">{form.errors.body}</p>{/if}
				</div>
				<Button type="submit">Open incident</Button>
			</form>
		</Card.Content>
	</Card.Root>
</div>

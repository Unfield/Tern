<script lang="ts">
	import { enhance } from '@ternkit/tern';
	import Incident from '$lib/Incident.svelte';
	import { Button } from '$lib/components/ui/button/index.js';
	import * as Card from '$lib/components/ui/card/index.js';
	import { Label } from '$lib/components/ui/label/index.js';
	import { Textarea } from '$lib/components/ui/textarea/index.js';
	import type { PageProps } from './$types';

	let { data, form }: PageProps = $props();
	let body = $state('');
</script>

<svelte:head><title>{data.incident.title} · Status</title></svelte:head>

<p class="text-muted-foreground text-sm"><a href="/admin/incidents" class="hover:underline">Incidents</a></p>

<div class="grid gap-6 lg:grid-cols-[1fr_380px]">
	<Incident incident={data.incident} />
	<div class="grid content-start gap-4">
		<Card.Root>
			<Card.Header><Card.Title>Post an update</Card.Title></Card.Header>
			<Card.Content>
				<form
					method="POST"
					action="?/update"
					class="grid gap-4"
					use:enhance={() =>
						async ({ result, update }) => {
							await update();
							if (result.type === 'success') body = '';
						}}
				>
					<div class="grid gap-2">
						<Label for="status">Status</Label>
						<select id="status" name="status" class="border-input bg-background h-9 rounded-md border px-3 text-sm">
							{#each ['investigating', 'identified', 'monitoring', 'resolved'] as s (s)}
								<option value={s} selected={s === data.incident.status} class="capitalize">{s}</option>
							{/each}
						</select>
					</div>
					<div class="grid gap-2">
						<Label for="body">Update</Label>
						<Textarea id="body" name="body" rows={5} bind:value={body} placeholder="We found the cause and are rolling out a fix." />
					</div>
					{#if form?.error}<p class="text-destructive text-sm" role="alert">{form.error}</p>{/if}
					<Button type="submit">Post update</Button>
				</form>
			</Card.Content>
		</Card.Root>
		<form method="POST" action="?/delete"><Button variant="ghost" class="text-destructive w-full" type="submit">Delete incident</Button></form>
	</div>
</div>

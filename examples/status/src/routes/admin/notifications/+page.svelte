<script lang="ts">
	import { enhance } from '@ternkit/tern';
	import { toast } from 'svelte-sonner';
	import Send from '@lucide/svelte/icons/send';
	import Trash from '@lucide/svelte/icons/trash-2';
	import Field from '$lib/Field.svelte';
	import { Badge } from '$lib/components/ui/badge/index.js';
	import { Button } from '$lib/components/ui/button/index.js';
	import * as Card from '$lib/components/ui/card/index.js';
	import { Label } from '$lib/components/ui/label/index.js';
	import * as Table from '$lib/components/ui/table/index.js';
	import { datetime } from '$lib/format.js';
	import type { PageProps } from './$types';

	let { data, form }: PageProps = $props();
	// svelte-ignore state_referenced_locally
	let type = $state(form?.values?.type ?? 'webhook');
	const labels: Record<string, string> = {
		url: 'Webhook URL',
		host: 'SMTP host',
		port: 'Port',
		username: 'Username',
		password: 'Password',
		from: 'From',
		to: 'To (comma-separated)'
	};
	const typeNames: Record<string, string> = { webhook: 'Webhook', slack: 'Slack', discord: 'Discord', email: 'Email (SMTP)' };

	$effect(() => {
		if (form?.tested) {
			if (form.error) toast.error(`${form.tested}: ${form.error}`);
			else toast.success(`Sent a test notification to ${form.tested}.`);
		}
	});
</script>

<svelte:head><title>Notifications · Status</title></svelte:head>

<div>
	<h1 class="text-2xl font-semibold tracking-tight">Notifications</h1>
	<p class="text-muted-foreground text-sm">Every enabled channel hears when a monitor goes down and when it comes back. Failed deliveries are retried.</p>
</div>

<div class="grid gap-6 lg:grid-cols-[1fr_380px]">
	<Card.Root class="self-start">
		<Card.Content class="divide-y p-0" data-testid="channels">
			{#each data.channels ?? [] as c (c.id)}
				<div class="flex flex-wrap items-center gap-3 p-4" data-testid="channel">
					<div class="min-w-0 flex-1">
						<p class="font-medium">{c.name} <Badge variant="outline">{typeNames[c.type] ?? c.type}</Badge></p>
						<p class="text-muted-foreground truncate text-xs">{c.config?.url ?? c.config?.to ?? ''}</p>
					</div>
					<form method="POST" action="?/toggle" use:enhance>
						<input type="hidden" name="id" value={c.id} />
						<Button size="sm" variant="ghost" type="submit">{c.enabled ? 'Enabled' : 'Disabled'}</Button>
					</form>
					<form method="POST" action="?/test" use:enhance>
						<input type="hidden" name="id" value={c.id} />
						<Button size="sm" variant="outline" type="submit"><Send /> Test</Button>
					</form>
					<form method="POST" action="?/delete" use:enhance>
						<input type="hidden" name="id" value={c.id} />
						<Button size="icon" variant="ghost" type="submit" aria-label="Delete {c.name}"><Trash /></Button>
					</form>
				</div>
			{:else}
				<p class="text-muted-foreground p-8 text-center text-sm">No channels yet. Add a webhook, Slack, Discord or email channel.</p>
			{/each}
		</Card.Content>
	</Card.Root>

	<Card.Root class="self-start">
		<Card.Header><Card.Title>Add a channel</Card.Title></Card.Header>
		<Card.Content>
			<form method="POST" action="?/create" use:enhance class="grid gap-4">
				<div class="grid gap-2">
					<Label for="type">Type</Label>
					<select id="type" name="type" bind:value={type} class="border-input bg-background h-9 rounded-md border px-3 text-sm">
						{#each Object.keys(data.types ?? {}) as t (t)}<option value={t}>{typeNames[t] ?? t}</option>{/each}
					</select>
				</div>
				<Field label="Name" name="name" value={form?.values?.name ?? ''} error={form?.errors?.name} placeholder="On-call" />
				{#each data.types?.[type] ?? [] as f (f)}
					<Field label={labels[f] ?? f} name={f} type={f === 'password' ? 'password' : 'text'} value={String(form?.values?.config?.[f] ?? '')} error={form?.errors?.[f]} />
				{/each}
				<Button type="submit">Add channel</Button>
			</form>
		</Card.Content>
	</Card.Root>
</div>

<Card.Root>
	<Card.Header><Card.Title>Recent deliveries</Card.Title><Card.Description>Notifications are sent by background jobs</Card.Description></Card.Header>
	<Card.Content class="p-0">
		<Table.Root>
			<Table.Body>
				{#each data.deliveries ?? [] as d (d.id)}
					<Table.Row>
						<Table.Cell>{d.monitor} is {d.status}</Table.Cell>
						<Table.Cell class="text-muted-foreground">{d.channel || 'deleted channel'}</Table.Cell>
						<Table.Cell><Badge variant={d.state === 'failed' ? 'destructive' : 'secondary'}>{d.state}</Badge></Table.Cell>
						<Table.Cell class="text-muted-foreground max-w-64 truncate text-xs" title={d.lastError}>{d.lastError}</Table.Cell>
						<Table.Cell class="text-muted-foreground text-right text-xs whitespace-nowrap">{datetime(new Date(d.at).getTime())}</Table.Cell>
						<Table.Cell class="w-20 text-right">
							{#if d.state === 'failed'}
								<form method="POST" action="?/retry" use:enhance><input type="hidden" name="id" value={d.id} /><Button size="sm" variant="ghost" type="submit">Retry</Button></form>
							{/if}
						</Table.Cell>
					</Table.Row>
				{:else}
					<Table.Row><Table.Cell class="text-muted-foreground h-16 text-center">Nothing sent yet.</Table.Cell></Table.Row>
				{/each}
			</Table.Body>
		</Table.Root>
	</Card.Content>
</Card.Root>

<script lang="ts">
	import { enhance, goto } from '@ternkit/tern';
	import { toast } from 'svelte-sonner';
	import Plus from '@lucide/svelte/icons/plus';
	import Search from '@lucide/svelte/icons/search';
	import { Badge } from '$lib/components/ui/badge/index.js';
	import { Button } from '$lib/components/ui/button/index.js';
	import * as Dialog from '$lib/components/ui/dialog/index.js';
	import { Input } from '$lib/components/ui/input/index.js';
	import { Label } from '$lib/components/ui/label/index.js';
	import { Progress } from '$lib/components/ui/progress/index.js';
	import * as Table from '$lib/components/ui/table/index.js';
	import * as Tabs from '$lib/components/ui/tabs/index.js';
	import { Textarea } from '$lib/components/ui/textarea/index.js';
	import type { PageProps } from './$types';

	let { data, form }: PageProps = $props();
	let open = $state(false);

	const statusVariant = { active: 'default', paused: 'secondary', done: 'outline' } as const;
	const filters = [
		{ value: 'all', label: 'All' },
		{ value: 'active', label: 'Active' },
		{ value: 'paused', label: 'Paused' },
		{ value: 'done', label: 'Done' }
	];

	function filter(status: string) {
		const params = new URLSearchParams();
		if (data.query) params.set('q', data.query);
		if (status !== 'all') params.set('status', status);
		const qs = params.toString();
		void goto(`/projects${qs ? `?${qs}` : ''}`, { keepFocus: true, noScroll: true });
	}
</script>

<svelte:head>
	<title>Projects · Acme Dashboard</title>
</svelte:head>

<div class="flex flex-wrap items-center justify-between gap-4">
	<div>
		<h1 class="text-2xl font-semibold tracking-tight">Projects</h1>
		<p class="text-muted-foreground">{data.projects?.length ?? 0} {data.projects?.length === 1 ? 'project' : 'projects'}</p>
	</div>

	<Dialog.Root bind:open>
		<Dialog.Trigger>
			{#snippet child({ props })}<Button {...props}><Plus /> New project</Button>{/snippet}
		</Dialog.Trigger>
		<Dialog.Content class="sm:max-w-md">
			<Dialog.Header>
				<Dialog.Title>New project</Dialog.Title>
				<Dialog.Description>Projects group related tasks.</Dialog.Description>
			</Dialog.Header>
			<form
				method="POST"
				action="?/create"
				class="grid gap-4"
				use:enhance={() =>
					async ({ result, update }) => {
						await update();
						if (result.type === 'success') {
							open = false;
							toast.success('Project created');
						}
					}}
			>
				<div class="grid gap-2">
					<Label for="name">Name</Label>
					<Input id="name" name="name" value={form?.values?.name ?? ''} aria-invalid={form?.errors?.name ? true : undefined} />
					{#if form?.errors?.name}<p class="text-destructive text-sm" role="alert">{form.errors.name}</p>{/if}
				</div>
				<div class="grid gap-2">
					<Label for="description">Description</Label>
					<Textarea id="description" name="description" value={form?.values?.description ?? ''} rows={3} />
					{#if form?.errors?.description}<p class="text-destructive text-sm" role="alert">{form.errors.description}</p>{/if}
				</div>
				<Dialog.Footer>
					<Button type="submit">Create project</Button>
				</Dialog.Footer>
			</form>
		</Dialog.Content>
	</Dialog.Root>
</div>

<div class="flex flex-wrap items-center justify-between gap-4">
	<Tabs.Root value={data.status || 'all'} onValueChange={filter}>
		<Tabs.List>
			{#each filters as f (f.value)}<Tabs.Trigger value={f.value}>{f.label}</Tabs.Trigger>{/each}
		</Tabs.List>
	</Tabs.Root>

	<form method="GET" class="relative w-full max-w-xs" role="search">
		<Search class="text-muted-foreground pointer-events-none absolute top-2.5 left-2.5 size-4" />
		<Input name="q" type="search" placeholder="Search projects…" value={data.query} class="pl-8" aria-label="Search projects" />
		{#if data.status}<input type="hidden" name="status" value={data.status} />{/if}
	</form>
</div>

<div class="rounded-lg border">
	<Table.Root>
		<Table.Header>
			<Table.Row>
				<Table.Head>Name</Table.Head>
				<Table.Head>Status</Table.Head>
				<Table.Head class="w-48">Progress</Table.Head>
				<Table.Head class="text-right">Tasks</Table.Head>
			</Table.Row>
		</Table.Header>
		<Table.Body data-testid="projects">
			{#each data.projects ?? [] as p (p.id)}
				<Table.Row>
					<Table.Cell>
						<a href="/projects/{p.id}" class="font-medium hover:underline">{p.name}</a>
						{#if p.description}<p class="text-muted-foreground line-clamp-1 text-sm">{p.description}</p>{/if}
					</Table.Cell>
					<Table.Cell><Badge variant={statusVariant[p.status as keyof typeof statusVariant] ?? 'outline'}>{p.status}</Badge></Table.Cell>
					<Table.Cell><Progress value={p.taskCount ? (p.doneCount / p.taskCount) * 100 : 0} class="h-2" /></Table.Cell>
					<Table.Cell class="text-right tabular-nums">{p.doneCount}/{p.taskCount}</Table.Cell>
				</Table.Row>
			{:else}
				<Table.Row>
					<Table.Cell colspan={4} class="text-muted-foreground h-24 text-center">No projects match.</Table.Cell>
				</Table.Row>
			{/each}
		</Table.Body>
	</Table.Root>
</div>

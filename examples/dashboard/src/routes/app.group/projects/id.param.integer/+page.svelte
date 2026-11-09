<script lang="ts">
	import { enhance } from '@ternkit/tern';
	import { toast } from 'svelte-sonner';
	import EllipsisVertical from '@lucide/svelte/icons/ellipsis-vertical';
	import { Badge } from '$lib/components/ui/badge/index.js';
	import * as Breadcrumb from '$lib/components/ui/breadcrumb/index.js';
	import { Button } from '$lib/components/ui/button/index.js';
	import * as Card from '$lib/components/ui/card/index.js';
	import { Checkbox } from '$lib/components/ui/checkbox/index.js';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu/index.js';
	import { Input } from '$lib/components/ui/input/index.js';
	import { Label } from '$lib/components/ui/label/index.js';
	import { Progress } from '$lib/components/ui/progress/index.js';
	import * as Select from '$lib/components/ui/select/index.js';
	import * as Tabs from '$lib/components/ui/tabs/index.js';
	import { Textarea } from '$lib/components/ui/textarea/index.js';
	import type { PageProps } from './$types';

	let { data, form }: PageProps = $props();

	const priorities = [
		{ value: 'high', label: 'High' },
		{ value: 'medium', label: 'Medium' },
		{ value: 'low', label: 'Low' }
	];
	const statuses = [
		{ value: 'active', label: 'Active' },
		{ value: 'paused', label: 'Paused' },
		{ value: 'done', label: 'Done' }
	];
	const priorityVariant = { high: 'destructive', medium: 'secondary', low: 'outline' } as const;

	let priority = $state('medium');
	let status = $state('');
	$effect.pre(() => {
		status = data.project.status;
	});

	const done = $derived((data.tasks ?? []).filter((t) => t.done).length);
	const total = $derived(data.tasks?.length ?? 0);
	const toggleForms: Record<number, HTMLFormElement> = $state({});
	const label = (list: { value: string; label: string }[], v: string) => list.find((i) => i.value === v)?.label ?? 'Select…';
</script>

<svelte:head>
	<title>{data.project.name} · Acme Dashboard</title>
</svelte:head>

<Breadcrumb.Root>
	<Breadcrumb.List>
		<Breadcrumb.Item><Breadcrumb.Link href="/projects">Projects</Breadcrumb.Link></Breadcrumb.Item>
		<Breadcrumb.Separator />
		<Breadcrumb.Item><Breadcrumb.Page>{data.project.name}</Breadcrumb.Page></Breadcrumb.Item>
	</Breadcrumb.List>
</Breadcrumb.Root>

<div class="flex flex-wrap items-start justify-between gap-4">
	<div class="grid gap-1">
		<h1 class="text-2xl font-semibold tracking-tight" data-testid="project-name">{data.project.name}</h1>
		{#if data.project.description}<p class="text-muted-foreground">{data.project.description}</p>{/if}
	</div>
	<Badge>{data.project.status}</Badge>
</div>

<div class="grid gap-2">
	<div class="flex justify-between text-sm"><span>Progress</span><span class="tabular-nums" data-testid="progress">{done}/{total} done</span></div>
	<Progress value={total ? (done / total) * 100 : 0} />
</div>

<Tabs.Root value="tasks">
	<Tabs.List>
		<Tabs.Trigger value="tasks">Tasks</Tabs.Trigger>
		<Tabs.Trigger value="settings">Settings</Tabs.Trigger>
	</Tabs.List>

	<Tabs.Content value="tasks" class="grid gap-6 pt-4">
		<Card.Root>
			<Card.Header><Card.Title>Add a task</Card.Title></Card.Header>
			<Card.Content>
				<form
					method="POST"
					action="?/addTask"
					class="grid gap-4 sm:grid-cols-[1fr_10rem_auto] sm:items-start"
					use:enhance={() =>
						async ({ result, update }) => {
							await update();
							if (result.type === 'success') toast.success('Task added');
						}}
				>
					<div class="grid gap-2">
						<Label for="title">Title</Label>
						<Input id="title" name="title" value={form?.values?.title ?? ''} aria-invalid={form?.errors?.title ? true : undefined} />
						{#if form?.errors?.title}<p class="text-destructive text-sm" role="alert">{form.errors.title}</p>{/if}
					</div>
					<div class="grid gap-2">
						<Label for="priority">Priority</Label>
						<Select.Root type="single" name="priority" bind:value={priority}>
							<Select.Trigger id="priority" class="w-full" aria-label="Priority">{label(priorities, priority)}</Select.Trigger>
							<Select.Content>
								{#each priorities as p (p.value)}<Select.Item value={p.value} label={p.label} />{/each}
							</Select.Content>
						</Select.Root>
					</div>
					<Button type="submit" class="sm:mt-[1.375rem]">Add task</Button>
					<div class="grid gap-2 sm:col-span-3">
						<Label for="notes">Notes</Label>
						<Textarea id="notes" name="notes" rows={2} value={form?.values?.notes ?? ''} />
					</div>
				</form>
			</Card.Content>
		</Card.Root>

		<ul class="divide-y rounded-lg border" data-testid="tasks">
			{#each data.tasks ?? [] as t (t.id)}
				<li class="flex items-center gap-3 p-3">
					<form method="POST" action="?/toggle" use:enhance class="flex" bind:this={toggleForms[t.id]}>
						<input type="hidden" name="task" value={t.id} />
						<input type="hidden" name="done" value={String(!t.done)} />
						<Checkbox
							checked={t.done}
							aria-label="{t.done ? 'Reopen' : 'Complete'} {t.title}"
							onCheckedChange={() => toggleForms[t.id]?.requestSubmit()}
						/>
					</form>
					<div class="grid flex-1 gap-0.5">
						<span class={t.done ? 'text-muted-foreground line-through' : 'font-medium'}>{t.title}</span>
						{#if t.notes}<span class="text-muted-foreground text-sm">{t.notes}</span>{/if}
					</div>
					<Badge variant={priorityVariant[t.priority as keyof typeof priorityVariant] ?? 'outline'}>{t.priority}</Badge>
					<form method="POST" action="?/deleteTask" use:enhance id="delete-{t.id}">
						<input type="hidden" name="task" value={t.id} />
					</form>
					<DropdownMenu.Root>
						<DropdownMenu.Trigger>
							{#snippet child({ props })}
								<Button variant="ghost" size="icon" {...props} aria-label="Actions for {t.title}"><EllipsisVertical /></Button>
							{/snippet}
						</DropdownMenu.Trigger>
						<DropdownMenu.Content align="end">
							<DropdownMenu.Item
								variant="destructive"
								onSelect={() => (document.getElementById(`delete-${t.id}`) as HTMLFormElement).requestSubmit()}>Delete task</DropdownMenu.Item
							>
						</DropdownMenu.Content>
					</DropdownMenu.Root>
				</li>
			{:else}
				<li class="text-muted-foreground p-6 text-center text-sm">No tasks yet.</li>
			{/each}
		</ul>
	</Tabs.Content>

	<Tabs.Content value="settings" class="pt-4">
		<Card.Root>
			<Card.Header>
				<Card.Title>Project settings</Card.Title>
			</Card.Header>
			<Card.Content>
				<form
					method="POST"
					action="?/update"
					class="grid max-w-lg gap-4"
					use:enhance={() =>
						async ({ result, update }) => {
							await update({ reset: false });
							if (result.type === 'success') toast.success('Project saved');
						}}
				>
					<div class="grid gap-2">
						<Label for="project-name">Name</Label>
						<Input id="project-name" name="name" value={data.project.name} />
						{#if form?.errors?.name}<p class="text-destructive text-sm" role="alert">{form.errors.name}</p>{/if}
					</div>
					<div class="grid gap-2">
						<Label for="project-description">Description</Label>
						<Textarea id="project-description" name="description" value={data.project.description} />
					</div>
					<div class="grid gap-2">
						<Label for="project-status">Status</Label>
						<Select.Root type="single" name="status" bind:value={status}>
							<Select.Trigger id="project-status" class="w-48">{label(statuses, status)}</Select.Trigger>
							<Select.Content>
								{#each statuses as s (s.value)}<Select.Item value={s.value} label={s.label} />{/each}
							</Select.Content>
						</Select.Root>
					</div>
					<div class="flex gap-2">
						<Button type="submit">Save changes</Button>
						<Button type="submit" variant="destructive" formaction="?/delete">Delete project</Button>
					</div>
				</form>
			</Card.Content>
		</Card.Root>
	</Tabs.Content>
</Tabs.Root>

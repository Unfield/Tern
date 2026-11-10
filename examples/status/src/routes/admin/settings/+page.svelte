<script lang="ts">
	import { enhance } from '@ternkit/tern';
	import Field from '$lib/Field.svelte';
	import { Button } from '$lib/components/ui/button/index.js';
	import * as Card from '$lib/components/ui/card/index.js';
	import { Label } from '$lib/components/ui/label/index.js';
	import { Textarea } from '$lib/components/ui/textarea/index.js';
	import type { PageProps } from './$types';

	let { data, form }: PageProps = $props();
</script>

<svelte:head><title>Settings · Status</title></svelte:head>

<h1 class="text-2xl font-semibold tracking-tight">Settings</h1>

<div class="grid gap-6 lg:grid-cols-2">
	<Card.Root>
		<Card.Header><Card.Title>Status page</Card.Title><Card.Description>What visitors see at the top of the page</Card.Description></Card.Header>
		<Card.Content>
			<form method="POST" action="?/page" use:enhance class="grid gap-4">
				<Field label="Title" name="title" value={data.title} error={form?.errors?.title} />
				<div class="grid gap-2">
					<Label for="description">Description</Label>
					<Textarea id="description" name="description" rows={3} value={data.description} />
				</div>
				<div class="flex items-center gap-3">
					<Button type="submit">Save</Button>
					{#if form?.saved === 'page'}<span class="text-sm text-emerald-600" role="status">Saved.</span>{/if}
				</div>
			</form>
		</Card.Content>
	</Card.Root>
	<Card.Root>
		<Card.Header><Card.Title>Password</Card.Title></Card.Header>
		<Card.Content>
			<form method="POST" action="?/password" use:enhance class="grid gap-4">
				<Field label="Current password" name="current" type="password" error={form?.errors?.current} autocomplete="current-password" />
				<Field label="New password" name="new" type="password" error={form?.errors?.new} autocomplete="new-password" />
				<div class="flex items-center gap-3">
					<Button type="submit">Change password</Button>
					{#if form?.saved === 'password'}<span class="text-sm text-emerald-600" role="status">Password changed.</span>{/if}
				</div>
			</form>
		</Card.Content>
	</Card.Root>
</div>

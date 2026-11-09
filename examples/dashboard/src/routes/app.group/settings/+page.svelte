<script lang="ts">
	import { beforeNavigate, enhance } from '@ternkit/tern';
	import { toast } from 'svelte-sonner';
	import { Button } from '$lib/components/ui/button/index.js';
	import * as Card from '$lib/components/ui/card/index.js';
	import { Checkbox } from '$lib/components/ui/checkbox/index.js';
	import { Input } from '$lib/components/ui/input/index.js';
	import { Label } from '$lib/components/ui/label/index.js';
	import type { PageProps } from './$types';

	let { data, form }: PageProps = $props();
	const values = $derived(form?.values ?? data.profile);

	// Warn before leaving with unsaved changes.
	let dirty = $state(false);
	beforeNavigate((nav) => {
		if (dirty && !confirm('You have unsaved changes. Leave this page?')) nav.cancel();
	});
</script>

<svelte:head>
	<title>Settings · Acme Dashboard</title>
</svelte:head>

<div>
	<h1 class="text-2xl font-semibold tracking-tight">Settings</h1>
	<p class="text-muted-foreground">Manage your profile and notifications.</p>
</div>

<Card.Root class="max-w-xl">
	<Card.Header>
		<Card.Title>Profile</Card.Title>
		<Card.Description>Shown in the sidebar and on your projects.</Card.Description>
	</Card.Header>
	<Card.Content>
		<form
			method="POST"
			class="grid gap-5"
			oninput={() => (dirty = true)}
			use:enhance={() =>
				async ({ result, update }) => {
					await update({ reset: false });
					if (result.type === 'success') {
						dirty = false;
						toast.success('Settings saved');
					}
				}}
		>
			<div class="grid gap-2">
				<Label for="displayName">Display name</Label>
				<Input id="displayName" name="displayName" value={values.displayName} aria-invalid={form?.errors?.displayName ? true : undefined} />
				{#if form?.errors?.displayName}<p class="text-destructive text-sm" role="alert">{form.errors.displayName}</p>{/if}
			</div>
			<div class="grid gap-2">
				<Label for="email">Email</Label>
				<Input id="email" name="email" type="text" inputmode="email" value={values.email} aria-invalid={form?.errors?.email ? true : undefined} />
				{#if form?.errors?.email}<p class="text-destructive text-sm" role="alert">{form.errors.email}</p>{/if}
			</div>
			<div class="flex items-center gap-2">
				<Checkbox id="weeklyDigest" name="weeklyDigest" checked={values.weeklyDigest} />
				<Label for="weeklyDigest">Email me a weekly digest</Label>
			</div>
			<Button type="submit" class="w-fit">Save</Button>
		</form>
	</Card.Content>
</Card.Root>

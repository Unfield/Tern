<script lang="ts">
	import { enhance } from '@ternkit/tern';
	import { Button } from '$lib/components/ui/button/index.js';
	import * as Card from '$lib/components/ui/card/index.js';
	import { Input } from '$lib/components/ui/input/index.js';
	import { Label } from '$lib/components/ui/label/index.js';
	import type { PageProps } from './$types';

	let { form }: PageProps = $props();
	let submitting = $state(false);
</script>

<svelte:head>
	<title>Sign in · Acme Dashboard</title>
</svelte:head>

<main class="bg-muted flex min-h-svh items-center justify-center p-6">
	<Card.Root class="w-full max-w-sm">
		<Card.Header>
			<Card.Title class="text-2xl">Sign in</Card.Title>
			<Card.Description>Enter your name to open your dashboard. New names get sample projects.</Card.Description>
		</Card.Header>
		<Card.Content>
			<form
				method="POST"
				class="grid gap-4"
				use:enhance={() => {
					submitting = true;
					return async ({ update }) => {
						await update();
						submitting = false;
					};
				}}
			>
				<div class="grid gap-2">
					<Label for="name">Name</Label>
					<Input id="name" name="name" value={form?.name ?? ''} autocomplete="name" aria-invalid={form?.error ? true : undefined} />
					{#if form?.error}<p class="text-destructive text-sm" role="alert">{form.error}</p>{/if}
				</div>
				<Button type="submit" disabled={submitting}>{submitting ? 'Signing in…' : 'Continue'}</Button>
			</form>
		</Card.Content>
	</Card.Root>
</main>

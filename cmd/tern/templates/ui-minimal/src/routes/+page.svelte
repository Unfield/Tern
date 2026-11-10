<script lang="ts">
	import { enhance } from '@ternkit/tern';
	import { Alert, Button, Card, Field, Input } from '@ternkit/tern/ui';
	import type { PageProps } from './$types';

	let { data, form }: PageProps = $props();
	let sending = $state(false);
</script>

<svelte:head>
	<title>Welcome to Tern</title>
</svelte:head>

<h1>{data.message}</h1>
<p class="lead">The server rendered this data at {new Date(data.serverTime).toLocaleTimeString()}.</p>

<Card title="Say hello" description="This form runs a Go action in src/routes/page.server.go." level={2}>
	<form
		method="POST"
		use:enhance={() => {
			sending = true;
			return async ({ update }) => {
				await update();
				sending = false;
			};
		}}
	>
		<Field label="Your name" error={form?.error}>
			<Input name="name" value={form?.name ?? ''} autocomplete="name" />
		</Field>
		<Button type="submit" loading={sending}>Say hello</Button>
	</form>
	{#if form?.greeting}
		<Alert tone="success" live>{form.greeting}</Alert>
	{/if}
</Card>

<p class="hint">
	Edit <code>src/routes/+page.svelte</code> to get started. The components come from <code>@ternkit/tern/ui</code>.
</p>

<style>
	h1 {
		margin: 0 0 0.25rem;
		font-size: 1.75rem;
	}
	.lead,
	.hint {
		color: var(--tern-muted);
	}
	.lead {
		margin: 0 0 1.5rem;
	}
	.hint {
		margin-top: 1.5rem;
		font-size: 0.875rem;
	}
	form {
		display: flex;
		gap: 0.75rem;
		align-items: end;
		margin-bottom: 1rem;
	}
	form :global(.tern-field) {
		flex: 1;
	}
</style>

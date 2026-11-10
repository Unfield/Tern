<script lang="ts">
	import { enhance } from '@ternkit/tern';
	import AuthCard from '$lib/AuthCard.svelte';
	import Field from '$lib/Field.svelte';
	import { Button } from '$lib/components/ui/button/index.js';
	import type { PageProps } from './$types';

	let { form }: PageProps = $props();
</script>

<svelte:head><title>Set up · Status</title></svelte:head>

<AuthCard title="Welcome" description="Create the administrator account. You'll use it to add monitors and post incidents.">
	<form method="POST" class="grid gap-4" use:enhance>
		<Field label="Username" name="username" value={form?.username ?? ''} error={form?.errors?.username} autocomplete="username" />
		<Field label="Password" name="password" type="password" error={form?.errors?.password} autocomplete="new-password" />
		<Field label="Confirm password" name="confirm" type="password" error={form?.errors?.confirm} autocomplete="new-password" />
		<Button type="submit">Create account</Button>
	</form>
</AuthCard>

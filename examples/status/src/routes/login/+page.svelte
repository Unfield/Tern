<script lang="ts">
	import { enhance } from '@ternkit/tern';
	import AuthCard from '$lib/AuthCard.svelte';
	import Field from '$lib/Field.svelte';
	import { Button } from '$lib/components/ui/button/index.js';
	import type { PageProps } from './$types';

	let { form }: PageProps = $props();
</script>

<svelte:head><title>Sign in · Status</title></svelte:head>

<AuthCard title="Sign in" description="Manage monitors, incidents and notifications.">
	<form method="POST" class="grid gap-4" use:enhance>
		<Field label="Username" name="username" value={form?.username ?? ''} autocomplete="username" />
		<Field label="Password" name="password" type="password" autocomplete="current-password" />
		{#if form?.error}<p class="text-destructive text-sm" role="alert">{form.error}</p>{/if}
		<Button type="submit">Sign in</Button>
	</form>
	<p class="text-muted-foreground mt-4 text-center text-sm"><a href="/" class="hover:underline">← Status page</a></p>
</AuthCard>

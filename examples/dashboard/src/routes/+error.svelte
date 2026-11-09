<script lang="ts">
	import { page } from '@ternkit/tern';
	import { Button } from '$lib/components/ui/button/index.js';
</script>

<svelte:head>
	<title>{page.status} · Acme Dashboard</title>
</svelte:head>

<main class="flex min-h-svh flex-col items-center justify-center gap-4 p-6 text-center">
	<p class="text-muted-foreground text-sm font-medium" data-testid="error-status">{page.status}</p>
	<h1 class="text-3xl font-semibold tracking-tight" data-testid="error-message">{page.error?.message}</h1>
	<Button href="/">Back to the dashboard</Button>
</main>

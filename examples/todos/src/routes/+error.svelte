<script lang="ts">
	import { page } from '@ternkit/tern';
	import { route } from '$routes';
</script>

<svelte:head>
	<title>{page.status} · Todos</title>
</svelte:head>

<main>
	<h1 data-testid="error-status">{page.status}</h1>
	<p data-testid="error-message">{page.error?.message}</p>
	<p><a href={route('/')}>Back to your list</a></p>
</main>

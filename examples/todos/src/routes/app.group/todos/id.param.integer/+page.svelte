<script lang="ts">
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
</script>

<svelte:head>
	<title>{data.todo.title} · Todos</title>
</svelte:head>

<main>
	<p><a href="/">← All todos</a></p>
	<h1 data-testid="detail-title">{data.todo.title}</h1>
	<p>{data.todo.done ? 'Done' : 'Not done yet'} · created {new Date(data.todo.created).toLocaleString()}</p>
</main>

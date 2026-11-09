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

	<h2>Similar todos</h2>
	{#await data.similar}
		<p data-testid="similar-loading">Looking for similar todos…</p>
	{:then similar}
		{#if similar?.length}
			<ul data-testid="similar">
				{#each similar as todo (todo.id)}<li><a href="/todos/{todo.id}">{todo.title}</a></li>{/each}
			</ul>
		{:else}
			<p data-testid="similar">No similar todos.</p>
		{/if}
	{:catch error}
		<p class="error">Could not load similar todos: {error.message}</p>
	{/await}
</main>

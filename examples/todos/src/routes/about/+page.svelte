<script lang="ts">
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
</script>

<svelte:head>
	<title>About · Todos</title>
</svelte:head>

<main>
	<h1>About</h1>
	<ul data-testid="features">
		{#each data.features ?? [] as feature (feature)}<li>{feature}</li>{/each}
	</ul>
	<p><a href="/login">Sign in</a></p>
</main>

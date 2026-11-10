<script lang="ts">
	import { route } from '$routes';
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
	<p><a href={route('/login')}>Sign in</a> · <a href={route('/blog')}>Blog</a> · <a href={route('/privacy')}>Privacy</a></p>
</main>

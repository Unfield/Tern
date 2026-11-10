<script lang="ts">
	import { route } from '$routes';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
</script>

<svelte:head>
	<title>{data.data.title} · Todos</title>
</svelte:head>

<main>
	<p><a href={route('/blog')}>← Blog</a></p>
	<h1>{data.data.title}</h1>
	{#if data.headings?.length}
		<nav class="toc" aria-label="On this page">
			{#each data.headings ?? [] as h (h.id)}<a href="#{h.id}">{h.text}</a>{/each}
		</nav>
	{/if}
	<article class="markdown">{@html data.html}</article>
</main>

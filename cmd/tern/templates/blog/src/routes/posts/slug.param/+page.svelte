<script lang="ts">
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
</script>

<svelte:head>
	<title>{data.data.title}</title>
	<meta name="description" content={data.data.description || data.excerpt} />
</svelte:head>

<p><a href="/">← All posts</a></p>
<article>
	<h1>{data.data.title}</h1>
	<p class="meta">{data.data.date.slice(0, 10)} · {data.readingMinutes} min read</p>
	{@html data.html}
</article>

<style>
	.meta {
		color: #78716c;
	}
	article :global(pre) {
		background: #1c1917;
		color: #f5f5f4;
		padding: 0.75rem 1rem;
		border-radius: 6px;
		overflow-x: auto;
	}
</style>

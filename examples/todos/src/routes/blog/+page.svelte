<script lang="ts">
	import { route } from '$routes';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
</script>

<svelte:head>
	<title>Blog · Todos</title>
</svelte:head>

<main>
	<h1>Blog</h1>
	<ul class="posts" data-testid="posts">
		{#each data.posts ?? [] as post (post.slug)}
			<li>
				<a href={route('/blog/[slug]', { slug: post.slug })}>{post.data.title}</a>
				<small>{post.data.date.slice(0, 10)} · {post.readingMinutes} min</small>
				<p>{post.excerpt}</p>
			</li>
		{/each}
	</ul>
</main>

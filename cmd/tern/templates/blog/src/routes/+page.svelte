<script lang="ts">
	import { route } from '$routes';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
</script>

<svelte:head>
	<title>Blog</title>
	<link rel="alternate" type="application/rss+xml" title="Blog" href="/rss.xml" />
</svelte:head>

<h1>Blog</h1>

<ul class="posts">
	{#each data.posts ?? [] as post (post.slug)}
		<li>
			<a href={route('/posts/[slug]', { slug: post.slug })}>{post.data.title}</a>
			<small>{post.data.date.slice(0, 10)} · {post.readingMinutes} min read</small>
			<p>{post.data.description || post.excerpt}</p>
		</li>
	{/each}
</ul>

<style>
	.posts {
		list-style: none;
		padding: 0;
	}
	li {
		margin: 1.5rem 0;
	}
	a {
		font-size: 1.2rem;
		font-weight: 600;
	}
	small {
		display: block;
		color: #78716c;
	}
</style>

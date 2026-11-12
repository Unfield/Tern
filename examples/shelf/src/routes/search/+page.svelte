<script lang="ts">
	import { api } from '$api';
	import { Highlight, SearchBox } from '@ternkit/tern/ui';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
</script>

<svelte:head>
	<title>{data.q ? `${data.q} · Search` : 'Search'} · Shelf</title>
</svelte:head>

<h1>Search</h1>

<SearchBox action="/search" value={data.q} search={(q, signal) => api('GET /api/search', { query: { q }, signal })} label="Search your shelf" />

{#if data.q}
	<p class="count">{data.total === 1 ? '1 link' : `${data.total} links`} match “{data.q}”.</p>
	<ul class="results">
		{#each data.results as r (r.id)}
			<li>
				<a href={r.href}><Highlight text={r.title} /></a>
				<span class="url"><Highlight text={r.snippet} /></span>
			</li>
		{/each}
	</ul>
{/if}

<style>
	h1 {
		margin: 0 0 1.25rem;
		font-size: 1.75rem;
	}
	.count {
		margin: 1.25rem 0 0.5rem;
		color: var(--tern-muted);
		font-size: 0.875rem;
	}
	.results {
		list-style: none;
		margin: 0;
		padding: 0;
	}
	.results li {
		display: grid;
		gap: 0.125rem;
		padding: 0.75rem 0;
		border-bottom: 1px solid var(--tern-border);
	}
	.results a {
		color: var(--tern-fg);
		font-weight: 550;
		text-decoration: none;
	}
	.results a:hover {
		text-decoration: underline;
	}
	.url {
		color: var(--tern-muted);
		font-size: 0.8125rem;
		overflow-wrap: anywhere;
	}
</style>

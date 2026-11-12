<script lang="ts">
	import { api } from '$api';
	import { route } from '$routes';
	import { Button, Field, Form, Input, SearchBox } from '@ternkit/tern/ui';
	import type { PageProps } from './$types';
	import { forms } from './$forms';

	let { data, form }: PageProps = $props();
	let saving = $state(false);

	const filters = [
		{ show: 'unread', label: 'To read' },
		{ show: 'read', label: 'Read' },
		{ show: 'all', label: 'All' }
	];
</script>

<svelte:head>
	<title>Shelf</title>
</svelte:head>

<header class="top">
	<h1>Your shelf</h1>
	{#if data.flags?.search}
		<SearchBox action="/search" hotkey="/" search={(q, signal) => api('GET /api/search', { query: { q }, signal })} label="Search your shelf" class="find" />
	{/if}
</header>

<Form action="?/add" rules={forms.add} {form} bind:submitting={saving} class="add">
	<Field label="Address" name="url"><Input type="url" placeholder="https://" /></Field>
	<Field label="Title" name="title"><Input placeholder="Optional" /></Field>
	<Button type="submit" loading={saving}>Save</Button>
</Form>

<nav class="filters" aria-label="Show">
	{#each filters as f (f.show)}
		<a href="?show={f.show}" aria-current={data.show === f.show ? 'page' : undefined}>{f.label}</a>
	{/each}
</nav>

{#if data.links.length === 0}
	<p class="empty">{data.show === 'read' ? 'Nothing read yet.' : 'Nothing here. Save a link above.'}</p>
{:else}
	<ul class="links">
		{#each data.links as link (link.id)}
			<li class:read={link.read}>
				<a href={route('/links/[id]', { id: link.id })}>{link.title}</a>
				<span class="host">{new URL(link.url).host}</span>
			</li>
		{/each}
	</ul>
{/if}

<style>
	.top {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 1rem;
		flex-wrap: wrap;
		margin-bottom: 1.5rem;
	}
	h1 {
		margin: 0;
		font-size: 1.75rem;
	}
	.top :global(.find) {
		width: auto;
		flex: 0 1 16rem;
	}
	@media (max-width: 560px) {
		.top :global(.find) {
			flex-basis: 100%;
			max-width: none;
		}
	}
	:global(.add) {
		display: grid;
		grid-template-columns: 2fr 1fr auto;
		gap: 0.75rem;
		align-items: start;
		margin-bottom: 2rem;
	}
	:global(.add) > :global(button) {
		margin-top: 1.5rem;
	}
	@media (max-width: 40rem) {
		:global(.add) {
			grid-template-columns: 1fr;
		}
		:global(.add) > :global(button) {
			margin-top: 0;
		}
	}
	.filters {
		display: flex;
		gap: 0.25rem;
		margin-bottom: 0.5rem;
	}
	.filters a {
		padding: 0.25rem 0.75rem;
		border-radius: 999px;
		color: var(--tern-muted);
		font-size: 0.875rem;
		text-decoration: none;
	}
	.filters a[aria-current='page'] {
		background: var(--tern-surface);
		color: var(--tern-fg);
	}
	.empty {
		color: var(--tern-muted);
	}
	.links {
		margin: 0;
		padding: 0;
		list-style: none;
	}
	.links li {
		display: flex;
		align-items: baseline;
		gap: 0.75rem;
		padding: 0.75rem 0;
		border-bottom: 1px solid var(--tern-border);
	}
	.links a {
		color: var(--tern-fg);
		font-weight: 500;
		text-decoration: none;
	}
	.links a:hover {
		text-decoration: underline;
	}
	.read a {
		color: var(--tern-muted);
	}
	.host {
		color: var(--tern-muted);
		font-size: 0.875rem;
	}
</style>

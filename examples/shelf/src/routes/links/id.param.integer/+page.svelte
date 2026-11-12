<script lang="ts">
	import { enhance } from '@ternkit/tern';
	import { Badge, Button } from '@ternkit/tern/ui';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	const link = $derived(data.link);
</script>

<svelte:head>
	<title>{link.title} · Shelf</title>
</svelte:head>

<p class="back"><a href="/">← Your shelf</a></p>

<h1>{link.title}</h1>
<p class="meta">
	{new URL(link.url).host} · saved {new Date(link.addedAt).toLocaleDateString()}
	{#if link.read}<Badge tone="success">Read</Badge>{/if}
</p>

<div class="actions">
	<Button href={link.url} target="_blank" rel="noopener noreferrer">Open</Button>
	<form method="POST" action="?/read" use:enhance>
		<input type="hidden" name="read" value={link.read ? '' : 'on'} />
		<Button type="submit" variant="secondary">{link.read ? 'Mark as unread' : 'Mark as read'}</Button>
	</form>
	<form method="POST" action="?/delete" use:enhance>
		<Button type="submit" variant="ghost">Delete</Button>
	</form>
</div>

<style>
	.back a {
		color: var(--tern-muted);
		text-decoration: none;
		font-size: 0.875rem;
	}
	h1 {
		margin: 1rem 0 0.25rem;
		font-size: 1.75rem;
		overflow-wrap: anywhere;
	}
	.meta {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		margin: 0 0 1.5rem;
		color: var(--tern-muted);
	}
	.actions {
		display: flex;
		flex-wrap: wrap;
		gap: 0.5rem;
	}
</style>

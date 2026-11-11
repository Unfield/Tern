<script lang="ts">
	import { tb } from '../runtime/i18n.svelte.js';
	import Menu from './Menu.svelte';

	type Crumb = { label: string; href?: string };
	type Props = {
		/** From the top level down to the current page, which is last. */
		items: Crumb[];
		/** With more items than this, the middle ones move into a menu. */
		max?: number;
		class?: string;
	};
	let { items, max = 4, class: klass = '' }: Props = $props();

	const collapsed = $derived(items.length > max);
	const head = $derived(collapsed ? items.slice(0, 1) : items);
	const middle = $derived(collapsed ? items.slice(1, items.length - (max - 2)) : []);
	const tail = $derived(collapsed ? items.slice(items.length - (max - 2)) : []);
</script>

{#snippet crumb(item: Crumb, last: boolean)}
	<li>
		{#if last}
			<span aria-current="page">{item.label}</span>
		{:else if item.href}
			<a href={item.href}>{item.label}</a>
		{:else}
			<span>{item.label}</span>
		{/if}
	</li>
{/snippet}

<nav class="tern-breadcrumb {klass}" aria-label={tb('tern.ui.breadcrumb.label')}>
	<ol>
		{#each head as item, i (i)}{@render crumb(item, !collapsed && i === items.length - 1)}{/each}
		{#if collapsed}
			<li>
				<Menu variant="ghost" items={middle.map((m) => ({ label: m.label, href: m.href }))} class="more">
					<span class="tern-sr-only">Show {middle.length} more</span><span aria-hidden="true">…</span>
				</Menu>
			</li>
			{#each tail as item, i (i)}{@render crumb(item, i === tail.length - 1)}{/each}
		{/if}
	</ol>
</nav>

<style>
	ol {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.25rem;
		margin: 0;
		padding: 0;
		list-style: none;
		font-size: 0.875rem;
		color: var(--tern-muted);
	}
	li {
		display: inline-flex;
		align-items: center;
		gap: 0.25rem;
	}
	li + li::before {
		content: '/';
		margin-inline-end: 0.25rem;
		color: var(--tern-border-strong);
	}
	a {
		color: var(--tern-muted);
		text-decoration: none;
		border-radius: 3px;
	}
	a:hover {
		color: var(--tern-fg);
		text-decoration: underline;
	}
	a:focus-visible {
		outline: 2px solid var(--tern-ring);
		outline-offset: 2px;
	}
	[aria-current='page'] {
		color: var(--tern-fg);
		font-weight: 550;
	}
	li :global(.more .trigger) {
		height: 1.5rem;
		padding: 0 0.4rem;
	}
	li :global(.more .trigger svg) {
		display: none;
	}
</style>

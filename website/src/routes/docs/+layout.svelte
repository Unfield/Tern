<script lang="ts">
	import { page } from '@ternkit/tern';
	import Icon from '$lib/Icon.svelte';
	import type { LayoutProps } from './$types';

	let { data, children }: LayoutProps = $props();
	let open = $state(false);

	const current = $derived(data.nav.flatMap((s) => s.links).find((l) => l.url === page.url.pathname));

	$effect(() => {
		void page.url.pathname;
		open = false;
	});
</script>

<div class="container docs">
	<button class="nav-toggle" type="button" onclick={() => (open = !open)} aria-expanded={open} aria-controls="docs-nav">
		<Icon name={open ? 'x' : 'menu'} />
		<span>{current?.title ?? 'Documentation'}</span>
	</button>
	<aside id="docs-nav" class="sidebar" class:open aria-label="Documentation">
		<nav>
			{#each data.nav as section (section.name)}
				<div class="group">
					<h2>{section.name}</h2>
					<ul>
						{#each section.links as l (l.url)}
							<li><a href={l.url} aria-current={l.url === page.url.pathname ? 'page' : undefined}>{l.title}</a></li>
						{/each}
					</ul>
				</div>
			{/each}
		</nav>
	</aside>
	<div class="main">
		{@render children()}
	</div>
</div>

<style>
	.docs {
		display: grid;
		grid-template-columns: 248px minmax(0, 1fr);
		gap: var(--s-12);
	}
	.sidebar {
		position: sticky;
		top: var(--header);
		height: calc(100vh - var(--header));
		padding: var(--s-10) var(--s-2) var(--s-16) 0;
		overflow-y: auto;
		scrollbar-width: thin;
		scrollbar-color: transparent transparent;
	}
	.sidebar:hover {
		scrollbar-color: var(--border-strong) transparent;
	}
	.group + .group {
		margin-top: var(--s-8);
	}
	h2 {
		margin: 0 0 var(--s-2) var(--s-3);
		color: var(--text);
		font-size: 13px;
		/* whole pixels in a sticky box; see .container in global.css */
		line-height: 16px;
		font-weight: 600;
		letter-spacing: 0;
	}
	ul {
		margin: 0;
		padding: 0;
		list-style: none;
	}
	li a {
		display: block;
		padding: 6px var(--s-3);
		border-radius: var(--radius-s);
		color: var(--text-3);
		font-size: 14px;
		line-height: 20px;
		transition: color 140ms, background 140ms;
	}
	li a:hover {
		color: var(--text);
		background: var(--surface-2);
	}
	li a[aria-current='page'] {
		color: var(--accent);
		background: var(--accent-soft);
		font-weight: 550;
	}
	.main {
		min-width: 0;
		padding: var(--s-10) 0 var(--s-24);
	}
	.nav-toggle {
		display: none;
	}
	@media (max-width: 900px) {
		.docs {
			display: block;
		}
		.nav-toggle {
			position: sticky;
			top: var(--header);
			z-index: 20;
			display: flex;
			align-items: center;
			gap: var(--s-3);
			width: calc(100% + 2 * var(--gutter));
			/* 48px inside the 1px border, so its content centers on a whole pixel */
			height: 49px;
			margin: 0 calc(-1 * var(--gutter));
			padding: 0 var(--gutter);
			border: 0;
			border-bottom: 1px solid var(--border);
			background: color-mix(in srgb, var(--bg) 92%, transparent);
			backdrop-filter: blur(12px);
			color: var(--text-2);
			font-size: 14px;
			font-weight: 550;
			/* whole pixels in a sticky, blurred bar; see Header.svelte */
			line-height: 20px;
			cursor: pointer;
		}
		.sidebar {
			display: none;
			position: static;
			height: auto;
			padding: var(--s-6) 0;
			border-bottom: 1px solid var(--border);
		}
		.sidebar.open {
			display: block;
		}
		.main {
			padding-top: var(--s-8);
		}
	}
</style>

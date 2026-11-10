<script lang="ts">
	import '@ternkit/tern/ui/theme.css';
	import { page, navigating } from '@ternkit/tern';
	import { Spinner, Toaster } from '@ternkit/tern/ui';
	import type { LayoutProps } from './$types';

	let { children }: LayoutProps = $props();
	const links = [
		{ href: '/', label: 'Home' },
		{ href: '/about', label: 'About' }
	];
</script>

<div class="tern shell">
	<header>
		<nav aria-label="Main">
			{#each links as link (link.href)}
				<a href={link.href} aria-current={page.url.pathname === link.href ? 'page' : undefined}>{link.label}</a>
			{/each}
		</nav>
		{#if navigating.current}<Spinner size="1rem" label="Loading" />{/if}
	</header>

	<main>
		{@render children()}
	</main>
</div>

<Toaster />

<style>
	:global(body) {
		margin: 0;
		background: var(--tern-bg);
	}
	.shell {
		min-height: 100vh;
	}
	header {
		display: flex;
		align-items: center;
		gap: 1rem;
		padding: 0.875rem 1.5rem;
		border-bottom: 1px solid var(--tern-border);
		background: var(--tern-surface);
	}
	nav {
		display: flex;
		gap: 1.25rem;
	}
	a {
		color: var(--tern-muted);
		text-decoration: none;
		font-weight: 500;
	}
	a:hover,
	a[aria-current='page'] {
		color: var(--tern-fg);
	}
	a:focus-visible {
		outline: 2px solid var(--tern-ring);
		outline-offset: 2px;
		border-radius: 2px;
	}
	main {
		max-width: 40rem;
		margin: 2.5rem auto;
		padding: 0 1.5rem;
	}
</style>

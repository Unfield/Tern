<script lang="ts">
	import '@ternkit/tern/ui/theme.css';
	import { page } from '@ternkit/tern';
	import { Button, Toaster } from '@ternkit/tern/ui';
	import type { LayoutProps } from './$types';

	let { children }: LayoutProps = $props();
	let dark = $state(false);
	$effect(() => {
		document.documentElement.dataset.theme = dark ? 'dark' : 'light';
	});
</script>

<a class="skip" href="#main">Skip to content</a>
<header>
	<a class="brand" href="/">Tern UI</a>
	<nav aria-label="Main">
		<a href="/" aria-current={page.url.pathname === '/' ? 'page' : undefined}>Components</a>
		<a href="/form" aria-current={page.url.pathname === '/form' ? 'page' : undefined}>Form</a>
	</nav>
	<Button variant="ghost" size="sm" onclick={() => (dark = !dark)} aria-pressed={dark} data-testid="theme">{dark ? 'Light' : 'Dark'} mode</Button>
</header>

<main id="main" tabindex="-1">
	{@render children()}
</main>

<Toaster />

<style>
	:global(body) {
		margin: 0;
		font-family: var(--tern-font);
		color: var(--tern-fg);
		background: var(--tern-bg);
		line-height: 1.5;
	}
	header {
		display: flex;
		align-items: center;
		gap: 1.5rem;
		padding: 0.75rem 1.5rem;
		border-bottom: 1px solid var(--tern-border);
		background: var(--tern-surface);
	}
	.brand {
		font-weight: 700;
		letter-spacing: -0.02em;
		color: var(--tern-fg);
		text-decoration: none;
	}
	nav {
		display: flex;
		gap: 1rem;
		flex: 1;
	}
	nav a {
		color: var(--tern-muted);
		text-decoration: none;
		font-size: 0.875rem;
		font-weight: 550;
	}
	nav a[aria-current='page'] {
		color: var(--tern-fg);
	}
	nav a:focus-visible,
	.brand:focus-visible {
		outline: 2px solid var(--tern-ring);
		outline-offset: 2px;
		border-radius: 4px;
	}
	main {
		max-width: 56rem;
		margin: 2rem auto;
		padding: 0 1.5rem 4rem;
	}
	main:focus {
		outline: none;
	}
	.skip {
		position: absolute;
		left: 1rem;
		top: -3rem;
		padding: 0.5rem 0.75rem;
		background: var(--tern-accent);
		color: var(--tern-accent-fg);
		border-radius: var(--tern-radius-sm);
		z-index: 200;
	}
	.skip:focus {
		top: 0.5rem;
	}
</style>

<script lang="ts">
	import { page, navigating } from '@unfield/tern';
	import type { LayoutProps } from './$types';

	let { children }: LayoutProps = $props();
</script>

<header>
	<nav>
		<a href="/" aria-current={page.url.pathname === '/' ? 'page' : undefined}>Home</a>
		<a href="/about" aria-current={page.url.pathname === '/about' ? 'page' : undefined}>About</a>
	</nav>
	{#if navigating.current}<span class="loading" aria-hidden="true"></span>{/if}
</header>

<main>
	{@render children()}
</main>

<style>
	:global(body) {
		margin: 0;
		font-family: system-ui, sans-serif;
		line-height: 1.6;
		color: #1c1917;
		background: #fafaf9;
	}
	header {
		display: flex;
		align-items: center;
		gap: 1rem;
		padding: 1rem 1.5rem;
		border-bottom: 1px solid #e7e5e4;
	}
	nav {
		display: flex;
		gap: 1rem;
	}
	a {
		color: #0f766e;
	}
	a[aria-current='page'] {
		font-weight: 600;
	}
	main {
		max-width: 40rem;
		margin: 2rem auto;
		padding: 0 1.5rem;
	}
	.loading {
		width: 0.75rem;
		height: 0.75rem;
		border-radius: 50%;
		background: #0f766e;
		animation: pulse 1s infinite;
	}
	@keyframes pulse {
		50% {
			opacity: 0.3;
		}
	}
</style>

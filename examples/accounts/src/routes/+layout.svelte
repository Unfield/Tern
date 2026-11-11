<script lang="ts">
	import '@ternkit/tern/ui/theme.css';
	import { Button } from '@ternkit/tern/ui';
	import type { LayoutProps } from './$types';

	let { data, children }: LayoutProps = $props();
</script>

<header>
	<a class="brand" href="/">Accounts</a>
	<nav aria-label="Main">
		{#if data.user}
			<a href="/account">{data.user.name || data.user.email}</a>
			<form method="POST" action="/logout"><Button type="submit" variant="ghost" size="sm">Sign out</Button></form>
		{:else}
			<a href="/login">Sign in</a>
			<a href="/signup">Create an account</a>
		{/if}
		<a href="/outbox">Outbox</a>
	</nav>
</header>

<main id="main">
	{@render children()}
</main>

<style>
	:global(body) {
		margin: 0;
		font-family: var(--tern-font);
		color: var(--tern-fg);
		background: var(--tern-bg);
	}
	header {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: space-between;
		gap: 1rem;
		padding: 0.75rem 1rem;
		border-bottom: 1px solid var(--tern-border);
	}
	nav {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 1rem;
		font-size: 0.875rem;
	}
	nav form {
		display: contents;
	}
	a {
		color: var(--tern-fg);
	}
	.brand {
		font-weight: 650;
		text-decoration: none;
	}
	main {
		max-width: 42rem;
		margin: 0 auto;
		padding: 2rem 1rem;
	}
	main :global(.tern-auth) {
		margin: 0 auto;
	}
</style>

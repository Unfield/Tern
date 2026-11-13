<script lang="ts">
	import '@ternkit/tern/ui/theme.css';
	import { navigating } from '@ternkit/tern';
	import { Button, Spinner, Toaster } from '@ternkit/tern/ui';
	import type { LayoutProps } from './$types';

	let { data, children }: LayoutProps = $props();
</script>

<div class="tern shell">
	<header>
		<a class="brand" href="/">Shelf</a>
		{#if navigating.current}<Spinner size="1rem" label="Loading" />{/if}
		<nav aria-label="Account">
			{#if data.user}
				{#if data.admin}<a href="/admin">Admin</a>{/if}
				<span class="who">{data.user.name}</span>
				<form method="POST" action="/logout">
					<Button type="submit" variant="ghost" size="sm">Sign out</Button>
				</form>
			{:else}
				<a href="/login">Sign in</a>
			{/if}
		</nav>
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
		padding: 0.75rem 1.5rem;
		border-bottom: 1px solid var(--tern-border);
		background: var(--tern-surface);
	}
	.brand {
		color: var(--tern-fg);
		font-weight: 650;
		text-decoration: none;
	}
	nav {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		margin-left: auto;
		font-size: 0.875rem;
	}
	nav a {
		color: var(--tern-fg);
	}
	.who {
		color: var(--tern-muted);
	}
	main {
		max-width: 40rem;
		margin: 2.5rem auto;
		padding: 0 1.5rem;
	}
	/* The admin screens' tables need the room. */
	main:has(:global(.tern-admin)) {
		max-width: 72rem;
	}
	main :global(.tern-auth) {
		margin: 0 auto;
	}
</style>

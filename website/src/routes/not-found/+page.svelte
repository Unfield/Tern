<script lang="ts">
	import Icon from '$lib/Icon.svelte';
</script>

<svelte:head>
	<title>Page not found · Tern</title>
	<meta name="robots" content="noindex" />
</svelte:head>

<section class="container err">
	<p class="code">404</p>
	<h1>This page doesn’t exist</h1>
	<p class="sub">It may have moved when the docs were reorganized. Search for it with <kbd>⌘K</kbd>, or start from the docs.</p>
	<div class="actions">
		<a class="btn btn-primary" href="/docs">Go to the docs <Icon name="arrow-right" /></a>
		<a class="btn" href="/">Home</a>
	</div>
</section>

<style>
	.err {
		display: flex;
		flex-direction: column;
		align-items: center;
		padding-top: var(--s-32);
		padding-bottom: var(--s-32);
		text-align: center;
	}
	.code {
		color: var(--accent);
		font-family: var(--mono);
		font-size: 15px;
		font-weight: 600;
	}
	h1 {
		margin-top: var(--s-4);
		font-size: clamp(32px, 5vw, 48px);
		font-weight: 700;
		letter-spacing: -0.035em;
	}
	.sub {
		max-width: 520px;
		margin-top: var(--s-4);
		color: var(--text-2);
		font-size: 17px;
	}
	kbd {
		padding: 1px 6px;
		border: 1px solid var(--border);
		border-radius: 5px;
		background: var(--surface-2);
		font-family: var(--font);
		font-size: 14px;
	}
	.actions {
		display: flex;
		flex-wrap: wrap;
		justify-content: center;
		gap: var(--s-3);
		margin-top: var(--s-10);
	}
</style>

<script lang="ts">
	import { page } from '@ternkit/tern';
	import Icon from '$lib/Icon.svelte';
</script>

<svelte:head>
	<title>{page.status === 404 ? 'Page not found' : 'Something went wrong'} · Tern</title>
</svelte:head>

<section class="container err">
	<p class="code">{page.status}</p>
	<h1>{page.status === 404 ? 'This page doesn’t exist' : 'Something went wrong'}</h1>
	<p class="sub">{page.status === 404 ? 'It may have moved when the docs were reorganized. Search for it, or start from the docs.' : page.error?.message}</p>
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
	.actions {
		display: flex;
		flex-wrap: wrap;
		justify-content: center;
		gap: var(--s-3);
		margin-top: var(--s-10);
	}
</style>

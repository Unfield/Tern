<script lang="ts">
	import Icon from '$lib/Icon.svelte';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	const entry = `{
  "name": "stripe-webhooks",
  "description": "Verifies Stripe's webhooks and calls a Go function for each event.",
  "import": "github.com/you/tern-stripe",
  "module": "github.com/you/tern-stripe",
  "add": "stripe.New(os.Getenv(\\"STRIPE_WEBHOOK_SECRET\\"))",
  "imports": ["os"],
  "env": ["STRIPE_WEBHOOK_SECRET"],
  "docs": "https://github.com/you/tern-stripe#readme",
  "author": "Your name",
  "url": "https://github.com/you/tern-stripe"
}`;
</script>

<svelte:head>
	<title>Plugins · Tern</title>
	<meta name="description" content="Plugins for Tern apps: compression, CORS, health checks, metrics, rate limits, sessions and more, added with one command." />
</svelte:head>

<section class="container page">
	<header>
		<p class="eyebrow">Plugins</p>
		<h1>Add what your app needs</h1>
		<p class="sub">A plugin adds hooks, middleware, endpoints or tags to every page. <code>tern add</code> puts one in your app’s <code>main.go</code>, and gets its module when it isn’t Tern’s own.</p>
		<pre class="cmd"><code>go tool tern add compress health</code></pre>
	</header>

	<h2 class="section">Tern’s own</h2>
	<div class="grid">
		{#each data.official as p (p.name)}
			{@render card(p)}
		{/each}
	</div>

	<h2 class="section">From the community</h2>
	{#if data.community.length}
		<div class="grid">
			{#each data.community as p (p.name)}
				{@render card(p)}
			{/each}
		</div>
	{:else}
		<p class="empty">None listed yet. Yours could be the first: see below.</p>
	{/if}

	<aside class="list">
		<h2>List yours</h2>
		<p>Any Go package with a function that returns a <code>kit.Plugin</code> works with <code>tern add</code>, listed or not:</p>
		<pre><code>go tool tern add github.com/you/tern-thing</code></pre>
		<p>It looks for a function that makes the plugin without arguments, such as <code>New(opts ...Option)</code>. To list it here, and let people add it by name, add an entry to <a href={data.edit}>plugins/registry.json</a> in a pull request:</p>
		<pre><code>{entry}</code></pre>
		<p><code>add</code> is the Go expression <code>tern add</code> puts in <code>kit.WithPlugins</code>; leave it out, and write <code>notes</code>, for a plugin that needs setting up first. A test checks that every entry compiles.</p>
	</aside>
</section>

{#snippet card(p: (typeof data.official)[number])}
	<article class="card">
		<div class="top">
			<h3>{p.name}</h3>
			{#if p.author !== 'Tern'}<span class="by">by {p.author}</span>{/if}
		</div>
		<p>{p.description}</p>
		{#if p.add}
			<code class="add">go tool tern add {p.name}</code>
		{:else}
			<p class="note">{p.notes}</p>
		{/if}
		{#if p.env.length}
			<p class="env">Reads {#each p.env as e, i (e)}{i ? ', ' : ''}<code>{e}</code>{/each}</p>
		{/if}
		<div class="links">
			<a href={p.docs}>Docs</a>
			{#if p.url}<a href={p.url}>Source <Icon name="external" size={13} /></a>{/if}
			<span class="import">{p.import}</span>
		</div>
	</article>
{/snippet}

<style>
	.page {
		padding-top: var(--s-20);
		padding-bottom: var(--s-24);
	}
	header {
		max-width: 680px;
	}
	h1 {
		margin-top: var(--s-4);
		font-size: clamp(36px, 5vw, 52px);
		font-weight: 700;
		letter-spacing: -0.04em;
		line-height: 1.06;
	}
	.sub {
		margin-top: var(--s-5);
		color: var(--text-2);
		font-size: 18px;
	}
	pre {
		margin: var(--s-5) 0 0;
		padding: var(--s-4) var(--s-5);
		border: 1px solid var(--border);
		border-radius: var(--radius);
		background: var(--code-bg);
		color: var(--text);
		font-size: 13.5px;
		line-height: 1.7;
		overflow-x: auto;
	}
	.section {
		margin-top: var(--s-16);
		font-size: 24px;
	}
	.grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
		gap: var(--s-5);
		margin-top: var(--s-6);
	}
	.card {
		display: flex;
		flex-direction: column;
		padding: var(--s-6);
		border: 1px solid var(--border);
		border-radius: var(--radius-l);
		background: var(--surface);
	}
	.top {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		gap: var(--s-3);
	}
	h3 {
		font-size: 17px;
		font-weight: 640;
	}
	.by {
		color: var(--text-3);
		font-size: 13px;
	}
	.card > p {
		flex: 1;
		margin-top: var(--s-2);
		color: var(--text-2);
		font-size: 14.5px;
		line-height: 1.6;
	}
	.card .note {
		flex: none;
		color: var(--text-3);
		font-size: 13.5px;
	}
	.add {
		display: block;
		margin-top: var(--s-4);
		padding: var(--s-2) var(--s-3);
		background: var(--code-bg);
		font-size: 13px;
		overflow-x: auto;
		white-space: nowrap;
		border-radius: var(--radius-s);
	}
	.card .env {
		flex: none;
		margin-top: var(--s-3);
		color: var(--text-3);
		font-size: 13px;
	}
	.links {
		display: flex;
		align-items: center;
		flex-wrap: wrap;
		gap: var(--s-2) var(--s-4);
		margin-top: var(--s-4);
		font-size: 13.5px;
	}
	.links a {
		display: inline-flex;
		align-items: center;
		gap: 4px;
	}
	.import {
		width: 100%;
		color: var(--text-3);
		font-family: var(--mono);
		font-size: 12px;
		overflow-wrap: anywhere;
	}
	.empty {
		margin-top: var(--s-4);
		color: var(--text-2);
	}
	.list {
		max-width: 720px;
		margin-top: var(--s-20);
	}
	.list h2 {
		font-size: 24px;
	}
	.list p {
		margin-top: var(--s-4);
		color: var(--text-2);
	}
	@media (max-width: 640px) {
		.page {
			padding-top: var(--s-12);
			padding-bottom: var(--s-16);
		}
		.grid {
			grid-template-columns: 1fr;
		}
	}
</style>

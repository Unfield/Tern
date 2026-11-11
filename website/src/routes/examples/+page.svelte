<script lang="ts">
	import Icon from '$lib/Icon.svelte';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
</script>

<svelte:head>
	<title>Examples · Tern</title>
	<meta name="description" content="Complete apps built with Tern: a status page, web analytics, accounts, a dashboard and more, with their source." />
</svelte:head>

<section class="container page">
	<header>
		<p class="eyebrow">Examples</p>
		<h1>Apps built with Tern</h1>
		<p class="sub">Complete apps in the repository, each a few commands from running on your machine. Read them to see how the pieces fit, or start from one.</p>
	</header>

	<div class="grid">
		{#each data.examples as e (e.dir)}
			<a class="card" href={e.url}>
				<div class="top">
					<h2>{e.name}</h2>
					<Icon name="external" size={16} />
				</div>
				<p>{e.about}</p>
				<ul class="tags" aria-label="Uses">
					{#each e.uses as u (u)}<li>{u}</li>{/each}
				</ul>
				<code>examples/{e.dir}</code>
			</a>
		{/each}
	</div>

	<aside class="run">
		<h2>Run one</h2>
		<p>Clone the repository, install, and start the dev server in an example’s folder:</p>
		<pre><code>git clone https://github.com/Unfield/Tern && cd Tern
pnpm install
cd examples/status && go tool tern dev</code></pre>
	</aside>
</section>

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
	.grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
		gap: var(--s-6);
		margin-top: var(--s-16);
	}
	.card {
		display: flex;
		flex-direction: column;
		padding: var(--s-8);
		border: 1px solid var(--border);
		border-radius: var(--radius-l);
		background: var(--surface);
		color: var(--text);
		transition:
			border-color 180ms var(--ease),
			transform 180ms var(--ease),
			box-shadow 180ms var(--ease);
	}
	.card:hover {
		color: var(--text);
		border-color: var(--accent-line);
		box-shadow: var(--shadow);
		transform: translateY(-2px);
	}
	.top {
		display: flex;
		align-items: center;
		justify-content: space-between;
		color: var(--text-3);
	}
	h2 {
		color: var(--text);
		font-size: 19px;
		font-weight: 640;
	}
	.card p {
		flex: 1;
		margin-top: var(--s-3);
		color: var(--text-2);
		font-size: 14.5px;
		line-height: 1.6;
	}
	.tags {
		display: flex;
		flex-wrap: wrap;
		gap: var(--s-2);
		margin: var(--s-6) 0 0;
		padding: 0;
		list-style: none;
	}
	.tags li {
		padding: 2px var(--s-2);
		border: 1px solid var(--border);
		border-radius: 999px;
		color: var(--text-2);
		font-size: 12.5px;
	}
	.card code {
		margin-top: var(--s-6);
		color: var(--text-3);
		font-size: 12.5px;
	}
	.run {
		max-width: 680px;
		margin-top: var(--s-20);
	}
	.run h2 {
		font-size: 24px;
	}
	.run p {
		margin-top: var(--s-3);
		color: var(--text-2);
	}
	.run pre {
		margin: var(--s-5) 0 0;
		padding: var(--s-5) var(--s-6);
		border: 1px solid var(--border);
		border-radius: var(--radius);
		background: var(--code-bg);
		color: var(--text);
		font-size: 13.5px;
		line-height: 1.75;
		overflow-x: auto;
	}
	@media (max-width: 640px) {
		.page {
			padding-top: var(--s-12);
			padding-bottom: var(--s-16);
		}
		.grid {
			grid-template-columns: 1fr;
			margin-top: var(--s-10);
		}
		.card {
			padding: var(--s-6);
		}
	}
</style>

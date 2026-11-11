<script lang="ts">
	import Logo from './Logo.svelte';

	let { repo, version }: { repo: string; version: string } = $props();

	const columns = [
		{
			title: 'Docs',
			links: [
				{ href: '/docs', label: 'Introduction' },
				{ href: '/docs/guide', label: 'Guide' },
				{ href: '/docs/accounts', label: 'Accounts' },
				{ href: '/docs/deployment', label: 'Deployment' }
			]
		},
		{
			title: 'Learn',
			links: [
				{ href: '/examples', label: 'Examples' },
				{ href: '/docs/migrating-from-sveltekit', label: 'Coming from SvelteKit' },
				{ href: '/docs/benchmarks', label: 'Benchmarks' },
				{ href: '/docs/architecture', label: 'Architecture' }
			]
		},
		{
			title: 'Project',
			links: [
				{ href: '/docs/changelog', label: 'Changelog' },
				{ href: '/docs/roadmap', label: 'Roadmap' },
				{ href: '/docs/decisions', label: 'Decision records' },
				{ href: '/docs/contributing', label: 'Contributing' }
			]
		}
	];
</script>

<footer>
	<div class="container grid">
		<div class="about">
			<a class="brand" href="/"><Logo size={24} /><span>Tern</span></a>
			<p>Svelte on the client, Go on the server. MIT licensed.</p>
			<p class="small">{version ? `Version ${version}. ` : ''}Built with Tern, and prerendered.</p>
		</div>
		{#each columns as col (col.title)}
			<nav aria-label={col.title}>
				<h2>{col.title}</h2>
				<ul>
					{#each col.links as l (l.href)}
						<li><a href={l.href}>{l.label}</a></li>
					{/each}
				</ul>
			</nav>
		{/each}
		<nav aria-label="Elsewhere">
			<h2>Elsewhere</h2>
			<ul>
				<li><a href={repo}>GitHub</a></li>
				<li><a href={repo + '/issues'}>Issues</a></li>
				<li><a href="https://www.npmjs.com/package/@ternkit/tern">npm</a></li>
				<li><a href="https://pkg.go.dev/github.com/Unfield/Tern">Go reference</a></li>
			</ul>
		</nav>
	</div>
</footer>

<style>
	footer {
		border-top: 1px solid var(--border);
		background: var(--bg-2);
		padding: var(--s-16) 0 var(--s-20);
	}
	.grid {
		display: grid;
		grid-template-columns: 2fr repeat(4, 1fr);
		gap: var(--s-10);
	}
	.brand {
		display: inline-flex;
		align-items: center;
		gap: var(--s-2);
		color: var(--text);
		font-weight: 700;
		font-size: 17px;
		letter-spacing: -0.02em;
	}
	.about p {
		margin-top: var(--s-4);
		max-width: 280px;
		color: var(--text-2);
		font-size: 14px;
	}
	.about .small {
		margin-top: var(--s-2);
		color: var(--text-3);
		font-size: 13px;
	}
	h2 {
		margin-bottom: var(--s-4);
		color: var(--text);
		font-size: 13px;
		font-weight: 600;
		letter-spacing: 0;
	}
	ul {
		display: flex;
		flex-direction: column;
		gap: var(--s-3);
		margin: 0;
		padding: 0;
		list-style: none;
	}
	ul a {
		color: var(--text-3);
		font-size: 14px;
		transition: color 160ms;
	}
	ul a:hover {
		color: var(--text);
	}
	@media (max-width: 900px) {
		.grid {
			grid-template-columns: repeat(2, 1fr);
			gap: var(--s-10) var(--s-6);
		}
		.about {
			grid-column: 1 / -1;
		}
	}
</style>

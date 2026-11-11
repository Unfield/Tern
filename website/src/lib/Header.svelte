<script lang="ts">
	import { page } from '@ternkit/tern';
	import Icon from './Icon.svelte';
	import Logo from './Logo.svelte';
	import Search from './Search.svelte';
	import { theme } from './theme.svelte';

	let { version, repo }: { version: string; repo: string } = $props();

	const links = [
		{ href: '/docs', label: 'Docs', match: (p: string) => p.startsWith('/docs') && !p.startsWith('/docs/benchmarks') && !p.startsWith('/docs/changelog') },
		{ href: '/examples', label: 'Examples', match: (p: string) => p.startsWith('/examples') },
		{ href: '/docs/benchmarks', label: 'Benchmarks', match: (p: string) => p.startsWith('/docs/benchmarks') },
		{ href: '/docs/changelog', label: 'Changelog', match: (p: string) => p.startsWith('/docs/changelog') }
	];

	let menu = $state(false);
	let search = $state<Search>();
	let scrolled = $state(false);

	$effect(() => {
		theme.init();
		const onScroll = () => (scrolled = window.scrollY > 4);
		onScroll();
		window.addEventListener('scroll', onScroll, { passive: true });
		return () => window.removeEventListener('scroll', onScroll);
	});

	$effect(() => {
		void page.url.pathname;
		menu = false;
	});
</script>

<header class:scrolled>
	<div class="bar container">
		<a class="brand" href="/" aria-label="Tern, home">
			<Logo size={28} />
			<span class="word">Tern</span>
		</a>
		{#if version}
			<a class="version" href="/docs/changelog" title="What's new in Tern {version}">v{version}</a>
		{/if}

		<nav class="links" aria-label="Main">
			{#each links as l (l.href)}
				<a href={l.href} aria-current={l.match(page.url.pathname) ? 'page' : undefined}>{l.label}</a>
			{/each}
		</nav>

		<div class="tools">
			<button class="search-btn" type="button" onclick={() => search?.show()} aria-label="Search the docs">
				<Icon name="search" />
				<span class="search-label">Search docs</span>
				<kbd>⌘K</kbd>
			</button>
			<!-- Both icons are in the HTML, and the theme on <html> picks one:
			     the server doesn't know the reader's theme, and a swap after
			     the script runs would flash. -->
			<button class="icon-btn theme-btn" type="button" onclick={() => theme.toggle()} aria-label="Switch between light and dark">
				<span class="when-dark"><Icon name="sun" size={18} /></span>
				<span class="when-light"><Icon name="moon" size={18} /></span>
			</button>
			<a class="icon-btn" href={repo} aria-label="Tern on GitHub"><Icon name="github" size={18} /></a>
			<button class="icon-btn menu-btn" type="button" onclick={() => (menu = !menu)} aria-expanded={menu} aria-controls="mobile-menu" aria-label="Menu">
				<Icon name={menu ? 'x' : 'menu'} size={20} />
			</button>
		</div>
	</div>

	{#if menu}
		<nav id="mobile-menu" class="mobile" aria-label="Main">
			<div class="container">
				{#each links as l (l.href)}
					<a href={l.href} aria-current={l.match(page.url.pathname) ? 'page' : undefined}>{l.label}</a>
				{/each}
				<a href={repo}>GitHub</a>
			</div>
		</nav>
	{/if}
</header>

<Search bind:this={search} />

<style>
	/* Everything in the bar has a whole-pixel height and line-height: in a
	   sticky, blurred header, boxes at fractional offsets are rounded
	   differently from one scroll frame to the next and twitch. */
	header {
		position: sticky;
		top: 0;
		z-index: 50;
		background: color-mix(in srgb, var(--bg) 80%, transparent);
		backdrop-filter: saturate(180%) blur(12px);
		-webkit-backdrop-filter: saturate(180%) blur(12px);
		border-bottom: 1px solid transparent;
		transition: border-color 200ms var(--ease);
	}
	header.scrolled {
		border-bottom-color: var(--border);
	}
	.bar {
		display: flex;
		align-items: center;
		gap: var(--s-4);
		height: var(--header);
	}
	.brand {
		display: flex;
		align-items: center;
		gap: var(--s-3);
		height: 32px;
		color: var(--text);
	}
	.brand:hover {
		color: var(--text);
	}
	.word {
		font-size: 19px;
		line-height: 32px;
		font-weight: 700;
		letter-spacing: -0.03em;
	}
	.version {
		display: inline-flex;
		align-items: center;
		height: 24px;
		padding: 0 var(--s-2);
		line-height: 22px;
		border: 1px solid var(--border);
		border-radius: 999px;
		color: var(--text-3);
		font-size: 12px;
		font-weight: 550;
		font-variant-numeric: tabular-nums;
		transition: color 160ms, border-color 160ms;
	}
	.version:hover {
		color: var(--text);
		border-color: var(--border-strong);
	}
	.links {
		display: flex;
		gap: var(--s-1);
		margin-left: var(--s-4);
	}
	.links a {
		display: inline-flex;
		align-items: center;
		height: 36px;
		padding: 0 var(--s-3);
		line-height: 20px;
		border-radius: var(--radius-s);
		color: var(--text-2);
		font-size: 14.5px;
		font-weight: 500;
		transition: color 160ms, background 160ms;
	}
	.links a:hover {
		color: var(--text);
		background: var(--surface-2);
	}
	.links a[aria-current='page'] {
		color: var(--text);
	}
	.tools {
		display: flex;
		align-items: center;
		gap: var(--s-2);
		margin-left: auto;
	}
	.search-btn {
		display: flex;
		align-items: center;
		gap: var(--s-2);
		height: 36px;
		width: 220px;
		padding: 0 var(--s-2) 0 var(--s-3);
		border: 1px solid var(--border);
		border-radius: var(--radius);
		background: var(--surface-2);
		color: var(--text-3);
		font-size: 14px;
		cursor: pointer;
		transition: border-color 160ms, color 160ms;
	}
	.search-btn:hover {
		border-color: var(--border-strong);
		color: var(--text-2);
	}
	.search-label {
		flex: 1;
		line-height: 20px;
		text-align: left;
	}
	kbd {
		display: inline-flex;
		align-items: center;
		height: 20px;
		padding: 0 6px;
		line-height: 18px;
		border: 1px solid var(--border);
		border-radius: 5px;
		background: var(--bg);
		color: var(--text-3);
		font-size: 11.5px;
		font-family: var(--font);
	}
	.icon-btn {
		display: grid;
		place-items: center;
		width: 36px;
		height: 36px;
		border: 0;
		border-radius: var(--radius-s);
		background: transparent;
		color: var(--text-2);
		cursor: pointer;
		transition: color 160ms, background 160ms;
	}
	.icon-btn:hover {
		color: var(--text);
		background: var(--surface-2);
	}
	.menu-btn {
		display: none;
	}
	.theme-btn span {
		display: contents;
	}
	:global(:root[data-theme='dark']) .when-light,
	:global(:root[data-theme='light']) .when-dark {
		display: none;
	}
	.mobile {
		border-top: 1px solid var(--border);
		background: var(--bg);
		padding: var(--s-2) 0 var(--s-4);
	}
	.mobile .container {
		display: flex;
		flex-direction: column;
	}
	.mobile a {
		padding: var(--s-3) 0;
		border-bottom: 1px solid var(--border);
		color: var(--text);
		font-size: 17px;
		font-weight: 500;
	}
	.mobile a:last-child {
		border-bottom: 0;
	}
	.mobile a[aria-current='page'] {
		color: var(--accent);
	}
	@media (max-width: 960px) {
		.search-btn {
			width: 36px;
			padding: 0;
			justify-content: center;
			background: transparent;
			border-color: transparent;
		}
		.search-label,
		kbd {
			display: none;
		}
	}
	@media (max-width: 760px) {
		.links {
			display: none;
		}
		.menu-btn {
			display: grid;
		}
	}
	@media (max-width: 400px) {
		.version {
			display: none;
		}
	}
</style>

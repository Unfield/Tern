<script lang="ts">
	import '@fontsource-variable/inter';
	import '@fontsource-variable/jetbrains-mono';
	import '$lib/global.css';
	import '$lib/code.css';
	import { navigating, page } from '@ternkit/tern';
	import Header from '$lib/Header.svelte';
	import Footer from '$lib/Footer.svelte';
	import type { LayoutProps } from './$types';

	let { data, children }: LayoutProps = $props();
	// The page's address on tern.land, without a trailing slash.
	const canonical = $derived('https://tern.land' + (page.url.pathname.replace(/\/+$/, '') || '/'));
</script>

<svelte:head>
	<meta name="theme-color" content="#09090b" media="(prefers-color-scheme: dark)" />
	<meta name="theme-color" content="#ffffff" media="(prefers-color-scheme: light)" />
	<meta property="og:site_name" content="Tern" />
	<meta property="og:type" content="website" />
	<meta name="twitter:card" content="summary" />
	<link rel="canonical" href={canonical} />
	<meta property="og:url" content={canonical} />
</svelte:head>

<a class="skip" href="#main">Skip to content</a>
{#if navigating.current}<div class="progress" aria-hidden="true"></div>{/if}
<Header version={data.version} repo={data.repo} />
<div id="main">
	{@render children()}
</div>
<Footer repo={data.repo} version={data.version} />

<style>
	.skip {
		position: absolute;
		left: var(--s-4);
		top: -60px;
		z-index: 100;
		padding: var(--s-2) var(--s-4);
		border-radius: var(--radius);
		background: var(--text);
		color: var(--bg);
		font-weight: 600;
	}
	.skip:focus {
		top: var(--s-3);
	}
	.progress {
		position: fixed;
		inset: 0 0 auto;
		z-index: 60;
		height: 2px;
		background: linear-gradient(90deg, transparent, var(--brand), transparent);
		background-size: 50% 100%;
		animation: slide 900ms linear infinite;
	}
	@keyframes slide {
		from {
			background-position: -50% 0;
		}
		to {
			background-position: 150% 0;
		}
	}
</style>

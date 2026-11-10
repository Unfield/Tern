<script lang="ts">
	import type { Snippet } from 'svelte';
	let { title, children }: { title: string; children: Snippet } = $props();
	const links = [
		{ href: '/', label: 'Home' },
		{ href: '/items', label: 'Items' },
		{ href: '/about', label: 'About' }
	];
</script>

<header>
	<a class="brand" href="/">Bench</a>
	<nav aria-label="Main">
		{#each links as link (link.href)}<a href={link.href}>{link.label}</a>{/each}
	</nav>
</header>
<main>
	<h1>{title}</h1>
	{@render children()}
</main>
<footer><p>The same components, rendered by two frameworks.</p></footer>

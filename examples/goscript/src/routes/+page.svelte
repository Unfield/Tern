<script lang="ts">
	import Counter from '$lib/Counter.svelte';
	import Primes from '$lib/Primes.svelte';
	import PrimesJS from '$lib/PrimesJS.svelte';
</script>

<svelte:head><title>Go in Svelte components</title></svelte:head>

<h1>&lt;script lang="go"&gt;</h1>
<p>These components' logic is Go, compiled to WebAssembly, with runes: <code>State</code>, <code>Derived</code> and <code>Effect</code>. The markup is ordinary Svelte.</p>

<Counter start={10} />

<h2>Go and TypeScript, side by side</h2>
<p>The same prime sieve, in a Go component and in a TypeScript component.</p>
<div class="side">
	<Primes />
	<PrimesJS />
</div>

<style>
	.side {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 1rem;
	}
</style>

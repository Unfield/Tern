<script lang="ts">
	import Counter from '$lib/Counter.svelte';
	import LiveRoom from '$lib/LiveRoom.svelte';
	import LiveTodos from '$lib/LiveTodos.svelte';
	import Primes from '$lib/Primes.svelte';
	import PrimesJS from '$lib/PrimesJS.svelte';

	let lastAdded = $state('');
</script>

<svelte:head><title>Go in Svelte components</title></svelte:head>

<h1>&lt;script lang="go"&gt;</h1>
<p>The first component's logic is Go, compiled to WebAssembly (experimental), with runes: <code>State</code>, <code>Derived</code> and <code>Effect</code>. The markup is ordinary Svelte.</p>

<Counter start={10} />

<h2>Live components</h2>
<p>With <code>&lt;script lang="go" live&gt;</code>, the same Go runs on the server instead, connected over a WebSocket. Nothing is downloaded, and state can be shared: open this page in two windows.</p>

<LiveRoom name="visitor" />

<LiveTodos onadded={(text: string) => (lastAdded = text)} />
{#if lastAdded}<p data-testid="last-added">Last added here: {lastAdded}</p>{/if}

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

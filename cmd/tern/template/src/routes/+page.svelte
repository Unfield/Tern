<script lang="ts">
	import { enhance } from '@unfield/tern';
	import type { PageProps } from './$types';

	let { data, form }: PageProps = $props();
</script>

<svelte:head>
	<title>Welcome to Tern</title>
</svelte:head>

<h1>{data.message}</h1>
<p>The server rendered this data at {new Date(data.serverTime).toLocaleTimeString()}.</p>
<p>Edit <code>src/routes/page.server.go</code> and <code>src/routes/+page.svelte</code> to get started.</p>

<form method="POST" use:enhance>
	<label>
		Your name
		<input name="name" value={form?.name ?? ''} autocomplete="name" />
	</label>
	<button>Say hello</button>
</form>

{#if form?.greeting}
	<p class="success">{form.greeting}</p>
{:else if form?.error}
	<p class="error">{form.error}</p>
{/if}

<style>
	form {
		display: flex;
		gap: 0.5rem;
		align-items: end;
		margin-top: 2rem;
	}
	label {
		display: grid;
	}
	input,
	button {
		font: inherit;
		padding: 0.4rem 0.75rem;
	}
	.success {
		color: #15803d;
	}
	.error {
		color: #b91c1c;
	}
</style>

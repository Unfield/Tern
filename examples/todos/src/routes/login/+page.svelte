<script lang="ts">
	import { enhance } from '@unfield/tern';
	import type { PageProps } from './$types';

	let { form }: PageProps = $props();
</script>

<svelte:head>
	<title>Sign in · Todos</title>
</svelte:head>

<main>
	<h1>Sign in</h1>
	<form method="POST" use:enhance>
		<label for="name">Your name</label>
		<input id="name" name="name" value={form?.name ?? ''} autocomplete="name" required />
		<button>Continue</button>
	</form>
	{#if form?.error}<p class="error" role="alert">{form.error}</p>{/if}
</main>

<script lang="ts">
	import { page } from './state.svelte.js';
</script>

<main style="font-family: system-ui, sans-serif; max-width: 36rem; margin: 4rem auto; padding: 0 1rem;">
	<h1>{page.status}</h1>
	<p>{page.error?.message}</p>
</main>

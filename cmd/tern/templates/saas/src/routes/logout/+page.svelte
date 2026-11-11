<script lang="ts">
	import { Button } from '@ternkit/tern/ui';
</script>

<svelte:head><title>Sign out</title></svelte:head>

<main class="center">
	<form method="POST"><Button type="submit">Sign out</Button></form>
</main>

<style>
	.center {
		display: grid;
		place-items: center;
		min-height: 60vh;
	}
</style>

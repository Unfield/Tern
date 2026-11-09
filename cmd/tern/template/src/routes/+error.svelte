<script lang="ts">
	import { page } from '@unfield/tern';
</script>

<svelte:head>
	<title>{page.status} · Tern</title>
</svelte:head>

<h1>{page.status}</h1>
<p>{page.error?.message}</p>
<p><a href="/">Back to the home page</a></p>

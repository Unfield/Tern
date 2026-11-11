<script lang="ts">
	import DocPage from '$lib/DocPage.svelte';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
</script>

<DocPage doc={data} />

<script lang="ts">
	import MonitorForm from '$lib/MonitorForm.svelte';
	import type { PageProps } from './$types';

	let { data, form }: PageProps = $props();
</script>

<svelte:head><title>New monitor · Status</title></svelte:head>

<h1 class="text-2xl font-semibold tracking-tight">New monitor</h1>
<MonitorForm values={form?.values ?? data.values} errors={form?.errors ?? {}} groups={data.groups ?? []} submit="Create monitor" />

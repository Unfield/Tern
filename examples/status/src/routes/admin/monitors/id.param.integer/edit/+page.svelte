<script lang="ts">
	import MonitorForm from '$lib/MonitorForm.svelte';
	import type { PageProps } from './$types';

	let { data, form }: PageProps = $props();
</script>

<svelte:head><title>Edit {data.values.name} · Status</title></svelte:head>

<h1 class="text-2xl font-semibold tracking-tight">Edit {data.values.name}</h1>
<MonitorForm values={form?.values ?? data.values} errors={form?.errors ?? {}} groups={data.groups ?? []} submit="Save changes" cancel="/admin/monitors/{data.values.id}" />

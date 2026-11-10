<script lang="ts">
	let { status, pulse = false }: { status: string; pulse?: boolean } = $props();
	const color: Record<string, string> = {
		up: 'bg-emerald-500',
		down: 'bg-red-500',
		pending: 'bg-amber-500',
		paused: 'bg-zinc-400'
	};
</script>

<span class="relative inline-flex size-2.5 shrink-0" data-status={status}>
	{#if pulse && (status === 'up' || status === 'down')}
		<span class="absolute inline-flex size-full animate-ping rounded-full opacity-60 {color[status]}"></span>
	{/if}
	<span class="relative inline-flex size-2.5 rounded-full {color[status] ?? 'bg-zinc-400'}"></span>
</span>

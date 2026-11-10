<script lang="ts">
	import { datetime } from './format.js';

	type Beat = { at: number; status: string; latencyMs: number; message: string };
	let { beats, count = 40 }: { beats: Beat[]; count?: number } = $props();

	const color: Record<string, string> = { up: 'bg-emerald-500', down: 'bg-red-500', pending: 'bg-amber-500', paused: 'bg-zinc-300' };
	const shown = $derived(beats.slice(-count));
	const empty = $derived(Math.max(0, count - shown.length));
</script>

<div class="flex h-6 items-stretch gap-[2px]" aria-label="Recent checks" data-testid="beats">
	{#each { length: empty }, i (i)}<span class="bg-muted w-1.5 rounded-sm"></span>{/each}
	{#each shown as b (b.at)}
		<span
			class="w-1.5 rounded-sm {color[b.status] ?? 'bg-zinc-300'}"
			title="{datetime(b.at)} · {b.status}{b.status === 'up' ? ` · ${b.latencyMs} ms` : ''}{b.message ? ` · ${b.message}` : ''}"
			data-beat={b.status}
		></span>
	{/each}
</div>

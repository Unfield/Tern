<script lang="ts">
	import { percent } from './format.js';

	type Day = { date: string; ratio: number; down: number };
	let { days }: { days: Day[] } = $props();

	function color(d: Day) {
		if (d.ratio < 0) return 'bg-muted';
		if (d.ratio >= 0.999) return 'bg-emerald-500';
		if (d.ratio >= 0.95) return 'bg-amber-400';
		return 'bg-red-500';
	}
</script>

<div class="flex h-8 items-stretch gap-[2px]" data-testid="days">
	{#each days as d (d.date)}
		<span class="min-w-0 flex-1 rounded-sm {color(d)}" title="{d.date}: {d.ratio < 0 ? 'no data' : `${percent(d.ratio)} uptime`}{d.down ? `, ${d.down} failed checks` : ''}"></span>
	{/each}
</div>
<div class="text-muted-foreground mt-1 flex justify-between text-xs">
	<span>{days.length} days ago</span>
	<span>Today</span>
</div>

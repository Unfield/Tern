<script lang="ts">
	let { name = 'Status' }: { name?: string } = $props();
</script>

<a href="/" class="flex items-center gap-2 font-semibold">
	<svg viewBox="0 0 32 32" class="size-7" aria-hidden="true"
		><rect width="32" height="32" rx="8" class="fill-emerald-600" /><path
			d="M5 17h6l3-7 4 13 3-6h6"
			fill="none"
			stroke="white"
			stroke-width="2.5"
			stroke-linecap="round"
			stroke-linejoin="round"
		/></svg
	>
	<span>{name}</span>
</a>

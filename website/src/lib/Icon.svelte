<script lang="ts">
	// Icons drawn on a 24px grid with 1.75px strokes, as Lucide draws them.
	type Name = 'github' | 'search' | 'sun' | 'moon' | 'menu' | 'x' | 'arrow-right' | 'copy' | 'check' | 'external' | 'chevron-right' | 'chevron-left' | 'edit' | 'clock';
	let { name, size = 16, label }: { name: Name; size?: number; label?: string } = $props();
</script>

<svg
	width={size}
	height={size}
	viewBox="0 0 24 24"
	fill="none"
	stroke="currentColor"
	stroke-width="1.75"
	stroke-linecap="round"
	stroke-linejoin="round"
	role={label ? 'img' : undefined}
	aria-label={label}
	aria-hidden={label ? undefined : 'true'}
>
	{#if name === 'github'}
		<path
			fill="currentColor"
			stroke="none"
			d="M12 1.5a10.5 10.5 0 0 0-3.32 20.46c.53.1.72-.23.72-.5v-1.77c-2.92.63-3.54-1.4-3.54-1.4-.48-1.21-1.17-1.54-1.17-1.54-.95-.65.08-.64.08-.64 1.05.08 1.6 1.08 1.6 1.08.94 1.6 2.46 1.14 3.06.87.1-.68.37-1.14.66-1.4-2.33-.27-4.78-1.17-4.78-5.18 0-1.15.41-2.08 1.08-2.82-.11-.26-.47-1.33.1-2.77 0 0 .88-.28 2.89 1.07a10 10 0 0 1 5.26 0c2-1.35 2.88-1.07 2.88-1.07.58 1.44.22 2.51.11 2.77.67.74 1.08 1.67 1.08 2.82 0 4.03-2.46 4.9-4.8 5.17.38.32.71.96.71 1.94v2.87c0 .28.19.61.73.5A10.5 10.5 0 0 0 12 1.5Z"
		/>
	{:else if name === 'search'}
		<circle cx="11" cy="11" r="7" /><path d="m20 20-3.5-3.5" />
	{:else if name === 'sun'}
		<circle cx="12" cy="12" r="4" /><path d="M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M6.34 17.66l-1.41 1.41M19.07 4.93l-1.41 1.41" />
	{:else if name === 'moon'}
		<path d="M20.5 14.1A8.5 8.5 0 1 1 9.9 3.5a6.6 6.6 0 0 0 10.6 10.6Z" />
	{:else if name === 'menu'}
		<path d="M4 7h16M4 12h16M4 17h16" />
	{:else if name === 'x'}
		<path d="M18 6 6 18M6 6l12 12" />
	{:else if name === 'arrow-right'}
		<path d="M5 12h14M13 6l6 6-6 6" />
	{:else if name === 'copy'}
		<rect x="8" y="8" width="12" height="12" rx="2.5" /><path d="M16 8V6.5A2.5 2.5 0 0 0 13.5 4h-7A2.5 2.5 0 0 0 4 6.5v7A2.5 2.5 0 0 0 6.5 16H8" />
	{:else if name === 'check'}
		<path d="m5 12.5 4.5 4.5L19 7.5" />
	{:else if name === 'external'}
		<path d="M14 4h6v6M20 4l-9 9M18 14v4.5a1.5 1.5 0 0 1-1.5 1.5h-11A1.5 1.5 0 0 1 4 18.5v-11A1.5 1.5 0 0 1 5.5 6H10" />
	{:else if name === 'chevron-right'}
		<path d="m9 6 6 6-6 6" />
	{:else if name === 'chevron-left'}
		<path d="m15 6-6 6 6 6" />
	{:else if name === 'edit'}
		<path d="M4 20h4L19 9a2.8 2.8 0 0 0-4-4L4 16v4Z" /><path d="m13.5 6.5 4 4" />
	{:else if name === 'clock'}
		<circle cx="12" cy="12" r="8.5" /><path d="M12 7.5V12l3 2" />
	{/if}
</svg>

<script lang="ts">
	let { size = 28 }: { size?: number } = $props();
	const id = 'tern-logo-' + Math.random().toString(36).slice(2, 8);
</script>

<svg width={size} height={size} viewBox="0 0 64 64" aria-hidden="true">
	<defs>
		<linearGradient {id} x1="0" y1="0" x2="1" y2="1">
			<stop offset="0" stop-color="#2dd4bf" />
			<stop offset="1" stop-color="#0f766e" />
		</linearGradient>
	</defs>
	<rect width="64" height="64" rx="15" fill="url(#{id})" />
	<path
		fill="#fff"
		d="M9 25c8.5 1.2 15.8 4.6 21.4 10.3 2.5-6.9 8.6-13.8 18.9-19.9 1.9-1.1 3.6.9 2.2 2.5-5.6 6.4-9.3 12.4-10.7 18.8 3.6-.3 7.1.4 10.4 2.2.8.4.6 1.6-.3 1.7-3.8.5-7.2 1.8-10.3 4.1l4 9.8c.4 1-.8 1.8-1.6 1.1l-6.7-6.6-3.4 8.2c-.4 1-1.9.8-2-.3L30 46.4c-6.2-3.2-13.1-9.4-21.6-19.5-.6-.8 0-2 1-1.9z"
	/>
	<circle cx="45.6" cy="40.3" r="1.1" fill="#0f766e" />
</svg>

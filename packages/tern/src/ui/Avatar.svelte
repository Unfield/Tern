<script lang="ts">
	type Props = {
		/** The person's name: the image's alt text, and the initials when there is no image. */
		name: string;
		src?: string | null;
		size?: string;
		class?: string;
	};
	let { name, src = null, size = '2.25rem', class: klass = '' }: Props = $props();
	let failed = $state(false);
	const initials = $derived(
		name
			.split(/\s+/)
			.filter(Boolean)
			.slice(0, 2)
			.map((w) => w[0]!.toUpperCase())
			.join('')
	);
</script>

<span class="tern-avatar {klass}" style:--size={size}>
	{#if src && !failed}
		<img {src} alt={name} onerror={() => (failed = true)} />
	{:else}
		<span role="img" aria-label={name}>{initials}</span>
	{/if}
</span>

<style>
	.tern-avatar {
		display: inline-grid;
		place-items: center;
		width: var(--size);
		height: var(--size);
		border-radius: 50%;
		overflow: hidden;
		background: var(--tern-accent-soft);
		color: var(--tern-accent);
		font-size: calc(var(--size) * 0.38);
		font-weight: 650;
		flex: none;
	}
	img {
		width: 100%;
		height: 100%;
		object-fit: cover;
	}
</style>

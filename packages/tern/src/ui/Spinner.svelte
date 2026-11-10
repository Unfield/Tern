<script lang="ts">
	type Props = { label?: string; size?: string; class?: string };
	let { label = 'Loading', size = '1.25rem', class: klass = '' }: Props = $props();
</script>

<span class="tern-spinner {klass}" role="status" style:--size={size}>
	<span class="ring" aria-hidden="true"></span>
	<span class="tern-sr-only">{label}</span>
</span>

<style>
	.tern-spinner {
		display: inline-flex;
		color: var(--tern-accent);
	}
	.ring {
		width: var(--size);
		height: var(--size);
		border-radius: 50%;
		border: 2px solid color-mix(in oklch, currentColor 20%, transparent);
		border-top-color: currentColor;
		animation: spin 0.75s linear infinite;
	}
	.tern-sr-only {
		position: absolute;
		width: 1px;
		height: 1px;
		overflow: hidden;
		clip: rect(0, 0, 0, 0);
		white-space: nowrap;
	}
	@keyframes spin {
		to {
			transform: rotate(1turn);
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.ring {
			animation-duration: 2s;
		}
	}
</style>

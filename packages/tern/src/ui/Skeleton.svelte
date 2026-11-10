<script lang="ts">
	type Props = { width?: string; height?: string; radius?: string; class?: string };
	let { width = '100%', height = '1rem', radius = 'var(--tern-radius-sm)', class: klass = '' }: Props = $props();
</script>

<!-- Decorative: pair it with a status message, such as aria-busy on the region it stands in for. -->
<span class="tern-skeleton {klass}" aria-hidden="true" style:width style:height style:border-radius={radius}></span>

<style>
	.tern-skeleton {
		display: block;
		background: linear-gradient(90deg, var(--tern-surface-2) 25%, color-mix(in oklch, var(--tern-surface-2) 60%, var(--tern-border)) 50%, var(--tern-surface-2) 75%);
		background-size: 200% 100%;
		animation: shimmer 1.4s ease-in-out infinite;
	}
	@keyframes shimmer {
		from {
			background-position: 100% 0;
		}
		to {
			background-position: -100% 0;
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.tern-skeleton {
			animation: none;
		}
	}
</style>

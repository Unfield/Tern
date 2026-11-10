<script lang="ts">
	import type { Snippet } from 'svelte';

	type Props = { tone?: 'neutral' | 'accent' | 'success' | 'danger' | 'warning'; dot?: boolean; class?: string; children: Snippet };
	let { tone = 'neutral', dot = false, class: klass = '', children }: Props = $props();
</script>

<span class="tern-badge {tone} {klass}">
	{#if dot}<span class="dot" aria-hidden="true"></span>{/if}
	{@render children()}
</span>

<style>
	.tern-badge {
		display: inline-flex;
		align-items: center;
		gap: 0.35rem;
		height: 1.375rem;
		padding: 0 0.55rem;
		border-radius: 999px;
		font-size: 0.75rem;
		font-weight: 600;
		line-height: 1;
		white-space: nowrap;
		border: 1px solid transparent;
	}
	.dot {
		width: 0.4rem;
		height: 0.4rem;
		border-radius: 50%;
		background: currentColor;
	}
	.neutral {
		background: var(--tern-surface-2);
		color: var(--tern-fg);
		border-color: var(--tern-border);
	}
	.accent {
		background: var(--tern-accent-soft);
		color: var(--tern-accent);
	}
	.success {
		background: var(--tern-success-soft);
		color: color-mix(in oklch, var(--tern-success) 80%, var(--tern-fg));
	}
	.danger {
		background: var(--tern-danger-soft);
		color: color-mix(in oklch, var(--tern-danger) 85%, var(--tern-fg));
	}
	.warning {
		background: var(--tern-warning-soft);
		color: color-mix(in oklch, var(--tern-warning) 55%, var(--tern-fg));
	}
</style>

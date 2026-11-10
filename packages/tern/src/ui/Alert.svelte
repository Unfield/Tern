<script lang="ts">
	import type { Snippet } from 'svelte';

	type Props = {
		tone?: 'info' | 'success' | 'warning' | 'danger';
		title?: string;
		/** Announce it when it appears: true for messages shown in response to something, false for ones on the page from the start. */
		live?: boolean;
		class?: string;
		children?: Snippet;
	};
	let { tone = 'info', title, live = false, class: klass = '', children }: Props = $props();
	const role = $derived(live ? (tone === 'danger' || tone === 'warning' ? 'alert' : 'status') : undefined);
	const icons = {
		info: 'M8 7v4M8 5h.01',
		success: 'M5 8.5l2 2 4-4.5',
		warning: 'M8 5v3.5M8 11h.01',
		danger: 'M6 6l4 4M10 6l-4 4'
	};
</script>

<div class="tern-alert {tone} {klass}" {role}>
	<svg class="icon" viewBox="0 0 16 16" width="18" height="18" aria-hidden="true">
		<circle cx="8" cy="8" r="6.5" fill="none" stroke="currentColor" stroke-width="1.4" />
		<path d={icons[tone]} fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" />
	</svg>
	<div class="text">
		{#if title}<p class="title">{title}</p>{/if}
		{#if children}<div class="body">{@render children()}</div>{/if}
	</div>
</div>

<style>
	.tern-alert {
		display: flex;
		gap: 0.7rem;
		padding: 0.85rem 1rem;
		border-radius: var(--tern-radius);
		border: 1px solid;
		font-size: 0.875rem;
	}
	.icon {
		flex: none;
		margin-top: 0.05rem;
	}
	.title {
		margin: 0;
		font-weight: 650;
	}
	.body {
		color: var(--tern-fg);
	}
	.title + .body {
		margin-top: 0.2rem;
	}
	.info {
		background: var(--tern-accent-soft);
		border-color: color-mix(in oklch, var(--tern-accent) 30%, transparent);
		color: var(--tern-accent);
	}
	.success {
		background: var(--tern-success-soft);
		border-color: color-mix(in oklch, var(--tern-success) 30%, transparent);
		color: color-mix(in oklch, var(--tern-success) 80%, var(--tern-fg));
	}
	.warning {
		background: var(--tern-warning-soft);
		border-color: color-mix(in oklch, var(--tern-warning) 35%, transparent);
		color: color-mix(in oklch, var(--tern-warning) 55%, var(--tern-fg));
	}
	.danger {
		background: var(--tern-danger-soft);
		border-color: color-mix(in oklch, var(--tern-danger) 30%, transparent);
		color: color-mix(in oklch, var(--tern-danger) 85%, var(--tern-fg));
	}
</style>

<script lang="ts">
	import type { Snippet } from 'svelte';

	type Props = {
		tone?: 'info' | 'success' | 'warning' | 'danger';
		title?: string;
		/** Announce it when it appears: true for messages shown in response to something, false for ones on the page from the start. */
		live?: boolean;
		class?: string;
		children?: Snippet;
		/** A button or link for what to do about it: at the end, or under the text when there's no room. */
		action?: Snippet;
	};
	let { tone = 'info', title, live = false, class: klass = '', children, action }: Props = $props();
	const role = $derived(live ? (tone === 'danger' || tone === 'warning' ? 'alert' : 'status') : undefined);
	const icons = {
		info: 'M8 7v4M8 5h.01',
		success: 'M5 8.5l2 2 4-4.5',
		warning: 'M8 5v3.5M8 11h.01',
		danger: 'M6 6l4 4M10 6l-4 4'
	};
</script>

<div class="tern-alert {tone} {klass}" class:has-action={!!action} {role}>
	<div class="row">
		<div class="main">
			<svg class="icon" viewBox="0 0 16 16" width="18" height="18" aria-hidden="true">
				<circle cx="8" cy="8" r="6.5" fill="none" stroke="currentColor" stroke-width="1.4" />
				<path d={icons[tone]} fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" />
			</svg>
			<div class="text">
				{#if title}<p class="title">{title}</p>{/if}
				{#if children}<div class="body">{@render children()}</div>{/if}
			</div>
		</div>
		{#if action}<div class="action">{@render action()}</div>{/if}
	</div>
</div>

<style>
	.tern-alert {
		padding: 0.85rem 1rem;
		border-radius: var(--tern-radius);
		border: 1px solid;
		font-size: 0.875rem;
	}
	.main {
		display: flex;
		flex: 1;
		gap: 0.7rem;
		min-width: 0;
	}
	.icon {
		flex: none;
		margin-top: 0.05rem;
	}
	.text {
		flex: 1;
		min-width: 0;
	}
	/* The action sits at the end, centered on the text; in a narrow
	   alert it goes under the text, lined up with it. */
	.has-action {
		container-type: inline-size;
	}
	.has-action .row {
		display: flex;
		align-items: center;
		gap: 0.6rem 1rem;
	}
	.action {
		display: flex;
		flex: none;
		gap: 0.5rem;
	}
	@container (max-width: 30rem) {
		.has-action .row {
			flex-direction: column;
			align-items: stretch;
		}
		.has-action .action {
			padding-left: calc(18px + 0.7rem);
		}
	}
	.action :global(form) {
		display: contents;
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

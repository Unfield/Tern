<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLAnchorAttributes, HTMLButtonAttributes } from 'svelte/elements';

	type Common = {
		/** primary for the main action, secondary for the others, ghost for toolbars, danger for destructive actions */
		variant?: 'primary' | 'secondary' | 'ghost' | 'danger';
		size?: 'sm' | 'md' | 'lg';
		/** Shows a spinner and blocks clicks, while staying focusable. */
		loading?: boolean;
		children?: Snippet;
	};
	type Props = Common & ((HTMLButtonAttributes & { href?: undefined }) | (HTMLAnchorAttributes & { href: string }));

	let { variant = 'primary', size = 'md', loading = false, children, class: klass = '', ...rest }: Props = $props();
</script>

{#if rest.href !== undefined}
	<a class="tern-btn {variant} {size} {klass}" {...rest as HTMLAnchorAttributes}>{@render children?.()}</a>
{:else}
	<button
		class="tern-btn {variant} {size} {klass}"
		type="button"
		{...rest as HTMLButtonAttributes}
		aria-busy={loading || undefined}
		aria-disabled={loading || (rest as HTMLButtonAttributes).disabled || undefined}
		onclick={(e) => {
			if (loading) {
				e.preventDefault();
				return;
			}
			(rest as HTMLButtonAttributes).onclick?.(e as never);
		}}
	>
		{#if loading}<span class="spinner" aria-hidden="true"></span>{/if}
		{@render children?.()}
	</button>
{/if}

<style>
	.tern-btn {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		gap: 0.5em;
		height: var(--tern-control-height);
		padding: 0 1rem;
		border-radius: var(--tern-radius-sm);
		border: 1px solid transparent;
		font: inherit;
		font-size: 0.875rem;
		font-weight: 550;
		line-height: 1;
		white-space: nowrap;
		text-decoration: none;
		cursor: pointer;
		user-select: none;
		transition:
			background-color var(--tern-duration),
			border-color var(--tern-duration),
			box-shadow var(--tern-duration),
			transform var(--tern-duration);
	}
	.tern-btn:focus-visible {
		outline: 2px solid var(--tern-ring);
		outline-offset: 2px;
	}
	.tern-btn:active:not([aria-disabled='true']) {
		transform: translateY(0.5px);
	}
	.tern-btn[aria-disabled='true'],
	.tern-btn:disabled {
		opacity: 0.55;
		cursor: not-allowed;
	}
	.tern-btn[aria-busy='true'] {
		cursor: progress;
	}
	.sm {
		height: calc(var(--tern-control-height) - 0.5rem);
		padding: 0 0.7rem;
		font-size: 0.8125rem;
	}
	.lg {
		height: calc(var(--tern-control-height) + 0.5rem);
		padding: 0 1.35rem;
		font-size: 0.9375rem;
	}
	.primary {
		background: var(--tern-accent);
		color: var(--tern-accent-fg);
		box-shadow:
			var(--tern-shadow),
			inset 0 1px 0 oklch(1 0 0 / 0.14);
	}
	.primary:hover:not([aria-disabled='true']) {
		background: var(--tern-accent-hover);
	}
	.secondary {
		background: var(--tern-surface);
		color: var(--tern-fg);
		border-color: var(--tern-border-strong);
		box-shadow: var(--tern-shadow);
	}
	.secondary:hover:not([aria-disabled='true']) {
		background: var(--tern-surface-2);
	}
	.ghost {
		background: transparent;
		color: var(--tern-fg);
	}
	.ghost:hover:not([aria-disabled='true']) {
		background: var(--tern-surface-2);
	}
	.danger {
		background: var(--tern-danger);
		color: var(--tern-danger-fg);
		box-shadow: var(--tern-shadow);
	}
	.danger:hover:not([aria-disabled='true']) {
		background: var(--tern-danger-hover);
	}
	.spinner {
		width: 1em;
		height: 1em;
		border-radius: 50%;
		border: 2px solid currentColor;
		border-right-color: transparent;
		animation: spin 0.7s linear infinite;
	}
	@keyframes spin {
		to {
			transform: rotate(1turn);
		}
	}
</style>

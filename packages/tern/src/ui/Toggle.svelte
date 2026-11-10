<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLButtonAttributes } from 'svelte/elements';

	type Props = Omit<HTMLButtonAttributes, 'value'> & {
		/** Whether it's on; bind it. */
		pressed?: boolean;
		variant?: 'secondary' | 'ghost';
		size?: 'sm' | 'md';
		/** Posts value (or "on") under this name while pressed. */
		name?: string;
		value?: string;
		/** The label. For an icon alone, set aria-label. */
		children: Snippet;
		onchange?: (pressed: boolean) => void;
	};
	let { pressed = $bindable(false), variant = 'secondary', size = 'md', name, value = 'on', class: klass = '', children, onchange, ...rest }: Props = $props();
</script>

<button
	type="button"
	class="tern-toggle {variant} {size} {klass}"
	aria-pressed={pressed}
	{...rest}
	onclick={() => {
		pressed = !pressed;
		onchange?.(pressed);
	}}
>
	{@render children()}
</button>
{#if name && pressed}<input type="hidden" {name} {value} />{/if}

<style>
	.tern-toggle {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		gap: 0.4rem;
		height: var(--tern-control-height);
		min-width: var(--tern-control-height);
		padding: 0 0.7rem;
		font: inherit;
		font-size: 0.875rem;
		font-weight: 550;
		color: var(--tern-muted);
		border: 1px solid transparent;
		border-radius: var(--tern-radius-sm);
		background: transparent;
		cursor: pointer;
		transition:
			background-color var(--tern-duration),
			color var(--tern-duration);
	}
	.secondary {
		border-color: var(--tern-border-strong);
		background: var(--tern-surface);
	}
	.sm {
		height: calc(var(--tern-control-height) - 0.5rem);
		min-width: calc(var(--tern-control-height) - 0.5rem);
		padding: 0 0.5rem;
		font-size: 0.8125rem;
	}
	.tern-toggle:hover:not(:disabled) {
		color: var(--tern-fg);
		background: var(--tern-surface-2);
	}
	.tern-toggle[aria-pressed='true'] {
		color: var(--tern-fg);
		background: var(--tern-accent-soft);
		border-color: color-mix(in oklch, var(--tern-accent) 45%, transparent);
	}
	.tern-toggle:focus-visible {
		outline: 2px solid var(--tern-ring);
		outline-offset: 2px;
	}
	.tern-toggle:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
</style>

<script lang="ts">
	import { announce } from './announce.js';
	import type { Snippet } from 'svelte';
	import type { HTMLButtonAttributes } from 'svelte/elements';

	type Props = Omit<HTMLButtonAttributes, 'role'> & {
		checked?: boolean;
		/** The label. */
		children?: Snippet;
		/** Submitted with a form as on/off under this name. */
		name?: string;
	};
	let { checked = $bindable(false), children, name, disabled, class: klass = '', ...rest }: Props = $props();
	const uid = $props.id();
</script>

<span class="tern-switch {klass}">
	<button
		type="button"
		role="switch"
		aria-checked={checked}
		aria-labelledby={children ? `tern-switch-${uid}` : undefined}
		{disabled}
		{...rest}
		onclick={(e) => {
			checked = !checked;
			rest.onclick?.(e as never);
		}}
	>
		<span class="thumb" aria-hidden="true"></span>
	</button>
	{#if children}
		<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
		<span class="label" id="tern-switch-{uid}" onclick={() => !disabled && (checked = !checked)}>{@render children()}</span>
	{/if}
	{#if name}<input type="hidden" {name} value={checked ? 'on' : 'off'} use:announce={checked} />{/if}
</span>

<style>
	.tern-switch {
		display: inline-flex;
		align-items: center;
		gap: 0.6rem;
		font-size: 0.875rem;
	}
	button {
		position: relative;
		flex: none;
		width: 2.25rem;
		height: 1.3rem;
		padding: 0;
		border-radius: 999px;
		border: 1px solid var(--tern-border-strong);
		background: var(--tern-surface-2);
		cursor: pointer;
		transition:
			background-color var(--tern-duration),
			border-color var(--tern-duration);
	}
	.thumb {
		position: absolute;
		top: 50%;
		left: 0.14rem;
		width: 0.95rem;
		height: 0.95rem;
		border-radius: 50%;
		background: var(--tern-surface);
		box-shadow: 0 1px 2px oklch(0 0 0 / 0.25);
		translate: 0 -50%;
		transition: left var(--tern-duration);
	}
	button[aria-checked='true'] {
		background: var(--tern-accent);
		border-color: var(--tern-accent);
	}
	button[aria-checked='true'] .thumb {
		left: calc(100% - 0.14rem - 0.95rem);
	}
	button:focus-visible {
		outline: 2px solid var(--tern-ring);
		outline-offset: 2px;
	}
	button:disabled {
		opacity: 0.55;
		cursor: not-allowed;
	}
	.label {
		cursor: pointer;
		user-select: none;
	}
</style>

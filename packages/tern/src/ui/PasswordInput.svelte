<script lang="ts">
	import { tb } from '../runtime/i18n.svelte.js';
	import type { HTMLInputAttributes } from 'svelte/elements';
	import { getField } from './field.js';

	type Props = Omit<HTMLInputAttributes, 'type'> & {
		value?: string;
		/** Whether the password is shown; bind it. */
		visible?: boolean;
		/** current-password on sign-in forms, new-password where it's set, so password managers help. */
		autocomplete?: 'current-password' | 'new-password';
	};
	let { value = $bindable(''), visible = $bindable(false), autocomplete = 'current-password', class: klass = '', ...rest }: Props = $props();

	const uid = $props.id();
	const field = getField();
	const id = field?.id ?? `tern-password-${uid}`;
	let input = $state<HTMLInputElement>();
</script>

<div class="tern-password {klass}">
	<input
		bind:this={input}
		class="input"
		{id}
		type={visible ? 'text' : 'password'}
		{autocomplete}
		autocapitalize="off"
		spellcheck="false"
		aria-describedby={field?.describedBy}
		aria-invalid={field?.invalid || undefined}
		required={field?.required}
		disabled={field?.disabled}
		name={field?.name}
		{...rest}
		bind:value
	/>
	<button
		type="button"
		class="toggle"
		aria-label={tb('tern.ui.password.show')}
		aria-pressed={visible}
		aria-controls={id}
		disabled={field?.disabled || rest.disabled}
		onclick={() => {
			// Keep the caret where it was when the type changes.
			const at = input?.selectionStart ?? null;
			visible = !visible;
			requestAnimationFrame(() => at !== null && input === document.activeElement && input.setSelectionRange(at, at));
		}}
	>
		{#if visible}
			<svg aria-hidden="true" viewBox="0 0 20 20" width="18" height="18" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
				<path d="M3 3l14 14" />
				<path d="M8.6 8.6a2 2 0 002.8 2.8" />
				<path d="M6.2 6.3C4.4 7.4 3.1 8.9 2.5 10c1.3 2.4 4.2 5 7.5 5 1.4 0 2.7-.4 3.8-1.1" />
				<path d="M9 5.1c.3 0 .7-.1 1-.1 3.3 0 6.2 2.6 7.5 5-.4.8-1 1.6-1.7 2.4" />
			</svg>
		{:else}
			<svg aria-hidden="true" viewBox="0 0 20 20" width="18" height="18" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
				<path d="M2.5 10c1.3-2.4 4.2-5 7.5-5s6.2 2.6 7.5 5c-1.3 2.4-4.2 5-7.5 5s-6.2-2.6-7.5-5z" />
				<circle cx="10" cy="10" r="2.25" />
			</svg>
		{/if}
	</button>
</div>

<style>
	.tern-password {
		position: relative;
		display: flex;
		align-items: center;
	}
	.input {
		width: 100%;
		box-sizing: border-box;
		height: var(--tern-control-height);
		padding: 0 2.5rem 0 0.75rem;
		font: inherit;
		font-size: 0.875rem;
		color: var(--tern-fg);
		background: var(--tern-surface);
		border: 1px solid var(--tern-border-strong);
		border-radius: var(--tern-radius-sm);
		transition:
			border-color var(--tern-duration),
			box-shadow var(--tern-duration);
	}
	/* Browsers' own reveal buttons would sit next to ours. */
	.input::-ms-reveal,
	.input::-ms-clear {
		display: none;
	}
	.input:hover:not(:disabled) {
		border-color: var(--tern-muted);
	}
	.input:focus-visible {
		outline: none;
		border-color: var(--tern-accent);
		box-shadow: 0 0 0 3px color-mix(in oklch, var(--tern-ring) 30%, transparent);
	}
	.input[aria-invalid='true'] {
		border-color: var(--tern-danger);
	}
	.input:disabled {
		opacity: 0.6;
		cursor: not-allowed;
	}
	.toggle {
		position: absolute;
		right: 0.3rem;
		display: grid;
		place-items: center;
		width: 1.875rem;
		height: 1.875rem;
		padding: 0;
		color: var(--tern-muted);
		background: transparent;
		border: 0;
		border-radius: calc(var(--tern-radius-sm) - 2px);
		cursor: pointer;
	}
	.toggle:hover:not(:disabled) {
		color: var(--tern-fg);
		background: var(--tern-surface-2);
	}
	.toggle:focus-visible {
		outline: 2px solid var(--tern-ring);
		outline-offset: 0;
	}
	.toggle:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
</style>

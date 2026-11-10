<script lang="ts">
	import { getField } from './field.js';

	type Props = {
		/** The code; bind it. */
		value?: string;
		/** How many characters. */
		length?: number;
		/** numeric for digits only, or alphanumeric. */
		mode?: 'numeric' | 'alphanumeric';
		/** Labels it when it isn't in a Field. */
		label?: string;
		name?: string;
		disabled?: boolean;
		class?: string;
		/** Runs once every character is entered, such as to submit. */
		oncomplete?: (code: string) => void;
	};
	let { value = $bindable(''), length = 6, mode = 'numeric', label, name, disabled = false, class: klass = '', oncomplete }: Props = $props();

	const field = getField();
	let input = $state<HTMLInputElement>();
	let focused = $state(false);
	let caret = $state(0);
	const pattern = $derived(mode === 'numeric' ? /[^0-9]/g : /[^0-9a-z]/gi);

	function oninput() {
		const clean = input!.value.replace(pattern, '').slice(0, length);
		input!.value = clean;
		value = clean;
		caret = clean.length;
		if (clean.length === length) oncomplete?.(clean);
	}
</script>

<!-- One real input, drawn as boxes: pasting, autofill from SMS and screen readers all just work. -->
<div class="tern-otp {klass}" class:focused class:disabled={disabled || field?.disabled} style:--length={length}>
	<input
		bind:this={input}
		{value}
		{name}
		id={field?.id}
		aria-label={field ? undefined : label}
		aria-describedby={field?.describedBy}
		aria-invalid={field?.invalid || undefined}
		required={field?.required}
		disabled={disabled || field?.disabled}
		inputmode={mode === 'numeric' ? 'numeric' : 'text'}
		autocomplete="one-time-code"
		autocapitalize="characters"
		spellcheck="false"
		pattern={mode === 'numeric' ? `[0-9]{${length}}` : `[0-9A-Za-z]{${length}}`}
		{oninput}
		onfocus={() => {
			focused = true;
			caret = value.length;
		}}
		onblur={() => (focused = false)}
		onselect={() => (caret = Math.min(input?.selectionStart ?? 0, length - 1))}
	/>
	<div class="boxes" aria-hidden="true">
		{#each { length }, i (i)}
			<span class="box" class:active={focused && i === Math.min(caret, length - 1)} class:filled={i < value.length}>
				{value[i] ?? ''}
				{#if focused && i === value.length}<span class="caret"></span>{/if}
			</span>
		{/each}
	</div>
</div>

<style>
	.tern-otp {
		position: relative;
		width: fit-content;
	}
	input {
		position: absolute;
		inset: 0;
		width: 100%;
		height: 100%;
		opacity: 0;
		font-size: 16px; /* no zoom on iOS */
		letter-spacing: 2.6rem;
		caret-color: transparent;
		border: 0;
		padding: 0;
		cursor: text;
		z-index: 1;
	}
	.boxes {
		display: flex;
		gap: 0.5rem;
	}
	.box {
		position: relative;
		display: grid;
		place-items: center;
		width: 2.5rem;
		height: 2.875rem;
		font-family: var(--tern-font-mono);
		font-size: 1.25rem;
		font-weight: 600;
		text-transform: uppercase;
		color: var(--tern-fg);
		background: var(--tern-surface);
		border: 1px solid var(--tern-border-strong);
		border-radius: var(--tern-radius-sm);
		transition:
			border-color var(--tern-duration),
			box-shadow var(--tern-duration);
	}
	.box.active {
		border-color: var(--tern-accent);
		box-shadow: 0 0 0 3px color-mix(in oklch, var(--tern-ring) 30%, transparent);
	}
	.caret {
		position: absolute;
		width: 1px;
		height: 1.25rem;
		background: var(--tern-fg);
		animation: blink 1s steps(1) infinite;
	}
	.tern-otp:has(input[aria-invalid='true']) .box {
		border-color: var(--tern-danger);
	}
	.disabled {
		opacity: 0.6;
	}
	@keyframes blink {
		50% {
			opacity: 0;
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.caret {
			animation: none;
		}
	}
</style>

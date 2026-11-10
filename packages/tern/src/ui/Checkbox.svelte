<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLInputAttributes } from 'svelte/elements';

	type Props = Omit<HTMLInputAttributes, 'type'> & {
		checked?: boolean;
		/** Shows a dash, for "some selected". */
		indeterminate?: boolean;
		/** The label. */
		children?: Snippet;
		/** Help text under the label. */
		description?: string;
	};
	let { checked = $bindable(false), indeterminate = $bindable(false), children, description, class: klass = '', ...rest }: Props = $props();
	const uid = $props.id();
</script>

<label class="tern-check {klass}">
	<input type="checkbox" {...rest} bind:checked bind:indeterminate aria-describedby={description ? `tern-check-${uid}` : undefined} />
	<span class="box" aria-hidden="true">
		<svg viewBox="0 0 16 16"><path class="tick" d="M3.5 8.5l3 3 6-7" /><path class="dash" d="M4 8h8" /></svg>
	</span>
	<span class="text">
		{@render children?.()}
		{#if description}<span class="description" id="tern-check-{uid}">{description}</span>{/if}
	</span>
</label>

<style>
	.tern-check {
		display: inline-flex;
		align-items: flex-start;
		gap: 0.6rem;
		font-size: 0.875rem;
		cursor: pointer;
		position: relative;
	}
	input {
		position: absolute;
		opacity: 0;
		width: 1.125rem;
		height: 1.125rem;
		margin: 0.1rem 0 0;
		cursor: inherit;
	}
	.box {
		flex: none;
		display: grid;
		place-items: center;
		width: 1.125rem;
		height: 1.125rem;
		margin-top: 0.1rem;
		border: 1px solid var(--tern-border-strong);
		border-radius: 5px;
		background: var(--tern-surface);
		transition:
			background-color var(--tern-duration),
			border-color var(--tern-duration);
	}
	svg {
		width: 0.8rem;
		height: 0.8rem;
		fill: none;
		stroke: var(--tern-accent-fg);
		stroke-width: 2.2;
		stroke-linecap: round;
		stroke-linejoin: round;
	}
	.tick,
	.dash {
		display: none;
	}
	input:checked + .box,
	input:indeterminate + .box {
		background: var(--tern-accent);
		border-color: var(--tern-accent);
	}
	input:checked:not(:indeterminate) + .box .tick,
	input:indeterminate + .box .dash {
		display: block;
	}
	input:focus-visible + .box {
		outline: 2px solid var(--tern-ring);
		outline-offset: 2px;
	}
	input:disabled ~ * {
		opacity: 0.55;
	}
	.tern-check:has(input:disabled) {
		cursor: not-allowed;
	}
	.text {
		display: grid;
		gap: 0.125rem;
	}
	.description {
		color: var(--tern-muted);
		font-size: 0.8125rem;
	}
</style>

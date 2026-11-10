<script lang="ts">
	type Option = { value: string; label: string; description?: string; disabled?: boolean };
	type Props = {
		/** The group's label, read as the fieldset's legend. */
		label: string;
		name: string;
		options: Option[];
		value?: string;
		/** Lay the options out in a row. */
		inline?: boolean;
		required?: boolean;
		disabled?: boolean;
		class?: string;
	};
	let { label, name, options, value = $bindable(), inline = false, required = false, disabled = false, class: klass = '' }: Props = $props();
	const uid = $props.id();
</script>

<!-- Native radios: arrow keys move between options, as screen readers expect. -->
<fieldset class="tern-radios {klass}" class:inline {disabled}>
	<legend>{label}</legend>
	<div class="options">
		{#each options as o, i (o.value)}
			<label class="option">
				<input type="radio" {name} value={o.value} bind:group={value} disabled={o.disabled} {required} aria-describedby={o.description ? `tern-radio-${uid}-${i}` : undefined} />
				<span class="dot" aria-hidden="true"></span>
				<span class="text">
					{o.label}
					{#if o.description}<span class="description" id="tern-radio-{uid}-{i}">{o.description}</span>{/if}
				</span>
			</label>
		{/each}
	</div>
</fieldset>

<style>
	fieldset {
		border: 0;
		padding: 0;
		margin: 0;
		min-width: 0;
	}
	legend {
		font-size: 0.875rem;
		font-weight: 550;
		margin-bottom: 0.5rem;
		padding: 0;
	}
	.options {
		display: grid;
		gap: 0.5rem;
	}
	.inline .options {
		display: flex;
		flex-wrap: wrap;
		gap: 1.25rem;
	}
	.option {
		position: relative;
		display: inline-flex;
		align-items: flex-start;
		gap: 0.6rem;
		font-size: 0.875rem;
		cursor: pointer;
	}
	input {
		position: absolute;
		opacity: 0;
		width: 1.125rem;
		height: 1.125rem;
		margin: 0.1rem 0 0;
		cursor: inherit;
	}
	.dot {
		flex: none;
		width: 1.125rem;
		height: 1.125rem;
		margin-top: 0.1rem;
		box-sizing: border-box;
		border-radius: 50%;
		border: 1px solid var(--tern-border-strong);
		background: var(--tern-surface);
		transition:
			border-width var(--tern-duration),
			border-color var(--tern-duration);
	}
	input:checked + .dot {
		border: 0.33rem solid var(--tern-accent);
	}
	input:focus-visible + .dot {
		outline: 2px solid var(--tern-ring);
		outline-offset: 2px;
	}
	input:disabled ~ * {
		opacity: 0.55;
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

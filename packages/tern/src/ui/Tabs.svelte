<script lang="ts">
	import type { Snippet } from 'svelte';

	type Tab = { id: string; label: string; disabled?: boolean };
	type Props = {
		tabs: Tab[];
		/** The selected tab's id; bind it. Defaults to the first tab. */
		value?: string;
		/** Names the tab list for screen readers. */
		label?: string;
		class?: string;
		/** Renders the selected tab's panel. */
		panel: Snippet<[string]>;
	};
	let { tabs, value = $bindable(), label, class: klass = '', panel }: Props = $props();
	const uid = $props.id();
	const selected = $derived(value ?? tabs.find((t) => !t.disabled)?.id);
	const buttons: HTMLButtonElement[] = [];

	// The ARIA tabs pattern: arrow keys move and select, Home and End jump.
	function onkeydown(e: KeyboardEvent, i: number) {
		const enabled = tabs.map((t, j) => (t.disabled ? -1 : j)).filter((j) => j >= 0);
		const at = enabled.indexOf(i);
		let next: number | undefined;
		if (e.key === 'ArrowRight') next = enabled[(at + 1) % enabled.length];
		else if (e.key === 'ArrowLeft') next = enabled[(at - 1 + enabled.length) % enabled.length];
		else if (e.key === 'Home') next = enabled[0];
		else if (e.key === 'End') next = enabled[enabled.length - 1];
		if (next === undefined) return;
		e.preventDefault();
		value = tabs[next]!.id;
		buttons[next]?.focus();
	}
</script>

<div class="tern-tabs {klass}">
	<div class="list" role="tablist" aria-label={label}>
		{#each tabs as t, i (t.id)}
			<button
				bind:this={buttons[i]}
				type="button"
				role="tab"
				id="tern-tab-{uid}-{t.id}"
				aria-selected={selected === t.id}
				aria-controls="tern-tabpanel-{uid}-{t.id}"
				tabindex={selected === t.id ? 0 : -1}
				disabled={t.disabled}
				onclick={() => (value = t.id)}
				onkeydown={(e) => onkeydown(e, i)}>{t.label}</button
			>
		{/each}
	</div>
	{#if selected}
		<div class="panel" role="tabpanel" id="tern-tabpanel-{uid}-{selected}" aria-labelledby="tern-tab-{uid}-{selected}" tabindex="0">
			{@render panel(selected)}
		</div>
	{/if}
</div>

<style>
	.list {
		display: flex;
		gap: 0.25rem;
		border-bottom: 1px solid var(--tern-border);
	}
	[role='tab'] {
		position: relative;
		padding: 0.55rem 0.8rem;
		margin-bottom: -1px;
		border: 0;
		background: transparent;
		font: inherit;
		font-size: 0.875rem;
		font-weight: 550;
		color: var(--tern-muted);
		cursor: pointer;
		border-radius: var(--tern-radius-sm) var(--tern-radius-sm) 0 0;
		transition: color var(--tern-duration);
	}
	[role='tab']:hover:not(:disabled) {
		color: var(--tern-fg);
	}
	[role='tab'][aria-selected='true'] {
		color: var(--tern-fg);
	}
	[role='tab'][aria-selected='true']::after {
		content: '';
		position: absolute;
		left: 0.5rem;
		right: 0.5rem;
		bottom: 0;
		height: 2px;
		border-radius: 2px;
		background: var(--tern-accent);
	}
	[role='tab']:focus-visible {
		outline: 2px solid var(--tern-ring);
		outline-offset: -2px;
	}
	[role='tab']:disabled {
		opacity: 0.45;
		cursor: not-allowed;
	}
	.panel {
		padding-top: 1rem;
	}
	.panel:focus-visible {
		outline: 2px solid var(--tern-ring);
		outline-offset: 4px;
		border-radius: 2px;
	}
</style>

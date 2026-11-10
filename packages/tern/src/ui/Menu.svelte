<script lang="ts">
	import type { Snippet } from 'svelte';
	import MenuList from './MenuList.svelte';
	import type { MenuItem } from './menu.js';

	type Props = {
		/** The items. Items can have submenus, checkboxes, shortcuts and group headings. */
		items: MenuItem[];
		/** The button's content. */
		children: Snippet;
		/** Opens to the start or end edge of the button. */
		align?: 'start' | 'end';
		variant?: 'secondary' | 'ghost';
		class?: string;
	};
	let { items, children, align = 'start', variant = 'secondary', class: klass = '' }: Props = $props();

	const uid = $props.id();
	let open = $state(false);
	let focus = $state<'first' | 'last'>('first');
	let button = $state<HTMLButtonElement>();

	function show(where: 'first' | 'last') {
		focus = where;
		open = true;
	}
</script>

<div class="tern-menu {klass}">
	<button
		bind:this={button}
		type="button"
		class="trigger {variant}"
		id="tern-menu-{uid}-button"
		aria-haspopup="menu"
		aria-expanded={open}
		aria-controls={open ? `tern-menu-${uid}` : undefined}
		onclick={() => (open ? (open = false) : show('first'))}
		onkeydown={(e) => {
			if (e.key === 'ArrowDown' || e.key === 'Enter' || e.key === ' ') {
				e.preventDefault();
				show('first');
			} else if (e.key === 'ArrowUp') {
				e.preventDefault();
				show('last');
			}
		}}
	>
		{@render children()}
		<svg aria-hidden="true" viewBox="0 0 16 16" width="14" height="14"><path d="M4 6l4 4 4-4" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" /></svg>
	</button>
	{#if open && button}
		<MenuList
			{items}
			id="tern-menu-{uid}"
			labelledby="tern-menu-{uid}-button"
			anchor={button}
			{align}
			{focus}
			inside={() => [button]}
			onclose={(how) => {
				open = false;
				if (how.returnFocus) button?.focus();
			}}
		/>
	{/if}
</div>

<style>
	.tern-menu {
		display: inline-block;
	}
	.trigger {
		display: inline-flex;
		align-items: center;
		gap: 0.4rem;
		height: var(--tern-control-height);
		padding: 0 0.75rem 0 0.9rem;
		font: inherit;
		font-size: 0.875rem;
		font-weight: 550;
		color: var(--tern-fg);
		border-radius: var(--tern-radius-sm);
		cursor: pointer;
	}
	.secondary {
		background: var(--tern-surface);
		border: 1px solid var(--tern-border-strong);
		box-shadow: var(--tern-shadow);
	}
	.ghost {
		background: transparent;
		border: 1px solid transparent;
	}
	.trigger:hover,
	.trigger[aria-expanded='true'] {
		background: var(--tern-surface-2);
	}
	.trigger:focus-visible {
		outline: 2px solid var(--tern-ring);
		outline-offset: 2px;
	}
</style>

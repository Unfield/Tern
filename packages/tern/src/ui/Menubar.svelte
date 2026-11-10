<script lang="ts">
	import MenuList from './MenuList.svelte';
	import type { MenuItem } from './menu.js';

	type Props = {
		/** The menus, such as File, Edit and View. */
		menus: { label: string; items: MenuItem[] }[];
		/** Names the bar, such as "Editor". */
		label: string;
		class?: string;
	};
	let { menus, label, class: klass = '' }: Props = $props();

	const uid = $props.id();
	let active = $state(0); // the top item in the tab order
	let open = $state<number | null>(null);
	let focus = $state<'first' | 'last'>('first');
	const buttons: HTMLButtonElement[] = [];

	function show(i: number, where: 'first' | 'last' = 'first') {
		active = i;
		focus = where;
		open = i;
	}

	function step(by: number, keepOpen: boolean) {
		const next = (active + by + menus.length) % menus.length;
		if (keepOpen) show(next);
		else {
			active = next;
			buttons[next]?.focus();
		}
	}

	function onbarkey(e: KeyboardEvent, i: number) {
		switch (e.key) {
			case 'ArrowRight':
				e.preventDefault();
				return step(1, false);
			case 'ArrowLeft':
				e.preventDefault();
				return step(-1, false);
			case 'Home':
				e.preventDefault();
				active = 0;
				return buttons[0]?.focus();
			case 'End':
				e.preventDefault();
				active = menus.length - 1;
				return buttons[active]?.focus();
			case 'ArrowDown':
			case 'Enter':
			case ' ':
				e.preventDefault();
				return show(i);
			case 'ArrowUp':
				e.preventDefault();
				return show(i, 'last');
		}
	}
</script>

<div class="tern-menubar {klass}" role="menubar" aria-label={label}>
	{#each menus as menu, i (i)}
		<button
			bind:this={buttons[i]}
			type="button"
			role="menuitem"
			id="tern-menubar-{uid}-{i}"
			tabindex={active === i ? 0 : -1}
			aria-haspopup="menu"
			aria-expanded={open === i}
			aria-controls={open === i ? `tern-menubar-${uid}-${i}-menu` : undefined}
			onclick={() => (open === i ? (open = null) : show(i))}
			onpointerenter={() => open !== null && open !== i && show(i)}
			onkeydown={(e) => onbarkey(e, i)}
		>
			{menu.label}
		</button>
		{#if open === i && buttons[i]}
			<MenuList
				items={menu.items}
				id="tern-menubar-{uid}-{i}-menu"
				labelledby="tern-menubar-{uid}-{i}"
				anchor={buttons[i]}
				{focus}
				inside={() => buttons}
				onkey={(e) => {
					if (e.key === 'ArrowRight' || e.key === 'ArrowLeft') {
						e.preventDefault();
						step(e.key === 'ArrowRight' ? 1 : -1, true);
					}
				}}
				onclose={(how) => {
					open = null;
					if (how.returnFocus) buttons[i]?.focus();
				}}
			/>
		{/if}
	{/each}
</div>

<style>
	.tern-menubar {
		display: inline-flex;
		gap: 0.125rem;
		padding: 0.2rem;
		background: var(--tern-surface);
		border: 1px solid var(--tern-border);
		border-radius: var(--tern-radius);
		box-shadow: var(--tern-shadow);
	}
	button {
		height: 1.875rem;
		padding: 0 0.7rem;
		font: inherit;
		font-size: 0.875rem;
		font-weight: 550;
		color: var(--tern-fg);
		background: transparent;
		border: 0;
		border-radius: var(--tern-radius-sm);
		cursor: pointer;
	}
	button:hover,
	button[aria-expanded='true'] {
		background: var(--tern-surface-2);
	}
	button:focus-visible {
		outline: 2px solid var(--tern-ring);
		outline-offset: 1px;
	}
</style>

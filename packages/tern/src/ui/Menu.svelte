<script lang="ts">
	import type { Snippet } from 'svelte';

	type Item = { label: string; onselect?: () => void; href?: string; danger?: boolean; disabled?: boolean; separator?: boolean };
	type Props = {
		/** The items; an item with separator: true draws a line. */
		items: Item[];
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
	let button = $state<HTMLButtonElement>();
	let menu = $state<HTMLDivElement>();
	let typed = '';
	let typedTimer: ReturnType<typeof setTimeout> | undefined;

	const entries = () => Array.from(menu?.querySelectorAll<HTMLElement>('[role="menuitem"]:not([aria-disabled="true"])') ?? []);

	async function show(focus: 'first' | 'last') {
		open = true;
		await Promise.resolve();
		requestAnimationFrame(() => {
			const list = entries();
			(focus === 'first' ? list[0] : list[list.length - 1])?.focus();
		});
	}

	function hide(returnFocus = true) {
		open = false;
		if (returnFocus) button?.focus();
	}

	function select(item: Item) {
		if (item.disabled) return;
		hide();
		item.onselect?.();
	}

	// The ARIA menu button pattern.
	function onmenukey(e: KeyboardEvent) {
		const list = entries();
		const i = list.indexOf(document.activeElement as HTMLElement);
		const move = (j: number) => {
			e.preventDefault();
			list[(j + list.length) % list.length]?.focus();
		};
		switch (e.key) {
			case 'ArrowDown':
				return move(i + 1);
			case 'ArrowUp':
				return move(i - 1);
			case 'Home':
				return move(0);
			case 'End':
				return move(list.length - 1);
			case 'Escape':
				e.preventDefault();
				return hide();
			case 'Tab':
				return hide(false);
		}
		if (e.key.length === 1 && /\S/.test(e.key)) {
			// Type to jump to an item.
			typed += e.key.toLowerCase();
			clearTimeout(typedTimer);
			typedTimer = setTimeout(() => (typed = ''), 500);
			const start = typed.length === 1 ? i + 1 : i;
			const order = [...list.slice(start), ...list.slice(0, start)];
			order.find((el) => el.textContent?.trim().toLowerCase().startsWith(typed))?.focus();
		}
	}

	$effect(() => {
		if (!open) return;
		const away = (e: PointerEvent) => {
			if (!menu?.contains(e.target as Node) && !button?.contains(e.target as Node)) hide(false);
		};
		document.addEventListener('pointerdown', away);
		return () => document.removeEventListener('pointerdown', away);
	});
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
		onclick={() => (open ? hide() : show('first'))}
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
	{#if open}
		<div bind:this={menu} class="menu {align}" role="menu" id="tern-menu-{uid}" aria-labelledby="tern-menu-{uid}-button" tabindex="-1" onkeydown={onmenukey}>
			{#each items as item, i (i)}
				{#if item.separator}
					<div class="separator" role="separator"></div>
				{:else if item.href && !item.disabled}
					<a role="menuitem" tabindex="-1" class:danger={item.danger} href={item.href} onclick={() => hide(false)}>{item.label}</a>
				{:else}
					<div
						role="menuitem"
						tabindex="-1"
						class:danger={item.danger}
						aria-disabled={item.disabled || undefined}
						onclick={() => select(item)}
						onkeydown={(e) => {
							if (e.key === 'Enter' || e.key === ' ') {
								e.preventDefault();
								select(item);
							}
						}}
					>
						{item.label}
					</div>
				{/if}
			{/each}
		</div>
	{/if}
</div>

<style>
	.tern-menu {
		position: relative;
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
	.menu {
		position: absolute;
		top: calc(100% + 0.375rem);
		z-index: 50;
		min-width: 11rem;
		padding: 0.3rem;
		background: var(--tern-surface);
		border: 1px solid var(--tern-border);
		border-radius: var(--tern-radius);
		box-shadow: var(--tern-shadow-lg);
		animation: pop var(--tern-duration) ease-out;
	}
	.start {
		left: 0;
	}
	.end {
		right: 0;
	}
	[role='menuitem'] {
		display: block;
		padding: 0.45rem 0.65rem;
		border-radius: calc(var(--tern-radius-sm) - 2px);
		font-size: 0.875rem;
		color: var(--tern-fg);
		text-decoration: none;
		cursor: pointer;
		outline: none;
	}
	[role='menuitem']:hover,
	[role='menuitem']:focus {
		background: var(--tern-accent-soft);
	}
	[role='menuitem'].danger {
		color: var(--tern-danger);
	}
	[role='menuitem'].danger:hover,
	[role='menuitem'].danger:focus {
		background: var(--tern-danger-soft);
	}
	[role='menuitem'][aria-disabled='true'] {
		opacity: 0.45;
		cursor: not-allowed;
		background: none;
	}
	.separator {
		height: 1px;
		margin: 0.3rem 0.2rem;
		background: var(--tern-border);
	}
	@keyframes pop {
		from {
			opacity: 0;
			transform: translateY(-4px);
		}
	}
</style>

<script lang="ts">
	import MenuList from './MenuList.svelte';
	import { float, type Align, type Side } from './floating.js';
	import type { MenuClose, MenuItem } from './menu.js';

	type Props = {
		items: MenuItem[];
		id: string;
		labelledby?: string;
		label?: string;
		/** What it opens next to: an element, or a point for context menus. */
		anchor: Element | { x: number; y: number };
		side?: Side;
		align?: Align;
		/** Which item gets focus when it opens. */
		focus?: 'first' | 'last' | 'none';
		/** Whether this is a submenu, which ArrowLeft closes. */
		sub?: boolean;
		onclose: (how: MenuClose) => void;
		/** Keys the list doesn't use, such as ArrowLeft and ArrowRight in a menubar. */
		onkey?: (e: KeyboardEvent) => void;
		/** Elements whose clicks don't close the menu, such as its button. */
		inside?: () => (Element | null | undefined)[];
		class?: string;
	};
	let { items, id, labelledby, label, anchor, side = 'bottom', align = 'start', focus = 'first', sub = false, onclose, onkey, inside, class: klass = '' }: Props = $props();

	let menu = $state<HTMLDivElement>();
	let openSub = $state<number | null>(null);
	let subFocus = $state<'first' | 'none'>('first');
	const itemEls: HTMLElement[] = [];
	let typed = '';
	let typedTimer: ReturnType<typeof setTimeout> | undefined;
	let hoverTimer: ReturnType<typeof setTimeout> | undefined;

	// Items split into groups at each heading, so screen readers announce them.
	const groups = $derived.by(() => {
		const out: { heading?: string; entries: { item: MenuItem; index: number }[] }[] = [{ entries: [] }];
		items.forEach((item, index) => {
			if (item.heading !== undefined) out.push({ heading: item.heading, entries: [] });
			else out[out.length - 1]!.entries.push({ item, index });
		});
		return out.filter((g) => g.heading !== undefined || g.entries.length > 0);
	});

	const entries = () =>
		Array.from(menu?.querySelectorAll<HTMLElement>('[data-tern-menuitem]:not([aria-disabled="true"])') ?? []).filter((el) => el.closest('[role="menu"]') === menu);

	$effect(() => {
		if (!menu) return;
		menu.showPopover?.();
		const stop = float(menu, anchor, { side, align, offset: sub ? 2 : 6 });
		const list = entries();
		if (focus === 'first') list[0]?.focus();
		else if (focus === 'last') list[list.length - 1]?.focus();
		return stop;
	});

	// The root list closes on clicks outside it and everything it opened.
	$effect(() => {
		if (sub) return;
		const away = (e: PointerEvent) => {
			const t = e.target as Node;
			if (menu?.contains(t) || inside?.().some((el) => el?.contains(t))) return;
			onclose({ all: true, returnFocus: false });
		};
		document.addEventListener('pointerdown', away, true);
		return () => document.removeEventListener('pointerdown', away, true);
	});

	function activate(item: MenuItem, index: number, how: 'key' | 'pointer') {
		if (item.disabled) return;
		if (item.items) {
			subFocus = how === 'key' ? 'first' : 'none';
			openSub = index;
			return;
		}
		onclose({ all: true, returnFocus: !item.href });
		item.onselect?.();
	}

	function onkeydown(e: KeyboardEvent) {
		if (e.target instanceof Node && (e.target as HTMLElement).closest?.('[role="menu"]') !== menu) return; // a submenu's key
		const list = entries();
		const current = list.indexOf(document.activeElement as HTMLElement);
		const move = (j: number) => {
			e.preventDefault();
			list[(j + list.length) % list.length]?.focus();
		};
		const index = Number((document.activeElement as HTMLElement | null)?.dataset.ternMenuitem);
		const item = Number.isNaN(index) ? undefined : items[index];
		switch (e.key) {
			case 'ArrowDown':
				return move(current + 1);
			case 'ArrowUp':
				return move(current < 0 ? list.length - 1 : current - 1);
			case 'Home':
			case 'PageUp':
				return move(0);
			case 'End':
			case 'PageDown':
				return move(list.length - 1);
			case 'Enter':
			case ' ':
				if (item) {
					e.preventDefault();
					if (item.href && e.key === 'Enter' && !item.items) return onclose({ all: true, returnFocus: false }); // the link follows itself
					activate(item, index, 'key');
				}
				return;
			case 'ArrowRight':
				if (item?.items && !item.disabled) {
					e.preventDefault();
					return activate(item, index, 'key');
				}
				return onkey?.(e);
			case 'ArrowLeft':
				if (sub) {
					e.preventDefault();
					return onclose({ all: false, returnFocus: true });
				}
				return onkey?.(e);
			case 'Escape':
				e.preventDefault();
				e.stopPropagation();
				return onclose({ all: !sub, returnFocus: true });
			case 'Tab':
				return onclose({ all: true, returnFocus: false });
		}
		if (e.key.length === 1 && /\S/.test(e.key) && !e.metaKey && !e.ctrlKey && !e.altKey) {
			// Type to jump to an item.
			typed += e.key.toLowerCase();
			clearTimeout(typedTimer);
			typedTimer = setTimeout(() => (typed = ''), 500);
			const start = typed.length === 1 ? current + 1 : Math.max(current, 0);
			const order = [...list.slice(start), ...list.slice(0, start)];
			order.find((el) => el.dataset.label?.toLowerCase().startsWith(typed))?.focus();
		}
	}

	function hover(item: MenuItem, index: number, el: HTMLElement) {
		if (item.disabled) return;
		el.focus({ preventScroll: true });
		clearTimeout(hoverTimer);
		if (item.items) hoverTimer = setTimeout(() => activate(item, index, 'pointer'), 120);
		else if (openSub !== null) hoverTimer = setTimeout(() => (openSub = null), 120);
	}

	const role = (item: MenuItem) => (item.type === 'checkbox' ? 'menuitemcheckbox' : item.type === 'radio' ? 'menuitemradio' : 'menuitem');
</script>

<div
	bind:this={menu}
	class="tern-menu-list {klass}"
	role="menu"
	{id}
	aria-labelledby={labelledby}
	aria-label={labelledby ? undefined : label}
	tabindex="-1"
	popover="manual"
	{onkeydown}
>
	{#each groups as group, g (g)}
		{#snippet entriesOf()}
			{#each group.entries as { item, index } (index)}
				{#if item.separator}
					<div class="separator" role="separator"></div>
				{:else}
					<svelte:element
						this={item.href && !item.disabled && !item.items ? 'a' : 'div'}
						bind:this={itemEls[index]}
						href={item.href && !item.disabled && !item.items ? item.href : undefined}
						role={role(item)}
						tabindex="-1"
						class="item"
						class:danger={item.danger}
						data-tern-menuitem={index}
						data-label={item.label}
						aria-disabled={item.disabled || undefined}
						aria-checked={item.type ? !!item.checked : undefined}
						aria-haspopup={item.items ? 'menu' : undefined}
						aria-expanded={item.items ? openSub === index : undefined}
						aria-controls={item.items && openSub === index ? `${id}-sub-${index}` : undefined}
						onclick={(e: MouseEvent) => {
							if (item.href && !item.items && !item.disabled) return onclose({ all: true, returnFocus: false });
							e.preventDefault();
							activate(item, index, 'pointer');
						}}
						onpointerenter={(e: PointerEvent) => hover(item, index, e.currentTarget as HTMLElement)}
						onpointerleave={() => clearTimeout(hoverTimer)}
					>
						<span class="check" aria-hidden="true">
							{#if item.type === 'checkbox' && item.checked}
								<svg viewBox="0 0 16 16" width="14" height="14"><path d="M3.5 8.5l3 3 6-7" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" /></svg>
							{:else if item.type === 'radio' && item.checked}
								<svg viewBox="0 0 16 16" width="14" height="14"><circle cx="8" cy="8" r="3" fill="currentColor" /></svg>
							{/if}
						</span>
						<span class="label">{item.label}</span>
						{#if item.shortcut}<kbd class="shortcut">{item.shortcut}</kbd>{/if}
						{#if item.items}
							<svg class="chevron" aria-hidden="true" viewBox="0 0 16 16" width="14" height="14"><path d="M6 4l4 4-4 4" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" /></svg>
						{/if}
					</svelte:element>
					{#if item.items && openSub === index && itemEls[index]}
						<MenuList
							items={item.items}
							id="{id}-sub-{index}"
							label={item.label}
							anchor={itemEls[index]}
							side="right"
							focus={subFocus}
							sub
							onclose={(how) => {
								openSub = null;
								if (how.all) onclose(how);
								else if (how.returnFocus) itemEls[index]?.focus();
							}}
						/>
					{/if}
				{/if}
			{/each}
		{/snippet}
		{#if group.heading !== undefined}
			<div role="group" aria-labelledby="{id}-group-{g}">
				<div class="heading" id="{id}-group-{g}">{group.heading}</div>
				{@render entriesOf()}
			</div>
		{:else}
			{@render entriesOf()}
		{/if}
	{/each}
</div>

<style>
	.tern-menu-list {
		min-width: 12rem;
		max-height: calc(100vh - 1rem);
		overflow: auto;
		padding: 0.3rem;
		color: var(--tern-fg);
		background: var(--tern-surface);
		border: 1px solid var(--tern-border);
		border-radius: var(--tern-radius);
		box-shadow: var(--tern-shadow-lg);
		font-size: 0.875rem;
		outline: none;
		animation: tern-menu-in var(--tern-duration) ease-out;
	}
	.item {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.4rem 0.6rem 0.4rem 0.35rem;
		border-radius: calc(var(--tern-radius-sm) - 2px);
		color: var(--tern-fg);
		text-decoration: none;
		cursor: pointer;
		outline: none;
		user-select: none;
	}
	.item:focus,
	.item[aria-expanded='true'] {
		background: var(--tern-accent-soft);
	}
	.item.danger {
		color: var(--tern-danger);
	}
	.item.danger:focus {
		background: var(--tern-danger-soft);
	}
	.item[aria-disabled='true'] {
		opacity: 0.45;
		cursor: not-allowed;
		background: none;
	}
	.check {
		display: inline-flex;
		width: 1rem;
		justify-content: center;
		color: var(--tern-accent);
	}
	.label {
		flex: 1;
	}
	.shortcut {
		font-family: var(--tern-font-mono);
		font-size: 0.75rem;
		color: var(--tern-muted);
		letter-spacing: 0.04em;
	}
	.chevron {
		color: var(--tern-muted);
	}
	.heading {
		padding: 0.45rem 0.6rem 0.25rem 1.85rem;
		font-size: 0.75rem;
		font-weight: 600;
		color: var(--tern-muted);
	}
	.separator {
		height: 1px;
		margin: 0.3rem 0.2rem;
		background: var(--tern-border);
	}
	@keyframes tern-menu-in {
		from {
			opacity: 0;
			transform: scale(0.97);
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.tern-menu-list {
			animation: none;
		}
	}
</style>

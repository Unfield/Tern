<script lang="ts">
	import { matches } from './options.js';
	import { trapTab } from './floating.js';

	export type CommandItem = {
		label: string;
		onselect: () => void;
		/** Groups commands under this heading. */
		group?: string;
		/** Other words that find it, such as ["preferences"] for Settings. */
		keywords?: string[];
		/** A shortcut to show, such as "⌘S". */
		shortcut?: string;
		disabled?: boolean;
	};

	type Props = {
		items: CommandItem[];
		/** Names the palette for screen readers, such as "Commands". */
		label?: string;
		placeholder?: string;
		empty?: string;
		/** Show it as a modal palette, opened with open or the hotkey. */
		dialog?: boolean;
		/** Whether the palette is open, with dialog; bind it. */
		open?: boolean;
		/** With dialog: the key that toggles it with ⌘ or Ctrl, such as "k". */
		hotkey?: string;
		class?: string;
	};
	let { items, label = 'Commands', placeholder = 'Type a command or search…', empty = 'No results', dialog = false, open = $bindable(false), hotkey, class: klass = '' }: Props = $props();

	const uid = $props.id();
	const id = `tern-command-${uid}`;
	let query = $state('');
	let active = $state(0);
	let input = $state<HTMLInputElement>();
	let modal = $state<HTMLDialogElement>();

	const shown = $derived(query ? items.filter((it) => matches(`${it.label} ${(it.keywords ?? []).join(' ')} ${it.group ?? ''}`, query)) : items);
	const groups = $derived.by(() => {
		const out: { group?: string; entries: { it: CommandItem; i: number }[] }[] = [];
		shown.forEach((it, i) => {
			const g = out.find((x) => x.group === it.group);
			if (g) g.entries.push({ it, i });
			else out.push({ group: it.group, entries: [{ it, i }] });
		});
		return out;
	});
	// The order on screen, which grouping may change.
	const order = $derived(groups.flatMap((g) => g.entries.map((e) => e.i)).filter((i) => !shown[i]!.disabled));

	$effect(() => {
		void query;
		active = order[0] ?? -1;
	});

	function run(i: number) {
		const it = shown[i];
		if (!it || it.disabled) return;
		if (dialog) open = false;
		query = '';
		it.onselect();
	}

	function onkeydown(e: KeyboardEvent) {
		const pos = order.indexOf(active);
		const go = (p: number) => {
			e.preventDefault();
			if (!order.length) return;
			active = order[(p + order.length) % order.length]!;
			document.getElementById(`${id}-item-${active}`)?.scrollIntoView?.({ block: 'nearest' });
		};
		if (e.key === 'ArrowDown') go(pos + 1);
		else if (e.key === 'ArrowUp') go(pos - 1);
		else if (e.key === 'Home' && e.ctrlKey) go(0);
		else if (e.key === 'End' && e.ctrlKey) go(order.length - 1);
		else if (e.key === 'Enter') {
			e.preventDefault();
			run(active);
		}
	}

	// The hotkey toggles the palette from anywhere on the page.
	$effect(() => {
		if (!dialog || !hotkey) return;
		const key = (e: KeyboardEvent) => {
			if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === hotkey.toLowerCase()) {
				e.preventDefault();
				open = !open;
			}
		};
		window.addEventListener('keydown', key);
		return () => window.removeEventListener('keydown', key);
	});

	$effect(() => {
		if (!dialog || !modal) return;
		if (open && !modal.open) {
			modal.showModal();
			input?.focus();
		} else if (!open && modal.open) modal.close();
	});
</script>

{#snippet palette()}
	<div class="tern-command {klass}" class:in-dialog={dialog}>
		<div class="search">
			<svg aria-hidden="true" viewBox="0 0 16 16" width="16" height="16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"><circle cx="7" cy="7" r="4.5" /><path d="M10.5 10.5L14 14" /></svg>
			<input
				bind:this={input}
				bind:value={query}
				type="text"
				role="combobox"
				aria-label={label}
				aria-expanded="true"
				aria-controls="{id}-list"
				aria-autocomplete="list"
				aria-activedescendant={active >= 0 ? `${id}-item-${active}` : undefined}
				autocomplete="off"
				spellcheck="false"
				{placeholder}
				{onkeydown}
			/>
		</div>
		<div class="list" id="{id}-list" role="listbox" aria-label={label}>
			{#each groups as g, gi (gi)}
				{#snippet entries()}
					{#each g.entries as { it, i } (i)}
						<!-- svelte-ignore a11y_click_events_have_key_events -->
						<div
							id="{id}-item-{i}"
							class="item"
							class:active={i === active}
							role="option"
							tabindex="-1"
							aria-selected={i === active}
							aria-disabled={it.disabled || undefined}
							onpointerdown={(e) => e.preventDefault()}
							onpointermove={() => !it.disabled && (active = i)}
							onclick={() => run(i)}
						>
							<span>{it.label}</span>
							{#if it.shortcut}<kbd>{it.shortcut}</kbd>{/if}
						</div>
					{/each}
				{/snippet}
				{#if g.group}
					<div role="group" aria-labelledby="{id}-group-{gi}">
						<div class="heading" id="{id}-group-{gi}" role="presentation">{g.group}</div>
						{@render entries()}
					</div>
				{:else}
					{@render entries()}
				{/if}
			{/each}
			{#if shown.length === 0}<div class="empty" role="status">{empty}</div>{/if}
		</div>
	</div>
{/snippet}

{#if dialog}
	<dialog
		bind:this={modal}
		class="tern-command-dialog"
		aria-label={label}
		onclose={() => {
			open = false;
			query = '';
		}}
		onclick={(e) => e.target === modal && (open = false)}
		onkeydown={(e) => modal && trapTab(modal, e)}
	>
		{#if open}{@render palette()}{/if}
	</dialog>
{:else}
	{@render palette()}
{/if}

<style>
	.tern-command {
		display: flex;
		flex-direction: column;
		overflow: hidden;
		color: var(--tern-fg);
		background: var(--tern-surface);
		border: 1px solid var(--tern-border);
		border-radius: var(--tern-radius);
		box-shadow: var(--tern-shadow);
		font-size: 0.875rem;
	}
	.in-dialog {
		box-shadow: var(--tern-shadow-lg);
		border-radius: calc(var(--tern-radius) + 4px);
	}
	.search {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0 0.85rem;
		border-bottom: 1px solid var(--tern-border);
		color: var(--tern-muted);
	}
	input {
		flex: 1;
		height: 2.75rem;
		font: inherit;
		color: var(--tern-fg);
		background: transparent;
		border: 0;
		outline: none;
	}
	input::placeholder {
		color: var(--tern-muted);
	}
	.search:has(input:focus-visible) {
		box-shadow: inset 0 -2px 0 var(--tern-accent);
	}
	.list {
		max-height: 20rem;
		overflow: auto;
		padding: 0.3rem;
	}
	.item {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 1rem;
		padding: 0.5rem 0.65rem;
		border-radius: calc(var(--tern-radius-sm) - 2px);
		cursor: pointer;
	}
	.item.active {
		background: var(--tern-accent-soft);
	}
	.item[aria-disabled='true'] {
		opacity: 0.45;
		cursor: not-allowed;
	}
	kbd {
		font-family: var(--tern-font-mono);
		font-size: 0.75rem;
		color: var(--tern-muted);
	}
	.heading {
		padding: 0.5rem 0.65rem 0.25rem;
		font-size: 0.75rem;
		font-weight: 600;
		color: var(--tern-muted);
	}
	.empty {
		padding: 1.5rem;
		text-align: center;
		color: var(--tern-muted);
	}
	.tern-command-dialog {
		width: min(36rem, calc(100vw - 2rem));
		margin: 12vh auto auto;
		padding: 0;
		border: 0;
		background: transparent;
		overflow: visible;
	}
	.tern-command-dialog::backdrop {
		background: oklch(0.2 0.02 240 / 0.45);
		backdrop-filter: blur(2px);
	}
</style>

<script lang="ts">
	import { getField } from './field.js';
	import { float } from './floating.js';
	import { grouped, type Option } from './options.js';

	type Props = {
		options: Option[];
		/** The chosen value; bind it. */
		value?: string | null;
		/** Shown when nothing is chosen. */
		placeholder?: string;
		/** Labels it when it isn't in a Field. */
		label?: string;
		/** Posts the value under this name. */
		name?: string;
		disabled?: boolean;
		class?: string;
		onchange?: (value: string, option: Option) => void;
	};
	let { options, value = $bindable(null), placeholder = 'Choose…', label, name, disabled = false, class: klass = '', onchange }: Props = $props();

	const uid = $props.id();
	const id = `tern-selectmenu-${uid}`;
	const field = getField();
	field?.custom();
	let button = $state<HTMLButtonElement>();
	let list = $state<HTMLDivElement>();
	let open = $state(false);
	let active = $state(-1);
	let typed = '';
	let typedTimer: ReturnType<typeof setTimeout> | undefined;

	const selectedIndex = $derived(options.findIndex((o) => o.value === value));
	const selected = $derived(options[selectedIndex]);
	const enabledIdx = $derived(options.map((o, i) => (o.disabled ? -1 : i)).filter((i) => i >= 0));

	function show() {
		if (disabled || field?.disabled) return;
		active = selectedIndex >= 0 && !options[selectedIndex]?.disabled ? selectedIndex : (enabledIdx[0] ?? -1);
		open = true;
	}
	function choose(i: number) {
		const o = options[i];
		if (!o || o.disabled) return;
		value = o.value;
		open = false;
		onchange?.(o.value, o);
		button?.focus();
	}
	function step(by: number | 'first' | 'last') {
		const list = enabledIdx;
		if (!list.length) return;
		const pos = list.indexOf(active);
		const next = by === 'first' ? 0 : by === 'last' ? list.length - 1 : Math.max(0, Math.min(list.length - 1, (pos < 0 ? 0 : pos) + by));
		active = list[next]!;
		document.getElementById(`${id}-opt-${active}`)?.scrollIntoView?.({ block: 'nearest' });
	}

	function onkeydown(e: KeyboardEvent) {
		const opening = ['ArrowDown', 'ArrowUp', 'Enter', ' ', 'Home', 'End'];
		if (!open) {
			if (opening.includes(e.key)) {
				e.preventDefault();
				show();
				if (e.key === 'Home') step('first');
				if (e.key === 'End') step('last');
				return;
			}
		} else {
			switch (e.key) {
				case 'ArrowDown':
					e.preventDefault();
					return e.altKey ? choose(active) : step(1);
				case 'ArrowUp':
					e.preventDefault();
					return e.altKey ? choose(active) : step(-1);
				case 'Home':
					e.preventDefault();
					return step('first');
				case 'End':
					e.preventDefault();
					return step('last');
				case 'PageDown':
					e.preventDefault();
					return step(10);
				case 'PageUp':
					e.preventDefault();
					return step(-10);
				case 'Enter':
				case ' ':
					if (e.key === ' ' && typed) break; // a space inside a typed name
					e.preventDefault();
					return choose(active);
				case 'Escape':
					e.preventDefault();
					e.stopPropagation();
					open = false;
					return;
				case 'Tab':
					open = false;
					return;
			}
		}
		if (e.key.length === 1 && !e.metaKey && !e.ctrlKey && !e.altKey) {
			// Type to jump, open or closed, like a native select.
			e.preventDefault();
			typed += e.key.toLowerCase();
			clearTimeout(typedTimer);
			typedTimer = setTimeout(() => (typed = ''), 500);
			const from = open ? active : selectedIndex;
			const start = typed.length === 1 ? from + 1 : Math.max(from, 0);
			const order = [...enabledIdx.filter((i) => i >= start), ...enabledIdx.filter((i) => i < start)];
			const hit = order.find((i) => options[i]!.label.toLowerCase().startsWith(typed));
			if (hit === undefined) return;
			if (open) active = hit;
			else choose(hit);
		}
	}

	$effect(() => {
		if (!open || !list || !button) return;
		list.showPopover?.();
		const stop = float(list, button, { side: 'bottom', align: 'start', offset: 4, matchWidth: true });
		document.getElementById(`${id}-opt-${active}`)?.scrollIntoView?.({ block: 'nearest' });
		const away = (e: PointerEvent) => {
			const t = e.target as Node;
			if (!list?.contains(t) && !button?.contains(t)) open = false;
		};
		document.addEventListener('pointerdown', away, true);
		return () => {
			stop();
			document.removeEventListener('pointerdown', away, true);
		};
	});
</script>

<button
	bind:this={button}
	type="button"
	class="tern-selectmenu {klass}"
	role="combobox"
	id={field?.id ?? id}
	aria-haspopup="listbox"
	aria-expanded={open}
	aria-controls="{id}-list"
	aria-activedescendant={open && active >= 0 ? `${id}-opt-${active}` : undefined}
	aria-label={field ? undefined : label}
	aria-labelledby={field ? `${field.labelId} ${id}-value` : undefined}
	aria-describedby={field?.describedBy}
	aria-invalid={field?.invalid || undefined}
	aria-required={field?.required || undefined}
	disabled={disabled || field?.disabled}
	onclick={() => (open ? (open = false) : show())}
	{onkeydown}
	onblur={(e) => !list?.contains(e.relatedTarget as Node) && (open = false)}
>
	<span class="value" class:placeholder={!selected} id="{id}-value">{selected?.label ?? placeholder}</span>
	<svg aria-hidden="true" viewBox="0 0 16 16" width="14" height="14"><path d="M5 6.5l3-3 3 3M5 9.5l3 3 3-3" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" /></svg>
</button>
{#if name}<input type="hidden" {name} value={value ?? ''} />{/if}
<div bind:this={list} id="{id}-list" class="tern-selectmenu-list" class:open role="listbox" aria-label={label} aria-labelledby={field?.labelId} tabindex="-1" popover="manual">
	{#if open}
		{#each grouped(options) as g, gi (gi)}
			{#snippet items()}
				{#each g.options as o (o.value)}
					{@const i = options.indexOf(o)}
					<!-- svelte-ignore a11y_click_events_have_key_events -->
					<div
						id="{id}-opt-{i}"
						class="option"
						class:active={i === active}
						role="option"
						tabindex="-1"
						aria-selected={o.value === value}
						aria-disabled={o.disabled || undefined}
						onpointerdown={(e) => e.preventDefault()}
						onpointermove={() => !o.disabled && (active = i)}
						onclick={() => choose(i)}
					>
						<span class="check" aria-hidden="true">
							{#if o.value === value}<svg viewBox="0 0 16 16" width="14" height="14"><path d="M3.5 8.5l3 3 6-7" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" /></svg>{/if}
						</span>
						<span class="text">
							<span>{o.label}</span>
							{#if o.description}<span class="description">{o.description}</span>{/if}
						</span>
					</div>
				{/each}
			{/snippet}
			{#if g.group}
				<div role="group" aria-labelledby="{id}-group-{gi}">
					<div class="heading" id="{id}-group-{gi}" role="presentation">{g.group}</div>
					{@render items()}
				</div>
			{:else}
				{@render items()}
			{/if}
		{/each}
	{/if}
</div>

<style>
	.tern-selectmenu {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.5rem;
		width: 100%;
		height: var(--tern-control-height);
		padding: 0 0.6rem 0 0.75rem;
		font: inherit;
		font-size: 0.875rem;
		color: var(--tern-fg);
		text-align: start;
		background: var(--tern-surface);
		border: 1px solid var(--tern-border-strong);
		border-radius: var(--tern-radius-sm);
		cursor: pointer;
		transition:
			border-color var(--tern-duration),
			box-shadow var(--tern-duration);
	}
	.tern-selectmenu svg {
		flex: none;
		color: var(--tern-muted);
	}
	.tern-selectmenu:hover:not(:disabled) {
		border-color: var(--tern-muted);
	}
	.tern-selectmenu:focus-visible {
		outline: none;
		border-color: var(--tern-accent);
		box-shadow: 0 0 0 3px color-mix(in oklch, var(--tern-ring) 30%, transparent);
	}
	.tern-selectmenu[aria-invalid='true'] {
		border-color: var(--tern-danger);
	}
	.tern-selectmenu:disabled {
		opacity: 0.6;
		cursor: not-allowed;
	}
	.value {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.placeholder {
		color: var(--tern-muted);
	}
	.tern-selectmenu-list {
		max-height: 18rem;
		overflow: auto;
		padding: 0.3rem;
		color: var(--tern-fg);
		background: var(--tern-surface);
		border: 1px solid var(--tern-border);
		border-radius: var(--tern-radius);
		box-shadow: var(--tern-shadow-lg);
		font-size: 0.875rem;
		outline: none;
	}
	.tern-selectmenu-list:not(.open) {
		display: none;
	}
	.option {
		display: flex;
		align-items: flex-start;
		gap: 0.4rem;
		padding: 0.4rem 0.6rem 0.4rem 0.35rem;
		border-radius: calc(var(--tern-radius-sm) - 2px);
		cursor: pointer;
	}
	.option.active {
		background: var(--tern-accent-soft);
	}
	.option[aria-disabled='true'] {
		opacity: 0.45;
		cursor: not-allowed;
	}
	.check {
		display: inline-flex;
		width: 1rem;
		padding-top: 0.15rem;
		justify-content: center;
		color: var(--tern-accent);
	}
	.text {
		display: grid;
	}
	.description {
		font-size: 0.8125rem;
		color: var(--tern-muted);
	}
	.heading {
		padding: 0.45rem 0.6rem 0.25rem 1.75rem;
		font-size: 0.75rem;
		font-weight: 600;
		color: var(--tern-muted);
	}
</style>

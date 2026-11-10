<script lang="ts">
	import { getField } from './field.js';
	import { float } from './floating.js';
	import { grouped, matches, type Option } from './options.js';
	import Spinner from './Spinner.svelte';

	type Props = {
		/** The options. With onsearch, the options for the current query. */
		options: Option[];
		/** The chosen option's value; bind it. */
		value?: string | null;
		placeholder?: string;
		/** Labels it when it isn't in a Field. */
		label?: string;
		/** Posts the chosen value under this name. */
		name?: string;
		/** Shown when nothing matches. */
		empty?: string;
		/** Searches elsewhere, such as an endpoint, instead of filtering options. Set options from it. */
		onsearch?: (query: string) => void | Promise<unknown>;
		/** Milliseconds to wait after typing before onsearch. */
		debounce?: number;
		disabled?: boolean;
		class?: string;
		onchange?: (value: string | null, option: Option | null) => void;
	};
	const field = getField();
	let {
		options,
		value = $bindable(field?.value ?? null),
		placeholder,
		label,
		name: nameProp,
		empty = 'No results',
		onsearch,
		debounce = 200,
		disabled = false,
		class: klass = '',
		onchange
	}: Props = $props();
	const name = $derived(nameProp ?? field?.name);

	const uid = $props.id();
	const id = `tern-combobox-${uid}`;
	let input = $state<HTMLInputElement>();
	let list = $state<HTMLDivElement>();
	let open = $state(false);
	let query = $state('');
	let typing = $state(false);
	let active = $state<string | null>(null);
	let searching = $state(false);
	let timer: ReturnType<typeof setTimeout> | undefined;
	// Options the user has seen, so the chosen one's label survives a new search.
	const known = new Map<string, Option>();

	$effect(() => {
		for (const o of options) known.set(o.value, o);
	});
	const selected = $derived(value === null ? null : (options.find((o) => o.value === value) ?? known.get(value) ?? null));
	const shown = $derived(onsearch || !typing || !query ? options : options.filter((o) => matches(`${o.label} ${o.description ?? ''}`, query)));
	const enabled = $derived(shown.filter((o) => !o.disabled));
	const text = $derived(typing ? query : (selected?.label ?? ''));

	function show() {
		if (disabled || field?.disabled) return;
		open = true;
		// Open on the chosen option, else the first.
		active = selected && !selected.disabled && enabled.includes(selected) ? selected.value : (enabled[0]?.value ?? null);
	}
	function hide() {
		open = false;
		typing = false;
	}
	function choose(o: Option) {
		if (o.disabled) return;
		value = o.value;
		known.set(o.value, o);
		hide();
		onchange?.(o.value, o);
		input?.focus();
	}
	function move(by: number | 'first' | 'last') {
		if (!open) return show();
		const list = enabled;
		if (list.length === 0) return;
		const i = list.findIndex((o) => o.value === active);
		const next = by === 'first' ? 0 : by === 'last' ? list.length - 1 : i < 0 ? (by > 0 ? 0 : list.length - 1) : Math.max(0, Math.min(list.length - 1, i + by));
		active = list[next]!.value;
		document.getElementById(`${id}-opt-${next}`)?.scrollIntoView?.({ block: 'nearest' });
	}

	function oninput() {
		query = input!.value;
		typing = true;
		open = true;
		if (query === '' && value !== null) {
			value = null;
			onchange?.(null, null);
		}
		if (onsearch) {
			searching = true; // until the results arrive, don't claim there are none
			clearTimeout(timer);
			timer = setTimeout(async () => {
				searching = true;
				try {
					await onsearch(query);
				} finally {
					searching = false;
				}
			}, debounce);
		}
	}
	$effect(() => {
		// Keep the active option valid as the list changes.
		if (open && !enabled.some((o) => o.value === active)) active = enabled[0]?.value ?? null;
	});

	function onkeydown(e: KeyboardEvent) {
		switch (e.key) {
			case 'ArrowDown':
				e.preventDefault();
				return e.altKey ? show() : move(1);
			case 'ArrowUp':
				e.preventDefault();
				return move(-1);
			case 'PageDown':
				e.preventDefault();
				return move(10);
			case 'PageUp':
				e.preventDefault();
				return move(-10);
			case 'Enter': {
				if (!open) return;
				const o = enabled.find((x) => x.value === active);
				if (o) {
					e.preventDefault();
					choose(o);
				}
				return;
			}
			case 'Escape':
				if (open) {
					e.preventDefault();
					e.stopPropagation();
					hide();
				} else if (value !== null) {
					e.preventDefault();
					value = null;
					onchange?.(null, null);
				}
				return;
			case 'Tab':
				return hide();
		}
	}

	$effect(() => {
		if (!open || !list || !input) return;
		list.showPopover?.();
		const stop = float(list, input.parentElement ?? input, { side: 'bottom', align: 'start', offset: 4, matchWidth: true });
		const away = (e: PointerEvent) => {
			const t = e.target as Node;
			if (!list?.contains(t) && !input?.parentElement?.contains(t)) hide();
		};
		document.addEventListener('pointerdown', away, true);
		return () => {
			stop();
			document.removeEventListener('pointerdown', away, true);
		};
	});

	const activeIndex = $derived(shown.findIndex((o) => o.value === active));
</script>

<div class="tern-combobox {klass}">
	<input
		bind:this={input}
		class="input"
		type="text"
		role="combobox"
		id={field?.id ?? id}
		value={text}
		{placeholder}
		aria-label={field ? undefined : label}
		aria-describedby={field?.describedBy}
		aria-invalid={field?.invalid || undefined}
		aria-required={field?.required || undefined}
		aria-expanded={open}
		aria-controls="{id}-list"
		aria-autocomplete="list"
		aria-activedescendant={open && activeIndex >= 0 ? `${id}-opt-${activeIndex}` : undefined}
		autocomplete="off"
		spellcheck="false"
		disabled={disabled || field?.disabled}
		{oninput}
		{onkeydown}
		onclick={() => (open ? hide() : show())}
		onblur={(e) => {
			if (!list?.contains(e.relatedTarget as Node)) typing = false;
		}}
	/>
	<span class="icon" aria-hidden="true">
		{#if searching}<Spinner size="0.875rem" label="Searching" />{:else}<svg viewBox="0 0 16 16" width="14" height="14"><path d="M5 6.5l3-3 3 3M5 9.5l3 3 3-3" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" /></svg>{/if}
	</span>
	{#if name}<input type="hidden" {name} value={value ?? ''} />{/if}
	<div bind:this={list} id="{id}-list" class="list" role="listbox" aria-label={label} aria-labelledby={field?.labelId} popover="manual" class:open>
		{#if open}
			{#each grouped(shown) as g, gi (gi)}
				{#snippet items()}
					{#each g.options as o (o.value)}
						{@const i = shown.indexOf(o)}
						<!-- svelte-ignore a11y_click_events_have_key_events -->
						<div
							id="{id}-opt-{i}"
							class="option"
							class:active={o.value === active}
							role="option"
							tabindex="-1"
							aria-selected={o.value === value}
							aria-disabled={o.disabled || undefined}
							onpointerdown={(e) => e.preventDefault()}
							onpointermove={() => !o.disabled && (active = o.value)}
							onclick={() => choose(o)}
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
			{:else}
				<div class="empty" role="status">{searching ? 'Searching…' : empty}</div>
			{/each}
		{/if}
	</div>
</div>

<style>
	.tern-combobox {
		position: relative;
		display: flex;
		align-items: center;
	}
	.input {
		width: 100%;
		box-sizing: border-box;
		height: var(--tern-control-height);
		padding: 0 2rem 0 0.75rem;
		font: inherit;
		font-size: 0.875rem;
		color: var(--tern-fg);
		background: var(--tern-surface);
		border: 1px solid var(--tern-border-strong);
		border-radius: var(--tern-radius-sm);
		transition:
			border-color var(--tern-duration),
			box-shadow var(--tern-duration);
	}
	.input::placeholder {
		color: var(--tern-muted);
	}
	.input:hover:not(:disabled) {
		border-color: var(--tern-muted);
	}
	.input:focus-visible {
		outline: none;
		border-color: var(--tern-accent);
		box-shadow: 0 0 0 3px color-mix(in oklch, var(--tern-ring) 30%, transparent);
	}
	.input[aria-invalid='true'] {
		border-color: var(--tern-danger);
	}
	.input:disabled {
		opacity: 0.6;
		cursor: not-allowed;
	}
	.icon {
		position: absolute;
		right: 0.6rem;
		display: inline-flex;
		color: var(--tern-muted);
		pointer-events: none;
	}
	.list {
		max-height: 18rem;
		overflow: auto;
		padding: 0.3rem;
		color: var(--tern-fg);
		background: var(--tern-surface);
		border: 1px solid var(--tern-border);
		border-radius: var(--tern-radius);
		box-shadow: var(--tern-shadow-lg);
		font-size: 0.875rem;
	}
	.list:not(.open) {
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
	.empty {
		padding: 0.75rem;
		text-align: center;
		color: var(--tern-muted);
	}
</style>

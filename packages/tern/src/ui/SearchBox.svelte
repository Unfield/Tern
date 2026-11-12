<script lang="ts">
	import { tb } from '../runtime/i18n.svelte.js';
	import { goto } from '../runtime/router.svelte.js';
	import Highlight from './Highlight.svelte';
	import Spinner from './Spinner.svelte';
	import type { SearchItem } from './search.js';

	type Props = {
		/** The results page the form goes to on Enter, such as "/search"; without JavaScript, the only way. */
		action?: string;
		/** The query's name in the results page's address. */
		name?: string;
		/** The query; bind it. */
		value?: string;
		/** Results as the user types, such as from a typed endpoint. Without it, the box is a plain search form. */
		search?: (query: string, signal: AbortSignal) => Promise<SearchItem[]>;
		/** Milliseconds to wait after typing before searching. */
		debounce?: number;
		/** The fewest characters to search for. */
		minLength?: number;
		/** A key that focuses the box from anywhere on the page, such as "/". */
		hotkey?: string;
		label?: string;
		placeholder?: string;
		/** Runs when a result is chosen; return false to stay on the page. */
		onselect?: (item: SearchItem) => void | boolean;
		class?: string;
	};
	let {
		action,
		name = 'q',
		value = $bindable(''),
		search,
		debounce = 150,
		minLength = 2,
		hotkey,
		label: labelProp,
		placeholder: placeholderProp,
		onselect,
		class: klass = ''
	}: Props = $props();
	const label = $derived(labelProp ?? tb('tern.ui.search.label'));
	const placeholder = $derived(placeholderProp ?? tb('tern.ui.search.placeholder'));

	const uid = $props.id();
	const id = `tern-search-${uid}`;
	let input = $state<HTMLInputElement>();
	let form = $state<HTMLFormElement>();
	let items = $state<SearchItem[]>([]);
	let searched = $state('');
	let active = $state(-1);
	let open = $state(false);
	let searching = $state(false);
	let failed = $state(false);
	let timer: ReturnType<typeof setTimeout> | undefined;
	let controller: AbortController | undefined;

	const query = $derived(value.trim());
	const showAll = $derived(!!action && items.length > 0);
	const count = $derived(items.length + (showAll ? 1 : 0));

	function run() {
		clearTimeout(timer);
		controller?.abort();
		if (!search || query.length < minLength) {
			items = [];
			searched = '';
			searching = false;
			open = false;
			return;
		}
		const q = query;
		timer = setTimeout(async () => {
			const c = (controller = new AbortController());
			searching = true;
			failed = false;
			try {
				const got = await search!(q, c.signal);
				if (c.signal.aborted) return;
				items = got;
				searched = q;
				active = -1;
				open = true;
			} catch (e) {
				if (c.signal.aborted) return;
				items = [];
				searched = q;
				failed = true;
				open = true;
			} finally {
				if (!c.signal.aborted) searching = false;
			}
		}, debounce);
	}

	function choose(i: number) {
		if (i === items.length && showAll) {
			form?.requestSubmit();
			return;
		}
		const item = items[i];
		if (!item) return;
		open = false;
		if (onselect?.(item) === false) return;
		void goto(item.href);
	}

	function onkeydown(e: KeyboardEvent) {
		if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
			if (!open && items.length) open = true;
			if (!count) return;
			e.preventDefault();
			const step = e.key === 'ArrowDown' ? 1 : -1;
			active = active < 0 && step < 0 ? count - 1 : (active + step + count) % count;
		} else if (e.key === 'Enter' && open && active >= 0) {
			e.preventDefault();
			choose(active);
		} else if (e.key === 'Escape') {
			if (open) {
				e.preventDefault();
				open = false;
			} else if (value) {
				e.preventDefault();
				value = '';
				run();
			}
		}
	}

	$effect(() => {
		if (!hotkey) return;
		const onKey = (e: KeyboardEvent) => {
			const t = e.target instanceof Element ? e.target : null;
			if (e.key !== hotkey || e.metaKey || e.ctrlKey || e.altKey || t?.closest('input, textarea, select, [contenteditable]')) return;
			e.preventDefault();
			input?.focus();
		};
		addEventListener('keydown', onKey);
		return () => removeEventListener('keydown', onKey);
	});
	$effect(() => () => {
		clearTimeout(timer);
		controller?.abort();
	});
</script>

<form bind:this={form} class="tern-search {klass}" role="search" method="GET" {action} onsubmit={(e) => !action && e.preventDefault()}>
	<span class="glass" aria-hidden="true">
		{#if searching}<Spinner size="0.875rem" />{:else}<svg viewBox="0 0 16 16" width="14" height="14"><circle cx="7" cy="7" r="4.5" fill="none" stroke="currentColor" stroke-width="1.5" /><path d="M10.5 10.5L14 14" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" /></svg>{/if}
	</span>
	<input
		bind:this={input}
		bind:value
		type="search"
		{name}
		{placeholder}
		aria-label={label}
		autocomplete="off"
		spellcheck="false"
		role={search ? 'combobox' : undefined}
		aria-expanded={search ? open : undefined}
		aria-controls={search ? `${id}-list` : undefined}
		aria-autocomplete={search ? 'list' : undefined}
		aria-activedescendant={open && active >= 0 ? `${id}-opt-${active}` : undefined}
		oninput={run}
		{onkeydown}
		onfocus={() => items.length && (open = true)}
		onblur={() => setTimeout(() => (open = false), 120)}
	/>
	{#if hotkey && !value}<kbd class="hotkey" aria-hidden="true">{hotkey}</kbd>{/if}
	{#if search}
		<div class="tern-sr-only" aria-live="polite">{open && !searching ? (failed ? tb('tern.ui.search.failed') : tb('tern.ui.search.results', { n: items.length })) : ''}</div>
		{#if open}
			<div id="{id}-list" class="list" role="listbox" aria-label={label}>
				{#if failed}
					<p class="note">{tb('tern.ui.search.failed')}</p>
				{:else if items.length === 0}
					<p class="note">{tb('tern.ui.search.empty', { query: searched })}</p>
				{:else}
					{#each items as item, i (item.href)}
						<a
							id="{id}-opt-{i}"
							class="item"
							class:active={i === active}
							role="option"
							aria-selected={i === active}
							href={item.href}
							tabindex="-1"
							onpointerdown={(e) => e.preventDefault()}
							onpointermove={() => (active = i)}
							onclick={(e) => {
								if (e.metaKey || e.ctrlKey || e.shiftKey || e.button !== 0) return;
								e.preventDefault();
								choose(i);
							}}
						>
							<span class="title"><Highlight text={item.title} />{#if item.meta}<span class="meta">{item.meta}</span>{/if}</span>
							{#if item.snippet?.length}<span class="snippet"><Highlight text={item.snippet} /></span>{/if}
						</a>
					{/each}
					{#if showAll}
						<button
							id="{id}-opt-{items.length}"
							type="submit"
							class="item all"
							class:active={active === items.length}
							role="option"
							aria-selected={active === items.length}
							tabindex="-1"
							onpointerdown={(e) => e.preventDefault()}
							onpointermove={() => (active = items.length)}>{tb('tern.ui.search.all', { query: searched })}</button
						>
					{/if}
				{/if}
			</div>
		{/if}
	{/if}
</form>

<style>
	.tern-search {
		position: relative;
		display: flex;
		align-items: center;
		width: 100%;
		max-width: 32rem;
		margin: 0;
	}
	.glass {
		position: absolute;
		left: 0.75rem;
		display: grid;
		place-items: center;
		color: var(--tern-muted);
		pointer-events: none;
	}
	input {
		box-sizing: border-box;
		width: 100%;
		height: var(--tern-control-height);
		padding: 0 2.25rem 0 2.25rem;
		border: 1px solid var(--tern-border);
		border-radius: var(--tern-radius-sm);
		background: var(--tern-bg);
		color: var(--tern-fg);
		font: inherit;
		font-size: 0.875rem;
	}
	input::-webkit-search-cancel-button {
		-webkit-appearance: none;
	}
	input:focus-visible {
		outline: none;
		border-color: var(--tern-accent);
		box-shadow: 0 0 0 3px var(--tern-ring);
	}
	.hotkey {
		position: absolute;
		right: 0.625rem;
		padding: 0 0.375rem;
		border: 1px solid var(--tern-border);
		border-radius: 4px;
		color: var(--tern-muted);
		font: 0.75rem/1.4 var(--tern-font-mono);
		pointer-events: none;
	}
	@media (pointer: coarse) {
		.hotkey {
			display: none;
		}
	}
	.list {
		box-sizing: border-box;
		overflow-x: hidden;
		position: absolute;
		top: calc(100% + 0.375rem);
		left: 0;
		right: 0;
		z-index: 50;
		max-height: min(26rem, 70vh);
		overflow-y: auto;
		padding: 0.25rem;
		border: 1px solid var(--tern-border);
		border-radius: var(--tern-radius);
		background: var(--tern-surface);
		box-shadow: var(--tern-shadow-lg);
	}
	.item {
		box-sizing: border-box;
		display: grid;
		gap: 0.125rem;
		width: 100%;
		padding: 0.5rem 0.625rem;
		border: 0;
		border-radius: var(--tern-radius-sm);
		background: none;
		color: var(--tern-fg);
		font: inherit;
		font-size: 0.875rem;
		text-align: start;
		text-decoration: none;
		cursor: pointer;
	}
	.item.active {
		background: var(--tern-accent-soft);
	}
	.item,
	.item:hover {
		text-decoration: none;
	}
	.title {
		display: flex;
		align-items: baseline;
		gap: 0.5rem;
		font-weight: 550;
		overflow: hidden;
	}
	.meta {
		margin-left: auto;
		flex: none;
		color: var(--tern-muted);
		font-size: 0.75rem;
		font-weight: 400;
	}
	.snippet {
		color: var(--tern-muted);
		font-size: 0.8125rem;
		overflow: hidden;
		display: -webkit-box;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		-webkit-box-orient: vertical;
	}
	.all {
		color: var(--tern-accent);
		font-weight: 550;
	}
	.note {
		margin: 0;
		padding: 0.75rem 0.625rem;
		color: var(--tern-muted);
		font-size: 0.875rem;
	}
</style>

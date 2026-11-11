<script lang="ts">
	import { goto } from '@ternkit/tern';
	import Icon from './Icon.svelte';

	type Entry = { title: string; section: string; url: string; text?: string };

	let dialog = $state<HTMLDialogElement>();
	let input = $state<HTMLInputElement>();
	let query = $state('');
	let entries = $state<Entry[] | null>(null);
	let failed = $state(false);
	let active = $state(0);

	export function show() {
		dialog?.showModal();
		query = '';
		active = 0;
		input?.focus();
		if (!entries) load();
	}

	async function load() {
		try {
			const res = await fetch('/search.json');
			if (!res.ok) throw new Error(String(res.status));
			entries = await res.json();
		} catch {
			failed = true;
		}
	}

	// Every word must appear; a match in the title counts most, then the
	// section, then the text.
	const results = $derived.by(() => {
		const words = query.toLowerCase().split(/\s+/).filter(Boolean);
		if (!entries || words.length === 0) return [];
		const scored: { e: Entry; score: number }[] = [];
		for (const e of entries) {
			const title = e.title.toLowerCase();
			const section = e.section.toLowerCase();
			const text = (e.text ?? '').toLowerCase();
			let score = 0;
			let all = true;
			for (const w of words) {
				if (title.startsWith(w)) score += 12;
				else if (title.includes(w)) score += 8;
				else if (section.includes(w)) score += 3;
				else if (text.includes(w)) score += 1;
				else {
					all = false;
					break;
				}
			}
			if (all) scored.push({ e, score: score + (e.text !== undefined && e.text !== '' ? 2 : 0) });
		}
		scored.sort((a, b) => b.score - a.score);
		return scored.slice(0, 12).map((s) => s.e);
	});

	$effect(() => {
		void query;
		active = 0;
	});

	function open(e: Entry) {
		dialog?.close();
		goto(e.url);
	}

	function onKey(ev: KeyboardEvent) {
		if (ev.key === 'ArrowDown') {
			ev.preventDefault();
			active = Math.min(active + 1, results.length - 1);
		} else if (ev.key === 'ArrowUp') {
			ev.preventDefault();
			active = Math.max(active - 1, 0);
		} else if (ev.key === 'Enter' && results[active]) {
			ev.preventDefault();
			open(results[active]);
		}
	}

	$effect(() => {
		const onGlobal = (ev: KeyboardEvent) => {
			const target = ev.target as HTMLElement | null;
			const typing = target && (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable);
			if ((ev.key === 'k' && (ev.metaKey || ev.ctrlKey)) || (ev.key === '/' && !typing)) {
				ev.preventDefault();
				show();
			}
		};
		window.addEventListener('keydown', onGlobal);
		return () => window.removeEventListener('keydown', onGlobal);
	});
</script>

<dialog bind:this={dialog} aria-label="Search the docs" onclick={(e) => e.target === dialog && dialog?.close()}>
	<div class="panel">
		<label class="field">
			<Icon name="search" size={18} />
			<input bind:this={input} bind:value={query} onkeydown={onKey} placeholder="Search the docs" autocomplete="off" spellcheck="false" aria-label="Search" aria-controls="search-results" />
			<button class="esc" type="button" onclick={() => dialog?.close()}>Esc</button>
		</label>
		<div class="body" id="search-results" role="listbox" aria-label="Results">
			{#if failed}
				<p class="hint">Search couldn't load. The docs are all in the sidebar.</p>
			{:else if !query}
				<p class="hint">Search titles and sections of every page. <span class="keys"><kbd>↑</kbd><kbd>↓</kbd> to move, <kbd>↵</kbd> to open</span></p>
			{:else if entries && results.length === 0}
				<p class="hint">Nothing matches “{query}”.</p>
			{:else}
				{#each results as r, i (r.url)}
					<a href={r.url} class="result" class:active={i === active} role="option" aria-selected={i === active} onmouseenter={() => (active = i)} onclick={(e) => { e.preventDefault(); open(r); }}>
						<span class="title">{r.title}</span>
						<span class="section">{r.section}</span>
						<Icon name="chevron-right" />
					</a>
				{/each}
			{/if}
		</div>
	</div>
</dialog>

<style>
	dialog {
		width: min(640px, calc(100vw - 2 * var(--gutter)));
		max-height: min(560px, calc(100vh - 120px));
		margin: 12vh auto auto;
		padding: 0;
		border: 1px solid var(--border-strong);
		border-radius: var(--radius-l);
		background: var(--surface);
		color: var(--text);
		box-shadow: var(--shadow-l);
		overflow: hidden;
	}
	dialog::backdrop {
		background: rgb(0 0 0 / 0.45);
		backdrop-filter: blur(2px);
	}
	dialog[open] {
		animation: pop 160ms var(--ease);
	}
	@keyframes pop {
		from {
			opacity: 0;
			transform: translateY(-6px) scale(0.98);
		}
	}
	.panel {
		display: flex;
		flex-direction: column;
		max-height: inherit;
	}
	.field {
		display: flex;
		align-items: center;
		gap: var(--s-3);
		padding: 0 var(--s-4);
		height: 56px;
		border-bottom: 1px solid var(--border);
		color: var(--text-3);
	}
	input {
		flex: 1;
		min-width: 0;
		height: 100%;
		border: 0;
		outline: 0;
		background: transparent;
		color: var(--text);
		font: inherit;
		font-size: 16px;
	}
	input::placeholder {
		color: var(--text-3);
	}
	.esc {
		padding: 2px 8px;
		border: 1px solid var(--border);
		border-radius: 6px;
		background: var(--surface-2);
		color: var(--text-3);
		font-size: 12px;
		cursor: pointer;
	}
	.body {
		overflow-y: auto;
		padding: var(--s-2);
	}
	.hint {
		padding: var(--s-6) var(--s-4);
		color: var(--text-3);
		font-size: 14px;
		text-align: center;
	}
	.keys {
		display: block;
		margin-top: var(--s-2);
	}
	kbd {
		display: inline-block;
		min-width: 20px;
		margin: 0 2px;
		padding: 0 5px;
		border: 1px solid var(--border);
		border-radius: 5px;
		font-family: var(--font);
		font-size: 12px;
	}
	.result {
		display: grid;
		grid-template-columns: 1fr auto;
		grid-template-rows: auto auto;
		column-gap: var(--s-3);
		align-items: center;
		padding: var(--s-3) var(--s-4);
		border-radius: var(--radius);
		color: var(--text);
	}
	.result :global(svg) {
		grid-row: 1 / 3;
		grid-column: 2;
		color: var(--text-3);
		opacity: 0;
	}
	.result.active {
		background: var(--accent-soft);
	}
	.result.active :global(svg) {
		opacity: 1;
		color: var(--accent);
	}
	.title {
		font-size: 15px;
		font-weight: 550;
	}
	.section {
		color: var(--text-3);
		font-size: 13px;
	}
</style>

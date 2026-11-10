<script lang="ts" generics="Row">
	import type { Snippet } from 'svelte';
	import Checkbox from './Checkbox.svelte';
	import Input from './Input.svelte';
	import Pagination from './Pagination.svelte';
	import type { Column, DataQuery } from './table.js';
	import { matches } from './options.js';


	type Props = {
		columns: Column<Row>[];
		/** All rows, or with onquery, the current page's. */
		rows: Row[];
		caption: string;
		hideCaption?: boolean;
		/** A stable key per row, for selection. */
		key: (row: Row) => string;
		/** Rows per page; 0 shows all. */
		pageSize?: number;
		/** Show a search box that filters rows. */
		searchable?: boolean;
		/** Checkboxes to select rows; bind the selected keys. */
		selectable?: boolean;
		selected?: string[];
		/** Sorting, searching and paging happen elsewhere, such as in a Go endpoint: this runs on each change, and you set rows and total. */
		onquery?: (q: DataQuery) => void;
		/** With onquery: how many rows match in all. */
		total?: number;
		/** Buttons for selected rows or the whole table, above it. */
		toolbar?: Snippet<[string[]]>;
		/** Actions at the end of each row. */
		actions?: Snippet<[Row]>;
		empty?: string;
		/** Dims the rows while new ones load, keeping the frame. */
		loading?: boolean;
		class?: string;
	};
	let {
		columns,
		rows,
		caption,
		hideCaption = false,
		key,
		pageSize = 10,
		searchable = false,
		selectable = false,
		selected = $bindable([]),
		onquery,
		total,
		toolbar,
		actions,
		empty = 'No rows match',
		loading = false,
		class: klass = ''
	}: Props = $props();

	const uid = $props.id();
	let sort = $state<DataQuery['sort']>(null);
	let search = $state('');
	let page = $state(1);

	const cell = (row: Row, col: Column<Row>) => (col.value ? col.value(row) : (row as Record<string, unknown>)[col.key]);
	const compare = (a: unknown, b: unknown) => {
		if (a == null) return b == null ? 0 : 1;
		if (b == null) return -1;
		if (a instanceof Date && b instanceof Date) return a.getTime() - b.getTime();
		if (typeof a === 'number' && typeof b === 'number') return a - b;
		return String(a).localeCompare(String(b), undefined, { numeric: true, sensitivity: 'base' });
	};

	const filtered = $derived(onquery || !search ? rows : rows.filter((r) => matches(columns.map((c) => String(cell(r, c) ?? '')).join(' '), search)));
	const sorted = $derived.by(() => {
		if (onquery || !sort) return filtered;
		const col = columns.find((c) => c.key === sort!.key);
		if (!col) return filtered;
		const dir = sort.dir === 'asc' ? 1 : -1;
		return [...filtered].sort((a, b) => compare(cell(a, col), cell(b, col)) * dir);
	});
	const count = $derived(onquery ? (total ?? rows.length) : sorted.length);
	const pages = $derived(pageSize ? Math.max(1, Math.ceil(count / pageSize)) : 1);
	const shown = $derived(onquery || !pageSize ? sorted : sorted.slice((page - 1) * pageSize, page * pageSize));

	$effect(() => {
		if (page > pages) page = pages;
	});
	$effect(() => {
		onquery?.({ page, pageSize, sort, search });
	});

	function toggleSort(col: Column<Row>) {
		if (!sort || sort.key !== col.key) sort = { key: col.key, dir: 'asc' };
		else if (sort.dir === 'asc') sort = { key: col.key, dir: 'desc' };
		else sort = null;
		page = 1;
	}

	const shownKeys = $derived(shown.map(key));
	const allShown = $derived(shownKeys.length > 0 && shownKeys.every((k) => selected.includes(k)));
	const someShown = $derived(!allShown && shownKeys.some((k) => selected.includes(k)));
	function toggleAll() {
		selected = allShown ? selected.filter((k) => !shownKeys.includes(k)) : [...new Set([...selected, ...shownKeys])];
	}
	function toggle(k: string) {
		selected = selected.includes(k) ? selected.filter((x) => x !== k) : [...selected, k];
	}
	const status = $derived(
		count === 0 ? empty : `${pageSize ? `${(page - 1) * pageSize + 1}–${Math.min(count, page * pageSize)} of ` : ''}${count} ${count === 1 ? 'row' : 'rows'}${selected.length ? `, ${selected.length} selected` : ''}`
	);
</script>

<div class="tern-datatable {klass}">
	{#if searchable || toolbar}
		<div class="bar">
			{#if searchable}
				<div class="search">
					<Input
						type="search"
						placeholder="Search…"
						aria-label="Search {caption}"
						aria-controls="tern-datatable-{uid}"
						bind:value={search}
						oninput={() => (page = 1)}
					/>
				</div>
			{/if}
			{#if toolbar}<div class="tools">{@render toolbar(selected)}</div>{/if}
		</div>
	{/if}
	<div class="wrap" class:loading>
		<table class="tern-table" id="tern-datatable-{uid}" aria-busy={loading || undefined}>
			<caption class:tern-sr-only={hideCaption}>{caption}</caption>
			<thead>
				<tr>
					{#if selectable}
						<th scope="col" class="select">
							<Checkbox checked={allShown} indeterminate={someShown} onchange={toggleAll} aria-label="Select all rows on this page" />
						</th>
					{/if}
					{#each columns as col (col.key)}
						<th scope="col" style:text-align={col.align} style:width={col.width} aria-sort={sort && sort.key === col.key ? (sort.dir === 'asc' ? 'ascending' : 'descending') : col.sortable ? 'none' : undefined}>
							{#if col.sortable}
								<button type="button" class="sort" onclick={() => toggleSort(col)}>
									{col.header}
									<svg aria-hidden="true" viewBox="0 0 16 16" width="12" height="12" class:on={sort?.key === col.key}>
										{#if sort && sort.key === col.key && sort.dir === 'desc'}<path d="M4 6l4 4 4-4" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" />{:else if sort?.key === col.key}<path d="M4 10l4-4 4 4" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" />{:else}<path d="M5 6.5l3-3 3 3M5 9.5l3 3 3-3" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round" />{/if}
									</svg>
								</button>
							{:else}
								{col.header}
							{/if}
						</th>
					{/each}
					{#if actions}<th scope="col"><span class="tern-sr-only">Actions</span></th>{/if}
				</tr>
			</thead>
			<tbody>
				{#each shown as row (key(row))}
					{@const k = key(row)}
					<tr class:selected={selected.includes(k)}>
						{#if selectable}
							<td class="select"><Checkbox checked={selected.includes(k)} onchange={() => toggle(k)} aria-label="Select row {String(cell(row, columns[0]!) ?? k)}" /></td>
						{/if}
						{#each columns as col (col.key)}
							<td style:text-align={col.align}>
								{#if col.cell}{@render col.cell(row)}{:else}{(row as Record<string, unknown>)[col.key] ?? ''}{/if}
							</td>
						{/each}
						{#if actions}<td class="actions">{@render actions(row)}</td>{/if}
					</tr>
				{:else}
					<tr><td class="empty" colspan={columns.length + (selectable ? 1 : 0) + (actions ? 1 : 0)}>{empty}</td></tr>
				{/each}
			</tbody>
		</table>
	</div>
	<div class="foot">
		<span class="status" role="status">{status}</span>
		{#if pages > 1}<Pagination bind:page {pages} label="{caption} pages" />{/if}
	</div>
</div>

<style>
	.tern-datatable {
		display: grid;
		gap: 0.75rem;
	}
	.bar {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: space-between;
		gap: 0.5rem;
	}
	.search {
		width: min(18rem, 100%);
	}
	.tools {
		display: flex;
		gap: 0.5rem;
	}
	.wrap {
		overflow-x: auto;
		background: var(--tern-surface);
		border: 1px solid var(--tern-border);
		border-radius: var(--tern-radius);
		transition: opacity var(--tern-duration);
	}
	.loading {
		opacity: 0.55;
	}
	caption {
		padding: 0.75rem;
		text-align: start;
		font-weight: 600;
		color: var(--tern-fg);
	}
	.sort {
		display: inline-flex;
		align-items: center;
		gap: 0.3rem;
		padding: 0.15rem 0.25rem;
		margin: -0.15rem -0.25rem;
		font: inherit;
		text-transform: inherit;
		letter-spacing: inherit;
		color: inherit;
		background: transparent;
		border: 0;
		border-radius: 4px;
		cursor: pointer;
	}
	.sort:hover {
		color: var(--tern-fg);
	}
	.sort:focus-visible {
		outline: 2px solid var(--tern-ring);
	}
	.sort svg {
		opacity: 0.6;
	}
	.sort svg.on {
		opacity: 1;
		color: var(--tern-fg);
	}
	.select {
		width: 2.25rem;
	}
	tr.selected td {
		background: var(--tern-accent-soft);
	}
	td {
		font-variant-numeric: tabular-nums;
	}
	.actions {
		text-align: end;
		white-space: nowrap;
	}
	.wrap :global(tbody tr:last-child td) {
		border-bottom: 0;
	}
	.empty {
		padding: 2rem;
		text-align: center;
		color: var(--tern-muted);
	}
	.foot {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: space-between;
		gap: 0.5rem;
	}
	.status {
		font-size: 0.8125rem;
		color: var(--tern-muted);
	}
</style>

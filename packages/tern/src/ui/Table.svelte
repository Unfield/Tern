<script lang="ts" generics="Row">
	import type { Snippet } from 'svelte';
	import type { Column } from './table.js';


	type Props = {
		columns: Column<Row>[];
		rows: Row[];
		/** A caption for the table; say what it lists. Hidden visually with hideCaption. */
		caption: string;
		hideCaption?: boolean;
		/** A stable key for each row. */
		key?: (row: Row) => string | number;
		/** Shown in place of rows when there are none. */
		empty?: string;
		class?: string;
	};
	let { columns, rows, caption, hideCaption = false, key = (r) => JSON.stringify(r), empty = 'Nothing to show', class: klass = '' }: Props = $props();
</script>

<!-- A semantic table, with a caption, column headers and numeric alignment. -->
<div class="tern-table-wrap {klass}">
	<table class="tern-table">
		<caption class:tern-sr-only={hideCaption}>{caption}</caption>
		<thead>
			<tr>
				{#each columns as col (col.key)}<th scope="col" style:text-align={col.align} style:width={col.width}>{col.header}</th>{/each}
			</tr>
		</thead>
		<tbody>
			{#each rows as row (key(row))}
				<tr>
					{#each columns as col (col.key)}
						<td style:text-align={col.align}>
							{#if col.cell}{@render col.cell(row)}{:else}{row[col.key] ?? ''}{/if}
						</td>
					{/each}
				</tr>
			{:else}
				<tr><td colspan={columns.length} class="empty">{empty}</td></tr>
			{/each}
		</tbody>
	</table>
</div>

<style>
	.tern-table-wrap {
		overflow-x: auto;
		border: 1px solid var(--tern-border);
		border-radius: var(--tern-radius);
		background: var(--tern-surface);
	}
	caption {
		padding: 0.75rem;
		text-align: start;
		font-weight: 600;
		color: var(--tern-fg);
	}
	td {
		font-variant-numeric: tabular-nums;
	}
	.tern-table-wrap :global(tbody tr:last-child td) {
		border-bottom: 0;
	}
	.empty {
		padding: 2rem;
		text-align: center;
		color: var(--tern-muted);
	}
</style>

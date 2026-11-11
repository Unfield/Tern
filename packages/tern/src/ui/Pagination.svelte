<script lang="ts">
	import { tb } from '../runtime/i18n.svelte.js';
	type Props = {
		/** The current page, from 1; bind it. */
		page?: number;
		/** How many pages there are. */
		pages: number;
		/** Makes pages links, such as (p) => `?page=${p}`, so they work without JavaScript and can be opened in new tabs. */
		href?: (page: number) => string;
		/** Pages shown on each side of the current one. */
		siblings?: number;
		label?: string;
		class?: string;
		onchange?: (page: number) => void;
	};
	let { page = $bindable(1), pages, href, siblings = 1, label: labelProp, class: klass = '', onchange }: Props = $props();
	const label = $derived(labelProp ?? tb('tern.ui.pagination.label'));

	// 1 … 4 5 6 … 20: the ends, the current page and its siblings.
	const shown = $derived.by(() => {
		const out: (number | 'gap')[] = [];
		const from = Math.max(2, page - siblings);
		const to = Math.min(pages - 1, page + siblings);
		out.push(1);
		if (from > 2) out.push(from === 3 ? 2 : 'gap');
		for (let p = from; p <= to; p++) out.push(p);
		if (to < pages - 1) out.push(to === pages - 2 ? pages - 1 : 'gap');
		if (pages > 1) out.push(pages);
		return out;
	});

	function go(p: number, e?: MouseEvent) {
		if (p < 1 || p > pages || p === page) return;
		if (!href) e?.preventDefault();
		page = p;
		onchange?.(p);
	}
</script>

{#snippet link(p: number, content: string, aria: string | undefined, disabled: boolean, cls: string)}
	{#if href && !disabled}
		<a href={href(p)} class={cls} aria-label={aria} aria-current={p === page && !aria ? 'page' : undefined} onclick={(e) => go(p, e)}>{content}</a>
	{:else}
		<button type="button" class={cls} aria-label={aria} aria-current={p === page && !aria ? 'page' : undefined} {disabled} onclick={(e) => go(p, e)}>{content}</button>
	{/if}
{/snippet}

<nav class="tern-pagination {klass}" aria-label={label}>
	<ul>
		<li>{@render link(page - 1, tb('tern.ui.pagination.previous'), tb('tern.ui.pagination.previousLabel'), page <= 1, 'step')}</li>
		{#each shown as p, i (i)}
			<li>
				{#if p === 'gap'}
					<span class="gap" aria-hidden="true">…</span>
				{:else}
					{@render link(p, String(p), undefined, false, 'num')}
				{/if}
			</li>
		{/each}
		<li>{@render link(page + 1, tb('tern.ui.pagination.next'), tb('tern.ui.pagination.nextLabel'), page >= pages, 'step')}</li>
	</ul>
</nav>

<style>
	ul {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.25rem;
		margin: 0;
		padding: 0;
		list-style: none;
	}
	a,
	button {
		display: inline-grid;
		place-items: center;
		min-width: 2.25rem;
		height: 2.25rem;
		padding: 0 0.6rem;
		box-sizing: border-box;
		font: inherit;
		font-size: 0.875rem;
		font-weight: 550;
		font-variant-numeric: tabular-nums;
		color: var(--tern-fg);
		text-decoration: none;
		background: transparent;
		border: 1px solid transparent;
		border-radius: var(--tern-radius-sm);
		cursor: pointer;
	}
	a:hover,
	button:hover:not(:disabled) {
		background: var(--tern-surface-2);
	}
	[aria-current='page'] {
		background: var(--tern-surface);
		border-color: var(--tern-border-strong);
		box-shadow: var(--tern-shadow);
	}
	button:disabled {
		color: var(--tern-muted);
		opacity: 0.5;
		cursor: not-allowed;
	}
	a:focus-visible,
	button:focus-visible {
		outline: 2px solid var(--tern-ring);
		outline-offset: 1px;
	}
	.gap {
		padding: 0 0.25rem;
		color: var(--tern-muted);
	}
</style>

<script lang="ts">
	import { mount, unmount } from 'svelte';
	import CopyButton from './CopyButton.svelte';
	import Icon from './Icon.svelte';

	type Link = { title: string; url: string };
	type Doc = {
		title: string;
		section: string;
		summary?: string;
		html: string;
		headings: { id: string; text: string; level: number }[] | null;
		edit: string;
		prev?: Link | null;
		next?: Link | null;
		words: number;
	};
	let { doc }: { doc: Doc } = $props();

	let body = $state<HTMLElement>();
	let active = $state('');
	const minutes = $derived(Math.max(1, Math.round(doc.words / 230)));
	const headings = $derived(doc.headings ?? []);

	// Copy buttons on code blocks, and wide tables in a scrolling box.
	$effect(() => {
		void doc.html;
		if (!body) return;
		const mounted: ReturnType<typeof mount>[] = [];
		for (const pre of body.querySelectorAll('pre')) {
			const wrap = document.createElement('div');
			wrap.className = 'code-block';
			pre.replaceWith(wrap);
			wrap.append(pre);
			const slot = document.createElement('div');
			slot.className = 'code-copy';
			wrap.append(slot);
			mounted.push(mount(CopyButton, { target: slot, props: { text: pre.innerText, label: 'Copy the code' } }));
		}
		for (const table of body.querySelectorAll('table')) {
			if (table.parentElement?.classList.contains('table-scroll')) continue;
			const wrap = document.createElement('div');
			wrap.className = 'table-scroll';
			table.replaceWith(wrap);
			wrap.append(table);
		}
		return () => mounted.forEach((m) => unmount(m));
	});

	// The section in view, for the table of contents.
	$effect(() => {
		void doc.html;
		if (!body || headings.length === 0) return;
		const els = headings.map((h) => document.getElementById(h.id)).filter((e): e is HTMLElement => !!e);
		const onScroll = () => {
			const line = 120;
			let cur = els[0]?.id ?? '';
			for (const e of els) {
				if (e.getBoundingClientRect().top - line <= 0) cur = e.id;
				else break;
			}
			active = cur;
		};
		onScroll();
		window.addEventListener('scroll', onScroll, { passive: true });
		return () => window.removeEventListener('scroll', onScroll);
	});
</script>

<svelte:head>
	<title>{doc.title} · Tern</title>
	{#if doc.summary}<meta name="description" content={doc.summary} />{/if}
	<meta property="og:title" content="{doc.title} · Tern" />
</svelte:head>

<div class="page" class:with-toc={headings.length > 2}>
	<article>
		<header>
			<p class="section">{doc.section}</p>
			<h1>{doc.title}</h1>
			{#if doc.summary}<p class="summary">{doc.summary}</p>{/if}
			<p class="meta">
				<span><Icon name="clock" size={14} /> {minutes} min read</span>
				<a href={doc.edit}><Icon name="edit" size={14} /> Edit on GitHub</a>
			</p>
		</header>
		<div class="prose" bind:this={body}>{@html doc.html}</div>
		{#if doc.prev || doc.next}
			<nav class="pager" aria-label="More pages">
				{#if doc.prev}
					<a class="prev" href={doc.prev.url}><span class="dir"><Icon name="chevron-left" size={14} /> Previous</span><span class="t">{doc.prev.title}</span></a>
				{:else}<span></span>{/if}
				{#if doc.next}
					<a class="next" href={doc.next.url}><span class="dir">Next <Icon name="chevron-right" size={14} /></span><span class="t">{doc.next.title}</span></a>
				{/if}
			</nav>
		{/if}
	</article>
	{#if headings.length > 2}
		<aside class="toc" aria-label="On this page">
			<h2>On this page</h2>
			<ul>
				{#each headings as h (h.id)}
					<li class:sub={h.level === 3}><a href="#{h.id}" aria-current={active === h.id ? 'location' : undefined}>{h.text}</a></li>
				{/each}
			</ul>
		</aside>
	{/if}
</div>

<style>
	.page {
		display: grid;
		grid-template-columns: minmax(0, 1fr);
		gap: var(--s-12);
	}
	.page.with-toc {
		grid-template-columns: minmax(0, 1fr) 220px;
	}
	article {
		min-width: 0;
		max-width: 760px;
	}
	header {
		padding-bottom: var(--s-8);
		margin-bottom: var(--s-8);
		border-bottom: 1px solid var(--border);
	}
	.section {
		color: var(--accent);
		font-size: 13.5px;
		font-weight: 600;
	}
	h1 {
		margin-top: var(--s-3);
		font-size: clamp(32px, 4vw, 42px);
		font-weight: 700;
		letter-spacing: -0.035em;
		line-height: 1.12;
	}
	.summary {
		margin-top: var(--s-4);
		color: var(--text-2);
		font-size: 18px;
		line-height: 1.55;
	}
	.meta {
		display: flex;
		flex-wrap: wrap;
		gap: var(--s-6);
		margin-top: var(--s-6);
		color: var(--text-3);
		font-size: 13.5px;
	}
	.meta span,
	.meta a {
		display: inline-flex;
		align-items: center;
		gap: 6px;
	}
	.meta a {
		color: var(--text-3);
	}
	.meta a:hover {
		color: var(--text);
	}

	/* The rendered Markdown */
	.prose {
		color: var(--text-2);
		font-size: 16px;
		line-height: 1.75;
	}
	.prose :global(> :first-child) {
		margin-top: 0;
	}
	.prose :global(p),
	.prose :global(ul),
	.prose :global(ol),
	.prose :global(blockquote),
	.prose :global(.code-block),
	.prose :global(.table-scroll) {
		margin: var(--s-5) 0 0;
	}
	.prose :global(h2) {
		margin: var(--s-16) 0 0;
		padding-top: var(--s-2);
		color: var(--text);
		font-size: 26px;
		font-weight: 660;
		letter-spacing: -0.025em;
		line-height: 1.25;
	}
	.prose :global(h3) {
		margin: var(--s-10) 0 0;
		color: var(--text);
		font-size: 19px;
		font-weight: 640;
		letter-spacing: -0.015em;
		line-height: 1.35;
	}
	.prose :global(h4) {
		margin: var(--s-8) 0 0;
		color: var(--text);
		font-size: 16px;
		font-weight: 640;
	}
	.prose :global(h2 + p),
	.prose :global(h3 + p),
	.prose :global(h4 + p) {
		margin-top: var(--s-3);
	}
	.prose :global(.anchor) {
		float: left;
		margin-left: -1.1em;
		padding-right: 0.3em;
		color: var(--text-3);
		font-weight: 400;
		opacity: 0;
		transition: opacity 140ms;
	}
	.prose :global(h2:hover .anchor),
	.prose :global(h3:hover .anchor),
	.prose :global(.anchor:focus-visible) {
		opacity: 1;
	}
	.prose :global(strong) {
		color: var(--text);
		font-weight: 620;
	}
	.prose :global(a) {
		color: var(--accent);
		text-decoration: underline;
		text-decoration-color: var(--accent-line);
		text-decoration-thickness: 1px;
		text-underline-offset: 3px;
		transition: text-decoration-color 140ms;
	}
	.prose :global(a:hover) {
		text-decoration-color: currentColor;
	}
	.prose :global(ul),
	.prose :global(ol) {
		padding-left: var(--s-6);
	}
	.prose :global(li + li),
	.prose :global(li > ul),
	.prose :global(li > ol) {
		margin-top: var(--s-2);
	}
	.prose :global(li::marker) {
		color: var(--text-3);
	}
	.prose :global(:not(pre) > code) {
		padding: 2px 6px;
		border: 1px solid var(--border);
		border-radius: 6px;
		background: var(--surface-2);
		color: var(--text);
		font-size: 0.86em;
	}
	.prose :global(p > code),
	.prose :global(li > code) {
		overflow-wrap: anywhere;
	}
	.prose :global(td code) {
		white-space: nowrap;
	}
	.prose :global(.code-block) {
		position: relative;
	}
	.prose :global(pre) {
		margin: 0;
		padding: var(--s-5) var(--s-6);
		border: 1px solid var(--border);
		border-radius: var(--radius);
		background: var(--code-bg);
		overflow-x: auto;
		font-size: 13.5px;
		line-height: 1.7;
		tab-size: 4;
	}
	.prose :global(.code-copy) {
		position: absolute;
		top: var(--s-2);
		right: var(--s-2);
		opacity: 0;
		transition: opacity 140ms;
	}
	.prose :global(.code-block:hover .code-copy),
	.prose :global(.code-copy:focus-within) {
		opacity: 1;
	}
	.prose :global(.table-scroll) {
		overflow-x: auto;
		border: 1px solid var(--border);
		border-radius: var(--radius);
	}
	.prose :global(table) {
		width: 100%;
		border-collapse: collapse;
		font-size: 14.5px;
		line-height: 1.55;
	}
	.prose :global(th),
	.prose :global(td) {
		padding: var(--s-3) var(--s-4);
		border-bottom: 1px solid var(--border);
		text-align: left;
		vertical-align: top;
	}
	.prose :global(th) {
		background: var(--surface-2);
		color: var(--text);
		font-size: 13.5px;
		font-weight: 600;
		white-space: nowrap;
	}
	.prose :global(tr:last-child td) {
		border-bottom: 0;
	}
	.prose :global(blockquote) {
		padding: var(--s-4) var(--s-5);
		border: 1px solid var(--accent-line);
		border-radius: var(--radius);
		background: var(--accent-soft);
		color: var(--text-2);
	}
	.prose :global(blockquote > :first-child) {
		margin-top: 0;
	}
	.prose :global(hr) {
		margin: var(--s-12) 0;
		border: 0;
		border-top: 1px solid var(--border);
	}
	.prose :global(img) {
		border-radius: var(--radius);
	}

	/* Pager */
	.pager {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: var(--s-4);
		margin-top: var(--s-20);
		padding-top: var(--s-8);
		border-top: 1px solid var(--border);
	}
	.pager a {
		display: flex;
		flex-direction: column;
		gap: var(--s-1);
		padding: var(--s-4) var(--s-5);
		border: 1px solid var(--border);
		border-radius: var(--radius);
		color: var(--text);
		transition: border-color 160ms, background 160ms;
	}
	.pager a:hover {
		border-color: var(--accent-line);
		background: var(--accent-soft);
	}
	.next {
		text-align: right;
		align-items: flex-end;
	}
	.dir {
		display: inline-flex;
		align-items: center;
		gap: 4px;
		color: var(--text-3);
		font-size: 13px;
	}
	.t {
		font-weight: 600;
		font-size: 15px;
	}

	/* Table of contents */
	.toc {
		position: sticky;
		top: calc(var(--header) + var(--s-10));
		align-self: start;
		max-height: calc(100vh - var(--header) - var(--s-20));
		overflow-y: auto;
		scrollbar-width: thin;
		scrollbar-color: transparent transparent;
	}
	.toc:hover {
		scrollbar-color: var(--border-strong) transparent;
	}
	.toc h2 {
		margin-bottom: var(--s-3);
		color: var(--text);
		font-size: 13px;
		font-weight: 600;
		letter-spacing: 0;
	}
	.toc ul {
		margin: 0;
		padding: 0;
		list-style: none;
		border-left: 1px solid var(--border);
	}
	.toc a {
		display: block;
		margin-left: -1px;
		padding: 5px 0 5px var(--s-3);
		border-left: 1px solid transparent;
		color: var(--text-3);
		font-size: 13px;
		line-height: 1.45;
		transition: color 140ms, border-color 140ms;
	}
	.toc .sub a {
		padding-left: var(--s-6);
	}
	.toc a:hover {
		color: var(--text);
	}
	.toc a[aria-current='location'] {
		color: var(--accent);
		border-left-color: var(--accent);
	}
	@media (max-width: 1240px) {
		.page.with-toc {
			grid-template-columns: minmax(0, 1fr);
		}
		.toc {
			display: none;
		}
	}
	@media (max-width: 640px) {
		.prose :global(pre) {
			padding: var(--s-4);
			font-size: 12.5px;
			border-radius: var(--radius-s);
		}
		.pager {
			grid-template-columns: 1fr;
		}
		.prose :global(h2) {
			margin-top: var(--s-12);
			font-size: 23px;
		}
	}
</style>

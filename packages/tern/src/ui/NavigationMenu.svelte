<script lang="ts">
	import { page } from '../runtime/state.svelte.js';
	import { float } from './floating.js';

	type Link = { label: string; href: string; description?: string };
	type Entry = { label: string; href?: string; items?: Link[] };
	type Props = {
		items: Entry[];
		label?: string;
		class?: string;
	};
	let { items, label = 'Main', class: klass = '' }: Props = $props();

	const uid = $props.id();
	let open = $state<number | null>(null);
	const buttons: HTMLButtonElement[] = [];
	let panel = $state<HTMLDivElement>();
	let hoverTimer: ReturnType<typeof setTimeout> | undefined;

	const current = (href: string) => page.url?.pathname === href;

	$effect(() => {
		if (open === null || !panel) return;
		panel.showPopover?.();
		const stop = float(panel, buttons[open]!, { side: 'bottom', align: 'start', offset: 8 });
		const away = (e: PointerEvent) => {
			const t = e.target as Node;
			if (!panel?.contains(t) && !buttons.some((b) => b?.contains(t))) open = null;
		};
		document.addEventListener('pointerdown', away, true);
		return () => {
			stop();
			document.removeEventListener('pointerdown', away, true);
		};
	});

	function hover(i: number | null) {
		clearTimeout(hoverTimer);
		if (matchMedia('(hover: none)').matches) return;
		hoverTimer = setTimeout(() => (open = i), i === null ? 250 : 120);
	}
</script>

<!-- Site navigation with dropdown panels: the disclosure pattern, since links don't belong in an ARIA menu. -->
<nav class="tern-navmenu {klass}" aria-label={label}>
	<ul>
		{#each items as entry, i (i)}
			<li>
				{#if entry.items}
					<button
						bind:this={buttons[i]}
						type="button"
						aria-expanded={open === i}
						aria-controls="tern-navmenu-{uid}-{i}"
						onclick={() => (open = open === i ? null : i)}
						onpointerenter={() => hover(i)}
						onpointerleave={() => hover(null)}
						onkeydown={(e) => {
							if (e.key === 'Escape' && open === i) {
								open = null;
							} else if (e.key === 'ArrowDown' && open === i) {
								e.preventDefault();
								panel?.querySelector('a')?.focus();
							}
						}}
					>
						{entry.label}
						<svg aria-hidden="true" viewBox="0 0 16 16" width="12" height="12"><path d="M4 6l4 4 4-4" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" /></svg>
					</button>
					{#if open === i}
						<div
							bind:this={panel}
							class="panel"
							id="tern-navmenu-{uid}-{i}"
							popover="manual"
							role="none"
							onpointerenter={() => clearTimeout(hoverTimer)}
							onpointerleave={() => hover(null)}
							onkeydown={(e) => {
								if (e.key === 'Escape') {
									open = null;
									buttons[i]?.focus();
								}
							}}
							onfocusout={(e) => {
								const to = e.relatedTarget as Node | null;
								if (to && !panel?.contains(to) && !buttons[i]?.contains(to)) open = null;
							}}
						>
							<ul class="links">
								{#each entry.items as link (link.href)}
									<li>
										<a href={link.href} aria-current={current(link.href) ? 'page' : undefined} onclick={() => (open = null)}>
											<span class="title">{link.label}</span>
											{#if link.description}<span class="description">{link.description}</span>{/if}
										</a>
									</li>
								{/each}
							</ul>
						</div>
					{/if}
				{:else if entry.href}
					<a class="top" href={entry.href} aria-current={current(entry.href) ? 'page' : undefined}>{entry.label}</a>
				{/if}
			</li>
		{/each}
	</ul>
</nav>

<style>
	ul {
		display: flex;
		gap: 0.125rem;
		margin: 0;
		padding: 0;
		list-style: none;
	}
	button,
	.top {
		display: inline-flex;
		align-items: center;
		gap: 0.3rem;
		height: 2.125rem;
		padding: 0 0.75rem;
		font: inherit;
		font-size: 0.875rem;
		font-weight: 550;
		color: var(--tern-fg);
		text-decoration: none;
		background: transparent;
		border: 0;
		border-radius: var(--tern-radius-sm);
		cursor: pointer;
	}
	button:hover,
	.top:hover,
	button[aria-expanded='true'] {
		background: var(--tern-surface-2);
	}
	button svg {
		color: var(--tern-muted);
		transition: rotate var(--tern-duration);
	}
	button[aria-expanded='true'] svg {
		rotate: 180deg;
	}
	.top[aria-current='page'] {
		color: var(--tern-accent);
	}
	button:focus-visible,
	a:focus-visible {
		outline: 2px solid var(--tern-ring);
		outline-offset: 1px;
	}
	.panel {
		padding: 0.5rem;
		color: var(--tern-fg);
		background: var(--tern-surface);
		border: 1px solid var(--tern-border);
		border-radius: var(--tern-radius);
		box-shadow: var(--tern-shadow-lg);
		animation: tern-nav-in var(--tern-duration) ease-out;
	}
	.links {
		display: grid;
		grid-template-columns: repeat(2, minmax(12rem, 16rem));
		gap: 0.25rem;
	}
	@media (max-width: 36rem) {
		.links {
			grid-template-columns: minmax(12rem, 1fr);
		}
	}
	.links a {
		display: grid;
		gap: 0.15rem;
		padding: 0.6rem 0.75rem;
		color: var(--tern-fg);
		text-decoration: none;
		border-radius: var(--tern-radius-sm);
	}
	.links a:hover {
		background: var(--tern-surface-2);
	}
	.links a[aria-current='page'] .title {
		color: var(--tern-accent);
	}
	.title {
		font-size: 0.875rem;
		font-weight: 600;
	}
	.description {
		font-size: 0.8125rem;
		color: var(--tern-muted);
		line-height: 1.4;
	}
	@keyframes tern-nav-in {
		from {
			opacity: 0;
			transform: translateY(-4px);
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.panel {
			animation: none;
		}
	}
</style>

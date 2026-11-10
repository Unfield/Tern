<script lang="ts">
	import type { Snippet } from 'svelte';
	import { page } from '../runtime/state.svelte.js';
	import Dialog from './Dialog.svelte';

	export type SidebarLink = {
		label: string;
		href: string;
		/** An icon, shown alone when the sidebar is collapsed. */
		icon?: Snippet;
		/** A count or label after the link, such as "3". */
		badge?: string;
		/** Links under this one, shown while it or one of them is current. */
		items?: { label: string; href: string }[];
	};
	export type SidebarGroup = { label?: string; items: SidebarLink[] };

	type Props = {
		groups: SidebarGroup[];
		/** The app's name or logo, at the top. */
		header?: Snippet;
		/** Things at the bottom, such as the signed-in user. */
		footer?: Snippet;
		/** Show icons only on wide screens; bind it. */
		collapsed?: boolean;
		/** Names the navigation. */
		label?: string;
		/** The page's content, next to the sidebar. */
		children: Snippet;
		class?: string;
	};
	let { groups, header, footer, collapsed = $bindable(false), label = 'Main', children, class: klass = '' }: Props = $props();

	let mobileOpen = $state(false);
	const path = $derived(page.url?.pathname ?? '');
	const isCurrent = (href: string) => path === href;
	const inSection = (l: SidebarLink) => isCurrent(l.href) || !!l.items?.some((s) => path === s.href || path.startsWith(s.href + '/'));

	// Close the mobile sheet after navigating.
	$effect(() => {
		void path;
		mobileOpen = false;
	});
</script>

{#snippet nav(compact: boolean)}
	<nav aria-label={label} class="nav" class:compact>
		{#each groups as group, gi (gi)}
			<div class="group">
				{#if group.label && !compact}<div class="group-label" id="tern-sidebar-group-{gi}">{group.label}</div>{/if}
				<ul aria-labelledby={group.label && !compact ? `tern-sidebar-group-${gi}` : undefined}>
					{#each group.items as link (link.href)}
						<li>
							<a href={link.href} class="link" aria-current={isCurrent(link.href) ? 'page' : undefined} aria-label={compact ? link.label : undefined} title={compact ? link.label : undefined}>
								{#if link.icon}<span class="icon" aria-hidden="true">{@render link.icon()}</span>{/if}
								{#if !compact}
									<span class="label">{link.label}</span>
									{#if link.badge}<span class="badge">{link.badge}</span>{/if}
								{/if}
							</a>
							{#if link.items && !compact && inSection(link)}
								<ul class="sub">
									{#each link.items as sub (sub.href)}
										<li><a href={sub.href} class="link" aria-current={isCurrent(sub.href) ? 'page' : undefined}>{sub.label}</a></li>
									{/each}
								</ul>
							{/if}
						</li>
					{/each}
				</ul>
			</div>
		{/each}
	</nav>
{/snippet}

<div class="tern-sidebar-layout {klass}" class:collapsed>
	<aside class="sidebar">
		<div class="top">
			{#if header && !collapsed}<div class="header">{@render header()}</div>{/if}
			<button type="button" class="collapse" aria-label={collapsed ? 'Expand sidebar' : 'Collapse sidebar'} aria-pressed={collapsed} onclick={() => (collapsed = !collapsed)}>
				<svg aria-hidden="true" viewBox="0 0 16 16" width="16" height="16" fill="none" stroke="currentColor" stroke-width="1.4"><rect x="2" y="2.5" width="12" height="11" rx="2" /><path d="M6 2.5v11" /></svg>
			</button>
		</div>
		{@render nav(collapsed)}
		{#if footer && !collapsed}<div class="footer">{@render footer()}</div>{/if}
	</aside>
	<div class="mobilebar">
		<button type="button" class="collapse" aria-label="Open navigation" aria-expanded={mobileOpen} onclick={() => (mobileOpen = true)}>
			<svg aria-hidden="true" viewBox="0 0 16 16" width="18" height="18" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"><path d="M2.5 4h11M2.5 8h11M2.5 12h11" /></svg>
		</button>
		{#if header}<div class="header">{@render header()}</div>{/if}
	</div>
	<div class="main">{@render children()}</div>
</div>

<Dialog bind:open={mobileOpen} title="Navigation" side="left" class="tern-sidebar-sheet">
	{@render nav(false)}
	{#if footer}<div class="footer">{@render footer()}</div>{/if}
</Dialog>

<style>
	.tern-sidebar-layout {
		display: grid;
		grid-template-columns: 15rem 1fr;
		min-height: 100vh;
	}
	.collapsed {
		grid-template-columns: 3.75rem 1fr;
	}
	.sidebar {
		position: sticky;
		top: 0;
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
		height: 100vh;
		box-sizing: border-box;
		padding: 0.75rem;
		overflow-y: auto;
		background: var(--tern-surface);
		border-right: 1px solid var(--tern-border);
	}
	.top {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.5rem;
		min-height: 2.25rem;
	}
	.collapsed .top {
		justify-content: center;
	}
	.header {
		font-weight: 700;
		letter-spacing: -0.01em;
		min-width: 0;
	}
	.collapse {
		display: grid;
		place-items: center;
		width: 2rem;
		height: 2rem;
		flex: none;
		color: var(--tern-muted);
		background: transparent;
		border: 0;
		border-radius: var(--tern-radius-sm);
		cursor: pointer;
	}
	.collapse:hover {
		color: var(--tern-fg);
		background: var(--tern-surface-2);
	}
	.collapse:focus-visible,
	.link:focus-visible {
		outline: 2px solid var(--tern-ring);
		outline-offset: -2px;
	}
	.nav {
		display: grid;
		gap: 1rem;
		align-content: start;
		flex: 1;
	}
	.group-label {
		padding: 0 0.6rem 0.3rem;
		font-size: 0.75rem;
		font-weight: 600;
		color: var(--tern-muted);
	}
	ul {
		display: grid;
		gap: 0.125rem;
		margin: 0;
		padding: 0;
		list-style: none;
	}
	.link {
		display: flex;
		align-items: center;
		gap: 0.6rem;
		min-height: 2.125rem;
		padding: 0 0.6rem;
		font-size: 0.875rem;
		font-weight: 500;
		color: var(--tern-muted);
		text-decoration: none;
		border-radius: var(--tern-radius-sm);
	}
	.compact .link {
		justify-content: center;
		padding: 0;
	}
	.link:hover {
		color: var(--tern-fg);
		background: var(--tern-surface-2);
	}
	.link[aria-current='page'] {
		color: var(--tern-fg);
		background: var(--tern-accent-soft);
		font-weight: 600;
	}
	.icon {
		display: inline-flex;
		width: 1.125rem;
		justify-content: center;
		flex: none;
	}
	.label {
		flex: 1;
	}
	.badge {
		min-width: 1.25rem;
		padding: 0 0.35rem;
		font-size: 0.75rem;
		font-weight: 600;
		text-align: center;
		color: var(--tern-fg);
		background: var(--tern-surface-2);
		border-radius: 1rem;
	}
	.sub {
		margin: 0.125rem 0 0.25rem 1.45rem;
		padding-left: 0.6rem;
		border-left: 1px solid var(--tern-border);
	}
	.sub .link {
		min-height: 1.875rem;
	}
	.footer {
		padding-top: 0.75rem;
		border-top: 1px solid var(--tern-border);
	}
	.main {
		min-width: 0;
	}
	.mobilebar {
		display: none;
	}
	@media (max-width: 48rem) {
		.tern-sidebar-layout,
		.collapsed {
			grid-template-columns: 1fr;
		}
		.sidebar {
			display: none;
		}
		.mobilebar {
			position: sticky;
			top: 0;
			z-index: 10;
			display: flex;
			align-items: center;
			gap: 0.5rem;
			padding: 0.5rem 0.75rem;
			background: var(--tern-surface);
			border-bottom: 1px solid var(--tern-border);
		}
	}
	:global(.tern-sidebar-sheet.left) {
		width: min(18rem, 100vw - 3rem);
	}
</style>

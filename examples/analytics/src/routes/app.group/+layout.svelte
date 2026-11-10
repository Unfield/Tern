<script lang="ts">
	import { Avatar, Button, Item, Sidebar, type SidebarGroup } from '@ternkit/tern/ui';
	import type { LayoutProps } from './$types';

	let { data, children }: LayoutProps = $props();
	const groups: SidebarGroup[] = $derived([
		{ items: [{ label: 'All sites', href: '/sites', icon: sitesIcon }] },
		{ label: 'Sites', items: (data.sites ?? []).map((s) => ({ label: s.name, href: `/sites/${s.domain}`, icon: dot })) }
	]);
</script>

{#snippet sitesIcon()}<svg viewBox="0 0 16 16" width="16" height="16" fill="none" stroke="currentColor" stroke-width="1.4"><rect x="2" y="2.5" width="12" height="11" rx="2" /><path d="M2 6h12" /></svg>{/snippet}
{#snippet dot()}<svg viewBox="0 0 16 16" width="16" height="16"><circle cx="8" cy="8" r="3" fill="currentColor" /></svg>{/snippet}

<Sidebar {groups} label="Analytics">
	{#snippet header()}<a class="brand" href="/sites">Analytics</a>{/snippet}
	{#snippet footer()}
		<Item title={data.user.name} description={data.user.email}>
			{#snippet media()}<Avatar name={data.user.name} size="1.75rem" />{/snippet}
		</Item>
		<form method="POST" action="/logout"><Button type="submit" variant="ghost" size="sm">Sign out</Button></form>
	{/snippet}
	<main class="main">{@render children()}</main>
</Sidebar>

<style>
	.brand {
		color: var(--tern-fg);
		text-decoration: none;
	}
	.main {
		max-width: 68rem;
		padding: 2rem 2rem 4rem;
		box-sizing: border-box;
	}
	@media (max-width: 48rem) {
		.main {
			padding: 1.25rem 1rem 3rem;
		}
	}
</style>

<script lang="ts">
	import {
		AspectRatio,
		Avatar,
		Badge,
		Breadcrumb,
		Button,
		Carousel,
		Collapsible,
		Empty,
		Item,
		NavigationMenu,
		Pagination,
		Resizable,
		ScrollArea,
		Sidebar,
		Table,
		type SidebarGroup
	} from '@ternkit/tern/ui';

	let page = $state(4);
	let split = $state(35);
	let collapsed = $state(false);
	let open = $state(false);

	const groups: SidebarGroup[] = [
		{ items: [{ label: 'Overview', href: '/layout', badge: '3' }, { label: 'Sites', href: '/layout/sites' }] },
		{ label: 'Settings', items: [{ label: 'Team', href: '/layout/team' }, { label: 'Billing', href: '/layout/billing' }] }
	];
	const projects = [
		{ name: 'Acme site', owner: 'Ada', visits: 12840 },
		{ name: 'Docs', owner: 'Grace', visits: 9312 },
		{ name: 'Blog', owner: 'Alan', visits: 4021 }
	];
	const slides = ['Fast by default', 'One binary', 'Go on the server', 'Svelte in the browser', 'Typed end to end'];
</script>

<svelte:head><title>Layout · Tern UI</title></svelte:head>

<h1>Layout and navigation</h1>

<section aria-labelledby="nav">
	<h2 id="nav">Navigation</h2>
	<NavigationMenu
		label="Product"
		items={[
			{
				label: 'Product',
				items: [
					{ label: 'Live components', href: '/layout#live', description: 'Component logic in Go, on the server.' },
					{ label: 'Tern UI', href: '/', description: 'Accessible components in Tern’s own style.' },
					{ label: 'Realtime', href: '/layout#realtime', description: 'WebSockets, server-sent events and pub/sub.' },
					{ label: 'Jobs', href: '/layout#jobs', description: 'Background jobs, cron and migrations.' }
				]
			},
			{ label: 'Docs', href: '/layout#docs' },
			{ label: 'Inputs', href: '/inputs' }
		]}
	/>
	<div class="stack">
		<Breadcrumb items={[{ label: 'Home', href: '/' }, { label: 'Projects', href: '/layout#p' }, { label: 'Acme', href: '/layout#a' }, { label: 'Settings', href: '/layout#s' }, { label: 'Domains' }]} />
		<Pagination bind:page pages={20} />
		<p data-testid="page">Page {page}</p>
	</div>
</section>

<section aria-labelledby="shell">
	<h2 id="shell">App shell</h2>
	<div class="shell">
		<Sidebar {groups} bind:collapsed label="App">
			{#snippet header()}Acme{/snippet}
			{#snippet footer()}<Item title="Ada Lovelace" description="ada@example.com">{#snippet media()}<Avatar name="Ada Lovelace" size="1.75rem" />{/snippet}</Item>{/snippet}
			<div class="content"><p>The page's content sits here, next to the sidebar. On a phone, the sidebar moves into a sheet.</p></div>
		</Sidebar>
	</div>
</section>

<section aria-labelledby="panes">
	<h2 id="panes">Panes and scrolling</h2>
	<div class="panes">
		<Resizable bind:size={split} label="Resize the file list">
			{#snippet first()}
				<ScrollArea label="Files" maxHeight="12rem">
					<ul class="files">{#each { length: 30 }, i (i)}<li>file-{i + 1}.go</li>{/each}</ul>
				</ScrollArea>
			{/snippet}
			{#snippet second()}<div class="content"><p data-testid="split">The file list takes {Math.round(split)}%.</p></div>{/snippet}
		</Resizable>
	</div>
	<Collapsible bind:open label="Advanced settings"><p>Hidden until opened.</p></Collapsible>
</section>

<section aria-labelledby="media">
	<h2 id="media">Media and lists</h2>
	<div class="media">
		<AspectRatio ratio={16 / 9}><div class="art" role="img" aria-label="A teal gradient"></div></AspectRatio>
	</div>
	<Carousel items={slides} label="Why Tern" perView={2}>
		{#snippet slide(text, i)}<div class="slide"><Badge>{i + 1}</Badge><strong>{text}</strong></div>{/snippet}
	</Carousel>
	<ul class="items">
		{#each projects as p (p.name)}
			<Item as="li" title={p.name} description="Owned by {p.owner}" href="/layout#{p.name}">
				{#snippet media()}<Avatar name={p.owner} size="2rem" />{/snippet}
				{#snippet actions()}<Button size="sm" variant="ghost">Open</Button>{/snippet}
			</Item>
		{/each}
	</ul>
	<Table
		caption="Projects"
		rows={projects}
		key={(p) => p.name}
		columns={[
			{ key: 'name', header: 'Project' },
			{ key: 'owner', header: 'Owner' },
			{ key: 'visits', header: 'Visits', align: 'end' }
		]}
	/>
	<Empty title="No deploys yet" description="Push to main and your first deploy shows up here." level={3}>
		{#snippet actions()}<Button>Connect a repository</Button>{/snippet}
	</Empty>
</section>

<style>
	section {
		margin-top: 2.5rem;
		display: grid;
		gap: 1rem;
	}
	.stack {
		display: grid;
		gap: 0.75rem;
	}
	.shell {
		border: 1px solid var(--tern-border);
		border-radius: var(--tern-radius);
		overflow: hidden;
	}
	.shell :global(.tern-sidebar-layout) {
		min-height: 18rem;
	}
	.shell :global(.sidebar) {
		position: static;
		height: auto;
	}
	.content {
		padding: 1rem 1.25rem;
	}
	.panes {
		height: 12rem;
	}
	.files {
		margin: 0;
		padding: 0.5rem 1rem;
		list-style: none;
		font-family: var(--tern-font-mono);
		font-size: 0.8125rem;
		line-height: 1.8;
	}
	.media {
		max-width: 24rem;
	}
	.art {
		position: absolute;
		inset: 0;
		background: linear-gradient(135deg, var(--tern-accent), var(--tern-accent-soft));
	}
	.slide {
		display: flex;
		align-items: center;
		gap: 0.6rem;
		height: 6rem;
		padding: 1rem;
		background: var(--tern-surface);
		border: 1px solid var(--tern-border);
		border-radius: var(--tern-radius);
	}
	.items {
		margin: 0;
		padding: 0;
	}
</style>

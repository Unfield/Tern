<script lang="ts">
	import { AlertDialog, Avatar, Button, ContextMenu, Drawer, Field, HoverCard, Input, Menu, Menubar, Popover, Sheet, Switch, toast, type MenuItem } from '@ternkit/tern/ui';

	let sheetOpen = $state(false);
	let drawerOpen = $state(false);
	let alertOpen = $state(false);
	let popoverOpen = $state(false);
	let width = $state('320');
	let wrap = $state(true);
	let grid = $state(true);
	let zoom = $state('100');
	let last = $state('');
	let removed = $state(false);

	const viewItems: MenuItem[] = $derived([
		{ heading: 'Show' },
		{ label: 'Word wrap', type: 'checkbox', checked: wrap, onselect: () => (wrap = !wrap) },
		{ label: 'Grid', type: 'checkbox', checked: grid, onselect: () => (grid = !grid) },
		{ separator: true },
		{ heading: 'Zoom' },
		...['75', '100', '150'].map((z): MenuItem => ({ label: `${z}%`, type: 'radio', checked: zoom === z, onselect: () => (zoom = z) }))
	]);
	const fileItems: MenuItem[] = [
		{ label: 'New file', shortcut: '⌘N', onselect: () => (last = 'New file') },
		{ label: 'Open…', shortcut: '⌘O', onselect: () => (last = 'Open') },
		{
			label: 'Open recent',
			items: [
				{ label: 'budget.csv', onselect: () => (last = 'budget.csv') },
				{ label: 'notes.md', onselect: () => (last = 'notes.md') },
				{ label: 'More', items: [{ label: 'archive.zip', onselect: () => (last = 'archive.zip') }] }
			]
		},
		{ separator: true },
		{ label: 'Print', shortcut: '⌘P', disabled: true },
		{ label: 'Close window', danger: true, onselect: () => (last = 'Close window') }
	];
	const editItems: MenuItem[] = [
		{ label: 'Undo', shortcut: '⌘Z', onselect: () => (last = 'Undo') },
		{ label: 'Redo', shortcut: '⇧⌘Z', onselect: () => (last = 'Redo') },
		{ separator: true },
		{ label: 'Find', items: [{ label: 'Find…', onselect: () => (last = 'Find') }, { label: 'Replace…', onselect: () => (last = 'Replace') }] }
	];
</script>

<svelte:head><title>Overlays · Tern UI</title></svelte:head>

<h1>Overlays</h1>
<p class="lead">Popovers, menus and sheets open in the browser's top layer, so a scrolling container or a z-index never clips them. They flip and shift to stay on screen.</p>

<section aria-labelledby="popover">
	<h2 id="popover">Popover and hover card</h2>
	<div class="row">
		<Popover title="Dimensions" bind:open={popoverOpen}>
			{#snippet trigger(props)}<Button variant="secondary" {...props} data-testid="popover-trigger">Set width</Button>{/snippet}
			<Field label="Width in pixels">
				<Input bind:value={width} inputmode="numeric" data-testid="popover-width" />
			</Field>
			<Button size="sm" onclick={() => (popoverOpen = false)}>Done</Button>
		</Popover>
		<span data-testid="width">Width: {width}px</span>

		<HoverCard>
			{#snippet trigger(props)}<a href="https://github.com/Unfield/Tern" {...props}>@ternkit</a>{/snippet}
			<div class="profile">
				<Avatar name="Tern Kit" />
				<div>
					<strong>Tern</strong>
					<p>Svelte on the client, Go on the server.</p>
				</div>
			</div>
		</HoverCard>
	</div>
</section>

<section aria-labelledby="menus">
	<h2 id="menus">Menus</h2>
	<div class="row">
		<Menubar
			label="Editor"
			menus={[
				{ label: 'File', items: fileItems },
				{ label: 'Edit', items: editItems },
				{ label: 'View', items: viewItems }
			]}
		/>
		<Menu items={viewItems}>View options</Menu>
	</div>
	<ContextMenu
		label="Card actions"
		items={[
			{ label: 'Copy link', shortcut: '⌘C', onselect: () => (last = 'Copy link') },
			{ label: 'Duplicate', onselect: () => (last = 'Duplicate') },
			{ separator: true },
			{ label: 'Remove', danger: true, onselect: () => (last = 'Remove') }
		]}
	>
		<button type="button" class="context" data-testid="context-area">Right-click, long-press or press Shift+F10 here</button>
	</ContextMenu>
	<p data-testid="last">Last chosen: {last || 'nothing yet'} · wrap {wrap ? 'on' : 'off'} · zoom {zoom}%</p>
</section>

<section aria-labelledby="sheets">
	<h2 id="sheets">Sheets, drawers and alert dialogs</h2>
	<div class="row">
		<Button variant="secondary" onclick={() => (sheetOpen = true)} data-testid="open-sheet">Open filters</Button>
		<Button variant="secondary" onclick={() => (drawerOpen = true)} data-testid="open-drawer">Open drawer</Button>
		<Button variant="danger" onclick={() => (alertOpen = true)} data-testid="open-alert">Remove member</Button>
		{#if removed}<span data-testid="removed">Member removed.</span>{/if}
	</div>
</section>

<Sheet bind:open={sheetOpen} title="Filters" description="Narrow down what the list shows.">
	<Switch checked>Only active projects</Switch>
	<Field label="Owner"><Input placeholder="Anyone" /></Field>
	{#snippet footer()}
		<Button variant="secondary" onclick={() => (sheetOpen = false)}>Reset</Button>
		<Button onclick={() => (sheetOpen = false)}>Apply</Button>
	{/snippet}
</Sheet>

<Drawer bind:open={drawerOpen} title="Share" description="Anyone with the link can view.">
	<Field label="Link"><Input value="https://example.com/p/acme" readonly /></Field>
	{#snippet footer()}<Button onclick={() => (drawerOpen = false)}>Done</Button>{/snippet}
</Drawer>

<AlertDialog
	bind:open={alertOpen}
	title="Remove Ada from the team?"
	description="She loses access to every project right away."
	confirmLabel="Remove"
	danger
	onconfirm={async () => {
		await new Promise((r) => setTimeout(r, 300));
		removed = true;
		toast.success('Ada was removed');
	}}
/>

<style>
	.lead {
		color: var(--tern-muted);
	}
	section {
		margin-top: 2.5rem;
	}
	.row {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.75rem;
	}
	.profile {
		display: flex;
		gap: 0.75rem;
	}
	.profile p {
		margin: 0.15rem 0 0;
		color: var(--tern-muted);
	}
	.context {
		display: grid;
		place-items: center;
		width: 100%;
		margin-top: 1rem;
		height: 7rem;
		font: inherit;
		color: var(--tern-muted);
		background: var(--tern-surface);
		border: 1px dashed var(--tern-border-strong);
		border-radius: var(--tern-radius);
	}
	.context:focus-visible {
		outline: 2px solid var(--tern-ring);
		outline-offset: 2px;
	}
	a {
		color: var(--tern-accent);
	}
</style>

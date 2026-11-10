<script lang="ts">
	import {
		Accordion,
		Alert,
		Avatar,
		Badge,
		Button,
		Card,
		Checkbox,
		Dialog,
		Field,
		Input,
		Kbd,
		Menu,
		Progress,
		RadioGroup,
		Select,
		Separator,
		Skeleton,
		Spinner,
		Switch,
		Tabs,
		Textarea,
		Tooltip,
		toast
	} from '@ternkit/tern/ui';

	let dialogOpen = $state(false);
	let deleted = $state('');
	let plan = $state('team');
	let notify = $state(true);
	let agree = $state(false);
	let tab = $state('overview');
	let region = $state('');
	let name = $state('');
	let saving = $state(false);
	let progress = $state(40);
	let lastAction = $state('');

	async function save() {
		saving = true;
		await new Promise((r) => setTimeout(r, 400));
		saving = false;
		toast.success('Changes saved', { description: 'Your project settings are up to date.' });
	}
</script>

<svelte:head><title>Components · Tern UI</title></svelte:head>

<h1>Components</h1>
<p class="lead">Accessible Svelte components with Tern's own look. Use the keyboard everywhere: every control here works without a mouse.</p>

<section aria-labelledby="buttons">
	<h2 id="buttons">Buttons</h2>
	<div class="row">
		<Button onclick={save} loading={saving} data-testid="save">Save changes</Button>
		<Button variant="secondary" onclick={() => toast('Draft kept')}>Keep draft</Button>
		<Button variant="ghost">Cancel</Button>
		<Button variant="danger" onclick={() => (dialogOpen = true)} data-testid="open-dialog">Delete project</Button>
		<Button size="sm" variant="secondary">Small</Button>
		<Button size="lg">Large</Button>
		<Button disabled>Disabled</Button>
		<Button href="/form" variant="secondary">A link that looks like a button</Button>
	</div>
</section>

<Dialog bind:open={dialogOpen} title="Delete this project?" description="Its monitors and history go too. This can't be undone.">
	<Field label="Type the project's name to confirm">
		<Input bind:value={name} placeholder="acme-site" data-testid="confirm-name" />
	</Field>
	{#snippet footer()}
		<Button variant="secondary" onclick={() => (dialogOpen = false)}>Cancel</Button>
		<Button
			variant="danger"
			disabled={name !== 'acme-site'}
			onclick={() => {
				deleted = name;
				dialogOpen = false;
				toast.error('Project deleted');
			}}>Delete</Button
		>
	{/snippet}
</Dialog>
{#if deleted}<p data-testid="deleted">Deleted {deleted}.</p>{/if}

<section aria-labelledby="fields">
	<h2 id="fields">Fields</h2>
	<div class="grid">
		<Field label="Project name" description="Shown on your status page." required>
			<Input placeholder="Acme" autocomplete="off" />
		</Field>
		<Field label="Email" error="That doesn't look like an email address.">
			<Input type="email" value="ada@" />
		</Field>
		<Field label="Region">
			<Select
				bind:value={region}
				placeholder="Choose a region"
				options={[
					{ value: 'eu', label: 'Europe (Frankfurt)' },
					{ value: 'us', label: 'United States (Virginia)' },
					{ value: 'ap', label: 'Asia Pacific (Singapore)' }
				]}
			/>
		</Field>
		<Field label="Notes">
			<Textarea placeholder="Anything we should know?" rows={3} />
		</Field>
	</div>
	<div class="row wide">
		<RadioGroup
			label="Plan"
			name="plan"
			bind:value={plan}
			options={[
				{ value: 'hobby', label: 'Hobby', description: 'One project' },
				{ value: 'team', label: 'Team', description: 'Up to 20 projects' },
				{ value: 'enterprise', label: 'Enterprise', description: 'Talk to us', disabled: true }
			]}
		/>
		<div class="stack">
			<Switch bind:checked={notify} data-testid="notify">Email me when a monitor goes down</Switch>
			<Checkbox bind:checked={agree} description="You can change this later.">Share anonymous usage data</Checkbox>
			<p class="muted" data-testid="choices">Plan: {plan} · notify: {notify ? 'on' : 'off'} · share: {agree ? 'yes' : 'no'}</p>
		</div>
	</div>
</section>

<section aria-labelledby="navigation">
	<h2 id="navigation">Tabs, menus and tooltips</h2>
	<Tabs
		label="Project"
		bind:value={tab}
		tabs={[
			{ id: 'overview', label: 'Overview' },
			{ id: 'activity', label: 'Activity' },
			{ id: 'settings', label: 'Settings' },
			{ id: 'billing', label: 'Billing', disabled: true }
		]}
	>
		{#snippet panel(id)}
			<p data-testid="panel">This is the {id} panel.</p>
		{/snippet}
	</Tabs>
	<div class="row">
		<Menu
			items={[
				{ label: 'Rename', onselect: () => (lastAction = 'rename') },
				{ label: 'Duplicate', onselect: () => (lastAction = 'duplicate') },
				{ label: 'Archive', disabled: true },
				{ separator: true, label: '' },
				{ label: 'Delete', danger: true, onselect: () => (lastAction = 'delete') }
			]}>Actions</Menu
		>
		<span data-testid="last-action">{lastAction || 'Nothing chosen'}</span>
		<Tooltip text="Copies the link to this project">
			{#snippet children(props)}
				<Button variant="secondary" size="sm" {...props}>Copy link</Button>
			{/snippet}
		</Tooltip>
		<span class="muted">Press <Kbd>⌘</Kbd> <Kbd>K</Kbd> to search</span>
	</div>
</section>

<section aria-labelledby="feedback">
	<h2 id="feedback">Feedback</h2>
	<div class="stack">
		<Alert tone="info" title="Maintenance on Sunday">Checks pause for about ten minutes at 02:00 UTC.</Alert>
		<Alert tone="success">All monitors are up.</Alert>
		<Alert tone="warning" title="Certificate expires soon">api.example.com's certificate expires in 6 days.</Alert>
		<Alert tone="danger" title="Checkout is down">Since 14:02 UTC. We're notifying the on-call team.</Alert>
		<div class="row">
			<Badge>Draft</Badge>
			<Badge tone="accent">New</Badge>
			<Badge tone="success" dot>Up</Badge>
			<Badge tone="warning" dot>Degraded</Badge>
			<Badge tone="danger" dot>Down</Badge>
			<Spinner label="Loading monitors" />
		</div>
		<Progress value={progress} label="Uploading backup" showLabel />
		<div class="row">
			<Button size="sm" variant="secondary" onclick={() => (progress = Math.min(100, progress + 20))}>Advance</Button>
			<Progress label="Checking" />
		</div>
	</div>
</section>

<section aria-labelledby="display">
	<h2 id="display">Cards and content</h2>
	<div class="grid">
		<Card title="Ada Lovelace" description="Administrator" as="article">
			{#snippet actions()}<Badge tone="success">Active</Badge>{/snippet}
			<div class="row">
				<Avatar name="Ada Lovelace" />
				<Avatar name="Grace Hopper" size="2.75rem" />
				<Avatar name="Alan Turing" src="/missing.png" />
			</div>
			{#snippet footer()}<Button size="sm" variant="secondary">Message</Button>{/snippet}
		</Card>
		<Card title="Loading" as="section" aria-busy="true">
			<div class="stack">
				<Skeleton height="1.25rem" width="60%" />
				<Skeleton />
				<Skeleton width="80%" />
			</div>
		</Card>
	</div>
	<Separator />
	<table class="tern-table">
		<caption class="tern-sr-only">Monitors</caption>
		<thead><tr><th scope="col">Monitor</th><th scope="col">Status</th><th scope="col">Uptime</th></tr></thead>
		<tbody>
			<tr><td>Website</td><td><Badge tone="success" dot>Up</Badge></td><td>99.98%</td></tr>
			<tr><td>API</td><td><Badge tone="danger" dot>Down</Badge></td><td>98.40%</td></tr>
		</tbody>
	</table>
	<Accordion
		items={[
			{ id: 'what', title: 'What is Tern UI?' },
			{ id: 'style', title: 'Can I change the colors?' },
			{ id: 'a11y', title: 'Is it accessible?' }
		]}
		level={3}
	>
		{#snippet content(id)}
			{#if id === 'what'}<p>Svelte 5 components that come with Tern, styled with CSS custom properties.</p>
			{:else if id === 'style'}<p>Override the <code>--tern-*</code> properties on :root, or on any element for part of a page.</p>
			{:else}<p>They follow the WAI-ARIA patterns, work with the keyboard and pass axe audits in CI.</p>{/if}
		{/snippet}
	</Accordion>
</section>

<style>
	h1 {
		margin: 0;
		font-size: 1.75rem;
		letter-spacing: -0.02em;
	}
	.lead {
		color: var(--tern-muted);
		margin: 0.5rem 0 2rem;
	}
	section {
		display: grid;
		gap: 1rem;
		margin-bottom: 2.5rem;
	}
	h2 {
		margin: 0;
		font-size: 1.125rem;
		letter-spacing: -0.01em;
	}
	.row {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.75rem;
	}
	.row.wide {
		gap: 3rem;
		align-items: flex-start;
	}
	.grid {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(16rem, 1fr));
		gap: 1rem;
	}
	.stack {
		display: grid;
		gap: 0.75rem;
	}
	.muted {
		color: var(--tern-muted);
		font-size: 0.875rem;
		margin: 0;
	}
	code {
		font-family: var(--tern-font-mono);
		font-size: 0.85em;
	}
</style>

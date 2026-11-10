<script lang="ts">
	import { Button, Card, Dialog, Empty, Field, Form, Input, Sparkline } from '@ternkit/tern/ui';
	import type { PageProps } from './$types';
	import { forms } from './$forms';

	let { data, form }: PageProps = $props();
	// Reopen the dialog when the server sent errors back.
	let adding = $state(false);
	$effect(() => {
		if (form && 'errors' in form) adding = true;
	});
	let submitting = $state(false);
	const cards = $derived(data.cards ?? []);
</script>

<svelte:head><title>Sites · Analytics</title></svelte:head>

<header class="head">
	<h1>Sites</h1>
	<Button onclick={() => (adding = true)} data-testid="add-site">Add a site</Button>
</header>

{#if cards.length === 0}
	<Empty title="No sites yet" description="Add a site, then put one script tag on it to start counting." level={2}>
		{#snippet actions()}<Button onclick={() => (adding = true)}>Add a site</Button>{/snippet}
	</Empty>
{:else}
	<ul class="cards">
		{#each cards as site (site.id)}
			<li>
				<Card as="article" title={site.name} description={site.domain} level={2}>
					<div class="row">
						<div>
							<div class="value">{site.visitors.toLocaleString()}</div>
							<div class="label">visitors in the last 7 days</div>
						</div>
						<Sparkline values={site.trend ?? []} label="{site.name}: visitors per day this week" />
					</div>
					{#snippet footer()}<Button variant="secondary" size="sm" href="/sites/{site.domain}">Open dashboard</Button>{/snippet}
				</Card>
			</li>
		{/each}
	</ul>
{/if}

<Dialog bind:open={adding} title="Add a site" description="Page views are counted for this domain and its subdomains.">
	<Form action="?/add" rules={forms.add} {form} bind:submitting reset={false} id="add-site-form">
		<Field label="Domain" name="domain" description="Such as example.com, without https://.">
			<Input autocomplete="off" spellcheck="false" placeholder="example.com" />
		</Field>
		<Field label="Name" name="name"><Input placeholder="My site" /></Field>
	</Form>
	{#snippet footer()}
		<Button variant="secondary" onclick={() => (adding = false)}>Cancel</Button>
		<Button type="submit" form="add-site-form" loading={submitting}>Add site</Button>
	{/snippet}
</Dialog>

<style>
	.head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 1rem;
		margin-bottom: 1.5rem;
	}
	h1 {
		margin: 0;
		font-size: 1.5rem;
		letter-spacing: -0.02em;
	}
	.cards {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(17rem, 1fr));
		gap: 1rem;
		margin: 0;
		padding: 0;
		list-style: none;
	}
	.row {
		display: flex;
		align-items: end;
		justify-content: space-between;
		gap: 1rem;
	}
	.value {
		font-size: 1.75rem;
		font-weight: 650;
		font-variant-numeric: tabular-nums;
	}
	.label {
		color: var(--tern-muted);
		font-size: 0.8125rem;
	}
</style>

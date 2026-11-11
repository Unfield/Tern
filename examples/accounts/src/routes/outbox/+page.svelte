<script lang="ts">
	import { invalidateAll } from '@ternkit/tern';
	import { Button, Card, Empty } from '@ternkit/tern/ui';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
</script>

<svelte:head><title>Outbox · Accounts</title></svelte:head>

<h1>Outbox</h1>
<p>The emails the app sent. A real app sends them with <code>accounts.SendWith(mailer, queue)</code>.</p>
<p><Button variant="secondary" size="sm" onclick={() => invalidateAll()}>Refresh</Button></p>
{#each data.emails as e, i (i)}
	<Card title={e.subject} description="To {e.to}" level={2}>
		<pre>{e.text}</pre>
		<a href={e.link} data-testid="email-link">{e.link}</a>
	</Card>
{:else}
	<Empty title="No emails yet" />
{/each}

<style>
	pre {
		white-space: pre-wrap;
		font: inherit;
		font-size: 0.875rem;
	}
	:global(.tern-card) {
		margin-block-end: 1rem;
	}
</style>

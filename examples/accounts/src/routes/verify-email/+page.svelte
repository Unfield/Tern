<script lang="ts">
	import { Alert, Button } from '@ternkit/tern/ui';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
</script>

<svelte:head><title>Confirm your email · Accounts</title></svelte:head>

<h1>Confirm your email</h1>
{#if data.verified}
	<Alert tone="success">{data.email} is confirmed.</Alert>
{:else}
	<Alert tone="danger">This link is invalid or has expired. Send a new one from your account.</Alert>
{/if}
<p><Button href="/account" variant="secondary">Go to your account</Button></p>

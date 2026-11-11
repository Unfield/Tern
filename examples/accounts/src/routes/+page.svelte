<script lang="ts">
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
</script>

<svelte:head><title>Accounts example</title></svelte:head>

<h1>Accounts</h1>
{#if data.user}
	<p>You're signed in as <strong>{data.user.email}</strong>. <a href="/account">Manage your account</a>.</p>
{:else}
	<p>
		Sign up with an email and a password, then add a passkey, see your devices, or reset your password: the emails land in the
		<a href="/outbox">outbox</a>.
	</p>
	<p><a href="/signup">Create an account</a> or <a href="/login">sign in</a>.</p>
{/if}

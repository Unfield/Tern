<script lang="ts">
	import { Alert, AlertDialog, Badge, Button, Devices, Field, Form, Input, Passkeys, PasswordInput, TwoStep } from '@ternkit/tern/ui';
	import type { PageProps } from './$types';
	import { forms } from './$forms';

	let { data, form }: PageProps = $props();
	const done = $derived((form as { done?: string } | null)?.done);
	const recoveryCodes = $derived((form as { recoveryCodes?: string[] } | null)?.recoveryCodes);
	let deleteOpen = $state(false);
	let deleteForm = $state<HTMLFormElement>();
</script>

<svelte:head><title>Your account · Accounts</title></svelte:head>

<h1>Your account</h1>

<section aria-labelledby="profile">
	<h2 id="profile">Profile</h2>
	<p class="email">
		{data.user.email}
		{#if data.user.emailVerified}<Badge tone="success">Confirmed</Badge>{:else}<Badge tone="warning">Not confirmed</Badge>{/if}
	</p>
	{#if !data.user.emailVerified}
		<Form action="?/verify" reset={false}>
			<div><Button type="submit" variant="secondary" size="sm">Send the confirmation email again</Button></div>
		</Form>
		{#if done === 'verify'}<Alert tone="success" live>Sent. Open the link in the <a href="/outbox">outbox</a>.</Alert>{/if}
	{/if}
	<Form action="?/name" rules={forms.name} {form} reset={false}>
		<Field label="Name" name="name"><Input value={data.user.name} autocomplete="name" /></Field>
		<div><Button type="submit" size="sm">Save name</Button></div>
	</Form>
</section>

<section aria-labelledby="email-change">
	<h2 id="email-change">Email address</h2>
	<Form action="?/email" rules={forms.email} {form}>
		<Field label="New email address" name="email"><Input type="email" autocomplete="email" spellcheck="false" /></Field>
		<div><Button type="submit" size="sm">Change email address</Button></div>
	</Form>
	{#if done === 'email'}<Alert tone="success" live>Check the new address for a link to confirm it.</Alert>{/if}
</section>

<section aria-labelledby="password">
	<h2 id="password">Password</h2>
	<Form action="?/password" rules={forms.password} {form}>
		{#if data.user.hasPassword}
			<Field label="Current password" name="current"><PasswordInput autocomplete="current-password" /></Field>
		{/if}
		<Field label="New password" name="password" description="At least 10 characters. Your other devices are signed out.">
			<PasswordInput autocomplete="new-password" />
		</Field>
		<div><Button type="submit" size="sm">Change password</Button></div>
	</Form>
	{#if done === 'password'}<Alert tone="success" live>Password changed.</Alert>{/if}
</section>

<TwoStep
	enabled={data.user.twoStep}
	setup={data.setup}
	{recoveryCodes}
	codesLeft={data.codesLeft}
	setupAction="?/twoStepSetup"
	confirmAction="?/twoStepConfirm"
	offAction="?/twoStepOff"
	codesAction="?/twoStepCodes"
	{form}
/>

<Passkeys passkeys={data.passkeys} removeAction="?/removePasskey" />

<Devices sessions={data.sessions} action="?/revoke" othersAction="?/others" />

<section aria-labelledby="danger">
	<h2 id="danger">Delete your account</h2>
	<p>This deletes your account, its passkeys and sessions. It can't be undone.</p>
	<form method="POST" action="?/delete" bind:this={deleteForm}>
		<Button type="button" variant="danger" size="sm" onclick={() => (deleteOpen = true)}>Delete account</Button>
	</form>
	<AlertDialog
		bind:open={deleteOpen}
		title="Delete your account?"
		description="Your account, passkeys and sessions are deleted for good."
		confirmLabel="Delete account"
		danger
		onconfirm={() => deleteForm?.requestSubmit()}
	/>
</section>

<style>
	section,
	:global(.tern-account) {
		margin-block: 2.5rem;
	}
	h2 {
		margin: 0 0 1rem;
		font-size: 1.0625rem;
	}
	.email {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}
	section :global(.tern-form) {
		max-width: 24rem;
		margin-block: 1rem;
	}
</style>

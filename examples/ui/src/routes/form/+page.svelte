<script lang="ts">
	import { Alert, Button, Card, Checkbox, Field, Form, Input, PasswordInput, Select } from '@ternkit/tern/ui';
	import type { PageProps } from './$types';
	import { forms } from './$forms';

	let { form }: PageProps = $props();
	let sending = $state(false);
	const welcome = $derived(form && 'email' in form && !('errors' in form) ? form : null);
</script>

<svelte:head><title>Form · Tern UI</title></svelte:head>

<h1>Sign up</h1>
<p class="lead">
	The rules are tags on a Go struct. The browser checks them as you type, the server checks them again, and the form works without JavaScript.
</p>

<Card as="section" title="Create your account" level={2}>
	{#if welcome}
		<Alert tone="success" title="You're in" live>Welcome, {welcome.name}. We sent a confirmation to {welcome.email}.</Alert>
	{:else}
		<Form rules={forms.default} {form} bind:submitting={sending} reset={false} class="signup">
			<Field label="Name" name="name">
				<Input autocomplete="name" />
			</Field>
			<Field label="Email" name="email" description="We'll only email you about your account.">
				<Input type="email" autocomplete="email" />
			</Field>
			<Field label="Password" name="password" description="At least 10 characters.">
				<PasswordInput autocomplete="new-password" />
			</Field>
			<Field label="Confirm password" name="confirm">
				<PasswordInput autocomplete="new-password" />
			</Field>
			<Field label="Role" name="role">
				<Select
					placeholder="Choose one"
					options={[
						{ value: 'dev', label: 'Developer' },
						{ value: 'ops', label: 'Operations' },
						{ value: 'other', label: 'Something else' }
					]}
				/>
			</Field>
			<Field label="Website" name="website" description="Optional.">
				<Input type="url" placeholder="https://" />
			</Field>
			<Checkbox name="terms">I accept the terms</Checkbox>
			<Button type="submit" loading={sending}>Create account</Button>
		</Form>
	{/if}
</Card>

<style>
	h1 {
		margin: 0;
		font-size: 1.75rem;
		letter-spacing: -0.02em;
	}
	.lead {
		color: var(--tern-muted);
		margin: 0.5rem 0 1.5rem;
	}
	:global(.signup) {
		max-width: 26rem;
	}
</style>

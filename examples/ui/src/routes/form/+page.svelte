<script lang="ts">
	import { enhance } from '@ternkit/tern';
	import { Alert, Button, Card, Checkbox, Field, Input, Select } from '@ternkit/tern/ui';
	import type { PageProps } from './$types';

	let { form }: PageProps = $props();
	let sending = $state(false);
	const errors = $derived(form?.errors ?? {});
	const values = $derived(form?.values ?? {});
</script>

<svelte:head><title>Form · Tern UI</title></svelte:head>

<h1>Sign up</h1>
<p class="lead">The form works without JavaScript. Go validates it, and the fields show the errors and announce them.</p>

<Card as="section" title="Create your account" level={2}>
	{#if form?.done}
		<Alert tone="success" title="You're in" live>Welcome, {values.name}. We sent a confirmation to {values.email}.</Alert>
	{:else}
		<form
			method="POST"
			novalidate
			use:enhance={() => {
				sending = true;
				return async ({ update }) => {
					await update({ reset: false });
					sending = false;
				};
			}}
		>
			{#if Object.keys(errors).length}
				<Alert tone="danger" title="Check the form" live>{Object.keys(errors).length} fields need your attention.</Alert>
			{/if}
			<Field label="Name" error={errors.name} required>
				<Input name="name" value={values.name ?? ''} autocomplete="name" />
			</Field>
			<Field label="Email" description="We'll only email you about your account." error={errors.email} required>
				<Input name="email" type="email" value={values.email ?? ''} autocomplete="email" />
			</Field>
			<Field label="Role" error={errors.role} required>
				<Select
					name="role"
					value={values.role ?? ''}
					placeholder="Choose one"
					options={[
						{ value: 'dev', label: 'Developer' },
						{ value: 'ops', label: 'Operations' },
						{ value: 'other', label: 'Something else' }
					]}
				/>
			</Field>
			<div>
				<Checkbox name="terms" aria-invalid={errors.terms ? true : undefined} aria-describedby={errors.terms ? 'terms-error' : undefined}>I accept the terms</Checkbox>
				{#if errors.terms}<p class="error" id="terms-error">{errors.terms}</p>{/if}
			</div>
			<Button type="submit" loading={sending}>Create account</Button>
		</form>
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
	form {
		display: grid;
		gap: 1rem;
		max-width: 26rem;
	}
	.error {
		margin: 0.35rem 0 0;
		color: var(--tern-danger);
		font-size: 0.8125rem;
		font-weight: 500;
	}
</style>

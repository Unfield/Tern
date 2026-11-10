<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { FormField } from '../runtime/validate.js';
	import Button from './Button.svelte';
	import Checkbox from './Checkbox.svelte';
	import Field from './Field.svelte';
	import Form from './Form.svelte';
	import Input from './Input.svelte';
	import PasswordInput from './PasswordInput.svelte';

	type Props = {
		title?: string;
		description?: string;
		level?: 1 | 2 | 3;
		action?: string;
		/** The sign-up action's rules, from ./$forms; they also set the password hint. */
		rules?: FormField[];
		form?: unknown;
		/** Ask for a name, posted as name. */
		name?: boolean;
		/** Ask to type the password twice, posted as confirm. */
		confirm?: boolean;
		/** Links to the terms; then a checkbox, posted as terms, must be checked. */
		termsHref?: string;
		signInHref?: string;
		footer?: Snippet;
		class?: string;
	};
	let { title = 'Create an account', description, level = 1, action, rules, form, name = true, confirm = false, termsHref, signInHref, footer, class: klass = '' }: Props = $props();

	let submitting = $state(false);
	const minPassword = $derived(rules?.find((f) => f.name === 'password')?.rules?.find((r) => r.name === 'min')?.arg);
</script>

<section class="tern-auth {klass}" aria-labelledby="tern-signup-title">
	<header>
		<svelte:element this={`h${level}`} id="tern-signup-title">{title}</svelte:element>
		{#if description}<p>{description}</p>{/if}
	</header>
	<Form {rules} {form} {action} bind:submitting reset={false}>
		{#if name}
			<Field label="Name" name="name"><Input autocomplete="name" /></Field>
		{/if}
		<Field label="Email" name="email"><Input type="email" autocomplete="email" spellcheck="false" /></Field>
		<Field label="Password" name="password" description={minPassword ? `At least ${minPassword} characters.` : undefined}>
			<PasswordInput autocomplete="new-password" />
		</Field>
		{#if confirm}
			<Field label="Confirm password" name="confirm"><PasswordInput autocomplete="new-password" /></Field>
		{/if}
		{#if termsHref}
			<Checkbox name="terms">I agree to the <a href={termsHref}>terms</a></Checkbox>
		{/if}
		<Button type="submit" loading={submitting}>Create account</Button>
	</Form>
	{#if signInHref}<p class="switch">Already have an account? <a href={signInHref}>Sign in</a></p>{/if}
	{#if footer}<div class="footer">{@render footer()}</div>{/if}
</section>

<style>
	.tern-auth {
		display: grid;
		gap: 1.25rem;
		width: 100%;
		max-width: 24rem;
		box-sizing: border-box;
		padding: 2rem;
		color: var(--tern-fg);
		background: var(--tern-surface);
		border: 1px solid var(--tern-border);
		border-radius: calc(var(--tern-radius) + 4px);
		box-shadow: var(--tern-shadow);
	}
	header {
		display: grid;
		gap: 0.35rem;
	}
	:is(h1, h2, h3) {
		margin: 0;
		font-size: 1.375rem;
		font-weight: 650;
		letter-spacing: -0.02em;
	}
	header p {
		margin: 0;
		color: var(--tern-muted);
		font-size: 0.875rem;
	}
	a {
		color: var(--tern-accent);
		font-weight: 550;
	}
	.switch {
		margin: 0;
		text-align: center;
		font-size: 0.875rem;
		color: var(--tern-muted);
	}
	.footer {
		font-size: 0.8125rem;
		color: var(--tern-muted);
	}
	.tern-auth :global(.tern-form > .tern-btn) {
		width: 100%;
	}
</style>

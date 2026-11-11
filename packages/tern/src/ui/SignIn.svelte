<script lang="ts">
	import { tb } from '../runtime/i18n.svelte.js';
	import type { Snippet } from 'svelte';
	import type { FormField } from '../runtime/validate.js';
	import Alert from './Alert.svelte';
	import Button from './Button.svelte';
	import Checkbox from './Checkbox.svelte';
	import Field from './Field.svelte';
	import Form from './Form.svelte';
	import Input from './Input.svelte';
	import PasswordInput from './PasswordInput.svelte';
	import Separator from './Separator.svelte';

	type Props = {
		title?: string;
		description?: string;
		/** The heading level; 1 when this is the page's main content. */
		level?: 1 | 2 | 3;
		/** Providers from plugins/oauth, such as plugin.Providers(): each links to /auth/<name>/login. */
		providers?: { name: string; label: string }[];
		/** Where plugins/oauth lives. */
		authPath?: string;
		/** Where to go after signing in, a path on this site. */
		next?: string;
		/** Show the email and password form, which posts to action. */
		password?: boolean;
		action?: string;
		/** The sign-in action's rules, from ./$forms. */
		rules?: FormField[];
		/** The page's form prop, for the server's errors. */
		form?: unknown;
		/** The ?error= code plugins/oauth sends back after a failed sign-in. */
		error?: string | null;
		/** A "Keep me signed in" checkbox, posted as remember. */
		remember?: boolean;
		/** Where to reset a forgotten password. */
		forgotHref?: string;
		/** Where to create an account. */
		signUpHref?: string;
		/** More content under the form, such as legal text. */
		footer?: Snippet;
		class?: string;
	};
	let {
		title: titleProp,
		description,
		level = 1,
		providers = [],
		authPath = '/auth',
		next,
		password = true,
		action,
		rules,
		form,
		error,
		remember = false,
		forgotHref,
		signUpHref,
		footer,
		class: klass = ''
	}: Props = $props();
	const title = $derived(titleProp ?? tb('tern.ui.signIn.title'));

	let submitting = $state(false);
	const errorText = $derived(
		!error ? null : error === 'access_denied' ? tb('tern.ui.signIn.cancelled') : tb('tern.ui.signIn.failed')
	);
	const login = (name: string) => `${authPath}/${encodeURIComponent(name)}/login${next ? `?next=${encodeURIComponent(next)}` : ''}`;
</script>

<section class="tern-auth {klass}" aria-labelledby="tern-signin-title">
	<header>
		<svelte:element this={`h${level}`} id="tern-signin-title">{title}</svelte:element>
		{#if description}<p>{description}</p>{/if}
	</header>
	{#if errorText}<Alert tone="danger" live>{errorText}</Alert>{/if}
	{#if providers.length}
		<div class="providers">
			{#each providers as p (p.name)}
				<Button variant="secondary" href={login(p.name)}>{tb('tern.ui.signIn.continueWith', { provider: p.label })}</Button>
			{/each}
		</div>
	{/if}
	{#if providers.length && password}<div class="or"><Separator /><span>{tb('tern.ui.signIn.or')}</span><Separator /></div>{/if}
	{#if password}
		<Form {rules} {form} {action} bind:submitting reset={false}>
			{#if next}<input type="hidden" name="next" value={next} />{/if}
			<Field label={tb('tern.ui.signIn.email')} name="email">
				<Input type="email" autocomplete="username" spellcheck="false" />
			</Field>
			<Field label={tb('tern.ui.signIn.password')} name="password">
				<PasswordInput autocomplete="current-password" />
			</Field>
			<div class="row">
				{#if remember}<Checkbox name="remember">{tb('tern.ui.signIn.remember')}</Checkbox>{/if}
				{#if forgotHref}<a href={forgotHref}>{tb('tern.ui.signIn.forgot')}</a>{/if}
			</div>
			<Button type="submit" loading={submitting}>{tb('tern.ui.signIn.submit')}</Button>
		</Form>
	{/if}
	{#if signUpHref}<p class="switch">{tb('tern.ui.signIn.new')} <a href={signUpHref}>{tb('tern.ui.signIn.signUp')}</a></p>{/if}
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
	.providers {
		display: grid;
		gap: 0.5rem;
	}
	.providers :global(.tern-btn) {
		width: 100%;
	}
	.or {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		color: var(--tern-muted);
		font-size: 0.8125rem;
	}
	.or :global(.tern-separator) {
		flex: 1;
	}
	.row {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: space-between;
		gap: 0.5rem;
		font-size: 0.875rem;
	}
	.row:empty {
		display: none;
	}
	a {
		color: var(--tern-accent);
		font-weight: 550;
	}
	a:focus-visible {
		outline: 2px solid var(--tern-ring);
		outline-offset: 2px;
		border-radius: 2px;
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

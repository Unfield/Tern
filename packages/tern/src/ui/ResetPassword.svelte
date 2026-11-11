<script lang="ts">
	import { tb } from '../runtime/i18n.svelte.js';
	import type { FormField } from '../runtime/validate.js';
	import Alert from './Alert.svelte';
	import AuthCard from './AuthCard.svelte';
	import Button from './Button.svelte';
	import Field from './Field.svelte';
	import Form from './Form.svelte';
	import PasswordInput from './PasswordInput.svelte';

	type Props = {
		title?: string;
		description?: string;
		level?: 1 | 2 | 3;
		/** The token from the email's link (?token=), posted as token. */
		token: string;
		/** The action that calls acc.ResetPassword with the posted token and password. */
		action?: string;
		/** Its rules; they also set the password hint. */
		rules?: FormField[];
		form?: unknown;
		/** Ask to type the password twice, posted as confirm. */
		confirm?: boolean;
		/** The link has been found invalid or expired: say so instead of showing the form. */
		invalid?: boolean;
		/** Where to ask for a new link. */
		forgotHref?: string;
		class?: string;
	};
	let { title, description, level = 1, token, action, rules, form, confirm = true, invalid = false, forgotHref, class: klass = '' }: Props = $props();

	let submitting = $state(false);
	const minPassword = $derived(rules?.find((f) => f.name === 'password')?.rules?.find((r) => r.name === 'min')?.arg);
</script>

<AuthCard id="tern-reset-title" title={title ?? tb('tern.ui.resetPassword.title')} {description} {level} class={klass}>
	{#if invalid || !token}
		<Alert tone="danger">{tb('tern.ui.resetPassword.invalid')}</Alert>
		{#if forgotHref}<Button variant="secondary" href={forgotHref}>{tb('tern.ui.resetPassword.again')}</Button>{/if}
	{:else}
		<Form {rules} {form} {action} bind:submitting reset={false}>
			<input type="hidden" name="token" value={token} />
			<Field label={tb('tern.ui.resetPassword.password')} name="password" description={minPassword ? tb('tern.ui.signUp.minPassword', { n: minPassword }) : undefined}>
				<PasswordInput autocomplete="new-password" />
			</Field>
			{#if confirm}
				<Field label={tb('tern.ui.resetPassword.confirm')} name="confirm">
					<PasswordInput autocomplete="new-password" />
				</Field>
			{/if}
			<Button type="submit" loading={submitting}>{tb('tern.ui.resetPassword.submit')}</Button>
		</Form>
	{/if}
</AuthCard>

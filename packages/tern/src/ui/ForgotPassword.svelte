<script lang="ts">
	import { tb } from '../runtime/i18n.svelte.js';
	import type { FormField } from '../runtime/validate.js';
	import Alert from './Alert.svelte';
	import AuthCard from './AuthCard.svelte';
	import Button from './Button.svelte';
	import Field from './Field.svelte';
	import Form from './Form.svelte';
	import Input from './Input.svelte';

	type Props = {
		title?: string;
		description?: string;
		level?: 1 | 2 | 3;
		/** The action that calls acc.RequestPasswordReset with the posted email. */
		action?: string;
		rules?: FormField[];
		/** The page's form prop. An action returning { sent: true } shows that the email is on its way. */
		form?: unknown;
		signInHref?: string;
		class?: string;
	};
	let { title, description, level = 1, action, rules, form, signInHref, class: klass = '' }: Props = $props();

	let submitting = $state(false);
	let sent = $state(false);
	// Without JavaScript, the action returns { sent: true } and it shows through the form prop.
	const done = $derived(sent || (!!form && typeof form === 'object' && (form as { sent?: unknown }).sent === true));
</script>

<AuthCard id="tern-forgot-title" title={title ?? tb('tern.ui.forgotPassword.title')} description={done ? undefined : (description ?? tb('tern.ui.forgotPassword.description'))} {level} class={klass}>
	{#if done}
		<Alert tone="success" live>{tb('tern.ui.forgotPassword.sent')}</Alert>
	{:else}
		<Form {rules} {form} {action} bind:submitting reset={false} onsuccess={() => (sent = true)}>
			<Field label={tb('tern.ui.forgotPassword.email')} name="email">
				<Input type="email" autocomplete="username" spellcheck="false" />
			</Field>
			<Button type="submit" loading={submitting}>{tb('tern.ui.forgotPassword.submit')}</Button>
		</Form>
	{/if}
	{#if signInHref}<p class="tern-auth-note"><a href={signInHref}>{tb('tern.ui.forgotPassword.back')}</a></p>{/if}
</AuthCard>

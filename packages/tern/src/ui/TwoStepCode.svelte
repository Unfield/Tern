<script lang="ts">
	import { tb } from '../runtime/i18n.svelte.js';
	import type { FormField } from '../runtime/validate.js';
	import AuthCard from './AuthCard.svelte';
	import Button from './Button.svelte';
	import Field from './Field.svelte';
	import Form from './Form.svelte';
	import Input from './Input.svelte';
	import InputOTP from './InputOTP.svelte';

	type Props = {
		title?: string;
		description?: string;
		level?: 1 | 2 | 3;
		/** The action that calls acc.CompleteSignIn with the posted code. */
		action?: string;
		rules?: FormField[];
		form?: unknown;
		/** Where to go back to sign in. */
		signInHref?: string;
		class?: string;
	};
	let { title, description, level = 1, action, rules, form, signInHref, class: klass = '' }: Props = $props();

	let submitting = $state(false);
	let recovery = $state(false);
	let el = $state<HTMLElement>();
	// A full code posts the form, as typing the last digit should.
	const submit = () => el?.querySelector('form')?.requestSubmit();
</script>

<div bind:this={el} style="display: contents">
	<AuthCard id="tern-two-step-title" title={title ?? tb('tern.ui.twoStep.codeTitle')} description={description ?? (recovery ? tb('tern.ui.twoStep.recoveryDescription') : tb('tern.ui.twoStep.codeDescription'))} {level} class={klass}>
		<Form {rules} {form} {action} bind:submitting reset={false}>
			{#if recovery}
				<Field label={tb('tern.ui.twoStep.recoveryLabel')} name="code">
					<Input autocomplete="off" spellcheck="false" autocapitalize="off" />
				</Field>
			{:else}
				<Field label={tb('tern.ui.twoStep.codeLabel')} name="code">
					<InputOTP length={6} oncomplete={submit} />
				</Field>
			{/if}
			<Button type="submit" loading={submitting}>{tb('tern.ui.twoStep.verify')}</Button>
		</Form>
		<p class="tern-auth-note">
			<button type="button" class="link" onclick={() => (recovery = !recovery)}>
				{recovery ? tb('tern.ui.twoStep.useApp') : tb('tern.ui.twoStep.useRecovery')}
			</button>
		</p>
		{#if signInHref}<p class="tern-auth-note"><a href={signInHref}>{tb('tern.ui.forgotPassword.back')}</a></p>{/if}
	</AuthCard>
</div>

<style>
	.link {
		padding: 0;
		border: 0;
		background: none;
		color: var(--tern-accent);
		font: inherit;
		font-weight: 550;
		cursor: pointer;
	}
	.link:focus-visible {
		outline: 2px solid var(--tern-ring);
		outline-offset: 2px;
		border-radius: 2px;
	}
</style>

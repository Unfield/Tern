<script lang="ts">
	import { tb } from '../runtime/i18n.svelte.js';
	import AccountSection from './AccountSection.svelte';
	import Alert from './Alert.svelte';
	import Badge from './Badge.svelte';
	import Button from './Button.svelte';
	import Field from './Field.svelte';
	import Form from './Form.svelte';
	import Input from './Input.svelte';
	import InputOTP from './InputOTP.svelte';

	type Setup = { secret: string; uri: string; qr: string };
	type Props = {
		title?: string;
		description?: string;
		level?: 2 | 3 | 4;
		/** User.twoStep: whether it's on. */
		enabled: boolean;
		/** From acc.SetUpTwoStep or acc.PendingTwoStepSetup, while turning it on. */
		setup?: Setup | null;
		/** Recovery codes to show once: from acc.ConfirmTwoStep or acc.RecoveryCodes. */
		recoveryCodes?: string[] | null;
		/** How many recovery codes are left, from acc.RecoveryCodesLeft. */
		codesLeft?: number;
		/** The action that calls acc.SetUpTwoStep. */
		setupAction?: string;
		/** The action that calls acc.ConfirmTwoStep with the posted code. */
		confirmAction?: string;
		/** The action that calls acc.TurnOffTwoStep with the posted code. */
		offAction?: string;
		/** The action that calls acc.RecoveryCodes. */
		codesAction?: string;
		form?: unknown;
		class?: string;
	};
	let { title, description, level = 2, enabled, setup, recoveryCodes, codesLeft, setupAction, confirmAction, offAction, codesAction, form, class: klass = '' }: Props = $props();
	let turningOff = $state(false);
	let copied = $state(false);
	async function copy() {
		try {
			await navigator.clipboard.writeText((recoveryCodes ?? []).join('\n'));
			copied = true;
		} catch {
			copied = false;
		}
	}
</script>

<AccountSection id="tern-two-step-settings" title={title ?? tb('tern.ui.twoStep.title')} description={description ?? tb('tern.ui.twoStep.description')} {level} class={klass}>
	{#if recoveryCodes?.length}
		<Alert tone="warning" title={tb('tern.ui.twoStep.codesTitle')}>{tb('tern.ui.twoStep.codesHelp')}</Alert>
		<ol class="codes" data-testid="recovery-codes">
			{#each recoveryCodes as c (c)}<li><code>{c}</code></li>{/each}
		</ol>
		<div><Button type="button" variant="secondary" size="sm" onclick={copy}>{copied ? tb('tern.ui.twoStep.copied') : tb('tern.ui.twoStep.copy')}</Button></div>
	{/if}
	{#if enabled}
		<p class="status">
			<Badge tone="success">{tb('tern.ui.twoStep.on')}</Badge>
			{#if codesLeft !== undefined}<span class="tern-account-meta">{tb('tern.ui.twoStep.codesLeft', { n: codesLeft })}</span>{/if}
		</p>
		<div class="row">
			{#if codesAction}
				<Form action={codesAction} reset={false}><Button type="submit" variant="secondary" size="sm">{tb('tern.ui.twoStep.newCodes')}</Button></Form>
			{/if}
			{#if offAction && !turningOff}
				<Button type="button" variant="ghost" size="sm" onclick={() => (turningOff = true)}>{tb('tern.ui.twoStep.turnOff')}</Button>
			{/if}
		</div>
		{#if offAction && turningOff}
			<Form action={offAction} {form} reset={false}>
				<Field label={tb('tern.ui.twoStep.offLabel')} name="code"><Input autocomplete="one-time-code" spellcheck="false" /></Field>
				<div class="row">
					<Button type="submit" variant="danger" size="sm">{tb('tern.ui.twoStep.turnOff')}</Button>
					<Button type="button" variant="ghost" size="sm" onclick={() => (turningOff = false)}>{tb('tern.ui.alertDialog.cancel')}</Button>
				</div>
			</Form>
		{/if}
	{:else if setup}
		<ol class="steps">
			<li>
				{tb('tern.ui.twoStep.scan')}
				<img src={setup.qr} alt={tb('tern.ui.twoStep.qrAlt')} width="200" height="200" />
				<details>
					<summary>{tb('tern.ui.twoStep.cantScan')}</summary>
					<p><code data-testid="two-step-secret">{setup.secret}</code></p>
					<p><a href={setup.uri}>{tb('tern.ui.twoStep.openApp')}</a></p>
				</details>
			</li>
			<li>
				<Form action={confirmAction} {form} reset={false}>
					<Field label={tb('tern.ui.twoStep.codeLabel')} name="code"><InputOTP length={6} /></Field>
					<div><Button type="submit" size="sm">{tb('tern.ui.twoStep.turnOn')}</Button></div>
				</Form>
			</li>
		</ol>
	{:else if setupAction}
		<Form action={setupAction} reset={false}>
			<div><Button type="submit" variant="secondary" size="sm">{tb('tern.ui.twoStep.setUp')}</Button></div>
		</Form>
	{/if}
</AccountSection>

<style>
	.codes {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(8rem, 1fr));
		gap: 0.5rem;
		margin: 0;
		padding: 1rem 1rem 1rem 2.25rem;
		border: 1px solid var(--tern-border);
		border-radius: var(--tern-radius);
		background: var(--tern-surface);
	}
	.codes code,
	.steps code {
		font-family: var(--tern-mono, ui-monospace, monospace);
		font-size: 0.9375rem;
	}
	.status,
	.row {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.75rem;
		margin: 0;
	}
	.steps {
		display: grid;
		gap: 1.25rem;
		margin: 0;
		padding-left: 1.25rem;
	}
	.steps li {
		display: grid;
		gap: 0.75rem;
		justify-items: start;
	}
	.steps img {
		border-radius: var(--tern-radius);
		background: #fff;
		padding: 0.5rem;
		image-rendering: pixelated;
	}
	details {
		font-size: 0.875rem;
		color: var(--tern-muted);
	}
	summary {
		cursor: pointer;
	}
	.steps a {
		color: var(--tern-accent);
	}
</style>

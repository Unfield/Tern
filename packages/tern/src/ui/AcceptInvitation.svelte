<script lang="ts">
	import { tb } from '../runtime/i18n.svelte.js';
	import Alert from './Alert.svelte';
	import AuthCard from './AuthCard.svelte';
	import Button from './Button.svelte';
	import Form from './Form.svelte';

	type Invitation = { teamName: string; email: string; role: string; invitedBy: string; expiresAt: string };
	type Props = {
		title?: string;
		level?: 1 | 2 | 3;
		/** From acc.InvitationFor, or null when the link is invalid or expired. */
		invitation: Invitation | null;
		/** The link's token, posted to the action. */
		token: string;
		/** The signed-in user's email, or null when signed out. */
		email: string | null;
		/** The action that calls acc.AcceptInvitation with the posted token. */
		action?: string;
		/** Sign-in and sign-up pages, for signed-out visitors: ?next= brings them back here. */
		signInHref?: string;
		signUpHref?: string;
		/** An action that signs out, offered when the user is signed in with another address. */
		signOutAction?: string;
		/** Posted to signOutAction as next: this page's address, to come back and sign in with the invited one. */
		signOutNext?: string;
		/** The role's label, when it's one of the app's own. */
		roleLabel?: string;
		/** The page's form prop, for the action's errors. */
		form?: unknown;
		class?: string;
	};
	let { title, level = 1, invitation, token, email, action = '?/accept', signInHref, signUpHref, signOutAction, signOutNext, roleLabel, form, class: klass = '' }: Props = $props();

	let submitting = $state(false);
	const role = $derived(roleLabel ?? (invitation ? tb(`tern.ui.team.role.${invitation.role}`) : ''));
	const heading = $derived(title ?? (invitation ? tb('tern.ui.team.accept.title', { team: invitation.teamName }) : tb('tern.ui.team.accept.invalidTitle')));
	const wrongAccount = $derived(!!invitation && !!email && email.toLowerCase() !== invitation.email.toLowerCase());
</script>

<AuthCard
	id="tern-accept-title"
	title={heading}
	description={invitation ? tb('tern.ui.team.accept.description', { inviter: invitation.invitedBy || invitation.teamName, team: invitation.teamName, role: role.toLowerCase() }) : tb('tern.ui.team.accept.invalid')}
	{level}
	class={klass}
>
	{#if invitation}
		{#if !email}
			<p class="tern-auth-note">{tb('tern.ui.team.accept.signedOut', { email: invitation.email })}</p>
			{#if signUpHref}<div class="tern-accept-cta"><Button href={signUpHref}>{tb('tern.ui.team.accept.signUp')}</Button></div>{/if}
			{#if signInHref}<p class="tern-auth-note"><a href={signInHref}>{tb('tern.ui.team.accept.signIn')}</a></p>{/if}
		{:else if wrongAccount}
			<Alert tone="warning">{tb('tern.ui.team.accept.wrongAccount', { email: invitation.email, current: email })}</Alert>
			{#if signOutAction}
				<form method="POST" action={signOutAction} class="tern-accept-cta">
					{#if signOutNext}<input type="hidden" name="next" value={signOutNext} />{/if}
					<Button type="submit" variant="secondary">{tb('tern.ui.team.accept.signOut')}</Button>
				</form>
			{/if}
		{:else}
			<Form {action} {form} bind:submitting reset={false}>
				<input type="hidden" name="token" value={token} />
				<Button type="submit" loading={submitting}>{tb('tern.ui.team.accept.submit', { team: invitation.teamName })}</Button>
			</Form>
		{/if}
	{/if}
</AuthCard>

<style>
	.tern-accept-cta {
		display: grid;
	}
</style>

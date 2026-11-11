<script lang="ts">
	import { tb } from '../runtime/i18n.svelte.js';
	import AccountSection from './AccountSection.svelte';
	import Alert from './Alert.svelte';
	import Button from './Button.svelte';
	import Field from './Field.svelte';
	import Form from './Form.svelte';
	import Input from './Input.svelte';
	import Select from './Select.svelte';
	import { when } from './accountDates.js';
	import { builtinRoles, formFor, grantable, roleLabel, type RoleOption } from './teams.js';

	type Invitation = { id: string; email: string; role: string; invitedBy: string; createdAt: string; expiresAt: string };
	type Props = {
		title?: string;
		description?: string;
		level?: 2 | 3 | 4;
		/** From acc.Invitations. */
		invitations: Invitation[];
		/** The signed-in user's role in the team: admins can't invite owners. */
		role: string;
		/** The roles to offer, with labels. The default is owner, admin and member. */
		roles?: RoleOption[];
		/** The role chosen at first. */
		defaultRole?: string;
		/** The action that calls acc.Invite with the posted email and role. */
		inviteAction: string;
		/** The action that calls acc.CancelInvitation with the posted id. */
		cancelAction?: string;
		/** The page's form prop, for the invite form's errors. */
		form?: unknown;
		class?: string;
	};
	let { title, description, level = 2, invitations, role, roles, defaultRole = 'member', inviteAction, cancelAction, form, class: klass = '' }: Props = $props();

	const all = $derived(roles ?? builtinRoles());
	const offer = $derived(grantable(all, role));
	const mine = $derived(formFor(form, 'invitations'));
	const sent = $derived((form as { invited?: string } | null)?.invited);
</script>

<AccountSection id="tern-invitations-title" title={title ?? tb('tern.ui.team.invite.title')} description={description ?? tb('tern.ui.team.invite.description')} {level} class={klass}>
	<div class="tern-invite">
		<Form action={inviteAction} form={mine}>
			<input type="hidden" name="tern_form" value="invitations" />
			<Field label={tb('tern.ui.team.invite.email')} name="email"><Input type="email" autocomplete="off" spellcheck="false" required /></Field>
			<Field label={tb('tern.ui.team.invite.role')} name="role"><Select options={offer} value={defaultRole} /></Field>
			<div class="tern-invite-submit"><Button type="submit">{tb('tern.ui.team.invite.send')}</Button></div>
		</Form>
	</div>
	{#if sent}<Alert tone="success" live>{tb('tern.ui.team.invite.sent', { email: sent })}</Alert>{/if}
	{#if invitations.length}
		<ul aria-label={tb('tern.ui.team.invite.pending')}>
			{#each invitations as inv (inv.id)}
				<li>
					<div class="tern-account-text">
						<span class="tern-account-name">{inv.email}</span>
						<span class="tern-account-meta">{roleLabel(all, inv.role)} · {tb('tern.ui.team.invite.expires', { when: when(inv.expiresAt) })}</span>
					</div>
					{#if cancelAction}
						<Form action={cancelAction} reset={false}>
							<input type="hidden" name="tern_form" value="invitations" />
							<input type="hidden" name="id" value={inv.id} />
							<Button type="submit" variant="ghost" size="sm" aria-label={tb('tern.ui.team.invite.cancelFor', { email: inv.email })}>{tb('tern.ui.team.invite.cancel')}</Button>
						</Form>
					{/if}
				</li>
			{/each}
		</ul>
	{/if}
</AccountSection>

<style>
	/* The Form is display: contents in an account section: its fields are this grid's. */
	.tern-invite {
		display: grid;
		grid-template-columns: minmax(0, 1fr) minmax(8rem, 12rem) auto;
		align-items: start;
		gap: 0.75rem;
	}
	.tern-invite :global([id^='tern-form-']) {
		grid-column: 1 / -1;
	}
	.tern-invite-submit {
		display: flex;
		/* Lines the button up with the inputs, under their labels. */
		padding-top: 1.625rem;
	}
	@media (max-width: 36rem) {
		.tern-invite {
			grid-template-columns: 1fr;
		}
		.tern-invite-submit {
			padding-top: 0;
		}
	}
</style>

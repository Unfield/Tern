<script lang="ts">
	import { tb } from '../runtime/i18n.svelte.js';
	import AccountSection from './AccountSection.svelte';
	import Alert from './Alert.svelte';
	import AlertDialog from './AlertDialog.svelte';
	import Button from './Button.svelte';
	import Field from './Field.svelte';
	import Form from './Form.svelte';
	import Input from './Input.svelte';
	import { formFor, manages, messageFor } from './teams.js';

	type Props = {
		title?: string;
		description?: string;
		level?: 2 | 3 | 4;
		/** The current team, from accounts.CurrentTeam. */
		team: { id: number; name: string };
		/** The signed-in user's role in it: admins rename, owners also delete. */
		role: string;
		/** The action that calls acc.RenameTeam with the posted name. */
		renameAction?: string;
		/** The action that calls acc.DeleteTeam. Deleting asks to type the team's name first. */
		deleteAction?: string;
		/** The page's form prop, for the actions' errors. */
		form?: unknown;
		class?: string;
	};
	let { title, description, level = 2, team, role, renameAction, deleteAction, form, class: klass = '' }: Props = $props();

	let deleteOpen = $state(false);
	let deleteForm = $state<HTMLFormElement>();
	let typed = $state('');
	const message = $derived(messageFor(form, 'team-delete'));
</script>

<AccountSection id="tern-team-settings-title" title={title ?? tb('tern.ui.team.settings.title')} description={description} {level} class={klass}>
	{#if renameAction && manages(role)}
		<Form action={renameAction} form={formFor(form, 'team-rename')} reset={false}>
			<input type="hidden" name="tern_form" value="team-rename" />
			<Field label={tb('tern.ui.team.settings.name')} name="name"><Input value={team.name} required maxlength={100} autocomplete="organization" /></Field>
			<div><Button type="submit" size="sm">{tb('tern.ui.team.settings.rename')}</Button></div>
		</Form>
	{/if}
	{#if deleteAction && role === 'owner'}
		<div class="tern-team-danger">
			<div class="tern-account-text">
				<span class="tern-account-name">{tb('tern.ui.team.settings.delete')}</span>
				<span class="tern-account-meta">{tb('tern.ui.team.settings.deleteHelp')}</span>
			</div>
			<form method="POST" action={deleteAction} bind:this={deleteForm}>
				<input type="hidden" name="tern_form" value="team-delete" />
				<Button type="button" variant="danger" size="sm" onclick={() => ((typed = ''), (deleteOpen = true))}>{tb('tern.ui.team.settings.deleteButton')}</Button>
			</form>
		</div>
		{#if message}<Alert tone="danger" live>{message}</Alert>{/if}
		<AlertDialog
			bind:open={deleteOpen}
			title={tb('tern.ui.team.settings.deleteConfirm', { team: team.name })}
			description={tb('tern.ui.team.settings.deleteWarning')}
			confirmLabel={tb('tern.ui.team.settings.deleteButton')}
			danger
			confirmDisabled={typed.trim() !== team.name.trim()}
			onconfirm={() => deleteForm?.requestSubmit()}
		>
			<Field label={tb('tern.ui.team.settings.typeName', { team: team.name })} name="confirm-team"><Input bind:value={typed} autocomplete="off" spellcheck="false" /></Field>
		</AlertDialog>
	{/if}
</AccountSection>

<style>
	.tern-team-danger {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 1rem;
		padding: 0.75rem 1rem;
		border: 1px solid var(--tern-danger-soft);
		border-radius: var(--tern-radius);
	}
	.tern-team-danger form {
		flex-shrink: 0;
	}
	@media (max-width: 30rem) {
		.tern-team-danger {
			flex-wrap: wrap;
		}
	}
</style>

<script lang="ts">
	import { tb } from '../runtime/i18n.svelte.js';
	import AccountSection from './AccountSection.svelte';
	import Alert from './Alert.svelte';
	import Badge from './Badge.svelte';
	import Button from './Button.svelte';
	import Form from './Form.svelte';
	import Select from './Select.svelte';
	import { when } from './accountDates.js';
	import { builtinRoles, grantable, messageFor, roleLabel, type RoleOption } from './teams.js';

	type Member = { userId: number; email: string; name: string; role: string; joinedAt: string };
	type Props = {
		title?: string;
		description?: string;
		level?: 2 | 3 | 4;
		/** From acc.Members. */
		members: Member[];
		/** The signed-in user's ID. */
		me: number;
		/** The signed-in user's role in the team, from accounts.CurrentTeam. */
		role: string;
		/** The roles to offer, with labels. The default is owner, admin and member; add the app's own TeamRoles. */
		roles?: RoleOption[];
		/** The action that calls acc.SetRole with the posted user and role. */
		roleAction?: string;
		/** The action that calls acc.RemoveMember with the posted user. */
		removeAction?: string;
		/** The action that calls acc.LeaveTeam; then the user's own row has a Leave button. */
		leaveAction?: string;
		/** The page's form prop, to show why a change was refused, such as the last owner leaving. */
		form?: unknown;
		class?: string;
	};
	let { title, description, level = 2, members, me, role, roles, roleAction, removeAction, leaveAction, form, class: klass = '' }: Props = $props();

	const all = $derived(roles ?? builtinRoles());
	const offer = $derived(grantable(all, role));
	// What the signed-in user may do to a member, as accounts allows it.
	const canChange = (m: Member) => offer.length > 0 && (role === 'owner' || m.role !== 'owner');
	const message = $derived(messageFor(form, 'members'));
	// The last owner can't leave: accounts refuses, so don't offer it.
	const owners = $derived(members.filter((m) => m.role === 'owner').length);
</script>

<AccountSection id="tern-members-title" title={title ?? tb('tern.ui.team.members.title')} description={description ?? tb('tern.ui.team.members.description')} {level} class={klass}>
	{#if message}<Alert tone="danger" live>{message}</Alert>{/if}
	<ul>
		{#each members as m (m.userId)}
			<li>
				<div class="tern-account-text">
					<span class="tern-account-name">{m.name || m.email}{#if m.userId === me}<Badge>{tb('tern.ui.team.you')}</Badge>{/if}</span>
					<span class="tern-account-meta">{m.name ? `${m.email} · ` : ''}{tb('tern.ui.team.joined', { when: when(m.joinedAt) })}</span>
				</div>
				<div class="tern-team-actions">
					{#if roleAction && canChange(m)}
						<Form action={roleAction} reset={false}>
							<input type="hidden" name="tern_form" value="members" /><input type="hidden" name="user" value={m.userId} />
							<Select
								name="role"
								aria-label={tb('tern.ui.team.roleOf', { name: m.name || m.email })}
								value={m.role}
								options={m.role === 'owner' && role !== 'owner' ? all : offer}
								onchange={(e) => (e.currentTarget as HTMLSelectElement).form?.requestSubmit()}
							/>
							<noscript><Button type="submit" variant="secondary" size="sm">{tb('tern.ui.team.save')}</Button></noscript>
						</Form>
					{:else}
						<Badge tone={m.role === 'owner' ? 'accent' : 'neutral'}>{roleLabel(all, m.role)}</Badge>
					{/if}
					{#if m.userId === me && leaveAction && !(m.role === 'owner' && owners === 1)}
						<Form action={leaveAction} reset={false}>
							<input type="hidden" name="tern_form" value="members" />
							<Button type="submit" variant="ghost" size="sm">{tb('tern.ui.team.leave')}</Button>
						</Form>
					{:else if m.userId !== me && removeAction && canChange(m)}
						<Form action={removeAction} reset={false}>
							<input type="hidden" name="tern_form" value="members" /><input type="hidden" name="user" value={m.userId} />
							<Button type="submit" variant="ghost" size="sm" aria-label={tb('tern.ui.team.removeName', { name: m.name || m.email })}>{tb('tern.ui.team.remove')}</Button>
						</Form>
					{/if}
				</div>
			</li>
		{/each}
	</ul>
</AccountSection>

<style>
	.tern-team-actions {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		flex-shrink: 0;
	}
	@media (max-width: 30rem) {
		:global(.tern-account) li:has(.tern-team-actions) {
			flex-wrap: wrap;
		}
	}
</style>

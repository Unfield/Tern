<script lang="ts">
	import { tb } from '../runtime/i18n.svelte.js';
	import Badge from './Badge.svelte';
	import AccountSection from './AccountSection.svelte';
	import Button from './Button.svelte';
	import Form from './Form.svelte';
	import { when } from './accountDates.js';

	type Session = { id: string; device: string; ip: string; createdAt: string; lastSeen: string; current: boolean };
	type Props = {
		title?: string;
		description?: string;
		level?: 2 | 3 | 4;
		/** From acc.Sessions: the current one first. */
		sessions: Session[];
		/** The action that calls acc.Revoke with the posted id. */
		action?: string;
		/** The action that calls acc.SignOutOthers; then a button signs out every other device. */
		othersAction?: string;
		class?: string;
	};
	let { title, description, level = 2, sessions, action, othersAction, class: klass = '' }: Props = $props();
	const others = $derived(sessions.filter((s) => !s.current).length);
</script>

<AccountSection id="tern-devices-title" title={title ?? tb('tern.ui.devices.title')} description={description ?? tb('tern.ui.devices.description')} {level} class={klass}>
	<ul>
		{#each sessions as s (s.id)}
			<li>
				<div class="tern-account-text">
					<span class="tern-account-name">{s.device}{#if s.current}<Badge tone="success">{tb('tern.ui.devices.current')}</Badge>{/if}</span>
					<span class="tern-account-meta">{tb('tern.ui.devices.lastSeen', { when: when(s.lastSeen) })}{s.ip ? ` · ${s.ip}` : ''}</span>
				</div>
				{#if action}
					<Form {action} reset={false}>
						<input type="hidden" name="id" value={s.id} />
						<Button type="submit" variant={s.current ? 'secondary' : 'ghost'} size="sm">{tb('tern.ui.devices.signOut')}</Button>
					</Form>
				{/if}
			</li>
		{/each}
	</ul>
	{#if othersAction && others > 0}
		<div>
			<Form action={othersAction} reset={false}>
				<Button type="submit" variant="danger" size="sm">{tb('tern.ui.devices.signOutOthers')}</Button>
			</Form>
		</div>
	{/if}
</AccountSection>

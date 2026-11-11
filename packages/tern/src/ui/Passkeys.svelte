<script lang="ts">
	import { tb } from '../runtime/i18n.svelte.js';
	import { invalidateAll } from '../runtime/router.svelte.js';
	import { passkeysSupported, registerPasskey } from '../runtime/passkeys.js';
	import Alert from './Alert.svelte';
	import AccountSection from './AccountSection.svelte';
	import Button from './Button.svelte';
	import Form from './Form.svelte';
	import { when } from './accountDates.js';

	type Passkey = { id: string; name: string; createdAt: string; lastUsed: string | null };
	type Props = {
		title?: string;
		description?: string;
		level?: 2 | 3 | 4;
		/** From acc.Passkeys. */
		passkeys: Passkey[];
		/** The action that calls acc.RemovePasskey with the posted id. */
		removeAction?: string;
		/** Where the accounts package mounts its passkey endpoints. */
		path?: string;
		class?: string;
	};
	let { title, description, level = 2, passkeys, removeAction, path, class: klass = '' }: Props = $props();

	let supported = $state(true);
	$effect(() => {
		supported = passkeysSupported();
	});
	let adding = $state(false);
	let failed = $state(false);
	async function add() {
		adding = true;
		failed = false;
		try {
			await registerPasskey({ path });
			await invalidateAll();
		} catch (e) {
			if (!(e as { cancelled?: boolean }).cancelled) failed = true;
		} finally {
			adding = false;
		}
	}
</script>

<AccountSection id="tern-passkeys-title" title={title ?? tb('tern.ui.passkeys.title')} description={description ?? tb('tern.ui.passkeys.description')} {level} class={klass}>
	{#if failed}<Alert tone="danger" live>{tb('tern.ui.passkeys.failed')}</Alert>{/if}
	{#if passkeys.length}
		<ul>
			{#each passkeys as p (p.id)}
				<li>
					<div class="tern-account-text">
						<span class="tern-account-name">{p.name}</span>
						<span class="tern-account-meta">{tb('tern.ui.passkeys.added', { when: when(p.createdAt) })}{' · '}{p.lastUsed ? tb('tern.ui.passkeys.lastUsed', { when: when(p.lastUsed) }) : tb('tern.ui.passkeys.never')}</span>
					</div>
					{#if removeAction}
						<Form action={removeAction} reset={false}>
							<input type="hidden" name="id" value={p.id} />
							<Button type="submit" variant="ghost" size="sm">{tb('tern.ui.passkeys.remove')}</Button>
						</Form>
					{/if}
				</li>
			{/each}
		</ul>
	{:else}
		<p class="tern-account-empty">{tb('tern.ui.passkeys.empty')}</p>
	{/if}
	{#if supported}
		<div><Button type="button" variant="secondary" size="sm" loading={adding} onclick={add}>{tb('tern.ui.passkeys.add')}</Button></div>
	{:else}
		<p class="tern-account-empty">{tb('tern.ui.passkeys.unsupported')}</p>
	{/if}
</AccountSection>

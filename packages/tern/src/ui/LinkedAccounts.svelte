<script lang="ts">
	import { tb } from '../runtime/i18n.svelte.js';
	import AccountSection from './AccountSection.svelte';
	import Button from './Button.svelte';
	import Form from './Form.svelte';
	import { when } from './accountDates.js';

	type Linked = { provider: string; email: string; createdAt: string };
	type Props = {
		title?: string;
		description?: string;
		level?: 2 | 3 | 4;
		/** From acc.Identities. */
		identities: Linked[];
		/** plugins/oauth's providers, such as plugin.Providers(): each one not linked yet gets a Connect button. */
		providers: { name: string; label: string }[];
		/** Where plugins/oauth lives. */
		authPath?: string;
		/** Where to come back to after connecting: this page. */
		next?: string;
		/** The action that calls acc.Unlink with the posted provider. */
		unlinkAction?: string;
		class?: string;
	};
	let { title, description, level = 2, identities, providers, authPath = '/auth', next, unlinkAction, class: klass = '' }: Props = $props();
	const label = (name: string) => providers.find((p) => p.name === name)?.label ?? name;
	const linked = $derived(new Set(identities.map((i) => i.provider)));
	const connect = (name: string) => `${authPath}/${encodeURIComponent(name)}/login${next ? `?next=${encodeURIComponent(next)}` : ''}`;
</script>

<AccountSection id="tern-linked-title" title={title ?? tb('tern.ui.linked.title')} description={description ?? tb('tern.ui.linked.description')} {level} class={klass}>
	<ul>
		{#each identities as i (i.provider)}
			<li>
				<div class="tern-account-text">
					<span class="tern-account-name">{label(i.provider)}</span>
					<span class="tern-account-meta">{i.email}{i.email ? ' · ' : ''}{tb('tern.ui.linked.since', { when: when(i.createdAt) })}</span>
				</div>
				{#if unlinkAction}
					<Form action={unlinkAction} reset={false}>
						<input type="hidden" name="provider" value={i.provider} />
						<Button type="submit" variant="ghost" size="sm">{tb('tern.ui.linked.disconnect')}</Button>
					</Form>
				{/if}
			</li>
		{/each}
		{#each providers.filter((p) => !linked.has(p.name)) as p (p.name)}
			<li>
				<div class="tern-account-text"><span class="tern-account-name">{p.label}</span></div>
				<Button variant="secondary" size="sm" href={connect(p.name)}>{tb('tern.ui.linked.connect')}</Button>
			</li>
		{/each}
	</ul>
</AccountSection>

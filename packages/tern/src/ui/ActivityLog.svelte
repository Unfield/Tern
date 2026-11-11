<script lang="ts">
	import { tb } from '../runtime/i18n.svelte.js';
	import AccountSection from './AccountSection.svelte';
	import Button from './Button.svelte';
	import { when } from './accountDates.js';

	type Entry = { id: number; at: string; actor: string; action: string; target: string; ip: string; device: string; data?: Record<string, string> | null };
	type Props = {
		title?: string;
		description?: string;
		level?: 2 | 3 | 4;
		/** From acc.Activity or acc.TeamActivity: the newest first. */
		entries: Entry[];
		/** Shows who did each thing: for a team's log. */
		showActor?: boolean;
		/** A link to the next page, such as `?before=${entries.at(-1).id}`; shown when there may be more. */
		moreHref?: string;
		/** Words for actions of the app's own, by action: { "project.deleted": "Deleted the project {target}" }. */
		labels?: Record<string, string>;
		class?: string;
	};
	let { title, description, level = 2, entries, showActor = false, moreHref, labels = {}, class: klass = '' }: Props = $props();

	// The built-in actions are worded in tern.ui.activity.<action>; others
	// come from labels, or show as they are.
	function describe(e: Entry): string {
		const params = { target: e.target, ...e.data };
		const own = labels[e.action];
		if (own) return own.replace(/\{(\w+)\}/g, (m, k: string) => (k in params ? String((params as Record<string, string>)[k]) : m));
		// The user's own log words some actions as theirs: "Joined Acme".
		if (!showActor) {
			const mine = `tern.ui.activity.mine.${e.action}`;
			const text = tb(mine, params);
			if (text !== mine) return text;
		}
		const key = `tern.ui.activity.${e.action}`;
		const text = tb(key, params);
		return text === key ? e.action : text;
	}
	const failed = (e: Entry) => e.action === 'user.sign_in_failed';
</script>

<AccountSection id="tern-activity-title" title={title ?? tb('tern.ui.activity.title')} description={description ?? tb('tern.ui.activity.description')} {level} class={klass}>
	{#if entries.length}
		<ul>
			{#each entries as e (e.id)}
				<li class:tern-activity-failed={failed(e)}>
					<div class="tern-account-text">
						<span class="tern-account-name">{describe(e)}</span>
						<span class="tern-account-meta">
							{#if showActor && e.actor}{e.actor}{' · '}{/if}<time datetime={e.at}>{when(e.at)}</time>{e.device ? ` · ${e.device}` : ''}{e.ip ? ` · ${e.ip}` : ''}
						</span>
					</div>
				</li>
			{/each}
		</ul>
		{#if moreHref}<div><Button href={moreHref} variant="secondary" size="sm">{tb('tern.ui.activity.more')}</Button></div>{/if}
	{:else}
		<p class="tern-account-empty">{tb('tern.ui.activity.empty')}</p>
	{/if}
</AccountSection>

<style>
	.tern-activity-failed .tern-account-name {
		color: var(--tern-danger);
	}
</style>

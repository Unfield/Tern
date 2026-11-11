<script lang="ts">
	import { tb } from '../runtime/i18n.svelte.js';
	import Avatar from './Avatar.svelte';
	import Button from './Button.svelte';

	type Props = {
		/** From plugins/oidc: provider.ConsentRequest(ctx), returned by the page's load. */
		consent: { client: string; scopes: string[]; action: string; params: Record<string, string> };
		/** Who is signed in, so they can tell they're approving as the right account. */
		user?: { name: string; email?: string };
		/** Where to switch accounts. */
		switchHref?: string;
		level?: 1 | 2 | 3;
		class?: string;
	};
	let { consent, user, switchHref, level = 1, class: klass = '' }: Props = $props();
</script>

<!-- Asks the signed-in user to let another app use their account. The form posts back to plugins/oidc, which checks the request again. -->
<section class="tern-consent {klass}" aria-labelledby="tern-consent-title">
	<svelte:element this={`h${level}`} id="tern-consent-title">{tb('tern.ui.consent.title', { client: consent.client })}</svelte:element>
	{#if user}
		<div class="user">
			<Avatar name={user.name} size="2rem" />
			<div>
				<div class="name">{user.name}</div>
				{#if user.email}<div class="email">{user.email}</div>{/if}
			</div>
			{#if switchHref}<a href={switchHref}>{tb('tern.ui.consent.notYou')}</a>{/if}
		</div>
	{/if}
	<div>
		<p id="tern-consent-scopes">{tb('tern.ui.consent.scopes')}</p>
		<ul aria-labelledby="tern-consent-scopes">
			{#each consent.scopes as scope (scope)}
				<li>
					<svg aria-hidden="true" viewBox="0 0 16 16" width="16" height="16"><path d="M3.5 8.5l3 3 6-7" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" /></svg>
					{scope}
				</li>
			{/each}
		</ul>
	</div>
	<p class="note">{tb('tern.ui.consent.note')}</p>
	<form method="POST" action={consent.action}>
		{#each Object.entries(consent.params) as [k, v] (k)}<input type="hidden" name={k} value={v} />{/each}
		<Button type="submit" name="decision" value="deny" variant="secondary">{tb('tern.ui.consent.deny')}</Button>
		<Button type="submit" name="decision" value="allow">{tb('tern.ui.consent.allow')}</Button>
	</form>
</section>

<style>
	.tern-consent {
		display: grid;
		gap: 1.25rem;
		width: 100%;
		max-width: 26rem;
		box-sizing: border-box;
		padding: 2rem;
		color: var(--tern-fg);
		background: var(--tern-surface);
		border: 1px solid var(--tern-border);
		border-radius: calc(var(--tern-radius) + 4px);
		box-shadow: var(--tern-shadow);
	}
	:is(h1, h2, h3) {
		margin: 0;
		font-size: 1.25rem;
		font-weight: 650;
		letter-spacing: -0.01em;
	}
	.user {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		padding: 0.75rem;
		background: var(--tern-surface-2);
		border-radius: var(--tern-radius);
	}
	.user > div {
		flex: 1;
		min-width: 0;
	}
	.name {
		font-weight: 600;
		font-size: 0.875rem;
	}
	.email {
		color: var(--tern-muted);
		font-size: 0.8125rem;
	}
	p {
		margin: 0;
		font-size: 0.875rem;
	}
	ul {
		display: grid;
		gap: 0.5rem;
		margin: 0.6rem 0 0;
		padding: 0;
		list-style: none;
	}
	li {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		font-size: 0.9375rem;
	}
	li svg {
		color: var(--tern-success);
		flex: none;
	}
	.note {
		color: var(--tern-muted);
		font-size: 0.8125rem;
	}
	form {
		display: flex;
		justify-content: flex-end;
		gap: 0.5rem;
	}
	a {
		color: var(--tern-accent);
		font-size: 0.8125rem;
		font-weight: 550;
	}
</style>

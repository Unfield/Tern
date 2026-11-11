<script lang="ts">
	import { tb } from '../runtime/i18n.svelte.js';
	import AccountSection from './AccountSection.svelte';
	import Alert from './Alert.svelte';
	import Badge from './Badge.svelte';
	import Button from './Button.svelte';
	import Form from './Form.svelte';
	import Progress from './Progress.svelte';
	import { day, money, per, type BillingPlan, type BillingSubscription } from './billing.js';

	type Usage = { limit: string; label: string; used: number };
	type Props = {
		title?: string;
		description?: string;
		level?: 2 | 3 | 4;
		/** The plan that applies, from b.Current. */
		plan: BillingPlan;
		/** The subscription, from b.Current; null for owners who never subscribed. */
		subscription: BillingSubscription | null;
		/** What's used of the plan's limits: [{ limit: "projects", label: "Projects", used: 3 }]. */
		usage?: Usage[];
		/** The action that calls b.Portal and redirects to it. */
		portalAction?: string;
		/** The pricing page, to change plans. */
		pricingHref?: string;
		/** The page's form prop, for the portal action's errors. */
		form?: unknown;
		class?: string;
	};
	let { title, description, level = 2, plan, subscription: s, usage = [], portalAction, pricingHref, form, class: klass = '' }: Props = $props();

	const active = $derived(!!s && ['active', 'trialing', 'past_due'].includes(s.status));
	const amount = $derived(active && s ? plan.amounts?.[s.interval] : undefined);
	const message = $derived((form as { message?: string } | null)?.message);
	// What happens next, in a sentence.
	const next = $derived.by(() => {
		if (!s || !active) return '';
		if (s.status === 'trialing' && s.trialEnd) return tb('tern.ui.billing.trialEnds', { date: day(s.trialEnd) });
		if (s.status === 'past_due') return tb('tern.ui.billing.pastDue');
		if (s.cancelAtPeriodEnd) return tb('tern.ui.billing.cancels', { date: day(s.currentPeriodEnd) });
		return tb('tern.ui.billing.renews', { date: day(s.currentPeriodEnd) });
	});
</script>

<AccountSection id="tern-billing-title" title={title ?? tb('tern.ui.billing.title')} {description} {level} class={klass}>
	<div class="tern-billing-plan">
		<div class="tern-account-text">
			<span class="tern-account-name">
				{plan.name}
				{#if s?.status === 'trialing'}<Badge tone="accent">{tb('tern.ui.billing.status.trialing')}</Badge>
				{:else if s?.status === 'past_due'}<Badge tone="danger">{tb('tern.ui.billing.status.pastDue')}</Badge>
				{:else if active && s?.cancelAtPeriodEnd}<Badge tone="warning">{tb('tern.ui.billing.status.canceling')}</Badge>{/if}
			</span>
			<span class="tern-account-meta">
				{#if amount !== undefined && s}{money(amount * (plan.perSeat ? s.seats : 1), plan.currency)} {per(s.interval)}{plan.perSeat ? ` · ${tb('tern.ui.billing.seats', { n: s.seats })}` : ''}{:else}{tb('tern.ui.billing.noCard')}{/if}
			</span>
		</div>
		<div class="tern-billing-actions">
			{#if pricingHref}<Button href={pricingHref} variant={active ? 'ghost' : 'primary'} size="sm">{active ? tb('tern.ui.billing.changePlan') : tb('tern.ui.billing.upgrade')}</Button>{/if}
			{#if portalAction && s}
				<Form action={portalAction} reset={false}>
					<Button type="submit" variant="secondary" size="sm">{tb('tern.ui.billing.manage')}</Button>
				</Form>
			{/if}
		</div>
	</div>
	{#if next}<Alert tone={s?.status === 'past_due' ? 'danger' : 'info'}>{next}</Alert>{/if}
	{#if message}<Alert tone="danger" live>{message}</Alert>{/if}
	{#if usage.length}
		<div class="tern-billing-usage">
			{#each usage as u (u.limit)}
				{@const max = plan.limits?.[u.limit]}
				<div class="tern-billing-meter">
					<div class="tern-billing-meter-head">
						<span>{u.label}</span>
						<span class="tern-account-meta">{max === undefined ? tb('tern.ui.billing.unlimitedUse', { used: u.used }) : tb('tern.ui.billing.used', { used: u.used, max })}</span>
					</div>
					{#if max !== undefined}<Progress value={Math.min(u.used, max)} max={Math.max(max, 1)} label={u.label} />{/if}
				</div>
			{/each}
		</div>
	{/if}
</AccountSection>

<style>
	.tern-billing-plan {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 1rem;
		flex-wrap: wrap;
		padding: 1rem;
		border: 1px solid var(--tern-border);
		border-radius: var(--tern-radius);
		background: var(--tern-surface);
	}
	.tern-billing-plan .tern-account-name {
		font-size: 1.0625rem;
	}
	.tern-billing-actions {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}
	.tern-billing-usage {
		display: grid;
		gap: 1rem;
	}
	.tern-billing-meter {
		display: grid;
		gap: 0.4rem;
	}
	.tern-billing-meter-head {
		display: flex;
		justify-content: space-between;
		gap: 1rem;
		font-size: 0.875rem;
		font-weight: 550;
	}
</style>

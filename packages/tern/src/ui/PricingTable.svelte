<script lang="ts">
	import { untrack } from 'svelte';
	import { tb } from '../runtime/i18n.svelte.js';
	import Alert from './Alert.svelte';
	import Badge from './Badge.svelte';
	import Button from './Button.svelte';
	import Form from './Form.svelte';
	import ToggleGroup from './ToggleGroup.svelte';
	import { money, per, type BillingPlan, type Interval } from './billing.js';

	type Props = {
		/** From b.Plans(). */
		plans: BillingPlan[];
		/** The plan the owner is on, to mark it and not offer it again. */
		current?: string;
		/** The action that calls b.Checkout with the posted plan and interval. Without it, the buttons link to href. */
		action?: string;
		/** For visitors who aren't signed in: where the buttons go, such as "/signup". */
		href?: string;
		/** Whether plans' trials apply: Checkout gives them to owners who never subscribed. */
		trial?: boolean;
		/** The interval shown first. */
		interval?: Interval;
		/** Words for the plans' limits, by name: { projects: "{n} projects" }. Without one, a limit isn't listed. */
		limitLabels?: Record<string, string>;
		/** The page's form prop, for the action's errors. */
		form?: unknown;
		class?: string;
	};
	let { plans, current, action, href, trial = true, interval = $bindable('month'), limitLabels = {}, form, class: klass = '' }: Props = $props();

	// The toggle's value: clicking the chosen interval again would clear
	// it, so a cleared one goes back to the interval shown.
	let picked = $state<string | null>(untrack(() => interval));
	$effect.pre(() => {
		if (picked === null) picked = interval;
		else if (picked !== interval) interval = picked as Interval;
	});
	const hasYearly = $derived(plans.some((p) => p.intervals.includes('year')));
	const hasMonthly = $derived(plans.some((p) => p.intervals.includes('month')));
	const message = $derived((form as { message?: string } | null)?.message);

	// The interval a plan shows: the chosen one if it has it, else its own.
	const shown = (p: BillingPlan): string | null => (p.intervals.includes(interval) ? interval : (p.intervals[0] ?? null));
	// How much a year's price saves over twelve months.
	function saving(p: BillingPlan): number {
		const m = p.amounts?.month, y = p.amounts?.year;
		return m && y && y < m * 12 ? Math.round((1 - y / (m * 12)) * 100) : 0;
	}
	const most = $derived(Math.max(0, ...plans.map(saving)));
	function limits(p: BillingPlan): string[] {
		return Object.entries(p.limits ?? {}).flatMap(([k, n]) => {
			const label = limitLabels[k];
			return label ? [label.replace('{n}', String(n))] : [];
		});
	}
</script>

<div class="tern-pricing {klass}">
	{#if hasYearly && hasMonthly}
		<div class="tern-pricing-toggle">
			<ToggleGroup
				bind:value={picked}
				label={tb('tern.ui.billing.interval')}
				options={[
					{ value: 'month', label: tb('tern.ui.billing.monthly') },
					{ value: 'year', label: most ? tb('tern.ui.billing.yearlySave', { percent: most }) : tb('tern.ui.billing.yearly') }
				]}
			/>
		</div>
	{/if}
	{#if message}<Alert tone="danger" live>{message}</Alert>{/if}
	<ul class="tern-pricing-plans">
		{#each plans as p (p.id)}
			{@const iv = shown(p)}
			{@const amount = iv ? p.amounts?.[iv] : 0}
			<li class="tern-pricing-plan" class:highlight={p.highlight}>
				<header>
					<h3>{p.name}{#if p.highlight}<Badge tone="accent">{tb('tern.ui.billing.popular')}</Badge>{/if}</h3>
					{#if p.description}<p>{p.description}</p>{/if}
				</header>
				<p class="tern-pricing-price">
					{#if !p.paid}
						<span class="amount">{money(0, p.currency)}</span>
					{:else if amount !== undefined && iv}
						<span class="amount">{money(amount, p.currency)}</span>
						<span class="per">{per(iv)}{p.perSeat ? ` · ${tb('tern.ui.billing.perSeat')}` : ''}</span>
					{/if}
				</p>
				<!-- A note on every plan keeps their buttons in line. -->
				<p class="tern-pricing-note">{p.trialDays && trial ? tb('tern.ui.billing.trial', { days: p.trialDays }) : !p.paid ? tb('tern.ui.billing.noCard') : tb('tern.ui.billing.cancelAnytime')}</p>
				<div class="tern-pricing-cta">
					{#if current === p.id}
						<Button variant="secondary" disabled>{tb('tern.ui.billing.current')}</Button>
					{:else if !p.paid}
						{#if href}<Button href={href} variant="secondary">{tb('tern.ui.billing.startFree')}</Button>{/if}
					{:else if action && iv}
						<Form {action} reset={false}>
							<input type="hidden" name="plan" value={p.id} />
							<input type="hidden" name="interval" value={iv} />
							<Button type="submit" variant={p.highlight ? 'primary' : 'secondary'}>{p.trialDays && trial ? tb('tern.ui.billing.startTrial') : tb('tern.ui.billing.choose', { plan: p.name })}</Button>
						</Form>
					{:else if href}
						<Button href={href} variant={p.highlight ? 'primary' : 'secondary'}>{p.trialDays ? tb('tern.ui.billing.startTrial') : tb('tern.ui.billing.choose', { plan: p.name })}</Button>
					{/if}
				</div>
				{#if limits(p).length || p.features?.length}
					<ul class="tern-pricing-features">
						{#each [...limits(p), ...(p.features ?? [])] as f (f)}
							<li>
								<svg aria-hidden="true" viewBox="0 0 16 16" width="16" height="16"><path d="M3.5 8.5l3 3 6-7" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" /></svg>
								{f}
							</li>
						{/each}
					</ul>
				{/if}
			</li>
		{/each}
	</ul>
</div>

<style>
	.tern-pricing {
		display: grid;
		gap: 1.5rem;
		color: var(--tern-fg);
	}
	.tern-pricing-toggle {
		display: flex;
		justify-content: center;
	}
	.tern-pricing-plans {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(min(100%, 15rem), 1fr));
		gap: 1rem;
		margin: 0;
		padding: 0;
		list-style: none;
	}
	.tern-pricing-plan {
		display: flex;
		flex-direction: column;
		gap: 1rem;
		padding: 1.5rem;
		background: var(--tern-surface);
		border: 1px solid var(--tern-border);
		border-radius: calc(var(--tern-radius) + 4px);
	}
	.tern-pricing-plan.highlight {
		border-color: var(--tern-accent);
		box-shadow: 0 0 0 1px var(--tern-accent), var(--tern-shadow);
	}
	header {
		display: grid;
		gap: 0.35rem;
	}
	h3 {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		/* As tall with a badge as without. */
		min-height: 1.75rem;
		margin: 0;
		font-size: 1.125rem;
		font-weight: 650;
	}
	header p,
	.tern-pricing-note {
		margin: 0;
		color: var(--tern-muted);
		font-size: 0.875rem;
	}
	.tern-pricing-price {
		display: flex;
		align-items: baseline;
		gap: 0.35rem;
		flex-wrap: wrap;
		margin: 0;
		min-height: 2.75rem;
	}
	.amount {
		font-size: 2.25rem;
		font-weight: 700;
		letter-spacing: -0.03em;
		line-height: 1.2;
		font-variant-numeric: tabular-nums;
	}
	.per {
		color: var(--tern-muted);
		font-size: 0.875rem;
	}
	.tern-pricing-cta {
		display: grid;
	}
	.tern-pricing-cta :global(.tern-form) {
		display: grid;
	}
	.tern-pricing-features {
		display: grid;
		gap: 0.5rem;
		margin: 0;
		padding: 1rem 0 0;
		list-style: none;
		border-top: 1px solid var(--tern-border);
		font-size: 0.875rem;
	}
	.tern-pricing-features li {
		display: flex;
		align-items: flex-start;
		gap: 0.5rem;
	}
	.tern-pricing-features svg {
		flex-shrink: 0;
		margin-top: 0.125rem;
		color: var(--tern-accent);
	}
</style>

# Billing

The `billing` package charges for an app with Stripe: plans defined in Go, Stripe Checkout to subscribe, Stripe's customer portal to change plans, update the card or cancel, trials, seats, and limits per plan checked in loads and actions. It speaks Stripe's API directly; there's no SDK to add. The full app `tern new` makes (the saas template) is set up with it.

## Setup

```go
b, err := billing.New(db, billing.Options{
	SecretKey:     os.Getenv("STRIPE_SECRET_KEY"),
	WebhookSecret: os.Getenv("STRIPE_WEBHOOK_SECRET"),
	Jobs:          queue, // processes webhooks in the background
	Plans: []billing.Plan{
		{ID: "free", Name: "Free", Limits: map[string]int{"projects": 3, "members": 2}},
		{ID: "pro", Name: "Pro",
			Prices:  map[billing.Interval]string{billing.Monthly: "price_…", billing.Yearly: "price_…"},
			Amounts: map[billing.Interval]int64{billing.Monthly: 1200, billing.Yearly: 12000}, Currency: "usd",
			PerSeat: true, TrialDays: 14, Highlight: true,
			Features: []string{"Unlimited projects"}},
	},
})
err = b.Migrate(ctx)
app, err := ternApp(kit.WithPlugins(b, queue))
```

`Migrate` creates `tern_billing_customers`, `tern_billing_subscriptions` and `tern_billing_events`. The plugin serves Stripe's webhook at `/_tern/billing/webhook` (`WebhookPath`).

| Option | |
|---|---|
| `SecretKey` | Stripe's secret key. Without it, `Checkout` and `Portal` return `ErrNotSet` and everyone is on the free plan |
| `WebhookSecret` | the webhook endpoint's signing secret, `whsec_…` |
| `Plans` | the plans, in the order the pricing page shows them |
| `FreePlan` | the plan of owners without an active subscription; the default is the first plan without prices |
| `Jobs` | a `jobs` queue to process webhooks in, with retries. Without one, they're processed in the request |
| `WebhookPath` | where Stripe posts events, `/_tern/billing/webhook` by default |
| `OnChange` | called after a subscription changes, to send an email or write to the audit log |

## Plans

A plan has prices in Stripe, by interval (`billing.Monthly`, `billing.Yearly`); a plan without prices is free. Make the products and prices in Stripe's dashboard and put their IDs in the plan: Stripe charges what its prices say. `Amounts` are only what the pricing page shows, so keep them the same.

- **`Limits`** cap what a plan allows: `{"projects": 3}`. A limit a plan doesn't list is unlimited.
- **`Features`** are what it includes, for the pricing page and `b.Has`.
- **`TrialDays`** start a subscription with a free trial, for owners who never subscribed before.
- **`PerSeat`** charges per member: the subscription's quantity is the seats.

## Who pays

Billing is per owner, a string the app picks: `billing.Team(id)` for a team, `billing.User(id)` for a user. Each owner is one Stripe customer and has at most one subscription.

## Subscribing

```go
"checkout": kit.FormAction(func(ctx *kit.Ctx, in Choice) (struct{}, error) {
	m, err := acc.RequireTeam(ctx)
	...
	url, err := b.Checkout(ctx, billing.CheckoutOptions{
		Owner: billing.Team(m.Team.ID), Plan: in.Plan, Interval: billing.Interval(in.Interval),
		Email: accounts.Current(ctx).Email, Seats: len(members),
		SuccessURL: "/settings/billing?subscribed=1", CancelURL: "/pricing",
	})
	if err != nil {
		return struct{}{}, err
	}
	return struct{}{}, kit.Redirect(http.StatusSeeOther, url)
}),
```

`Checkout` makes the owner's Stripe customer the first time and returns the Checkout page to redirect to. `b.Portal(ctx, owner, returnURL)` does the same for the customer portal, where customers change their plan or card, see invoices, and cancel. Turn the portal on in Stripe's billing settings, with the plans customers may switch between.

## Webhooks

Stripe tells the app about every change through webhooks: subscribing, renewing, a failed payment, a plan change in the portal, canceling. The endpoint:

1. checks the `Stripe-Signature` header against `WebhookSecret`, and refuses events signed more than five minutes ago;
2. records the event's ID, so an event Stripe sends twice is processed once;
3. queues a job, in the same transaction, that fetches the subscription from Stripe and stores the app's copy.

Fetching the subscription, instead of trusting what the event says, makes events that arrive late or out of order harmless: each job stores what Stripe has now. A job that fails is retried with backoff.

Add an endpoint in Stripe's dashboard at `<your address>/_tern/billing/webhook` for the `customer.subscription.created`, `.updated` and `.deleted`, `checkout.session.completed`, `invoice.paid` and `invoice.payment_failed` events. In development, the Stripe CLI forwards them and prints the secret:

```sh
stripe listen --forward-to localhost:5173/_tern/billing/webhook
```

`b.SyncSubscription(ctx, id)` does what a webhook's job does, for a subscription the app missed.

## The plan that applies

```go
plan, sub, err := b.Current(ctx, billing.Team(teamID))
```

`Current` is the subscription's plan while it's active (paid, in its trial, or past due while Stripe retries the payment), and the free plan otherwise. `sub` is nil for owners who never subscribed; `sub.Status`, `sub.CurrentPeriodEnd`, `sub.CancelAtPeriodEnd` and `sub.TrialEnd` say where it stands.

## Limits

Check a limit before creating, with how many there are now:

```go
n, err := projects.Count(ctx, teamID)
if err := b.Check(ctx, billing.Team(teamID), "projects", n); err != nil {
	return Done{}, billing.FormError(c, err) // "The Free plan allows 3 projects. Upgrade to add more."
}
```

`Check` returns a `*billing.LimitError` (`errors.Is(err, billing.ErrLimit)`) when one more would go past the plan's limit. `billing.FormError` turns it into a form error for an action, and `billing.HTTPError` into a 402 for a load or an endpoint. `b.Has(ctx, owner, "export")` checks a feature, and `b.Limit` reads a limit.

## Seats

For a per-seat plan, `Checkout` takes the number of seats, and `b.SetSeats(ctx, owner, n)` changes it as members join and leave. Stripe prorates the change. It does nothing for owners without an active per-seat subscription, so calling it after every change is fine.

## The screens

Tern UI has two blocks ([docs/ui.md](ui.md)):

```svelte
<PricingTable plans={data.plans} current={data.current} action="?/checkout" trial={data.trial} limitLabels={{ projects: '{n} projects' }} {form} />
<BillingSettings plan={data.plan} subscription={data.subscription} usage={data.usage} portalAction="?/portal" pricingHref="/pricing" />
```

`PricingTable` shows each plan's price by month or year, its trial, limits and features, and a button that posts the plan and interval to the checkout action. Signed out, `href` links the buttons to sign up instead. `BillingSettings` shows the plan, when it renews, ends or its trial does, a failed payment, how much of each limit is used, and buttons for the portal and the pricing page.

## Testing

`Options.APIBase` points the package at another server: a fake Stripe in tests, as the package's own tests do. Sign the test webhooks with `WebhookSecret` the way `billing.VerifySignature` checks them.

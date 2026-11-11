# 0020. Billing with Stripe

- Status: Accepted
- Date: 2026-11-12

## Context

A SaaS app charges for itself, and every one rebuilds the same parts: plans, Checkout, the customer portal, webhooks, and checks against what the plan allows. The webhooks are where it goes wrong. Stripe sends events at least once, in no guaranteed order, and expects an answer within seconds. An app that applies each event as it comes can process a payment twice, or downgrade a customer because an old `updated` event arrived after a new one.

Stripe's Go SDK is large, tied to an API version per release, and most of it is for calls an app never makes.

## Decision

- **A `billing` package, for Stripe, speaking its REST API directly** with a pinned API version: the handful of calls it makes (customers, Checkout and portal sessions, subscriptions) and nothing else. Stripe is the first provider; the plans, owners, limits and screens don't depend on it.
- **Plans live in Go,** with Stripe's price IDs and the amounts the pricing page shows. Limits and features are the app's, checked by the app with `b.Check` and `b.Has`. Stripe stays the authority on what is charged.
- **An owner is a string the app chooses,** `billing.Team(id)` or `billing.User(id)`, so billing works with teams from `accounts` or without them.
- **Webhooks are verified, recorded once and processed as jobs that fetch the subscription from Stripe,** instead of applying the event's contents. The event's record and the job are written in one transaction. Fetching makes duplicate, late and out-of-order events harmless, and a job retries when Stripe or the database is down.
- **One subscription per owner.** A canceled subscription doesn't replace an active one, as when a customer resubscribes before the old one ends.
- **Past due still counts as subscribed** while Stripe retries the payment, so a failed card doesn't lock a team out mid-month. The screens show it.

## Consequences

- An app gets plans, Checkout, the portal, trials, seats and limits with a few calls and two Tern UI blocks; the saas template has all of it.
- Each webhook costs one API call to Stripe. That's well within Stripe's limits for any app's rate of subscription changes.
- One subscription per owner rules out add-ons sold as separate subscriptions. Apps that need them can still call Stripe themselves.
- Usage-based billing (metered prices) isn't covered yet.

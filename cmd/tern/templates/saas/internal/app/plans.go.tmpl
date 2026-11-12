package app

import (
	"context"

	"github.com/Unfield/Tern/billing"
)

// Plans are what teams can subscribe to. The prices are Stripe's: make
// them in Stripe's dashboard, then set their IDs in the environment.
// Amounts are what the pricing page shows; keep them the same as Stripe's.
func Plans() []billing.Plan {
	return []billing.Plan{
		{
			ID: "free", Name: "Free", Description: "For trying it out, alone or with a friend.",
			Limits:   map[string]int{"projects": 3, "members": 2},
			Features: []string{"Passkeys and two-step sign-in"},
		},
		{
			ID: "pro", Name: "Pro", Description: "For teams that work in it every day.",
			Prices: map[billing.Interval]string{
				billing.Monthly: env("STRIPE_PRICE_PRO_MONTHLY", "price_pro_monthly"),
				billing.Yearly:  env("STRIPE_PRICE_PRO_YEARLY", "price_pro_yearly"),
			},
			Amounts:   map[billing.Interval]int64{billing.Monthly: 1200, billing.Yearly: 12000},
			Currency:  "usd",
			PerSeat:   true,
			TrialDays: 14,
			Highlight: true,
			Features:  []string{"Unlimited projects", "Unlimited members", "The team’s activity log", "Priority support"},
		},
	}
}

// Owner is a team's billing owner.
func Owner(teamID int64) string { return billing.Team(teamID) }

// SyncSeats sets a team's per-member subscription to its number of
// members, after someone joins or leaves.
func (a *App) SyncSeats(ctx context.Context, teamID int64) error {
	var n int
	if err := a.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM tern_members WHERE team_id = ?`, teamID).Scan(&n); err != nil {
		return err
	}
	return a.Billing.SetSeats(ctx, Owner(teamID), n)
}

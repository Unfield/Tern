package billing

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"

	"github.com/Unfield/Tern/kit"
)

// ErrLimit is what Check returns past a plan's limit; errors.Is matches
// every *LimitError.
var ErrLimit = errors.New("billing: the plan's limit is reached")

// LimitError says which limit was reached, on which plan.
type LimitError struct {
	Limit string // "projects"
	Max   int
	Plan  Plan
}

func (e *LimitError) Error() string {
	return fmt.Sprintf("billing: the %s plan allows %d %s", e.Plan.Name, e.Max, e.Limit)
}

// Is makes errors.Is(err, ErrLimit) true.
func (e *LimitError) Is(target error) bool { return target == ErrLimit }

// Limit is the owner's plan's limit, and whether it has one.
func (b *Billing) Limit(ctx context.Context, owner, limit string) (int, bool, error) {
	p, _, err := b.Current(ctx, owner)
	if err != nil {
		return 0, false, err
	}
	n, ok := p.Limits[limit]
	return n, ok, nil
}

// Check returns a *LimitError if adding one more would go past the
// owner's plan's limit, given how many they have now; nil if the plan
// allows it or has no such limit. In an action, before creating:
//
//	n, err := projects.Count(ctx, teamID)
//	if err := b.Check(ctx, billing.Team(teamID), "projects", n); err != nil {
//		return Done{}, billing.FormError(c, err)
//	}
func (b *Billing) Check(ctx context.Context, owner, limit string, used int) error {
	p, _, err := b.Current(ctx, owner)
	if err != nil {
		return err
	}
	if n, ok := p.Limits[limit]; ok && used >= n {
		return &LimitError{Limit: limit, Max: n, Plan: p}
	}
	return nil
}

// Has reports whether the owner's plan includes a feature.
func (b *Billing) Has(ctx context.Context, owner, feature string) (bool, error) {
	p, _, err := b.Current(ctx, owner)
	if err != nil {
		return false, err
	}
	return slices.Contains(p.Features, feature), nil
}

// FormError turns a *LimitError into a form error, in the user's
// language, for an action to return. Other errors stay as they are.
func FormError(c *kit.Ctx, err error) error {
	var le *LimitError
	if errors.As(err, &le) {
		return kit.FormError(c.TB("tern.billing.limit", map[string]any{"plan": le.Plan.Name, "max": le.Max, "limit": le.Limit}))
	}
	return err
}

// HTTPError turns a *LimitError into a 402 Payment Required, for a load
// or an endpoint. Other errors stay as they are.
func HTTPError(c *kit.Ctx, err error) error {
	var le *LimitError
	if errors.As(err, &le) {
		return kit.Error(http.StatusPaymentRequired, c.TB("tern.billing.limit", map[string]any{"plan": le.Plan.Name, "max": le.Max, "limit": le.Limit}))
	}
	return err
}

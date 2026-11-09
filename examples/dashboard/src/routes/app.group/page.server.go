package app

import (
	"time"

	"github.com/Unfield/Tern/kit"

	"dashboard/internal/app"
	"dashboard/internal/db"
)

type Overview struct {
	Stats db.Stats `json:"stats"`
	// Activity is slower to compute, so it streams in after the page.
	Activity kit.Deferred[[]db.Activity] `json:"activity"`
}

func Load(ctx *kit.Ctx) (Overview, error) {
	user, store, err := app.User(ctx)
	if err != nil {
		return Overview{}, err
	}
	ctx.Depends("app:stats") // the Refresh button reruns this load with invalidate("app:stats")
	stats, err := store.Stats(ctx, user)
	if err != nil {
		return Overview{}, err
	}
	return Overview{
		Stats: stats,
		Activity: kit.Defer(ctx, func() ([]db.Activity, error) {
			select {
			case <-time.After(400 * time.Millisecond): // simulate a slow query
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			return store.RecentActivity(ctx, user, 6)
		}),
	}, nil
}

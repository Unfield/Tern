// Package stats is the stats API: what the dashboard shows, as JSON, for
// your own scripts. It's described in /api/openapi.json.
package stats

import (
	"errors"
	"net/http"
	"time"

	"github.com/Unfield/Tern/kit"

	"analytics/internal/app"
	"analytics/internal/store"
)

type Query struct {
	Domain string `path:"domain" json:"domain"`
	// Days is how many days back, including today.
	Days int `json:"days,omitempty" validate:"min=1,max=365"`
}

type Stats struct {
	Domain  string          `json:"domain"`
	Summary store.Summary   `json:"summary"`
	Days    []store.Day     `json:"days"`
	Pages   []store.Count   `json:"pages"`
	Sources []store.Count   `json:"sources"`
}

func GET(ctx *kit.Ctx, in Query) (Stats, error) {
	if _, ok := app.CurrentUser(ctx); !ok {
		return Stats{}, kit.Error(http.StatusUnauthorized, "Sign in first.")
	}
	site, err := app.Store.SiteByDomain(ctx, in.Domain)
	if errors.Is(err, store.ErrNotFound) {
		return Stats{}, kit.NotFound()
	}
	if err != nil {
		return Stats{}, err
	}
	if in.Days == 0 {
		in.Days = 30
	}
	p := store.LastDays(in.Days, time.Now())
	out := Stats{Domain: site.Domain}
	if out.Summary, err = app.Store.Summary(ctx, site.ID, p); err != nil {
		return out, err
	}
	if out.Days, err = app.Store.Days(ctx, site.ID, p); err != nil {
		return out, err
	}
	if out.Pages, err = app.Store.Top(ctx, site.ID, p, "path", 20); err != nil {
		return out, err
	}
	out.Sources, err = app.Store.Top(ctx, site.ID, p, "referrer", 20)
	return out, err
}

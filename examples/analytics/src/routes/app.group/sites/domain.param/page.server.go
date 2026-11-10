// Package site is one site's dashboard.
package site

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/Unfield/Tern/kit"

	"analytics/internal/app"
	"analytics/internal/store"
)

type Dashboard struct {
	Site     store.Site    `json:"site"`
	Days     int           `json:"days"`
	Summary  store.Summary `json:"summary"`
	Previous store.Summary `json:"previous"`
	Series   []store.Day   `json:"series"`
	Pages    []store.Count `json:"pages"`
	Sources  []store.Count `json:"sources"`
	Browsers []store.Count `json:"browsers"`
	Devices  []store.Count `json:"devices"`
	Systems  []store.Count `json:"systems"`
}

func site(ctx *kit.Ctx) (store.Site, error) {
	s, err := app.Store.SiteByDomain(ctx, ctx.Param("domain"))
	if errors.Is(err, store.ErrNotFound) {
		return s, kit.NotFound()
	}
	return s, err
}

func Load(ctx *kit.Ctx) (Dashboard, error) {
	s, err := site(ctx)
	if err != nil {
		return Dashboard{}, err
	}
	days, _ := strconv.Atoi(ctx.URL().Query().Get("days"))
	if days != 7 && days != 30 && days != 90 {
		days = 30
	}
	p := store.LastDays(days, time.Now())
	d := Dashboard{Site: s, Days: days}
	st := app.Store
	if d.Summary, err = st.Summary(ctx, s.ID, p); err != nil {
		return d, err
	}
	if d.Previous, err = st.Summary(ctx, s.ID, p.Previous()); err != nil {
		return d, err
	}
	if d.Series, err = st.Days(ctx, s.ID, p); err != nil {
		return d, err
	}
	for _, t := range []struct {
		col string
		dst *[]store.Count
		n   int
	}{{"path", &d.Pages, 100}, {"referrer", &d.Sources, 8}, {"browser", &d.Browsers, 8}, {"device", &d.Devices, 4}, {"os", &d.Systems, 8}} {
		if *t.dst, err = st.Top(ctx, s.ID, p, t.col, t.n); err != nil {
			return d, err
		}
	}
	return d, nil
}

var Actions = kit.Actions{
	"delete": func(ctx *kit.Ctx) (any, error) {
		s, err := site(ctx)
		if err != nil {
			return nil, err
		}
		if err := app.Store.DeleteSite(ctx, s.ID); err != nil {
			return nil, err
		}
		return nil, kit.Redirect(http.StatusSeeOther, "/sites")
	},
}

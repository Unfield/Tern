// Package sites lists the sites and adds new ones.
package sites

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Unfield/Tern/kit"

	"analytics/internal/app"
	"analytics/internal/store"
)

type Card struct {
	store.Site
	Visitors int   `json:"visitors"`
	Trend    []int `json:"trend"`
}

type Page struct {
	Cards []Card `json:"cards"`
}

func Load(ctx *kit.Ctx) (Page, error) {
	sites, err := app.Store.Sites(ctx)
	if err != nil {
		return Page{}, err
	}
	week := store.LastDays(7, time.Now())
	out := Page{Cards: []Card{}}
	for _, s := range sites {
		sum, err := app.Store.Summary(ctx, s.ID, week)
		if err != nil {
			return Page{}, err
		}
		days, err := app.Store.Days(ctx, s.ID, week)
		if err != nil {
			return Page{}, err
		}
		c := Card{Site: s, Visitors: sum.Visitors}
		for _, d := range days {
			c.Trend = append(c.Trend, d.Visitors)
		}
		out.Cards = append(out.Cards, c)
	}
	return out, nil
}

type NewSite struct {
	Domain string `form:"domain" validate:"required,max=253,pattern=([a-z0-9]([a-z0-9-]*[a-z0-9])?\\.)+[a-z]{2,}" message:"Enter a domain such as example.com, without https://."`
	Name   string `form:"name" validate:"required,max=80"`
}

var Actions = kit.Actions{
	"add": kit.FormAction(func(ctx *kit.Ctx, in NewSite) (store.Site, error) {
		s, err := app.Store.CreateSite(ctx, strings.TrimPrefix(in.Domain, "www."), in.Name)
		if errors.Is(err, store.ErrExists) {
			return s, kit.FieldError("domain", "This site is already here.")
		}
		if err != nil {
			return s, err
		}
		return s, kit.Redirect(http.StatusSeeOther, "/sites/"+s.Domain)
	}),
}

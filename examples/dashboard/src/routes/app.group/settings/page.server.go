package settings

import (
	"net/http"
	"net/mail"
	"strings"
	"unicode/utf8"

	"github.com/Unfield/Tern/kit"

	"dashboard/internal/app"
	"dashboard/internal/db"
)

type Settings struct {
	Profile db.Profile `json:"profile"`
}

func Load(ctx *kit.Ctx) (Settings, error) {
	user, store, err := app.User(ctx)
	if err != nil {
		return Settings{}, err
	}
	p, err := store.Profile(ctx, user)
	return Settings{Profile: p}, err
}

type Saved struct {
	Saved bool `json:"saved"`
}

type Invalid struct {
	Values db.Profile        `json:"values"`
	Errors map[string]string `json:"errors"`
}

var Actions = kit.Actions{
	"default": kit.Action(func(ctx *kit.Ctx) (Saved, error) {
		user, store, err := app.User(ctx)
		if err != nil {
			return Saved{}, err
		}
		form, err := ctx.Form()
		if err != nil {
			return Saved{}, err
		}
		p := db.Profile{
			DisplayName:  strings.TrimSpace(form.Get("displayName")),
			Email:        strings.TrimSpace(form.Get("email")),
			WeeklyDigest: form.Get("weeklyDigest") == "on",
		}
		errs := map[string]string{}
		if p.DisplayName == "" || utf8.RuneCountInString(p.DisplayName) > 40 {
			errs["displayName"] = "Use 1 to 40 characters."
		}
		if p.Email != "" {
			if _, err := mail.ParseAddress(p.Email); err != nil {
				errs["email"] = "That doesn't look like an email address."
			}
		}
		if len(errs) > 0 {
			return Saved{}, kit.Fail(http.StatusBadRequest, Invalid{Values: p, Errors: errs})
		}
		return Saved{Saved: true}, store.SaveProfile(ctx, user, p)
	}),
}

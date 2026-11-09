package projects

import (
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/Unfield/Tern/kit"

	"dashboard/internal/app"
	"dashboard/internal/db"
)

type List struct {
	Projects []db.Project `json:"projects"`
	Query    string       `json:"query"`
	Status   string       `json:"status"`
}

func Load(ctx *kit.Ctx) (List, error) {
	user, store, err := app.User(ctx)
	if err != nil {
		return List{}, err
	}
	q := strings.TrimSpace(ctx.URL().Query().Get("q"))
	status := ctx.URL().Query().Get("status")
	if status != "active" && status != "paused" && status != "done" {
		status = ""
	}
	projects, err := store.Projects(ctx, user, q, status)
	return List{Projects: projects, Query: q, Status: status}, err
}

type Created struct {
	Created db.Project `json:"created"`
}

// Invalid reports field errors so the dialog can show them next to inputs.
type Invalid struct {
	Values map[string]string `json:"values"`
	Errors map[string]string `json:"errors"`
}

var Actions = kit.Actions{
	"create": kit.Action(func(ctx *kit.Ctx) (Created, error) {
		user, store, err := app.User(ctx)
		if err != nil {
			return Created{}, err
		}
		form, err := ctx.Form()
		if err != nil {
			return Created{}, err
		}
		name := strings.TrimSpace(form.Get("name"))
		desc := strings.TrimSpace(form.Get("description"))
		errs := map[string]string{}
		if name == "" {
			errs["name"] = "Give the project a name."
		} else if utf8.RuneCountInString(name) > 60 {
			errs["name"] = "Keep the name under 60 characters."
		}
		if utf8.RuneCountInString(desc) > 280 {
			errs["description"] = "Keep the description under 280 characters."
		}
		if len(errs) > 0 {
			return Created{}, kit.Fail(http.StatusBadRequest, Invalid{
				Values: map[string]string{"name": name, "description": desc},
				Errors: errs,
			})
		}
		p, err := store.CreateProject(ctx, user, name, desc)
		return Created{Created: p}, err
	}),
}

// Package incident posts updates to an incident and resolves it.
package incident

import (
	"net/http"
	"strings"

	"github.com/Unfield/Tern/kit"

	"status/internal/app"
	"status/internal/store"
)

type Page struct {
	Incident store.Incident `json:"incident"`
}

func Load(ctx *kit.Ctx) (Page, error) {
	in, err := app.Store.Incident(ctx, app.ID(ctx))
	if err != nil {
		return Page{}, app.NotFound(err)
	}
	return Page{Incident: app.RenderIncidents([]store.Incident{in})[0]}, nil
}

type Invalid struct {
	Error string `json:"error"`
}

var statuses = map[string]bool{store.Investigating: true, store.Identified: true, store.Monitoring: true, store.Resolved: true}

var Actions = kit.Actions{
	"update": kit.Action(func(ctx *kit.Ctx) (struct{}, error) {
		form, err := ctx.Form()
		if err != nil {
			return struct{}{}, err
		}
		status, body := form.Get("status"), strings.TrimSpace(form.Get("body"))
		if !statuses[status] || body == "" {
			return struct{}{}, kit.Fail(http.StatusBadRequest, Invalid{Error: "Choose a status and write an update."})
		}
		if err := app.Store.UpdateIncident(ctx, app.ID(ctx), status, body); err != nil {
			return struct{}{}, app.NotFound(err)
		}
		return struct{}{}, nil
	}),
	"delete": kit.Action(func(ctx *kit.Ctx) (struct{}, error) {
		if err := app.Store.DeleteIncident(ctx, app.ID(ctx)); err != nil {
			return struct{}{}, err
		}
		app.Flash(ctx, "Incident deleted.")
		return struct{}{}, kit.Redirect(http.StatusSeeOther, "/admin/incidents")
	}),
}

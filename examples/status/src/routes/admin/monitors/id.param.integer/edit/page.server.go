// Package edit changes a monitor's settings.
package edit

import (
	"fmt"
	"net/http"

	"github.com/Unfield/Tern/kit"

	"status/internal/app"
	"status/internal/store"
)

type Form struct {
	Values store.Monitor `json:"values"`
	Groups []string      `json:"groups"`
}

func Load(ctx *kit.Ctx) (Form, error) {
	m, err := app.Store.Monitor(ctx, app.ID(ctx))
	if err != nil {
		return Form{}, app.NotFound(err)
	}
	return Form{Values: m, Groups: app.Groups(ctx)}, nil
}

var Actions = kit.Actions{
	"default": kit.Action(func(ctx *kit.Ctx) (struct{}, error) {
		id := app.ID(ctx)
		m, errs, err := app.ReadMonitor(ctx, id)
		if err != nil {
			return struct{}{}, err
		}
		if len(errs) > 0 {
			return struct{}{}, kit.Fail(http.StatusBadRequest, app.MonitorInvalid{Values: m, Errors: errs})
		}
		if _, err := app.Store.SaveMonitor(ctx, m); err != nil {
			return struct{}{}, app.NotFound(err)
		}
		app.Engine.Reload(ctx, id)
		app.Flash(ctx, "Saved "+m.Name+".")
		return struct{}{}, kit.Redirect(http.StatusSeeOther, fmt.Sprintf("/admin/monitors/%d", id))
	}),
}

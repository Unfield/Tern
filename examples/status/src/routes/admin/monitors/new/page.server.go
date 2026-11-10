// Package newmonitor adds a monitor.
package newmonitor

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
	return Form{
		Values: store.Monitor{Type: "http", Method: "GET", ExpectedStatus: "200-399", IntervalSeconds: 60, TimeoutSeconds: 10, Retries: 1, CertExpiryDays: 14, Group: "Services", Public: true},
		Groups: app.Groups(ctx),
	}, nil
}

var Actions = kit.Actions{
	"default": kit.Action(func(ctx *kit.Ctx) (struct{}, error) {
		m, errs, err := app.ReadMonitor(ctx, 0)
		if err != nil {
			return struct{}{}, err
		}
		if len(errs) > 0 {
			return struct{}{}, kit.Fail(http.StatusBadRequest, app.MonitorInvalid{Values: m, Errors: errs})
		}
		id, err := app.Store.SaveMonitor(ctx, m)
		if err != nil {
			return struct{}{}, err
		}
		app.Engine.Reload(ctx, id)
		app.Flash(ctx, fmt.Sprintf("%s is being checked every %d seconds.", m.Name, m.IntervalSeconds))
		return struct{}{}, kit.Redirect(http.StatusSeeOther, fmt.Sprintf("/admin/monitors/%d", id))
	}),
}

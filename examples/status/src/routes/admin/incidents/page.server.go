// Package incidents lists incidents and opens new ones.
package incidents

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Unfield/Tern/kit"

	"status/internal/app"
	"status/internal/store"
)

type MonitorOption struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type List struct {
	Incidents []store.Incident `json:"incidents"`
	Monitors  []MonitorOption  `json:"monitors"`
}

func Load(ctx *kit.Ctx) (List, error) {
	all, err := app.Store.Incidents(ctx, time.Now().AddDate(0, 0, -90))
	if err != nil {
		return List{}, err
	}
	monitors, err := app.Store.Monitors(ctx)
	if err != nil {
		return List{}, err
	}
	out := List{Incidents: app.RenderIncidents(all), Monitors: []MonitorOption{}}
	if out.Incidents == nil {
		out.Incidents = []store.Incident{}
	}
	for _, m := range monitors {
		out.Monitors = append(out.Monitors, MonitorOption{ID: m.ID, Name: m.Name})
	}
	return out, nil
}

type Invalid struct {
	Title  string            `json:"title"`
	Body   string            `json:"body"`
	Errors map[string]string `json:"errors"`
}

var Actions = kit.Actions{
	"default": kit.Action(func(ctx *kit.Ctx) (struct{}, error) {
		form, err := ctx.Form()
		if err != nil {
			return struct{}{}, err
		}
		title, body := strings.TrimSpace(form.Get("title")), strings.TrimSpace(form.Get("body"))
		impact := form.Get("impact")
		errs := map[string]string{}
		if title == "" {
			errs["title"] = "Enter a title."
		}
		if body == "" {
			errs["body"] = "Tell visitors what is happening."
		}
		if impact != "minor" && impact != "major" && impact != "maintenance" {
			impact = "minor"
		}
		if len(errs) > 0 {
			return struct{}{}, kit.Fail(http.StatusBadRequest, Invalid{Title: title, Body: body, Errors: errs})
		}
		var monitorID *int64
		if id, err := strconv.ParseInt(form.Get("monitor"), 10, 64); err == nil && id > 0 {
			monitorID = &id
		}
		id, err := app.Store.OpenIncident(ctx, title, impact, body, monitorID, false)
		if err != nil {
			return struct{}{}, err
		}
		app.Flash(ctx, "The incident is on the status page.")
		return struct{}{}, kit.Redirect(http.StatusSeeOther, fmt.Sprintf("/admin/incidents/%d", id))
	}),
}

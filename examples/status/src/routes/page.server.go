// Package status is the public status page.
package status

import (
	"time"

	"github.com/Unfield/Tern/kit"

	"status/internal/app"
	"status/internal/monitor"
	"status/internal/store"
)

type Monitor struct {
	monitor.PublicMonitor
	Days   []store.Day `json:"days"`
	Uptime float64     `json:"uptime"` // over 90 days, -1 without data
}

type Group struct {
	Name     string    `json:"name"`
	Monitors []Monitor `json:"monitors"`
}

type Page struct {
	Title       string           `json:"title"`
	Description string           `json:"description"`
	Groups      []Group          `json:"groups"`
	Incidents   []store.Incident `json:"incidents"`
	Now         int64            `json:"now"`
	Admin       bool             `json:"admin"`
}

func Load(ctx *kit.Ctx) (Page, error) {
	monitors, err := app.Store.Monitors(ctx)
	if err != nil {
		return Page{}, err
	}
	daily, err := app.Store.Daily(ctx, 90)
	if err != nil {
		return Page{}, err
	}
	p := Page{
		Title:       app.Store.Setting(ctx, "title", "Status"),
		Description: app.Store.Setting(ctx, "description", ""),
		Now:         time.Now().UnixMilli(),
		Groups:      []Group{},
	}
	_, p.Admin = app.CurrentAdmin(ctx)
	index := map[string]int{}
	for _, m := range monitors {
		if !m.Public {
			continue
		}
		days := daily[m.ID]
		if days == nil {
			days = emptyDays(90)
		}
		up, total := 0.0, 0
		for _, d := range days {
			if d.Ratio >= 0 {
				up += d.Ratio
				total++
			}
		}
		uptime := -1.0
		if total > 0 {
			uptime = up / float64(total)
		}
		i, ok := index[m.Group]
		if !ok {
			i = len(p.Groups)
			index[m.Group] = i
			p.Groups = append(p.Groups, Group{Name: m.Group})
		}
		p.Groups[i].Monitors = append(p.Groups[i].Monitors, Monitor{PublicMonitor: monitor.Public(m), Days: days, Uptime: uptime})
	}
	incidents, err := app.Store.Incidents(ctx, time.Now().AddDate(0, 0, -14))
	if err != nil {
		return Page{}, err
	}
	p.Incidents = app.RenderIncidents(incidents)
	if p.Incidents == nil {
		p.Incidents = []store.Incident{}
	}
	return p, nil
}

func emptyDays(n int) []store.Day {
	start := time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, -(n - 1))
	out := make([]store.Day, n)
	for i := range out {
		out[i] = store.Day{Date: start.AddDate(0, 0, i).Format("2006-01-02"), Ratio: -1}
	}
	return out
}

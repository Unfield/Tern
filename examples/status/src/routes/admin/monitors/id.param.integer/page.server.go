// Package detail shows one monitor: its uptime, response times and events.
package detail

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/Unfield/Tern/kit"

	"status/internal/app"
	"status/internal/store"
)

type Point struct {
	At      int64  `json:"at"`
	Latency int    `json:"latency"`
	Status  string `json:"status"`
}

type Change struct {
	At      int64  `json:"at"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

type Detail struct {
	Monitor  store.Monitor           `json:"monitor"`
	Uptime   map[string]store.Uptime `json:"uptime"`
	Period   string                  `json:"period"`
	Points   []Point                 `json:"points"`
	Changes  []Change                `json:"changes"`
	Beats    []store.Heartbeat       `json:"beats"`
	Now      int64                   `json:"now"`
	BadgeURL string                  `json:"badgeUrl"`
}

func Load(ctx *kit.Ctx) (Detail, error) {
	id := app.ID(ctx)
	ctx.Depends(fmt.Sprintf("app:monitor:%d", id))
	m, err := app.Store.Monitor(ctx, id)
	if err != nil {
		return Detail{}, app.NotFound(err)
	}
	d := Detail{Monitor: m, Uptime: map[string]store.Uptime{}, Now: time.Now().UnixMilli(), Period: ctx.Request.URL.Query().Get("period")}
	if d.Period != "7d" && d.Period != "30d" {
		d.Period = "24h"
	}
	for _, p := range []string{"24h", "7d", "30d"} {
		if d.Uptime[p], err = app.Store.Uptime(ctx, id, app.Since(p)); err != nil {
			return Detail{}, err
		}
	}
	beats, err := app.Store.Heartbeats(ctx, id, app.Since(d.Period), 50000)
	if err != nil {
		return Detail{}, err
	}
	d.Points = downsample(beats, 240)
	d.Changes = changes(beats)
	d.Beats = beats[max(0, len(beats)-40):]
	d.BadgeURL = "/api/badge/" + fmt.Sprint(id)
	return d, nil
}

// downsample averages checks into at most n points, keeping the worst
// status of each bucket so outages stay visible.
func downsample(beats []store.Heartbeat, n int) []Point {
	out := []Point{}
	if len(beats) == 0 {
		return out
	}
	size := (len(beats) + n - 1) / n
	rank := map[store.Status]int{store.Up: 0, store.Pending: 1, store.Down: 2}
	for i := 0; i < len(beats); i += size {
		chunk := beats[i:min(i+size, len(beats))]
		sum, ups := 0, 0
		worst := store.Up
		for _, b := range chunk {
			if b.Status == store.Up {
				sum += b.LatencyMS
				ups++
			}
			if rank[b.Status] > rank[worst] {
				worst = b.Status
			}
		}
		p := Point{At: chunk[len(chunk)-1].At, Status: string(worst)}
		if ups > 0 {
			p.Latency = sum / ups
		}
		out = append(out, p)
	}
	return out
}

// changes lists when the status changed, newest first.
func changes(beats []store.Heartbeat) []Change {
	out := []Change{}
	var last store.Status
	for _, b := range beats {
		if b.Status == store.Pending || b.Status == last {
			continue
		}
		last = b.Status
		out = append(out, Change{At: b.At, Status: string(b.Status), Message: b.Message})
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out[:min(len(out), 50)]
}

var Actions = kit.Actions{
	"pause":  kit.Action(func(ctx *kit.Ctx) (struct{}, error) { return struct{}{}, setPaused(ctx, true) }),
	"resume": kit.Action(func(ctx *kit.Ctx) (struct{}, error) { return struct{}{}, setPaused(ctx, false) }),
	"check": kit.Action(func(ctx *kit.Ctx) (struct{}, error) {
		check, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
		defer cancel()
		app.Engine.CheckNow(check, app.ID(ctx))
		return struct{}{}, nil
	}),
	"delete": kit.Action(func(ctx *kit.Ctx) (struct{}, error) {
		id := app.ID(ctx)
		m, err := app.Store.Monitor(ctx, id)
		if err != nil {
			return struct{}{}, app.NotFound(err)
		}
		if err := app.Store.DeleteMonitor(ctx, id); err != nil {
			return struct{}{}, err
		}
		app.Engine.Reload(ctx, id)
		app.Flash(ctx, "Deleted "+m.Name+".")
		return struct{}{}, kit.Redirect(http.StatusSeeOther, "/admin")
	}),
}

func setPaused(ctx *kit.Ctx, paused bool) error {
	id := app.ID(ctx)
	if err := app.Store.SetPaused(ctx, id, paused); err != nil {
		return err
	}
	app.Engine.Reload(ctx, id)
	return nil
}

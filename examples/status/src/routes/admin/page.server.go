package admin

import (
	"time"

	"github.com/Unfield/Tern/kit"

	"status/internal/app"
	"status/internal/store"
)

type Row struct {
	store.Monitor
	Beats  []store.Heartbeat `json:"beats"`
	Uptime float64           `json:"uptime"` // 24 hours
}

type Overview struct {
	Monitors []Row `json:"monitors"`
	Now      int64 `json:"now"`
}

func Load(ctx *kit.Ctx) (Overview, error) {
	ctx.Depends("app:monitors")
	monitors, err := app.Store.Monitors(ctx)
	if err != nil {
		return Overview{}, err
	}
	beats, err := app.Store.RecentBeats(ctx, 40)
	if err != nil {
		return Overview{}, err
	}
	out := Overview{Monitors: []Row{}, Now: time.Now().UnixMilli()}
	for _, m := range monitors {
		u, err := app.Store.Uptime(ctx, m.ID, time.Now().Add(-24*time.Hour))
		if err != nil {
			return Overview{}, err
		}
		b := beats[m.ID]
		if b == nil {
			b = []store.Heartbeat{}
		}
		out.Monitors = append(out.Monitors, Row{Monitor: m, Beats: b, Uptime: u.Ratio})
	}
	return out, nil
}

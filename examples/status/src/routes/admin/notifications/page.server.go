// Package notifications manages where alerts go.
package notifications

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Unfield/Tern/kit"

	"status/internal/app"
	"status/internal/monitor"
	"status/internal/notify"
	"status/internal/store"
)

type Delivery struct {
	ID        int64     `json:"id"`
	Channel   string    `json:"channel"`
	Monitor   string    `json:"monitor"`
	Status    string    `json:"status"`
	State     string    `json:"state"`
	Attempts  int       `json:"attempts"`
	LastError string    `json:"lastError"`
	At        time.Time `json:"at"`
}

type Page struct {
	Channels   []store.Channel     `json:"channels"`
	Types      map[string][]string `json:"types"`
	Deliveries []Delivery          `json:"deliveries"`
}

func Load(ctx *kit.Ctx) (Page, error) {
	channels, err := app.Store.Channels(ctx, false)
	if err != nil {
		return Page{}, err
	}
	// Hide secrets such as SMTP passwords.
	for i := range channels {
		if _, ok := channels[i].Config["password"]; ok {
			channels[i].Config["password"] = "••••••"
		}
	}
	names := map[int64]string{}
	for _, c := range channels {
		names[c.ID] = c.Name
	}
	recent, err := app.Queue.List(ctx, "", 50)
	if err != nil {
		return Page{}, err
	}
	deliveries := []Delivery{}
	for _, j := range recent {
		if j.Kind != "notify" {
			continue
		}
		var p app.NotifyJob
		_ = json.Unmarshal(j.Payload, &p)
		deliveries = append(deliveries, Delivery{ID: j.ID, Channel: names[p.ChannelID], Monitor: p.Change.Name, Status: string(p.Change.Status),
			State: string(j.State), Attempts: j.Attempts, LastError: j.LastError, At: j.CreatedAt})
	}
	return Page{Channels: channels, Types: notify.Types, Deliveries: deliveries}, nil
}

type Invalid struct {
	Values store.Channel      `json:"values"`
	Errors map[string]string `json:"errors"`
}

type Result struct {
	Tested string `json:"tested"`
	Error  string `json:"error"`
}

func formID(ctx *kit.Ctx) int64 {
	form, _ := ctx.Form()
	id, _ := strconv.ParseInt(form.Get("id"), 10, 64)
	return id
}

var Actions = kit.Actions{
	"create": kit.Action(func(ctx *kit.Ctx) (struct{}, error) {
		form, err := ctx.Form()
		if err != nil {
			return struct{}{}, err
		}
		c := store.Channel{Name: strings.TrimSpace(form.Get("name")), Type: form.Get("type"), Enabled: true, Config: map[string]any{}}
		for _, f := range notify.Types[c.Type] {
			c.Config[f] = strings.TrimSpace(form.Get(f))
		}
		if errs := notify.Validate(c); len(errs) > 0 {
			delete(c.Config, "password")
			return struct{}{}, kit.Fail(http.StatusBadRequest, Invalid{Values: c, Errors: errs})
		}
		if _, err := app.Store.SaveChannel(ctx, c); err != nil {
			return struct{}{}, err
		}
		return struct{}{}, nil
	}),
	"toggle": kit.Action(func(ctx *kit.Ctx) (struct{}, error) {
		c, err := app.Store.Channel(ctx, formID(ctx))
		if err != nil {
			return struct{}{}, app.NotFound(err)
		}
		c.Enabled = !c.Enabled
		_, err = app.Store.SaveChannel(ctx, c)
		return struct{}{}, err
	}),
	"delete": kit.Action(func(ctx *kit.Ctx) (struct{}, error) {
		return struct{}{}, app.Store.DeleteChannel(ctx, formID(ctx))
	}),
	// test sends a message at once, so the result shows on the page.
	"test": kit.Action(func(ctx *kit.Ctx) (Result, error) {
		c, err := app.Store.Channel(ctx, formID(ctx))
		if err != nil {
			return Result{}, app.NotFound(err)
		}
		send, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		err = notify.Send(send, c, monitor.Change{Name: "Test monitor", Target: "https://example.com", Status: store.Down, Message: "this is a test notification", At: time.Now()})
		if err != nil {
			return Result{Tested: c.Name, Error: err.Error()}, nil
		}
		return Result{Tested: c.Name}, nil
	}),
	"retry": kit.Action(func(ctx *kit.Ctx) (struct{}, error) {
		return struct{}{}, app.Queue.Retry(ctx, formID(ctx))
	}),
}


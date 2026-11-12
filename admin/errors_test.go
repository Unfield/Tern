package admin_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Unfield/Tern/admin"
	"github.com/Unfield/Tern/internal/admintest"
	"github.com/Unfield/Tern/kit"
	"github.com/Unfield/Tern/report"
)

func TestErrorsScreen(t *testing.T) {
	r := report.New(report.Options{})
	r.Report(context.Background(), errors.New("database is down"), func(rep *report.Report) { rep.Route = "/items" })
	r.Report(context.Background(), errors.New("database is down"), func(rep *report.Report) { rep.Route = "/items" })
	r.Send(report.Report{Source: "browser", Type: "TypeError", Message: "x is undefined", Stack: []report.Frame{{Function: "explode", File: "src/routes/+page.svelte", Line: 14, InApp: true}}})
	app := admintest.App(t, admin.New(admin.Options{Authorize: func(*kit.Ctx) error { return nil }}, admin.Errors(r)))
	p := admintest.Get(t, app, "/admin/errors?sort=-count")
	if p.List.Total != 2 || p.List.Rows[0].Values["message"] != "database is down" || p.List.Rows[0].Values["count"] != "2" {
		t.Fatalf("list = %+v", p.List)
	}
	if b := admintest.Get(t, app, "/admin/errors?source=browser"); b.List.Total != 1 {
		t.Errorf("browser only = %+v", b.List)
	}
	one := admintest.Get(t, app, "/admin/errors/"+p.List.Rows[0].Key)
	if one.Record.Values["route"] != "/items" || one.Record.Values["count"] != "2" {
		t.Errorf("record = %+v", one.Record.Values)
	}
	if b := admintest.Get(t, app, "/admin/errors/"+p.List.Rows[1].Key); b.Record.Values["stack"] != "→ explode\n    src/routes/+page.svelte:14\n" {
		t.Errorf("stack = %q", b.Record.Values["stack"])
	}
	if admintest.Get(t, app, "/admin/errors/nope").Status != 404 {
		t.Error("an unknown group is found")
	}
}

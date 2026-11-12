package jobs_test

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/Unfield/Tern/admin"
	"github.com/Unfield/Tern/internal/admintest"
	"github.com/Unfield/Tern/internal/dbtest"
	"github.com/Unfield/Tern/jobs"
	"github.com/Unfield/Tern/kit"
)

func TestJobsAdmin(t *testing.T) {
	dbtest.Each(t, testJobsAdmin)
}

func testJobsAdmin(t *testing.T, db *sql.DB) {
	q, err := jobs.New(db, jobs.Options{})
	if err != nil {
		t.Fatal(err)
	}
	fail := jobs.Handle(q, "email", func(context.Context, string) error { return errors.New("smtp is down") }, jobs.MaxAttempts(1))
	ctx := context.Background()
	if err := q.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	id, err := fail.Enqueue(ctx, "ada@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if err := q.Start(ctx); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if list, _ := q.List(ctx, jobs.Failed, 10); len(list) == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the job never failed")
		}
		time.Sleep(10 * time.Millisecond)
	}
	_ = q.Stop(ctx)

	app := admintest.App(t, admin.New(admin.Options{Authorize: func(*kit.Ctx) error { return nil }}, q.Admin()))
	p := admintest.Get(t, app, "/admin/jobs?state=failed&q=smtp")
	if p.List.Total != 1 || p.List.Rows[0].Values["kind"] != "email" || p.List.Rows[0].Values["state"] != "failed" {
		t.Fatalf("list = %+v", p.List)
	}
	key := p.List.Rows[0].Key
	rec := admintest.Get(t, app, "/admin/jobs/"+key)
	if rec.Record.Values["last_error"] != "smtp is down" || rec.Resource.Edit || len(rec.Resource.Actions) != 1 {
		t.Errorf("record = %+v, resource = %+v", rec.Record.Values, rec.Resource)
	}
	if loc, msg, _ := admintest.Post(t, app, "/admin/jobs/"+key+"?/do", url.Values{"action": {"retry"}}); loc != "/admin/jobs/"+key+"?done=retry" {
		t.Fatalf("retry: %q %q", loc, msg)
	}
	if list, _ := q.List(ctx, jobs.Queued, 10); len(list) != 1 || list[0].ID != id || list[0].Attempts != 0 {
		t.Errorf("after retry: %+v", list)
	}
	// Retrying a job that isn't failed says so on the page.
	if _, msg, status := admintest.Post(t, app, "/admin/jobs/"+key+"?/do", url.Values{"action": {"retry"}}); status != 400 || msg == "" {
		t.Errorf("retry again: %d %q", status, msg)
	}
	if _, _, status := admintest.Post(t, app, "/admin/jobs/"+key+"?/do", url.Values{"action": {"nope"}}); status != 404 {
		t.Errorf("unknown action: %d", status)
	}
}

package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"analytics/migrations"

	"github.com/Unfield/Tern/migrate"
)

func open(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := migrate.Up(context.Background(), s.DB, migrations.FS); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestStats(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	site, err := s.CreateSite(ctx, "Example.com", "Example")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateSite(ctx, "example.com", "Again"); !errors.Is(err, ErrExists) {
		t.Errorf("a second site with the domain: %v", err)
	}
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	add := func(visitor, path string, at time.Time) {
		if err := s.AddEvent(ctx, Event{SiteID: site.ID, At: at, Visitor: visitor, Path: path, Browser: "Chrome", OS: "macOS", Device: "Desktop", Referrer: "google.com"}); err != nil {
			t.Fatal(err)
		}
	}
	add("a", "/", now.Add(-time.Hour))
	add("a", "/pricing", now.Add(-50*time.Minute))
	add("b", "/", now.Add(-2*time.Minute))
	add("c", "/", now.AddDate(0, 0, -3))

	p := LastDays(7, now)
	sum, err := s.Summary(ctx, site.ID, p)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Visitors != 3 || sum.Pageviews != 4 || sum.BounceRate < 66 || sum.BounceRate > 67 {
		t.Errorf("summary = %+v", sum)
	}
	days, err := s.Days(ctx, site.ID, p)
	if err != nil {
		t.Fatal(err)
	}
	if len(days) != 7 || days[6].Day != "2026-11-10" || days[6].Visitors != 2 || days[6].Pageviews != 3 || days[3].Visitors != 1 {
		t.Errorf("days = %+v", days)
	}
	top, _ := s.Top(ctx, site.ID, p, "path", 10)
	if len(top) != 2 || top[0] != (Count{"/", 3, 3}) {
		t.Errorf("top = %+v", top)
	}
	if live, _ := s.Live(ctx, site.ID, now); live != 1 {
		t.Errorf("live = %d", live)
	}

	// Totals stay after the events are pruned.
	if err := s.Rollup(ctx, now.AddDate(0, 0, -7)); err != nil {
		t.Fatal(err)
	}
	if n, err := s.Prune(ctx, now.AddDate(0, 0, -1)); err != nil || n != 1 {
		t.Fatalf("pruned %d: %v", n, err)
	}
	days, _ = s.Days(ctx, site.ID, p)
	if days[3].Visitors != 1 {
		t.Errorf("after pruning, days = %+v", days)
	}
}

func TestVisitor(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	day := time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)
	a, _ := s.Visitor(ctx, day, "example.com", "203.0.113.7", "Firefox")
	b, _ := s.Visitor(ctx, day.Add(5*time.Hour), "example.com", "203.0.113.7", "Firefox")
	c, _ := s.Visitor(ctx, day.AddDate(0, 0, 1), "example.com", "203.0.113.7", "Firefox")
	d, _ := s.Visitor(ctx, day, "other.example", "203.0.113.7", "Firefox")
	if a != b || a == c || a == d || len(a) != 24 {
		t.Errorf("visitor hashes: %s %s %s %s", a, b, c, d)
	}
	if err := s.ForgetSalts(ctx, day.AddDate(0, 0, 1)); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = s.DB.QueryRow(`SELECT COUNT(*) FROM salts`).Scan(&n)
	if n != 1 {
		t.Errorf("salts left: %d", n)
	}
}

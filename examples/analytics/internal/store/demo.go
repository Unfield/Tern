package store

import (
	"context"
	"fmt"
	"math"
	"math/rand/v2"
	"time"
)

// SeedDemo fills a site with 30 days of made-up visits, so a new install
// has something to show. It does nothing if the site has visits.
func (s *Store) SeedDemo(ctx context.Context, site Site, now time.Time) error {
	var n int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE site_id = ?`, site.ID).Scan(&n); err != nil || n > 0 {
		return err
	}
	r := rand.New(rand.NewPCG(1, 2)) //nolint:gosec // demo data
	paths := []string{"/", "/", "/", "/pricing", "/docs", "/docs/getting-started", "/blog", "/blog/hello-world", "/about", "/changelog"}
	refs := []string{"", "", "", "google.com", "google.com", "news.ycombinator.com", "github.com", "duckduckgo.com", "twitter.com", "newsletter.example"}
	browsers := []string{"Chrome", "Chrome", "Chrome", "Safari", "Safari", "Firefox", "Edge", "Samsung Internet"}
	oses := []string{"macOS", "Windows", "Windows", "iOS", "Android", "Linux"}
	devices := []string{"Desktop", "Desktop", "Desktop", "Phone", "Phone", "Tablet"}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO events (site_id, ts, visitor, path, referrer, browser, os, device) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	start := startOfDay(now).AddDate(0, 0, -29)
	for d := range 30 {
		day := start.AddDate(0, 0, d)
		// Growing, with quieter weekends.
		visitors := 60 + d*4 + int(18*math.Sin(float64(d)/2.5))
		if wd := day.Weekday(); wd == time.Saturday || wd == time.Sunday {
			visitors = visitors * 6 / 10
		}
		for v := range visitors {
			id := fmt.Sprintf("demo-%d-%d", d, v)
			b, o, dev, ref := browsers[r.IntN(len(browsers))], oses[r.IntN(len(oses))], devices[r.IntN(len(devices))], refs[r.IntN(len(refs))]
			views := 1 + r.IntN(4)
			at := day.Add(time.Duration(r.IntN(86_400)) * time.Second)
			if at.After(now) {
				continue
			}
			for range views {
				if _, err := stmt.ExecContext(ctx, site.ID, at.UnixMilli(), id, paths[r.IntN(len(paths))], ref, b, o, dev); err != nil {
					return err
				}
				at = at.Add(time.Duration(20+r.IntN(200)) * time.Second)
				ref = ""
			}
		}
	}
	return tx.Commit()
}

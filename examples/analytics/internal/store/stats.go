package store

import (
	"context"
	"time"
)

// Period is a span of days, ending today.
type Period struct {
	From, To time.Time // To is exclusive
}

// LastDays is the period of the last n days, including today.
func LastDays(n int, now time.Time) Period {
	end := startOfDay(now).AddDate(0, 0, 1)
	return Period{From: end.AddDate(0, 0, -n), To: end}
}

// Previous is the period of the same length just before p.
func (p Period) Previous() Period {
	return Period{From: p.From.Add(-p.To.Sub(p.From)), To: p.From}
}

// Summary is a period's headline numbers.
type Summary struct {
	Visitors   int     `json:"visitors"`
	Pageviews  int     `json:"pageviews"`
	BounceRate float64 `json:"bounceRate"` // percent of visitors who saw one page
}

// Summary counts visitors and page views in p.
func (s *Store) Summary(ctx context.Context, siteID int64, p Period) (Summary, error) {
	var out Summary
	var bounced int
	err := s.DB.QueryRowContext(ctx, `
		SELECT COUNT(*), COALESCE(SUM(n), 0), COALESCE(SUM(n = 1), 0) FROM (
			SELECT visitor, COUNT(*) AS n FROM events
			WHERE site_id = ? AND ts >= ? AND ts < ?
			GROUP BY visitor, date(ts / 1000, 'unixepoch')
		)`, siteID, p.From.UnixMilli(), p.To.UnixMilli()).Scan(&out.Visitors, &out.Pageviews, &bounced)
	if err != nil {
		return out, err
	}
	if out.Visitors > 0 {
		out.BounceRate = float64(bounced) * 100 / float64(out.Visitors)
	}
	return out, nil
}

// Day is a day's totals.
type Day struct {
	Day       string `json:"day"`
	Visitors  int    `json:"visitors"`
	Pageviews int    `json:"pageviews"`
}

// Days returns every day of p, with zeros for days without visits. Days
// still in the events are counted from them; older ones come from daily.
func (s *Store) Days(ctx context.Context, siteID int64, p Period) ([]Day, error) {
	counts := map[string]Day{}
	rows, err := s.DB.QueryContext(ctx, `SELECT day, visitors, pageviews FROM daily WHERE site_id = ? AND day >= ? AND day < ?`,
		siteID, p.From.Format(time.DateOnly), p.To.Format(time.DateOnly))
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var d Day
		if err := rows.Scan(&d.Day, &d.Visitors, &d.Pageviews); err != nil {
			rows.Close()
			return nil, err
		}
		counts[d.Day] = d
	}
	rows.Close()
	rows, err = s.DB.QueryContext(ctx, `
		SELECT date(ts / 1000, 'unixepoch') AS day, COUNT(DISTINCT visitor), COUNT(*) FROM events
		WHERE site_id = ? AND ts >= ? AND ts < ? GROUP BY day`, siteID, p.From.UnixMilli(), p.To.UnixMilli())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var d Day
		if err := rows.Scan(&d.Day, &d.Visitors, &d.Pageviews); err != nil {
			return nil, err
		}
		counts[d.Day] = d
	}
	var out []Day
	for t := p.From; t.Before(p.To); t = t.AddDate(0, 0, 1) {
		day := t.Format(time.DateOnly)
		d, ok := counts[day]
		if !ok {
			d = Day{Day: day}
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// Count is a value and how many visitors and views it had.
type Count struct {
	Value     string `json:"value"`
	Visitors  int    `json:"visitors"`
	Pageviews int    `json:"pageviews"`
}

// Top returns the most visited values of a column: "path", "referrer",
// "browser", "os" or "device".
func (s *Store) Top(ctx context.Context, siteID int64, p Period, column string, limit int) ([]Count, error) {
	switch column {
	case "path", "referrer", "browser", "os", "device":
	default:
		panic("store.Top: unknown column " + column)
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT `+column+`, COUNT(DISTINCT visitor), COUNT(*) FROM events
		WHERE site_id = ? AND ts >= ? AND ts < ? AND `+column+` != ''
		GROUP BY `+column+` ORDER BY 2 DESC, 3 DESC, 1 LIMIT ?`, siteID, p.From.UnixMilli(), p.To.UnixMilli(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Count{}
	for rows.Next() {
		var c Count
		if err := rows.Scan(&c.Value, &c.Visitors, &c.Pageviews); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Live counts the visitors of the last five minutes.
func (s *Store) Live(ctx context.Context, siteID int64, now time.Time) (int, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT COUNT(DISTINCT visitor) FROM events WHERE site_id = ? AND ts >= ?`, siteID, now.Add(-5*time.Minute).UnixMilli()).Scan(&n)
	return n, err
}

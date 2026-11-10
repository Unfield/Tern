package jobs

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Schedule computes when a recurring job runs next.
type Schedule interface {
	// Next returns the first run time strictly after t.
	Next(t time.Time) time.Time
}

// ParseSchedule parses a schedule:
//
//   - a cron expression with five fields, "minute hour day-of-month month
//     day-of-week", such as "*/5 * * * *" or "0 9 * * mon-fri". Fields take
//     *, numbers, ranges (1-5), lists (1,15), steps (*/10, 0-30/5) and the
//     names jan-dec and sun-sat. When both day fields are restricted, a day
//     matching either runs, as in standard cron.
//   - @yearly, @monthly, @weekly, @daily (or @midnight) and @hourly
//   - @every followed by a duration, such as "@every 30s"
//
// Cron expressions are evaluated in loc, or UTC if loc is nil.
func ParseSchedule(spec string, loc *time.Location) (Schedule, error) {
	if loc == nil {
		loc = time.UTC
	}
	spec = strings.TrimSpace(spec)
	if rest, ok := strings.CutPrefix(spec, "@every "); ok {
		d, err := time.ParseDuration(strings.TrimSpace(rest))
		if err != nil || d < time.Second {
			return nil, fmt.Errorf("jobs: @every needs a duration of at least 1s, such as 30s or 5m: %q", rest)
		}
		return every(d), nil
	}
	switch spec {
	case "@yearly", "@annually":
		spec = "0 0 1 1 *"
	case "@monthly":
		spec = "0 0 1 * *"
	case "@weekly":
		spec = "0 0 * * 0"
	case "@daily", "@midnight":
		spec = "0 0 * * *"
	case "@hourly":
		spec = "0 * * * *"
	}
	fields := strings.Fields(spec)
	if len(fields) != 5 {
		return nil, fmt.Errorf("jobs: a cron expression has 5 fields (minute hour day month weekday), got %q", spec)
	}
	var c cron
	var err error
	parsers := []struct {
		dst      *uint64
		min, max int
		names    []string
	}{
		{&c.minute, 0, 59, nil},
		{&c.hour, 0, 23, nil},
		{&c.dom, 1, 31, nil},
		{&c.month, 1, 12, months},
		{&c.dow, 0, 7, weekdays},
	}
	for i, p := range parsers {
		if *p.dst, err = parseField(fields[i], p.min, p.max, p.names); err != nil {
			return nil, fmt.Errorf("jobs: cron %q: %w", spec, err)
		}
	}
	if c.dow&(1<<7) != 0 { // 7 is Sunday too
		c.dow |= 1
	}
	c.domStar = fields[2] == "*" || strings.HasPrefix(fields[2], "*/")
	c.dowStar = fields[4] == "*" || strings.HasPrefix(fields[4], "*/")
	c.loc = loc
	return &c, nil
}

var (
	months   = []string{"", "jan", "feb", "mar", "apr", "may", "jun", "jul", "aug", "sep", "oct", "nov", "dec"}
	weekdays = []string{"sun", "mon", "tue", "wed", "thu", "fri", "sat"}
)

func parseField(field string, lo, hi int, names []string) (uint64, error) {
	var set uint64
	for _, part := range strings.Split(field, ",") {
		rng, stepStr, hasStep := strings.Cut(part, "/")
		step := 1
		if hasStep {
			n, err := strconv.Atoi(stepStr)
			if err != nil || n <= 0 {
				return 0, fmt.Errorf("invalid step %q", stepStr)
			}
			step = n
		}
		var from, to int
		switch {
		case rng == "*":
			from, to = lo, hi
		case strings.Contains(rng, "-"):
			a, b, _ := strings.Cut(rng, "-")
			var err error
			if from, err = value(a, lo, hi, names); err != nil {
				return 0, err
			}
			if to, err = value(b, lo, hi, names); err != nil {
				return 0, err
			}
			if from > to {
				return 0, fmt.Errorf("invalid range %q", rng)
			}
		default:
			v, err := value(rng, lo, hi, names)
			if err != nil {
				return 0, err
			}
			from, to = v, v
			if hasStep {
				to = hi
			}
		}
		for v := from; v <= to; v += step {
			set |= 1 << v
		}
	}
	return set, nil
}

func value(s string, lo, hi int, names []string) (int, error) {
	for i, n := range names {
		if n != "" && strings.EqualFold(s, n) {
			return i, nil
		}
	}
	v, err := strconv.Atoi(s)
	if err != nil || v < lo || v > hi {
		return 0, fmt.Errorf("%q is not between %d and %d", s, lo, hi)
	}
	return v, nil
}

type cron struct {
	minute, hour, dom, month, dow uint64
	domStar, dowStar              bool
	loc                           *time.Location
}

func (c *cron) dayMatches(t time.Time) bool {
	dom := c.dom&(1<<t.Day()) != 0
	dow := c.dow&(1<<int(t.Weekday())) != 0
	switch {
	case c.domStar && c.dowStar:
		return true
	case c.domStar:
		return dow
	case c.dowStar:
		return dom
	default:
		return dom || dow
	}
}

// Next walks forward field by field, which finds the next time in at most
// a few hundred steps even for rare schedules such as Feb 29.
func (c *cron) Next(t time.Time) time.Time {
	t = t.In(c.loc).Truncate(time.Minute).Add(time.Minute)
	limit := t.AddDate(5, 0, 0)
	for t.Before(limit) {
		if c.month&(1<<int(t.Month())) == 0 {
			t = time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, c.loc)
			continue
		}
		if !c.dayMatches(t) {
			t = time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, c.loc)
			continue
		}
		if c.hour&(1<<t.Hour()) == 0 {
			t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour()+1, 0, 0, 0, c.loc)
			continue
		}
		if c.minute&(1<<t.Minute()) == 0 {
			t = t.Add(time.Minute)
			continue
		}
		return t
	}
	return time.Time{} // never, such as "0 0 31 2 *"
}

type every time.Duration

func (e every) Next(t time.Time) time.Time {
	d := time.Duration(e)
	return t.Truncate(d).Add(d)
}

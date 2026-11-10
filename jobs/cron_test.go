package jobs

import (
	"testing"
	"time"
)

func TestSchedules(t *testing.T) {
	base := time.Date(2026, 11, 10, 10, 7, 30, 0, time.UTC) // a Tuesday
	berlin, _ := time.LoadLocation("Europe/Berlin")
	for _, tt := range []struct {
		spec string
		loc  *time.Location
		want string
	}{
		{"* * * * *", nil, "2026-11-10T10:08:00Z"},
		{"*/15 * * * *", nil, "2026-11-10T10:15:00Z"},
		{"0 * * * *", nil, "2026-11-10T11:00:00Z"},
		{"30 9 * * *", nil, "2026-11-11T09:30:00Z"},
		{"0 9 * * mon-fri", nil, "2026-11-11T09:00:00Z"},
		{"0 9 * * sat,sun", nil, "2026-11-14T09:00:00Z"},
		{"0 0 * * 7", nil, "2026-11-15T00:00:00Z"},
		{"0 0 1 * *", nil, "2026-12-01T00:00:00Z"},
		{"0 0 1 jan *", nil, "2027-01-01T00:00:00Z"},
		{"0 0 29 2 *", nil, "2028-02-29T00:00:00Z"},
		{"0 0 13 * fri", nil, "2026-11-13T00:00:00Z"}, // day 13 or a Friday
		{"5-10/5 * * * *", nil, "2026-11-10T10:10:00Z"},
		{"@hourly", nil, "2026-11-10T11:00:00Z"},
		{"@daily", nil, "2026-11-11T00:00:00Z"},
		{"@weekly", nil, "2026-11-15T00:00:00Z"},
		{"@every 30s", nil, "2026-11-10T10:08:00Z"},
		{"@every 1h", nil, "2026-11-10T11:00:00Z"},
		{"0 9 * * *", berlin, "2026-11-11T08:00:00Z"}, // 9:00 in Berlin
	} {
		s, err := ParseSchedule(tt.spec, tt.loc)
		if err != nil {
			t.Errorf("%s: %v", tt.spec, err)
			continue
		}
		if got := s.Next(base).UTC().Format(time.RFC3339); got != tt.want {
			t.Errorf("%s: next = %s, want %s", tt.spec, got, tt.want)
		}
	}
	never, _ := ParseSchedule("0 0 31 2 *", nil)
	if !never.Next(base).IsZero() {
		t.Error("Feb 31 should never run")
	}
}

func TestBadSchedules(t *testing.T) {
	for _, spec := range []string{"", "* * * *", "60 * * * *", "* 24 * * *", "5-1 * * * *", "*/0 * * * *", "@every 10ms", "@every soon", "* * * foo *"} {
		if _, err := ParseSchedule(spec, nil); err == nil {
			t.Errorf("%q: want an error", spec)
		}
	}
}

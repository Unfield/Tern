// Package store holds the status page's data in SQLite.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite" // the SQLite driver
)

// ErrNotFound is returned when a row does not exist.
var ErrNotFound = errors.New("not found")

// Store wraps the database.
type Store struct{ DB *sql.DB }

// Open opens (and creates) the SQLite database at path.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)")
	if err != nil {
		return nil, err
	}
	// One writer at a time keeps SQLite from returning "database is locked".
	db.SetMaxOpenConns(8)
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	return &Store{DB: db}, nil
}

// Ping checks the database, for the health check.
func (s *Store) Ping(ctx context.Context) error { return s.DB.PingContext(ctx) }

func now() int64 { return time.Now().UnixMilli() }

// Status is a monitor's state.
type Status string

// The states of a monitor.
const (
	Up      Status = "up"
	Down    Status = "down"
	Pending Status = "pending" // not checked yet, or failing but within its retries
	Paused  Status = "paused"
)

// Monitor is something the status page checks.
type Monitor struct {
	ID              int64     `json:"id"`
	Name            string    `json:"name"`
	Type            string    `json:"type"`
	Target          string    `json:"target"`
	Method          string    `json:"method"`
	ExpectedStatus  string    `json:"expectedStatus"`
	Keyword         string    `json:"keyword"`
	IntervalSeconds int       `json:"intervalSeconds"`
	TimeoutSeconds  int       `json:"timeoutSeconds"`
	Retries         int       `json:"retries"`
	CertExpiryDays  int       `json:"certExpiryDays"`
	Group           string    `json:"group"`
	Public          bool      `json:"public"`
	Paused          bool      `json:"paused"`
	Status          Status    `json:"status"`
	LastCheck       *int64    `json:"lastCheck"`
	LastLatency     *int      `json:"lastLatency"`
	LastMessage     string    `json:"lastMessage"`
	CertExpires     *int64    `json:"certExpires"`
	CreatedAt       time.Time `json:"createdAt"`
}

const monitorColumns = `id, name, type, target, method, expected_status, keyword, interval_seconds, timeout_seconds, retries,
	cert_expiry_days, group_name, public, paused, status, last_check, last_latency, last_message, cert_expires, created_at`

func scanMonitor(row interface{ Scan(...any) error }) (Monitor, error) {
	var m Monitor
	var lastCheck, certExpires sql.NullInt64
	var lastLatency sql.NullInt32
	var created int64
	err := row.Scan(&m.ID, &m.Name, &m.Type, &m.Target, &m.Method, &m.ExpectedStatus, &m.Keyword, &m.IntervalSeconds,
		&m.TimeoutSeconds, &m.Retries, &m.CertExpiryDays, &m.Group, &m.Public, &m.Paused, &m.Status, &lastCheck,
		&lastLatency, &m.LastMessage, &certExpires, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return m, ErrNotFound
	}
	if lastCheck.Valid {
		m.LastCheck = &lastCheck.Int64
	}
	if lastLatency.Valid {
		v := int(lastLatency.Int32)
		m.LastLatency = &v
	}
	if certExpires.Valid {
		m.CertExpires = &certExpires.Int64
	}
	m.CreatedAt = time.UnixMilli(created)
	return m, err
}

// Monitors returns every monitor, by group and name.
func (s *Store) Monitors(ctx context.Context) ([]Monitor, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+monitorColumns+` FROM monitors ORDER BY group_name, name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Monitor
	for rows.Next() {
		m, err := scanMonitor(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// Monitor returns one monitor.
func (s *Store) Monitor(ctx context.Context, id int64) (Monitor, error) {
	return scanMonitor(s.DB.QueryRowContext(ctx, `SELECT `+monitorColumns+` FROM monitors WHERE id = ?`, id))
}

// SaveMonitor inserts m if its ID is 0, or updates it, and returns its ID.
func (s *Store) SaveMonitor(ctx context.Context, m Monitor) (int64, error) {
	if m.ID == 0 {
		res, err := s.DB.ExecContext(ctx, `INSERT INTO monitors (name, type, target, method, expected_status, keyword, interval_seconds,
			timeout_seconds, retries, cert_expiry_days, group_name, public, paused, status, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			m.Name, m.Type, m.Target, m.Method, m.ExpectedStatus, m.Keyword, m.IntervalSeconds, m.TimeoutSeconds,
			m.Retries, m.CertExpiryDays, m.Group, m.Public, m.Paused, statusFor(m.Paused), now())
		if err != nil {
			return 0, err
		}
		return res.LastInsertId()
	}
	res, err := s.DB.ExecContext(ctx, `UPDATE monitors SET name = ?, type = ?, target = ?, method = ?, expected_status = ?, keyword = ?,
		interval_seconds = ?, timeout_seconds = ?, retries = ?, cert_expiry_days = ?, group_name = ?, public = ?
		WHERE id = ?`,
		m.Name, m.Type, m.Target, m.Method, m.ExpectedStatus, m.Keyword, m.IntervalSeconds, m.TimeoutSeconds,
		m.Retries, m.CertExpiryDays, m.Group, m.Public, m.ID)
	if err != nil {
		return 0, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return 0, ErrNotFound
	}
	return m.ID, nil
}

func statusFor(paused bool) Status {
	if paused {
		return Paused
	}
	return Pending
}

// SetPaused pauses or resumes a monitor.
func (s *Store) SetPaused(ctx context.Context, id int64, paused bool) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE monitors SET paused = ?, status = ? WHERE id = ?`, paused, statusFor(paused), id)
	return err
}

// DeleteMonitor deletes a monitor and its heartbeats.
func (s *Store) DeleteMonitor(ctx context.Context, id int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM monitors WHERE id = ?`, id)
	return err
}

// Heartbeat is the result of one check.
type Heartbeat struct {
	MonitorID int64  `json:"monitorId"`
	At        int64  `json:"at"`
	Status    Status `json:"status"`
	LatencyMS int    `json:"latencyMs"`
	Message   string `json:"message"`
}

// Record stores a check's result and the monitor's resulting status.
func (s *Store) Record(ctx context.Context, h Heartbeat, monitorStatus Status, certExpires *int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `INSERT INTO heartbeats (monitor_id, at, status, latency_ms, message) VALUES (?, ?, ?, ?, ?)`,
		h.MonitorID, h.At, h.Status, h.LatencyMS, h.Message); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE monitors SET status = ?, last_check = ?, last_latency = ?, last_message = ?,
		cert_expires = COALESCE(?, cert_expires) WHERE id = ? AND paused = 0`,
		monitorStatus, h.At, h.LatencyMS, h.Message, certExpires, h.MonitorID); err != nil {
		return err
	}
	return tx.Commit()
}

// Heartbeats returns a monitor's checks since a time, oldest first, at most
// limit of the newest.
func (s *Store) Heartbeats(ctx context.Context, id int64, since time.Time, limit int) ([]Heartbeat, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT monitor_id, at, status, latency_ms, message FROM (
		SELECT * FROM heartbeats WHERE monitor_id = ? AND at >= ? ORDER BY at DESC LIMIT ?
	) ORDER BY at`, id, since.UnixMilli(), limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []Heartbeat{}
	for rows.Next() {
		var h Heartbeat
		if err := rows.Scan(&h.MonitorID, &h.At, &h.Status, &h.LatencyMS, &h.Message); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// RecentBeats returns the newest n heartbeats of every monitor, oldest
// first, for the bars on lists.
func (s *Store) RecentBeats(ctx context.Context, n int) (map[int64][]Heartbeat, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT monitor_id, at, status, latency_ms, message FROM (
		SELECT *, ROW_NUMBER() OVER (PARTITION BY monitor_id ORDER BY at DESC) AS rn FROM heartbeats
	) WHERE rn <= ? ORDER BY monitor_id, at`, n)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[int64][]Heartbeat{}
	for rows.Next() {
		var h Heartbeat
		if err := rows.Scan(&h.MonitorID, &h.At, &h.Status, &h.LatencyMS, &h.Message); err != nil {
			return nil, err
		}
		out[h.MonitorID] = append(out[h.MonitorID], h)
	}
	return out, rows.Err()
}

// Uptime is the share of up checks in a period, from 0 to 1, or -1 if there
// were none, and the average latency of up checks.
type Uptime struct {
	Ratio      float64 `json:"ratio"`
	AvgLatency int     `json:"avgLatency"`
	Checks     int     `json:"checks"`
}

// Uptime computes a monitor's uptime since a time. Pending checks (failures
// within the retries) don't count against it.
func (s *Store) Uptime(ctx context.Context, id int64, since time.Time) (Uptime, error) {
	var up, total int
	var avg sql.NullFloat64
	err := s.DB.QueryRowContext(ctx, `SELECT
		COALESCE(SUM(status = 'up'), 0), COALESCE(SUM(status IN ('up', 'down')), 0),
		AVG(CASE WHEN status = 'up' THEN latency_ms END)
		FROM heartbeats WHERE monitor_id = ? AND at >= ?`, id, since.UnixMilli()).Scan(&up, &total, &avg)
	if err != nil {
		return Uptime{}, err
	}
	u := Uptime{Ratio: -1, Checks: total, AvgLatency: int(avg.Float64)}
	if total > 0 {
		u.Ratio = float64(up) / float64(total)
	}
	return u, nil
}

// Day is one day's uptime of a monitor, for the 90-day bars.
type Day struct {
	Date  string  `json:"date"` // YYYY-MM-DD in UTC
	Ratio float64 `json:"ratio"`
	Down  int     `json:"down"` // failed checks
}

// Daily returns each day's uptime of every monitor over the last days days,
// oldest first; days without checks have a ratio of -1.
func (s *Store) Daily(ctx context.Context, days int) (map[int64][]Day, error) {
	start := time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, -(days - 1))
	rows, err := s.DB.QueryContext(ctx, `SELECT monitor_id, at / 86400000 AS day,
		SUM(status = 'up'), SUM(status IN ('up', 'down')), SUM(status = 'down')
		FROM heartbeats WHERE at >= ? GROUP BY monitor_id, day`, start.UnixMilli())
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	type key struct{ monitor, day int64 }
	counts := map[key][3]int{}
	monitors := map[int64]bool{}
	for rows.Next() {
		var k key
		var up, total, down int
		if err := rows.Scan(&k.monitor, &k.day, &up, &total, &down); err != nil {
			return nil, err
		}
		counts[k] = [3]int{up, total, down}
		monitors[k.monitor] = true
	}
	out := map[int64][]Day{}
	for id := range monitors {
		list := make([]Day, days)
		for i := range days {
			d := start.AddDate(0, 0, i)
			c := counts[key{id, d.UnixMilli() / 86400000}]
			ratio := -1.0
			if c[1] > 0 {
				ratio = float64(c[0]) / float64(c[1])
			}
			list[i] = Day{Date: d.Format("2006-01-02"), Ratio: ratio, Down: c[2]}
		}
		out[id] = list
	}
	return out, rows.Err()
}

// DeleteHeartbeatsBefore removes old checks and returns how many.
func (s *Store) DeleteHeartbeatsBefore(ctx context.Context, t time.Time) (int64, error) {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM heartbeats WHERE at < ?`, t.UnixMilli())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// Setting returns a setting, or def if it is not set.
func (s *Store) Setting(ctx context.Context, key, def string) string {
	var v string
	if err := s.DB.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&v); err != nil {
		return def
	}
	return v
}

// SetSetting stores a setting.
func (s *Store) SetSetting(ctx context.Context, key, value string) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT (key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

// Placeholders returns "?, ?, ?" for n values.
func Placeholders(n int) string { return strings.TrimSuffix(strings.Repeat("?, ", n), ", ") }

package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// User is an administrator.
type User struct {
	ID           int64
	Username     string
	PasswordHash string
}

// UserCount returns how many administrators exist; 0 means first-run setup.
func (s *Store) UserCount(ctx context.Context) (int, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

// CreateUser adds an administrator.
func (s *Store) CreateUser(ctx context.Context, username, hash string) (int64, error) {
	res, err := s.DB.ExecContext(ctx, `INSERT INTO users (username, password_hash, created_at) VALUES (?, ?, ?)`, username, hash, now())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UserByName finds an administrator.
func (s *Store) UserByName(ctx context.Context, username string) (User, error) {
	var u User
	err := s.DB.QueryRowContext(ctx, `SELECT id, username, password_hash FROM users WHERE username = ?`, username).Scan(&u.ID, &u.Username, &u.PasswordHash)
	if errors.Is(err, sql.ErrNoRows) {
		return u, ErrNotFound
	}
	return u, err
}

// SetPassword changes an administrator's password hash.
func (s *Store) SetPassword(ctx context.Context, id int64, hash string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE users SET password_hash = ? WHERE id = ?`, hash, id)
	return err
}

// Incident is a problem or maintenance shown on the status page.
type Incident struct {
	ID         int64            `json:"id"`
	Title      string           `json:"title"`
	Status     string           `json:"status"`
	Impact     string           `json:"impact"`
	MonitorID  *int64           `json:"monitorId"`
	Auto       bool             `json:"auto"`
	CreatedAt  int64            `json:"createdAt"`
	ResolvedAt *int64           `json:"resolvedAt"`
	Updates    []IncidentUpdate `json:"updates"`
}

// IncidentUpdate is a post on an incident's timeline.
type IncidentUpdate struct {
	ID        int64  `json:"id"`
	Status    string `json:"status"`
	Body      string `json:"body"` // Markdown
	HTML      string `json:"html"` // rendered by the caller
	CreatedAt int64  `json:"createdAt"`
}

// Incident statuses.
const (
	Investigating = "investigating"
	Identified    = "identified"
	Monitoring    = "monitoring"
	Resolved      = "resolved"
)

// OpenIncident creates an incident with its first update.
func (s *Store) OpenIncident(ctx context.Context, title, impact, body string, monitorID *int64, auto bool) (int64, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	t := now()
	res, err := tx.ExecContext(ctx, `INSERT INTO incidents (title, status, impact, monitor_id, auto, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		title, Investigating, impact, monitorID, auto, t)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	if _, err := tx.ExecContext(ctx, `INSERT INTO incident_updates (incident_id, status, body, created_at) VALUES (?, ?, ?, ?)`, id, Investigating, body, t); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

// UpdateIncident posts an update and sets the incident's status;
// "resolved" closes it.
func (s *Store) UpdateIncident(ctx context.Context, id int64, status, body string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	t := now()
	var resolved any
	if status == Resolved {
		resolved = t
	}
	res, err := tx.ExecContext(ctx, `UPDATE incidents SET status = ?, resolved_at = ? WHERE id = ?`, status, resolved, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO incident_updates (incident_id, status, body, created_at) VALUES (?, ?, ?, ?)`, id, status, body, t); err != nil {
		return err
	}
	return tx.Commit()
}

// OpenAutoIncident returns the unresolved incident a monitor opened, if any.
func (s *Store) OpenAutoIncident(ctx context.Context, monitorID int64) (int64, bool, error) {
	var id int64
	err := s.DB.QueryRowContext(ctx, `SELECT id FROM incidents WHERE monitor_id = ? AND auto = 1 AND resolved_at IS NULL ORDER BY id DESC LIMIT 1`, monitorID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	return id, err == nil, err
}

// DeleteIncident deletes an incident.
func (s *Store) DeleteIncident(ctx context.Context, id int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM incidents WHERE id = ?`, id)
	return err
}

// Incidents returns unresolved incidents and those resolved since a time,
// newest first, with their updates.
func (s *Store) Incidents(ctx context.Context, resolvedSince time.Time) ([]Incident, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, title, status, impact, monitor_id, auto, created_at, resolved_at FROM incidents
		WHERE resolved_at IS NULL OR resolved_at >= ? ORDER BY resolved_at IS NOT NULL, created_at DESC`, resolvedSince.UnixMilli())
	if err != nil {
		return nil, err
	}
	var out []Incident
	byID := map[int64]int{}
	for rows.Next() {
		var in Incident
		var monitor, resolved sql.NullInt64
		if err := rows.Scan(&in.ID, &in.Title, &in.Status, &in.Impact, &monitor, &in.Auto, &in.CreatedAt, &resolved); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if monitor.Valid {
			in.MonitorID = &monitor.Int64
		}
		if resolved.Valid {
			in.ResolvedAt = &resolved.Int64
		}
		in.Updates = []IncidentUpdate{}
		byID[in.ID] = len(out)
		out = append(out, in)
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil || len(out) == 0 {
		return out, err
	}
	ups, err := s.DB.QueryContext(ctx, `SELECT u.id, u.incident_id, u.status, u.body, u.created_at FROM incident_updates u
		JOIN incidents i ON i.id = u.incident_id WHERE i.resolved_at IS NULL OR i.resolved_at >= ? ORDER BY u.created_at DESC, u.id DESC`, resolvedSince.UnixMilli())
	if err != nil {
		return nil, err
	}
	defer func() { _ = ups.Close() }()
	for ups.Next() {
		var u IncidentUpdate
		var incident int64
		if err := ups.Scan(&u.ID, &incident, &u.Status, &u.Body, &u.CreatedAt); err != nil {
			return nil, err
		}
		if i, ok := byID[incident]; ok {
			out[i].Updates = append(out[i].Updates, u)
		}
	}
	return out, ups.Err()
}

// Incident returns one incident with its updates.
func (s *Store) Incident(ctx context.Context, id int64) (Incident, error) {
	all, err := s.Incidents(ctx, time.Unix(0, 0))
	if err != nil {
		return Incident{}, err
	}
	for _, in := range all {
		if in.ID == id {
			return in, nil
		}
	}
	return Incident{}, ErrNotFound
}

// Channel is where notifications go.
type Channel struct {
	ID      int64          `json:"id"`
	Name    string         `json:"name"`
	Type    string         `json:"type"`
	Config  map[string]any `json:"config"`
	Enabled bool           `json:"enabled"`
}

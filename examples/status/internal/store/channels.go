package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

// Channels returns every notification channel.
func (s *Store) Channels(ctx context.Context, enabledOnly bool) ([]Channel, error) {
	q := `SELECT id, name, type, config, enabled FROM channels`
	if enabledOnly {
		q += ` WHERE enabled = 1`
	}
	rows, err := s.DB.QueryContext(ctx, q+` ORDER BY name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []Channel{}
	for rows.Next() {
		c, err := scanChannel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func scanChannel(row interface{ Scan(...any) error }) (Channel, error) {
	var c Channel
	var config string
	if err := row.Scan(&c.ID, &c.Name, &c.Type, &config, &c.Enabled); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c, ErrNotFound
		}
		return c, err
	}
	if err := json.Unmarshal([]byte(config), &c.Config); err != nil {
		c.Config = map[string]any{}
	}
	return c, nil
}

// Channel returns one channel.
func (s *Store) Channel(ctx context.Context, id int64) (Channel, error) {
	return scanChannel(s.DB.QueryRowContext(ctx, `SELECT id, name, type, config, enabled FROM channels WHERE id = ?`, id))
}

// SaveChannel inserts c if its ID is 0, or updates it.
func (s *Store) SaveChannel(ctx context.Context, c Channel) (int64, error) {
	config, err := json.Marshal(c.Config)
	if err != nil {
		return 0, err
	}
	if c.ID == 0 {
		res, err := s.DB.ExecContext(ctx, `INSERT INTO channels (name, type, config, enabled, created_at) VALUES (?, ?, ?, ?, ?)`,
			c.Name, c.Type, string(config), c.Enabled, now())
		if err != nil {
			return 0, err
		}
		return res.LastInsertId()
	}
	_, err = s.DB.ExecContext(ctx, `UPDATE channels SET name = ?, type = ?, config = ?, enabled = ? WHERE id = ?`, c.Name, c.Type, string(config), c.Enabled, c.ID)
	return c.ID, err
}

// DeleteChannel deletes a channel.
func (s *Store) DeleteChannel(ctx context.Context, id int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM channels WHERE id = ?`, id)
	return err
}

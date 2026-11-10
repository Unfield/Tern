// Package store keeps the analytics in SQLite: sites, page views, and
// totals per day.
package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
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
	db.SetMaxOpenConns(8)
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	return &Store{DB: db}, nil
}

// Ping checks the database, for the health check.
func (s *Store) Ping(ctx context.Context) error { return s.DB.PingContext(ctx) }

// User is someone who can sign in.
type User struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	Email        string `json:"email"`
	WeeklyReport bool   `json:"weeklyReport"`
	hash         []byte
}

// Hash is the user's bcrypt password hash.
func (u User) Hash() []byte { return u.hash }

// CountUsers is how many accounts there are; none means the app is new.
func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

// CreateUser adds an account.
func (s *Store) CreateUser(ctx context.Context, name, email string, hash []byte) (User, error) {
	res, err := s.DB.ExecContext(ctx, `INSERT INTO users (name, email, password_hash, created_at) VALUES (?, ?, ?, ?)`, name, strings.ToLower(email), hash, time.Now().UnixMilli())
	if err != nil {
		return User{}, err
	}
	id, _ := res.LastInsertId()
	return User{ID: id, Name: name, Email: strings.ToLower(email), WeeklyReport: true, hash: hash}, nil
}

func (s *Store) user(ctx context.Context, where string, arg any) (User, error) {
	var u User
	var weekly int
	err := s.DB.QueryRowContext(ctx, `SELECT id, name, email, weekly_report, password_hash FROM users WHERE `+where, arg).Scan(&u.ID, &u.Name, &u.Email, &weekly, &u.hash)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	u.WeeklyReport = weekly == 1
	return u, err
}

// UserByEmail finds an account to sign in.
func (s *Store) UserByEmail(ctx context.Context, email string) (User, error) {
	return s.user(ctx, "email = ?", strings.ToLower(email))
}

// UserByID finds the signed-in account.
func (s *Store) UserByID(ctx context.Context, id int64) (User, error) {
	return s.user(ctx, "id = ?", id)
}

// ReportRecipients are the accounts that get the weekly email.
func (s *Store) ReportRecipients(ctx context.Context) ([]User, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, name, email FROM users WHERE weekly_report = 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Name, &u.Email); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// Site is a website whose visits are counted.
type Site struct {
	ID     int64  `json:"id"`
	Domain string `json:"domain"`
	Name   string `json:"name"`
}

// CreateSite adds a site.
func (s *Store) CreateSite(ctx context.Context, domain, name string) (Site, error) {
	domain = strings.ToLower(domain)
	res, err := s.DB.ExecContext(ctx, `INSERT INTO sites (domain, name, created_at) VALUES (?, ?, ?)`, domain, name, time.Now().UnixMilli())
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return Site{}, ErrExists
		}
		return Site{}, err
	}
	id, _ := res.LastInsertId()
	return Site{ID: id, Domain: domain, Name: name}, nil
}

// ErrExists is returned when a site is already counted.
var ErrExists = errors.New("already exists")

// Sites lists every site.
func (s *Store) Sites(ctx context.Context) ([]Site, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, domain, name FROM sites ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Site{}
	for rows.Next() {
		var x Site
		if err := rows.Scan(&x.ID, &x.Domain, &x.Name); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// SiteByDomain finds a site.
func (s *Store) SiteByDomain(ctx context.Context, domain string) (Site, error) {
	var x Site
	err := s.DB.QueryRowContext(ctx, `SELECT id, domain, name FROM sites WHERE domain = ?`, strings.ToLower(domain)).Scan(&x.ID, &x.Domain, &x.Name)
	if errors.Is(err, sql.ErrNoRows) {
		return Site{}, ErrNotFound
	}
	return x, err
}

// DeleteSite removes a site and everything counted for it.
func (s *Store) DeleteSite(ctx context.Context, id int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM sites WHERE id = ?`, id)
	return err
}

// Event is one page view.
type Event struct {
	SiteID   int64
	At       time.Time
	Visitor  string
	Path     string
	Referrer string
	Browser  string
	OS       string
	Device   string
}

// AddEvent records a page view.
func (s *Store) AddEvent(ctx context.Context, e Event) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO events (site_id, ts, visitor, path, referrer, browser, os, device) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		e.SiteID, e.At.UnixMilli(), e.Visitor, e.Path, e.Referrer, e.Browser, e.OS, e.Device)
	return err
}

// Visitor returns the anonymous visitor ID for a page view: a hash of the
// day's secret, the site, the address and the browser. It's the same for a
// visitor all day, and can't be linked to them or to the next day.
func (s *Store) Visitor(ctx context.Context, at time.Time, domain, ip, userAgent string) (string, error) {
	day := at.UTC().Format(time.DateOnly)
	var salt []byte
	err := s.DB.QueryRowContext(ctx, `SELECT salt FROM salts WHERE day = ?`, day).Scan(&salt)
	if errors.Is(err, sql.ErrNoRows) {
		salt = make([]byte, 32)
		if _, err := rand.Read(salt); err != nil {
			return "", err
		}
		if _, err := s.DB.ExecContext(ctx, `INSERT OR IGNORE INTO salts (day, salt) VALUES (?, ?)`, day, salt); err != nil {
			return "", err
		}
		// Another request may have won the race; use whichever is stored.
		if err := s.DB.QueryRowContext(ctx, `SELECT salt FROM salts WHERE day = ?`, day).Scan(&salt); err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	}
	h := sha256.New()
	h.Write(salt)
	for _, part := range []string{domain, ip, userAgent} {
		h.Write([]byte{0})
		h.Write([]byte(part))
	}
	return hex.EncodeToString(h.Sum(nil)[:12]), nil
}

// ForgetSalts deletes the secrets of past days, so their hashes can't be
// recomputed.
func (s *Store) ForgetSalts(ctx context.Context, now time.Time) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM salts WHERE day < ?`, now.UTC().Format(time.DateOnly))
	return err
}

// Rollup stores totals per day for the events since from.
func (s *Store) Rollup(ctx context.Context, from time.Time) error {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO daily (site_id, day, visitors, pageviews)
		SELECT site_id, date(ts / 1000, 'unixepoch'), COUNT(DISTINCT visitor), COUNT(*)
		FROM events WHERE ts >= ?
		GROUP BY site_id, date(ts / 1000, 'unixepoch')
		ON CONFLICT (site_id, day) DO UPDATE SET visitors = excluded.visitors, pageviews = excluded.pageviews`,
		startOfDay(from).UnixMilli())
	return err
}

// Prune deletes page views before t; their totals stay in daily.
func (s *Store) Prune(ctx context.Context, before time.Time) (int64, error) {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM events WHERE ts < ?`, before.UnixMilli())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func startOfDay(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

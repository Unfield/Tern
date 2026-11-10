// Package migrate applies SQL migrations to SQLite or PostgreSQL. Write
// migrations as files named <version>_<name>.sql, embed them, and apply them
// when the app starts:
//
//	//go:embed migrations/*.sql
//	var migrations embed.FS
//
//	app, err := ternApp(kit.WithPlugins(migrate.Plugin(db, migrations)))
//
// `tern migrate new add users` creates a file with a timestamp version. A
// file may have an "-- +down" section that undoes it, for [Down]:
//
//	CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL);
//
//	-- +down
//	DROP TABLE users;
//
// Each migration runs in a transaction and is recorded in the
// tern_migrations table with a checksum, so an applied migration that was
// edited afterwards is reported instead of silently ignored.
package migrate

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Unfield/Tern/kit"
	"github.com/Unfield/Tern/sqldb"
)

// Migration is one migration file and whether it was applied.
type Migration struct {
	Version   int64
	Name      string
	Applied   bool
	AppliedAt time.Time

	file     string
	up, down string
	checksum string
}

var fileRe = regexp.MustCompile(`^(\d+)_([\w.-]+)\.sql$`)

// Load reads the migrations in fsys: every <version>_<name>.sql file in its
// root, or in a migrations directory if the root has one.
func Load(fsys fs.FS) ([]Migration, error) {
	dir := "."
	if info, err := fs.Stat(fsys, "migrations"); err == nil && info.IsDir() {
		dir = "migrations"
	}
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	var out []Migration
	seen := map[int64]string{}
	for _, e := range entries {
		if e.IsDir() || path.Ext(e.Name()) != ".sql" {
			continue
		}
		m := fileRe.FindStringSubmatch(e.Name())
		if m == nil {
			return nil, fmt.Errorf("migrate: %s: migration files are named <version>_<name>.sql", e.Name())
		}
		version, err := strconv.ParseInt(m[1], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("migrate: %s: %w", e.Name(), err)
		}
		if other, ok := seen[version]; ok {
			return nil, fmt.Errorf("migrate: %s and %s have the same version", other, e.Name())
		}
		seen[version] = e.Name()
		src, err := fs.ReadFile(fsys, path.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		up, down := split(string(src))
		sum := sha256.Sum256([]byte(strings.TrimSpace(up)))
		out = append(out, Migration{Version: version, Name: m[2], file: e.Name(), up: up, down: down, checksum: hex.EncodeToString(sum[:])})
	}
	slices.SortFunc(out, func(a, b Migration) int { return int(a.Version - b.Version) })
	return out, nil
}

var downRe = regexp.MustCompile(`(?m)^--\s*\+down\s*$`)
var upRe = regexp.MustCompile(`(?m)^--\s*\+up\s*$`)

func split(src string) (up, down string) {
	src = upRe.ReplaceAllString(src, "")
	if loc := downRe.FindStringIndex(src); loc != nil {
		return src[:loc[0]], src[loc[1]:]
	}
	return src, ""
}

const table = "tern_migrations"

func ensureTable(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS `+table+` (
		version BIGINT PRIMARY KEY,
		name TEXT NOT NULL,
		checksum TEXT NOT NULL,
		applied_at BIGINT NOT NULL
	)`)
	return err
}

type applied struct {
	checksum string
	at       time.Time
}

func appliedVersions(ctx context.Context, q interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}) (map[int64]applied, error) {
	rows, err := q.QueryContext(ctx, `SELECT version, checksum, applied_at FROM `+table)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[int64]applied{}
	for rows.Next() {
		var v, at int64
		var sum string
		if err := rows.Scan(&v, &sum, &at); err != nil {
			return nil, err
		}
		out[v] = applied{checksum: sum, at: time.UnixMilli(at)}
	}
	return out, rows.Err()
}

// Status lists every migration and whether it was applied.
func Status(ctx context.Context, db *sql.DB, fsys fs.FS) ([]Migration, error) {
	ms, err := Load(fsys)
	if err != nil {
		return nil, err
	}
	if err := ensureTable(ctx, db); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	done, err := appliedVersions(ctx, db)
	if err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	for i := range ms {
		if a, ok := done[ms[i].Version]; ok {
			ms[i].Applied, ms[i].AppliedAt = true, a.at
		}
	}
	return ms, nil
}

// ErrChanged is returned when an applied migration's file was edited.
var ErrChanged = errors.New("migrate: an applied migration was changed")

// Up applies every pending migration in version order and returns the ones
// it applied. It stops at the first error; migrations before it stay
// applied. Running instances may call it concurrently on Postgres, where an
// advisory lock serializes them.
func Up(ctx context.Context, db *sql.DB, fsys fs.FS) ([]Migration, error) {
	d, err := sqldb.Detect(db)
	if err != nil {
		return nil, err
	}
	ms, err := Load(fsys)
	if err != nil {
		return nil, err
	}
	if err := ensureTable(ctx, db); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	done, err := appliedVersions(ctx, db)
	if err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	for _, m := range ms {
		if a, ok := done[m.Version]; ok && a.checksum != m.checksum {
			return nil, fmt.Errorf("%w: %s was edited after it was applied; add a new migration instead", ErrChanged, m.file)
		}
	}
	var out []Migration
	for _, m := range ms {
		if _, ok := done[m.Version]; ok {
			continue
		}
		ran, err := apply(ctx, db, d, m)
		if err != nil {
			return out, err
		}
		if ran {
			m.Applied, m.AppliedAt = true, time.Now()
			out = append(out, m)
		}
	}
	return out, nil
}

func apply(ctx context.Context, db *sql.DB, d sqldb.Dialect, m Migration) (bool, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	if d == sqldb.Postgres {
		if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(7366734)`); err != nil {
			return false, fmt.Errorf("migrate: lock: %w", err)
		}
		// Another instance may have applied it while we waited.
		var n int
		if err := tx.QueryRowContext(ctx, d.Rebind(`SELECT COUNT(*) FROM `+table+` WHERE version = ?`), m.Version).Scan(&n); err != nil {
			return false, err
		}
		if n > 0 {
			return false, nil
		}
	}
	if strings.TrimSpace(m.up) != "" {
		if _, err := tx.ExecContext(ctx, m.up); err != nil {
			return false, fmt.Errorf("migrate: %s: %w", m.file, err)
		}
	}
	if _, err := tx.ExecContext(ctx, d.Rebind(`INSERT INTO `+table+` (version, name, checksum, applied_at) VALUES (?, ?, ?, ?)`),
		m.Version, m.Name, m.checksum, time.Now().UnixMilli()); err != nil {
		return false, fmt.Errorf("migrate: recording %s: %w", m.file, err)
	}
	return true, tx.Commit()
}

// Down undoes the last steps applied migrations with their "-- +down"
// sections, newest first, and returns them.
func Down(ctx context.Context, db *sql.DB, fsys fs.FS, steps int) ([]Migration, error) {
	d, err := sqldb.Detect(db)
	if err != nil {
		return nil, err
	}
	ms, err := Status(ctx, db, fsys)
	if err != nil {
		return nil, err
	}
	var out []Migration
	for i := len(ms) - 1; i >= 0 && len(out) < steps; i-- {
		m := ms[i]
		if !m.Applied {
			continue
		}
		if strings.TrimSpace(m.down) == "" {
			return out, fmt.Errorf("migrate: %s has no -- +down section", m.file)
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return out, err
		}
		if _, err := tx.ExecContext(ctx, m.down); err != nil {
			_ = tx.Rollback()
			return out, fmt.Errorf("migrate: %s down: %w", m.file, err)
		}
		if _, err := tx.ExecContext(ctx, d.Rebind(`DELETE FROM `+table+` WHERE version = ?`), m.Version); err != nil {
			_ = tx.Rollback()
			return out, err
		}
		if err := tx.Commit(); err != nil {
			return out, err
		}
		m.Applied = false
		out = append(out, m)
	}
	return out, nil
}

// Plugin applies pending migrations when the server starts, before it
// accepts requests, and logs each one. A failed migration stops the server.
func Plugin(db *sql.DB, fsys fs.FS) kit.Plugin { return &plugin{db: db, fsys: fsys} }

type plugin struct {
	db   *sql.DB
	fsys fs.FS
}

func (p *plugin) Name() string { return "migrate" }

func (p *plugin) Setup(s *kit.Setup) error {
	if _, err := Load(p.fsys); err != nil {
		return err
	}
	log := s.Logger()
	s.OnStart(func(ctx context.Context) error {
		ran, err := Up(ctx, p.db, p.fsys)
		for _, m := range ran {
			log.Info("applied migration", "version", m.Version, "name", m.Name)
		}
		return err
	})
	return nil
}

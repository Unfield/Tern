// Package dbtest opens the databases that Tern's database packages are
// tested against: a fresh SQLite file always, and PostgreSQL when
// TERN_TEST_POSTGRES holds a connection string (CI sets it).
package dbtest

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // postgres driver
	_ "modernc.org/sqlite"             // sqlite driver
)

// Each runs fn once per available database, as a subtest named after it.
func Each(t *testing.T, fn func(t *testing.T, db *sql.DB)) {
	t.Run("sqlite", func(t *testing.T) {
		db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "test.db")+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = db.Close() })
		fn(t, db)
	})
	dsn := os.Getenv("TERN_TEST_POSTGRES")
	if dsn == "" {
		return
	}
	t.Run("postgres", func(t *testing.T) {
		admin, err := sql.Open("pgx", dsn)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = admin.Close() }()
		// A schema per test keeps tests independent.
		schema := fmt.Sprintf("t%d", time.Now().UnixNano())
		ctx := context.Background()
		if _, err := admin.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if a, err := sql.Open("pgx", dsn); err == nil {
				_, _ = a.ExecContext(ctx, "DROP SCHEMA "+schema+" CASCADE")
				_ = a.Close()
			}
		})
		db, err := sql.Open("pgx", dsn+sep(dsn)+"search_path="+schema)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = db.Close() })
		fn(t, db)
	})
}

func sep(dsn string) string {
	for _, c := range dsn {
		if c == '?' {
			return "&"
		}
	}
	return "?"
}

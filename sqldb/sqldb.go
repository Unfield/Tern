// Package sqldb holds the small amount of SQL dialect knowledge shared by
// Tern's database packages (migrate and jobs). They support SQLite and
// PostgreSQL through database/sql, with any driver.
package sqldb

import (
	"database/sql"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// Dialect is a SQL dialect.
type Dialect int

const (
	// SQLite is for drivers such as modernc.org/sqlite and mattn/go-sqlite3.
	SQLite Dialect = iota + 1
	// Postgres is for drivers such as pgx and lib/pq.
	Postgres
)

func (d Dialect) String() string {
	switch d {
	case SQLite:
		return "sqlite"
	case Postgres:
		return "postgres"
	}
	return "unknown"
}

// Detect returns the dialect of db's driver.
func Detect(db *sql.DB) (Dialect, error) {
	t := reflect.TypeOf(db.Driver())
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	name := strings.ToLower(t.PkgPath() + "." + t.Name())
	switch {
	case strings.Contains(name, "sqlite"):
		return SQLite, nil
	case strings.Contains(name, "pgx"), strings.Contains(name, "lib/pq"), strings.Contains(name, "postgres"):
		return Postgres, nil
	}
	return 0, fmt.Errorf("sqldb: unsupported database driver %s (Tern supports SQLite and PostgreSQL)", name)
}

// Rebind rewrites ? placeholders to $1, $2, … for Postgres. It leaves ?
// inside single-quoted strings alone.
func (d Dialect) Rebind(query string) string {
	if d != Postgres || !strings.Contains(query, "?") {
		return query
	}
	var b strings.Builder
	n := 0
	quoted := false
	for _, r := range query {
		switch {
		case r == '\'':
			quoted = !quoted
		case r == '?' && !quoted:
			n++
			b.WriteString("$" + strconv.Itoa(n))
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// AutoID is the column type of an auto-incrementing integer primary key.
func (d Dialect) AutoID() string {
	if d == Postgres {
		return "BIGSERIAL PRIMARY KEY"
	}
	return "INTEGER PRIMARY KEY AUTOINCREMENT"
}

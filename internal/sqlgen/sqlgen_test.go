package sqlgen

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite sqlgentest/queries/queries.gen.go")

// The generated package for sqlgentest is committed; its own tests run the
// queries on SQLite and PostgreSQL.
func TestGenerateGolden(t *testing.T) {
	got, err := Generate(context.Background(), "sqlgentest")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join("sqlgentest", Dir, File)
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, _ := os.ReadFile(path)
	if string(got) != string(want) {
		t.Errorf("%s is out of date; run go test ./internal/sqlgen -update and review the diff\n%s", path, got)
	}
}

func TestGenerateErrors(t *testing.T) {
	for _, tt := range []struct{ name, sql, want string }{
		{"unknown column", "-- name: Q :many\nSELECT nope FROM users;", "queries/q.sql:1: Q: no such column: nope"},
		{"unknown parameter type", "-- name: Q :many\nSELECT id FROM users WHERE length(email) > :n;", "can't tell the type of :n from the schema; say it: -- param n int64"},
		{"declared parameter", "-- name: Q :many\n-- param n int64\nSELECT id FROM users WHERE length(email) > :n;", ""},
		{"expression column", "-- name: Q :one\nSELECT max(id) + 1 AS next FROM users;", "can't tell the type of column next; say it: -- column next int64"},
		{"declared column", "-- name: Q :one\n-- column next *int64\nSELECT max(id) + 1 AS next FROM users;", ""},
		{"declared time parameter", "-- name: Since :many\n-- param since time.Time\nSELECT id FROM links WHERE julianday(added_at) > julianday(:since);", ""},
		{"unchecked", "-- name: Find :many\n-- unchecked\n-- param q string\n-- column id int64\n-- column title string\nSELECT id, title FROM links WHERE title ILIKE '%' || :q || '%';", ""},
		{"unchecked without types", "-- name: Find :many\n-- unchecked\nSELECT id FROM links WHERE title ILIKE :q;", "needs a -- param line for :q"},
		{"unnamed column", "-- name: Q :one\nSELECT id + 1 FROM users;", `column 1 is named "id + 1"; name it with AS`},
		{"positional placeholders", "-- name: Q :many\nSELECT id FROM users WHERE email = ?;", "use named parameters, :name, instead of ?"},
		{"exec returning", "-- name: Q :exec\nDELETE FROM users WHERE id = :id RETURNING id;", "the query returns rows (RETURNING); make it :one or :many"},
		{"no columns", "-- name: Q :one\nUPDATE users SET name = :name WHERE id = :id;", "the query returns no columns; make it :exec"},
		{"bad name line", "-- name: Q one\nSELECT 1;", "write the name line as -- name: QueryName :one"},
		{"twice", "-- name: Q :exec\nDELETE FROM users;\n-- name: Q :exec\nDELETE FROM links;", "query Q is also defined at"},
		{"row types differ", "-- name: A :many\n-- row R\nSELECT id, email FROM users;\n-- name: B :many\n-- row R\nSELECT id, name FROM users;", "B returns other columns than A, which also uses the row type R"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			mustCopy(t, "sqlgentest/migrations/20261112000000_links.sql", filepath.Join(root, "migrations", "20261112000000_links.sql"))
			if err := os.MkdirAll(filepath.Join(root, "queries"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "queries", "q.sql"), []byte(tt.sql), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := Generate(context.Background(), root)
			switch {
			case tt.want == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tt.want != "" && (err == nil || !strings.Contains(err.Error(), tt.want)):
				t.Fatalf("error %v, want it to contain %q", err, tt.want)
			}
		})
	}
}

func TestNames(t *testing.T) {
	for in, want := range map[string][3]string{
		"user_id":  {"UserID", "userID", "userId"},
		"added_at": {"AddedAt", "addedAt", "addedAt"},
		"url":      {"URL", "url", "url"},
		"type":     {"Type", "type_", "type"},
		"n":        {"N", "n", "n"},
	} {
		if got := [3]string{goName(in), lowerName(in), jsonName(in)}; got != want {
			t.Errorf("%s: %v, want %v", in, got, want)
		}
	}
}

func mustCopy(t *testing.T, from, to string) {
	t.Helper()
	b, err := os.ReadFile(from)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(to, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

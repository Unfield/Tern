package migrate

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"testing/fstest"

	_ "modernc.org/sqlite"

	"github.com/Unfield/Tern/sqldb"
)

func TestCheck(t *testing.T) {
	fsys := fstest.MapFS{
		"1_users.sql": {Data: []byte("CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT NOT NULL, old TEXT);\n-- Fine: the table is new in this file.\nALTER TABLE users ADD COLUMN name TEXT NOT NULL;\n\n-- +down\nDROP TABLE users;\n")},
		"2_change.sql": {Data: []byte(`ALTER TABLE users ADD COLUMN plan TEXT NOT NULL;
ALTER TABLE users ADD COLUMN team INTEGER NOT NULL DEFAULT 0;
ALTER TABLE users DROP COLUMN old;
-- safe: nothing has read it since v1.2
ALTER TABLE users RENAME COLUMN email TO address;
UPDATE users SET team = 1;
DELETE FROM users WHERE id = 0;
ALTER TABLE users ALTER COLUMN team TYPE BIGINT;
`)},
	}
	ps, err := Check(context.Background(), fsys, CheckOptions{Dialect: sqldb.Postgres})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]Problem{}
	for _, p := range ps {
		w := strings.SplitN(p.Message, " ", 3)
		got[p.File+":"+w[0]+" "+w[1]+":"+string(p.Level)] = p
	}
	for key, line := range map[string]int{
		"2_change.sql:adds users.plan:error": 1,
		"2_change.sql:drops the:warning":     3,
		"2_change.sql:changes every:warning": 6,
		"2_change.sql:changes the:warning":   8,
		"2_change.sql:has no:note":           1,
	} {
		p, ok := got[key]
		if !ok || p.Line != line {
			t.Errorf("%s: %+v (all: %v)", key, p, ps)
		}
	}
	for _, p := range ps {
		switch {
		case p.File == "1_users.sql":
			t.Errorf("the first migration only changes a table it made: %v", p)
		case strings.Contains(p.Message, "renames"):
			t.Errorf("a statement marked safe: %v", p)
		case strings.Contains(p.Message, "deletes every row"):
			t.Errorf("a DELETE with a WHERE: %v", p)
		}
	}
	// The type change only locks on PostgreSQL.
	ps, _ = Check(context.Background(), fsys, CheckOptions{Dialect: sqldb.SQLite})
	for _, p := range ps {
		if strings.Contains(p.Message, "changes the type") {
			t.Errorf("SQLite: %v", p)
		}
	}
}

func TestCheckRoundTrip(t *testing.T) {
	open := func(t *testing.T) *sql.DB {
		db, err := sql.Open("sqlite", "file:"+t.Name()+"?mode=memory&cache=shared")
		if err != nil {
			t.Fatal(err)
		}
		db.SetMaxOpenConns(1)
		t.Cleanup(func() { _ = db.Close() })
		return db
	}
	good := fstest.MapFS{
		"1_a.sql": {Data: []byte("CREATE TABLE a (id INTEGER);\n-- +down\nDROP TABLE a;\n")},
		"2_b.sql": {Data: []byte("CREATE INDEX a_id ON a (id);\n-- +down\nDROP INDEX a_id;\n")},
	}
	if ps, err := Check(context.Background(), good, CheckOptions{DB: open(t)}); err != nil || len(ps) != 0 {
		t.Errorf("good migrations: %v, %v", ps, err)
	}
	t.Run("down leaves something", func(t *testing.T) {
		bad := fstest.MapFS{
			"1_a.sql": {Data: []byte("CREATE TABLE a (id INTEGER);\n-- +down\nSELECT 1;\n")},
		}
		ps, _ := Check(context.Background(), bad, CheckOptions{DB: open(t)})
		if len(ps) != 1 || ps[0].Level != Error || !strings.Contains(ps[0].Message, "doesn't undo it all") {
			t.Errorf("%v", ps)
		}
	})
	t.Run("fails", func(t *testing.T) {
		bad := fstest.MapFS{"1_a.sql": {Data: []byte("CREATE TABLE a (id INTEGER;\n-- +down\nDROP TABLE a;\n")}}
		ps, _ := Check(context.Background(), bad, CheckOptions{DB: open(t)})
		if len(ps) != 1 || !strings.Contains(ps[0].Message, "fails on an empty database") {
			t.Errorf("%v", ps)
		}
	})
}

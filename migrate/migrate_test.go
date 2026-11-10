package migrate_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"testing/fstest"

	"github.com/Unfield/Tern/internal/dbtest"
	"github.com/Unfield/Tern/migrate"
)

func files() fstest.MapFS {
	return fstest.MapFS{
		"migrations/0001_users.sql": {Data: []byte("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL);\n\n-- +down\nDROP TABLE users;\n")},
		"migrations/0002_posts.sql": {Data: []byte("-- +up\nCREATE TABLE posts (id INTEGER PRIMARY KEY, title TEXT);\nCREATE INDEX posts_title ON posts (title);\n-- +down\nDROP TABLE posts;\n")},
		"migrations/README.md":      {Data: []byte("not a migration")},
		"migrations/0003_seed.sql":  {Data: []byte("INSERT INTO users (id, name) VALUES (1, 'ada');\n")},
	}
}

func tables(t *testing.T, db *sql.DB, names ...string) {
	t.Helper()
	for _, n := range names {
		if _, err := db.Exec("SELECT COUNT(*) FROM " + n); err != nil {
			t.Errorf("table %s: %v", n, err)
		}
	}
}

func TestUpDownStatus(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db *sql.DB) {
		ctx := context.Background()
		fsys := files()
		ran, err := migrate.Up(ctx, db, fsys)
		if err != nil || len(ran) != 3 || ran[0].Name != "users" || ran[2].Version != 3 {
			t.Fatalf("Up = %+v, %v", ran, err)
		}
		tables(t, db, "users", "posts")
		if ran, err := migrate.Up(ctx, db, fsys); err != nil || len(ran) != 0 {
			t.Fatalf("second Up = %+v, %v", ran, err)
		}

		st, err := migrate.Status(ctx, db, fsys)
		if err != nil || len(st) != 3 || !st[1].Applied || st[1].AppliedAt.IsZero() {
			t.Fatalf("Status = %+v, %v", st, err)
		}

		// 0003 has no down section.
		if _, err := migrate.Down(ctx, db, fsys, 1); err == nil {
			t.Error("Down past a migration without -- +down should fail")
		}
		fsys["migrations/0003_seed.sql"] = &fstest.MapFile{Data: []byte("INSERT INTO users (id, name) VALUES (1, 'ada');\n-- +down\nDELETE FROM users WHERE id = 1;\n")}
		down, err := migrate.Down(ctx, db, fsys, 2)
		if err != nil || len(down) != 2 || down[0].Version != 3 || down[1].Version != 2 {
			t.Fatalf("Down = %+v, %v", down, err)
		}
		if _, err := db.Exec("SELECT 1 FROM posts"); err == nil {
			t.Error("posts still exists after Down")
		}
		if ran, err := migrate.Up(ctx, db, fsys); err != nil || len(ran) != 2 {
			t.Fatalf("Up after Down = %+v, %v", ran, err)
		}
	})
}

func TestEditedMigration(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db *sql.DB) {
		ctx := context.Background()
		fsys := files()
		if _, err := migrate.Up(ctx, db, fsys); err != nil {
			t.Fatal(err)
		}
		fsys["migrations/0001_users.sql"] = &fstest.MapFile{Data: []byte("CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT);")}
		if _, err := migrate.Up(ctx, db, fsys); !errors.Is(err, migrate.ErrChanged) {
			t.Errorf("err = %v, want ErrChanged", err)
		}
	})
}

func TestFailedMigrationRollsBack(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db *sql.DB) {
		ctx := context.Background()
		fsys := fstest.MapFS{
			"0001_ok.sql":  {Data: []byte("CREATE TABLE a (id INTEGER);")},
			"0002_bad.sql": {Data: []byte("CREATE TABLE b (id INTEGER); THIS IS NOT SQL;")},
		}
		ran, err := migrate.Up(ctx, db, fsys)
		if err == nil || len(ran) != 1 {
			t.Fatalf("Up = %+v, %v", ran, err)
		}
		if _, err := db.Exec("SELECT 1 FROM b"); err == nil {
			t.Error("the failed migration's table exists: it was not rolled back")
		}
		st, _ := migrate.Status(ctx, db, fsys)
		if st[1].Applied {
			t.Error("the failed migration is recorded as applied")
		}
	})
}

func TestBadFileNames(t *testing.T) {
	for name, fsys := range map[string]fstest.MapFS{
		"no version": {"users.sql": {Data: []byte("x")}},
		"duplicate":  {"1_a.sql": {Data: []byte("x")}, "01_b.sql": {Data: []byte("y")}},
	} {
		if _, err := migrate.Load(fsys); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

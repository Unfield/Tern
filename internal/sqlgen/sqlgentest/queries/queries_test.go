package queries_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/Unfield/Tern/internal/dbtest"
	"github.com/Unfield/Tern/internal/sqlgen/sqlgentest/migrations"
	"github.com/Unfield/Tern/internal/sqlgen/sqlgentest/queries"
	"github.com/Unfield/Tern/migrate"
)

// The generated queries run on SQLite, and on PostgreSQL when
// TERN_TEST_POSTGRES is set.
func TestQueries(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db *sql.DB) {
		ctx := context.Background()
		if _, err := migrate.Up(ctx, db, migrations.FS); err != nil {
			t.Fatal(err)
		}
		q, err := queries.New(db)
		if err != nil {
			t.Fatal(err)
		}
		name := "Ada"
		id, err := q.AddUser(ctx, 1, "ada@example.com", &name)
		if err != nil || id != 1 {
			t.Fatalf("AddUser: %d, %v", id, err)
		}
		u, err := q.UserByEmail(ctx, "ada@example.com")
		if err != nil || u.ID != 1 || u.Name == nil || *u.Name != "Ada" {
			t.Fatalf("UserByEmail: %+v, %v", u, err)
		}
		if _, err := q.UserByEmail(ctx, "nobody@example.com"); !errors.Is(err, queries.ErrNotFound) {
			t.Errorf("a missing user: %v", err)
		}

		note := "later"
		for i, title := range []string{"Runes", "Go 1.26", "Rune tips"} {
			in := queries.AddLinkParams{ID: int64(i + 1), UserID: 1, URL: "https://example.com", Title: title}
			if i == 0 {
				in.Note = &note
			}
			l, err := q.AddLink(ctx, in)
			if err != nil || l.ID != int64(i+1) || l.Title != title || l.Read || l.AddedAt.IsZero() {
				t.Fatalf("AddLink: %+v, %v", l, err)
			}
		}
		if n, err := q.MarkRead(ctx, true, 2, 1); err != nil || n != 1 {
			t.Fatalf("MarkRead: %d, %v", n, err)
		}
		if n, _ := q.MarkRead(ctx, true, 2, 99); n != 0 {
			t.Errorf("MarkRead of another user's link changed %d rows", n)
		}
		unread, err := q.LinksOf(ctx, 1, "unread", 10)
		if err != nil || len(unread) != 2 || unread[0].ID != 3 || unread[1].Note == nil || *unread[1].Note != "later" {
			t.Fatalf("LinksOf unread: %+v, %v", unread, err)
		}
		if read, _ := q.LinksOf(ctx, 1, "read", 10); len(read) != 1 || read[0].ID != 2 {
			t.Errorf("LinksOf read: %+v", read)
		}
		if limited, _ := q.LinksOf(ctx, 1, "all", 2); len(limited) != 2 {
			t.Errorf("LinksOf with a limit: %d", len(limited))
		}
		if none, err := q.LinksOf(ctx, 99, "all", 10); err != nil || none == nil || len(none) != 0 {
			t.Errorf("LinksOf for nobody: %#v, %v (want an empty slice, for JSON)", none, err)
		}
		if n, err := q.CountLinks(ctx, 1); err != nil || n != 3 {
			t.Errorf("CountLinks: %d, %v", n, err)
		}
		if titles, err := q.Titles(ctx, 1, "Rune%"); err != nil || len(titles) != 2 || titles[0] != "Rune tips" {
			t.Errorf("Titles: %v, %v", titles, err)
		}
		// A transaction that's rolled back leaves nothing.
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := q.WithTx(tx).DeleteLinks(ctx, 1); err != nil {
			t.Fatal(err)
		}
		_ = tx.Rollback()
		if n, _ := q.CountLinks(ctx, 1); n != 3 {
			t.Errorf("after a rolled-back delete: %d links", n)
		}
	})
}

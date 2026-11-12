package sealed

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/Unfield/Tern/internal/dbtest"
	"github.com/Unfield/Tern/settings"
	"github.com/Unfield/Tern/sqldb"
)

var (
	keyOld = settings.Secret(strings.Repeat("o", 32))
	keyNew = settings.Secret(strings.Repeat("n", 32))
)

func TestColumns(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db *sql.DB) {
		ctx := context.Background()
		AllowPlain(false)
		if err := SetKeys(keyOld); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, `CREATE TABLE people (id INTEGER PRIMARY KEY, phone TEXT, notes TEXT, phone_hash TEXT)`); err != nil {
			t.Fatal(err)
		}
		hash, _ := Blind("+49 30 1234567")
		ins := `INSERT INTO people (id, phone, notes, phone_hash) VALUES (1, ?, ?, ?)`
		if _, err := db.ExecContext(ctx, rebind(db, ins), String("+49 30 1234567"), Bytes("private"), hash); err != nil {
			t.Fatal(err)
		}
		// In the database: ciphertext.
		var raw string
		if err := db.QueryRowContext(ctx, `SELECT phone FROM people`).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(raw, "s1.") || strings.Contains(raw, "1234567") {
			t.Fatalf("stored as %q", raw)
		}
		// Found by its blind hash, and opened.
		var phone String
		var notes Bytes
		if err := db.QueryRowContext(ctx, rebind(db, `SELECT phone, notes FROM people WHERE phone_hash = ?`), hash).Scan(&phone, &notes); err != nil {
			t.Fatal(err)
		}
		if phone != "+49 30 1234567" || string(notes) != "private" {
			t.Errorf("opened %q, %q", phone, notes)
		}

		// Rotation: the old key still opens what it sealed.
		if err := SetKeys(keyNew, keyOld); err != nil {
			t.Fatal(err)
		}
		if Current(raw) {
			t.Error("a value sealed with the old key is current")
		}
		phone = ""
		if err := db.QueryRowContext(ctx, `SELECT phone FROM people`).Scan(&phone); err != nil || phone != "+49 30 1234567" {
			t.Errorf("after rotating: %q, %v", phone, err)
		}
		// Without the old key, it can't.
		_ = SetKeys(keyNew)
		if err := db.QueryRowContext(ctx, `SELECT phone FROM people`).Scan(&phone); !errors.Is(err, ErrUnknownKey) {
			t.Errorf("old key dropped: %v", err)
		}
	})
}

func rebind(db *sql.DB, q string) string {
	d, _ := sqldb.Detect(db)
	return d.Rebind(q)
}

func TestPlainAndNull(t *testing.T) {
	_ = SetKeys(keyNew)
	var s String
	if err := s.Scan("not sealed"); !errors.Is(err, ErrNotSealed) {
		t.Errorf("a plain value: %v", err)
	}
	AllowPlain(true)
	defer AllowPlain(false)
	if err := s.Scan([]byte("not sealed")); err != nil || s != "not sealed" {
		t.Errorf("a plain value allowed: %q, %v", s, err)
	}
	if err := s.Scan(nil); err != nil || s != "" {
		t.Errorf("NULL: %q, %v", s, err)
	}
	if v, _ := s.LogValue().Any().(string); v != "[sealed]" {
		t.Errorf("logged as %q", v)
	}
	if err := SetKeys("short"); err == nil {
		t.Error("a short key")
	}
}

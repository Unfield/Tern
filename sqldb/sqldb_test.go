package sqldb

import "testing"

func TestRebind(t *testing.T) {
	for in, want := range map[string]string{
		"SELECT ? , ?":                 "SELECT $1 , $2",
		"SELECT '?' WHERE a = ?":       "SELECT '?' WHERE a = $1",
		"SELECT 1":                     "SELECT 1",
		"UPDATE t SET a='it''s?', b=?": "UPDATE t SET a='it''s?', b=$1",
	} {
		if got := Postgres.Rebind(in); got != want {
			t.Errorf("Rebind(%q) = %q, want %q", in, got, want)
		}
		if got := SQLite.Rebind(in); got != in {
			t.Errorf("SQLite changed %q", in)
		}
	}
}

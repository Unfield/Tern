package accounts_test

import (
	"context"
	"database/sql"
	"net/url"
	"testing"
	"time"

	"github.com/Unfield/Tern/accounts"
	"github.com/Unfield/Tern/admin"
	"github.com/Unfield/Tern/internal/admintest"
	"github.com/Unfield/Tern/internal/dbtest"
	"github.com/Unfield/Tern/kit"
	"github.com/Unfield/Tern/sqldb"
)

func TestAccountsAdmin(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db *sql.DB) {
		ctx := context.Background()
		acc, err := accounts.New(db, accounts.Options{})
		if err != nil {
			t.Fatal(err)
		}
		if err := acc.Migrate(ctx); err != nil {
			t.Fatal(err)
		}
		d, _ := sqldb.Detect(db)
		now := time.Now().UnixMilli()
		exec := func(q string, args ...any) {
			if _, err := db.ExecContext(ctx, d.Rebind(q), args...); err != nil {
				t.Fatal(err)
			}
		}
		exec(`INSERT INTO tern_users (email, name, password, created_at) VALUES (?, ?, '', ?)`, "ada@example.com", "Ada", now)
		var id int64
		if err := db.QueryRowContext(ctx, `SELECT id FROM tern_users`).Scan(&id); err != nil {
			t.Fatal(err)
		}
		exec(`INSERT INTO tern_sessions (id, user_id, created_at, last_seen, expires_at, remember, user_agent, ip) VALUES ('s1', ?, ?, ?, ?, 1, 'test', '127.0.0.1')`, id, now, now, now+3600000)
		exec(`INSERT INTO tern_audit (at, user_id, team_id, action, target, ip, user_agent, data) VALUES (?, ?, 0, 'user.signed_in', '', '203.0.113.7', 'test', '{}')`, now, id)

		app := admintest.App(t, admin.New(admin.Options{Authorize: func(*kit.Ctx) error { return nil }}, acc.UsersAdmin(), acc.AuditAdmin()), acc)
		users := admintest.Get(t, app, "/admin/users?q=ada")
		if users.List.Total != 1 || users.List.Rows[0].Values["email"] != "ada@example.com" {
			t.Fatalf("users = %+v", users.List)
		}
		key := users.List.Rows[0].Key
		if p := admintest.Get(t, app, "/admin/users/"+key); !p.Resource.Edit || len(p.Resource.Actions) != 1 || !p.Resource.Actions[0].Danger {
			t.Errorf("user page: %+v", p.Resource)
		}
		// Only the name changes here.
		if loc, msg, _ := admintest.Post(t, app, "/admin/users/"+key+"?/save", url.Values{"name": {"Ada L."}, "email": {"mallory@example.com"}}); loc == "" {
			t.Fatalf("save: %s", msg)
		}
		if u, _ := acc.UserByID(ctx, id); u.Name != "Ada L." || u.Email != "ada@example.com" {
			t.Errorf("after save: %+v", u)
		}
		// Sign out everywhere.
		if loc, msg, _ := admintest.Post(t, app, "/admin/users/"+key+"?/do", url.Values{"action": {"sign-out"}}); loc == "" {
			t.Fatalf("sign out: %s", msg)
		}
		var n int
		_ = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tern_sessions`).Scan(&n)
		if n != 0 {
			t.Errorf("%d sessions left", n)
		}
		// The audit log, read only, linked to its users.
		audit := admintest.Get(t, app, "/admin/audit?q=203.0.113")
		if audit.List.Total != 1 || audit.List.Rows[0].Values["action"] != "user.signed_in" || audit.List.Rows[0].Values["user_id"] != key {
			t.Errorf("audit = %+v", audit.List)
		}
		if _, _, status := admintest.Post(t, app, "/admin/audit/"+audit.List.Rows[0].Key+"?/delete", nil); status != 405 {
			t.Errorf("deleting an audit entry: %d", status)
		}
	})
}

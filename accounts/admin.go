package accounts

import (
	"context"
	"strconv"

	"github.com/Unfield/Tern/admin"
)

// userRow is an account as the admin screens show it. People change their
// own email and password, through the flows that check them, so only the
// name is edited here.
type userRow struct {
	ID            int64  `admin:"key"`
	Email         string `admin:"list,search,readonly"`
	Name          string `validate:"max=100" admin:"list,search"`
	EmailVerified *int64 `label:"Email verified" admin:"list,readonly,unixms"`
	CreatedAt     int64  `label:"Joined" admin:"list,readonly,unixms"`
}

// UsersAdmin is a resource for the admin package's screens: the accounts,
// searched by email and name, with "Sign out everywhere" on each.
//
//	admin.New(admin.Options{Authorize: onlyAdmins}, acc.UsersAdmin(), acc.AuditAdmin())
func (a *Accounts) UsersAdmin(opts ...admin.Option) admin.Res {
	table, err := admin.NewTable[userRow](a.db, "tern_users", "")
	if err != nil {
		panic(err)
	}
	return admin.Resource("users", table, append([]admin.Option{
		admin.Label("Users", "User"),
		admin.NoCreate(),
		admin.NoDelete(),
		admin.DangerAction("sign-out", "Sign out everywhere", func(ctx context.Context, key string) error {
			id, _ := strconv.ParseInt(key, 10, 64)
			return a.SignOutUser(ctx, id)
		}),
	}, opts...)...)
}

// auditRow is an audit log entry as the admin screens show it.
type auditRow struct {
	ID        int64  `admin:"key"`
	At        int64  `label:"When" admin:"list,unixms"`
	Action    string `admin:"list,search"`
	Target    string `admin:"list,search"`
	UserID    int64  `label:"User" admin:"list,ref=users"`
	TeamID    int64  `label:"Team" admin:"list"`
	IP        string `label:"IP address" admin:"search"`
	UserAgent string `label:"Browser"`
	Data      string `admin:"textarea"`
}

// AuditAdmin is a resource for the admin package's screens: the audit
// log of every account, newest first, searched by action, target and IP
// address. Its users link to UsersAdmin's.
func (a *Accounts) AuditAdmin(opts ...admin.Option) admin.Res {
	table, err := admin.NewTable[auditRow](a.db, "tern_audit", "")
	if err != nil {
		panic(err)
	}
	return admin.Resource("audit", table, append([]admin.Option{admin.Label("Audit log", "Audit entry"), admin.ReadOnly()}, opts...)...)
}

// SignOutUser ends every session of the user, on every device, such as
// for an account that was taken over.
func (a *Accounts) SignOutUser(ctx context.Context, userID int64) error {
	_, err := a.exec(ctx, `DELETE FROM tern_sessions WHERE user_id = ?`, userID)
	return err
}

package accounts

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Unfield/Tern/kit"
)

// The audit log records what happened to accounts and teams: sign-ins,
// failed ones among them, password and email changes, passkeys, members
// invited and removed. Users see their own (Activity), team admins their
// team's (TeamActivity), and support tools anyone's (AuditEntries). Apps
// add events of their own with Audit.
//
// The built-in actions:
//
//	user.signed_up user.signed_in user.sign_in_failed user.signed_out
//	user.password_changed user.password_reset user.email_verified
//	user.email_changed user.deleted user.two_step_on user.two_step_off
//	passkey.added passkey.removed identity.linked identity.unlinked
//	session.revoked session.others_signed_out
//	team.created team.renamed team.deleted
//	member.invited member.uninvited member.joined member.role
//	member.removed member.left

// Event is an event for Audit.
type Event struct {
	// Action names what happened, as "noun.verb": "project.deleted".
	Action string
	// TeamID is the team it happened in, 0 for none. Audit's caller
	// sets it; the built-in events do.
	TeamID int64
	// Target is what it happened to, in words: an email, a name, an ID.
	Target string
	// Data is anything else worth keeping: {"from": "admin", "to": "member"}.
	Data map[string]string
	// UserID is who did it. 0 is the signed-in user.
	UserID int64
}

// AuditEntry is an event in the audit log.
type AuditEntry struct {
	ID     int64             `json:"id"`
	At     time.Time         `json:"at"`
	UserID int64             `json:"userId"`
	Actor  string            `json:"actor"` // the user's email, "" for none
	TeamID int64             `json:"teamId"`
	Action string            `json:"action"`
	Target string            `json:"target"`
	IP     string            `json:"ip"`
	Device string            `json:"device"` // such as "Firefox on macOS"
	Data   map[string]string `json:"data,omitempty"`
}

// AuditFilter selects entries for AuditEntries. Its zero value is the
// latest entries of everyone.
type AuditFilter struct {
	UserID int64  // only this user's
	TeamID int64  // only this team's
	Action string // only actions starting with this: "member." or "user.signed_in"
	// Before pages back: only entries with smaller IDs than this.
	Before int64
	// Limit is how many, 50 by default and 500 at most.
	Limit int
}

func (a *Accounts) auditTables() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS tern_audit (
			id ` + a.d.AutoID() + `,
			at BIGINT NOT NULL,
			user_id BIGINT NOT NULL,
			team_id BIGINT NOT NULL,
			action TEXT NOT NULL,
			target TEXT NOT NULL,
			ip TEXT NOT NULL,
			user_agent TEXT NOT NULL,
			data TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS tern_audit_user ON tern_audit (user_id, id)`,
		`CREATE INDEX IF NOT EXISTS tern_audit_team ON tern_audit (team_id, id)`,
		`CREATE INDEX IF NOT EXISTS tern_audit_at ON tern_audit (at)`,
	}
}

// pruneEvery is how many events are recorded between deletions of those
// older than Options.AuditFor.
const pruneEvery = 500

var auditCount atomic.Int64

// Audit records an event. A failure to record it is logged, not
// returned: the event has happened either way.
func (a *Accounts) Audit(c *kit.Ctx, e Event) {
	if a.opts.AuditFor < 0 {
		return
	}
	if e.UserID == 0 {
		if u := Current(c); u != nil {
			e.UserID = u.ID
		}
	}
	data := "{}"
	if len(e.Data) > 0 {
		if b, err := json.Marshal(e.Data); err == nil {
			data = string(b)
		}
	}
	ctx := context.WithoutCancel(c.Request.Context())
	now := time.Now()
	_, err := a.exec(ctx, `INSERT INTO tern_audit (at, user_id, team_id, action, target, ip, user_agent, data) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		now.UnixMilli(), e.UserID, e.TeamID, e.Action, truncate(e.Target, 400), c.ClientIP(), truncate(c.Request.UserAgent(), 400), data)
	if err != nil {
		c.Logger().Error("accounts: recording an audit event", "action", e.Action, "err", err)
		return
	}
	if auditCount.Add(1)%pruneEvery == 0 {
		_, _ = a.exec(ctx, `DELETE FROM tern_audit WHERE at < ?`, now.Add(-a.auditFor()).UnixMilli())
	}
}

func (a *Accounts) auditFor() time.Duration {
	if a.opts.AuditFor == 0 {
		return 365 * 24 * time.Hour
	}
	return a.opts.AuditFor
}

// AuditEntries returns entries of the audit log, the newest first, with
// no checks of who asks: for support tools behind checks of their own.
func (a *Accounts) AuditEntries(ctx context.Context, f AuditFilter) ([]AuditEntry, error) {
	q := `SELECT l.id, l.at, l.user_id, COALESCE(u.email, ''), l.team_id, l.action, l.target, l.ip, l.user_agent, l.data FROM tern_audit l LEFT JOIN tern_users u ON u.id = l.user_id WHERE 1 = 1`
	var args []any
	if f.UserID != 0 {
		q += ` AND l.user_id = ?`
		args = append(args, f.UserID)
	}
	if f.TeamID != 0 {
		q += ` AND l.team_id = ?`
		args = append(args, f.TeamID)
	}
	if f.Action != "" {
		q += ` AND l.action LIKE ? ESCAPE '\'`
		args = append(args, likeEscape(f.Action)+"%")
	}
	if f.Before > 0 {
		q += ` AND l.id < ?`
		args = append(args, f.Before)
	}
	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	limit = min(limit, 500)
	q += ` ORDER BY l.id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := a.query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []AuditEntry{}
	for rows.Next() {
		var e AuditEntry
		var at int64
		var ua, data string
		if err := rows.Scan(&e.ID, &at, &e.UserID, &e.Actor, &e.TeamID, &e.Action, &e.Target, &e.IP, &ua, &data); err != nil {
			return nil, err
		}
		e.At = time.UnixMilli(at).UTC()
		e.Device = describeDevice(ua)
		if data != "{}" {
			_ = json.Unmarshal([]byte(data), &e.Data)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func likeEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// Activity is the signed-in user's own audit log, the newest first: what
// they did, and the failed sign-ins to their account. before pages back
// (an entry's ID; 0 for the latest).
func (a *Accounts) Activity(c *kit.Ctx, before int64) ([]AuditEntry, error) {
	u := Current(c)
	if u == nil {
		return nil, ErrSignedOut
	}
	return a.AuditEntries(c.Request.Context(), AuditFilter{UserID: u.ID, Before: before})
}

// TeamActivity is the current team's audit log, the newest first, for
// its owners and admins. before pages back as in Activity.
func (a *Accounts) TeamActivity(c *kit.Ctx, before int64) ([]AuditEntry, error) {
	m, err := a.manage(c)
	if err != nil {
		return nil, err
	}
	return a.AuditEntries(c.Request.Context(), AuditFilter{TeamID: m.Team.ID, Before: before})
}

package accounts

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/Unfield/Tern/kit"
	"github.com/Unfield/Tern/sqldb"
)

// Teams are groups of users who share an app's data: a company, a
// household, a class. A user can be in several and works in one at a
// time, the current team, which each session remembers.
//
// Each member has a role. Three are built in: an owner can do anything,
// deleting the team among it; an admin manages members and invitations;
// a member uses the app. Options.TeamRoles adds roles of the app's own,
// such as "viewer" or "billing", which manage nothing here; check them
// with package rbac and TeamRoles.

// The built-in roles.
const (
	RoleOwner  = "owner"
	RoleAdmin  = "admin"
	RoleMember = "member"
)

// The errors of teams. FormError turns them into form errors.
var (
	ErrNoTeam        = errors.New("accounts: the user isn't in a team")
	ErrNotMember     = errors.New("accounts: not a member of this team")
	ErrNotAllowed    = errors.New("accounts: your role in the team doesn't allow this")
	ErrLastOwner     = errors.New("accounts: a team needs an owner; make someone else owner first")
	ErrAlreadyMember = errors.New("accounts: they're already in the team")
	ErrWrongInvitee  = errors.New("accounts: the invitation is for another email address")
	ErrUnknownRole   = errors.New("accounts: unknown role")
)

// inviteFor is how long an invitation can be accepted.
const inviteFor = 7 * 24 * time.Hour

// Team is a team.
type Team struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
}

// Membership is a team of the user's, with their role in it.
type Membership struct {
	Team Team   `json:"team"`
	Role string `json:"role"`
}

// Manages reports whether the role manages the team's members.
func (m *Membership) Manages() bool {
	return m != nil && (m.Role == RoleOwner || m.Role == RoleAdmin)
}

// Member is a user in the current team.
type Member struct {
	UserID   int64     `json:"userId"`
	Email    string    `json:"email"`
	Name     string    `json:"name"`
	Role     string    `json:"role"`
	JoinedAt time.Time `json:"joinedAt"`
}

// Invitation is an invitation to a team, not accepted yet.
type Invitation struct {
	// ID identifies it for CancelInvitation. It isn't the link's token.
	ID        string    `json:"id"`
	TeamID    int64     `json:"teamId"`
	TeamName  string    `json:"teamName"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	InvitedBy string    `json:"invitedBy"` // the inviter's name
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt"`
}

func (a *Accounts) teamTables() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS tern_teams (
			id ` + a.d.AutoID() + `,
			name TEXT NOT NULL,
			created_at BIGINT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS tern_members (
			team_id BIGINT NOT NULL,
			user_id BIGINT NOT NULL,
			role TEXT NOT NULL,
			created_at BIGINT NOT NULL,
			PRIMARY KEY (team_id, user_id)
		)`,
		`CREATE INDEX IF NOT EXISTS tern_members_user ON tern_members (user_id)`,
		`CREATE TABLE IF NOT EXISTS tern_invitations (
			hash TEXT PRIMARY KEY,
			id TEXT NOT NULL UNIQUE,
			team_id BIGINT NOT NULL,
			email TEXT NOT NULL,
			role TEXT NOT NULL,
			invited_by BIGINT NOT NULL,
			created_at BIGINT NOT NULL,
			expires_at BIGINT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS tern_invitations_team ON tern_invitations (team_id)`,
	}
}

// addColumn adds a column to a table if it isn't there.
func (a *Accounts) addColumn(ctx context.Context, table, name, typ string) error {
	if a.d == sqldb.Postgres {
		_, err := a.db.ExecContext(ctx, `ALTER TABLE `+table+` ADD COLUMN IF NOT EXISTS `+name+` `+typ)
		return err
	}
	_, err := a.db.ExecContext(ctx, `ALTER TABLE `+table+` ADD COLUMN `+name+` `+typ)
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
		return nil
	}
	return err
}

// insertID runs an INSERT and returns the new row's id.
func (a *Accounts) insertID(ctx context.Context, q string, args ...any) (int64, error) {
	if a.d == sqldb.Postgres {
		var id int64
		err := a.queryRow(ctx, q+` RETURNING id`, args...).Scan(&id)
		return id, err
	}
	res, err := a.exec(ctx, q, args...)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// roleOK reports whether role is a role teams have.
func (a *Accounts) roleOK(role string) bool {
	switch role {
	case RoleOwner, RoleAdmin, RoleMember:
		return true
	}
	return slices.Contains(a.opts.TeamRoles, role)
}

// CurrentTeam is the team the signed-in user works in, with their role:
// the one they last switched to, else the one they joined first. It is
// nil when nobody is signed in or the user isn't in a team.
func CurrentTeam(c *kit.Ctx) *Membership {
	st := stateOf(c)
	if st.user == nil {
		return nil
	}
	st.teamMu.Lock()
	defer st.teamMu.Unlock()
	if !st.teamLoaded {
		m, err := st.acc.loadTeam(c.Request.Context(), st.user.ID, st.teamID)
		if err != nil {
			c.Logger().Error("accounts: loading the current team", "err", err)
			return nil // not cached: the next call tries again
		}
		st.team, st.teamLoaded = m, true
	}
	return st.team
}

// TeamRoles is the signed-in user's role in the current team, for
// rbac.New:
//
//	policy := rbac.New(roles, accounts.TeamRoles)
func TeamRoles(c *kit.Ctx) []string {
	if m := CurrentTeam(c); m != nil {
		return []string{m.Role}
	}
	return nil
}

// loadTeam returns the user's team preferred, or their first.
func (a *Accounts) loadTeam(ctx context.Context, userID, preferred int64) (*Membership, error) {
	var m Membership
	var created int64
	err := a.queryRow(ctx, `SELECT t.id, t.name, t.created_at, m.role FROM tern_members m JOIN tern_teams t ON t.id = m.team_id
		WHERE m.user_id = ? ORDER BY CASE WHEN t.id = ? THEN 0 ELSE 1 END, m.created_at, t.id LIMIT 1`, userID, preferred).
		Scan(&m.Team.ID, &m.Team.Name, &created, &m.Role)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	m.Team.CreatedAt = time.UnixMilli(created).UTC()
	return &m, nil
}

// RequireTeam is the current team; signed out, it redirects to sign in
// as Require does, and without a team, to Options.NewTeamPath.
func (a *Accounts) RequireTeam(c *kit.Ctx) (*Membership, error) {
	if _, err := a.Require(c); err != nil {
		return nil, err
	}
	if m := CurrentTeam(c); m != nil {
		return m, nil
	}
	return nil, kit.Redirect(http.StatusSeeOther, c.BasePath()+a.opts.NewTeamPath)
}

// CreateTeam makes a team with the signed-in user as its owner, and
// switches to it.
func (a *Accounts) CreateTeam(c *kit.Ctx, name string) (*Membership, error) {
	u := Current(c)
	if u == nil {
		return nil, ErrSignedOut
	}
	ctx := c.Request.Context()
	now := time.Now()
	name = strings.TrimSpace(name)
	id, err := a.insertID(ctx, `INSERT INTO tern_teams (name, created_at) VALUES (?, ?)`, name, now.UnixMilli())
	if err != nil {
		return nil, err
	}
	if _, err := a.exec(ctx, `INSERT INTO tern_members (team_id, user_id, role, created_at) VALUES (?, ?, ?, ?)`, id, u.ID, RoleOwner, now.UnixMilli()); err != nil {
		return nil, err
	}
	m := &Membership{Team: Team{ID: id, Name: name, CreatedAt: time.UnixMilli(now.UnixMilli()).UTC()}, Role: RoleOwner}
	a.Audit(c, Event{Action: "team.created", TeamID: id, Target: name})
	return m, a.setTeam(c, m)
}

// setTeam makes m the request's current team, and the session's.
func (a *Accounts) setTeam(c *kit.Ctx, m *Membership) error {
	st := stateOf(c)
	st.teamMu.Lock()
	st.team, st.teamLoaded = m, true
	st.teamID = 0
	if m != nil {
		st.teamID = m.Team.ID
	}
	st.teamMu.Unlock()
	if st.session == "" {
		return nil
	}
	_, err := a.exec(c.Request.Context(), `UPDATE tern_sessions SET team_id = ? WHERE id = ?`, st.teamID, st.session)
	return err
}

// Teams lists the signed-in user's teams, by name.
func (a *Accounts) Teams(c *kit.Ctx) ([]Membership, error) {
	u := Current(c)
	if u == nil {
		return nil, ErrSignedOut
	}
	rows, err := a.query(c.Request.Context(), `SELECT t.id, t.name, t.created_at, m.role FROM tern_members m JOIN tern_teams t ON t.id = m.team_id WHERE m.user_id = ? ORDER BY t.name, t.id`, u.ID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Membership
	for rows.Next() {
		var m Membership
		var created int64
		if err := rows.Scan(&m.Team.ID, &m.Team.Name, &created, &m.Role); err != nil {
			return nil, err
		}
		m.Team.CreatedAt = time.UnixMilli(created).UTC()
		out = append(out, m)
	}
	return out, rows.Err()
}

// SwitchTeam makes one of the user's teams the current one.
func (a *Accounts) SwitchTeam(c *kit.Ctx, teamID int64) error {
	u := Current(c)
	if u == nil {
		return ErrSignedOut
	}
	m, err := a.loadTeam(c.Request.Context(), u.ID, teamID)
	if err != nil {
		return err
	}
	if m == nil || m.Team.ID != teamID {
		return ErrNotMember
	}
	return a.setTeam(c, m)
}

// manage returns the current team, if the user's role manages it.
func (a *Accounts) manage(c *kit.Ctx) (*Membership, error) {
	if Current(c) == nil {
		return nil, ErrSignedOut
	}
	m := CurrentTeam(c)
	if m == nil {
		return nil, ErrNoTeam
	}
	if !m.Manages() {
		return nil, ErrNotAllowed
	}
	return m, nil
}

// RenameTeam renames the current team. Owners and admins may.
func (a *Accounts) RenameTeam(c *kit.Ctx, name string) error {
	m, err := a.manage(c)
	if err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	if _, err := a.exec(c.Request.Context(), `UPDATE tern_teams SET name = ? WHERE id = ?`, name, m.Team.ID); err != nil {
		return err
	}
	a.Audit(c, Event{Action: "team.renamed", TeamID: m.Team.ID, Target: name})
	m.Team.Name = name
	return nil
}

// DeleteTeam deletes the current team, its members and invitations, after
// Options.OnDeleteTeam has deleted the app's data of it. Only owners may.
func (a *Accounts) DeleteTeam(c *kit.Ctx) error {
	m := CurrentTeam(c)
	switch {
	case Current(c) == nil:
		return ErrSignedOut
	case m == nil:
		return ErrNoTeam
	case m.Role != RoleOwner:
		return ErrNotAllowed
	}
	if err := a.deleteTeam(c.Request.Context(), m.Team.ID); err != nil {
		return err
	}
	a.Audit(c, Event{Action: "team.deleted", TeamID: m.Team.ID, Target: m.Team.Name})
	next, err := a.loadTeam(c.Request.Context(), Current(c).ID, 0)
	if err != nil {
		return err
	}
	return a.setTeam(c, next)
}

func (a *Accounts) deleteTeam(ctx context.Context, id int64) error {
	if a.opts.OnDeleteTeam != nil {
		if err := a.opts.OnDeleteTeam(ctx, id); err != nil {
			return err
		}
	}
	for _, q := range []string{`DELETE FROM tern_invitations WHERE team_id = ?`, `DELETE FROM tern_members WHERE team_id = ?`, `UPDATE tern_sessions SET team_id = 0 WHERE team_id = ?`, `DELETE FROM tern_teams WHERE id = ?`} {
		if _, err := a.exec(ctx, q, id); err != nil {
			return err
		}
	}
	return nil
}

// Members lists the current team's members: owners first, then by name.
func (a *Accounts) Members(c *kit.Ctx) ([]Member, error) {
	m := CurrentTeam(c)
	if m == nil {
		if Current(c) == nil {
			return nil, ErrSignedOut
		}
		return nil, ErrNoTeam
	}
	return a.members(c.Request.Context(), m.Team.ID)
}

func (a *Accounts) members(ctx context.Context, teamID int64) ([]Member, error) {
	rows, err := a.query(ctx, `SELECT u.id, u.email, u.name, m.role, m.created_at FROM tern_members m JOIN tern_users u ON u.id = m.user_id
		WHERE m.team_id = ? ORDER BY CASE m.role WHEN 'owner' THEN 0 WHEN 'admin' THEN 1 ELSE 2 END, u.name, u.email`, teamID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Member
	for rows.Next() {
		var mb Member
		var joined int64
		if err := rows.Scan(&mb.UserID, &mb.Email, &mb.Name, &mb.Role, &joined); err != nil {
			return nil, err
		}
		mb.JoinedAt = time.UnixMilli(joined).UTC()
		out = append(out, mb)
	}
	return out, rows.Err()
}

// roleOf is a user's role in a team, "" if they aren't in it.
func (a *Accounts) roleOf(ctx context.Context, teamID, userID int64) (string, error) {
	var role string
	err := a.queryRow(ctx, `SELECT role FROM tern_members WHERE team_id = ? AND user_id = ?`, teamID, userID).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return role, err
}

func (a *Accounts) owners(ctx context.Context, teamID int64) (int, error) {
	var n int
	err := a.queryRow(ctx, `SELECT COUNT(*) FROM tern_members WHERE team_id = ? AND role = ?`, teamID, RoleOwner).Scan(&n)
	return n, err
}

// SetRole changes a member's role in the current team. Owners and admins
// may; making someone an owner, or changing an owner's role, only owners.
// The last owner stays one: ErrLastOwner.
func (a *Accounts) SetRole(c *kit.Ctx, userID int64, role string) error {
	m, err := a.manage(c)
	if err != nil {
		return err
	}
	if !a.roleOK(role) {
		return ErrUnknownRole
	}
	ctx := c.Request.Context()
	old, err := a.roleOf(ctx, m.Team.ID, userID)
	if err != nil {
		return err
	}
	if old == "" {
		return ErrNotMember
	}
	if old == role {
		return nil
	}
	if (old == RoleOwner || role == RoleOwner) && m.Role != RoleOwner {
		return ErrNotAllowed
	}
	if old == RoleOwner {
		if n, err := a.owners(ctx, m.Team.ID); err != nil {
			return err
		} else if n <= 1 {
			return ErrLastOwner
		}
	}
	if _, err := a.exec(ctx, `UPDATE tern_members SET role = ? WHERE team_id = ? AND user_id = ?`, role, m.Team.ID, userID); err != nil {
		return err
	}
	a.Audit(c, Event{Action: "member.role", TeamID: m.Team.ID, Target: a.emailOf(ctx, userID), Data: map[string]string{"from": old, "to": role}})
	if userID == Current(c).ID {
		m.Role = role
	}
	return nil
}

// RemoveMember takes a user out of the current team. Owners and admins
// may, and anyone may remove themselves (see LeaveTeam); removing an
// owner takes an owner. The last owner stays: ErrLastOwner.
func (a *Accounts) RemoveMember(c *kit.Ctx, userID int64) error {
	u := Current(c)
	if u == nil {
		return ErrSignedOut
	}
	m := CurrentTeam(c)
	if m == nil {
		return ErrNoTeam
	}
	self := userID == u.ID
	if !self && !m.Manages() {
		return ErrNotAllowed
	}
	ctx := c.Request.Context()
	role, err := a.roleOf(ctx, m.Team.ID, userID)
	if err != nil {
		return err
	}
	if role == "" {
		return ErrNotMember
	}
	if role == RoleOwner {
		if !self && m.Role != RoleOwner {
			return ErrNotAllowed
		}
		if n, err := a.owners(ctx, m.Team.ID); err != nil {
			return err
		} else if n <= 1 {
			return ErrLastOwner
		}
	}
	target := a.emailOf(ctx, userID)
	if _, err := a.exec(ctx, `DELETE FROM tern_members WHERE team_id = ? AND user_id = ?`, m.Team.ID, userID); err != nil {
		return err
	}
	if _, err := a.exec(ctx, `UPDATE tern_sessions SET team_id = 0 WHERE user_id = ? AND team_id = ?`, userID, m.Team.ID); err != nil {
		return err
	}
	action := "member.removed"
	if self {
		action = "member.left"
	}
	a.Audit(c, Event{Action: action, TeamID: m.Team.ID, Target: target, Data: map[string]string{"team": m.Team.Name}})
	if self {
		next, err := a.loadTeam(ctx, u.ID, 0)
		if err != nil {
			return err
		}
		return a.setTeam(c, next)
	}
	return nil
}

// LeaveTeam takes the signed-in user out of the current team.
func (a *Accounts) LeaveTeam(c *kit.Ctx) error {
	u := Current(c)
	if u == nil {
		return ErrSignedOut
	}
	return a.RemoveMember(c, u.ID)
}

func (a *Accounts) emailOf(ctx context.Context, userID int64) string {
	var email string
	_ = a.queryRow(ctx, `SELECT email FROM tern_users WHERE id = ?`, userID).Scan(&email)
	return email
}

// Invite emails an invitation to the current team, with a link to
// Options.InvitePath that AcceptInvitation takes. Owners and admins may
// invite; to invite an owner takes an owner. A new invitation for the
// same address replaces the one before.
func (a *Accounts) Invite(c *kit.Ctx, email, role string) (*Invitation, error) {
	m, err := a.manage(c)
	if err != nil {
		return nil, err
	}
	if a.opts.Send == nil {
		return nil, ErrNoEmail
	}
	if !a.roleOK(role) {
		return nil, ErrUnknownRole
	}
	if role == RoleOwner && m.Role != RoleOwner {
		return nil, ErrNotAllowed
	}
	u := Current(c)
	if err := a.limit(c, "invite", u.Email); err != nil {
		return nil, err
	}
	ctx := c.Request.Context()
	email = normalizeEmail(email)
	var n int
	if err := a.queryRow(ctx, `SELECT COUNT(*) FROM tern_members m JOIN tern_users u ON u.id = m.user_id WHERE m.team_id = ? AND u.email = ?`, m.Team.ID, email).Scan(&n); err != nil {
		return nil, err
	} else if n > 0 {
		return nil, ErrAlreadyMember
	}
	now := time.Now()
	if _, err := a.exec(ctx, `DELETE FROM tern_invitations WHERE (team_id = ? AND email = ?) OR expires_at < ?`, m.Team.ID, email, now.UnixMilli()); err != nil {
		return nil, err
	}
	token, hash := newToken()
	idToken, _ := newToken()
	inv := &Invitation{ID: idToken[:16], TeamID: m.Team.ID, TeamName: m.Team.Name, Email: email, Role: role, InvitedBy: u.Name,
		CreatedAt: time.UnixMilli(now.UnixMilli()).UTC(), ExpiresAt: time.UnixMilli(now.Add(inviteFor).UnixMilli()).UTC()}
	if _, err := a.exec(ctx, `INSERT INTO tern_invitations (hash, id, team_id, email, role, invited_by, created_at, expires_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		hash, inv.ID, inv.TeamID, email, role, u.ID, now.UnixMilli(), now.Add(inviteFor).UnixMilli()); err != nil {
		return nil, err
	}
	inviter := u.Name
	if inviter == "" {
		inviter = u.Email
	}
	if err := a.sendWith(c, "invite", &User{Email: email}, a.link(c, a.opts.InvitePath, token), map[string]any{"team": m.Team.Name, "inviter": inviter, "role": role}); err != nil {
		return nil, err
	}
	a.Audit(c, Event{Action: "member.invited", TeamID: m.Team.ID, Target: email, Data: map[string]string{"role": role}})
	return inv, nil
}

const invitationColumns = `i.id, i.team_id, t.name, i.email, i.role, COALESCE(u.name, ''), i.created_at, i.expires_at`

func scanInvitation(sc interface{ Scan(...any) error }) (*Invitation, error) {
	var inv Invitation
	var created, expires int64
	if err := sc.Scan(&inv.ID, &inv.TeamID, &inv.TeamName, &inv.Email, &inv.Role, &inv.InvitedBy, &created, &expires); err != nil {
		return nil, err
	}
	inv.CreatedAt, inv.ExpiresAt = time.UnixMilli(created).UTC(), time.UnixMilli(expires).UTC()
	return &inv, nil
}

// Invitations lists the current team's pending invitations, the newest
// first. Owners and admins may.
func (a *Accounts) Invitations(c *kit.Ctx) ([]Invitation, error) {
	m, err := a.manage(c)
	if err != nil {
		return nil, err
	}
	rows, err := a.query(c.Request.Context(), `SELECT `+invitationColumns+` FROM tern_invitations i JOIN tern_teams t ON t.id = i.team_id LEFT JOIN tern_users u ON u.id = i.invited_by
		WHERE i.team_id = ? AND i.expires_at > ? ORDER BY i.created_at DESC`, m.Team.ID, time.Now().UnixMilli())
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Invitation
	for rows.Next() {
		inv, err := scanInvitation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *inv)
	}
	return out, rows.Err()
}

// CancelInvitation withdraws one of the current team's invitations, so
// its link no longer works.
func (a *Accounts) CancelInvitation(c *kit.Ctx, id string) error {
	m, err := a.manage(c)
	if err != nil {
		return err
	}
	ctx := c.Request.Context()
	var email string
	err = a.queryRow(ctx, `SELECT email FROM tern_invitations WHERE id = ? AND team_id = ?`, id, m.Team.ID).Scan(&email)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrInvalidToken
	}
	if err != nil {
		return err
	}
	if _, err := a.exec(ctx, `DELETE FROM tern_invitations WHERE id = ? AND team_id = ?`, id, m.Team.ID); err != nil {
		return err
	}
	a.Audit(c, Event{Action: "member.uninvited", TeamID: m.Team.ID, Target: email})
	return nil
}

// InvitationFor returns the invitation of a link's token, for the page
// that offers to accept it; ErrInvalidToken if it doesn't exist or has
// expired.
func (a *Accounts) InvitationFor(ctx context.Context, token string) (*Invitation, error) {
	inv, err := scanInvitation(a.queryRow(ctx, `SELECT `+invitationColumns+` FROM tern_invitations i JOIN tern_teams t ON t.id = i.team_id LEFT JOIN tern_users u ON u.id = i.invited_by
		WHERE i.hash = ? AND i.expires_at > ?`, hashToken(token), time.Now().UnixMilli()))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInvalidToken
	}
	return inv, err
}

// AcceptInvitation adds the signed-in user to the invitation's team, and
// switches to it. The user's email must be the one invited
// (ErrWrongInvitee); opening the emailed link verifies it, too.
func (a *Accounts) AcceptInvitation(c *kit.Ctx, token string) (*Membership, error) {
	u := Current(c)
	if u == nil {
		return nil, ErrSignedOut
	}
	ctx := c.Request.Context()
	inv, err := a.InvitationFor(ctx, token)
	if err != nil {
		return nil, err
	}
	if normalizeEmail(u.Email) != inv.Email {
		return nil, ErrWrongInvitee
	}
	now := time.Now().UnixMilli()
	role, err := a.roleOf(ctx, inv.TeamID, u.ID)
	if err != nil {
		return nil, err
	}
	if role == "" {
		role = inv.Role
		if _, err := a.exec(ctx, `INSERT INTO tern_members (team_id, user_id, role, created_at) VALUES (?, ?, ?, ?)`, inv.TeamID, u.ID, role, now); err != nil {
			return nil, err
		}
	}
	if _, err := a.exec(ctx, `DELETE FROM tern_invitations WHERE hash = ?`, hashToken(token)); err != nil {
		return nil, err
	}
	if !u.EmailVerified {
		if _, err := a.exec(ctx, `UPDATE tern_users SET email_verified = ? WHERE id = ? AND email = ?`, now, u.ID, inv.Email); err != nil {
			return nil, err
		}
		u.EmailVerified = true
	}
	a.Audit(c, Event{Action: "member.joined", TeamID: inv.TeamID, Target: u.Email, Data: map[string]string{"role": role, "team": inv.TeamName}})
	m, err := a.loadTeam(ctx, u.ID, inv.TeamID)
	if err != nil {
		return nil, err
	}
	return m, a.setTeam(c, m)
}

// leaveAllTeams takes a user being deleted out of their teams: a team
// they were alone in is deleted; one they're the last owner of, with
// other members, stops the deletion with ErrLastOwner.
func (a *Accounts) leaveAllTeams(ctx context.Context, userID int64) error {
	rows, err := a.query(ctx, `SELECT team_id, role FROM tern_members WHERE user_id = ?`, userID)
	if err != nil {
		return err
	}
	type mem struct {
		team int64
		role string
	}
	var mine []mem
	for rows.Next() {
		var m mem
		if err := rows.Scan(&m.team, &m.role); err != nil {
			_ = rows.Close()
			return err
		}
		mine = append(mine, m)
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	var alone []int64
	for _, m := range mine {
		var others, otherOwners int
		if err := a.queryRow(ctx, `SELECT COUNT(*), COALESCE(SUM(CASE WHEN role = ? THEN 1 ELSE 0 END), 0) FROM tern_members WHERE team_id = ? AND user_id <> ?`, RoleOwner, m.team, userID).Scan(&others, &otherOwners); err != nil {
			return err
		}
		switch {
		case others == 0:
			alone = append(alone, m.team)
		case m.role == RoleOwner && otherOwners == 0:
			return ErrLastOwner
		}
	}
	for _, id := range alone {
		if err := a.deleteTeam(ctx, id); err != nil {
			return err
		}
	}
	_, err = a.exec(ctx, `DELETE FROM tern_members WHERE user_id = ?`, userID)
	return err
}

package accounts

import (
	"database/sql"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Unfield/Tern/kit"
)

// Sessions live in the database, so users can see the devices they're
// signed in on and sign any of them out. The cookie holds a random token;
// the database, its hash.

// Session is a device the user is signed in on.
type Session struct {
	// ID identifies it for Revoke. It isn't the cookie's token.
	ID        string    `json:"id"`
	Device    string    `json:"device"` // such as "Firefox on macOS"
	IP        string    `json:"ip"`
	CreatedAt time.Time `json:"createdAt"`
	LastSeen  time.Time `json:"lastSeen"`
	// Current is the session of this request.
	Current bool `json:"current"`
}

// lastSeenEvery is how often a session's last use is written: not on every
// request.
const lastSeenEvery = 5 * time.Minute

const shortSession = 24 * time.Hour

func (a *Accounts) startSession(c *kit.Ctx, u *User, remember bool) error {
	token, id := newToken()
	now := time.Now()
	length := shortSession
	if remember {
		length = a.opts.SessionLength
	}
	rem := 0
	if remember {
		rem = 1
	}
	_, err := a.exec(c.Request.Context(), `INSERT INTO tern_sessions (id, user_id, created_at, last_seen, expires_at, remember, user_agent, ip) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, u.ID, now.UnixMilli(), now.UnixMilli(), now.Add(length).UnixMilli(), rem, truncate(c.Request.UserAgent(), 400), c.ClientIP())
	if err != nil {
		return err
	}
	// A new session replaces the request's, as signing in does.
	if st := stateOf(c); st.session != "" {
		_, _ = a.exec(c.Request.Context(), `DELETE FROM tern_sessions WHERE id = ?`, st.session)
	}
	ck := &http.Cookie{Name: a.opts.Cookie, Value: token, HttpOnly: true, SameSite: http.SameSiteLaxMode}
	if remember {
		ck.MaxAge = int(length / time.Second)
	}
	c.SetCookie(ck)
	st := stateOf(c)
	st.user, st.session = u, id
	return nil
}

func (a *Accounts) loadSession(c *kit.Ctx, st *state, token string) {
	ctx := c.Request.Context()
	id := hashToken(token)
	var lastSeen, expires int64
	var remember int
	row := a.queryRow(ctx, `SELECT `+userColumns+`, s.last_seen, s.expires_at, s.remember FROM tern_sessions s JOIN tern_users u ON u.id = s.user_id WHERE s.id = ?`, id)
	var u User
	var hash string
	var verified sql.NullInt64
	var created, twoStep int64
	if err := row.Scan(&u.ID, &u.Email, &u.Name, &hash, &verified, &created, &twoStep, &lastSeen, &expires, &remember); err != nil {
		return // unknown or deleted: signed out
	}
	u.TwoStep = twoStep > 0
	now := time.Now()
	if now.UnixMilli() > expires {
		_, _ = a.exec(ctx, `DELETE FROM tern_sessions WHERE id = ?`, id)
		return
	}
	u.EmailVerified = verified.Valid
	u.CreatedAt = time.UnixMilli(created).UTC()
	u.HasPassword = hash != ""
	st.user, st.session = &u, id
	if now.Sub(time.UnixMilli(lastSeen)) > lastSeenEvery {
		length := shortSession
		if remember == 1 {
			length = a.opts.SessionLength
		}
		// Sessions in use stay: their expiry moves with their last use.
		_, _ = a.exec(ctx, `UPDATE tern_sessions SET last_seen = ?, expires_at = ?, ip = ? WHERE id = ?`, now.UnixMilli(), now.Add(length).UnixMilli(), c.ClientIP(), id)
	}
}

// Sessions lists the devices the signed-in user is signed in on, the
// current one first, then the most recently used.
func (a *Accounts) Sessions(c *kit.Ctx) ([]Session, error) {
	st := stateOf(c)
	if st.user == nil {
		return nil, ErrSignedOut
	}
	rows, err := a.query(c.Request.Context(), `SELECT id, user_agent, ip, created_at, last_seen FROM tern_sessions WHERE user_id = ? AND expires_at > ? ORDER BY last_seen DESC`, st.user.ID, time.Now().UnixMilli())
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []Session{}
	for rows.Next() {
		var s Session
		var ua string
		var created, seen int64
		if err := rows.Scan(&s.ID, &ua, &s.IP, &created, &seen); err != nil {
			return nil, err
		}
		s.Device = describeDevice(ua)
		s.CreatedAt, s.LastSeen = time.UnixMilli(created).UTC(), time.UnixMilli(seen).UTC()
		s.Current = s.ID == st.session
		// The id given out is the hash of the stored id: the stored one is
		// what the cookie's token hashes to, and stays private.
		s.ID = hashToken(s.ID)
		if s.Current {
			out = append([]Session{s}, out...)
		} else {
			out = append(out, s)
		}
	}
	return out, rows.Err()
}

// Revoke signs out one of the user's sessions, by its Session.ID.
func (a *Accounts) Revoke(c *kit.Ctx, sessionID string) error {
	st := stateOf(c)
	if st.user == nil {
		return ErrSignedOut
	}
	rows, err := a.query(c.Request.Context(), `SELECT id FROM tern_sessions WHERE user_id = ?`, st.user.ID)
	if err != nil {
		return err
	}
	var target string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return err
		}
		if hashToken(id) == sessionID {
			target = id
		}
	}
	_ = rows.Close()
	if target == "" {
		return nil // already gone
	}
	if target == st.session {
		return a.SignOut(c)
	}
	_, err = a.exec(c.Request.Context(), `DELETE FROM tern_sessions WHERE id = ? AND user_id = ?`, target, st.user.ID)
	return err
}

// SignOutOthers signs out every session of the user but this one.
func (a *Accounts) SignOutOthers(c *kit.Ctx) error {
	st := stateOf(c)
	if st.user == nil {
		return ErrSignedOut
	}
	_, err := a.exec(c.Request.Context(), `DELETE FROM tern_sessions WHERE user_id = ? AND id <> ?`, st.user.ID, st.session)
	return err
}

// describeDevice names a browser and system from a User-Agent header.
func describeDevice(ua string) string {
	browser := ""
	switch {
	case strings.Contains(ua, "Edg/"):
		browser = "Edge"
	case strings.Contains(ua, "OPR/") || strings.Contains(ua, "Opera"):
		browser = "Opera"
	case strings.Contains(ua, "Firefox/") || strings.Contains(ua, "FxiOS/"):
		browser = "Firefox"
	case strings.Contains(ua, "Chrome/") || strings.Contains(ua, "CriOS/"):
		browser = "Chrome"
	case strings.Contains(ua, "Safari/"):
		browser = "Safari"
	}
	system := ""
	switch {
	case strings.Contains(ua, "iPhone") || strings.Contains(ua, "iPad"):
		system = "iOS"
	case strings.Contains(ua, "Android"):
		system = "Android"
	case strings.Contains(ua, "Mac OS X") || strings.Contains(ua, "Macintosh"):
		system = "macOS"
	case strings.Contains(ua, "Windows"):
		system = "Windows"
	case strings.Contains(ua, "CrOS"):
		system = "ChromeOS"
	case strings.Contains(ua, "Linux"):
		system = "Linux"
	}
	switch {
	case browser != "" && system != "":
		return browser + " on " + system
	case browser != "":
		return browser
	case system != "":
		return system
	}
	return "Unknown device"
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func urlQueryEscape(s string) string { return url.QueryEscape(s) }

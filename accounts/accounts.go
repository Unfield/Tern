// Package accounts signs users up and in, with passwords or passkeys, and
// keeps their accounts: email verification, password resets, and the
// devices they're signed in on, any of which they can sign out.
//
// It stores users and sessions in your database (SQLite or PostgreSQL),
// in tables whose names start with tern_. Install it as a plugin, after
// creating its tables:
//
//	acc, err := accounts.New(db, accounts.Options{Send: accounts.SendWith(mailer, queue)})
//	err = acc.Migrate(ctx)
//	app, err := ternApp(kit.WithPlugins(acc))
//
// In loads and actions, accounts.Current(ctx) is the signed-in user, and
// acc.SignIn, acc.SignUp and the rest change who that is.
package accounts

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Unfield/Tern/kit"
	"github.com/Unfield/Tern/sqldb"
)

// Options configure accounts.
type Options struct {
	// Send delivers the verification and password reset emails. SendWith
	// sends them with a mail.Mailer through a jobs queue. Without it,
	// those features fail with ErrNoEmail.
	Send func(ctx context.Context, e Email) error
	// BaseURL is the app's address, such as "https://example.com", for
	// links in emails. The default is the request's own.
	BaseURL string
	// ResetPath and VerifyPath are the pages the emails link to, with the
	// token in ?token=. The defaults are /reset-password and /verify-email.
	ResetPath, VerifyPath string
	// SignInPath is where Require sends signed-out users. The default is
	// /login.
	SignInPath string
	// Cookie names the session cookie. The default is "tern_session".
	Cookie string
	// SessionLength is how long a "keep me signed in" session lasts
	// without use. The default is 30 days; other sessions last a day.
	SessionLength time.Duration
	// PasskeyPath is where the passkey endpoints are mounted, for the
	// browser's passkeys helpers. The default is /_tern/passkeys.
	PasskeyPath string
	// RPName is the app's name the browser shows when creating a passkey.
	// The default is the host name.
	RPName string
	// TwoStepPath is the page that asks for the two-step code after a
	// password sign-in, for apps to redirect to on ErrTwoStepRequired. The
	// default is /login/code.
	TwoStepPath string
	// Issuer names the app in authenticator apps. The default is RPName,
	// then the host name.
	Issuer string
	// NoRateLimits turns off the built-in limits on attempts (see
	// ErrTooManyAttempts), for tests or a limiter of your own.
	NoRateLimits bool

	// NotifyNewDevices emails users when their account is signed in to
	// from a device it hadn't been signed in on before. OnNewDevice is
	// called then, too. Both need plugins/signals installed, which gives
	// each browser a device ID; see NewDevice.
	NotifyNewDevices bool
	OnNewDevice      func(c *kit.Ctx, d NewDevice)
}

// Accounts is the accounts plugin. Create it with New.
type Accounts struct {
	db     *sql.DB
	d      sqldb.Dialect
	opts   Options
	limits *limiter
}

// User is an account.
type User struct {
	ID            int64     `json:"id"`
	Email         string    `json:"email"`
	Name          string    `json:"name"`
	EmailVerified bool      `json:"emailVerified"`
	CreatedAt     time.Time `json:"createdAt"`
	// HasPassword is false for accounts made with a passkey only.
	HasPassword bool `json:"hasPassword"`
	// TwoStep is whether signing in with a password also asks for a code
	// from an authenticator app.
	TwoStep bool `json:"twoStep"`
}

// The errors that tell users what went wrong. Show them as form errors.
var (
	ErrInvalidCredentials = errors.New("accounts: wrong email or password")
	ErrWrongPassword      = errors.New("accounts: the current password is wrong")
	ErrEmailTaken         = errors.New("accounts: an account with this email already exists")
	ErrInvalidToken       = errors.New("accounts: the link is invalid or has expired")
	ErrSignedOut          = errors.New("accounts: not signed in")
	ErrNoEmail            = errors.New("accounts: Options.Send is not set, so no email can be sent")
	ErrPasskey            = errors.New("accounts: the passkey didn't work")
	// ErrTwoStepRequired means the password was right and the account asks
	// for a code too: send the user to a page that calls CompleteSignIn.
	ErrTwoStepRequired = errors.New("accounts: enter the code from your authenticator app")
	ErrInvalidCode     = errors.New("accounts: the code is wrong or has expired")
)

// New returns the accounts plugin.
func New(db *sql.DB, opts Options) (*Accounts, error) {
	d, err := sqldb.Detect(db)
	if err != nil {
		return nil, fmt.Errorf("accounts: %w", err)
	}
	if opts.ResetPath == "" {
		opts.ResetPath = "/reset-password"
	}
	if opts.VerifyPath == "" {
		opts.VerifyPath = "/verify-email"
	}
	if opts.SignInPath == "" {
		opts.SignInPath = "/login"
	}
	if opts.Cookie == "" {
		opts.Cookie = "tern_session"
	}
	if opts.SessionLength <= 0 {
		opts.SessionLength = 30 * 24 * time.Hour
	}
	if opts.TwoStepPath == "" {
		opts.TwoStepPath = "/login/code"
	}
	if opts.Issuer == "" {
		opts.Issuer = opts.RPName
	}
	if opts.PasskeyPath == "" {
		opts.PasskeyPath = "/_tern/passkeys"
	}
	opts.BaseURL = strings.TrimSuffix(opts.BaseURL, "/")
	return &Accounts{db: db, d: d, opts: opts, limits: newLimiter()}, nil
}

// Migrate creates the tables, if they don't exist yet.
func (a *Accounts) Migrate(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS tern_users (
			id ` + a.d.AutoID() + `,
			email TEXT NOT NULL UNIQUE,
			name TEXT NOT NULL,
			password TEXT NOT NULL,
			email_verified BIGINT,
			created_at BIGINT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS tern_sessions (
			id TEXT PRIMARY KEY,
			user_id BIGINT NOT NULL,
			created_at BIGINT NOT NULL,
			last_seen BIGINT NOT NULL,
			expires_at BIGINT NOT NULL,
			remember INTEGER NOT NULL,
			user_agent TEXT NOT NULL,
			ip TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS tern_sessions_user ON tern_sessions (user_id)`,
		`CREATE TABLE IF NOT EXISTS tern_tokens (
			hash TEXT PRIMARY KEY,
			purpose TEXT NOT NULL,
			user_id BIGINT NOT NULL,
			data TEXT NOT NULL,
			created_at BIGINT NOT NULL,
			expires_at BIGINT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS tern_passkeys (
			id TEXT PRIMARY KEY,
			user_id BIGINT NOT NULL,
			public_key TEXT NOT NULL,
			alg INTEGER NOT NULL,
			sign_count BIGINT NOT NULL,
			name TEXT NOT NULL,
			created_at BIGINT NOT NULL,
			last_used BIGINT
		)`,
		`CREATE INDEX IF NOT EXISTS tern_passkeys_user ON tern_passkeys (user_id)`,
		`CREATE TABLE IF NOT EXISTS tern_two_step (
			user_id BIGINT PRIMARY KEY,
			secret TEXT NOT NULL,
			confirmed_at BIGINT,
			last_step BIGINT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS tern_recovery_codes (
			hash TEXT PRIMARY KEY,
			user_id BIGINT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS tern_recovery_codes_user ON tern_recovery_codes (user_id)`,
		`CREATE TABLE IF NOT EXISTS tern_identities (
			provider TEXT NOT NULL,
			subject TEXT NOT NULL,
			user_id BIGINT NOT NULL,
			email TEXT NOT NULL,
			created_at BIGINT NOT NULL,
			PRIMARY KEY (provider, subject)
		)`,
		`CREATE INDEX IF NOT EXISTS tern_identities_user ON tern_identities (user_id)`,
		`CREATE TABLE IF NOT EXISTS tern_known_devices (
			user_id BIGINT NOT NULL,
			device TEXT NOT NULL,
			first_seen BIGINT NOT NULL,
			PRIMARY KEY (user_id, device)
		)`,
	}
	for _, s := range stmts {
		if _, err := a.db.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("accounts: creating tables: %w", err)
		}
	}
	return nil
}

func (a *Accounts) exec(ctx context.Context, q string, args ...any) (sql.Result, error) {
	return a.db.ExecContext(ctx, a.d.Rebind(q), args...)
}

func (a *Accounts) query(ctx context.Context, q string, args ...any) (*sql.Rows, error) {
	return a.db.QueryContext(ctx, a.d.Rebind(q), args...)
}

func (a *Accounts) queryRow(ctx context.Context, q string, args ...any) *sql.Row {
	return a.db.QueryRowContext(ctx, a.d.Rebind(q), args...)
}

// Name implements kit.Plugin.
func (*Accounts) Name() string { return "accounts" }

const localsKey = "tern/accounts"

// state is the request's account state.
type state struct {
	user    *User
	session string // its id, the hash of the cookie's token
}

// Setup implements kit.Plugin: each request gets its user from the
// session cookie.
func (a *Accounts) Setup(s *kit.Setup) error {
	s.Hook(func(c *kit.Ctx) error {
		st := &state{}
		c.Locals[localsKey] = st
		token, ok := c.Cookie(a.opts.Cookie)
		if !ok || token == "" {
			return nil
		}
		a.loadSession(c, st, token)
		return nil
	})
	a.passkeyEndpoints(s)
	return nil
}

func stateOf(c *kit.Ctx) *state {
	st, ok := c.Locals[localsKey].(*state)
	if !ok {
		panic("accounts: add the plugin with kit.WithPlugins")
	}
	return st
}

// Current is the signed-in user, or nil.
func Current(c *kit.Ctx) *User { return stateOf(c).user }

// Require is the signed-in user; signed out, it returns a redirect to the
// sign-in page that comes back here, to return from a load or action.
func (a *Accounts) Require(c *kit.Ctx) (*User, error) {
	if u := Current(c); u != nil {
		return u, nil
	}
	// The page's address: a client navigation asks for its data at
	// <page>/__data.json.
	u := *c.Request.URL
	u.Path = strings.TrimSuffix(u.Path, "/__data.json")
	u.RawPath = ""
	if u.Path == "" {
		u.Path = "/"
	}
	next := u.RequestURI()
	return nil, kit.Redirect(http.StatusSeeOther, c.BasePath()+a.opts.SignInPath+"?next="+urlQueryEscape(next))
}

const userColumns = `u.id, u.email, u.name, u.password, u.email_verified, u.created_at, (SELECT COUNT(*) FROM tern_two_step t WHERE t.user_id = u.id AND t.confirmed_at IS NOT NULL)`

func scanUser(sc interface{ Scan(...any) error }) (*User, string, error) {
	var u User
	var hash string
	var verified sql.NullInt64
	var created, twoStep int64
	if err := sc.Scan(&u.ID, &u.Email, &u.Name, &hash, &verified, &created, &twoStep); err != nil {
		return nil, "", err
	}
	u.TwoStep = twoStep > 0
	u.EmailVerified = verified.Valid
	u.CreatedAt = time.UnixMilli(created).UTC()
	u.HasPassword = hash != ""
	return &u, hash, nil
}

// UserByID returns a user, or nil if there's none.
func (a *Accounts) UserByID(ctx context.Context, id int64) (*User, error) {
	u, _, err := scanUser(a.queryRow(ctx, `SELECT `+userColumns+` FROM tern_users u WHERE u.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return u, err
}

// UserByEmail returns a user, or nil if there's none.
func (a *Accounts) UserByEmail(ctx context.Context, email string) (*User, error) {
	u, _, err := scanUser(a.queryRow(ctx, `SELECT `+userColumns+` FROM tern_users u WHERE u.email = ?`, normalizeEmail(email)))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return u, err
}

func normalizeEmail(email string) string { return strings.ToLower(strings.TrimSpace(email)) }

// SignUp creates an account with a password and signs it in. password
// may be "" for an account that signs in with passkeys only.
func (a *Accounts) SignUp(c *kit.Ctx, email, name, password string) (*User, error) {
	if err := a.limit(c, "signup", ""); err != nil {
		return nil, err
	}
	u, err := a.createUser(c, email, name, password)
	if err != nil {
		return nil, err
	}
	if err := a.startSession(c, u, false); err != nil {
		return nil, err
	}
	return u, nil
}

// createUser adds a user, without signing them in.
func (a *Accounts) createUser(c *kit.Ctx, email, name, password string) (*User, error) {
	email = normalizeEmail(email)
	hash := ""
	if password != "" {
		var err error
		if hash, err = HashPassword(password); err != nil {
			return nil, err
		}
	}
	if existing, err := a.UserByEmail(c.Request.Context(), email); err != nil {
		return nil, err
	} else if existing != nil {
		return nil, ErrEmailTaken
	}
	now := time.Now().UnixMilli()
	var id int64
	if a.d == sqldb.Postgres {
		err := a.queryRow(c.Request.Context(), `INSERT INTO tern_users (email, name, password, created_at) VALUES (?, ?, ?, ?) RETURNING id`, email, strings.TrimSpace(name), hash, now).Scan(&id)
		if err != nil {
			return nil, a.uniqueErr(err)
		}
	} else {
		res, err := a.exec(c.Request.Context(), `INSERT INTO tern_users (email, name, password, created_at) VALUES (?, ?, ?, ?)`, email, strings.TrimSpace(name), hash, now)
		if err != nil {
			return nil, a.uniqueErr(err)
		}
		if id, err = res.LastInsertId(); err != nil {
			return nil, err
		}
	}
	return &User{ID: id, Email: email, Name: strings.TrimSpace(name), CreatedAt: time.UnixMilli(now).UTC(), HasPassword: hash != ""}, nil
}

func (a *Accounts) uniqueErr(err error) error {
	if s := strings.ToLower(err.Error()); strings.Contains(s, "unique") || strings.Contains(s, "duplicate") {
		return ErrEmailTaken
	}
	return err
}

// SignIn checks a password and signs the user in. remember keeps the
// session for Options.SessionLength instead of a day.
func (a *Accounts) SignIn(c *kit.Ctx, email, password string, remember bool) (*User, error) {
	ctx := c.Request.Context()
	if err := a.limit(c, "signin", normalizeEmail(email)); err != nil {
		return nil, err
	}
	u, hash, err := scanUser(a.queryRow(ctx, `SELECT `+userColumns+` FROM tern_users u WHERE u.email = ?`, normalizeEmail(email)))
	if errors.Is(err, sql.ErrNoRows) || (err == nil && hash == "") {
		CheckPassword(dummyHash, password) // as slow as a real check
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}
	ok, rehash := CheckPassword(hash, password)
	if !ok {
		return nil, ErrInvalidCredentials
	}
	a.succeeded("signin", u.Email)
	if rehash {
		if h, err := HashPassword(password); err == nil {
			_, _ = a.exec(ctx, `UPDATE tern_users SET password = ? WHERE id = ?`, h, u.ID)
		}
	}
	if u.TwoStep {
		return nil, a.awaitCode(c, u, remember)
	}
	if err := a.startSession(c, u, remember); err != nil {
		return nil, err
	}
	return u, nil
}

// SignOut ends the request's session.
func (a *Accounts) SignOut(c *kit.Ctx) error {
	st := stateOf(c)
	if st.session != "" {
		if _, err := a.exec(c.Request.Context(), `DELETE FROM tern_sessions WHERE id = ?`, st.session); err != nil {
			return err
		}
	}
	st.user, st.session = nil, ""
	c.SetCookie(&http.Cookie{Name: a.opts.Cookie, Value: "", MaxAge: -1, HttpOnly: true})
	return nil
}

// ChangePassword sets a new password after checking the current one, and
// signs out the user's other devices. An account without a password gets
// one with current "".
func (a *Accounts) ChangePassword(c *kit.Ctx, current, password string) error {
	st := stateOf(c)
	if st.user == nil {
		return ErrSignedOut
	}
	ctx := c.Request.Context()
	_, hash, err := scanUser(a.queryRow(ctx, `SELECT `+userColumns+` FROM tern_users u WHERE u.id = ?`, st.user.ID))
	if err != nil {
		return err
	}
	if hash != "" {
		if err := a.limit(c, "signin", st.user.Email); err != nil {
			return err
		}
		if ok, _ := CheckPassword(hash, current); !ok {
			return ErrWrongPassword
		}
	}
	h, err := HashPassword(password)
	if err != nil {
		return err
	}
	if _, err := a.exec(ctx, `UPDATE tern_users SET password = ? WHERE id = ?`, h, st.user.ID); err != nil {
		return err
	}
	st.user.HasPassword = true
	return a.SignOutOthers(c)
}

// SetName changes the signed-in user's name.
func (a *Accounts) SetName(c *kit.Ctx, name string) error {
	st := stateOf(c)
	if st.user == nil {
		return ErrSignedOut
	}
	if _, err := a.exec(c.Request.Context(), `UPDATE tern_users SET name = ? WHERE id = ?`, strings.TrimSpace(name), st.user.ID); err != nil {
		return err
	}
	st.user.Name = strings.TrimSpace(name)
	return nil
}

// DeleteAccount deletes the signed-in user, their sessions and passkeys.
func (a *Accounts) DeleteAccount(c *kit.Ctx) error {
	st := stateOf(c)
	if st.user == nil {
		return ErrSignedOut
	}
	ctx := c.Request.Context()
	for _, q := range []string{`DELETE FROM tern_identities WHERE user_id = ?`, `DELETE FROM tern_two_step WHERE user_id = ?`, `DELETE FROM tern_recovery_codes WHERE user_id = ?`, `DELETE FROM tern_sessions WHERE user_id = ?`, `DELETE FROM tern_tokens WHERE user_id = ?`, `DELETE FROM tern_passkeys WHERE user_id = ?`, `DELETE FROM tern_users WHERE id = ?`} {
		if _, err := a.exec(ctx, q, st.user.ID); err != nil {
			return err
		}
	}
	return a.SignOut(c)
}

// ActAs makes u the signed-in user for the rest of c's request, without a
// session: Current returns u and Require lets it through. For tests (see
// kit/kittest), and for support tools that act for a user once their own
// checks have passed. c must have run the plugin's hook, as a Ctx from
// kit.App.NewCtx does.
func (a *Accounts) ActAs(c *kit.Ctx, u *User) {
	stateOf(c).user = u
}

package accounts

import (
	"database/sql"
	"errors"
	"time"

	"github.com/Unfield/Tern/kit"
)

// Sign-in with other providers: GitHub, Google, any OpenID Connect
// provider through plugins/oauth, whose oauth.SignInToAccounts calls
// SignInWith. An account can have a password, passkeys and any number of
// providers; each way signs in to the same user.

// Identity is a user as another provider knows them.
type Identity struct {
	// Provider is its name, such as "github", and Subject its id for the
	// user, which never changes.
	Provider, Subject string
	Email             string
	// EmailVerified is whether the provider checked the address. Only then
	// does a first sign-in join an existing account with that address.
	EmailVerified bool
	Name          string
}

// LinkedIdentity is a provider linked to the signed-in user.
type LinkedIdentity struct {
	Provider  string    `json:"provider"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"createdAt"`
}

var (
	// ErrIdentityTaken is a provider account already linked to another user.
	ErrIdentityTaken = errors.New("accounts: this account is already linked to another user")
	// ErrIdentityNoEmail is a first sign-in from a provider that gave no
	// email address, which an account needs.
	ErrIdentityNoEmail = errors.New("accounts: the provider didn't share an email address")
	// ErrEmailUnverified is a first sign-in with the address of an existing
	// account, which the provider didn't verify: sign in another way and
	// link the provider from there.
	ErrEmailUnverified = errors.New("accounts: an account with this email exists; sign in to it to link this provider")
	// ErrLastSignIn is unlinking the only way an account can sign in.
	ErrLastSignIn = errors.New("accounts: add a password or a passkey before removing the last way to sign in")
)

func (a *Accounts) identityUser(c *kit.Ctx, id Identity) (int64, error) {
	var userID int64
	err := a.queryRow(c.Request.Context(), `SELECT user_id FROM tern_identities WHERE provider = ? AND subject = ?`, id.Provider, id.Subject).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return userID, err
}

func (a *Accounts) linkIdentity(c *kit.Ctx, userID int64, id Identity) error {
	_, err := a.exec(c.Request.Context(), `INSERT INTO tern_identities (provider, subject, user_id, email, created_at) VALUES (?, ?, ?, ?, ?)`,
		id.Provider, id.Subject, userID, normalizeEmail(id.Email), time.Now().UnixMilli())
	return err
}

// SignInWith signs in with another provider's identity:
//
//   - signed in already, it links the identity to the current user;
//   - an identity linked before signs in its user;
//   - otherwise it joins the account with the same verified email, or
//     creates an account.
//
// With two-step sign-in on, it returns ErrTwoStepRequired as SignIn does:
// the provider counts as the first step.
func (a *Accounts) SignInWith(c *kit.Ctx, id Identity) (*User, error) {
	if id.Provider == "" || id.Subject == "" {
		return nil, errors.New("accounts: an identity needs a provider and a subject")
	}
	ctx := c.Request.Context()
	linked, err := a.identityUser(c, id)
	if err != nil {
		return nil, err
	}
	if cur := Current(c); cur != nil {
		switch {
		case linked == cur.ID:
			return cur, nil
		case linked != 0:
			return nil, ErrIdentityTaken
		}
		return cur, a.linkIdentity(c, cur.ID, id)
	}
	if err := a.limit(c, "signin", ""); err != nil {
		return nil, err
	}
	var u *User
	switch {
	case linked != 0:
		if u, err = a.UserByID(ctx, linked); err != nil || u == nil {
			return nil, errors.Join(ErrInvalidCredentials, err)
		}
	case normalizeEmail(id.Email) == "":
		return nil, ErrIdentityNoEmail
	default:
		existing, err := a.UserByEmail(ctx, id.Email)
		if err != nil {
			return nil, err
		}
		switch {
		case existing != nil && !id.EmailVerified:
			return nil, ErrEmailUnverified
		case existing != nil:
			u = existing
		default:
			if u, err = a.createUser(c, id.Email, id.Name, ""); err != nil {
				return nil, err
			}
		}
		if id.EmailVerified && !u.EmailVerified && normalizeEmail(id.Email) == u.Email {
			_, _ = a.exec(ctx, `UPDATE tern_users SET email_verified = ? WHERE id = ?`, time.Now().UnixMilli(), u.ID)
			u.EmailVerified = true
		}
		if err := a.linkIdentity(c, u.ID, id); err != nil {
			return nil, err
		}
	}
	if u.TwoStep {
		return nil, a.awaitCode(c, u, true)
	}
	if err := a.startSession(c, u, true); err != nil {
		return nil, err
	}
	return u, nil
}

// Identities lists the providers linked to the signed-in user.
func (a *Accounts) Identities(c *kit.Ctx) ([]LinkedIdentity, error) {
	u := Current(c)
	if u == nil {
		return nil, ErrSignedOut
	}
	rows, err := a.query(c.Request.Context(), `SELECT provider, email, created_at FROM tern_identities WHERE user_id = ? ORDER BY created_at`, u.ID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []LinkedIdentity{}
	for rows.Next() {
		var l LinkedIdentity
		var created int64
		if err := rows.Scan(&l.Provider, &l.Email, &created); err != nil {
			return nil, err
		}
		l.CreatedAt = time.UnixMilli(created).UTC()
		out = append(out, l)
	}
	return out, rows.Err()
}

// Unlink removes a provider from the signed-in user, unless it's the only
// way left to sign in.
func (a *Accounts) Unlink(c *kit.Ctx, provider string) error {
	u := Current(c)
	if u == nil {
		return ErrSignedOut
	}
	ctx := c.Request.Context()
	var others, keys int
	if err := a.queryRow(ctx, `SELECT COUNT(*) FROM tern_identities WHERE user_id = ? AND provider <> ?`, u.ID, provider).Scan(&others); err != nil {
		return err
	}
	if err := a.queryRow(ctx, `SELECT COUNT(*) FROM tern_passkeys WHERE user_id = ?`, u.ID).Scan(&keys); err != nil {
		return err
	}
	if !u.HasPassword && others == 0 && keys == 0 {
		return ErrLastSignIn
	}
	_, err := a.exec(ctx, `DELETE FROM tern_identities WHERE user_id = ? AND provider = ?`, u.ID, provider)
	return err
}

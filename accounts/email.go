package accounts

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/Unfield/Tern/jobs"
	"github.com/Unfield/Tern/kit"
	"github.com/Unfield/Tern/mail"
)

// Email is a verification or password reset email for Options.Send. Its
// texts are Tern's own (tern.accounts.*), translated like any other into
// the request's locale.
type Email struct {
	// Kind is "verify", "reset", "change" (confirm a new address),
	// "changed" (to the old one) or "new_device".
	Kind    string `json:"kind"`
	To      string `json:"to"`
	Name    string `json:"name"`
	Link    string `json:"link"`
	Subject string `json:"subject"`
	Text    string `json:"text"`
	HTML    string `json:"html"`
}

// SendWith sends the emails with mailer, in the background through q, so
// they're retried and requests don't wait for them.
func SendWith(mailer *mail.Mailer, q *jobs.Queue) func(ctx context.Context, e Email) error {
	job := mail.Job(q, mailer)
	return func(ctx context.Context, e Email) error {
		_, err := job.Enqueue(ctx, mail.Message{To: []string{e.To}, Subject: e.Subject, Text: e.Text, HTML: e.HTML})
		return err
	}
}

const (
	verifyFor = 48 * time.Hour
	resetFor  = time.Hour
	// resendAfter stops a page from sending one email after another.
	resendAfter = 2 * time.Minute
)

func (a *Accounts) link(c *kit.Ctx, path, token string) string {
	base := a.opts.BaseURL
	if base == "" {
		scheme := "https"
		if c.Request.TLS == nil && !strings.EqualFold(c.Request.Header.Get("X-Forwarded-Proto"), "https") {
			if h := c.Request.Host; strings.HasPrefix(h, "localhost") || strings.HasPrefix(h, "127.0.0.1") {
				scheme = "http"
			}
		}
		base = scheme + "://" + c.Request.Host
	}
	return base + c.BasePath() + path + "?token=" + token
}

// issue makes a one-time token, unless one was made moments ago.
func (a *Accounts) issue(ctx context.Context, purpose string, userID int64, data string, valid time.Duration) (string, error) {
	now := time.Now()
	var last int64
	err := a.queryRow(ctx, `SELECT created_at FROM tern_tokens WHERE purpose = ? AND user_id = ? AND expires_at > ? ORDER BY created_at DESC LIMIT 1`, purpose, userID, now.UnixMilli()).Scan(&last)
	if err == nil && now.Sub(time.UnixMilli(last)) < resendAfter {
		return "", nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	token, hash := newToken()
	if _, err := a.exec(ctx, `INSERT INTO tern_tokens (hash, purpose, user_id, data, created_at, expires_at) VALUES (?, ?, ?, ?, ?, ?)`, hash, purpose, userID, data, now.UnixMilli(), now.Add(valid).UnixMilli()); err != nil {
		return "", err
	}
	return token, nil
}

// redeem uses up a token, returning its user and data.
func (a *Accounts) redeem(ctx context.Context, purpose, token string) (int64, string, error) {
	hash := hashToken(token)
	var userID, expires int64
	var data string
	err := a.queryRow(ctx, `SELECT user_id, data, expires_at FROM tern_tokens WHERE hash = ? AND purpose = ?`, hash, purpose).Scan(&userID, &data, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", ErrInvalidToken
	}
	if err != nil {
		return 0, "", err
	}
	if _, err := a.exec(ctx, `DELETE FROM tern_tokens WHERE hash = ?`, hash); err != nil {
		return 0, "", err
	}
	if time.Now().UnixMilli() > expires {
		return 0, "", ErrInvalidToken
	}
	return userID, data, nil
}

func (a *Accounts) send(c *kit.Ctx, kind string, u *User, link string) error {
	return a.sendWith(c, kind, u, link, nil)
}

// sendWith sends an email whose text takes params besides name and link.
func (a *Accounts) sendWith(c *kit.Ctx, kind string, u *User, link string, params map[string]any) error {
	if a.opts.Send == nil {
		return ErrNoEmail
	}
	p := map[string]any{"name": u.Name, "link": link}
	for k, v := range params {
		p[k] = v
	}
	key := "tern.accounts." + kind
	e := Email{Kind: kind, To: u.Email, Name: u.Name, Link: link,
		Subject: c.TB(key+".subject", p),
		Text:    c.TB(key+".text", p),
	}
	paras := strings.Split(e.Text, "\n\n")
	var b strings.Builder
	for _, para := range paras {
		para = html.EscapeString(para)
		if link != "" {
			para = strings.ReplaceAll(para, html.EscapeString(link), fmt.Sprintf(`<a href="%s">%s</a>`, html.EscapeString(link), html.EscapeString(c.TB(key+".button", p))))
		}
		fmt.Fprintf(&b, "<p>%s</p>\n", strings.ReplaceAll(para, "\n", "<br>"))
	}
	e.HTML = b.String()
	return a.opts.Send(c.Request.Context(), e)
}

// SendVerification emails the signed-in user a link that verifies their
// email address.
func (a *Accounts) SendVerification(c *kit.Ctx) error {
	u := Current(c)
	if u == nil {
		return ErrSignedOut
	}
	if u.EmailVerified {
		return nil
	}
	if err := a.limit(c, "email", u.Email); err != nil {
		return err
	}
	token, err := a.issue(c.Request.Context(), "verify", u.ID, u.Email, verifyFor)
	if err != nil || token == "" {
		return err
	}
	return a.send(c, "verify", u, a.link(c, a.opts.VerifyPath, token))
}

// VerifyEmail verifies the address a verification link was sent to, or
// confirms a new address from RequestEmailChange's link.
func (a *Accounts) VerifyEmail(c *kit.Ctx, token string) (*User, error) {
	ctx := c.Request.Context()
	id, email, err := a.redeem(ctx, "verify", token)
	if errors.Is(err, ErrInvalidToken) {
		return a.confirmEmailChange(c, token)
	}
	if err != nil {
		return nil, err
	}
	// Only the address the link was sent to: not one changed since.
	if _, err := a.exec(ctx, `UPDATE tern_users SET email_verified = ? WHERE id = ? AND email = ?`, time.Now().UnixMilli(), id, email); err != nil {
		return nil, err
	}
	a.Audit(c, Event{Action: "user.email_verified", UserID: id, Target: email})
	if cur := Current(c); cur != nil && cur.ID == id {
		cur.EmailVerified = true
	}
	return a.UserByID(ctx, id)
}

// RequestPasswordReset emails a reset link if an account has this email.
// It returns nil either way, so the form doesn't reveal who has one.
func (a *Accounts) RequestPasswordReset(c *kit.Ctx, email string) error {
	if a.opts.Send == nil {
		return ErrNoEmail
	}
	if err := a.limit(c, "email", normalizeEmail(email)); err != nil {
		return err
	}
	u, err := a.UserByEmail(c.Request.Context(), email)
	if err != nil || u == nil {
		return err
	}
	token, err := a.issue(c.Request.Context(), "reset", u.ID, u.Email, resetFor)
	if err != nil || token == "" {
		return err
	}
	return a.send(c, "reset", u, a.link(c, a.opts.ResetPath, token))
}

// ResetPassword sets a new password with a reset link's token. It signs
// the user out everywhere and in here, and verifies their email: the link
// reached it. With two-step sign-in on, it returns ErrTwoStepRequired
// instead of signing in, as SignIn does.
func (a *Accounts) ResetPassword(c *kit.Ctx, token, password string) (*User, error) {
	ctx := c.Request.Context()
	id, email, err := a.redeem(ctx, "reset", token)
	if err != nil {
		return nil, err
	}
	h, err := HashPassword(password)
	if err != nil {
		return nil, err
	}
	if _, err := a.exec(ctx, `UPDATE tern_users SET password = ?, email_verified = COALESCE(email_verified, ?) WHERE id = ? AND email = ?`, h, time.Now().UnixMilli(), id, email); err != nil {
		return nil, err
	}
	if _, err := a.exec(ctx, `DELETE FROM tern_sessions WHERE user_id = ?`, id); err != nil {
		return nil, err
	}
	a.Audit(c, Event{Action: "user.password_reset", UserID: id, Target: email})
	u, err := a.UserByID(ctx, id)
	if err != nil || u == nil {
		return nil, ErrInvalidToken
	}
	// The link proves the email, not the second step: ask for the code.
	if u.TwoStep {
		return u, a.awaitCode(c, u, false)
	}
	if err := a.startSession(c, u, false); err != nil {
		return nil, err
	}
	return u, nil
}

// RequestEmailChange emails a link to newEmail that makes it the signed-in
// user's address. The address only changes once the link is opened, at
// VerifyPath, so a typo can't lock anyone out. The old address is told.
func (a *Accounts) RequestEmailChange(c *kit.Ctx, newEmail string) error {
	u := Current(c)
	if u == nil {
		return ErrSignedOut
	}
	if a.opts.Send == nil {
		return ErrNoEmail
	}
	newEmail = normalizeEmail(newEmail)
	if newEmail == u.Email {
		return nil
	}
	if err := a.limit(c, "email", u.Email); err != nil {
		return err
	}
	if other, err := a.UserByEmail(c.Request.Context(), newEmail); err != nil {
		return err
	} else if other != nil {
		return ErrEmailTaken
	}
	token, err := a.issue(c.Request.Context(), "email-change", u.ID, newEmail, verifyFor)
	if err != nil || token == "" {
		return err
	}
	to := *u
	to.Email = newEmail
	return a.send(c, "change", &to, a.link(c, a.opts.VerifyPath, token))
}

func (a *Accounts) confirmEmailChange(c *kit.Ctx, token string) (*User, error) {
	ctx := c.Request.Context()
	id, email, err := a.redeem(ctx, "email-change", token)
	if err != nil {
		return nil, err
	}
	old, err := a.UserByID(ctx, id)
	if err != nil || old == nil {
		return nil, ErrInvalidToken
	}
	if _, err := a.exec(ctx, `UPDATE tern_users SET email = ?, email_verified = ? WHERE id = ?`, email, time.Now().UnixMilli(), id); err != nil {
		return nil, a.uniqueErr(err)
	}
	// Links sent to the old address no longer apply.
	_, _ = a.exec(ctx, `DELETE FROM tern_tokens WHERE user_id = ? AND purpose IN ('verify', 'reset', 'email-change')`, id)
	a.Audit(c, Event{Action: "user.email_changed", UserID: id, Target: email, Data: map[string]string{"from": old.Email, "to": email}})
	if a.opts.Send != nil {
		notice := *old
		_ = a.send(c, "changed", &notice, "")
	}
	if cur := Current(c); cur != nil && cur.ID == id {
		cur.Email, cur.EmailVerified = email, true
	}
	return a.UserByID(ctx, id)
}

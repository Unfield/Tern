package accounts

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1" //nolint:gosec // TOTP is defined with HMAC-SHA1, which authenticator apps use
	"crypto/subtle"
	"database/sql"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"rsc.io/qr"

	"github.com/Unfield/Tern/kit"
)

// Two-step sign-in: after the password, a code from an authenticator app
// (TOTP, RFC 6238: HMAC-SHA1, six digits, 30 seconds). Recovery codes
// sign in once each when the phone is lost. Passkeys don't ask for a code:
// they're two steps already, the device and its screen lock.

const (
	totpStep    = 30 // seconds
	totpDigits  = 6
	twoStepFor  = 10 * time.Minute // between the password and the code
	twoStepName = "_2fa"           // suffix of the pending sign-in's cookie
	recoveryN   = 10
)

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// TwoStepSetup is what a user needs to add the account to their
// authenticator app: a QR code to scan, or the secret to type.
type TwoStepSetup struct {
	// Secret is the key in base32, grouped by four for typing.
	Secret string `json:"secret"`
	// URI is the otpauth:// address the QR code holds; on a phone, a link
	// to it opens the authenticator app.
	URI string `json:"uri"`
	// QR is the QR code as a data: URL of a PNG image.
	QR string `json:"qr"`
}

// totp is the code for a time step.
func totp(secret []byte, step int64) string {
	m := hmac.New(sha1.New, secret)
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], uint64(step))
	m.Write(b[:])
	sum := m.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	n := binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff
	return fmt.Sprintf("%0*d", totpDigits, n%1000000)
}

// checkTOTP finds the step a code is for, within one step of now, and
// after the last one used, so a code works once.
func checkTOTP(secret []byte, code string, now time.Time, lastStep int64) (int64, bool) {
	code = strings.ReplaceAll(strings.TrimSpace(code), " ", "")
	if len(code) != totpDigits {
		return 0, false
	}
	cur := now.Unix() / totpStep
	for _, s := range []int64{cur, cur - 1, cur + 1} {
		if s > lastStep && subtle.ConstantTimeCompare([]byte(totp(secret, s)), []byte(code)) == 1 {
			return s, true
		}
	}
	return 0, false
}

func (a *Accounts) issuer(c *kit.Ctx) string {
	if a.opts.Issuer != "" {
		return a.opts.Issuer
	}
	id, _ := a.rp(c)
	return id
}

// SetUpTwoStep starts turning on two-step sign-in for the signed-in user:
// show the result, then call ConfirmTwoStep with a code from the app.
// Calling it again starts over with a new secret, until it's confirmed.
func (a *Accounts) SetUpTwoStep(c *kit.Ctx) (*TwoStepSetup, error) {
	u := Current(c)
	if u == nil {
		return nil, ErrSignedOut
	}
	if u.TwoStep {
		return nil, errors.New("accounts: two-step sign-in is already on")
	}
	raw := make([]byte, 20)
	_, _ = rand.Read(raw)
	secret := b32.EncodeToString(raw)
	ctx := c.Request.Context()
	if _, err := a.exec(ctx, `DELETE FROM tern_two_step WHERE user_id = ?`, u.ID); err != nil {
		return nil, err
	}
	if _, err := a.exec(ctx, `INSERT INTO tern_two_step (user_id, secret, last_step) VALUES (?, ?, 0)`, u.ID, secret); err != nil {
		return nil, err
	}
	return a.setup(c, u, secret)
}

// PendingTwoStepSetup is the setup SetUpTwoStep started and that isn't
// confirmed yet, or nil, for showing it again after a reload.
func (a *Accounts) PendingTwoStepSetup(c *kit.Ctx) (*TwoStepSetup, error) {
	u := Current(c)
	if u == nil || u.TwoStep {
		return nil, nil
	}
	var secret string
	err := a.queryRow(c.Request.Context(), `SELECT secret FROM tern_two_step WHERE user_id = ? AND confirmed_at IS NULL`, u.ID).Scan(&secret)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return a.setup(c, u, secret)
}

func (a *Accounts) setup(c *kit.Ctx, u *User, secret string) (*TwoStepSetup, error) {
	issuer := a.issuer(c)
	label := url.PathEscape(issuer + ":" + u.Email)
	q := url.Values{"secret": {secret}, "issuer": {issuer}, "algorithm": {"SHA1"}, "digits": {"6"}, "period": {"30"}}
	uri := "otpauth://totp/" + label + "?" + q.Encode()
	code, err := qr.Encode(uri, qr.M)
	if err != nil {
		return nil, err
	}
	code.Scale = 6
	var grouped strings.Builder
	for i, r := range secret {
		if i > 0 && i%4 == 0 {
			grouped.WriteByte(' ')
		}
		grouped.WriteRune(r)
	}
	return &TwoStepSetup{Secret: grouped.String(), URI: uri, QR: "data:image/png;base64," + base64.StdEncoding.EncodeToString(code.PNG())}, nil
}

// ConfirmTwoStep turns two-step sign-in on with a code from the app, and
// returns the recovery codes, to show the user once.
func (a *Accounts) ConfirmTwoStep(c *kit.Ctx, code string) ([]string, error) {
	u := Current(c)
	if u == nil {
		return nil, ErrSignedOut
	}
	if err := a.limit(c, "code", fmt.Sprint(u.ID)); err != nil {
		return nil, err
	}
	ctx := c.Request.Context()
	var secret string
	var last int64
	err := a.queryRow(ctx, `SELECT secret, last_step FROM tern_two_step WHERE user_id = ? AND confirmed_at IS NULL`, u.ID).Scan(&secret, &last)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInvalidCode
	}
	if err != nil {
		return nil, err
	}
	key, err := b32.DecodeString(secret)
	if err != nil {
		return nil, err
	}
	step, ok := checkTOTP(key, code, time.Now(), last)
	if !ok {
		return nil, ErrInvalidCode
	}
	if _, err := a.exec(ctx, `UPDATE tern_two_step SET confirmed_at = ?, last_step = ? WHERE user_id = ?`, time.Now().UnixMilli(), step, u.ID); err != nil {
		return nil, err
	}
	a.succeeded("code", fmt.Sprint(u.ID))
	u.TwoStep = true
	return a.newRecoveryCodes(c, u.ID)
}

// RecoveryCodes replaces the signed-in user's recovery codes with new
// ones, to show once.
func (a *Accounts) RecoveryCodes(c *kit.Ctx) ([]string, error) {
	u := Current(c)
	if u == nil {
		return nil, ErrSignedOut
	}
	if !u.TwoStep {
		return nil, errors.New("accounts: two-step sign-in is off")
	}
	return a.newRecoveryCodes(c, u.ID)
}

// RecoveryCodesLeft is how many unused recovery codes the signed-in user has.
func (a *Accounts) RecoveryCodesLeft(c *kit.Ctx) (int, error) {
	u := Current(c)
	if u == nil {
		return 0, ErrSignedOut
	}
	var n int
	err := a.queryRow(c.Request.Context(), `SELECT COUNT(*) FROM tern_recovery_codes WHERE user_id = ?`, u.ID).Scan(&n)
	return n, err
}

func (a *Accounts) newRecoveryCodes(c *kit.Ctx, userID int64) ([]string, error) {
	ctx := c.Request.Context()
	if _, err := a.exec(ctx, `DELETE FROM tern_recovery_codes WHERE user_id = ?`, userID); err != nil {
		return nil, err
	}
	codes := make([]string, recoveryN)
	for i := range codes {
		raw := make([]byte, 5)
		_, _ = rand.Read(raw)
		s := strings.ToLower(b32.EncodeToString(raw)) // 8 characters
		codes[i] = s[:4] + "-" + s[4:]
		if _, err := a.exec(ctx, `INSERT INTO tern_recovery_codes (hash, user_id) VALUES (?, ?)`, hashToken(codes[i]), userID); err != nil {
			return nil, err
		}
	}
	return codes, nil
}

// TurnOffTwoStep turns two-step sign-in off, with a current code or a
// recovery code.
func (a *Accounts) TurnOffTwoStep(c *kit.Ctx, code string) error {
	u := Current(c)
	if u == nil {
		return ErrSignedOut
	}
	if !u.TwoStep {
		return nil
	}
	if err := a.checkCode(c, u.ID, code); err != nil {
		return err
	}
	ctx := c.Request.Context()
	for _, q := range []string{`DELETE FROM tern_two_step WHERE user_id = ?`, `DELETE FROM tern_recovery_codes WHERE user_id = ?`} {
		if _, err := a.exec(ctx, q, u.ID); err != nil {
			return err
		}
	}
	u.TwoStep = false
	return nil
}

// checkCode checks a code from the app, or uses up a recovery code.
func (a *Accounts) checkCode(c *kit.Ctx, userID int64, code string) error {
	key := fmt.Sprint(userID)
	if err := a.limit(c, "code", key); err != nil {
		return err
	}
	ctx := c.Request.Context()
	code = strings.ToLower(strings.TrimSpace(code))
	if strings.Contains(code, "-") || len(code) == 8 {
		if len(code) == 8 {
			code = code[:4] + "-" + code[4:]
		}
		res, err := a.exec(ctx, `DELETE FROM tern_recovery_codes WHERE hash = ? AND user_id = ?`, hashToken(code), userID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 1 {
			a.succeeded("code", key)
			return nil
		}
		return ErrInvalidCode
	}
	var secret string
	var last int64
	err := a.queryRow(ctx, `SELECT secret, last_step FROM tern_two_step WHERE user_id = ? AND confirmed_at IS NOT NULL`, userID).Scan(&secret, &last)
	if err != nil {
		return ErrInvalidCode
	}
	raw, err := b32.DecodeString(secret)
	if err != nil {
		return err
	}
	step, ok := checkTOTP(raw, code, time.Now(), last)
	if !ok {
		return ErrInvalidCode
	}
	// Only if no one used this step meanwhile: a code works once.
	res, err := a.exec(ctx, `UPDATE tern_two_step SET last_step = ? WHERE user_id = ? AND last_step < ?`, step, userID, step)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return ErrInvalidCode
	}
	a.succeeded("code", key)
	return nil
}

// awaitCode remembers a password sign-in waiting for its code, in a
// cookie of its own, and returns ErrTwoStepRequired.
func (a *Accounts) awaitCode(c *kit.Ctx, u *User, remember bool) error {
	data := "0"
	if remember {
		data = "1"
	}
	token, hash := newToken()
	now := time.Now()
	if _, err := a.exec(c.Request.Context(), `INSERT INTO tern_tokens (hash, purpose, user_id, data, created_at, expires_at) VALUES (?, ?, ?, ?, ?, ?)`,
		hash, "two-step", u.ID, data, now.UnixMilli(), now.Add(twoStepFor).UnixMilli()); err != nil {
		return err
	}
	c.SetCookie(&http.Cookie{Name: a.opts.Cookie + twoStepName, Value: token, HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: int(twoStepFor / time.Second)})
	return ErrTwoStepRequired
}

// pendingSignIn is the user a sign-in waits on a code for.
func (a *Accounts) pendingSignIn(c *kit.Ctx) (userID int64, remember bool, token string) {
	token, ok := c.Cookie(a.opts.Cookie + twoStepName)
	if !ok || token == "" {
		return 0, false, ""
	}
	var data string
	var expires int64
	err := a.queryRow(c.Request.Context(), `SELECT user_id, data, expires_at FROM tern_tokens WHERE hash = ? AND purpose = 'two-step'`, hashToken(token)).Scan(&userID, &data, &expires)
	if err != nil || time.Now().UnixMilli() > expires {
		return 0, false, ""
	}
	return userID, data == "1", token
}

// AwaitingCode reports whether this browser has signed in with a password
// and still has to enter its code, for the code page's load: without one,
// send the user back to sign in.
func (a *Accounts) AwaitingCode(c *kit.Ctx) bool {
	id, _, _ := a.pendingSignIn(c)
	return id != 0
}

// CompleteSignIn finishes a sign-in that returned ErrTwoStepRequired, with
// a code from the app or a recovery code.
func (a *Accounts) CompleteSignIn(c *kit.Ctx, code string) (*User, error) {
	id, remember, token := a.pendingSignIn(c)
	if id == 0 {
		return nil, ErrInvalidToken
	}
	if err := a.checkCode(c, id, code); err != nil {
		return nil, err
	}
	ctx := c.Request.Context()
	_, _ = a.exec(ctx, `DELETE FROM tern_tokens WHERE hash = ?`, hashToken(token))
	c.SetCookie(&http.Cookie{Name: a.opts.Cookie + twoStepName, Value: "", MaxAge: -1, HttpOnly: true})
	u, err := a.UserByID(ctx, id)
	if err != nil || u == nil {
		return nil, ErrInvalidToken
	}
	if err := a.startSession(c, u, remember); err != nil {
		return nil, err
	}
	return u, nil
}

// TwoStepPath is Options.TwoStepPath, for redirects to the code page.
func (a *Accounts) TwoStepPath() string { return a.opts.TwoStepPath }

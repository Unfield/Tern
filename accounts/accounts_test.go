package accounts

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/Unfield/Tern/kit"
)

type harness struct {
	t    *testing.T
	acc  *Accounts
	srv  *httptest.Server
	mu   sync.Mutex
	sent []Email
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+t.Name()+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	h := &harness{t: t}
	acc, err := New(db, Options{Send: func(_ context.Context, e Email) error {
		h.mu.Lock()
		h.sent = append(h.sent, e)
		h.mu.Unlock()
		return nil
	}, RPName: "Test"})
	if err != nil {
		t.Fatal(err)
	}
	if err := acc.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	h.acc = acc
	post := func(f func(c *kit.Ctx) (any, error)) map[string]kit.HandlerFunc {
		return map[string]kit.HandlerFunc{"POST": func(c *kit.Ctx) error {
			v, err := f(c)
			if err != nil {
				return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
			}
			return c.JSON(http.StatusOK, v)
		}}
	}
	form := func(c *kit.Ctx, k string) string { return c.Request.FormValue(k) }
	m := &kit.Manifest{
		AppHTML: `<html><head>%tern.head%</head><body>%tern.body%</body></html>`,
		Routes: []kit.RouteDef{
			{ID: "/signup", Endpoint: post(func(c *kit.Ctx) (any, error) {
				return acc.SignUp(c, form(c, "email"), form(c, "name"), form(c, "password"))
			})},
			{ID: "/signin", Endpoint: post(func(c *kit.Ctx) (any, error) {
				return acc.SignIn(c, form(c, "email"), form(c, "password"), form(c, "remember") == "1")
			})},
			{ID: "/signout", Endpoint: post(func(c *kit.Ctx) (any, error) { return nil, acc.SignOut(c) })},
			{ID: "/me", Endpoint: post(func(c *kit.Ctx) (any, error) { return acc.Require(c) })},
			{ID: "/sessions", Endpoint: post(func(c *kit.Ctx) (any, error) { return acc.Sessions(c) })},
			{ID: "/revoke", Endpoint: post(func(c *kit.Ctx) (any, error) { return nil, acc.Revoke(c, form(c, "id")) })},
			{ID: "/others", Endpoint: post(func(c *kit.Ctx) (any, error) { return nil, acc.SignOutOthers(c) })},
			{ID: "/password", Endpoint: post(func(c *kit.Ctx) (any, error) {
				return nil, acc.ChangePassword(c, form(c, "current"), form(c, "password"))
			})},
			{ID: "/verify/send", Endpoint: post(func(c *kit.Ctx) (any, error) { return nil, acc.SendVerification(c) })},
			{ID: "/verify", Endpoint: post(func(c *kit.Ctx) (any, error) { return acc.VerifyEmail(c, form(c, "token")) })},
			{ID: "/reset/send", Endpoint: post(func(c *kit.Ctx) (any, error) { return nil, acc.RequestPasswordReset(c, form(c, "email")) })},
			{ID: "/reset", Endpoint: post(func(c *kit.Ctx) (any, error) {
				return acc.ResetPassword(c, form(c, "token"), form(c, "password"))
			})},
			{ID: "/passkeys", Endpoint: post(func(c *kit.Ctx) (any, error) { return acc.Passkeys(c) })},
			{ID: "/email/change", Endpoint: post(func(c *kit.Ctx) (any, error) { return nil, acc.RequestEmailChange(c, form(c, "email")) })},
			{ID: "/2fa/setup", Endpoint: post(func(c *kit.Ctx) (any, error) { return acc.SetUpTwoStep(c) })},
			{ID: "/2fa/confirm", Endpoint: post(func(c *kit.Ctx) (any, error) {
				codes, err := acc.ConfirmTwoStep(c, form(c, "code"))
				return map[string]any{"codes": codes}, err
			})},
			{ID: "/2fa/code", Endpoint: post(func(c *kit.Ctx) (any, error) { return acc.CompleteSignIn(c, form(c, "code")) })},
			{ID: "/2fa/off", Endpoint: post(func(c *kit.Ctx) (any, error) { return nil, acc.TurnOffTwoStep(c, form(c, "code")) })},
			{ID: "/with", Endpoint: post(func(c *kit.Ctx) (any, error) {
				return acc.SignInWith(c, Identity{Provider: form(c, "provider"), Subject: form(c, "subject"), Email: form(c, "email"), EmailVerified: form(c, "verified") == "1", Name: "X"})
			})},
			{ID: "/identities", Endpoint: post(func(c *kit.Ctx) (any, error) { return acc.Identities(c) })},
			{ID: "/unlink", Endpoint: post(func(c *kit.Ctx) (any, error) { return nil, acc.Unlink(c, form(c, "provider")) })},
			{ID: "/delete", Endpoint: post(func(c *kit.Ctx) (any, error) { return nil, acc.DeleteAccount(c) })},
		},
	}
	app, err := kit.New(m, kit.WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))), kit.WithPlugins(acc))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	h.srv = httptest.NewServer(app)
	t.Cleanup(h.srv.Close)
	return h
}

// client is one browser.
func (h *harness) client() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar}
}

func (h *harness) post(c *http.Client, path string, kv ...string) (int, map[string]any) {
	h.t.Helper()
	v := url.Values{}
	for i := 0; i+1 < len(kv); i += 2 {
		v.Set(kv[i], kv[i+1])
	}
	req, _ := http.NewRequest(http.MethodPost, h.srv.URL+path, strings.NewReader(v.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", h.srv.URL)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_0) Firefox/130.0")
	return h.do(c, req)
}

func (h *harness) do(c *http.Client, req *http.Request) (int, map[string]any) {
	h.t.Helper()
	res, err := c.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	body, _ := io.ReadAll(res.Body)
	var out map[string]any
	if len(body) > 0 && body[0] == '{' {
		_ = json.Unmarshal(body, &out)
	} else if len(body) > 0 && body[0] == '[' {
		var list []any
		_ = json.Unmarshal(body, &list)
		out = map[string]any{"list": list}
	}
	if out == nil {
		out = map[string]any{"raw": string(body)}
	}
	return res.StatusCode, out
}

func (h *harness) lastEmail() Email {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.sent) == 0 {
		h.t.Fatal("no email sent")
	}
	return h.sent[len(h.sent)-1]
}

func tokenIn(t *testing.T, e Email) string {
	t.Helper()
	i := strings.Index(e.Link, "token=")
	if i < 0 {
		t.Fatalf("no token in %q", e.Link)
	}
	tok, _ := url.QueryUnescape(e.Link[i+len("token="):])
	return tok
}

func TestPasswords(t *testing.T) {
	hash, err := HashPassword("correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if ok, rehash := CheckPassword(hash, "correct horse"); !ok || rehash {
		t.Fatalf("ok=%v rehash=%v", ok, rehash)
	}
	if ok, _ := CheckPassword(hash, "wrong"); ok {
		t.Fatal("wrong password accepted")
	}
	if ok, _ := CheckPassword("", "anything"); ok {
		t.Fatal("empty hash accepted")
	}
}

func TestSignUpInOut(t *testing.T) {
	h := newHarness(t)
	b := h.client()
	if code, out := h.post(b, "/signup", "email", " Ada@Example.com ", "name", "Ada", "password", "analytical engine"); code != 200 || out["email"] != "ada@example.com" {
		t.Fatalf("signup: %d %v", code, out)
	}
	if code, out := h.post(b, "/me"); code != 200 || out["name"] != "Ada" {
		t.Fatalf("me after signup: %d %v", code, out)
	}
	if code, out := h.post(h.client(), "/signup", "email", "ada@example.com", "password", "another one"); code != 400 || !strings.Contains(out["error"].(string), "already exists") {
		t.Fatalf("duplicate signup: %d %v", code, out)
	}
	if code, _ := h.post(b, "/signout"); code != 200 {
		t.Fatal("signout")
	}
	if code, _ := h.post(b, "/me"); code == 200 {
		t.Fatal("still signed in after signout")
	}
	if code, out := h.post(b, "/signin", "email", "ada@example.com", "password", "nope"); code != 400 || out["error"] != ErrInvalidCredentials.Error() {
		t.Fatalf("wrong password: %d %v", code, out)
	}
	if code, out := h.post(b, "/signin", "email", "nobody@example.com", "password", "nope"); code != 400 || out["error"] != ErrInvalidCredentials.Error() {
		t.Fatalf("unknown email: %d %v", code, out)
	}
	if code, _ := h.post(b, "/signin", "email", "ADA@example.com", "password", "analytical engine", "remember", "1"); code != 200 {
		t.Fatal("signin")
	}
	if code, _ := h.post(b, "/me"); code != 200 {
		t.Fatal("not signed in after signin")
	}
}

func TestSessionsAndRevoke(t *testing.T) {
	h := newHarness(t)
	laptop, phone := h.client(), h.client()
	h.post(laptop, "/signup", "email", "a@example.com", "password", "long enough")
	h.post(phone, "/signin", "email", "a@example.com", "password", "long enough")
	_, out := h.post(laptop, "/sessions")
	list := out["list"].([]any)
	if len(list) != 2 {
		t.Fatalf("sessions: %v", out)
	}
	first := list[0].(map[string]any)
	if first["current"] != true || first["device"] != "Firefox on macOS" {
		t.Fatalf("first session: %v", first)
	}
	other := list[1].(map[string]any)
	if code, _ := h.post(laptop, "/revoke", "id", other["id"].(string)); code != 200 {
		t.Fatal("revoke")
	}
	if code, _ := h.post(phone, "/me"); code == 200 {
		t.Fatal("revoked session still signed in")
	}
	if code, _ := h.post(laptop, "/me"); code != 200 {
		t.Fatal("revoking another session signed this one out")
	}

	h.post(phone, "/signin", "email", "a@example.com", "password", "long enough")
	h.post(laptop, "/others")
	if code, _ := h.post(phone, "/me"); code == 200 {
		t.Fatal("sign out others left a session")
	}
	if code, _ := h.post(laptop, "/me"); code != 200 {
		t.Fatal("sign out others signed out this session")
	}
}

func TestChangePasswordSignsOutOthers(t *testing.T) {
	h := newHarness(t)
	a, b := h.client(), h.client()
	h.post(a, "/signup", "email", "a@example.com", "password", "first password")
	h.post(b, "/signin", "email", "a@example.com", "password", "first password")
	if code, out := h.post(a, "/password", "current", "wrong", "password", "second password"); code != 400 {
		t.Fatalf("wrong current password accepted: %v", out)
	}
	if code, out := h.post(a, "/password", "current", "first password", "password", "second password"); code != 200 {
		t.Fatalf("change password: %v", out)
	}
	if code, _ := h.post(b, "/me"); code == 200 {
		t.Fatal("other session survived a password change")
	}
	if code, _ := h.post(a, "/me"); code != 200 {
		t.Fatal("password change signed out the session that made it")
	}
	if code, _ := h.post(h.client(), "/signin", "email", "a@example.com", "password", "second password"); code != 200 {
		t.Fatal("new password doesn't sign in")
	}
}

func TestVerifyEmail(t *testing.T) {
	h := newHarness(t)
	b := h.client()
	h.post(b, "/signup", "email", "a@example.com", "password", "long enough")
	if code, out := h.post(b, "/verify/send"); code != 200 {
		t.Fatalf("send verification: %v", out)
	}
	e := h.lastEmail()
	if e.To != "a@example.com" || !strings.HasPrefix(e.Link, h.srv.URL+"/verify-email?token=") {
		t.Fatalf("email: %+v", e)
	}
	tok := tokenIn(t, e)
	if code, out := h.post(h.client(), "/verify", "token", tok); code != 200 || out["emailVerified"] != true {
		t.Fatalf("verify: %d %v", code, out)
	}
	if code, _ := h.post(h.client(), "/verify", "token", tok); code != 400 {
		t.Fatal("a verification link worked twice")
	}
}

func TestPasswordReset(t *testing.T) {
	h := newHarness(t)
	old := h.client()
	h.post(old, "/signup", "email", "a@example.com", "password", "forgotten one")
	// Unknown emails look the same as known ones, and send nothing.
	if code, _ := h.post(h.client(), "/reset/send", "email", "nobody@example.com"); code != 200 {
		t.Fatal("reset for unknown email failed")
	}
	h.mu.Lock()
	n := len(h.sent)
	h.mu.Unlock()
	if n != 0 {
		t.Fatal("reset email sent to an unknown address")
	}
	h.post(h.client(), "/reset/send", "email", "a@example.com")
	tok := tokenIn(t, h.lastEmail())
	b := h.client()
	if code, out := h.post(b, "/reset", "token", tok, "password", "remembered now"); code != 200 {
		t.Fatalf("reset: %v", out)
	}
	if code, _ := h.post(old, "/me"); code == 200 {
		t.Fatal("old session survived a reset")
	}
	if code, _ := h.post(b, "/reset", "token", tok, "password", "again and again"); code != 400 {
		t.Fatal("a reset link worked twice")
	}
	if code, _ := h.post(h.client(), "/signin", "email", "a@example.com", "password", "remembered now"); code != 200 {
		t.Fatal("reset password doesn't sign in")
	}
}

func TestDeleteAccount(t *testing.T) {
	h := newHarness(t)
	b := h.client()
	h.post(b, "/signup", "email", "a@example.com", "password", "long enough")
	if code, _ := h.post(b, "/delete"); code != 200 {
		t.Fatal("delete")
	}
	if code, _ := h.post(b, "/me"); code == 200 {
		t.Fatal("signed in after delete")
	}
	if code, _ := h.post(h.client(), "/signup", "email", "a@example.com", "password", "long enough"); code != 200 {
		t.Fatal("email not free after delete")
	}
}

func TestCBOR(t *testing.T) {
	// {1: 2, -1: h'0102', "a": [true, null]}
	b := []byte{0xa3, 0x01, 0x02, 0x20, 0x42, 0x01, 0x02, 0x61, 'a', 0x82, 0xf5, 0xf6}
	v, n, err := decodeCBOR(b)
	if err != nil || n != len(b) {
		t.Fatalf("%v %d", err, n)
	}
	m := v.(map[any]any)
	if m[int64(1)] != int64(2) || !bytes.Equal(m[int64(-1)].([]byte), []byte{1, 2}) || len(m["a"].([]any)) != 2 {
		t.Fatalf("%#v", m)
	}
	for _, bad := range [][]byte{{}, {0x5a, 0xff, 0xff, 0xff, 0xff}, {0x9f}, {0xa1}, {0x1c}} {
		if _, _, err := decodeCBOR(bad); err == nil {
			t.Fatalf("% x decoded", bad)
		}
	}
}

// A software authenticator, for the passkey ceremonies.

type authenticator struct {
	key    *ecdsa.PrivateKey
	credID []byte
	count  uint32
	user   []byte
}

func cborHead(major byte, n int) []byte {
	switch {
	case n < 24:
		return []byte{major<<5 | byte(n)}
	case n < 256:
		return []byte{major<<5 | 24, byte(n)}
	default:
		return []byte{major<<5 | 25, byte(n >> 8), byte(n)}
	}
}

func cborBytes(b []byte) []byte { return append(cborHead(2, len(b)), b...) }
func cborText(s string) []byte  { return append(cborHead(3, len(s)), s...) }
func cborInt(n int) []byte {
	if n < 0 {
		return cborHead(1, -1-n)
	}
	return cborHead(0, n)
}

func (au *authenticator) coseKey() []byte {
	x := au.key.X.FillBytes(make([]byte, 32))
	y := au.key.Y.FillBytes(make([]byte, 32))
	out := cborHead(5, 5)
	out = append(out, cborInt(1)...)
	out = append(out, cborInt(2)...) // kty: EC2
	out = append(out, cborInt(3)...)
	out = append(out, cborInt(-7)...) // alg: ES256
	out = append(out, cborInt(-1)...)
	out = append(out, cborInt(1)...) // crv: P-256
	out = append(out, cborInt(-2)...)
	out = append(out, cborBytes(x)...)
	out = append(out, cborInt(-3)...)
	out = append(out, cborBytes(y)...)
	return out
}

func (au *authenticator) authData(rpID string, attest bool) []byte {
	h := sha256.Sum256([]byte(rpID))
	out := append([]byte(nil), h[:]...)
	flags := byte(flagUP | 0x04)
	if attest {
		flags |= flagAT
	}
	au.count++
	out = append(out, flags)
	out = binary.BigEndian.AppendUint32(out, au.count)
	if attest {
		out = append(out, make([]byte, 16)...) // aaguid
		out = binary.BigEndian.AppendUint16(out, uint16(len(au.credID)))
		out = append(out, au.credID...)
		out = append(out, au.coseKey()...)
	}
	return out
}

func clientDataJSON(typ, challenge, origin string) []byte {
	b, _ := json.Marshal(map[string]any{"type": typ, "challenge": challenge, "origin": origin})
	return b
}

func (h *harness) postJSON(c *http.Client, path string, v any) (int, map[string]any) {
	h.t.Helper()
	body, _ := json.Marshal(v)
	req, _ := http.NewRequest(http.MethodPost, h.srv.URL+path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", h.srv.URL)
	return h.do(c, req)
}

func TestPasskeys(t *testing.T) {
	h := newHarness(t)
	b := h.client()
	h.post(b, "/signup", "email", "a@example.com", "password", "long enough")

	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	au := &authenticator{key: key, credID: []byte("credential-1")}

	// Register.
	code, opts := h.postJSON(b, "/_tern/passkeys/register/options", nil)
	if code != 200 {
		t.Fatalf("register options: %d %v", code, opts)
	}
	rp := opts["rp"].(map[string]any)
	rpID := rp["id"].(string)
	if rpID != "127.0.0.1" || rp["name"] != "Test" {
		t.Fatalf("rp: %v", rp)
	}
	au.user, _ = decodeB64(opts["user"].(map[string]any)["id"].(string))
	att := cborHead(5, 3)
	att = append(att, cborText("fmt")...)
	att = append(att, cborText("none")...)
	att = append(att, cborText("attStmt")...)
	att = append(att, cborHead(5, 0)...)
	att = append(att, cborText("authData")...)
	att = append(att, cborBytes(au.authData(rpID, true))...)
	reg := map[string]any{
		"id": b64.EncodeToString(au.credID), "type": "public-key", "name": "Test key",
		"response": map[string]any{
			"clientDataJSON":    b64.EncodeToString(clientDataJSON("webauthn.create", opts["challenge"].(string), h.srv.URL)),
			"attestationObject": b64.EncodeToString(att),
		},
	}
	if code, out := h.postJSON(b, "/_tern/passkeys/register", reg); code != 200 {
		t.Fatalf("register: %d %v", code, out)
	}
	if code, _ := h.postJSON(b, "/_tern/passkeys/register", reg); code == 200 {
		t.Fatal("a register challenge worked twice")
	}
	_, out := h.post(b, "/passkeys")
	if list := out["list"].([]any); len(list) != 1 || list[0].(map[string]any)["name"] != "Test key" {
		t.Fatalf("passkeys: %v", out)
	}

	// Sign in, in another browser.
	signIn := func(c *http.Client, origin string, tamper bool) int {
		_, opts := h.postJSON(c, "/_tern/passkeys/signin/options", nil)
		cd := clientDataJSON("webauthn.get", opts["challenge"].(string), origin)
		ad := au.authData(rpID, false)
		sum := sha256.Sum256(cd)
		digest := sha256.Sum256(append(append([]byte(nil), ad...), sum[:]...))
		sig, _ := ecdsa.SignASN1(rand.Reader, key, digest[:])
		if tamper {
			ad[len(ad)-1] ^= 1
		}
		code, _ := h.postJSON(c, "/_tern/passkeys/signin", map[string]any{
			"id": b64.EncodeToString(au.credID), "type": "public-key",
			"response": map[string]any{
				"clientDataJSON":    b64.EncodeToString(cd),
				"authenticatorData": b64.EncodeToString(ad),
				"signature":         b64.EncodeToString(sig),
				"userHandle":        b64.EncodeToString(au.user),
			},
		})
		return code
	}
	other := h.client()
	if code := signIn(other, "https://evil.example", false); code == 200 {
		t.Fatal("wrong origin signed in")
	}
	if code := signIn(other, h.srv.URL, true); code == 200 {
		t.Fatal("tampered data signed in")
	}
	if code := signIn(other, h.srv.URL, false); code != 200 {
		t.Fatalf("passkey sign in: %d", code)
	}
	if code, out := h.post(other, "/me"); code != 200 || out["email"] != "a@example.com" {
		t.Fatalf("me after passkey: %d %v", code, out)
	}
	// A counter going backwards is a cloned key.
	au.count = 0
	if code := signIn(h.client(), h.srv.URL, false); code == 200 {
		t.Fatal("a cloned passkey signed in")
	}
}

func TestPasskeySignedOut(t *testing.T) {
	h := newHarness(t)
	code, _ := h.postJSON(h.client(), "/_tern/passkeys/register/options", nil)
	if code != http.StatusUnauthorized {
		t.Fatalf("register options while signed out: %d", code)
	}
}

func TestEmailChange(t *testing.T) {
	h := newHarness(t)
	b := h.client()
	h.post(b, "/signup", "email", "old@example.com", "password", "long enough")
	h.post(h.client(), "/signup", "email", "taken@example.com", "password", "long enough")
	if code, out := h.post(b, "/email/change", "email", "taken@example.com"); code != 400 || out["error"] != ErrEmailTaken.Error() {
		t.Fatalf("taken: %d %v", code, out)
	}
	if code, out := h.post(b, "/email/change", "email", "New@Example.com"); code != 200 {
		t.Fatalf("request: %d %v", code, out)
	}
	e := h.lastEmail()
	if e.To != "new@example.com" || e.Kind != "change" {
		t.Fatalf("email: %+v", e)
	}
	// Nothing changes until the link is opened.
	if _, out := h.post(b, "/me"); out["email"] != "old@example.com" {
		t.Fatalf("changed early: %v", out)
	}
	if code, out := h.post(h.client(), "/verify", "token", tokenIn(t, e)); code != 200 || out["email"] != "new@example.com" || out["emailVerified"] != true {
		t.Fatalf("confirm: %d %v", code, out)
	}
	notice := h.lastEmail()
	if notice.To != "old@example.com" || notice.Kind != "changed" || notice.Link != "" || strings.Contains(notice.HTML, "<a") {
		t.Fatalf("notice: %+v", notice)
	}
	if code, _ := h.post(h.client(), "/signin", "email", "new@example.com", "password", "long enough"); code != 200 {
		t.Fatal("new address doesn't sign in")
	}
	if code, _ := h.post(h.client(), "/signin", "email", "old@example.com", "password", "long enough"); code == 200 {
		t.Fatal("old address still signs in")
	}
}

func TestRateLimits(t *testing.T) {
	h := newHarness(t)
	h.post(h.client(), "/signup", "email", "a@example.com", "password", "long enough")
	b := h.client()
	for i := range 10 {
		if code, out := h.post(b, "/signin", "email", "a@example.com", "password", "wrong"); out["error"] != ErrInvalidCredentials.Error() {
			t.Fatalf("attempt %d: %d %v", i, code, out)
		}
	}
	if _, out := h.post(b, "/signin", "email", "a@example.com", "password", "long enough"); out["error"] != ErrTooManyAttempts.Error() {
		t.Fatalf("11th attempt: %v", out)
	}
	// Another account from the same client isn't locked.
	h.post(h.client(), "/signup", "email", "b@example.com", "password", "long enough")
	if code, _ := h.post(b, "/signin", "email", "b@example.com", "password", "long enough"); code != 200 {
		t.Fatal("other account locked")
	}
}

func TestLimiter(t *testing.T) {
	l := newLimiter()
	now := time.Now()
	for i := range 3 {
		if !l.allow("k", 3, time.Minute, now.Add(time.Duration(i)*time.Second)) {
			t.Fatal("blocked early")
		}
	}
	if l.allow("k", 3, time.Minute, now.Add(5*time.Second)) {
		t.Fatal("4th allowed")
	}
	if !l.allow("k", 3, time.Minute, now.Add(61*time.Second)) {
		t.Fatal("window didn't move")
	}
	l.reset("k")
	if len(l.hits) != 0 {
		t.Fatal("reset")
	}
}

func TestTOTPVector(t *testing.T) {
	// RFC 6238, appendix B: SHA1 at T=59 is 94287082; six digits keep 287082.
	if got := totp([]byte("12345678901234567890"), 59/30); got != "287082" {
		t.Fatal(got)
	}
	if got := totp([]byte("12345678901234567890"), 1111111109/30); got != "081804" {
		t.Fatal(got)
	}
}

func TestTwoStep(t *testing.T) {
	h := newHarness(t)
	b := h.client()
	h.post(b, "/signup", "email", "a@example.com", "password", "long enough")
	_, setup := h.post(b, "/2fa/setup")
	secret := strings.ReplaceAll(setup["secret"].(string), " ", "")
	if !strings.HasPrefix(setup["uri"].(string), "otpauth://totp/Test:a@example.com?") || !strings.HasPrefix(setup["qr"].(string), "data:image/png;base64,") {
		t.Fatalf("setup: %v", setup)
	}
	key, err := b32.DecodeString(secret)
	if err != nil {
		t.Fatal(err)
	}
	step := time.Now().Unix() / totpStep
	if code, _ := h.post(b, "/2fa/confirm", "code", "000000"); code == 200 && totp(key, step) != "000000" {
		t.Fatal("wrong code confirmed")
	}
	code, out := h.post(b, "/2fa/confirm", "code", totp(key, step))
	if code != 200 {
		t.Fatalf("confirm: %d %v", code, out)
	}
	recovery := out["codes"].([]any)
	if len(recovery) != 10 {
		t.Fatalf("codes: %v", out)
	}
	if _, me := h.post(b, "/me"); me["twoStep"] != true {
		t.Fatalf("me: %v", me)
	}

	// A password alone no longer signs in.
	other := h.client()
	if _, out := h.post(other, "/signin", "email", "a@example.com", "password", "long enough"); out["error"] != ErrTwoStepRequired.Error() {
		t.Fatalf("signin: %v", out)
	}
	if code, _ := h.post(other, "/me"); code == 200 {
		t.Fatal("signed in without the code")
	}
	// The code used to confirm doesn't work again.
	if _, out := h.post(other, "/2fa/code", "code", totp(key, step)); out["error"] != ErrInvalidCode.Error() {
		t.Fatalf("reused code: %v", out)
	}
	if code, out := h.post(other, "/2fa/code", "code", totp(key, step+1)); code != 200 {
		t.Fatalf("code: %d %v", code, out)
	}
	if code, _ := h.post(other, "/me"); code != 200 {
		t.Fatal("not signed in after the code")
	}

	// A recovery code works once, typed with or without its dash.
	third := h.client()
	h.post(third, "/signin", "email", "a@example.com", "password", "long enough")
	rc := recovery[0].(string)
	if code, out := h.post(third, "/2fa/code", "code", strings.ToUpper(strings.ReplaceAll(rc, "-", ""))); code != 200 {
		t.Fatalf("recovery: %d %v", code, out)
	}
	fourth := h.client()
	h.post(fourth, "/signin", "email", "a@example.com", "password", "long enough")
	if _, out := h.post(fourth, "/2fa/code", "code", rc); out["error"] != ErrInvalidCode.Error() {
		t.Fatalf("recovery reused: %v", out)
	}
	// Without a pending sign-in, a code does nothing.
	if _, out := h.post(h.client(), "/2fa/code", "code", recovery[1].(string)); out["error"] != ErrInvalidToken.Error() {
		t.Fatalf("no pending sign-in: %v", out)
	}

	// Turning it off takes a code.
	if _, out := h.post(b, "/2fa/off", "code", "123"); out["error"] != ErrInvalidCode.Error() {
		t.Fatalf("off without code: %v", out)
	}
	if code, out := h.post(b, "/2fa/off", "code", recovery[2].(string)); code != 200 {
		t.Fatalf("off: %v", out)
	}
	if code, _ := h.post(h.client(), "/signin", "email", "a@example.com", "password", "long enough"); code != 200 {
		t.Fatal("password alone doesn't sign in after turning it off")
	}
}

func TestTwoStepAfterReset(t *testing.T) {
	h := newHarness(t)
	b := h.client()
	h.post(b, "/signup", "email", "a@example.com", "password", "long enough")
	_, setup := h.post(b, "/2fa/setup")
	key, _ := b32.DecodeString(strings.ReplaceAll(setup["secret"].(string), " ", ""))
	h.post(b, "/2fa/confirm", "code", totp(key, time.Now().Unix()/totpStep))
	h.post(h.client(), "/reset/send", "email", "a@example.com")
	c := h.client()
	if _, out := h.post(c, "/reset", "token", tokenIn(t, h.lastEmail()), "password", "brand new one"); out["error"] != ErrTwoStepRequired.Error() {
		t.Fatalf("reset: %v", out)
	}
	if code, _ := h.post(c, "/me"); code == 200 {
		t.Fatal("a reset link skipped the second step")
	}
	if code, _ := h.post(c, "/2fa/code", "code", totp(key, time.Now().Unix()/totpStep+1)); code != 200 {
		t.Fatal("code after reset")
	}
}

func TestSignInWith(t *testing.T) {
	h := newHarness(t)
	// A first sign-in creates an account without a password.
	gh := h.client()
	if code, out := h.post(gh, "/with", "provider", "github", "subject", "42", "email", "Octo@Example.com", "verified", "1"); code != 200 || out["email"] != "octo@example.com" || out["hasPassword"] != false || out["emailVerified"] != true {
		t.Fatalf("first: %d %v", code, out)
	}
	// Its only way to sign in can't be removed.
	if _, out := h.post(gh, "/unlink", "provider", "github"); out["error"] != ErrLastSignIn.Error() {
		t.Fatalf("unlink last: %v", out)
	}
	// Again: the same user, by its subject, whatever the email says now.
	again := h.client()
	if _, out := h.post(again, "/with", "provider", "github", "subject", "42", "email", "changed@example.com"); out["email"] != "octo@example.com" {
		t.Fatalf("again: %v", out)
	}

	// An existing password account joins only with a verified email.
	pw := h.client()
	h.post(pw, "/signup", "email", "ada@example.com", "password", "long enough")
	if _, out := h.post(h.client(), "/with", "provider", "google", "subject", "g1", "email", "ada@example.com"); out["error"] != ErrEmailUnverified.Error() {
		t.Fatalf("unverified: %v", out)
	}
	if _, out := h.post(h.client(), "/with", "provider", "google", "subject", "g1", "email", "ada@example.com", "verified", "1"); out["email"] != "ada@example.com" {
		t.Fatalf("verified join: %v", out)
	}
	// Signed in, a provider links to the current user, and only once.
	if _, out := h.post(pw, "/with", "provider", "gitlab", "subject", "x", "email", "other@example.com"); out["email"] != "ada@example.com" {
		t.Fatalf("link: %v", out)
	}
	if _, out := h.post(pw, "/with", "provider", "github", "subject", "42"); out["error"] != ErrIdentityTaken.Error() {
		t.Fatalf("taken: %v", out)
	}
	_, out := h.post(pw, "/identities")
	if list := out["list"].([]any); len(list) != 2 {
		t.Fatalf("identities: %v", out)
	}
	if code, _ := h.post(pw, "/unlink", "provider", "gitlab"); code != 200 {
		t.Fatal("unlink with a password left")
	}
	if _, out := h.post(h.client(), "/with", "provider", "x", "subject", "1"); out["error"] != ErrIdentityNoEmail.Error() {
		t.Fatalf("no email: %v", out)
	}
}

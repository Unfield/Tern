package accounts

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Unfield/Tern/kit"
)

// Passkeys (WebAuthn). The browser helpers in @ternkit/tern call the
// endpoints under Options.PasskeyPath: options to start, then the
// authenticator's answer to finish. Passkeys are discoverable
// credentials, so signing in needs no email. Attestation isn't asked
// for: the passkey proves it holds the key, not what device it is.

// Passkey is one of a user's passkeys.
type Passkey struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	CreatedAt time.Time  `json:"createdAt"`
	LastUsed  *time.Time `json:"lastUsed"`
}

const challengeFor = 5 * time.Minute

var b64 = base64.RawURLEncoding

// decodeB64 accepts base64url with or without padding.
func decodeB64(s string) ([]byte, error) { return b64.DecodeString(strings.TrimRight(s, "=")) }

// rp is the relying party: the app's host, and its origin.
func (a *Accounts) rp(c *kit.Ctx) (id, origin string) {
	if a.opts.BaseURL != "" {
		if u, err := url.Parse(a.opts.BaseURL); err == nil {
			return u.Hostname(), u.Scheme + "://" + u.Host
		}
	}
	host := c.Request.Host
	scheme := "https"
	if c.Request.TLS == nil && (strings.HasPrefix(host, "localhost") || strings.HasPrefix(host, "127.0.0.1")) {
		scheme = "http"
	}
	h := host
	if i := strings.LastIndex(h, ":"); i >= 0 && !strings.Contains(h[i:], "]") {
		h = h[:i]
	}
	return h, scheme + "://" + host
}

func (a *Accounts) passkeyEndpoints(s *kit.Setup) {
	p := strings.TrimSuffix(a.opts.PasskeyPath, "/")
	s.Endpoint("POST "+p+"/register/options", a.registerOptions)
	s.Endpoint("POST "+p+"/register", a.register)
	s.Endpoint("POST "+p+"/signin/options", a.signInOptions)
	s.Endpoint("POST "+p+"/signin", a.signIn)
}

func (a *Accounts) challenge(c *kit.Ctx, purpose string, userID int64) (string, error) {
	raw := make([]byte, 32)
	_, _ = rand.Read(raw)
	ch := b64.EncodeToString(raw)
	now := time.Now()
	_, err := a.exec(c.Request.Context(), `INSERT INTO tern_tokens (hash, purpose, user_id, data, created_at, expires_at) VALUES (?, ?, ?, ?, ?, ?)`,
		hashToken(ch), purpose, userID, "", now.UnixMilli(), now.Add(challengeFor).UnixMilli())
	return ch, err
}

func (a *Accounts) registerOptions(c *kit.Ctx) error {
	u := Current(c)
	if u == nil {
		return kit.Error(http.StatusUnauthorized, "sign in to add a passkey")
	}
	ch, err := a.challenge(c, "passkey-register", u.ID)
	if err != nil {
		return err
	}
	rpID, _ := a.rp(c)
	name := a.opts.RPName
	if name == "" {
		name = rpID
	}
	exclude := []map[string]any{}
	keys, err := a.passkeysOf(c, u.ID)
	if err != nil {
		return err
	}
	for _, k := range keys {
		exclude = append(exclude, map[string]any{"type": "public-key", "id": k.ID})
	}
	return c.JSON(http.StatusOK, map[string]any{
		"rp":                 map[string]any{"id": rpID, "name": name},
		"user":               map[string]any{"id": b64.EncodeToString([]byte(strconv.FormatInt(u.ID, 10))), "name": u.Email, "displayName": displayName(u)},
		"challenge":          ch,
		"pubKeyCredParams":   []map[string]any{{"type": "public-key", "alg": -7}, {"type": "public-key", "alg": -8}, {"type": "public-key", "alg": -257}},
		"timeout":            int(challengeFor / time.Millisecond),
		"attestation":        "none",
		"excludeCredentials": exclude,
		"authenticatorSelection": map[string]any{
			"residentKey": "required", "requireResidentKey": true, "userVerification": "preferred",
		},
	})
}

func displayName(u *User) string {
	if u.Name != "" {
		return u.Name
	}
	return u.Email
}

// credentialJSON is what the browser helpers post: a PublicKeyCredential's
// fields, binary ones in base64url.
type credentialJSON struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Response struct {
		ClientDataJSON    string `json:"clientDataJSON"`
		AttestationObject string `json:"attestationObject"`
		AuthenticatorData string `json:"authenticatorData"`
		Signature         string `json:"signature"`
		UserHandle        string `json:"userHandle"`
	} `json:"response"`
	Name     string `json:"name"`
	Remember bool   `json:"remember"`
}

type clientData struct {
	Type        string `json:"type"`
	Challenge   string `json:"challenge"`
	Origin      string `json:"origin"`
	CrossOrigin bool   `json:"crossOrigin"`
}

// checkClientData checks the browser's statement of what was signed, and
// uses up its challenge, returning the challenge's user.
func (a *Accounts) checkClientData(c *kit.Ctx, raw []byte, typ, purpose string) (int64, error) {
	var cd clientData
	if err := json.Unmarshal(raw, &cd); err != nil {
		return 0, ErrPasskey
	}
	_, origin := a.rp(c)
	if cd.Type != typ || cd.Origin != origin || cd.CrossOrigin {
		return 0, ErrPasskey
	}
	userID, _, err := a.redeem(c.Request.Context(), purpose, cd.Challenge)
	if err != nil {
		return 0, ErrPasskey
	}
	return userID, nil
}

// authData is the authenticator's data.
type authData struct {
	rpIDHash  []byte
	flags     byte
	signCount uint32
	credID    []byte
	key       []byte // the COSE public key, as CBOR
}

const (
	flagUP = 0x01 // the user was present
	flagAT = 0x40 // attested credential data follows
)

func parseAuthData(b []byte) (*authData, error) {
	if len(b) < 37 {
		return nil, ErrPasskey
	}
	ad := &authData{rpIDHash: b[:32], flags: b[32], signCount: binary.BigEndian.Uint32(b[33:37])}
	if ad.flags&flagAT != 0 {
		rest := b[37:]
		if len(rest) < 18 {
			return nil, ErrPasskey
		}
		n := int(binary.BigEndian.Uint16(rest[16:18]))
		if len(rest) < 18+n {
			return nil, ErrPasskey
		}
		ad.credID = rest[18 : 18+n]
		_, used, err := decodeCBOR(rest[18+n:])
		if err != nil {
			return nil, ErrPasskey
		}
		ad.key = rest[18+n : 18+n+used]
	}
	return ad, nil
}

func (a *Accounts) checkRP(c *kit.Ctx, ad *authData) error {
	rpID, _ := a.rp(c)
	want := sha256.Sum256([]byte(rpID))
	if !bytes.Equal(ad.rpIDHash, want[:]) || ad.flags&flagUP == 0 {
		return ErrPasskey
	}
	return nil
}

func (a *Accounts) register(c *kit.Ctx) error {
	u := Current(c)
	if u == nil {
		return kit.Error(http.StatusUnauthorized, "sign in to add a passkey")
	}
	var cred credentialJSON
	if err := json.NewDecoder(http.MaxBytesReader(nil, c.Request.Body, 64<<10)).Decode(&cred); err != nil {
		return kit.Error(http.StatusBadRequest, "invalid passkey response")
	}
	cdRaw, err1 := decodeB64(cred.Response.ClientDataJSON)
	attRaw, err2 := decodeB64(cred.Response.AttestationObject)
	if err1 != nil || err2 != nil {
		return kit.Error(http.StatusBadRequest, "invalid passkey response")
	}
	userID, err := a.checkClientData(c, cdRaw, "webauthn.create", "passkey-register")
	if err != nil || userID != u.ID {
		return kit.Error(http.StatusBadRequest, ErrPasskey.Error())
	}
	att, _, err := decodeCBOR(attRaw)
	m, ok := att.(map[any]any)
	if err != nil || !ok {
		return kit.Error(http.StatusBadRequest, ErrPasskey.Error())
	}
	adRaw, _ := m["authData"].([]byte)
	ad, err := parseAuthData(adRaw)
	if err != nil || a.checkRP(c, ad) != nil || ad.credID == nil {
		return kit.Error(http.StatusBadRequest, ErrPasskey.Error())
	}
	alg, err := coseAlg(ad.key)
	if err != nil {
		return kit.Error(http.StatusBadRequest, "this kind of passkey isn't supported")
	}
	name := strings.TrimSpace(cred.Name)
	if name == "" {
		name = describeDevice(c.Request.UserAgent())
	}
	_, err = a.exec(c.Request.Context(), `INSERT INTO tern_passkeys (id, user_id, public_key, alg, sign_count, name, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		b64.EncodeToString(ad.credID), u.ID, b64.EncodeToString(ad.key), alg, int64(ad.signCount), truncate(name, 100), time.Now().UnixMilli())
	if err != nil {
		return kit.Error(http.StatusConflict, "this passkey is already added")
	}
	return c.JSON(http.StatusOK, map[string]any{"ok": true})
}

func (a *Accounts) signInOptions(c *kit.Ctx) error {
	if err := a.limit(c, "passkey", ""); err != nil {
		return kit.Error(http.StatusTooManyRequests, ErrTooManyAttempts.Error())
	}
	ch, err := a.challenge(c, "passkey-signin", 0)
	if err != nil {
		return err
	}
	rpID, _ := a.rp(c)
	return c.JSON(http.StatusOK, map[string]any{
		"challenge": ch, "rpId": rpID, "timeout": int(challengeFor / time.Millisecond),
		"userVerification": "preferred", "allowCredentials": []any{},
	})
}

func (a *Accounts) signIn(c *kit.Ctx) error {
	var cred credentialJSON
	if err := json.NewDecoder(http.MaxBytesReader(nil, c.Request.Body, 64<<10)).Decode(&cred); err != nil {
		return kit.Error(http.StatusBadRequest, "invalid passkey response")
	}
	fail := func() error { return kit.Error(http.StatusUnauthorized, ErrPasskey.Error()) }
	cdRaw, err1 := decodeB64(cred.Response.ClientDataJSON)
	adRaw, err2 := decodeB64(cred.Response.AuthenticatorData)
	sig, err3 := decodeB64(cred.Response.Signature)
	if err1 != nil || err2 != nil || err3 != nil {
		return fail()
	}
	if _, err := a.checkClientData(c, cdRaw, "webauthn.get", "passkey-signin"); err != nil {
		return fail()
	}
	ad, err := parseAuthData(adRaw)
	if err != nil || a.checkRP(c, ad) != nil {
		return fail()
	}
	ctx := c.Request.Context()
	var userID, count int64
	var keyB64 string
	var alg int
	err = a.queryRow(ctx, `SELECT user_id, public_key, alg, sign_count FROM tern_passkeys WHERE id = ?`, strings.TrimRight(cred.ID, "=")).Scan(&userID, &keyB64, &alg, &count)
	if err != nil {
		return fail()
	}
	key, err := decodeB64(keyB64)
	if err != nil {
		return fail()
	}
	hash := sha256.Sum256(cdRaw)
	if !verifySignature(key, alg, append(append([]byte(nil), adRaw...), hash[:]...), sig) {
		return fail()
	}
	// A counter that doesn't go up means the passkey may have been cloned.
	if count > 0 && int64(ad.signCount) <= count {
		return fail()
	}
	_, _ = a.exec(ctx, `UPDATE tern_passkeys SET sign_count = ?, last_used = ? WHERE id = ?`, int64(ad.signCount), time.Now().UnixMilli(), strings.TrimRight(cred.ID, "="))
	u, err := a.UserByID(ctx, userID)
	if err != nil || u == nil {
		return fail()
	}
	if err := a.startSession(c, u, cred.Remember); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{"ok": true})
}

func (a *Accounts) passkeysOf(c *kit.Ctx, userID int64) ([]Passkey, error) {
	rows, err := a.query(c.Request.Context(), `SELECT id, name, created_at, last_used FROM tern_passkeys WHERE user_id = ? ORDER BY created_at`, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []Passkey{}
	for rows.Next() {
		var p Passkey
		var created int64
		var used *int64
		if err := rows.Scan(&p.ID, &p.Name, &created, &used); err != nil {
			return nil, err
		}
		p.CreatedAt = time.UnixMilli(created).UTC()
		if used != nil {
			t := time.UnixMilli(*used).UTC()
			p.LastUsed = &t
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Passkeys lists the signed-in user's passkeys.
func (a *Accounts) Passkeys(c *kit.Ctx) ([]Passkey, error) {
	u := Current(c)
	if u == nil {
		return nil, ErrSignedOut
	}
	return a.passkeysOf(c, u.ID)
}

// RemovePasskey deletes one of the signed-in user's passkeys.
func (a *Accounts) RemovePasskey(c *kit.Ctx, id string) error {
	u := Current(c)
	if u == nil {
		return ErrSignedOut
	}
	_, err := a.exec(c.Request.Context(), `DELETE FROM tern_passkeys WHERE id = ? AND user_id = ?`, id, u.ID)
	return err
}

// COSE keys (RFC 9053): the algorithms passkeys use.
const (
	algES256 = -7
	algEdDSA = -8
	algRS256 = -257
)

func coseKey(raw []byte) (map[any]any, error) {
	v, _, err := decodeCBOR(raw)
	m, ok := v.(map[any]any)
	if err != nil || !ok {
		return nil, ErrPasskey
	}
	return m, nil
}

func coseAlg(raw []byte) (int, error) {
	m, err := coseKey(raw)
	if err != nil {
		return 0, err
	}
	alg, _ := m[int64(3)].(int64)
	switch alg {
	case algES256, algEdDSA, algRS256:
		if _, err := publicKey(m, int(alg)); err != nil {
			return 0, err
		}
		return int(alg), nil
	}
	return 0, errors.New("unsupported algorithm")
}

func publicKey(m map[any]any, alg int) (crypto.PublicKey, error) {
	bytesAt := func(k int64) []byte { b, _ := m[k].([]byte); return b }
	switch alg {
	case algES256:
		x, y := bytesAt(-2), bytesAt(-3)
		if crv, _ := m[int64(-1)].(int64); crv != 1 || len(x) != 32 || len(y) != 32 {
			return nil, ErrPasskey
		}
		raw := append(append([]byte{4}, x...), y...)
		pub, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), raw)
		if err != nil {
			return nil, ErrPasskey
		}
		return pub, nil
	case algEdDSA:
		x := bytesAt(-2)
		if crv, _ := m[int64(-1)].(int64); crv != 6 || len(x) != ed25519.PublicKeySize {
			return nil, ErrPasskey
		}
		return ed25519.PublicKey(x), nil
	case algRS256:
		n, e := bytesAt(-1), bytesAt(-2)
		if len(n) < 256 || len(e) == 0 || len(e) > 4 {
			return nil, ErrPasskey // at least 2048 bits
		}
		return &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}, nil
	}
	return nil, ErrPasskey
}

func verifySignature(rawKey []byte, alg int, data, sig []byte) bool {
	m, err := coseKey(rawKey)
	if err != nil {
		return false
	}
	pub, err := publicKey(m, alg)
	if err != nil {
		return false
	}
	switch k := pub.(type) {
	case *ecdsa.PublicKey:
		h := sha256.Sum256(data)
		return ecdsa.VerifyASN1(k, h[:], sig)
	case ed25519.PublicKey:
		return ed25519.Verify(k, data, sig)
	case *rsa.PublicKey:
		h := sha256.Sum256(data)
		return rsa.VerifyPKCS1v15(k, crypto.SHA256, h[:], sig) == nil
	}
	return false
}

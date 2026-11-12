// Package sealed encrypts personal data in database columns: phone
// numbers, addresses, tokens of other services. A sealed.String is an
// ordinary string in Go and AES-256-GCM ciphertext in the database, so a
// copy of the database, a backup or a stray SELECT shows nothing.
//
//	sealed.SetKeys(s.FieldKey)                    // at start, from settings
//	db.ExecContext(ctx, `UPDATE users SET phone = ? WHERE id = ?`, sealed.String(phone), id)
//	var phone sealed.String
//	db.QueryRowContext(ctx, `SELECT phone FROM users WHERE id = ?`, id).Scan(&phone)
//
// Keys rotate: SetKeys takes the key to encrypt with and older ones that
// still decrypt; each value records which key sealed it. Encrypted values
// can't be searched; Blind gives a keyed hash to store beside one, for
// finding rows by an exact value.
package sealed

import (
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql/driver"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/Unfield/Tern/internal/seal"
	"github.com/Unfield/Tern/settings"
)

// prefix starts every sealed value: the format's version, then the key's
// ID and the ciphertext, separated by dots.
const prefix = "s1."

var (
	// ErrNoKeys is returned when values are sealed or opened before
	// SetKeys.
	ErrNoKeys = errors.New("sealed: no keys; call sealed.SetKeys when the app starts")
	// ErrUnknownKey is a value sealed with a key that isn't among the
	// keys, such as one dropped too early in a rotation.
	ErrUnknownKey = errors.New("sealed: the value was sealed with a key that isn't set")
	// ErrNotSealed is a value in the database that isn't encrypted; see
	// AllowPlain.
	ErrNotSealed = errors.New("sealed: the column holds a value that isn't sealed")
)

type key struct {
	id    string
	box   *seal.Box
	blind []byte
}

var (
	mu         sync.RWMutex
	keys       []key // keys[0] seals
	allowPlain bool
)

// SetKeys sets the key that seals values, and older keys that still open
// them, each at least 32 characters. Rotate by passing a new key first and
// the old one after it; once every row has been written again, drop it.
func SetKeys(primary settings.Secret, old ...settings.Secret) error {
	all := append([]settings.Secret{primary}, old...)
	ks := make([]key, 0, len(all))
	for _, s := range all {
		k, err := newKey(s.Reveal())
		if err != nil {
			return err
		}
		ks = append(ks, k)
	}
	mu.Lock()
	keys = ks
	mu.Unlock()
	return nil
}

// AllowPlain lets columns that aren't encrypted yet be read: a value
// without the sealed format is returned as it is, and sealed when it's
// written again. Use it while encrypting a column that has data.
func AllowPlain(allow bool) {
	mu.Lock()
	allowPlain = allow
	mu.Unlock()
}

func newKey(secret string) (key, error) {
	box, err := seal.New(secret, "sealed")
	if err != nil {
		return key{}, fmt.Errorf("sealed: %w", err)
	}
	blind, err := hkdf.Key(sha256.New, []byte(secret), nil, "tern/sealed/blind", 32)
	if err != nil {
		return key{}, err
	}
	sum := sha256.Sum256([]byte("tern/sealed/id:" + secret))
	return key{id: hex.EncodeToString(sum[:4]), box: box, blind: blind}, nil
}

// Seal encrypts plain with the current key.
func Seal(plain []byte) (string, error) {
	mu.RLock()
	defer mu.RUnlock()
	if len(keys) == 0 {
		return "", ErrNoKeys
	}
	return prefix + keys[0].id + "." + keys[0].box.Seal(plain), nil
}

// Open decrypts a value Seal made, with whichever key sealed it.
func Open(sealedValue string) ([]byte, error) {
	mu.RLock()
	defer mu.RUnlock()
	rest, ok := strings.CutPrefix(sealedValue, prefix)
	if !ok {
		if allowPlain {
			return []byte(sealedValue), nil
		}
		return nil, ErrNotSealed
	}
	if len(keys) == 0 {
		return nil, ErrNoKeys
	}
	id, ct, ok := strings.Cut(rest, ".")
	if !ok {
		return nil, errors.New("sealed: malformed value")
	}
	for _, k := range keys {
		if k.id == id {
			return k.box.Open(ct)
		}
	}
	return nil, ErrUnknownKey
}

// Current reports whether a stored value is sealed with the current key,
// to find the rows a rotation hasn't rewritten yet.
func Current(sealedValue string) bool {
	mu.RLock()
	defer mu.RUnlock()
	return len(keys) > 0 && strings.HasPrefix(sealedValue, prefix+keys[0].id+".")
}

// Blind is a keyed hash of value, to store in a column beside a sealed one
// and find rows by an exact value: WHERE email_hash = ?. It's the same for
// the same value, so it shows which rows share one, and nothing else.
// Rotating keys changes it: write the hashes again with the new key.
func Blind(value string) (string, error) {
	mu.RLock()
	defer mu.RUnlock()
	if len(keys) == 0 {
		return "", ErrNoKeys
	}
	m := hmac.New(sha256.New, keys[0].blind)
	m.Write([]byte(value))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil)), nil
}

// String is a string column stored sealed.
type String string

// Value implements driver.Valuer: the string, sealed.
func (s String) Value() (driver.Value, error) { return Seal([]byte(s)) }

// Scan implements sql.Scanner: the column, opened. NULL is "".
func (s *String) Scan(src any) error {
	b, err := scanned(src)
	if err != nil || b == nil {
		*s = ""
		return err
	}
	*s = String(b)
	return nil
}

// LogValue implements slog.LogValuer: personal data stays out of logs.
func (s String) LogValue() slog.Value { return slog.StringValue("[sealed]") }

// Bytes is a binary column stored sealed.
type Bytes []byte

// Value implements driver.Valuer: the bytes, sealed.
func (b Bytes) Value() (driver.Value, error) { return Seal(b) }

// Scan implements sql.Scanner: the column, opened. NULL is nil.
func (b *Bytes) Scan(src any) error {
	v, err := scanned(src)
	*b = v
	return err
}

// LogValue implements slog.LogValuer: personal data stays out of logs.
func (b Bytes) LogValue() slog.Value { return slog.StringValue("[sealed]") }

func scanned(src any) ([]byte, error) {
	switch v := src.(type) {
	case nil:
		return nil, nil
	case string:
		return Open(v)
	case []byte:
		return Open(string(v))
	}
	return nil, fmt.Errorf("sealed: can't read a %T column", src)
}

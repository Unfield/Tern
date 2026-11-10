// Package seal encrypts and authenticates small values, such as cookie
// contents, with AES-256-GCM. The key is derived from a secret, so any
// string of at least 32 bytes works.
package seal

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
)

// Box seals and opens values for one purpose.
type Box struct{ aead cipher.AEAD }

// New derives a key for purpose from secret. Values sealed for one purpose
// can't be opened for another.
func New(secret, purpose string) (*Box, error) {
	if len(secret) < 32 {
		return nil, errors.New("seal: the secret must be at least 32 bytes")
	}
	key, err := hkdf.Key(sha256.New, []byte(secret), nil, "tern/"+purpose, 32)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead: aead}, nil
}

// Seal encrypts plain into URL-safe text.
func (b *Box) Seal(plain []byte) string {
	nonce := make([]byte, b.aead.NonceSize())
	_, _ = rand.Read(nonce)
	return base64.RawURLEncoding.EncodeToString(b.aead.Seal(nonce, nonce, plain, nil))
}

// ErrInvalid is returned for values that were not sealed by this box or
// were changed.
var ErrInvalid = errors.New("seal: invalid value")

// Open decrypts a value from Seal.
func (b *Box) Open(sealed string) ([]byte, error) {
	raw, err := base64.RawURLEncoding.DecodeString(sealed)
	n := b.aead.NonceSize()
	if err != nil || len(raw) < n+b.aead.Overhead() {
		return nil, ErrInvalid
	}
	plain, err := b.aead.Open(nil, raw[:n], raw[n:], nil)
	if err != nil {
		return nil, ErrInvalid
	}
	return plain, nil
}

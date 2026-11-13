package accounts

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/bcrypt"
)

// Passwords are hashed with argon2id, with the parameters OWASP
// recommends. Hashes made with bcrypt, as apps often have, still verify,
// and are rehashed with argon2id at the next sign-in.

const (
	argonTime    = 2
	argonMemory  = 19 * 1024 // KiB
	argonThreads = 1
	argonKeyLen  = 32
)

// HashPassword hashes a password for storing.
func HashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	b64 := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, argonMemory, argonTime, argonThreads, b64.EncodeToString(salt), b64.EncodeToString(key)), nil
}

// CheckPassword reports whether password matches hash, and whether the
// hash should be replaced with a fresh HashPassword.
func CheckPassword(hash, password string) (ok, rehash bool) {
	switch {
	case strings.HasPrefix(hash, "$argon2id$"):
		var v, m, t, p int
		parts := strings.Split(hash, "$")
		if len(parts) != 6 {
			return false, false
		}
		if _, err := fmt.Sscanf(parts[2], "v=%d", &v); err != nil {
			return false, false
		}
		if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
			return false, false
		}
		salt, err1 := base64.RawStdEncoding.DecodeString(parts[4])
		want, err2 := base64.RawStdEncoding.DecodeString(parts[5])
		if err1 != nil || err2 != nil || len(want) == 0 || t < 1 || p < 1 || p > 255 || m < 8 {
			return false, false
		}
		got := argon2.IDKey([]byte(password), salt, uint32(t), uint32(m), uint8(p), uint32(len(want)))
		ok := subtle.ConstantTimeCompare(got, want) == 1
		return ok, ok && (m != argonMemory || t != argonTime || p != argonThreads || len(want) != argonKeyLen)
	case strings.HasPrefix(hash, "$2"):
		err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
		return err == nil, err == nil
	}
	return false, false
}

// dummyHash keeps a sign-in for an unknown email as slow as for a known
// one, so timing doesn't reveal accounts. It's made on first use: hashing
// takes tens of milliseconds and 64 MB, which every start of the app (and
// of a phone app) would otherwise pay.
var dummyHash = sync.OnceValue(func() string {
	h, _ := HashPassword("tern: a password nobody has")
	return h
})

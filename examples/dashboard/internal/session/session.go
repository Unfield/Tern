// Package session stores the signed-in user's name in a signed cookie.
package session

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"os"
	"strings"

	"github.com/Unfield/Tern/kit"
)

const cookieName = "session"

var secret = loadSecret()

// loadSecret reads SESSION_SECRET, or generates a random secret so
// sessions last until the server restarts.
func loadSecret() []byte {
	if s := os.Getenv("SESSION_SECRET"); s != "" {
		return []byte(s)
	}
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return b
}

func sign(value string) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(value))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// Hook reads the session cookie and stores the user in ctx.Locals["user"].
func Hook(ctx *kit.Ctx) error {
	raw, ok := ctx.Cookie(cookieName)
	if !ok {
		return nil
	}
	encoded, sig, ok := strings.Cut(raw, ".")
	if !ok || !hmac.Equal([]byte(sig), []byte(sign(encoded))) {
		return nil
	}
	name, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil //nolint:nilerr // an invalid cookie means signed out
	}
	ctx.Locals["user"] = string(name)
	return nil
}

// User returns the signed-in user, or "".
func User(ctx *kit.Ctx) string {
	u, _ := ctx.Locals["user"].(string)
	return u
}

// SignIn sets the session cookie.
func SignIn(ctx *kit.Ctx, name string) {
	encoded := base64.RawURLEncoding.EncodeToString([]byte(name))
	ctx.SetCookie(&http.Cookie{
		Name:     cookieName,
		Value:    encoded + "." + sign(encoded),
		HttpOnly: true,
		MaxAge:   60 * 60 * 24 * 30,
	})
}

// SignOut clears the session cookie.
func SignOut(ctx *kit.Ctx) { ctx.DeleteCookie(cookieName) }

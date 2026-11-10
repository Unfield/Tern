// Package sessions stores per-user data in an encrypted cookie:
//
//	ternApp(kit.WithPlugins(sessions.New(os.Getenv("SESSION_SECRET"))))
//
//	func Load(ctx *kit.Ctx) (Data, error) {
//		s := sessions.Get(ctx)
//		user := s.GetString("user")
//		notice := s.Flash("notice") // read once
//		...
//	}
//
// Values are JSON-encoded and sealed with AES-GCM, so users can neither
// read nor change them. Keep sessions small: cookies are limited to about
// 4 KB. Changes are saved automatically.
package sessions

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/Unfield/Tern/kit"
)

const localsKey = "tern.sessions"

// Option configures the plugin.
type Option func(*Plugin)

// CookieName changes the cookie from "session".
func CookieName(name string) Option { return func(p *Plugin) { p.cookie = name } }

// MaxAge sets how long sessions last. The default is 30 days.
func MaxAge(d time.Duration) Option { return func(p *Plugin) { p.maxAge = d } }

// OldSecrets keeps sessions sealed with previous secrets readable, so the
// secret can be rotated without signing everyone out.
func OldSecrets(secrets ...string) Option {
	return func(p *Plugin) { p.old = append(p.old, secrets...) }
}

// Plugin loads and saves sessions. Create it with New.
type Plugin struct {
	secret string
	old    []string
	cookie string
	maxAge time.Duration
	aeads  []cipher.AEAD // the first seals; all open
}

// New returns the sessions plugin. secret must be at least 32 characters
// and stay the same across restarts and servers.
func New(secret string, opts ...Option) *Plugin {
	p := &Plugin{secret: secret, cookie: "session", maxAge: 30 * 24 * time.Hour}
	for _, o := range opts {
		o(p)
	}
	return p
}

// Name implements kit.Plugin.
func (*Plugin) Name() string { return "sessions" }

// Setup implements kit.Plugin.
func (p *Plugin) Setup(s *kit.Setup) error {
	if len(p.secret) < 32 {
		return errors.New("the secret must be at least 32 characters")
	}
	for _, sec := range append([]string{p.secret}, p.old...) {
		key := sha256.Sum256([]byte(sec))
		block, err := aes.NewCipher(key[:])
		if err != nil {
			return err
		}
		aead, err := cipher.NewGCM(block)
		if err != nil {
			return err
		}
		p.aeads = append(p.aeads, aead)
	}
	s.Hook(func(c *kit.Ctx) error {
		sess := &Session{p: p, ctx: c, values: map[string]json.RawMessage{}}
		if raw, ok := c.Cookie(p.cookie); ok {
			sess.values = p.open(raw)
		}
		c.Locals[localsKey] = sess
		return nil
	})
	return nil
}

func (p *Plugin) seal(values map[string]json.RawMessage) (string, error) {
	plain, err := json.Marshal(values)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, p.aeads[0].NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := p.aeads[0].Seal(nonce, nonce, plain, []byte(p.cookie))
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

func (p *Plugin) open(raw string) map[string]json.RawMessage {
	data, err := base64.RawURLEncoding.DecodeString(raw)
	if err == nil {
		for _, aead := range p.aeads {
			n := aead.NonceSize()
			if len(data) <= n {
				break
			}
			if plain, err := aead.Open(nil, data[:n], data[n:], []byte(p.cookie)); err == nil {
				var values map[string]json.RawMessage
				if json.Unmarshal(plain, &values) == nil && values != nil {
					return values
				}
			}
		}
	}
	return map[string]json.RawMessage{} // missing, tampered with or expired: start fresh
}

// Session is one user's session data.
type Session struct {
	p      *Plugin
	ctx    *kit.Ctx
	mu     sync.Mutex
	values map[string]json.RawMessage
}

// Get returns the request's session. It panics if the sessions plugin is
// not installed, which is a programming error.
func Get(ctx *kit.Ctx) *Session {
	s, ok := ctx.Locals[localsKey].(*Session)
	if !ok {
		panic("sessions.Get: add sessions.New(...) with kit.WithPlugins")
	}
	return s
}

// Value decodes the value stored at key into dst and reports whether it was set.
func (s *Session) Value(key string, dst any) bool {
	s.mu.Lock()
	raw, ok := s.values[key]
	s.mu.Unlock()
	return ok && json.Unmarshal(raw, dst) == nil
}

// GetString returns the string stored at key, or "".
func (s *Session) GetString(key string) string {
	var v string
	s.Value(key, &v)
	return v
}

// Set stores v (any JSON-encodable value) at key.
func (s *Session) Set(key string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("sessions: encoding %q: %w", key, err)
	}
	s.mu.Lock()
	s.values[key] = raw
	s.mu.Unlock()
	return s.save()
}

// Delete removes key.
func (s *Session) Delete(key string) error {
	s.mu.Lock()
	delete(s.values, key)
	s.mu.Unlock()
	return s.save()
}

// Clear removes everything, for example when signing out.
func (s *Session) Clear() {
	s.mu.Lock()
	s.values = map[string]json.RawMessage{}
	s.mu.Unlock()
	s.ctx.DeleteCookie(s.p.cookie)
}

// AddFlash stores a message to show once, typically after a redirect.
func (s *Session) AddFlash(key, message string) error { return s.Set("_flash."+key, message) }

// Flash returns a message stored with AddFlash and removes it.
func (s *Session) Flash(key string) string {
	v := s.GetString("_flash." + key)
	if v != "" {
		_ = s.Delete("_flash." + key)
	}
	return v
}

func (s *Session) save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.values) == 0 {
		s.ctx.DeleteCookie(s.p.cookie)
		return nil
	}
	v, err := s.p.seal(s.values)
	if err != nil {
		return err
	}
	if len(v) > 3800 {
		return fmt.Errorf("sessions: the session is %d bytes, too large for a cookie; store an ID and keep the data on the server", len(v))
	}
	s.ctx.SetCookie(&http.Cookie{Name: s.p.cookie, Value: v, HttpOnly: true, MaxAge: int(s.p.maxAge.Seconds())})
	return nil
}

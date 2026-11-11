package accounts

import (
	"errors"
	"sync"
	"time"

	"github.com/Unfield/Tern/kit"
)

// ErrTooManyAttempts is returned when an address or a client has tried
// too often: guessing passwords or codes, or sending emails.
var ErrTooManyAttempts = errors.New("accounts: too many attempts, try again later")

// Each kind of attempt is limited per account (its email address or id)
// and per client IP, over a window. The counts are kept in memory, so with
// several processes each counts its own.
type rule struct {
	perKey, perIP int
	window        time.Duration
}

var rules = map[string]rule{
	"signin":  {perKey: 10, perIP: 50, window: 15 * time.Minute},
	"signup":  {perIP: 20, window: time.Hour},
	"email":   {perKey: 5, perIP: 30, window: time.Hour}, // verification, reset and email change emails
	"passkey": {perIP: 50, window: 15 * time.Minute},
	"code":    {perKey: 6, perIP: 50, window: 15 * time.Minute}, // two-step codes
	"invite":  {perKey: 50, perIP: 100, window: time.Hour},
}

type limiter struct {
	mu    sync.Mutex
	hits  map[string][]time.Time
	sweep time.Time
}

func newLimiter() *limiter { return &limiter{hits: map[string][]time.Time{}} }

// allow records an attempt for key and reports whether it's within n per
// window.
func (l *limiter) allow(key string, n int, window time.Duration, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if now.Sub(l.sweep) > window {
		l.sweep = now
		for k, ts := range l.hits {
			if len(ts) == 0 || now.Sub(ts[len(ts)-1]) > time.Hour {
				delete(l.hits, k)
			}
		}
	}
	ts := l.hits[key]
	i := 0
	for i < len(ts) && now.Sub(ts[i]) > window {
		i++
	}
	ts = ts[i:]
	if len(ts) >= n {
		l.hits[key] = ts
		return false
	}
	l.hits[key] = append(ts, now)
	return true
}

func (l *limiter) reset(key string) {
	l.mu.Lock()
	delete(l.hits, key)
	l.mu.Unlock()
}

// limit counts an attempt of kind for key (an email or user id, or ""),
// and fails with ErrTooManyAttempts past the rule.
func (a *Accounts) limit(c *kit.Ctx, kind, key string) error {
	if a.opts.NoRateLimits {
		return nil
	}
	r := rules[kind]
	now := time.Now()
	if r.perIP > 0 && !a.limits.allow(kind+"\x00ip\x00"+c.ClientIP(), r.perIP, r.window, now) {
		return ErrTooManyAttempts
	}
	if key != "" && r.perKey > 0 && !a.limits.allow(kind+"\x00"+key, r.perKey, r.window, now) {
		return ErrTooManyAttempts
	}
	return nil
}

// succeeded clears an account's failed attempts of kind.
func (a *Accounts) succeeded(kind, key string) { a.limits.reset(kind + "\x00" + key) }

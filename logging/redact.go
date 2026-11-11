package logging

import (
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
)

// Redacted is what a redacted value is replaced with.
const Redacted = "[redacted]"

// Secret is a string that never shows in logs: its value logs as
// [Redacted], whatever its key and whatever the handler.
//
//	logger.Info("calling the API", "key", logging.Secret(key))
type Secret string

// LogValue implements slog.LogValuer.
func (Secret) LogValue() slog.Value { return slog.StringValue(Redacted) }

// String keeps the value out of fmt output too.
func (Secret) String() string { return Redacted }

// DefaultRedact is the list of keys whose values are redacted unless
// [Options.Keep] names them. Keys match without regard to case, with "-"
// and "_" the same, and so do keys ending in "_token", "_secret",
// "_password" or "_key" after one of these words: "access_token" and
// "stripe_secret" are redacted, "cache_key" is not.
var DefaultRedact = []string{
	"password", "passwd", "new_password", "current_password",
	"secret", "client_secret",
	"token", "access_token", "refresh_token", "id_token", "bearer",
	"api_key", "apikey", "private_key", "secret_key", "signing_key",
	"authorization", "proxy_authorization",
	"cookie", "set_cookie",
	"session", "session_id", "sessionid",
	"csrf", "csrf_token", "xsrf_token",
	"otp", "totp", "recovery_code", "recovery_codes",
	"credit_card", "card_number", "cvv", "cvc",
}

// redactor decides which keys to redact.
type redactor struct {
	keys map[string]bool
	keep map[string]bool
}

func newRedactor(extra, keep []string, noDefaults bool) *redactor {
	r := &redactor{keys: map[string]bool{}, keep: map[string]bool{}}
	if !noDefaults {
		for _, k := range DefaultRedact {
			r.keys[normKey(k)] = true
		}
	}
	for _, k := range extra {
		r.keys[normKey(k)] = true
	}
	for _, k := range keep {
		r.keep[normKey(k)] = true
	}
	return r
}

func normKey(k string) string {
	return strings.ReplaceAll(strings.ToLower(k), "-", "_")
}

var redactSuffixes = []string{"_token", "_secret", "_password"}

// match reports whether values under key are redacted.
func (r *redactor) match(key string) bool {
	if r == nil || key == "" {
		return false
	}
	k := normKey(key)
	if r.keep[k] {
		return false
	}
	if r.keys[k] {
		return true
	}
	for _, s := range redactSuffixes {
		if strings.HasSuffix(k, s) && len(k) > len(s) {
			return true
		}
	}
	// "x_key" where x is itself a redacted word: "stripe_secret_key".
	if before, ok := strings.CutSuffix(k, "_key"); ok {
		for _, s := range []string{"secret", "private", "api", "signing"} {
			if strings.HasSuffix(before, s) {
				return true
			}
		}
	}
	return false
}

// clean resolves a, redacts it and its members as r says, and drops what
// slog drops: attributes with no key and an empty value, and empty groups.
// It appends the result to dst; a group with no key adds its members.
func (r *redactor) clean(dst []slog.Attr, a slog.Attr) []slog.Attr {
	a.Value = a.Value.Resolve()
	if a.Key == "" && a.Value.Kind() != slog.KindGroup && a.Value.Equal(slog.Value{}) {
		return dst
	}
	if r.match(a.Key) {
		return append(dst, slog.String(a.Key, Redacted))
	}
	switch a.Value.Kind() {
	case slog.KindGroup:
		var members []slog.Attr
		for _, m := range a.Value.Group() {
			members = r.clean(members, m)
		}
		if len(members) == 0 {
			return dst
		}
		if a.Key == "" {
			return append(dst, members...)
		}
		return append(dst, slog.Attr{Key: a.Key, Value: slog.GroupValue(members...)})
	case slog.KindAny:
		switch v := a.Value.Any().(type) {
		case http.Header:
			return append(dst, slog.Attr{Key: a.Key, Value: r.header(v)})
		case *url.URL:
			if v != nil {
				return append(dst, slog.String(a.Key, r.url(v)))
			}
		case url.URL:
			return append(dst, slog.String(a.Key, r.url(&v)))
		case url.Values:
			return append(dst, slog.Attr{Key: a.Key, Value: r.values(v)})
		}
	}
	return append(dst, a)
}

// header logs h as a group, one member per header, with the values of
// redacted ones replaced.
func (r *redactor) header(h http.Header) slog.Value {
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	attrs := make([]slog.Attr, 0, len(keys))
	for _, k := range keys {
		if r.match(k) {
			attrs = append(attrs, slog.String(k, Redacted))
			continue
		}
		attrs = append(attrs, slog.String(k, strings.Join(h[k], ", ")))
	}
	return slog.GroupValue(attrs...)
}

func (r *redactor) values(v url.Values) slog.Value {
	keys := make([]string, 0, len(v))
	for k := range v {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	attrs := make([]slog.Attr, 0, len(keys))
	for _, k := range keys {
		if r.match(k) {
			attrs = append(attrs, slog.String(k, Redacted))
			continue
		}
		attrs = append(attrs, slog.String(k, strings.Join(v[k], ", ")))
	}
	return slog.GroupValue(attrs...)
}

// url is u with the user's password and redacted query parameters
// replaced; query parameters are re-encoded when one is.
func (r *redactor) url(u *url.URL) string {
	c := *u
	if c.RawQuery != "" {
		q, err := url.ParseQuery(c.RawQuery)
		if err == nil {
			changed := false
			for k := range q {
				if r.match(k) {
					q[k] = []string{Redacted}
					changed = true
				}
			}
			if changed {
				// Brackets read better than %5B…%5D, and logs aren't URLs
				// to follow.
				c.RawQuery = strings.ReplaceAll(q.Encode(), url.QueryEscape(Redacted), Redacted)
			}
		}
	}
	return c.Redacted() // the password, if any, as xxxxx
}

// defaultRedactor redacts DefaultRedact, for handlers made by Wrap.
var defaultRedactor = newRedactor(nil, nil, false)

// RedactURL returns u as a string with its password and the values of
// query parameters [DefaultRedact] names replaced, as log records have
// URLs.
func RedactURL(u *url.URL) string { return defaultRedactor.url(u) }

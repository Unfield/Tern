package kit

import (
	"math"
	"net/http"
	"net/netip"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/Unfield/Tern/internal/ratelimit"
)

// DefaultContentSecurityPolicy is the Content-Security-Policy pages are sent
// unless the app sets its own (WithContentSecurityPolicy) or none
// (WithoutContentSecurityPolicy); see ADR 0027. Only the app's own scripts,
// and inline scripts with the request's nonce, run.
//
// The one inline event handler allowed is Svelte's: server rendering puts
// onload="this.__e=event" and onerror="this.__e=event" on elements whose
// load and error events must not be lost before hydration. 'unsafe-hashes'
// with its hash allows that exact text and no other handler.
const DefaultContentSecurityPolicy = "default-src 'self'; script-src 'self' 'unsafe-hashes' '" + svelteEventHash + "'; style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data: blob: https:; font-src 'self' data:; connect-src 'self'; object-src 'none'; base-uri 'self'"

// svelteEventHash is the CSP hash of Svelte's inline event handler,
// "this.__e=event".
const svelteEventHash = "sha256-7dQwUgLau1NFCCGjfn9FsYptB6ZtWxJin6VohGIu20I="

// securityHeaders are sent with every response unless the app opts out
// with WithoutSecurityHeaders. plugins/secure changes them.
var securityHeaders = [][2]string{
	{"X-Frame-Options", "DENY"},
	{"Referrer-Policy", "strict-origin-when-cross-origin"},
	{"Permissions-Policy", "camera=(), microphone=(), geolocation=(), interest-cohort=()"},
	{"Cross-Origin-Opener-Policy", "same-origin"},
	{"X-Permitted-Cross-Domain-Policies", "none"},
}

// WithoutSecurityHeaders stops sending the security headers Tern sends by
// default: X-Frame-Options, Referrer-Policy, Permissions-Policy,
// Cross-Origin-Opener-Policy and X-Permitted-Cross-Domain-Policies. To
// change one instead, use plugins/secure.
func WithoutSecurityHeaders() Option {
	return func(o *options) { o.noSecHeaders = true }
}

// WithoutContentSecurityPolicy sends pages without a
// Content-Security-Policy. Injected markup can then run scripts; prefer
// WithCSPSources to allow what a page needs.
func WithoutContentSecurityPolicy() Option {
	return func(o *options) { o.csp, o.cspReportOnly, o.cspSet = "", false, true }
}

// WithCSPSources allows more sources for one directive of the
// Content-Security-Policy, such as an analytics script:
//
//	kit.WithCSPSources("script-src", "https://plausible.io")
//	kit.WithCSPSources("frame-src", "https://www.youtube-nocookie.com")
//
// A directive the policy doesn't have is created from default-src's
// sources and these.
func WithCSPSources(directive string, sources ...string) Option {
	return func(o *options) {
		o.cspSources = append(o.cspSources, append([]string{strings.ToLower(directive)}, sources...))
	}
}

// resolveCSP is the policy the app sends, from its options.
func resolveCSP(o *options) string {
	policy := o.csp
	if !o.cspSet {
		policy = DefaultContentSecurityPolicy
	}
	if policy == "" {
		return ""
	}
	for _, src := range o.cspSources {
		policy = addSources(policy, src[0], src[1:])
	}
	return policy
}

// addSources adds sources to a directive of policy, creating it from
// default-src's sources when it's missing.
func addSources(policy, directive string, sources []string) string {
	parts := trimAll(strings.Split(policy, ";"))
	var defaultSrc []string
	for i, d := range parts {
		fields := strings.Fields(d)
		switch strings.ToLower(fields[0]) {
		case directive:
			for _, s := range sources {
				if !slices.Contains(fields[1:], s) {
					fields = append(fields, s)
				}
			}
			parts[i] = strings.Join(fields, " ")
			return strings.Join(parts, "; ")
		case "default-src":
			defaultSrc = fields[1:]
		}
	}
	if len(defaultSrc) == 0 {
		defaultSrc = []string{"'self'"}
	}
	fields := append([]string{directive}, slices.Clone(defaultSrc)...)
	for _, s := range sources {
		if !slices.Contains(fields[1:], s) {
			fields = append(fields, s)
		}
	}
	return strings.Join(append(parts, strings.Join(fields, " ")), "; ")
}

// secureHeaders sets the default security headers on every response.
func secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		for _, kv := range securityHeaders {
			h.Set(kv[0], kv[1])
		}
		next.ServeHTTP(w, r)
	})
}

// scriptTag matches the start of a <script> element, to give inline
// scripts in <svelte:head> the request's nonce.
var scriptTag = regexp.MustCompile(`(?i)<script\b`)

// scriptType reads a <script> tag's type attribute.
var scriptType = regexp.MustCompile(`\btype\s*=\s*["']?([^"'\s>]+)`)

// executable reports whether a <script> tag's attributes (lowercased) make
// it code: no type, a JavaScript type or a module. JSON data needs no nonce.
func executable(attrs string) bool {
	m := scriptType.FindStringSubmatch(attrs)
	if m == nil {
		return true
	}
	switch m[1] {
	case "module", "text/javascript", "application/javascript":
		return true
	}
	return false
}

// stampNonce puts nonce on the <script> elements in head that have none.
// Only the head: it's the app's own markup, where the body may hold
// content from users, put there with {@html}.
func stampNonce(head, nonce string) string {
	if !strings.Contains(head, "<script") && !strings.Contains(head, "<SCRIPT") {
		return head
	}
	attr := ` nonce="` + nonce + `"`
	var b strings.Builder
	last := 0
	for _, loc := range scriptTag.FindAllStringIndex(head, -1) {
		end := strings.IndexByte(head[loc[1]:], '>')
		if end < 0 {
			break
		}
		tag := strings.ToLower(head[loc[1] : loc[1]+end])
		if strings.Contains(tag, "nonce=") || !executable(tag) {
			continue
		}
		b.WriteString(head[last:loc[1]])
		b.WriteString(attr)
		last = loc[1]
	}
	if last == 0 {
		return head
	}
	b.WriteString(head[last:])
	return b.String()
}

// The default rate limit: state-changing requests, per client.
const (
	defaultRatePerMinute = 120
	defaultRateBurst     = 60
)

// WithoutRateLimit turns off the default limit on state-changing requests
// (POST, PUT, PATCH and DELETE): 120 a minute per client, in bursts of 60.
// To change the limit instead, add plugins/ratelimit, which replaces it.
func WithoutRateLimit() Option {
	return func(o *options) { o.noRateLimit = true }
}

// rateLimit is the default limit on state-changing requests. Clients on
// loopback addresses aren't limited: they're the developer, or a proxy on
// the same machine that isn't trusted, which would make every client one.
func (a *App) rateLimit(next http.Handler) http.Handler {
	l := ratelimit.New(defaultRatePerMinute/60.0, defaultRateBurst)
	go l.Sweep(a.shutdownCtx.Done())
	var warned atomic.Bool
	trusting := len(a.opts.trustedProxies) > 0
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		default:
			next.ServeHTTP(w, r)
			return
		}
		ip := ClientIP(r)
		if addr, err := netip.ParseAddr(ip); err == nil && addr.IsLoopback() {
			next.ServeHTTP(w, r)
			return
		}
		if !trusting && r.Header.Get("X-Forwarded-For") != "" && warned.CompareAndSwap(false, true) {
			a.logger.Warn("tern: requests come through a proxy that isn't trusted, so every client looks like one to the rate limit; name it with kit.WithTrustedProxies",
				"proxy", ip)
		}
		if wait, ok := l.Take(ip); !ok {
			w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(wait.Seconds()))))
			http.Error(w, "Too many requests; try again later.", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

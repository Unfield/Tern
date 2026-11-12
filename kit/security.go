package kit

import (
	"net/http"
	"regexp"
	"slices"
	"strings"
)

// DefaultContentSecurityPolicy is the Content-Security-Policy pages are sent
// unless the app sets its own (WithContentSecurityPolicy) or none
// (WithoutContentSecurityPolicy); see ADR 0027. Only the app's own scripts,
// and inline scripts with the request's nonce, run.
const DefaultContentSecurityPolicy = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data: blob: https:; font-src 'self' data:; connect-src 'self'; object-src 'none'; base-uri 'self'"

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
		tag := head[loc[1] : loc[1]+end]
		if strings.Contains(strings.ToLower(tag), "nonce=") {
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

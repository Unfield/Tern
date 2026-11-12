package kit

import (
	"net/http"
	"strings"
	"testing"
)

func TestSecureByDefault(t *testing.T) {
	a, _ := newTestApp(t, testManifest())
	rec := do(t, a, "GET", "/", nil)
	for _, kv := range securityHeaders {
		if got := rec.Header().Get(kv[0]); got != kv[1] {
			t.Errorf("%s = %q, want %q", kv[0], got, kv[1])
		}
	}
	csp := rec.Header().Get("Content-Security-Policy")
	if !strings.HasPrefix(csp, "default-src 'self'; script-src 'self' 'nonce-") || !strings.Contains(csp, "object-src 'none'") {
		t.Errorf("Content-Security-Policy = %q", csp)
	}
	// Endpoints get the headers too, but no policy: they aren't pages.
	api := do(t, a, "GET", "/api/items/1", nil)
	if api.Header().Get("X-Frame-Options") != "DENY" || api.Header().Get("Content-Security-Policy") != "" {
		t.Errorf("endpoint headers: %v", api.Header())
	}
}

func TestSecurityOptOuts(t *testing.T) {
	a, _ := newTestApp(t, testManifest(), WithoutSecurityHeaders(), WithoutContentSecurityPolicy())
	rec := do(t, a, "GET", "/", nil)
	if rec.Header().Get("X-Frame-Options") != "" || rec.Header().Get("Content-Security-Policy") != "" {
		t.Errorf("opted out, but got %v", rec.Header())
	}
}

func TestCSPSources(t *testing.T) {
	a, _ := newTestApp(t, testManifest(),
		WithCSPSources("script-src", "https://plausible.io"),
		WithCSPSources("frame-src", "https://www.youtube-nocookie.com"),
		WithCSPSources("img-src", "https:"), // already there
	)
	csp := do(t, a, "GET", "/", nil).Header().Get("Content-Security-Policy")
	for _, want := range []string{"script-src 'self' https://plausible.io 'nonce-", "frame-src 'self' https://www.youtube-nocookie.com", "img-src 'self' data: blob: https:;"} {
		if !strings.Contains(csp, want) {
			t.Errorf("policy %q lacks %q", csp, want)
		}
	}
	// With a policy of the app's own, the sources are added to it.
	b, _ := newTestApp(t, testManifest(), WithContentSecurityPolicy("default-src 'none'"), WithCSPSources("img-src", "https://cdn.example"))
	if csp := do(t, b, "GET", "/", nil).Header().Get("Content-Security-Policy"); !strings.HasPrefix(csp, "default-src 'none'; img-src 'none' https://cdn.example; script-src 'none' 'nonce-") {
		t.Errorf("policy = %q", csp)
	}
}

func TestStampNonce(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{`<title>x</title>`, `<title>x</title>`},
		{`<script>a()</script>`, `<script nonce="N">a()</script>`},
		{`<script nonce="M">a()</script><SCRIPT type="module">b()</SCRIPT>`, `<script nonce="M">a()</script><SCRIPT nonce="N" type="module">b()</SCRIPT>`},
		{`<scripts>`, `<scripts>`},
		{`<script`, `<script`},
	} {
		if got := stampNonce(c.in, "N"); got != c.want {
			t.Errorf("stampNonce(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSecurePluginChangesDefaults(t *testing.T) {
	// plugins/secure runs inside the defaults; a middleware stands in
	// for it here.
	a, _ := newTestApp(t, testManifest(), WithMiddleware(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Frame-Options", "SAMEORIGIN")
			w.Header().Del("Permissions-Policy")
			next.ServeHTTP(w, r)
		})
	}))
	rec := do(t, a, "GET", "/", nil)
	if rec.Header().Get("X-Frame-Options") != "SAMEORIGIN" || rec.Header().Get("Permissions-Policy") != "" {
		t.Errorf("headers = %v", rec.Header())
	}
}

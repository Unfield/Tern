package kit

import (
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
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
	if !strings.HasPrefix(csp, "default-src 'self'; script-src 'self' 'unsafe-hashes' '"+svelteEventHash+"' 'nonce-") || !strings.Contains(csp, "object-src 'none'") {
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
	for _, want := range []string{"script-src 'self' 'unsafe-hashes' '" + svelteEventHash + "' https://plausible.io 'nonce-", "frame-src 'self' https://www.youtube-nocookie.com", "img-src 'self' data: blob: https:;"} {
		if !strings.Contains(csp, want) {
			t.Errorf("policy %q lacks %q", csp, want)
		}
	}
	// With a policy of the app's own, the sources are added to it.
	b, _ := newTestApp(t, testManifest(), WithContentSecurityPolicy("default-src 'none'"), WithCSPSources("img-src", "https://cdn.example"))
	if csp := do(t, b, "GET", "/", nil).Header().Get("Content-Security-Policy"); !strings.HasPrefix(csp, "default-src 'none'; img-src 'none' https://cdn.example; script-src 'none' 'unsafe-hashes' '"+svelteEventHash+"' 'nonce-") {
		t.Errorf("policy = %q", csp)
	}
}

func TestStampNonce(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{`<title>x</title>`, `<title>x</title>`},
		{`<script>a()</script>`, `<script nonce="N">a()</script>`},
		{`<script nonce="M">a()</script><SCRIPT type="module">b()</SCRIPT>`, `<script nonce="M">a()</script><SCRIPT nonce="N" type="module">b()</SCRIPT>`},
		{`<scripts>`, `<scripts>`},
		{`<script type="application/json" id="x">{}</script><script type=module>m()</script>`, `<script type="application/json" id="x">{}</script><script nonce="N" type=module>m()</script>`},
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

func TestDefaultRateLimit(t *testing.T) {
	post := func(a *App, remote string, headers ...string) int {
		req := httptest.NewRequest(http.MethodPost, "/todos?/clear", strings.NewReader(""))
		req.RemoteAddr = remote
		for i := 0; i+1 < len(headers); i += 2 {
			req.Header.Set(headers[i], headers[i+1])
		}
		rec := httptest.NewRecorder()
		a.ServeHTTP(rec, req)
		return rec.Code
	}
	a, logs := newTestApp(t, testManifest())
	for i := range defaultRateBurst {
		if code := post(a, "203.0.113.7:1234"); code == http.StatusTooManyRequests {
			t.Fatalf("request %d was limited", i+1)
		}
	}
	if code := post(a, "203.0.113.7:1234"); code != http.StatusTooManyRequests {
		t.Errorf("past the burst: %d, want 429", code)
	}
	if code := post(a, "203.0.113.8:1234"); code == http.StatusTooManyRequests {
		t.Error("another client was limited")
	}
	// GETs, and loopback clients, aren't limited.
	if code := do(t, a, "GET", "/todos", nil).Code; code != http.StatusOK {
		t.Errorf("GET: %d", code)
	}
	for range defaultRateBurst + 5 {
		if code := post(a, "127.0.0.1:1234"); code == http.StatusTooManyRequests {
			t.Fatal("a loopback client was limited")
		}
	}
	// A proxy that isn't trusted is pointed out, once.
	post(a, "198.51.100.1:1234", "X-Forwarded-For", "203.0.113.9")
	post(a, "198.51.100.1:1234", "X-Forwarded-For", "203.0.113.10")
	if n := strings.Count(logs.String(), "isn't trusted"); n != 1 {
		t.Errorf("warned %d times:\n%s", n, logs)
	}

	off, _ := newTestApp(t, testManifest(), WithoutRateLimit())
	for range defaultRateBurst + 5 {
		if code := post(off, "203.0.113.7:1234"); code == http.StatusTooManyRequests {
			t.Fatal("limited with WithoutRateLimit")
		}
	}
}

func TestCSPAllowsSvelteAndWebAssembly(t *testing.T) {
	sum := sha256.Sum256([]byte("this.__e=event"))
	if want := "sha256-" + base64.StdEncoding.EncodeToString(sum[:]); want != svelteEventHash {
		t.Errorf("svelteEventHash = %s, want %s", svelteEventHash, want)
	}
	m := testManifest()
	m.WebAssembly = true
	a, _ := newTestApp(t, m)
	csp := do(t, a, "GET", "/", nil).Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "script-src 'self' 'unsafe-hashes' '"+svelteEventHash+"' 'wasm-unsafe-eval' 'nonce-") {
		t.Errorf("policy = %q", csp)
	}
}

package plugins_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Unfield/Tern/kit"
	"github.com/Unfield/Tern/plugins/cors"
	"github.com/Unfield/Tern/plugins/metrics"
	"github.com/Unfield/Tern/plugins/ratelimit"
	"github.com/Unfield/Tern/plugins/sessions"
)

func TestMetrics(t *testing.T) {
	a := app(t, kit.WithPlugins(metrics.New()))
	get(t, a, "/")
	get(t, a, "/")
	get(t, a, "/missing", "Accept", "application/json")
	get(t, a, "/nowhere")
	body := get(t, a, "/metrics").Body.String()
	for _, want := range []string{
		`tern_http_requests_total{method="GET",route="/",status="200"} 2`,
		`tern_http_requests_total{method="GET",route="/missing",status="404"} 1`,
		`tern_http_requests_total{method="GET",route="other",status="404"} 1`,
		`tern_http_request_duration_seconds_count{method="GET",route="/"} 2`,
		"go_goroutines",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics missing %s\n%s", want, body)
		}
	}
	if strings.Contains(body, `route="/metrics"`) {
		t.Error("the metrics endpoint itself should not be counted")
	}
}

func post(t *testing.T, h http.Handler, path string, form url.Values, headers ...string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	req.RemoteAddr = "203.0.113.7:1234"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestRateLimit(t *testing.T) {
	a := app(t, kit.WithPlugins(ratelimit.New(ratelimit.Rate(2, time.Hour))))
	codes := []int{}
	for range 3 {
		codes = append(codes, post(t, a, "/", url.Values{}).Code)
	}
	if codes[0] == http.StatusTooManyRequests || codes[1] == http.StatusTooManyRequests || codes[2] != http.StatusTooManyRequests {
		t.Errorf("codes = %v, want the third request limited", codes)
	}
	rec := post(t, a, "/", url.Values{})
	if rec.Header().Get("Retry-After") == "" {
		t.Error("limited responses need Retry-After")
	}
	if get(t, a, "/").Code != 200 {
		t.Error("GET requests are not limited by default")
	}
}

func TestSessions(t *testing.T) {
	secret := strings.Repeat("s", 32)
	m := &kit.Manifest{
		AppHTML: `<html><head>%tern.head%</head><body>%tern.body%</body></html>`,
		Routes: []kit.RouteDef{
			{ID: "/set", Endpoint: map[string]kit.HandlerFunc{"GET": func(c *kit.Ctx) error {
				s := sessions.Get(c)
				if err := s.Set("user", "ada"); err != nil {
					return err
				}
				if err := s.AddFlash("notice", "Welcome!"); err != nil {
					return err
				}
				return c.Text(200, "ok")
			}}},
			{ID: "/read", Endpoint: map[string]kit.HandlerFunc{"GET": func(c *kit.Ctx) error {
				s := sessions.Get(c)
				return c.Text(200, s.GetString("user")+"|"+s.Flash("notice"))
			}}},
		},
	}
	newApp := func(opts ...sessions.Option) *kit.App {
		a, err := kit.New(m, kit.WithPlugins(sessions.New(secret, opts...)))
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	a := newApp()
	setCookies := get(t, a, "http://localhost/set").Header().Values("Set-Cookie")
	if len(setCookies) != 1 || !strings.Contains(setCookies[0], "HttpOnly") {
		t.Fatalf("Set-Cookie = %v", setCookies)
	}
	cookie := strings.SplitN(setCookies[0], ";", 2)[0]
	if strings.Contains(cookie, "ada") {
		t.Error("session values must be encrypted")
	}

	rec := get(t, a, "http://localhost/read", "Cookie", cookie)
	if rec.Body.String() != "ada|Welcome!" {
		t.Errorf("read = %q", rec.Body.String())
	}
	// Reading the flash rewrote the cookie without it.
	next := strings.SplitN(rec.Header().Get("Set-Cookie"), ";", 2)[0]
	if body := get(t, a, "http://localhost/read", "Cookie", next).Body.String(); body != "ada|" {
		t.Errorf("flash should be shown once, got %q", body)
	}

	// Tampered cookies are ignored.
	if body := get(t, a, "http://localhost/read", "Cookie", cookie+"x").Body.String(); body != "|" {
		t.Errorf("tampered cookie accepted: %q", body)
	}

	// Rotating the secret keeps old sessions readable.
	rotated, err := kit.New(m, kit.WithPlugins(sessions.New(strings.Repeat("n", 32), sessions.OldSecrets(secret))))
	if err != nil {
		t.Fatal(err)
	}
	if body := get(t, rotated, "http://localhost/read", "Cookie", next).Body.String(); body != "ada|" {
		t.Errorf("rotated secret lost the session: %q", body)
	}

	if _, err := kit.New(m, kit.WithPlugins(sessions.New("short"))); err == nil {
		t.Error("short secrets must be rejected")
	}
}

func TestCORS(t *testing.T) {
	a := app(t, kit.WithPlugins(cors.New(cors.Origins("https://app.example.com"), cors.Credentials())))

	req := httptest.NewRequest(http.MethodOptions, "http://api.example.com/missing", nil)
	req.Header.Set("Origin", "https://app.example.com")
	req.Header.Set("Access-Control-Request-Method", "DELETE")
	rec := httptest.NewRecorder()
	a.ServeHTTP(rec, req)
	if rec.Code != 204 || rec.Header().Get("Access-Control-Allow-Origin") != "https://app.example.com" ||
		!strings.Contains(rec.Header().Get("Access-Control-Allow-Methods"), "DELETE") || rec.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Errorf("preflight = %d %v", rec.Code, rec.Header())
	}

	if rec := get(t, a, "http://api.example.com/", "Origin", "https://evil.example.com"); rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("unlisted origins must not be allowed")
	}

	// Allowed origins also pass Tern's cross-site protection for writes.
	rec = post(t, a, "http://api.example.com/", url.Values{}, "Origin", "https://app.example.com", "Sec-Fetch-Site", "cross-site")
	if rec.Code == http.StatusForbidden {
		t.Errorf("a CORS origin should be trusted for writes, got %d", rec.Code)
	}
	rec = post(t, a, "http://api.example.com/", url.Values{}, "Origin", "https://evil.example.com", "Sec-Fetch-Site", "cross-site")
	if rec.Code != http.StatusForbidden {
		t.Errorf("other cross-site writes must stay blocked, got %d", rec.Code)
	}
}

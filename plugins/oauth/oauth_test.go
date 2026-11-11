package oauth

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"golang.org/x/oauth2"
	_ "modernc.org/sqlite"

	"github.com/Unfield/Tern/accounts"
	"github.com/Unfield/Tern/kit"
)

func TestSafeNext(t *testing.T) {
	for in, want := range map[string]string{
		"/dashboard?x=1":        "/dashboard?x=1",
		"":                      "/",
		"https://evil.example/": "/",
		"//evil.example/":       "/",
		"/\\evil.example":       "/",
		"javascript:alert(1)":   "/",
	} {
		if got := safeNext(in); got != want {
			t.Errorf("safeNext(%q) = %q, want %q", in, got, want)
		}
	}
}

// fakeGitHub serves GitHub's OAuth and user endpoints.
func fakeGitHub(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /login/oauth/authorize", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("code_challenge") == "" || q.Get("code_challenge_method") != "S256" {
			http.Error(w, "no PKCE", http.StatusBadRequest)
			return
		}
		http.Redirect(w, r, q.Get("redirect_uri")+"?code=c0de&state="+url.QueryEscape(q.Get("state")), http.StatusFound)
	})
	mux.HandleFunc("POST /login/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.PostForm.Get("code") != "c0de" || r.PostForm.Get("code_verifier") == "" {
			http.Error(w, "bad code", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"gh-token","token_type":"bearer"}`))
	})
	mux.HandleFunc("GET /user", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer gh-token" {
			http.Error(w, "no token", http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 42, "login": "octo", "name": "Octo Cat", "avatar_url": "https://avatars/42"})
	})
	mux.HandleFunc("GET /user/emails", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{{"email": "old@example.com", "primary": false, "verified": true}, {"email": "octo@example.com", "primary": true, "verified": true}})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestGitHubSignIn(t *testing.T) {
	gh := fakeGitHub(t)
	p := GitHub("id", "secret")
	p.Config.Endpoint = oauth2.Endpoint{AuthURL: gh.URL + "/login/oauth/authorize", TokenURL: gh.URL + "/login/oauth/access_token"}
	p.UserInfo = githubUser(gh.URL)
	var got *Identity
	app, err := kit.New(&kit.Manifest{
		AppHTML: `<html><head>%tern.head%</head><body>%tern.body%</body></html>`,
		Routes:  []kit.RouteDef{{ID: "/welcome", Endpoint: map[string]kit.HandlerFunc{"GET": func(c *kit.Ctx) error { return c.Text(200, "welcome") }}}},
	}, kit.WithPlugins(New(Options{
		Secret:    strings.Repeat("s", 32),
		Providers: []*Provider{p},
		OnLogin:   func(_ *kit.Ctx, id Identity) error { got = &id; return nil },
	})))
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(app)
	defer srv.Close()
	jar, _ := cookiejar.New(nil)
	b := &http.Client{Jar: jar}

	res, err := b.Get(srv.URL + "/auth/github/login?next=/welcome")
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.Request.URL.Path != "/welcome" || got == nil {
		t.Fatalf("ended at %s, identity %+v", res.Request.URL, got)
	}
	if got.Subject != "42" || got.Username != "octo" || got.Email != "octo@example.com" || !got.EmailVerified || got.Token.AccessToken != "gh-token" {
		t.Errorf("identity = %+v", got)
	}

	// A callback without the flow's cookie, or with another state, fails.
	b.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	plain := &http.Client{CheckRedirect: b.CheckRedirect}
	res, _ = plain.Get(srv.URL + "/auth/github/callback?code=c0de&state=x")
	_ = res.Body.Close()
	if loc := res.Header.Get("Location"); loc != "/login?error=expired" {
		t.Errorf("no cookie: %q", loc)
	}
	res, _ = b.Get(srv.URL + "/auth/github/login")
	_ = res.Body.Close()
	res, _ = b.Get(srv.URL + "/auth/github/callback?code=c0de&state=forged")
	_ = res.Body.Close()
	if loc := res.Header.Get("Location"); loc != "/login?error=state" {
		t.Errorf("forged state: %q", loc)
	}
	res, _ = plain.Get(srv.URL + "/auth/nope/login")
	_ = res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("unknown provider: %d", res.StatusCode)
	}
}

func TestOptionsValidated(t *testing.T) {
	m := &kit.Manifest{AppHTML: `<html><head>%tern.head%</head><body>%tern.body%</body></html>`}
	if _, err := kit.New(m, kit.WithPlugins(New(Options{Secret: strings.Repeat("s", 32)}))); err == nil {
		t.Error("OnLogin is required")
	}
	if _, err := kit.New(m, kit.WithPlugins(New(Options{Secret: "short", OnLogin: func(*kit.Ctx, Identity) error { return nil }}))); err == nil {
		t.Error("a short secret is refused")
	}
}

func TestSignInToAccounts(t *testing.T) {
	gh := fakeGitHub(t)
	p := GitHub("id", "secret")
	p.Config.Endpoint = oauth2.Endpoint{AuthURL: gh.URL + "/login/oauth/authorize", TokenURL: gh.URL + "/login/oauth/access_token"}
	p.UserInfo = githubUser(gh.URL)
	db, err := sql.Open("sqlite", "file:"+t.Name()+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	defer func() { _ = db.Close() }()
	acc, err := accounts.New(db, accounts.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := acc.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	app, err := kit.New(&kit.Manifest{
		AppHTML: `<html><head>%tern.head%</head><body>%tern.body%</body></html>`,
		Routes: []kit.RouteDef{{ID: "/me", Endpoint: map[string]kit.HandlerFunc{"GET": func(c *kit.Ctx) error {
			if u := accounts.Current(c); u != nil {
				return c.Text(200, u.Email)
			}
			return c.Text(401, "signed out")
		}}}},
	}, kit.WithPlugins(acc, New(Options{Secret: strings.Repeat("s", 32), Providers: []*Provider{p}, Accounts: acc})))
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(app)
	defer srv.Close()
	for range 2 { // the first sign-in creates the account, the second finds it
		jar, _ := cookiejar.New(nil)
		b := &http.Client{Jar: jar}
		res, err := b.Get(srv.URL + "/auth/github/login?next=/me")
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(res.Body)
		_ = res.Body.Close()
		if res.StatusCode != http.StatusOK || string(body) != "octo@example.com" {
			t.Fatalf("%d %s at %s", res.StatusCode, body, res.Request.URL)
		}
	}
	var users int
	_ = db.QueryRow(`SELECT COUNT(*) FROM tern_users WHERE email_verified IS NOT NULL`).Scan(&users)
	if users != 1 {
		t.Fatalf("%d verified users", users)
	}
}

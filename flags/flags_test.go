package flags_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Unfield/Tern/admin"
	"github.com/Unfield/Tern/flags"
	"github.com/Unfield/Tern/internal/dbtest"
	"github.com/Unfield/Tern/kit"
)

func TestDecide(t *testing.T) {
	fl := flags.Flag{Name: "new-editor", On: true, Percent: 0, By: "user", Users: "7, 9", Teams: "3"}
	cases := []struct {
		s    flags.Subject
		want bool
	}{
		{flags.Subject{User: "7"}, true},
		{flags.Subject{User: "9"}, true},
		{flags.Subject{User: "8", Team: "3"}, true},
		{flags.Subject{User: "8"}, false},
		{flags.Subject{}, false},
	}
	for _, c := range cases {
		if got := fl.Decide(c.s, ""); got != c.want {
			t.Errorf("%+v: %v", c.s, got)
		}
	}
	// Off is off, for everyone.
	fl.On = false
	if fl.Decide(flags.Subject{User: "7"}, "") {
		t.Error("an off flag is on for a listed user")
	}

	// A rollout reaches about its share, and growing it keeps everyone in.
	fl = flags.Flag{Name: "faster", On: true, By: "user"}
	had := map[int]bool{}
	for _, pct := range []int{1, 10, 30, 60, 100} {
		fl.Percent = pct
		n := 0
		for u := range 10000 {
			on := fl.Decide(flags.Subject{User: fmt.Sprint(u)}, "")
			if had[u] && !on {
				t.Fatalf("user %d lost the flag going to %d%%", u, pct)
			}
			if on {
				had[u] = true
				n++
			}
		}
		if share := float64(n) / 100; share < float64(pct)-2 || share > float64(pct)+2 {
			t.Errorf("%d%%: %.1f%% got it", pct, share)
		}
	}

	// Counted by team, a team's users decide together; another flag splits them differently.
	fl = flags.Flag{Name: "a", On: true, By: "team", Percent: 50}
	other := flags.Flag{Name: "b", On: true, By: "team", Percent: 50}
	same, diff := 0, 0
	for team := range 1000 {
		s := flags.Subject{Team: fmt.Sprint(team)}
		if fl.Decide(flags.Subject{User: "1", Team: s.Team}, "") != fl.Decide(flags.Subject{User: "2", Team: s.Team}, "") {
			t.Fatal("two users of a team decided differently")
		}
		if fl.Decide(s, "") == other.Decide(s, "") {
			same++
		} else {
			diff++
		}
	}
	if diff < 400 {
		t.Errorf("flags aren't independent: %d same, %d different", same, diff)
	}
}

func app(t *testing.T, db *sql.DB, defined ...flags.Flag) (*flags.Flags, http.Handler) {
	t.Helper()
	f, err := flags.New(db, flags.Options{Subject: func(c *kit.Ctx) flags.Subject {
		return flags.Subject{User: c.Request.Header.Get("X-User"), Team: c.Request.Header.Get("X-Team")}
	}}, defined...)
	if err != nil {
		t.Fatal(err)
	}
	panel := admin.New(admin.Options{Authorize: func(*kit.Ctx) error { return nil }}, f.Admin())
	a, err := kit.New(&kit.Manifest{AppHTML: `<html><head>%tern.head%</head><body>%tern.body%</body></html>`, Routes: []kit.RouteDef{
		{ID: "/check", Endpoint: map[string]kit.HandlerFunc{"GET": func(c *kit.Ctx) error {
			return c.JSON(200, f.For(c, strings.Split(c.URL().Query().Get("f"), ",")...))
		}}},
		{ID: "/admin/path.rest", Page: &kit.PageDef{Component: "src/routes/admin/+page.svelte", Load: kit.LoadOf(admin.Load), Actions: admin.Actions}},
	}}, kit.WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))), kit.WithPlugins(f, panel))
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Start(); err != nil {
		t.Fatal(err)
	}
	return f, a
}

func check(t *testing.T, h http.Handler, names string, headers ...string) (map[string]bool, []*http.Cookie) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/check?f="+names, nil)
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var got map[string]bool
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("%s: %s", err, rec.Body)
	}
	res := rec.Result()
	_ = res.Body.Close()
	return got, res.Cookies()
}

func TestFlags(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db *sql.DB) {
		ctx := context.Background()
		f, h := app(t, db, flags.Flag{Name: "new-editor", Description: "The new editor"}, flags.Flag{Name: "dark-mode", On: true, Percent: 100})

		// Defined flags start as defined; an unknown one is off.
		got, _ := check(t, h, "new-editor,dark-mode,nope", "X-User", "7")
		if got["new-editor"] || !got["dark-mode"] || got["nope"] {
			t.Errorf("start: %v", got)
		}

		// Set applies at once.
		if err := f.Set(ctx, flags.Flag{Name: "new-editor", Description: "The new editor", On: true, Users: "7"}); err != nil {
			t.Fatal(err)
		}
		if got, _ := check(t, h, "new-editor", "X-User", "7"); !got["new-editor"] {
			t.Error("user 7 should have it")
		}
		if got, _ := check(t, h, "new-editor", "X-User", "8"); got["new-editor"] {
			t.Error("user 8 shouldn't")
		}

		// Starting again keeps what was set, and another process sees it on Reload.
		f2, err := flags.New(db, flags.Options{}, flags.Flag{Name: "new-editor"})
		if err != nil {
			t.Fatal(err)
		}
		if err := f2.Migrate(ctx); err != nil {
			t.Fatal(err)
		}
		if fl, _ := f2.Get("new-editor"); !fl.On || fl.Users != "7" {
			t.Errorf("after a restart: %+v", fl)
		}
		if err := f2.Set(ctx, flags.Flag{Name: "new-editor", On: false}); err != nil {
			t.Fatal(err)
		}
		if err := f.Reload(ctx); err != nil {
			t.Fatal(err)
		}
		if got, _ := check(t, h, "new-editor", "X-User", "7"); got["new-editor"] {
			t.Error("the kill switch from another process didn't apply")
		}

		// Counted by visitor: a cookie keeps the decision.
		if err := f.Set(ctx, flags.Flag{Name: "banner", On: true, Percent: 50, By: "visitor"}); err != nil {
			t.Fatal(err)
		}
		first, cookies := check(t, h, "banner")
		var cookie *http.Cookie
		for _, c := range cookies {
			if c.Name == "tern_visitor" {
				cookie = c
			}
		}
		if cookie == nil || !cookie.HttpOnly {
			t.Fatalf("no visitor cookie: %v", cookies)
		}
		for range 5 {
			if again, _ := check(t, h, "banner", "Cookie", "tern_visitor="+cookie.Value); again["banner"] != first["banner"] {
				t.Fatal("a visitor's decision changed")
			}
		}

		// The admin screens list and change flags.
		post := func(path string, form url.Values) *httptest.ResponseRecorder {
			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("X-Tern-Action", "true")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			return rec
		}
		rec := post("/admin/flags/dark-mode?/save", url.Values{"description": {"Dark colors"}, "on": {"on"}, "percent": {"25"}, "by": {"team"}})
		if !strings.Contains(rec.Body.String(), `"redirect"`) {
			t.Fatalf("admin save: %s", rec.Body)
		}
		if fl, _ := f.Get("dark-mode"); fl.Percent != 25 || fl.By != "team" || fl.Description != "Dark colors" {
			t.Errorf("after the admin save: %+v", fl)
		}
		if rec := post("/admin/flags/new?/save", url.Values{"name": {"dark-mode"}, "by": {"user"}}); !strings.Contains(rec.Body.String(), "already a flag") {
			t.Errorf("a taken name: %s", rec.Body)
		}
		if rec := post("/admin/flags/new?/save", url.Values{"name": {"Bad Name"}, "by": {"user"}}); !strings.Contains(rec.Body.String(), `"failure"`) {
			t.Errorf("a bad name: %s", rec.Body)
		}
		if rec := post("/admin/flags/banner?/delete", nil); !strings.Contains(rec.Body.String(), `"redirect"`) {
			t.Errorf("admin delete: %s", rec.Body)
		}
		if _, ok := f.Get("banner"); ok {
			t.Error("the deleted flag is still there")
		}
		req := httptest.NewRequest(http.MethodGet, "/admin/flags/__data.json?q=dark", nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if !strings.Contains(rr.Body.String(), `"dark-mode"`) || strings.Contains(rr.Body.String(), `"new-editor"`) {
			t.Errorf("admin list search: %s", rr.Body)
		}
	})
}

func TestBadNames(t *testing.T) {
	db, _ := sql.Open("sqlite", ":memory:")
	defer func() { _ = db.Close() }()
	if _, err := flags.New(db, flags.Options{}, flags.Flag{Name: "Has Spaces"}); err == nil {
		t.Error("a bad name was accepted")
	}
}

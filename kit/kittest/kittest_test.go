package kittest_test

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"testing"
	"testing/fstest"

	"github.com/Unfield/Tern/accounts"
	"github.com/Unfield/Tern/kit"
	"github.com/Unfield/Tern/kit/kittest"
)

type post struct {
	Slug  string `json:"slug"`
	Title string `json:"title"`
}

type layout struct {
	Site string `json:"site"`
}

// load is a route's Load, as an app has it.
func load(c *kit.Ctx) (post, error) {
	l, err := kit.Parent[layout](c)
	if err != nil {
		return post{}, err
	}
	switch c.Param("slug") {
	case "old":
		return post{}, kit.Redirect(http.StatusMovedPermanently, "/blog/new")
	case "missing":
		return post{}, kit.NotFound()
	}
	return post{Slug: c.Param("slug"), Title: l.Site + ": " + c.Param("slug")}, nil
}

func create(c *kit.Ctx) (post, error) {
	form, err := c.Form()
	if err != nil {
		return post{}, err
	}
	if form.Get("title") == "" {
		return post{}, kit.Fail(http.StatusBadRequest, map[string]string{"title": "required"})
	}
	c.SetCookie(&http.Cookie{Name: "flash", Value: "created"})
	return post{Title: form.Get("title")}, nil
}

func TestLoadsAndActions(t *testing.T) {
	app := kittest.App(t)
	c := kittest.Ctx(t, app, kittest.Path("/blog/hello"), kittest.Route("/blog/slug.param", "slug", "hello"), kittest.Parent(layout{Site: "Tern"}))
	got, err := load(c)
	if err != nil || got.Title != "Tern: hello" || c.RouteID() != "/blog/slug.param" {
		t.Fatalf("%+v %v", got, err)
	}
	_, err = load(kittest.Ctx(t, app, kittest.Route("/blog/slug.param", "slug", "old"), kittest.Parent(layout{})))
	if status, loc, ok := kittest.Redirected(err); !ok || status != 301 || loc != "/blog/new" {
		t.Errorf("redirect %d %s %v", status, loc, ok)
	}
	_, err = load(kittest.Ctx(t, app, kittest.Route("/blog/slug.param", "slug", "missing"), kittest.Parent(layout{})))
	if kittest.Status(err) != 404 || kittest.Status(nil) != 200 || kittest.Status(errors.New("x")) != 500 {
		t.Errorf("status %d", kittest.Status(err))
	}

	_, err = create(kittest.Ctx(t, app, kittest.Form(url.Values{"title": {""}})))
	if status, data, ok := kittest.Failed(err); !ok || status != 400 || data.(map[string]string)["title"] != "required" {
		t.Errorf("fail %d %v %v", status, data, ok)
	}
	c = kittest.Ctx(t, app, kittest.Form(url.Values{"title": {"Hi"}}))
	if p, err := create(c); err != nil || p.Title != "Hi" || c.Request.Method != http.MethodPost {
		t.Errorf("%+v %v", p, err)
	}
	res := kittest.Response(c).Result()
	defer func() { _ = res.Body.Close() }()
	if ck := res.Cookies(); len(ck) != 1 || ck[0].Value != "created" {
		t.Errorf("cookies %v", ck)
	}
}

func TestHooksAndAccounts(t *testing.T) {
	db := kittest.SQLite(t)
	acc, err := accounts.New(db, accounts.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := acc.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	app := kittest.App(t, kit.WithPlugins(acc))
	c := kittest.Ctx(t, app, kittest.Path("/settings"))
	if _, err := acc.Require(c); err == nil {
		t.Fatal("signed out, Require let the request through")
	}
	u := &accounts.User{ID: 7, Email: "ada@example.com", Name: "Ada"}
	acc.ActAs(c, u)
	if got, err := acc.Require(c); err != nil || got.ID != 7 || accounts.Current(c) != u {
		t.Fatalf("%v %v", got, err)
	}
}

func TestSQLite(t *testing.T) {
	migrations := fstest.MapFS{"0001_notes.sql": {Data: []byte("CREATE TABLE notes (id INTEGER PRIMARY KEY, text TEXT NOT NULL);")}}
	a, b := kittest.SQLite(t, migrations), kittest.SQLite(t, migrations)
	if _, err := a.Exec(`INSERT INTO notes (text) VALUES ('hi')`); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := b.QueryRow(`SELECT count(*) FROM notes`).Scan(&n); err != nil || n != 0 {
		t.Errorf("the databases aren't separate: %d %v", n, err)
	}
	if err := a.QueryRow(`SELECT count(*) FROM notes`).Scan(&n); err != nil || n != 1 {
		t.Errorf("%d %v", n, err)
	}
}

func TestClient(t *testing.T) {
	page := func(c *kit.Ctx) error {
		c.SetHeader("Content-Type", "text/html; charset=utf-8")
		_, err := c.Writer().Write([]byte(`<!doctype html><main><h1 class="title big">Hello,   <b>world</b></h1>
<ul id="items"><li><a href="/a" class="item">A</a></li><li><a href="/b" class="item active" data-x="1">B</a></li></ul>
<form><input name="email" value="x@y.z"></form></main>`))
		return err
	}
	m := &kit.Manifest{AppHTML: `<html><head>%tern.head%</head><body>%tern.body%</body></html>`, Routes: []kit.RouteDef{
		{ID: "/", Endpoint: map[string]kit.HandlerFunc{"GET": page, "POST": func(c *kit.Ctx) error {
			return kit.Redirect(http.StatusSeeOther, "/done")
		}}},
	}}
	app, err := kit.New(m)
	if err != nil {
		t.Fatal(err)
	}
	client := kittest.NewClient(t, app)
	p := client.Get("/")
	if p.Status != 200 || p.Text("h1.title") != "Hello, world" || p.Text("h1") != "Hello, world" {
		t.Fatalf("%d %q", p.Status, p.Text("h1"))
	}
	if got := len(p.Find("#items > li > a.item")); got != 2 {
		t.Errorf("child selector: %d", got)
	}
	if e := p.First("a.item.active[data-x=1]"); e == nil || e.Attr("href") != "/b" || e.Tag() != "a" {
		t.Errorf("compound selector: %v", e)
	}
	if !p.Has("main input[name=email]") || p.Has("#items > a") || len(p.Find("h1, input")) != 2 {
		t.Error("descendant, child or list selectors")
	}
	if p := client.PostForm("/", url.Values{"x": {"1"}}); p.Status != http.StatusSeeOther || p.Location() != "/done" {
		t.Errorf("post: %d %s", p.Status, p.Location())
	}
}

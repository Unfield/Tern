package main

import (
	"net/url"
	"path/filepath"
	"testing"

	"github.com/Unfield/Tern/kit/kittest"
)

// These tests run the whole app, as a browser would use it: npm test, or
// go tool tern build && go test ./...

// signedIn starts the app on an empty database and returns a client
// signed in as a new user.
func signedIn(t *testing.T) *kittest.Client {
	t.Helper()
	app, err := newApp(filepath.Join(t.TempDir(), "shelf.db"))
	if err != nil {
		t.Fatal(err)
	}
	return signUp(t, kittest.NewClient(t, app), "ada@example.com")
}

func signUp(t *testing.T, client *kittest.Client, email string) *kittest.Client {
	t.Helper()
	page := client.PostForm("/signup", url.Values{"name": {"Ada"}, "email": {email}, "password": {"a long enough password"}})
	if page.Status != 303 {
		t.Fatalf("signing up: %d", page.Status)
	}
	return client
}

func TestSignedOut(t *testing.T) {
	app, err := newApp(filepath.Join(t.TempDir(), "shelf.db"))
	if err != nil {
		t.Fatal(err)
	}
	page := kittest.NewClient(t, app).Get("/")
	if page.Status != 303 || page.Location() != "/login?next=%2F" {
		t.Fatalf("GET / signed out: %d %s", page.Status, page.Location())
	}
}

func TestSavingALink(t *testing.T) {
	client := signedIn(t)
	page := client.PostForm("/?/add", url.Values{"url": {"https://go.dev/blog/go1.26"}, "title": {"Go 1.26 is released"}})
	if page.Status != 200 {
		t.Fatalf("saving: %d", page.Status)
	}
	page = client.Get("/")
	if got := page.Text(".links li a"); got != "Go 1.26 is released" {
		t.Errorf("the shelf shows %q", got)
	}
}

func TestAnAddressIsRequired(t *testing.T) {
	client := signedIn(t)
	if page := client.PostForm("/?/add", url.Values{"url": {"not an address"}}); page.Status != 400 {
		t.Errorf("an invalid address: %d, want 400", page.Status)
	}
	if page := client.Get("/"); page.Has(".links li") {
		t.Error("an invalid link was saved")
	}
}

func TestLinksArePrivate(t *testing.T) {
	ada := signedIn(t)
	ada.PostForm("/?/add", url.Values{"url": {"https://go.dev"}})
	if page := ada.Get("/links/1"); page.Status != 200 {
		t.Fatalf("Ada's own link: %d", page.Status)
	}
	// Bob is another browser, on the same app.
	bob := signUp(t, ada.Browser(), "bob@example.com")
	if page := bob.Get("/links/1"); page.Status != 404 {
		t.Errorf("Bob sees Ada's link: %d", page.Status)
	}
}

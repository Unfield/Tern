package kittest

import (
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"golang.org/x/net/html"

	"github.com/Unfield/Tern/kit"
)

// Client sends requests to an app in the test, as a browser would: it
// keeps cookies, sends forms from the app's own origin, and doesn't follow
// redirects, so tests see them.
type Client struct {
	t      testing.TB
	srv    *httptest.Server
	client *http.Client
}

// NewClient serves h, usually the App from the app's ternApp, for the
// test. An App is started first, so its plugins' start functions run,
// migrations among them, and is closed when the test ends.
func NewClient(t testing.TB, h http.Handler) *Client {
	t.Helper()
	if a, ok := h.(*kit.App); ok {
		if err := a.Start(); err != nil {
			t.Fatalf("kittest: starting the app: %v", err)
		}
		t.Cleanup(func() { _ = a.Close() })
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	jar, _ := cookiejar.New(nil)
	return &Client{t: t, srv: srv, client: &http.Client{
		Jar:           jar,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

// URL is the test server's address for path.
func (c *Client) URL(path string) string { return c.srv.URL + path }

// Page is a response, its body read, and its HTML parsed when it is HTML.
type Page struct {
	*http.Response
	Status int
	Body   string
	doc    *html.Node
}

// Get requests path as a page load.
func (c *Client) Get(path string) *Page {
	c.t.Helper()
	req, err := http.NewRequest(http.MethodGet, c.URL(path), nil)
	if err != nil {
		c.t.Fatalf("kittest: %v", err)
	}
	req.Header.Set("Accept", "text/html")
	return c.Do(req)
}

// PostForm submits values to path, as a form on the app's own pages does:
// action "?/name" in path for a named action.
func (c *Client) PostForm(path string, values url.Values) *Page {
	c.t.Helper()
	req, err := http.NewRequest(http.MethodPost, c.URL(path), strings.NewReader(values.Encode()))
	if err != nil {
		c.t.Fatalf("kittest: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "text/html")
	return c.Do(req)
}

// Do sends req, from the app's own origin.
func (c *Client) Do(req *http.Request) *Page {
	c.t.Helper()
	if req.Header.Get("Origin") == "" && req.Method != http.MethodGet && req.Method != http.MethodHead {
		req.Header.Set("Origin", c.srv.URL)
		req.Header.Set("Sec-Fetch-Site", "same-origin")
	}
	resp, err := c.client.Do(req)
	if err != nil {
		c.t.Fatalf("kittest: %s %s: %v", req.Method, req.URL.Path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		c.t.Fatalf("kittest: reading %s: %v", req.URL.Path, err)
	}
	p := &Page{Response: resp, Status: resp.StatusCode, Body: string(b)}
	if strings.HasPrefix(resp.Header.Get("Content-Type"), "text/html") {
		p.doc, _ = html.Parse(strings.NewReader(p.Body))
	}
	return p
}

// Location is where a redirect goes, "" for other responses.
func (p *Page) Location() string { return p.Header.Get("Location") }

// Find returns the elements matching a CSS selector. The selectors are a
// subset: tag names, #id, .class, [attr], [attr=value], combined
// (a.button[href]), with descendant (space) and child (>) combinators,
// and lists (h1, h2).
func (p *Page) Find(selector string) []*Element {
	if p.doc == nil {
		return nil
	}
	return find(p.doc, selector)
}

// First returns the first element matching selector, or nil.
func (p *Page) First(selector string) *Element {
	if all := p.Find(selector); len(all) > 0 {
		return all[0]
	}
	return nil
}

// Has reports whether an element matches selector.
func (p *Page) Has(selector string) bool { return p.First(selector) != nil }

// Text is the text of the first element matching selector, with spaces
// collapsed; "" if none does.
func (p *Page) Text(selector string) string {
	if e := p.First(selector); e != nil {
		return e.Text()
	}
	return ""
}

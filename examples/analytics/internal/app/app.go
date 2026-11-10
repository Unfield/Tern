// Package app holds the services the routes share, and what happens when a
// page view arrives.
package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Unfield/Tern/goscript"
	"github.com/Unfield/Tern/kit"
	"github.com/Unfield/Tern/plugins/sessions"

	"analytics/internal/store"
	"analytics/internal/ua"
)

// Store is set up once by main.
var Store *store.Store

// User is the signed-in account, as hooks put it in Locals.
type User struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// LoadUser is a hook: it reads the session and puts the user in Locals.
func LoadUser(ctx *kit.Ctx) error {
	raw := sessions.Get(ctx).GetString("user")
	if raw == "" {
		return nil
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return nil //nolint:nilerr // a bad session means signed out
	}
	u, err := Store.UserByID(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		sessions.Get(ctx).Clear()
		return nil
	}
	if err != nil {
		return err
	}
	ctx.Locals["user"] = User{ID: u.ID, Name: u.Name, Email: u.Email}
	return nil
}

// CurrentUser returns the signed-in user.
func CurrentUser(ctx *kit.Ctx) (User, bool) {
	u, ok := ctx.Locals["user"].(User)
	return u, ok
}

// SignIn starts a session for u.
func SignIn(ctx *kit.Ctx, id int64) error {
	return sessions.Get(ctx).Set("user", strconv.FormatInt(id, 10))
}

// Topic is where page views of a site are announced, for live counts.
func Topic(domain string) string { return "site:" + domain }

// hit is what the tracking script posts.
type hit struct {
	Domain   string `json:"d"`
	URL      string `json:"u"`
	Referrer string `json:"r"`
}

// Collect records a page view posted by /script.js. It is public, posted
// from other sites as text/plain so browsers send it without a preflight,
// and answers 202 even for views it ignores, so it reveals nothing.
func Collect(ctx *kit.Ctx) error {
	w := ctx.Writer()
	w.Header().Set("Access-Control-Allow-Origin", "*")
	if ctx.Request.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Methods", "POST")
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	var h hit
	if err := json.NewDecoder(io.LimitReader(ctx.Request.Body, 4<<10)).Decode(&h); err != nil {
		return kit.Error(http.StatusBadRequest, "send JSON")
	}
	agent := ctx.Request.UserAgent()
	info := ua.Parse(agent)
	if info.Bot {
		w.WriteHeader(http.StatusAccepted)
		return nil
	}
	site, err := Store.SiteByDomain(ctx, h.Domain)
	if errors.Is(err, store.ErrNotFound) {
		w.WriteHeader(http.StatusAccepted)
		return nil
	}
	if err != nil {
		return err
	}
	page, err := url.Parse(h.URL)
	if err != nil || page.Path == "" {
		return kit.Error(http.StatusBadRequest, "bad url")
	}
	path := page.Path
	if len(path) > 300 {
		path = path[:300]
	}
	now := time.Now()
	visitor, err := Store.Visitor(ctx, now, site.Domain, ctx.ClientIP(), agent)
	if err != nil {
		return err
	}
	err = Store.AddEvent(ctx, store.Event{SiteID: site.ID, At: now, Visitor: visitor, Path: path, Referrer: referrerHost(h.Referrer, site.Domain), Browser: info.Browser, OS: info.OS, Device: info.Device})
	if err != nil {
		return err
	}
	_ = goscript.Publish(context.WithoutCancel(ctx), Topic(site.Domain), now.UnixMilli())
	w.WriteHeader(http.StatusAccepted)
	return nil
}

// referrerHost keeps only the host a visitor came from, and nothing for
// links within the site.
func referrerHost(ref, domain string) string {
	u, err := url.Parse(ref)
	if err != nil || u.Host == "" {
		return ""
	}
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	if host == domain || strings.HasSuffix(host, "."+domain) {
		return ""
	}
	return host
}

// Package app holds the status page's shared services and helpers for its
// routes.
package app

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Unfield/Tern/content"
	"github.com/Unfield/Tern/jobs"
	"github.com/Unfield/Tern/kit"
	"github.com/Unfield/Tern/plugins/sessions"
	"github.com/Unfield/Tern/realtime"
	"golang.org/x/crypto/bcrypt"

	"status/internal/monitor"
	"status/internal/store"
)

// Services are set up once by main.
var (
	Store  *store.Store
	Engine *monitor.Engine
	Hub    *realtime.Hub[monitor.Event]
	Queue  *jobs.Queue
	// Notify sends a change to one channel, in the background.
	Notify *jobs.Kind[NotifyJob]
)

// NotifyJob is the payload of a notification job.
type NotifyJob struct {
	ChannelID int64          `json:"channelId"`
	Change    monitor.Change `json:"change"`
}

// Admin is the signed-in administrator.
type Admin struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

// CurrentAdmin returns the signed-in administrator, if any.
func CurrentAdmin(ctx *kit.Ctx) (Admin, bool) {
	var a Admin
	ok := sessions.Get(ctx).Value("admin", &a)
	return a, ok && a.ID != 0
}

// RequireAdmin returns the administrator, or redirects to the login page,
// or to the setup page while there is no administrator yet.
func RequireAdmin(ctx *kit.Ctx) (Admin, error) {
	if a, ok := CurrentAdmin(ctx); ok {
		return a, nil
	}
	if n, err := Store.UserCount(ctx); err == nil && n == 0 {
		return Admin{}, kit.Redirect(http.StatusSeeOther, "/setup")
	}
	return Admin{}, kit.Redirect(http.StatusSeeOther, "/login?next="+ctx.Request.URL.Path)
}

// SignIn stores the administrator in the session.
func SignIn(ctx *kit.Ctx, a Admin) error { return sessions.Get(ctx).Set("admin", a) }

// SignOut clears the session.
func SignOut(ctx *kit.Ctx) { sessions.Get(ctx).Clear() }

// HashPassword hashes a password with bcrypt.
func HashPassword(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(b), err
}

// CheckPassword compares a password with a hash in constant time.
func CheckPassword(hash, pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}

// NotFound turns store.ErrNotFound into a 404.
func NotFound(err error) error {
	if errors.Is(err, store.ErrNotFound) {
		return kit.NotFound()
	}
	return err
}

// ID reads a route's integer id parameter.
func ID(ctx *kit.Ctx) int64 {
	id, _ := strconv.ParseInt(ctx.Param("id"), 10, 64)
	return id
}

// Flash stores a message for the next page.
func Flash(ctx *kit.Ctx, msg string) { _ = sessions.Get(ctx).AddFlash("notice", msg) }

// TakeFlash returns and clears the flash message.
func TakeFlash(ctx *kit.Ctx) string { return sessions.Get(ctx).Flash("notice") }

// RenderIncidents renders the Markdown of each incident update.
func RenderIncidents(in []store.Incident) []store.Incident {
	for i := range in {
		for j := range in[i].Updates {
			html, err := content.Render(in[i].Updates[j].Body)
			if err != nil {
				html = "<p>" + strings.ReplaceAll(in[i].Updates[j].Body, "<", "&lt;") + "</p>"
			}
			in[i].Updates[j].HTML = html
		}
	}
	return in
}

// NotifyAll enqueues a notification of c to every enabled channel.
func NotifyAll(ctx context.Context, c monitor.Change) {
	channels, err := Store.Channels(ctx, true)
	if err != nil {
		return
	}
	for _, ch := range channels {
		_, _ = Notify.Enqueue(ctx, NotifyJob{ChannelID: ch.ID, Change: c})
	}
}

// Since returns the start of a period such as "24h", "7d" or "30d".
func Since(period string) time.Time {
	switch period {
	case "7d":
		return time.Now().AddDate(0, 0, -7)
	case "30d":
		return time.Now().AddDate(0, 0, -30)
	case "90d":
		return time.Now().AddDate(0, 0, -90)
	}
	return time.Now().Add(-24 * time.Hour)
}

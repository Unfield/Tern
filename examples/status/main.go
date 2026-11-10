// Command status is a self-hosted status page: it checks websites and
// services, shows their uptime on a public page, and notifies you when
// something goes down. See README.md.
package main

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"errors"
	"log"
	"log/slog"
	"os"
	"time"

	"github.com/Unfield/Tern/jobs"
	"github.com/Unfield/Tern/kit"
	"github.com/Unfield/Tern/mail"
	"github.com/Unfield/Tern/migrate"
	"github.com/Unfield/Tern/plugins/compress"
	"github.com/Unfield/Tern/plugins/health"
	"github.com/Unfield/Tern/plugins/requestlog"
	"github.com/Unfield/Tern/plugins/secure"
	"github.com/Unfield/Tern/plugins/sessions"
	"github.com/Unfield/Tern/realtime"

	"status/internal/app"
	"status/internal/monitor"
	"status/internal/notify"
	"status/internal/store"
)

//go:embed migrations/*.sql
var migrations embed.FS

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	path := env("DATABASE_PATH", "status.db")
	st, err := store.Open(path)
	if err != nil {
		log.Fatal(err)
	}

	q, err := jobs.New(st.DB, jobs.Options{Logger: logger})
	if err != nil {
		log.Fatal(err)
	}
	app.Store = st
	app.Hub = realtime.NewHub[monitor.Event]()
	app.Queue = q
	app.Notify = jobs.Handle(q, "notify", func(ctx context.Context, j app.NotifyJob) error {
		ch, err := st.Channel(ctx, j.ChannelID)
		if errors.Is(err, store.ErrNotFound) || (err == nil && !ch.Enabled) {
			return nil // deleted or disabled since
		}
		if err != nil {
			return err
		}
		return notify.Send(ctx, ch, j.Change)
	}, jobs.MaxAttempts(5), jobs.Timeout(30*time.Second))
	app.Engine = &monitor.Engine{Store: st, Hub: app.Hub, Notify: app.NotifyAll, Log: logger}

	// Keep 90 days of checks, for the uptime bars.
	if err := q.Schedule("prune-heartbeats", "@daily", func(ctx context.Context) error {
		n, err := st.DeleteHeartbeatsBefore(ctx, time.Now().AddDate(0, 0, -91))
		logger.Info("pruned old checks", "deleted", n)
		return err
	}); err != nil {
		log.Fatal(err)
	}

	secret, err := sessionSecret(path)
	if err != nil {
		log.Fatal(err)
	}
	application, err := ternApp(
		kit.WithLogger(logger),
		kit.WithPlugins(
			// Start order: migrations, then the job queue, then the checks.
			migrate.Plugin(st.DB, migrations),
			q,
			app.Engine,
			sessions.New(secret, sessions.MaxAge(30*24*time.Hour)),
			health.New(health.Check("database", st.Ping)),
			requestlog.New(requestlog.SkipAssets()),
			compress.New(),
			secure.New(),
		),
	)
	if err != nil {
		log.Fatal(err)
	}
	// Email notifications render src/emails/MonitorAlert.svelte.
	notify.Mailer = func(ctx context.Context, smtp mail.SMTP, from string, to []string, c monitor.Change) error {
		props := map[string]any{
			"name": c.Name, "target": c.Target, "status": c.Status, "message": c.Message,
			"at": c.At.UTC().Format(time.RFC1123), "downtime": "", "statusPage": env("PUBLIC_URL", "http://localhost:"+env("PORT", "3000")),
		}
		if c.Downtime > 0 {
			props["downtime"] = c.Downtime.Round(time.Second).String()
		}
		m := mail.New(smtp, from)
		msg, err := m.Template(ctx, application, "MonitorAlert", props)
		if err != nil {
			return err
		}
		msg.To = to
		return m.Send(ctx, msg)
	}
	if err := application.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// sessionSecret reads SESSION_SECRET, or a secret kept next to the
// database, created on first start, so sign-ins survive restarts.
func sessionSecret(dbPath string) (string, error) {
	if s := os.Getenv("SESSION_SECRET"); s != "" {
		return s, nil
	}
	file := dbPath + ".secret"
	if b, err := os.ReadFile(file); err == nil && len(b) >= 32 {
		return string(b), nil
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	s := hex.EncodeToString(b)
	return s, os.WriteFile(file, []byte(s), 0o600)
}

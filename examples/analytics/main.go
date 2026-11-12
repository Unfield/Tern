// Command analytics is a self-hosted, privacy-friendly web analytics app:
// a small script counts page views without cookies, and a live dashboard
// shows them. See README.md.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Unfield/Tern/jobs"
	"github.com/Unfield/Tern/kit"
	"github.com/Unfield/Tern/logging"
	"github.com/Unfield/Tern/mail"
	"github.com/Unfield/Tern/migrate"
	"github.com/Unfield/Tern/plugins/compress"
	"github.com/Unfield/Tern/plugins/health"
	"github.com/Unfield/Tern/plugins/ratelimit"
	"github.com/Unfield/Tern/plugins/requestlog"
	"github.com/Unfield/Tern/plugins/secure"
	"github.com/Unfield/Tern/plugins/sessions"

	"analytics/internal/app"
	"analytics/internal/store"
	"analytics/migrations"
)

func main() {
	logger := logging.New(logging.Options{})
	path := env("DATABASE_PATH", "analytics.db")
	st, err := store.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	app.Store = st

	q, err := jobs.New(st.DB, jobs.Options{Logger: logger})
	if err != nil {
		log.Fatal(err)
	}
	retention, _ := strconv.Atoi(env("RETENTION_DAYS", "180"))
	// Totals per day, so charts stay fast and survive pruning.
	must(q.Schedule("rollup", "@hourly", func(ctx context.Context) error {
		return st.Rollup(ctx, time.Now().AddDate(0, 0, -2))
	}))
	// Raw page views are kept for RETENTION_DAYS; the day's secret for
	// visitor hashes only until the day is over.
	must(q.Schedule("prune", "@daily", func(ctx context.Context) error {
		if err := st.Rollup(ctx, time.Now().AddDate(0, 0, -retention-1)); err != nil {
			return err
		}
		n, err := st.Prune(ctx, time.Now().AddDate(0, 0, -retention))
		logger.Info("pruned old page views", "deleted", n)
		if err != nil {
			return err
		}
		return st.ForgetSalts(ctx, time.Now())
	}))

	secret, err := sessionSecret(path)
	if err != nil {
		log.Fatal(err)
	}
	var proxies []string
	if p := os.Getenv("TRUSTED_PROXIES"); p != "" {
		proxies = strings.Split(p, ",")
	}
	application, err := ternApp(
		kit.WithLogger(logger),
		// Behind a reverse proxy, visitors' addresses come from it.
		kit.WithTrustedProxies(proxies...),
		kit.WithHook(app.LoadUser),
		// Routes with `var Access = kit.SignedIn` send signed-out visitors to
		// the login page.
		kit.WithSignedIn(func(ctx *kit.Ctx) error {
			if _, ok := app.CurrentUser(ctx); !ok {
				return kit.Redirect(http.StatusSeeOther, "/login?next="+url.QueryEscape(ctx.URL().RequestURI()))
			}
			return nil
		}),
		kit.WithPlugins(
			migrate.Plugin(st.DB, migrations.FS),
			seedPlugin{demo: os.Getenv("DEMO") == "1"},
			q,
			sessions.New(secret, sessions.MaxAge(30*24*time.Hour)),
			// Tracked sites post page views from their own origin.
			collectPlugin{},
			ratelimit.New(ratelimit.Rate(120, time.Minute)),
			health.New(health.Check("database", st.Ping)),
			requestlog.New(requestlog.SkipAssets()),
			compress.New(),
			secure.New(),
		),
	)
	if err != nil {
		log.Fatal(err)
	}

	// The weekly report renders src/emails/WeeklyReport.svelte. Without
	// SMTP_HOST it's logged instead of sent.
	var transport mail.Transport = &mail.Log{Logger: logger}
	if host := os.Getenv("SMTP_HOST"); host != "" {
		port, _ := strconv.Atoi(env("SMTP_PORT", "587"))
		transport = mail.SMTP{Host: host, Port: port, Username: os.Getenv("SMTP_USERNAME"), Password: os.Getenv("SMTP_PASSWORD")}
	}
	mailer := mail.New(transport, env("MAIL_FROM", "analytics@localhost"))
	must(q.Schedule("weekly-report", "0 8 * * 1", func(ctx context.Context) error {
		return sendReports(ctx, application, mailer, st)
	}))

	if err := application.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

// collectPlugin lets other sites post page views: the endpoint uses no
// cookies, so cross-origin protection has nothing to protect there.
type collectPlugin struct{}

func (collectPlugin) Name() string { return "collect" }

func (collectPlugin) Setup(s *kit.Setup) error {
	s.AllowCrossOrigin("POST /api/event")
	return nil
}

// seedPlugin adds a demo site with a month of visits when DEMO=1.
type seedPlugin struct{ demo bool }

func (seedPlugin) Name() string { return "demo" }

func (p seedPlugin) Setup(s *kit.Setup) error {
	if !p.demo {
		return nil
	}
	s.OnStart(func(ctx context.Context) error {
		site, err := app.Store.SiteByDomain(ctx, "demo.example")
		if errors.Is(err, store.ErrNotFound) {
			site, err = app.Store.CreateSite(ctx, "demo.example", "Demo site")
		}
		if err != nil {
			return err
		}
		if err := app.Store.SeedDemo(ctx, site, time.Now()); err != nil {
			return err
		}
		return app.Store.Rollup(ctx, time.Now().AddDate(0, 0, -31))
	})
	return nil
}

func sendReports(ctx context.Context, application *kit.App, m *mail.Mailer, st *store.Store) error {
	users, err := st.ReportRecipients(ctx)
	if err != nil || len(users) == 0 {
		return err
	}
	sites, err := st.Sites(ctx)
	if err != nil {
		return err
	}
	week := store.LastDays(7, time.Now())
	type row struct {
		Name      string  `json:"name"`
		Domain    string  `json:"domain"`
		Visitors  int     `json:"visitors"`
		Pageviews int     `json:"pageviews"`
		Change    float64 `json:"change"`
	}
	var rows []row
	for _, s := range sites {
		now, err := st.Summary(ctx, s.ID, week)
		if err != nil {
			return err
		}
		before, err := st.Summary(ctx, s.ID, week.Previous())
		if err != nil {
			return err
		}
		change := 0.0
		if before.Visitors > 0 {
			change = float64(now.Visitors-before.Visitors) * 100 / float64(before.Visitors)
		}
		rows = append(rows, row{s.Name, s.Domain, now.Visitors, now.Pageviews, change})
	}
	for _, u := range users {
		msg, err := m.Template(ctx, application, "WeeklyReport", map[string]any{"name": u.Name, "sites": rows, "url": env("PUBLIC_URL", "http://localhost:"+env("PORT", "3000"))})
		if err != nil {
			return err
		}
		msg.To = []string{u.Email}
		if err := m.Send(ctx, msg); err != nil {
			return err
		}
	}
	return nil
}

func must(err error) {
	if err != nil {
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

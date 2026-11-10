# Status

A self-hosted status page and uptime monitor built with [Tern](https://github.com/Unfield/Tern), in the spirit of Uptime Kuma. One Go binary with an embedded SQLite database.

- **Monitors.** HTTP(S) with accepted status codes and an optional keyword, TCP ports, DNS names and TLS certificate expiry. Each has its own interval, timeout and number of retries before it counts as down.
- **Live dashboard.** Up/down counts, the last 40 checks of every monitor, uptime and response times. New checks arrive over server-sent events, so nothing needs a reload.
- **Monitor pages.** A response-time chart for 24 hours, 7 days or 30 days, uptime, certificate expiry, the status-change history, and pause, resume and check-now buttons.
- **Public status page.** An overall banner, 90-day uptime bars per monitor grouped by service, and open and recent incidents, all updating live.
- **Incidents.** Opened automatically when a public monitor goes down and resolved when it recovers. Post your own incidents and maintenance with Markdown updates.
- **Notifications.** Webhooks, Slack, Discord and email (SMTP), sent by background jobs with retries. A test button, and a log of recent deliveries with a retry button.
- **Badges.** `/api/badge/<id>` shows a monitor's status, and `?style=uptime` its 30-day uptime, as an SVG for READMEs.
- **Accounts.** The first visit creates the administrator. Passwords are hashed with bcrypt, and sessions are encrypted cookies.

## Run it

```sh
npm install
go tool tern build
./build/status
```

Open <http://localhost:3000> and create the administrator account. Settings come from the environment:

| Variable | Default | |
|---|---|---|
| `PORT` | `3000` | where to listen |
| `DATABASE_PATH` | `status.db` | the SQLite database; a `.secret` file next to it holds the session key |
| `SESSION_SECRET` | | the session key, instead of the file |

For development, run `go tool tern dev`.

## How it's built

| Feature | Tern API |
|---|---|
| Schema in `migrations/`, applied on start | `migrate.Plugin` |
| Checks every N seconds, in a goroutine per monitor | `internal/monitor`, started with `Setup.OnStart` |
| Live updates to the dashboard and status page | `realtime.Hub`, `realtime.Stream`, and `stream()` in Svelte |
| Notifications with retries; pruning old checks daily | `jobs.Handle`, `q.Schedule("prune-heartbeats", "@daily", …)` |
| Sign-in | `plugins/sessions` |
| Incident updates in Markdown | `content.Render`, which escapes HTML |
| Forms that work without JavaScript, with field errors | `kit.Actions`, `kit.Fail`, `use:enhance` |
| Health checks, access logs, gzip, security headers | `plugins/health`, `requestlog`, `compress`, `secure` |

Relative times ("checked 5s ago") start from the server's clock, passed in the page data, so the server-rendered HTML and the hydrated page agree; they tick on in the browser.

## Tests

```sh
go test ./...            # checks, retries, incidents
npx playwright test      # the whole app, in a browser
```

The end-to-end tests run the app with `STATUS_TEST_MODE=1`. In that mode it serves a target whose status code the tests set (`/api/test/target`) and a webhook receiver (`/api/test/webhook`), so they can cause an outage and see the notification arrive.

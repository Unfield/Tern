# Analytics

A self-hosted, privacy-friendly web analytics app, built with Tern. A small script counts page views without cookies; a live dashboard shows visitors, pages, sources, browsers and devices.

```sh
npm install
DEMO=1 npm run dev     # with a demo site and a month of visits
```

Open <http://localhost:5173>, create the owner account, and add your site. Then put the script on every page of it:

```html
<script defer data-domain="example.com" src="https://analytics.example.com/script.js"></script>
```

## What it shows of Tern

- **Form validation from Go:** setup, sign-in and new sites use `kit.FormAction` with `validate` tags, checked as you type through `./$forms` and Tern UI's `Form`.
- **Live components:** `src/lib/LiveVisitors.svelte` counts visitors right now, updated by each page view through `goscript.Publish`.
- **Tern UI:** `Sidebar`, `Stat`, `AreaChart`, `BarChart`, `DonutChart`, `DataTable`, `SignIn`, `SignUp`, `Dialog` and `AlertDialog`.
- **Jobs:** hourly totals, daily pruning, and a weekly email report rendered from `src/emails/WeeklyReport.svelte`.
- **Plugins:** sessions, rate limiting, health checks, compression, security headers, and a two-line plugin of its own that lets other sites post page views.
- **Typed endpoints:** `GET /api/sites/{domain}/stats` is in `/api/openapi.json`.
- **One binary:** `npm run build` writes `build/analytics`.

## Privacy

- **No cookies, no stored addresses:** a visitor is a hash of the day's secret, the site, the IP address and the browser. The secret is deleted when the day ends, so nobody can be followed across days.
- **Referrers:** only the host is kept.
- **Bots:** crawlers and monitors aren't counted.
- **Localhost:** views from localhost aren't counted unless the script has `data-local`.

## Settings

| Variable | Default | |
|---|---|---|
| `DATABASE_PATH` | `analytics.db` | the SQLite database |
| `RETENTION_DAYS` | `180` | how long page views are kept; totals per day stay |
| `TRUSTED_PROXIES` | | comma-separated proxy addresses or CIDRs, so visitors' addresses come through |
| `SMTP_HOST`, `SMTP_PORT`, `SMTP_USERNAME`, `SMTP_PASSWORD`, `MAIL_FROM` | | for the weekly report; without them it's logged |
| `PUBLIC_URL` | `http://localhost:$PORT` | links in emails |
| `SESSION_SECRET` | kept next to the database | |
| `DEMO` | | `1` adds a demo site with a month of visits |

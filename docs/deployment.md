# Deployment

`tern build` produces one static binary that contains the server, the client assets, prerendered pages and the server renderer. Deploying a Tern app means running that binary.

```sh
npm run build          # or: go tool tern build
PORT=8080 ./build/my-app
```

The binary needs no Node.js, no files next to it, and no cgo, so it cross-compiles like any Go program:

```sh
GOOS=linux GOARCH=arm64 go tool tern build -o build/my-app-linux-arm64
```

## tern deploy

`tern deploy` builds the app and puts it on a server, on Fly.io, or in a container registry. Targets are named in `tern.json`:

```json
{
  "deploy": {
    "production": {
      "provider": "ssh",
      "host": "root@203.0.113.5",
      "domains": ["example.com", "www.example.com"],
      "envFile": ".env.production"
    }
  }
}
```

```sh
go tool tern deploy              # the only target, or: tern deploy production
go tool tern deploy -rollback    # back to the release before
go tool tern deploy -releases    # list the releases on the target
go tool tern deploy -logs        # follow the app's logs
```

Without a `tern.json` entry, `tern deploy -host root@203.0.113.5 -domain example.com` deploys over SSH all the same.

Every target takes these fields:
- `provider` is required: `ssh`, `fly`, `container`, or the name of an external provider.
- `name` is the app's name on the target. The default is the project directory's.
- `env` and `envFile` set the app's environment. `env` wins over the file. Keep the file out of git.
- `domains` are the app's domains.

### To a server over SSH

The `ssh` provider needs a Linux server with systemd, any of the common distributions, and `ssh` on your machine. It uses your `~/.ssh/config`, keys and agent, as `ssh` in a terminal does. On every deploy it:
1. Connects and finds the server's architecture, then builds for it (`linux/amd64`, `arm64`…).
2. Creates a system user for the app and a systemd unit, `<name>@.service`, the first time. It needs root or sudo without a password.
3. Uploads the binary, compressed, as a new release in `/srv/<name>/releases/`, with the target's environment in the release's own `.env`. A release that fails, or a later restart, never sees another release's settings. `/srv/<name>/shared` is the app's working directory, kept across releases: put SQLite databases there. Variables you keep on the server, outside `tern.json`, go in `shared/.env`; the release's win over them.
4. Starts the new release next to the running one, on the same port, and waits until it reports that it is ready: its migrations and start functions have run and it is listening. Only then is the old release stopped, and it finishes the requests it has. No request is refused while this happens.
5. Keeps the last five releases for `-rollback`.

If the new release doesn't start, its log is shown, it is stopped and removed, and the old release keeps serving.

With `domains`, the app serves HTTPS itself on ports 443 and 80, with certificates from Let's Encrypt ([HTTPS without a proxy](#https-without-a-proxy)), so the server needs nothing but systemd. Point the domains' DNS at the server first. Without domains, the app listens on port 3000 (`appPort`), for a proxy in front.

The ssh provider's own fields:

| Field | Default | |
|---|---|---|
| `host` | | Where to connect: `user@address`, or a Host from `~/.ssh/config` |
| `port`, `identity`, `sshArgs` | | ssh's `-p`, `-i`, and more arguments |
| `dir` | `/srv/<name>` | Where the app lives on the server |
| `user` | `<name>` | The system user that runs the app |
| `appPort` | `3000` | The port without domains |
| `keep` | `5` | Releases kept |

The unit runs the app with `ProtectSystem=strict`: it can write only to `shared`, and binds ports below 1024 through `CAP_NET_BIND_SERVICE` alone.

### To Fly.io

```json
"fly": { "provider": "fly", "app": "my-app", "region": "fra" }
```

The `fly` provider needs [flyctl](https://fly.io/docs/flyctl/install/), signed in. It writes a `fly.toml` and a `Dockerfile` if the project has none, creates the app if it doesn't exist, stages `env` as secrets, and runs `fly deploy`. `-releases` and `-logs` work too.

### To a container registry

```json
"image": {
  "provider": "container",
  "image": "ghcr.io/you/my-app",
  "after": ["gcloud run deploy my-app --image {image} --region europe-west1"]
}
```

The `container` provider builds the `Dockerfile` with docker or podman, for `linux/amd64` (`platform`), tags the image with the release's version and `latest`, and pushes both (`noPush` skips that). `after` runs commands once the image is pushed, with `{image}` replaced: the deploy command of Cloud Run, ECS, Azure Container Apps, Kubernetes or Railway.

### Providers of your own

Any other `provider` value runs a program named `tern-deploy-<provider>` from the `PATH`. A hosting platform can ship its own provider this way. The program gets the deployment as JSON on its standard input, and its output is shown:

```json
{
  "action": "deploy",
  "target": "production",
  "name": "my-app",
  "root": "/home/you/my-app",
  "version": "v1.4.0-3-g1a2b3c4",
  "env": { "DATABASE_URL": "…" },
  "domains": ["example.com"],
  "config": { "provider": "acme", "project": "p_123" }
}
```

- `action` is `deploy`, `rollback`, `releases` or `logs`.
- `config` is the target's entry from `tern.json`, with any fields the provider wants.
- The program builds the app as it needs. `GOOS=linux GOARCH=amd64 go tool tern build -o …` cross-compiles, or it can use the Dockerfile.
- A nonzero exit status fails the deploy.

## Configuration

| Variable | Default | Meaning |
|---|---|---|
| `PORT` | `3000`, or `443` with HTTPS | Port to listen on |
| `HOST` | all interfaces | Address to listen on, e.g. `127.0.0.1` behind a local proxy |
| `TERN_TLS_DOMAINS` | | Serve HTTPS for these domains, comma-separated, with certificates from Let's Encrypt (see [HTTPS without a proxy](#https-without-a-proxy)) |
| `TERN_TLS_EMAIL`, `TERN_TLS_CACHE` | none, `tern-certs` | The email given to Let's Encrypt, and the directory certificates are kept in |
| `TERN_LOG_LEVEL`, `TERN_LOG_FORMAT` | `info`, JSON | See [logging](logging.md) |

Everything else is configured in `main.go` with `kit` options:

```go
app, err := ternApp(
	kit.WithLogger(slog.New(slog.NewJSONHandler(os.Stdout, nil))), // structured logs
	kit.WithTrustedOrigins("https://admin.example.com"),          // cross-origin form posts
	kit.WithBodyLimit(10<<20),                                     // 10 MiB uploads
	kit.WithSSRWorkers(4),                                         // concurrent server renders
)
```

## Behavior in production

- **Shutdown.** On `SIGINT` or `SIGTERM` the server stops accepting connections and lets in-flight requests finish for up to 10 seconds.
- **Caching.** Content-hashed assets under `/_app/immutable/` are served with `Cache-Control: public, max-age=31536000, immutable`. Other static files and prerendered pages use ETags and revalidate. Dynamic pages are `private, no-cache`.
- **Compression.** Text assets are precompressed with gzip at build time and served to clients that accept it.
- **Security.** Cross-origin POST, PUT, PATCH and DELETE requests from browsers are rejected (`http.CrossOriginProtection`). Request bodies for actions and endpoints are limited to 1 MiB by default. Every response has `X-Content-Type-Options: nosniff`. Unexpected errors are logged and shown to users only as "Internal Error".
- **Server rendering.** `tern build` compiles the server renderer to Go, and pages and emails render natively, one worker per CPU by default. What the compiler leaves to the JavaScript engine (listed in `.tern/native/report.txt`) renders in embedded QuickJS VMs, as does any page whose native render fails. A render that fails both ways or takes longer than 5 seconds falls back to client rendering and is logged, so one broken component cannot take a page down.
- **Checking native rendering.** `TERN_NATIVE_VERIFY=1` renders every page both ways, serves the JavaScript engine's HTML and logs where native rendering differs. `TERN_NATIVE=0` turns native rendering off, at run time or for `tern build`; `"native": false` in `tern.json` does too. One difference is expected: a page that prints a function's source (`fn.toString()`, as mode-watcher's theme script does) gets the original source natively and the minified bundle's from the engine. Both run the same.
- **Binaries without the JavaScript engine.** When the compiler leaves nothing to the engine (the report is empty), `tern build` leaves QuickJS out of the binary, a tenth of its size, and says so: "Compiling the server without the JavaScript engine". Pages, emails and live components all render natively. Should a render still fail natively, the page renders in the browser instead, and is logged. `TERN_KEEP_ENGINE=1 tern build` keeps the engine, which `TERN_NATIVE_VERIFY` needs; so does a `-tags` of your own in `GOFLAGS`.

## Behind a reverse proxy

Tern works behind nginx, Caddy, Traefik, or a cloud load balancer:

- The proxy must pass the `Host` header through unchanged.
- If the proxy terminates TLS, it must set `X-Forwarded-Proto: https`. Tern uses it to mark cookies `Secure` and to build absolute URLs for server rendering.
- Leave `Origin` and `Sec-Fetch-Site` intact; cross-origin protection reads them.
- An `X-Request-Id` the proxy sets is kept as the request's ID in the logs.
- Tell Tern which addresses are your proxies, so it can find the client's real IP:

```go
app, err := ternApp(kit.WithTrustedProxies("127.0.0.1", "10.0.0.0/8"))
```

- **`ctx.ClientIP()`:** returns the client's address; middleware can call `kit.ClientIP(r)`. The rate limiting plugin uses it.
- **Which address it picks:** if the request came from a trusted proxy, it reads `X-Forwarded-For` from the right and returns the first address that isn't a proxy. Entries further left were written by the client and could be anything, so they're never used. A request from any other peer gets its own address.
- **`X-Forwarded-Proto`:** once proxies are configured, it is only believed from them.
- **Without the option:** `ClientIP` is the proxy's address, and `X-Forwarded-Proto` is believed from anyone.

Example Caddyfile:

```
example.com {
	reverse_proxy 127.0.0.1:3000
}
```

## HTTPS without a proxy

A Tern app can serve HTTPS itself, with certificates from Let's Encrypt that it gets and renews on its own. No nginx or Caddy is needed in front:

```go
app, err := ternApp(kit.WithAutoTLS(kit.AutoTLS{Domains: []string{"example.com", "www.example.com"}, Email: "ops@example.com"}))
```

or, with no code, `TERN_TLS_DOMAINS=example.com,www.example.com`.

- HTTPS is served on port 443, and plain HTTP on port 80. Port 80 answers Let's Encrypt's challenges and redirects everything else to HTTPS.
- The machine must be reachable at those domains. Certificates are kept in `tern-certs` (`AutoTLS.Cache`), which must survive restarts, or Let's Encrypt's rate limits stop new ones.
- Ports below 1024 need privileges: on Linux, `setcap 'cap_net_bind_service=+ep' ./app`, or `AmbientCapabilities=CAP_NET_BIND_SERVICE` in a systemd unit.

`kit.WithTLS(cfg)` serves HTTPS with certificates of your own. HTTP/2 is offered in both cases. Serving TLS itself lets the app see the client's TLS fingerprint, which [security signals](signals.md) use.

## Docker

Projects created with `tern new` include a `Dockerfile` that builds in a Go + Node image and ships the binary on `distroless/static`:

```sh
docker build -t my-app .
docker run -p 3000:3000 my-app
```

The final image contains only the binary and runs as a non-root user.

## systemd

```ini
# /etc/systemd/system/my-app.service
[Unit]
Description=my-app
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=/opt/my-app/my-app
Environment=PORT=3000 HOST=127.0.0.1
User=my-app
Restart=on-failure
# Hardening
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true

[Install]
WantedBy=multi-user.target
```

```sh
sudo systemctl enable --now my-app
```

## Fly.io

```toml
# fly.toml
app = "my-app"

[build]
  dockerfile = "Dockerfile"

[http_service]
  internal_port = 3000
  force_https = true

[[http_service.checks]]
  method = "GET"
  path = "/"
  interval = "15s"
  timeout = "5s"
```

## Health checks

Add an endpoint for your platform's health checks:

```go
// src/routes/healthz/server.go
package healthz

import "github.com/Unfield/Tern/kit"

func GET(ctx *kit.Ctx) error { return ctx.Text(200, "ok") }
```

## Serverless and edge platforms

Tern apps are long-running HTTP servers. Platforms that run containers (Cloud Run, Fly.io, Railway, Render, ECS, Kubernetes) work out of the box. Function platforms that expect a request handler can mount the app, since `*kit.App` is an `http.Handler`, using an adapter such as `aws-lambda-go-api-proxy`.

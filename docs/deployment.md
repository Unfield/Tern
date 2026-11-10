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

## Configuration

| Variable | Default | Meaning |
|---|---|---|
| `PORT` | `3000` | Port to listen on |
| `HOST` | all interfaces | Address to listen on, e.g. `127.0.0.1` behind a local proxy |

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
- **Server rendering.** `tern build` compiles the server renderer to Go, and pages render natively, one worker per CPU by default. What the compiler leaves to the JavaScript engine (listed in `.tern/native/report.txt`) renders in embedded QuickJS VMs, as does any page whose native render fails. A render that fails both ways or takes longer than 5 seconds falls back to client rendering and is logged, so one broken component cannot take a page down.
- **Checking native rendering.** `TERN_NATIVE_VERIFY=1` renders every page both ways, serves the JavaScript engine's HTML and logs where native rendering differs. `TERN_NATIVE=0` turns native rendering off, at run time or for `tern build`; `"native": false` in `tern.json` does too. One difference is expected: a page that prints a function's source (`fn.toString()`, as mode-watcher's theme script does) gets the original source natively and the minified bundle's from the engine. Both run the same.

## Behind a reverse proxy

Tern works behind nginx, Caddy, Traefik, or a cloud load balancer:

- The proxy must pass the `Host` header through unchanged.
- If the proxy terminates TLS, it must set `X-Forwarded-Proto: https`. Tern uses it to mark cookies `Secure` and to build absolute URLs for server rendering.
- Leave `Origin` and `Sec-Fetch-Site` intact; cross-origin protection reads them.
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

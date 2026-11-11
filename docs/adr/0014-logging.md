# 0014. Logging

- Status: Accepted
- Date: 2026-11-11

## Context

Tern apps logged with `log/slog` and nothing more. Running one in production needs more than that:
- a way to follow one request through its records;
- secrets kept out of the logs;
- protection against a loop flooding them;
- a way to get the records to wherever the team reads logs.

During development, records were mixed into the terminal with Vite's and the compiler's output.

The choice was between a logging API of Tern's own, and building on slog. Apps, libraries and plugins already log through slog, and anything else would need adapters both ways.

## Decision

- **Build on slog.** `logging.New` returns a `*slog.Logger`. Its handler does the work, and `slog.SetDefault` gives it to everything that logs with slog or `log`. Apps keep using `slog.InfoContext` and friends.
- **Request context in the context.** kit gives every request an ID, the trusted `X-Request-Id` or a new one, and reads a trusted `traceparent`. It puts them in the request's `context.Context` as attributes, which the handler adds to records logged with that context. `Ctx.Logger` binds them for code that logs without one. kit wraps a handler it didn't make with `logging.Wrap`, so a plain `slog.Default()` gets them too.
- **Redaction and sampling in the handler,** before formatting or shipping, so no output or sink can see what they remove. Redaction goes by key: secrets are logged under a few dozen predictable names. A string inside a message can't be recognized reliably, so it isn't tried. Sampling counts records by level and message in a fixed table, so its memory is bounded whatever the messages.
- **Sinks for shipping.** A sink is one method, `Export(ctx, []Entry) error`, called with batches from a goroutine per sink. A bounded queue sits between the handler and the sink: logging never blocks on the network, and a sink that is down costs dropped records, which are counted, not memory. The four sinks talk HTTP and JSON with the standard library only: OTLP, Loki, Datadog and a rotating file. The OpenTelemetry SDK would add a large dependency tree for one exporter.
- **The dev log view in tern dev.** Under tern dev, loggers send their records to tern dev over a loopback port with a token. tern dev adds the requests it proxies, which carry the request ID, and serves both at `/__tern/dev/logs`. The view lives in tern dev rather than in the app, so it survives the app's restarts.

## Consequences

- Every response has an `X-Request-Id` header, and every record logged in a request names it.
- Records logged with `slog`'s package functions get the request's ID only with a context (`slog.InfoContext(c, …)`) or through `c.Logger()`. `slog.Info` has no request to go by.
- Records that a sink couldn't take, after retries or with its queue full, are lost. `logging.StatsOf` counts them, and a line on stderr says so at most once a minute. Logs are not an audit trail: `accounts` keeps its own tables for that.
- New sinks don't need Tern: any type with `Export` is one.

# Logging

Tern logs with the standard library's `log/slog`. The `logging` package adds what a running app needs: the request's ID on every record, secrets redacted, floods sampled, and records shipped to a collector or log service in the background. Under `tern dev`, records show up in a log view in the browser.

```go
import (
	"log/slog"

	"github.com/Unfield/Tern/kit"
	"github.com/Unfield/Tern/logging"
	"github.com/Unfield/Tern/logging/otlp"
)

func main() {
	logger := logging.New(logging.Options{
		Sinks: []logging.Sink{otlp.New(otlp.Options{})},
	})
	slog.SetDefault(logger)
	server, err := ternApp(kit.WithLogger(logger))
	// …
}
```

`slog.SetDefault` makes `slog.Info` and the rest use it, and the standard `log` package too. `kit.WithLogger` gives it to kit and to plugins. When the server shuts down, kit waits for the sinks to send what is queued, for up to the shutdown timeout.

## Level and format

`TERN_LOG_LEVEL` sets the lowest level logged: `debug`, `info` (the default), `warn` or `error`. `TERN_LOG_FORMAT` is `text` or `json`. Without it, output is text on a terminal and under `tern dev`, and JSON otherwise, as log collectors read it.

`Options.Level` and `Options.Format` set them in code instead. Pass a `*slog.LevelVar` as the level to change it while the app runs. `Options.Output` is where formatted records go, `os.Stderr` by default. Set it to `io.Discard` to send records only to sinks.

## Requests

Every request gets an ID. kit adds it to the request's context, sets it as the `X-Request-Id` response header, and every record logged with that context includes it as `request_id`:

```go
func Load(c *kit.Ctx) (Data, error) {
	slog.InfoContext(c, "loading the cart")     // … request_id=3f9a2c1e8b7d6a54
	c.Logger().Info("loading the cart")         // the same, plus route=/cart
	return Data{}, nil
}
```

`c.Logger()` returns the app's logger with the request's attributes and route already bound, so records logged with it carry them even without a context. Plugins that log with the request's context, such as `plugins/requestlog`, include the ID too.

An incoming `X-Request-Id` is kept when the request comes from a peer whose forwarding headers are trusted (see `kit.WithTrustedProxies`), so a load balancer's ID follows the request into your logs. The same goes for a W3C `traceparent` header: its trace and span IDs are added as `trace_id` and `span_id`, and the OTLP and Datadog sinks send them so the logs join the traces.

Add attributes of your own with `c.LogWith`, which adds them to the request's context and `c.Logger()`. A hook is a good place, for the signed-in user say:

```go
kit.WithHook(func(c *kit.Ctx) error {
	if u, ok := c.Locals["user"].(*User); ok { // set by the hook that reads the session
		c.LogWith(slog.Int64("user", u.ID))
	}
	return nil
})
```

Outside a request, `logging.With(ctx, attrs...)` does the same for any context.

A logger that wasn't made by `logging.New`, such as `slog.Default()` when none was set, still gets the request's attributes: kit wraps its handler with `logging.Wrap`.

## Redaction

Values whose key is a password, token, secret, API key, cookie, session or the like are written as `[redacted]`, wherever they are: record attributes, `With` attributes, groups and the context's. The full list is `logging.DefaultRedact`. Keys match without regard to case, with `-` and `_` the same, and keys ending in `_token`, `_secret` or `_password` match too.

- `Options.Redact` adds keys, and `Options.Keep` takes some out of the list.
- `logging.Secret(s)` is a string that always logs as `[redacted]`, whatever its key.
- An `http.Header` logs as a group of its headers, with `Authorization` and `Cookie` redacted.
- A `*url.URL` logs without its password and with the values of query parameters such as `token` redacted.

Redaction catches the common mistakes. It can't recognize a secret inside a message string, so don't build messages from secrets.

## Sampling

A bug in a loop can log the same line thousands of times a second, which drowns everything else and costs money to ship. With `Options.Sampling` set, the first 100 records a second with the same level and message are kept, then one in every 100:

```go
logging.New(logging.Options{Sampling: &logging.Sampling{}})                               // the defaults
logging.New(logging.Options{Sampling: &logging.Sampling{First: 10, Thereafter: 1000}})
```

Only records at `Info` and below are sampled (`MaxLevel` changes that). Warnings and errors are always kept. `logging.StatsOf(logger)` counts what sampling left out, and what sinks dropped or failed to send.

## Sinks

A sink sends records somewhere. Each sink gets records in batches, from a goroutine of its own, so logging never waits on the network. Tern has four:

| Package | Sends to |
|---------|----------|
| `logging/otlp` | An OpenTelemetry collector, or any service that takes OTLP over HTTP: Grafana Cloud, Honeycomb, New Relic, SigNoz, Axiom and others. Reads the standard `OTEL_EXPORTER_OTLP_*` variables, `OTEL_SERVICE_NAME` and `OTEL_RESOURCE_ATTRIBUTES`. |
| `logging/loki` | Grafana Loki, as JSON lines in streams labeled by level. Reads `LOKI_URL`, `LOKI_USERNAME`, `LOKI_PASSWORD` and `LOKI_TENANT_ID`. |
| `logging/datadog` | Datadog's log intake. Reads `DD_API_KEY`, `DD_SITE`, `DD_SERVICE`, `DD_ENV`, `DD_VERSION` and `DD_TAGS`. |
| `logging/file` | A file of JSON lines, rotated by size, with old files kept, removed or gzipped. |

```go
logging.New(logging.Options{
	Sinks: []logging.Sink{
		loki.New(loki.Options{Labels: map[string]string{"app": "shop"}}),
		logging.MinLevel(file.New(file.Options{Path: "logs/errors.log"}), slog.LevelError),
	},
})
```

`logging.MinLevel` sends a sink only the records at a level or above. `Options.Batch` sets how batches are made:
- Size is 512 records and Interval is 1 second.
- Queue is 16384 records. When it is full, new records are dropped and counted.
- A failed batch is retried up to MaxRetries times, 5 by default, with growing pauses, and a service's `Retry-After` is honored.
- A request the service rejects, such as a bad API key, isn't retried. A line on stderr says so, at most once a minute.

A sink of your own is a type with an `Export` method:

```go
type Sink interface {
	Export(ctx context.Context, entries []logging.Entry) error
}
```

An `Entry` has the record's time, level, message and attributes, resolved and redacted, with groups nested. `Entry.Walk` visits them with dotted keys, and `logging.EncodeJSON` turns entries into plain JSON objects. Return `logging.Permanent(err)` for errors that retrying won't fix, and `logging.RetryAfter(err, d)` to ask for a pause. A sink that is an `io.Closer` is closed by `logging.Close`.

## The log view in tern dev

`tern dev` prints a third address next to Local and Network: `http://localhost:5173/__tern/dev/logs`. The page shows, live:
- the app's records, sent by `logging.New`'s loggers and by kit's logger;
- each request `tern dev` passes to the app, with its status and time.

Click a request ID to see only that request's records and the request itself, and click a row to see all of its attributes. You can filter by level or text and hide asset requests. It keeps the last 5000 rows, across restarts of the app. Records go to `tern dev` over a loopback port with a token, and never leave the machine.

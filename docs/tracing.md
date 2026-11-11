# Tracing

A trace is what one request did, as a tree of timed spans: the request, each of its loads, the server render, the SQL queries and HTTP calls they made, and the jobs they queued. The `tracing` package records them and exports them over OTLP to an OpenTelemetry collector or a service that takes OTLP: Grafana Tempo, Honeycomb, Jaeger, SigNoz, Datadog and others.

```go
import (
	"github.com/Unfield/Tern/kit"
	"github.com/Unfield/Tern/tracing"
	"github.com/Unfield/Tern/tracing/otlp"
)

func main() {
	tracer := tracing.New(tracing.Options{
		Exporters: []tracing.Exporter{otlp.New(otlp.Options{})},
	})
	server, err := ternApp(kit.WithTracer(tracer))
	// …
}
```

kit makes the tracer the default for work outside requests, such as jobs, unless another one already is (`tracing.SetDefault`). When the server shuts down, kit waits for the exporters to send what is queued.

## What kit records

| Span | Kind | Attributes |
|---|---|---|
| `GET /blog/slug.param`: the request, named after its route | server | `http.request.method`, `url.path`, `http.route`, `http.response.status_code`, `request_id` |
| `load layout /`, `load layout /blog`, `load page` | internal | |
| `action create`, `endpoint POST` | internal | |
| `render`: the server render | internal | |

A load, action or endpoint that returns an error marks its span failed and records the error. Redirects and 4xx errors, such as `NotFound` and `Fail`, are answers, not failures. A 5xx response marks the request's span failed.

A request from a trusted proxy or service (`kit.WithTrustedProxies`) with a W3C `traceparent` header continues that trace, so the app's spans join the caller's.

## Your own spans

```go
ctx, span := tracing.Start(ctx, "charge card", tracing.WithAttrs(slog.String("order", id)))
defer span.End()
if err := charge(ctx); err != nil {
	span.RecordError(err)
	return err
}
```

`ctx` can be the `*kit.Ctx` of a load or action. `span.SetAttrs`, `span.AddEvent` and `span.SetStatus` add to a span while it runs. Every method is safe on a span that isn't recorded: one without a tracer, or one that sampling left out.

## Logs in traces

A record logged inside a span, with its context, carries its `trace_id` and `span_id` (see [logging](logging.md)). Collectors and services join logs to traces by these, so you can go from a slow span to what it logged, or from an error line to its trace.

## Outgoing requests, SQL and jobs

- **HTTP:** `tracing.Transport(nil)` wraps `http.DefaultTransport` (or the transport you pass). Each request becomes a client span, and carries `traceparent` to the service it calls:

  ```go
  client := &http.Client{Transport: tracing.Transport(nil)}
  ```

- **SQL:** `tracing.OpenDB` opens a database as `sql.Open` does, with each query a client span named by its statement (`SELECT`, `INSERT`…) with the query text. Queries run with a context in no trace, such as migrations at startup, record nothing. `tracing.WrapConnector` does the same for `sql.OpenDB`.

  ```go
  db, err := tracing.OpenDB("sqlite", "app.db")
  ```

- **Jobs:** a job queued in a trace runs in that trace. Queuing is a producer span (`enqueue email`), and running it a consumer span (`job email`), with the job's ID and attempt.

## Sampling

`Options.Sample`, or `TERN_TRACE_SAMPLE`, is the fraction of new traces recorded: `0.1` keeps one in ten. A trace that comes in from elsewhere keeps the caller's decision, so a trace is recorded whole or not at all. Spans that aren't recorded still carry their IDs, so the logs keep the trace.

## The waterfall in tern dev

Under `tern dev`, every request is traced, with or without a tracer of your own. Its spans go to the log view (`/__tern/dev/logs`): requests with a trace are marked, and clicking one shows its spans as a waterfall, nested and timed. The dev server's own work, such as collecting a page's CSS, shows there too, as `dev: …` spans.

## Exporters

`tracing/otlp` sends OTLP/HTTP JSON. With no options it reads the standard variables: `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` or `OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_EXPORTER_OTLP_TRACES_HEADERS` or `OTEL_EXPORTER_OTLP_HEADERS`, `OTEL_SERVICE_NAME` and `OTEL_RESOURCE_ATTRIBUTES`. Spans are sent in batches from a goroutine, retried, and dropped (and counted, `tracer.Stats()`) when a collector stays down, as logging's sinks are.

An exporter of your own is one method:

```go
type Exporter interface {
	Export(ctx context.Context, spans []tracing.SpanData) error
}
```

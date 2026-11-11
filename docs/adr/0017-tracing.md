# 0017. Tracing

- Status: Accepted
- Date: 2026-11-11

## Context

Logs say what happened. A trace also says where the time went in a request: which load was slow, which query, whether the render or the database is the problem. OpenTelemetry is the standard for this, and collectors and services all take OTLP.

The OpenTelemetry Go SDK is the obvious choice, but it is large: its API, SDK, exporters and their dependencies, gRPC among them, would come into every Tern app that turned tracing on. It also has its own context propagation, sampling and batching, separate from what `logging` already does ([ADR 0014](0014-logging.md)).

## Decision

- **A small tracing package of Tern's own** implements the part of OpenTelemetry's model apps use: spans with attributes, events and status; kinds; parent-based sampling; and W3C trace context. It exports OTLP/HTTP JSON, which every collector takes. Spans and logs share the batching and retry code (`internal/batch`) and OTLP's encoding (`internal/telemetry`).
- **kit traces requests by itself** when it has a tracer (`kit.WithTracer`, or `tracing.Default`), with spans for each load, action, endpoint and render. A trusted `traceparent` header continues the caller's trace.
- **Spans join logs through the context.** Starting a span puts its `trace_id` and `span_id` in the context's logging attributes, so every record logged under it names it, with no change to how apps log.
- **Wrappers, not hooks,** carry traces out of the process and back. `tracing.Transport` wraps an HTTP transport, `tracing.OpenDB` wraps a `database/sql` driver, and `jobs` stores the `traceparent` with each job, in a new column, and continues it when the job runs.
- **tern dev always traces,** with a tracer that only feeds its log view, which draws each request's waterfall.

## Consequences

- Apps get traces without new dependencies, in the same shape as the SDK's, and any OTLP backend reads them.
- What the SDK has and this doesn't: metrics through OTLP, span links, baggage, and gRPC export. Apps that need them can use the SDK alongside: both speak W3C trace context, so traces cross between them.
- The `tern_jobs` table gains a `trace` column, added by `Migrate` on existing databases.
- Every request under tern dev records a few spans. It costs microseconds and is dev only. In production, tracing costs nothing unless a tracer is set.

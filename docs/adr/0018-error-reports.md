# 0018. Error reports

- Status: Accepted
- Date: 2026-11-11

## Context

kit logs unexpected errors, and logs are where to look when you know something broke. But a log line doesn't say that the same error happened four hundred times since the last deploy, to how many users, or in which release it started. Error trackers do, and Sentry's protocol is what most of them take.

The Sentry SDK hooks into the process on its own terms: its own scope and hub, integrations and transport. Most of what it adds for a Go app is already in Tern: the request context ([ADR 0014](0014-logging.md)), the trace ([ADR 0017](0017-tracing.md)), and kit's own catching of panics.

## Decision

- **kit reports what it would log as unexpected** to a reporter (`kit.WithReporter`): errors from loads, actions and endpoints that aren't answers, panics with the program counters captured where they happened, failed jobs, and live components' panics.
- **The browser reports too.** When the app has a reporter, the page payload says so, and the client sends uncaught errors and unhandled rejections to `/__tern/errors`, with a limit per page and per address.
- **Grouping happens in Tern,** by a fingerprint of the error type, the normalized message and the route, or for panics the app's innermost frames. Groups are rate-limited before anything is sent, and their fingerprint is what Sentry groups by, so both agree.
- **Reports carry the request's context:** its ID, trace and span, and the user and other attributes a hook added with `c.LogWith`. No separate API sets the user.
- **Sinks, with the shared batching.** The Sentry sink speaks the envelope protocol directly, through `internal/telemetry`, the same as the OTLP exporters.
- **tern dev lists the groups** in its log view, from a reporter that only feeds it.

## Consequences

- Apps get grouped error reports, with traces and request IDs, without the Sentry SDK.
- Plain Go errors have no stack unless they're wrapped with `report.WithStack` or made with `report.Errorf`. The report still has the request, route and trace.
- Browser stacks are of the bundled code. Source maps are for a later release.

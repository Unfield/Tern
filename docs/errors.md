# Error reports

When something goes wrong in production, a log line says so once. An error report also says how often, since when, for which users, and where in the code. The `report` package turns an app's unexpected errors into reports, groups them by cause, and sends them to an error tracker: Sentry, or a service that takes its protocol, such as GlitchTip or Bugsink.

```go
import (
	"github.com/Unfield/Tern/kit"
	"github.com/Unfield/Tern/report"
	"github.com/Unfield/Tern/report/sentry"
)

func main() {
	reporter := report.New(report.Options{
		Sinks: []report.Sink{sentry.New(sentry.Options{})}, // SENTRY_DSN
	})
	server, err := ternApp(kit.WithReporter(reporter))
	// …
}
```

## What is reported

- **Errors from loads, actions and endpoints** that aren't answers. `NotFound`, `Fail`, redirects and other 4xx errors are how a page responds; anything else is unexpected and reported.
- **Panics** in loads, actions, endpoints, hooks, live components and jobs, with the stack where they happened.
- **Jobs that fail** their last attempt.
- **Uncaught errors in the browser:** errors and unhandled promise rejections on the app's pages. The client sends them to `/__tern/errors`, at most ten per page and each message once. The server takes at most twenty a minute from one address.

Each report has the error's type and message, the stack where there is one, the request's method, URL (with secrets redacted) and route, its request ID and trace, the release and the environment. If a hook added a user to the logging context, it has that too:

```go
kit.WithHook(func(c *kit.Ctx) error {
	if u := accounts.Current(c); u != nil {
		c.LogWith(slog.Int64("user", u.ID))
	}
	return nil
})
```

## Groups

Reports with the same cause share a group:
- **Errors** are grouped by error type, the message with numbers, IDs and quoted values taken out, and the route.
- **Panics** are grouped by where in the app's code they happened.

Each group is sent at most `PerMinute` times a minute, 10 by default, and the rest are counted, so a failing loop doesn't flood the tracker. The group is Sentry's fingerprint, so Sentry's issues match Tern's groups. `reporter.Groups()` lists the groups the process has seen, with counts.

## Stacks for ordinary errors

Go errors don't carry a stack. Reports of panics have one. For an error you return, wrap it where it happens to see where it came from:

```go
if err != nil {
	return report.WithStack(err)
}
return report.Errorf("charging order %d: %w", id, err)
```

## Release and environment

`Options.Release` names the build, `TERN_RELEASE` by default or the commit Go stamped into the binary, so a tracker can say which release an error first appeared in. `Options.Environment`, or `TERN_ENV`, tells production from staging.

## Errors in tern dev

Under `tern dev`, every app has a reporter, and errors open over the page with their stacks and source ([Dev tools](dev-tools.md)). The log view's **Errors** button opens the groups, with counts, the route and when each last happened; click one for its stack, with the app's own frames in bold. The request ID and trace lead to the request in the log, and to its waterfall.

## Sinks of your own

A sink is one method, called with batches from a goroutine:

```go
type Sink interface {
	Send(ctx context.Context, reports []report.Report) error
}
```

Return `report.Permanent(err)` for errors that retrying won't fix.

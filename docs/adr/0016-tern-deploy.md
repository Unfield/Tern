# 0016. tern deploy

- Status: Accepted
- Date: 2026-11-11

## Context

A Tern app builds to one binary, which makes deploying it simple in principle. In practice it still meant:
- writing a systemd unit and a reverse proxy configuration;
- getting certificates;
- copying the binary;
- restarting without dropping requests;
- keeping the old binary to roll back to.

Platforms differ too: some run a binary on a server, some run containers, some are platforms of their own with their own APIs, and a hosting platform built for Tern apps needs to plug in without Tern knowing about it.

## Decision

- **Targets in `tern.json`, providers behind them.** `tern deploy [target]` reads `deploy.<target>`, whose `provider` picks the code that deploys. Common fields (`name`, `env`, `envFile`, `domains`) mean the same everywhere; the rest of the entry belongs to the provider.
- **Three built-in providers.** `ssh` runs the binary on a Linux server under systemd. `fly` deploys to Fly.io through flyctl. `container` builds and pushes an image, and runs the platform's own deploy command after.
- **External providers by name.** Any other provider runs `tern-deploy-<name>` from the PATH with the deployment as JSON on its standard input, as git does with its subcommands. A platform ships a program, and the protocol is the whole contract: no Go plugin API to keep stable.
- **Zero-downtime restarts with systemd and SO_REUSEPORT,** not a proxy of our own. Each release is an instance of a template unit, `<name>@<release>`. The new instance starts while the old one runs, and both listen on the port with SO_REUSEPORT (`TERN_REUSEPORT=1`). systemd's `Type=notify` waits until kit reports `READY=1`, after the start functions run and the listener is open, and only then is the old instance stopped, through kit's graceful shutdown. A release that never becomes ready is stopped, and nothing changes.
- **The app is the web server.** With domains, the unit runs the app on ports 443 and 80 with `kit.WithAutoTLS` ([ADR 0015](0015-security-signals.md)) and `CAP_NET_BIND_SERVICE`. A server needs nothing but systemd.
- **The system's ssh, not a Go SSH client,** so `~/.ssh/config`, agents, jump hosts and hardware keys work as they do in a terminal. A control socket keeps one connection for the deploy's steps.

## Consequences

- Deploying to a fresh server is one entry in `tern.json` and `tern deploy`. Releases, rollbacks and logs come with it.
- For a few seconds two releases run at once, against the same database. Migrations must be ones the old release survives, which is the rule for any rolling deploy.
- Zero-downtime restarts need Linux. SO_REUSEPORT is also on macOS and the BSDs, but the ssh provider targets systemd.
- Platforms Tern doesn't know about deploy through `tern-deploy-<name>`, and a provider can become a built-in one later without changing its targets.

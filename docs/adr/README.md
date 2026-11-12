# Architecture Decision Records

An ADR captures one significant decision: the context, what was decided, and the consequences. ADRs are not rewritten once accepted; if a decision changes, a new ADR supersedes the old one.

| # | Title | Status |
|---|---|---|
| [0001](0001-use-real-svelte.md) | Use the official Svelte compiler instead of a custom component language | Accepted |
| [0002](0002-client-rendering-first.md) | Ship client-side rendering first, SSR in v0.2 | Accepted |
| [0003](0003-route-files-as-go-packages.md) | Route directories are Go packages, with Go-safe names | Accepted |
| [0004](0004-ssr-with-embedded-quickjs.md) | Server-side rendering with embedded QuickJS | Accepted |
| [0005](0005-selective-reloading.md) | Selective reloading of layout data | Accepted |
| [0006](0006-plugins.md) | A small plugin interface on top of net/http | Accepted |
| [0007](0007-ssr-throughput-and-memory.md) | Server-rendering throughput and memory | Accepted |
| [0008](0008-oauth-and-oidc.md) | OAuth 2.0 and OpenID Connect, as a client and as a provider | Accepted |
| [0009](0009-experimental-go-script.md) | Experimental `<script lang="go">` | Accepted (experimental) |
| [0010](0010-live-components.md) | Live components | Accepted |
| [0011](0011-native-rendering.md) | Native rendering | Accepted |
| [0012](0012-accounts.md) | User accounts | Accepted |
| [0013](0013-native-rendering-phase-2.md) | Native rendering, phase 2 | Accepted |
| [0014](0014-logging.md) | Logging | Accepted |
| [0015](0015-security-signals.md) | Security signals | Accepted |
| [0016](0016-tern-deploy.md) | tern deploy | Accepted |
| [0017](0017-tracing.md) | Tracing | Accepted |
| [0018](0018-error-reports.md) | Error reports | Accepted |
| [0019](0019-teams.md) | Teams and the audit log in accounts | Accepted |
| [0020](0020-billing.md) | Billing with Stripe | Accepted |
| [0021](0021-rendering-on-demand.md) | Rendering on demand and caching | Accepted |
| [0022](0022-keep-boxed-values.md) | Keep boxed values in native rendering | Accepted |
| [0023](0023-apps-over-mcp.md) | Apps over MCP | Accepted |

## Template

```markdown
# NNNN. Title

- Status: Proposed | Accepted | Superseded by NNNN
- Date: YYYY-MM-DD

## Context
## Decision
## Consequences
```

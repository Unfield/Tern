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

## Template

```markdown
# NNNN. Title

- Status: Proposed | Accepted | Superseded by NNNN
- Date: YYYY-MM-DD

## Context
## Decision
## Consequences
```

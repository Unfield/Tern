# 0001. Use the official Svelte compiler

- Status: Accepted
- Date: 2026-11-09

## Context

Tern needs a component model for the client. The options were to design a Svelte-like language with our own compiler (possibly one that also compiles to Go), or to use Svelte itself.

A custom language would make server-side rendering in Go simpler, but it means building and maintaining a compiler, a language server, editor plugins and a component ecosystem, all of which Svelte already has. Previous projects in this space stalled largely from scope like this.

## Decision

Tern uses the official Svelte 5 compiler through Vite. Tern does not fork or patch Svelte. Svelte is a peer dependency of the client package.

## Consequences

- Users get the full Svelte ecosystem, editor tooling and documentation from day one.
- Svelte upgrades are mostly a matter of bumping a version.
- Server-side rendering requires executing JavaScript from Go (see [ADR 0002](0002-client-rendering-first.md)).
- Tern's client runtime must track Svelte 5 APIs such as runes, rather than SvelteKit internals.

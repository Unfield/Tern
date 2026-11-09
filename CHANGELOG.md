# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html). Before v1.0.0, minor versions may contain breaking changes.

## [Unreleased]

### Added

- Project foundation: license, contribution guidelines, architecture docs, CI.
- Route scanning with SvelteKit-style conventions: `+page.svelte`, `+page.server.go`, `+layout.svelte`, `+layout.server.go`, `+error.svelte` and `+server.go`, with `[param]`, `[[optional]]`, `[...rest]`, `[id=matcher]`, mixed segments such as `[slug].json`, and `(group)` directories. All problems are reported at once, including conflicting routes.
- URL matching by route priority, with correct handling of encoded characters and trailing slashes.
- `tern routes` command to list an app's routes.

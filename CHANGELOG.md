# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html). Before v1.0.0, minor versions may contain breaking changes.

## [Unreleased]

### Added

- Project foundation: license, contribution guidelines, architecture docs, CI.
- Route scanning: `+page.svelte`, `+layout.svelte` and `+error.svelte`, plus each route directory's Go package (`Load`, `Actions`, `LayoutLoad`, `GET`, `POST`, …). Dynamic directories use Go-safe names: `slug.param`, `id.param.matcher`, `lang.optional`, `path.rest` and `app.group`. All problems are reported at once, including conflicting routes, and SvelteKit-style names get a suggested rename.
- URL matching by route priority, with correct handling of encoded characters and trailing slashes.
- `tern routes` command to list an app's routes.

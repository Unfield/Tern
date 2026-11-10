# Contributing to Tern

Thanks for your interest in Tern. This guide explains how to get set up and how changes get merged.

## Ground rules

- Be kind. Everyone taking part is expected to follow the [Code of Conduct](CODE_OF_CONDUCT.md).
- **Open an issue before starting large changes.** New features, API changes and anything touching the architecture should be discussed first, so nobody's work is wasted. Small fixes (typos, obvious bugs, docs) can go straight to a pull request.
- Report security problems privately. See [SECURITY.md](SECURITY.md).

## Prerequisites

| Tool | Version |
|---|---|
| Go | 1.26 or newer |
| Node.js | 22 or newer |
| pnpm | 10 or newer |
| [golangci-lint](https://golangci-lint.run) | v2 |

## Getting started

```sh
git clone https://github.com/Unfield/Tern.git
cd Tern
pnpm install
make check   # format check, lint, vet, tests: the same as CI
```

Useful targets:

```sh
make build   # build the tern CLI into ./bin
make test    # Go tests with the race detector
make lint    # golangci-lint + TypeScript typecheck
make fmt     # format Go code
```

## Repository layout

```
cmd/tern/           CLI entry point
kit/                Public Go runtime package that apps import
internal/           Private Go packages (codegen, dev server, build)
packages/tern/      npm package: client runtime and Vite plugin
examples/           Example apps
docs/               Architecture, roadmap, decision records
```

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for how the pieces fit together.

## Making a change

1. Fork the repo and create a branch from `main`, e.g. `fix/router-trailing-slash`.
2. Make your change, with tests. Bug fixes should include a test that fails without the fix.
3. Run `make check`.
4. Add an entry under `Unreleased` in [CHANGELOG.md](CHANGELOG.md) if users will notice the change.
5. Open a pull request and fill in the template.

### Commit messages

We use [Conventional Commits](https://www.conventionalcommits.org):

```
feat(router): support optional [[param]] segments
fix(typegen): emit nullable fields for pointer types
docs: explain layout data inheritance
```

Common types: `feat`, `fix`, `docs`, `refactor`, `perf`, `test`, `build`, `ci`, `chore`. Mark breaking changes with `!` (e.g. `feat(kit)!: ...`) and explain them in the body.

### Code style

- **Go:** `gofmt`/`goimports` formatting, and `golangci-lint` must pass. Exported identifiers need doc comments. Prefer the standard library; new dependencies need a reason in the PR.
- **TypeScript:** strict mode, no `any` without a comment explaining why.
- Keep the public API small. Anything exported from `kit/` or `@ternkit/tern` is a promise we have to keep.

## Architectural decisions

Significant design decisions are recorded as ADRs in [docs/adr](docs/adr). If your change alters one, or makes a new decision of similar weight, add or update an ADR in the same pull request.

## Releases

Maintainers cut releases from `main` using semantic versioning. Changes to anything covered by the [stability promise](docs/stability.md) must be backward compatible, except in a major release.

To release:

1. Move the `Unreleased` entries in [CHANGELOG.md](CHANGELOG.md) under a new version heading and commit.
2. Tag and push: `git tag v0.4.0 && git push origin v0.4.0`.

The [release workflow](.github/workflows/release.yml) then tests, builds `tern` binaries for Linux, macOS and Windows with GoReleaser, and publishes a GitHub release. The Go module is available from the module proxy as soon as the tag exists.

Publishing `@ternkit/tern` to npm is part of the same workflow and is enabled by the repository variable `NPM_PUBLISH=true`. It uses npm [trusted publishing](https://docs.npmjs.com/trusted-publishers): npmjs.com trusts the `release.yml` workflow of `Unfield/Tern`, so no npm token is stored in the repository.

## License

By contributing, you agree that your contributions are licensed under the [MIT License](LICENSE).

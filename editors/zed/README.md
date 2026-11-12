# Tern for Zed

For apps built with [Tern](https://tern.land), with Svelte and Go:

- **Page ⇄ load:** go to definition (F12) on `data` or `PageProps` in a page or layout opens its Go load; on `Load` in the Go file, the component. On `route('/blog/[slug]')`, the route's page.
- **Routes:** project symbols (**Cmd+T** / **Ctrl+T**) list the app's routes, such as `/blog/[slug]`.
- **tern check:** saving a file runs code generation and svelte-check with Go components' types, and shows their errors.

The extension starts `go tool tern lsp` in the project, so it uses the project's own version of Tern, next to gopls and the [Svelte extension](https://zed.dev/extensions/svelte)'s language server.

## Installing

Until it's in Zed's extension registry, install it from this folder: in Zed, run **zed: install dev extension** and choose `editors/zed`. Zed builds it with Rust, which it needs on the machine (install it with [rustup](https://rustup.rs)).

## Settings

To run another command, such as a `tern` binary on the PATH:

```json
{
  "lsp": {
    "tern": { "binary": { "path": "tern", "arguments": ["lsp"] } }
  }
}
```

If Svelte files don't get Tern's features, add it to their language servers:

```json
{
  "languages": {
    "Svelte": { "language_servers": ["svelte-language-server", "tern", "..."] }
  }
}
```

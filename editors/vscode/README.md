# Tern for VS Code

For apps built with [Tern](https://tern.land), with Svelte and Go:

- **Page ⇄ load:** go to definition on `data` or `PageProps` in a page or layout opens its Go load; on `Load` in the Go file, the component. A code lens at the top of each file does the same, and so does **Alt+O**.
- **Routes:** the Explorer's **Routes** view lists the app's routes; `route('/blog/[slug]')` goes to that route's page. Routes are workspace symbols too (**Ctrl+T** / **Cmd+T**).
- **tern check:** saving a file runs code generation and svelte-check with Go components' types, and shows their errors in the editor.

It runs `go tool tern lsp` in each Tern project of the workspace, so it uses the project's own version of Tern. Use it with the Svelte and Go extensions, which it works beside.

## Settings

- `tern.server`: the command that starts the language server, `["go", "tool", "tern", "lsp"]` by default.

## Building

```sh
npm install
npm run package   # tern-0.1.0.vsix
code --install-extension tern-0.1.0.vsix
```

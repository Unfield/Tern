# Editors

`tern lsp` is a language server for Tern apps, beside gopls and the Svelte language server. It knows what they don't: which Go load a page's data comes from, the app's routes, and the types of Go components. Extensions for VS Code and Zed start it; any editor with LSP can.

- **Page ⇄ load:** go to definition on `data`, `form`, `PageProps` or `LayoutProps` in a page or layout opens its Go load; on `Load`, `LayoutLoad` or `Actions` in the Go file, the component. On the pattern in `route('/blog/[slug]')`, that route's page. Elsewhere it answers nothing, so the editor's other servers answer.
- **Routes:** the app's routes are workspace symbols, such as `/blog/[slug]`: **Cmd+T** or **Ctrl+T** finds them.
- **tern check:** when a file is saved, or changes outside the editor, it runs code generation and [tern check](live-components.md#types-tern-check), and shows their errors in the files they're in. A check takes a few seconds, so it runs on save rather than as you type; the Svelte and Go servers still check as you type.
- **Code lenses:** a link at the top of each page to its load, and above each load to its page, in editors that show them.

It runs as `go tool tern lsp`, in the project, so it's the project's own version of Tern.

## VS Code

The extension is in [editors/vscode](https://github.com/Unfield/Tern/tree/main/editors/vscode). Build and install it:

```sh
cd editors/vscode
npm install
npm run package
code --install-extension tern-0.1.0.vsix
```

Besides the language server, it adds a **Routes** view to the Explorer, and **Alt+O** to go between a page and its load. Use it with the Svelte and Go extensions. `tern.server` sets the command it starts.

## Zed

The extension is in [editors/zed](https://github.com/Unfield/Tern/tree/main/editors/zed). In Zed, run **zed: install dev extension** and choose that folder; Zed builds it, with Rust installed. Use it with the Svelte extension.

To run another command than `go tool tern lsp`:

```json
{
  "lsp": {
    "tern": { "binary": { "path": "tern", "arguments": ["lsp"] } }
  }
}
```

## Other editors

Start `go tool tern lsp` in the project, for Svelte, Go and TypeScript files, next to the other servers. In Neovim:

```lua
vim.lsp.config('tern', {
  cmd = { 'go', 'tool', 'tern', 'lsp' },
  filetypes = { 'svelte', 'go', 'typescript' },
  root_markers = { 'go.mod' },
})
vim.lsp.enable('tern')
```

In Helix, in `languages.toml`:

```toml
[language-server.tern]
command = "go"
args = ["tool", "tern", "lsp"]

[[language]]
name = "svelte"
language-servers = ["svelteserver", "tern"]
```

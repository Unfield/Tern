//! Tern for Zed: starts `go tool tern lsp` in the project, next to gopls
//! and the Svelte language server. It jumps between a page and its Go load
//! (go to definition on `data` or `Load`), lists the routes as project
//! symbols, and shows tern check's errors when a file is saved.

use zed_extension_api::{self as zed, settings::LspSettings, LanguageServerId, Result, Worktree};

struct Tern;

impl zed::Extension for Tern {
    fn new() -> Self {
        Tern
    }

    fn language_server_command(&mut self, id: &LanguageServerId, worktree: &Worktree) -> Result<zed::Command> {
        // A command of the user's own, from settings:
        // "lsp": { "tern": { "binary": { "path": "tern", "arguments": ["lsp"] } } }
        if let Ok(settings) = LspSettings::for_worktree(id.as_ref(), worktree) {
            if let Some(binary) = settings.binary {
                if let Some(path) = binary.path {
                    return Ok(zed::Command {
                        command: path,
                        args: binary.arguments.unwrap_or_else(|| vec!["lsp".into()]),
                        env: worktree.shell_env(),
                    });
                }
            }
        }
        // The project's own version of Tern, as its go.mod's tool directive says.
        let go = worktree
            .which("go")
            .ok_or_else(|| "Tern needs Go on the PATH, to run `go tool tern lsp`".to_string())?;
        Ok(zed::Command {
            command: go,
            args: vec!["tool".into(), "tern".into(), "lsp".into()],
            env: worktree.shell_env(),
        })
    }
}

zed::register_extension!(Tern);

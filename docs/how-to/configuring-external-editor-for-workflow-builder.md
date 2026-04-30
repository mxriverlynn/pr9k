# Configuring an External Editor for the Workflow Builder

← [Back to How-To Guides](README.md)

The workflow builder uses your system's external editor to edit multi-line content — prompt files and scripts — that cannot be edited inside the TUI. This guide explains how to configure the editor, what values are accepted and rejected, and how to verify the configuration.

**Prerequisites**: you've launched the workflow builder at least once — see [Using the Workflow Builder](using-the-workflow-builder.md). The short version: set `VISUAL=nano` (or `nvim`, or `code --wait`) in your shell rc, then press `Ctrl+E` on a step's prompt or script in the builder. The rest of this page is the full rule set behind that one-liner.

## How the Builder Resolves Your Editor

The builder checks environment variables in this order:

1. `$VISUAL`
2. `$EDITOR`

The value is treated as a **command with optional arguments**. A value like `code --wait` is split at whitespace into `["code", "--wait"]` and passed directly to `exec` — no shell is involved.

If neither variable is set, the builder shows a dialog with the path of the file and a copy-pasteable instruction for setting an editor. It does **not** fall back to `vi` or any other default, because a surprise editor trap is worse than an explicit error.

## Setting Your Editor

Add one of these lines to your shell configuration (`~/.bashrc`, `~/.zshrc`, or equivalent):

```bash
# nano (beginner-friendly)
export VISUAL=nano

# neovim
export VISUAL=nvim

# VS Code (requires the --wait flag so the builder waits for the window to close)
export VISUAL="code --wait"

# Helix
export VISUAL=hx

# Emacs (terminal mode)
export VISUAL="emacs -nw"
```

After editing your shell config, open a new terminal session (or run `source ~/.bashrc`) so the variable is available when you launch `pr9k workflow`.

## Accepted Values

The builder accepts an editor value when all of the following are true:

- The command name (the first whitespace-separated token) resolves to an executable on `PATH`, **or** is given as an absolute path
- The value does **not** contain shell metacharacters (`|`, `&`, `;`, `$`, `` ` ``, `(`, `)`, `{`, `}`, `<`, `>`, `\`)
- If the command is a relative path (e.g., `./editor`), it must exist and be executable

## Rejection Set

The builder rejects these editor values and shows an error dialog explaining the specific problem:

| Problem | Example | Why Rejected |
|---------|---------|--------------|
| Shell metacharacters | `vim && echo done` | Builder uses direct exec, not a shell; the metacharacter would be passed as a literal argument |
| Not found on `PATH` | `subl` (Sublime Text, not on `PATH`) | Binary not executable from the current environment |
| Relative path not found | `./my-editor` | The resolved path must exist and be executable |
| Permission denied | `/usr/local/bin/editor` (not executable) | OS refuses to exec the binary |

## Verifying Your Configuration

After setting `$VISUAL` or `$EDITOR`, open the workflow builder and navigate to a step's prompt-file field in the detail pane. Press `Ctrl+E` to invoke the editor.

If the configuration is correct:
1. The session header shows `Opening editor…`
2. The terminal yields to the editor
3. After you save and close the editor, the builder reclaims the terminal and re-reads the file

If the configuration has a problem, the builder shows an error dialog naming the value and the specific issue. Fix the value in your shell environment and restart the builder (or, in some shells, you can re-export the variable in the same shell session without restarting).

## tmux and SSH: The `Alt` Modifier Caveat

In many tmux configurations and some SSH connections, `Alt+key` combinations are not forwarded correctly. This affects the step reorder shortcut (`Alt+↑` / `Alt+↓`), not the external editor shortcut (`Ctrl+E`). If you cannot use `Alt+↑`/`Alt+↓` to reorder steps, press `r` on a focused step to enter reorder mode, then use `↑` / `↓` to move and `Enter` to commit.

## GUI Editors and Daemonization

Some GUI editors (such as VS Code without `--wait`, or `atom`) daemonize — they start a background process and the foreground process returns immediately. When this happens, the builder reclaims the terminal before the editor window opens, which means the builder re-reads the file before you have a chance to edit it.

**Always use `--wait` (or the equivalent flag) with GUI editors:**

```bash
export VISUAL="code --wait"
export VISUAL="subl --wait"
export VISUAL="atom --wait"
```

## SIGINT Behavior

If an external editor is open and you send SIGINT (`Ctrl+C`) from another terminal, the foreground process group receives the signal. Most terminal editors handle SIGINT and exit cleanly. If the editor ignores SIGINT, you must terminate it from another terminal session.

The builder never silently loses the terminal in a released-but-not-reclaimed state: it completes the editor-spawn step (so terminal restore runs) before entering the normal quit flow.

## Related Documentation

- ← [Back to How-To Guides](README.md)
- [Using the Workflow Builder](using-the-workflow-builder.md) — launching the builder, editing steps, saving
- [Building Custom Workflows](building-custom-workflows.md) — what the prompt and script files you'll edit are for
- [Workflow Builder Feature Reference](../features/workflow-builder.md) — full feature behavior including the external-editor flow and the model field suggestion list

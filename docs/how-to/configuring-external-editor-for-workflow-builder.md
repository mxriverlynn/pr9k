# Configuring an External Editor for the Workflow Builder

The workflow builder can open companion files (prompt files and scripts) in your preferred text editor via `Ctrl+O`. This guide covers how to configure `$VISUAL` and `$EDITOR`, quoting rules for editors with arguments, shell-metacharacter rejection, SIGINT behavior, and editor-specific examples.

## How the Editor Is Resolved

When you press `Ctrl+O`, the builder resolves the editor binary from the following sources in order:

1. `$VISUAL` environment variable
2. `$EDITOR` environment variable
3. A platform default (`vi` on Unix, `notepad` on Windows)

The value is word-split using POSIX `shlex` rules (same as how a shell splits unquoted words). This means you can pass flags:

```sh
export VISUAL="code --wait"
```

Word-splitting produces `["code", "--wait"]`. The first token is looked up via `exec.LookPath`; the remaining tokens are passed as arguments after the filename.

## Shell-Metacharacter Rejection

For security, the builder rejects editor values that contain shell metacharacters: `|`, `&`, `;`, `<`, `>`, `` ` ``, `$`, `(`, `)`. These characters are never valid in an editor binary name or simple flag.

If `$VISUAL` or `$EDITOR` contains a rejected character, the builder falls through to the next source. If all sources fail, a restore-failed dialog opens.

Examples of **rejected** values:

```sh
export VISUAL="$(which code) --wait"   # $ rejected
export EDITOR="emacs &"               # & rejected
export VISUAL="cat | less"            # | rejected
```

Examples of **accepted** values:

```sh
export VISUAL="code --wait"
export EDITOR="nvim"
export VISUAL="/usr/local/bin/subl --wait"
```

## Quoting Rules

Use `$VISUAL` or `$EDITOR` with standard shell quoting when your editor path or flags contain spaces:

```sh
# Editor binary path contains a space
export VISUAL="'/Applications/Sublime Text.app/Contents/SharedSupport/bin/subl' --wait"
```

The value is parsed by `shlex` before being passed to the OS — the surrounding shell quotes (single or double) are stripped by the shell before the variable is set, so the value stored in the environment is:

```
'/Applications/Sublime Text.app/Contents/SharedSupport/bin/subl' --wait
```

`shlex` then processes the single-quoted portion and produces:

```
["/Applications/Sublime Text.app/Contents/SharedSupport/bin/subl", "--wait"]
```

## SIGINT Behavior

If you press `Ctrl+C` in the editor (causing it to exit with code 130), the builder treats this as a cancel and does not reload the file. The builder continues normally with the previous content.

Any other non-zero exit code is treated as a failure and opens a restore-failed dialog, allowing you to retry or continue without reloading.

## VS Code

VS Code must be launched with `--wait` so the builder knows when editing is complete:

```sh
export VISUAL="code --wait"
```

Verify `code` is on your `$PATH`:

```sh
which code
```

On macOS, if `code` is not on `$PATH`, install it from within VS Code: open the Command Palette (`Cmd+Shift+P`), search for "Shell Command: Install 'code' command in PATH", and run it.

## Sublime Text

Sublime Text requires `--wait` to block until the file is closed:

```sh
export VISUAL="subl --wait"
```

On macOS with the default installation path:

```sh
export VISUAL="'/Applications/Sublime Text.app/Contents/SharedSupport/bin/subl' --wait"
```

Note: `subl` extracts only the binary name from the path — a path like `/opt/Sublime Text/subl --wait` logs `subl` in the session log, not the full path.

## Neovim / Vim

No special flags needed; these editors block until the user quits:

```sh
export VISUAL="nvim"
# or
export VISUAL="vim"
```

## Emacs

Use `emacsclient` to avoid opening a new Emacs process each time. Ensure an Emacs server is running:

```sh
emacs --daemon          # start the server once
export VISUAL="emacsclient --tty"
```

## tmux Users

If you run the workflow builder inside tmux, the editor receives a child PTY. Standard editor behavior applies; no special configuration is needed for `$VISUAL`/`$EDITOR`.

If your editor opens in a background pane, ensure it waits for the user before exiting — otherwise the builder will reload the file before editing is complete.

## Verifying Your Configuration

From within the workflow builder:

1. Focus the detail pane on a step that has a companion file (a claude step with a prompt file, or a shell step with a script command).
2. Press `Ctrl+O`.
3. If the editor opens: configuration is correct.
4. If you see a "restore failed" dialog: check `$VISUAL` and `$EDITOR` for rejected metacharacters or an unresolvable binary name.

## Debugging

Session events are logged to `.pr9k/logs/workflow-*.log` in the project directory. Look for:

```
editor_opened binary=<first-token>
editor_sigint
```

If `editor_opened` does not appear after pressing `Ctrl+O`, the editor resolution failed before launch.

## Related Documentation

- [`docs/features/workflow-builder.md`](../features/workflow-builder.md) — Full workflow builder feature reference
- [`docs/how-to/using-the-workflow-builder.md`](using-the-workflow-builder.md) — Step-by-step usage guide

# Using the Workflow Builder

← [Back to How-To Guides](README.md)

A step-by-step guide to launching the pr9k workflow builder, creating or opening a workflow, editing steps, and saving your changes.

The builder is an interactive TUI for editing `config.json` and its companion files (prompt files, scripts, ralph-art). It validates as you type and writes safely (atomic temp-file + rename). If you'd rather edit `config.json` directly in your editor, that works too — see [Building Custom Workflows](building-custom-workflows.md).

## Prerequisites

- A working install of pr9k — see [Getting Started](getting-started.md). You can launch the builder with the absolute path (`/path/to/pr9k/bin/pr9k workflow`) or, if you've put the binary on `$PATH`, just `pr9k workflow`.
- Familiarity with the workflow concepts — see [Building Custom Workflows](building-custom-workflows.md)
- A terminal that supports alt-screen and cell-motion mouse (the same capabilities the main `pr9k` command requires)
- Write permission to the directory where you want to save the workflow

## Launching the Builder

```bash
pr9k workflow
```

Optional flags:

| Flag | Default | Effect |
|------|---------|--------|
| `--workflow-dir <path>` | `<projectDir>/.pr9k/workflow/`, then `<executableDir>/.pr9k/workflow/` | Open this workflow file at launch (auto-open) |
| `--project-dir <path>` | Current working directory | Sets the base for `<projectDir>` and the log file location |

When `--workflow-dir` is supplied, the builder auto-opens that file instead of showing the empty-editor hint state.

## Creating a New Workflow

1. From the empty-editor state, press `Ctrl+N` (or activate **File > New** via `F10`).
2. Choose between:
   - **Copy from default workflow** — loads the bundled default as a starting point (the builder runs a pre-copy integrity check first)
   - **Start with empty workflow** — creates a minimal scaffold with one placeholder iteration step
3. The path picker appears, pre-filled with `<projectDir>/.pr9k/workflow/`. Edit the path if needed, then press `Enter` to confirm.
4. The workflow loads into the edit view. Nothing is written to disk until you save.

## Opening an Existing Workflow

1. Press `Ctrl+O` (or **File > Open** via `F10`).
2. The path picker appears, pre-filled with `<projectDir>/.pr9k/workflow/config.json`. Navigate to your `config.json` and press `Enter`.
3. The file loads. Any load-time warnings appear in the session header banner.

## Navigating the Outline

The left pane lists all workflow sections:

- **env** — top-level environment passthrough list (shown when non-empty)
- **containerEnv** — container environment variables (shown when non-empty)
- **statusLine** — status-line block (shown when present)
- **initialize** / **iteration** / **finalize** — the three ordered phases, always shown

Use `↑` / `↓` to move the cursor. Press `Space` to collapse or expand a section.

At the bottom of each section is a `+ Add step` (or `+ Add item`) row. Press `Enter` on that row to create a new empty item.

**Keyboard shortcuts in the outline:**

| Key | Action |
|-----|--------|
| `↑` / `↓` | Move cursor |
| `Space` | Toggle section collapse/expand |
| `a` | Add item (when on a section header) |
| `Enter` | Add item (when on a `+ Add` row) |
| `Tab` | Move focus to the detail pane (on a step row) |
| `Del` | Remove selected step (confirmation required) |
| `Alt+↑` / `Alt+↓` | Move step up/down within its phase |
| `r` | Enter reorder mode (for environments where `Alt` is intercepted) |

## Editing Step Fields

When focus is in the detail pane, `Tab` and `Shift+Tab` move between fields.

**Field kinds:**

| Field | How to edit |
|-------|-------------|
| Plain text (Name, CaptureAs, etc.) | Type directly; newlines and ANSI escapes are stripped |
| Choice list (Kind, CaptureMode, etc.) | `Enter` or `Space` to open; `↑`/`↓` to navigate; `Enter` to confirm; `Esc` to dismiss |
| Numeric (Timeout, RefreshInterval) | Type digits; non-numeric input is silently ignored |
| Model suggest | Type freely; a suggestion list appears and can be navigated with `↑`/`↓` |
| Secret value (containerEnv secrets) | Shown as `••••••••`; press `r` to reveal while focused; re-masks on focus leave |

## Opening a Companion File in External Editor

To edit a prompt file or script, focus the corresponding path field in the detail pane and press `Ctrl+E`. The builder:

1. Validates your `$VISUAL` or `$EDITOR` setting
2. Displays `Opening editor…`
3. Yields the terminal to the editor
4. Reclaims the terminal when the editor exits and re-reads the file from disk

If no editor is configured, a dialog explains how to set one. See [Configuring an External Editor](configuring-external-editor-for-workflow-builder.md) for setup instructions.

## Saving

Press `Ctrl+S` (or **File > Save** via `F10`).

While the validator is running, the footer briefly shows `Validating…`; while the file is being written it shows `Saving…`. These transient messages replace the normal shortcuts until the operation completes.

- If there are **fatal validation findings**, the findings panel opens. Jump to each field using the affordance in the panel, fix the issue, and press `Ctrl+S` again.
- If there are **warnings or info findings only**, a one-time acknowledgment dialog appears. After acknowledging, warnings are suppressed for the rest of the session.
- If there are **no findings**, the file writes atomically and a `Saved at HH:MM:SS` banner appears in the session header row for ~3 seconds.
- If there are **no changes**, the save is a no-op and the feedback shows `No changes to save`.

**Note on `Ctrl+S` in some terminals:** terminals with XON/XOFF flow control enabled intercept `Ctrl+S`. Run `stty -ixon` in your shell to disable it, or use **File > Save** from the menu as a fallback.

## Understanding the Session Header

The session header (third row) assembles up to five pieces of information left-to-right:

1. **Target path** (or `(unsaved)` before the first save)
2. **Dirty indicator `●`** (green) — shown when there are unsaved edits and the file is writable
3. **Warning banner** — one of the tags below, colored by severity
4. **`[N more warnings]`** — when more than one banner is active; opens an all-banners panel
5. **Validation indicator · findings summary** — right-aligned; shows after any `Ctrl+S` attempt

**Banner priority and colors** (highest first):

| Priority | Banner | Color | Condition |
|----------|--------|-------|-----------|
| 1 | `[ro]` | Red | Target file is not writable |
| 2 | `[ext]` | Yellow | Target is outside project dir and home dir |
| 3 | `[sym]` or `[sym → target]` | Yellow | config.json or a companion is a symlink |
| 4 | `[shared]` | Yellow | Target is the bundled default on a shared install |
| 5 | `[?fields]` | Cyan | Config contains fields the builder does not recognize |

**Read-only mode:** when `[ro]` is active, File > Save is greyed out in the menu and the footer appends a dim `save  [ro]` hint. Navigate to a writable target using File > New or File > Open.

## Quitting

Press `Ctrl+Q` (or **File > Quit** via `F10`).

- **With unsaved changes:** a dialog offers Save / Cancel / Discard. Cancel is the keyboard default (pressing `Enter` cancels the quit).
- **Without unsaved changes:** a two-option dialog asks `Quit the workflow builder? (Yes / No)`. No is the keyboard default; press `y` to exit.

Escape is always equivalent to Cancel or No.

## Getting Help

Press `?` at any time to open the help modal, which lists every keyboard shortcut for the current focus state. Press `?` or `Escape` to close it. The `?  help` hint in the footer is always available unless a dialog (other than the findings panel) is open.

## Related Documentation

- ← [Back to How-To Guides](README.md)
- [Configuring an External Editor](configuring-external-editor-for-workflow-builder.md) — set `$VISUAL`/`$EDITOR` for the builder's `Ctrl+E` companion-file editor
- [Building Custom Workflows](building-custom-workflows.md) — workflow bundle structure (prompts, scripts, config.json layout)
- [Workflow Variables](workflow-variables.md) — the `{{VAR}}` substitution language used inside prompts and commands the builder edits
- [Debugging a Run](debugging-a-run.md) — verify your saved workflow actually does what you expect on the next run
- [Workflow Builder Feature Reference](../features/workflow-builder.md) — full feature behavior, TUI layout, all keyboard shortcuts (contributor reference)

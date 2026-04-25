# Workflow Builder

An interactive terminal interface for authoring and editing pr9k workflow bundles — configuration, prompt files, and scripts — without hand-editing JSON.

- **Last Updated:** 2026-04-25
- **Authors:**
  - River Bailey

## Overview

The `pr9k workflow` subcommand opens the workflow builder TUI. A workflow author uses it to:

- Create a new workflow from scratch or by copying the bundled default
- Open an existing `config.json` for editing
- Navigate the workflow structure via an outline panel and edit fields in a detail pane
- Validate, save, and quit — all from keyboard or mouse

The builder runs the same validator as the main `pr9k` command at save time, so the workflow on disk is always either valid or clearly marked with findings that block the save.

## TUI Layout

```
┌─ File ──────────────────────────────────────────────────────────────┐
│  File                                                               │
├─────────────────────────────────────────────────────────────────────┤
│  /path/to/workflow  [unsaved]  [symlink]                            │
├────────────────┬────────────────────────────────────────────────────┤
│ ▾ initialize   │ Name          my-step                              │
│   step-a       │ Kind          claude ▾                             │
│   + Add step   │ Model         claude-sonnet-4-6 ▾                  │
│ ▾ iteration    │ Prompt File   prompts/my-step.md [Ctrl+E edit]     │
│   step-b       │ CaptureAs     RESULT                               │
│   + Add step   │ Timeout       300                                  │
│ ▾ finalize     │                                                    │
│   + Add step   │                                                    │
├────────────────┴────────────────────────────────────────────────────┤
│  Tab next  ↑↓ outline  Ctrl+S save  Ctrl+Q quit  ? help            │
└─────────────────────────────────────────────────────────────────────┘
```

Five persistent surfaces:

| Surface | Description |
|---------|-------------|
| **Menu bar** (row 1) | `File` menu with `New`, `Open`, `Save`, `Quit`; activated by `F10`, `Alt+F`, or mouse click |
| **Session header** (row 3) | Target path, unsaved-changes indicator, priority banner (read-only / external-workflow / symlink / shared-install / unknown-field), `[N more warnings]` when multiple banners are active |
| **Workflow outline** (left pane) | Collapsible sections for env, containerEnv, statusLine, and three phases (initialize, iteration, finalize); each phase section ends with a `+ Add step` row |
| **Detail pane** (right pane) | Fields for the currently selected outline item; choice lists, plain text, numeric, model-suggest, and secret-masked field kinds; independently scrollable |
| **Shortcut footer** (bottom row) | Context-sensitive keyboard hints; updates when focus changes |

## Keyboard Map

### Global shortcuts (always active)

| Key | Action |
|-----|--------|
| `Ctrl+N` | File > New |
| `Ctrl+O` | File > Open |
| `Ctrl+S` | File > Save |
| `Ctrl+Q` | File > Quit |
| `F10` | Open File menu |
| `Alt+F` | Open File menu |
| `?` | Toggle help modal |

### Outline navigation

| Key | Action |
|-----|--------|
| `↑` / `↓` | Move cursor up/down in outline |
| `Space` | Toggle collapse/expand for current section |
| `a` | Add item (when on a section header) |
| `Enter` | Add item (when on a `+ Add` row) |
| `Tab` | Switch focus to detail pane (when on a step row) |
| `Del` | Remove selected step (with confirmation) |
| `r` | Enter reorder mode (moves step up/down without Alt) |
| `Alt+↑` / `Alt+↓` | Move step up/down within its phase |

### Detail pane

| Key | Action |
|-----|--------|
| `Tab` | Move to next field |
| `Shift+Tab` | Move to previous field |
| `Enter` / `Space` | Open choice list (on choice fields) |
| `↑` / `↓` | Navigate choice list |
| `Escape` | Close choice list, restore prior value |
| `r` | Toggle secret mask reveal (on sensitive containerEnv fields) |
| `Ctrl+E` | Open companion file in external editor (on prompt/script path fields) |

### Help modal

| Key | Action |
|-----|--------|
| `?` | Toggle help modal open/closed |
| `Escape` | Close help modal |

The help modal is unconditionally reachable from the edit view or the findings panel, regardless of any other configuration.

### Menu navigation (while File menu is open)

| Key | Action |
|-----|--------|
| `↑` / `↓` | Navigate menu items |
| `Enter` | Select highlighted item |
| `Escape` | Close menu |

## File Menu Flows

### File > New

1. If a workflow is loaded with unsaved edits, a three-option dialog intercepts: **Save** / **Cancel** (keyboard default) / **Discard**.
2. A choice dialog offers: **Copy from default workflow** / **Start with empty workflow** / **Cancel** (keyboard default).
3. For the Copy option, the builder performs a pre-copy integrity check; a broken default triggers a **Copy anyway** / **Cancel** dialog.
4. A path picker (pre-filled with `<projectDir>/.pr9k/workflow/`) asks where to place the new workflow.
5. The new in-memory workflow loads into the edit view. Nothing is written to disk until File > Save.

### File > Open

1. If a workflow is loaded with unsaved edits, the three-option dialog intercepts (same as File > New).
2. A path picker (pre-filled with `<projectDir>/.pr9k/workflow/config.json`) asks which `config.json` to open.
3. All load-time behaviors apply: read-only detection, symlink banner, external-workflow banner, unknown-field warning, crash-era temp-file notice, parse-error recovery view.

### File > Save

1. The full workflow validator runs against the in-memory state.
2. **Fatal findings** block the save; the findings panel opens listing each finding with a jump-to-field affordance.
3. **Warnings/info only** — the save proceeds after the user acknowledges the findings panel. Acknowledged warnings are suppressed for the remainder of the session.
4. **No findings, no changes** — no-op save; the file is not rewritten; feedback shows `No changes to save`.
5. **No findings, changes present** — atomic write (temp file + rename); the unsaved-changes indicator clears; a transient `Saved at HH:MM:SS` banner appears in the session header for ~3 seconds.

On external-workflow targets, the first save in the session additionally prompts for confirmation before writing.

### File > Quit

Two dialog shapes:

- **Unsaved changes present:** three-option dialog — **Save** / **Cancel** (keyboard default) / **Discard**. If Save surfaces fatal findings, quit is cancelled and the findings panel opens.
- **No unsaved changes:** two-option dialog — `Quit the workflow builder? (Yes / No)`. `No` is the keyboard default; `y` exits.

Escape is always equivalent to Cancel/No.

## Session Header Banners

The session header shows at most one warning banner at a time, selected by priority:

| Priority | Banner | Condition |
|----------|--------|-----------|
| 1 (highest) | `read-only` | Target file is not writable |
| 2 | `external workflow` | Target path is outside project dir and home dir |
| 3 | `symlink` | config.json or a companion file is a symlink |
| 4 | `shared install` | Target is the bundled default on a shared install |
| 5 (lowest) | `unknown fields` | Config contains fields the builder does not recognize |

When multiple banners are active, a `[N more warnings]` affordance appears; selecting it opens a panel listing all active banners.

## Field Types

| Field Kind | Description | Keyboard |
|------------|-------------|----------|
| Plain text | Single-line input; newlines and ANSI escapes stripped on input | Any printable character |
| Choice list | Fixed set of values; shown with trailing `▾`; invalid values never offered | `Enter`/`Space` to open; `↑`/`↓` to navigate; `Enter` confirm; `Esc` dismiss; typing jumps to first matching option |
| Numeric | Integer with visible bounds; non-numeric input silently ignored; pasted input stripped at first non-digit | Digits only |
| Model suggest | Free-text with hardcoded suggestion list; values outside the list accepted | Any printable character; suggestion list navigable with `↑`/`↓` |
| Secret mask | containerEnv value whose key matches a secret pattern (`_TOKEN`, `_SECRET`, `_KEY`, `_PASSWORD`, `_PASSPHRASE`, `_CREDENTIAL`, `_APIKEY`); rendered as `••••••••`; `[press r to reveal]` label | `r` toggles mask; re-masks on focus leave |

## External Editor Integration

When the user presses `Ctrl+E` on a prompt-file or script-path field, the builder:

1. Resolves the configured external editor (`$VISUAL` first, then `$EDITOR`).
2. Validates the editor value (rejects shell metacharacters; rejects relative paths not on `PATH`).
3. Displays `Opening editor…` in the session header.
4. Yields the terminal to the editor process.
5. Waits for the editor to exit, then reclaims the terminal.
6. Re-reads the file from disk.

If `$VISUAL` and `$EDITOR` are both unset, or the configured value fails validation, a dialog explains the problem and offers retry after the user fixes the configuration.

See [`docs/how-to/configuring-external-editor-for-workflow-builder.md`](../how-to/configuring-external-editor-for-workflow-builder.md) for setup instructions.

## Validator Integration

The builder uses the same `internal/workflowvalidate` bridge that the main `pr9k` command uses, passing the current in-memory `WorkflowDoc` rather than a file path. Validation runs:

- At every **File > Save** attempt
- Automatically when the findings panel is open and a field value changes (re-runs on each save attempt with the panel open)

See [`docs/code-packages/workflowvalidate.md`](../code-packages/workflowvalidate.md) for the validator API and [`docs/code-packages/workflowedit.md`](../code-packages/workflowedit.md) for the Bubble Tea model internals.

## Parse-Error Recovery

If a `config.json` cannot be parsed, the builder enters a **recovery view** instead of the edit view:

- Shows the file's raw bytes plus the parse error with its location
- Offers: open in external editor, reload (re-parse), discard (return to empty-editor state), cancel
- After a successful open-in-editor invocation, the builder auto-reloads; success transitions directly to the edit view

## Empty-Editor State

When the builder is running but no workflow is loaded:

- Outline shows `No workflow open`
- Detail pane shows a centered hint: `` `File > New` (Ctrl+N) — create a workflow `` and `` `File > Open` (Ctrl+O) — open an existing config.json ``
- File > Save is greyed out
- Footer shows: `F10 menu  Ctrl+N new  Ctrl+O open`

## Session Lifecycle

A session begins when a workflow loads (File > New completes, File > Open completes, or `--workflow-dir` auto-opens a file) and ends when:

- The builder process exits (File > Quit confirmed)
- The user invokes File > New or File > Open (ends current session, starts a new one after unsaved-changes interception)

Session-scoped state that is released at session end: acknowledged warnings, first-save confirmation, unsaved-changes indicator, outline scroll position, collapse state, file-change mtime snapshot.

## Save Durability

Every configuration file write uses `internal/atomicwrite` (temp file + atomic rename). A crash or interruption during save never leaves the file torn or partially written — the file on disk contains either the prior content or the new content.

Companion files (prompt files, scripts) are written before `config.json` in every save; if the save fails partway through, `config.json` is not updated. See [`docs/adr/20260424120000-workflow-builder-save-durability.md`](../adr/20260424120000-workflow-builder-save-durability.md) for the decision rationale.

## Session Event Logging

The builder logs session-level events to `.pr9k/logs/` using the same logger as the main `pr9k` command. Logged events include:

- `symlink_detected` — on load when the target is a symlink
- `shared_install_detected` — on load when editing the bundled default on a shared install
- `external_workflow_detected` — on load when the target is outside trusted paths
- `read_only_detected` — on load when the target is not writable
- Editor invoked / exited — with the binary name and exit code

`containerEnv` secret values are never written to the log.

## Related Documentation

- [`docs/how-to/using-the-workflow-builder.md`](../how-to/using-the-workflow-builder.md) — step-by-step guide for new users
- [`docs/how-to/configuring-external-editor-for-workflow-builder.md`](../how-to/configuring-external-editor-for-workflow-builder.md) — `$VISUAL`/`$EDITOR` setup
- [`docs/features/cli-configuration.md`](cli-configuration.md) — `pr9k workflow` subcommand flags
- [`docs/code-packages/workflowedit.md`](../code-packages/workflowedit.md) — Bubble Tea model internals
- [`docs/code-packages/workflowio.md`](../code-packages/workflowio.md) — load/save/detect package
- [`docs/code-packages/workflowmodel.md`](../code-packages/workflowmodel.md) — in-memory WorkflowDoc types
- [`docs/adr/20260424120000-workflow-builder-save-durability.md`](../adr/20260424120000-workflow-builder-save-durability.md) — save durability ADR

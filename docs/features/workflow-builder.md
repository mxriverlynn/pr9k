# Workflow Builder

The workflow builder is an interactive TUI editor for `config.json` workflow bundles. It lets users create, inspect, and modify workflow steps without hand-editing JSON, and applies the D13 validator before every save so broken configurations are caught before they reach a `pr9k` run.

- **Last Updated:** 2026-04-24
- **Authors:**
  - River Bailey

## Overview

- Launch with `pr9k workflow [--workflow-dir <path>] [--project-dir <path>]` (the command is hidden from `pr9k --help` until the TUI is fully wired; it is functional)
- Opens a two-panel TUI: a left outline listing all steps, and a right detail pane for editing each step's fields
- Changes are kept in memory until the user explicitly saves with `Ctrl+S`; unsaved edits are tracked with a dirty flag and a `*` indicator
- Before writing to disk, the builder runs the D13 validator; fatal errors open the findings panel and block the save, while warnings allow the save to proceed after acknowledgment
- External editor handoff (`Ctrl+O`) opens the focused companion file (prompt file or script) in `$VISUAL` or `$EDITOR` and reloads it on return
- Session events (save, open, quit, editor sigint) are logged to `.pr9k/logs/workflow-<timestamp>.log` inside `projectDir`

Key implementation files:

- `src/internal/workflowedit/` — Bubble Tea model, outline, detail pane, dialogs, findings panel, path picker
- `src/internal/workflowmodel/` — in-memory `WorkflowDoc` and `Step` types; `IsDirty`, `Empty`, `CopyFromDefault`
- `src/internal/workflowio/` — `Load`, `Save`, `DetectSymlink`, `DetectReadOnly`, `DetectCrashTempFiles`
- `src/internal/workflowvalidate/` — thin bridge to `internal/validator.ValidateDoc`
- `src/cmd/pr9k/workflow.go` — `newWorkflowCmd`, `runWorkflowBuilder`, signal handling, log-dir resolution
- `src/cmd/pr9k/editor.go` — `realEditorRunner`, `resolveEditor`, `makeExecCallback`

See [`docs/code-packages/workflowedit.md`](../code-packages/workflowedit.md) for the package-level API reference.

## Layout

```
┌──────────────────────────────────────────────────────────────┐
│  File                                              [menu bar] │
├──────────────────────┬───────────────────────────────────────┤
│  ▸ initialize        │  Name:         get-next-issue         │
│    get-next-issue    │  Kind:         shell                  │
│    feature-work      │  Command:      scripts/get_next_issue │
│  ▸ iteration         │  Capture As:   ISSUE_ID               │
│    test-planning     │  Capture Mode: lastLine               │
│  ▸ finalize          │                                       │
│    code-review       │  [detail pane — editable fields]      │
│                      │                                       │
├──────────────────────┴───────────────────────────────────────┤
│  Ctrl+S save  Ctrl+Q quit  Ctrl+O open in editor  F10 menu   │
└──────────────────────────────────────────────────────────────┘
```

The outline (left, 40% of width, min 20 cols, max 40 cols) shows section headers for each phase (initialize, iteration, finalize) and the step names within each section. The detail pane (right, remaining width) shows editable fields for the currently focused step.

## Navigation

| Key | Action |
|-----|--------|
| `↑` / `↓` or `j` / `k` | Move cursor in outline |
| `Tab` | Cycle focus: outline → detail → menu → outline |
| `Shift+Tab` | Cycle focus in reverse |
| `Enter` | Confirm field edit in detail pane |
| `Esc` | Cancel current operation or close dialog |
| `F10` | Open File menu |

## File Menu

Press `F10` or navigate to the menu bar and press `Enter` to open the File menu. Available actions:

| Menu Item | Shortcut | Action |
|-----------|----------|--------|
| New… | — | Create a new workflow (opens `DialogNewChoice`) |
| Open… | `Ctrl+O` (from outline) | Load a workflow bundle from disk |
| Save | `Ctrl+S` | Validate and save to disk |
| Quit | `Ctrl+Q` | Quit (prompts if unsaved changes) |

### File→New Dialog (`DialogNewChoice`)

After choosing **New…** from the File menu, a dialog opens with two options:

| Key | Action |
|-----|--------|
| `e` | Create an empty document (`workflowmodel.Empty()` — one placeholder shell step) |
| `c` | Copy the default bundle (`workflowmodel.CopyFromDefault`) into memory for editing |
| `Esc` | Cancel, returning to the current document |

Both options set the dirty flag immediately so the first `Ctrl+S` will prompt for a save path.

## Save Flow

Pressing `Ctrl+S` triggers a three-stage async flow:

1. **Validate** — runs `workflowvalidate.Validate` in a goroutine with a deep copy of the current document
2. **Findings** — if validation found fatals, the findings panel opens and the save is blocked; if warnings only, an acknowledgment dialog opens; if clean, the save proceeds immediately
3. **Save** — calls `workflowio.Save` to atomically write companion files (prompt files, scripts) first, then `config.json`. The outcome is displayed as a save banner or an error dialog.

### Findings Panel

When the validator finds fatal errors, the findings panel opens and shows each error with its step name, field, and message. Navigation:

| Key | Action |
|-----|--------|
| `↑` / `↓` | Scroll findings |
| `Enter` | Jump to the first referenced step in the outline |
| `Esc` | Close findings panel (save remains blocked until errors are fixed) |

### Path-Traversal Protection

`workflowio.Load` and `workflowio.Save` validate every companion path with `pathContainedIn(workflowDir, path)`. If a `promptFile` value such as `../../etc/passwd` would resolve outside `workflowDir` after symlink evaluation, `Load` returns `ErrPathEscape` and `Save` returns `SaveErrorSymlinkEscape`. The TUI surfaces this as a `DialogError`.

### Conflict Detection

If `config.json` has been modified on disk since the last load or save (detected by nanosecond-precision mtime comparison), the save is blocked and a conflict dialog opens. Options: reload from disk (discarding unsaved changes) or force-save (overwriting the disk version).

## External Editor Handoff (`Ctrl+O`)

When the detail pane has focus and the current field is a file path (prompt file or command script), pressing `Ctrl+O` opens a path-picker dialog first (if the field is empty), then launches the configured external editor with that file. The editor binary is resolved from `$VISUAL` first, then `$EDITOR`, then a platform default.

- The editor command is word-split via shlex. Shell metacharacters (`|`, `&`, `;`, `<`, `>`, `` ` ``, `$`, `(`, `)`) are rejected to prevent injection.
- If the editor exits with code 130 (SIGINT), the builder treats it as a cancel (no reload).
- If the editor exits with any other non-zero code, a restore-failed dialog opens.

See [`docs/how-to/configuring-external-editor-for-workflow-builder.md`](../how-to/configuring-external-editor-for-workflow-builder.md) for setup examples.

## Path Picker

Pressing `Ctrl+O` when the focused field is empty opens the path picker dialog. It supports async tab completion:

| Key | Action |
|-----|--------|
| `Tab` | Cycle forward through completions |
| `Shift+Tab` | Cycle backward through completions |
| Any character | Append to current path; clear match cache |
| `Backspace` | Remove last character; clear match cache |
| `Enter` | Confirm selected path and close dialog |
| `Esc` | Cancel and close dialog |

Completions exclude hidden files unless the current prefix starts with `.`. Directories are shown with a trailing `/`. Completions are sorted alphabetically. Tilde (`~`) is expanded to the user's home directory.

## Session Logging

The builder logs session events to `.pr9k/logs/workflow-<timestamp>.log`. Log lines use the format expected by the pr9k log viewer. Events logged:

| Event | Log line |
|-------|----------|
| Builder started | `session_start workflow_dir=<path>` |
| Saved successfully | `workflow_saved path=<path>` |
| Save failed | `save_failed reason=<short>` |
| External editor opened | `editor_opened binary=<first-token>` |
| Editor exited via SIGINT | `editor_sigint` |
| Quit cleanly | `quit_clean` |
| Quit with discarded changes | `quit_discarded_changes` |
| Quit cancelled | `quit_cancelled` |
| Shared-install warning shown | `shared_install_detected` |

No `containerEnv`, `env`, or `prompt-file` content appears in any log line (F-113 field-exclusion contract).

## Shared-Install Warning

If the workflow bundle is owned by a different user (detected via `syscall.Stat_t.Uid` on Unix), a warning banner is shown at the top of the TUI before any content. This indicates a shared-install scenario where writes to `workflowDir` may fail.

## Crash Temp File Detection

On startup, `workflowio.DetectCrashTempFiles` scans for `.*.tmp` files left by a previous crashed session. If any are found and the originating process is no longer alive, they are reported in the TUI. Live PIDs are skipped (another writer is still active).

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--workflow-dir` | `<projectDir>/.pr9k/workflow/`, then `<executableDir>/.pr9k/workflow/` | Path to the workflow bundle directory |
| `--project-dir` | Current working directory | Path to the target repository (governs log file location) |

Note: `--iterations` / `-n` is **not available** on the `workflow` subcommand. The builder is a standalone editor, not a runner.

## Log Directory Resolution

The builder log base directory is resolved in this order (D-44):
1. `--project-dir` flag value (if provided)
2. Current working directory (`os.Getwd()`)
3. `os.UserConfigDir()/.pr9k/` (fallback if CWD is unavailable)
4. `os.TempDir()` (last resort)

## Out of Scope

- Running the workflow (use `pr9k` without the `workflow` subcommand)
- Multi-user collaboration or locking
- Undo/redo history beyond the current session's in-memory state
- Phase-aware step ordering in the UI (steps are displayed in flat order matching `config.json` sections)

## Related Documentation

- [`docs/how-to/using-the-workflow-builder.md`](../how-to/using-the-workflow-builder.md) — Step-by-step walkthrough
- [`docs/how-to/configuring-external-editor-for-workflow-builder.md`](../how-to/configuring-external-editor-for-workflow-builder.md) — `$VISUAL`/`$EDITOR` setup and quoting rules
- [`docs/code-packages/workflowedit.md`](../code-packages/workflowedit.md) — Package-level API reference
- [`docs/code-packages/workflowio.md`](../code-packages/workflowio.md) — Load, Save, and detect operations
- [`docs/adr/20260424120000-workflow-builder-save-durability.md`](../adr/20260424120000-workflow-builder-save-durability.md) — Save durability decision record
- [`docs/features/cli-configuration.md`](cli-configuration.md) — CLI flags and directory resolution

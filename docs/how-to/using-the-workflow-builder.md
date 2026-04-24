# Using the Workflow Builder

The workflow builder (`pr9k workflow`) is an interactive TUI for creating and editing `config.json` workflow bundles. This guide walks through the common tasks: opening a bundle, editing a step, saving, handling a fatal finding, handling a file conflict, and quitting.

## Prerequisites

- `pr9k` installed and on your `$PATH` (or run from the repo with `./bin/pr9k`)
- A workflow bundle directory (either the default shipped bundle or a custom one)
- A terminal with at least 80 columns

## Starting the Builder

```
pr9k workflow
```

This opens the builder with the default workflow bundle (resolved the same way as the main runner: `<projectDir>/.pr9k/workflow/` first, then `<executableDir>/.pr9k/workflow/`).

To open a specific bundle:

```
pr9k workflow --workflow-dir /path/to/my-workflow
```

To set the project directory (governs where the session log is written):

```
pr9k workflow --project-dir /path/to/my-project
```

## Understanding the Layout

```
┌──────────────────────────────────────────────────────────────┐
│  File                                              [menu bar] │
├──────────────────────┬───────────────────────────────────────┤
│  ▸ initialize        │  Name:         get-next-issue         │
│    get-next-issue    │  Kind:         shell                  │
│    feature-work      │  Command:      scripts/get_next_issue │
│  ▸ iteration         │  Capture As:   ISSUE_ID               │
│    test-planning     │                                       │
│  ▸ finalize          │  [scroll for more fields]             │
│    code-review       │                                       │
├──────────────────────┴───────────────────────────────────────┤
│  Ctrl+S save  Ctrl+Q quit  Ctrl+O open in editor  F10 menu   │
└──────────────────────────────────────────────────────────────┘
```

- **Left panel (outline)**: step list organized by phase (initialize, iteration, finalize). Navigate with `↑`/`↓` or `j`/`k`.
- **Right panel (detail)**: editable fields for the focused step. Press `Tab` to move focus here.
- **Footer**: current shortcut hints.

## Opening an Existing Bundle

1. Press `F10` to open the File menu.
2. Navigate to **Open…** and press `Enter`, or use the keyboard shortcut.
3. A path-picker dialog opens. Type a path prefix and press `Tab` to auto-complete, or type the full path.
4. Press `Enter` to load the bundle.

The builder validates the bundle on load. If the current session has unsaved changes, a "Unsaved changes" dialog appears first asking whether to save, discard, or cancel.

## Editing a Step

1. In the outline, navigate to the step you want to edit.
2. Press `Tab` to move focus to the detail pane.
3. Use `↑`/`↓` to navigate between fields.
4. Press `Enter` to begin editing the focused field.
5. Type the new value.
6. Press `Enter` again to confirm, or `Esc` to cancel.

The step name in the outline updates immediately to reflect the `Name` field. An asterisk (`*`) in the title bar indicates unsaved changes.

## Adding or Removing Steps

Steps are added and removed from the outline panel:

- **Add step**: Press `n` in the outline to insert a new step after the cursor position.
- **Remove step**: Press `d` in the outline. A confirmation dialog appears; press `d` to confirm or `Esc` to cancel.
- **Reorder steps**: Press `r` to enter reorder mode. Use `↑`/`↓` to move the selected step. Press `r` or `Enter` to exit reorder mode.

## Opening a Companion File in an External Editor

Companion files (prompt files referenced by claude steps, and scripts referenced by shell steps) can be opened in your configured external editor.

1. Navigate to the step and focus the detail pane.
2. Move to the `Prompt File` or `Command` field.
3. Press `Ctrl+O`.
   - If the field is empty, a path-picker dialog opens first. Complete the path and press `Enter`.
   - If the field has a value, the editor opens immediately.
4. Edit the file in your editor. Save and close the editor.
5. The builder reloads the companion file automatically.

See [`configuring-external-editor-for-workflow-builder.md`](configuring-external-editor-for-workflow-builder.md) for setup instructions.

## Saving

Press `Ctrl+S` to save. The save follows a three-stage flow:

### Stage 1: Validation

The builder runs the D13 validator against the current document. This validates:
- Step names are non-empty
- Claude steps have a model set
- Prompt files exist (or have in-memory content)
- Command scripts exist and are within `workflowDir`
- Environment variable names are valid

### Stage 2: Findings

**If there are fatal errors**: the findings panel opens.

```
┌─── Findings ──────────────────────────────────────────────────┐
│ ✗ feature-work: model is required for claude steps            │
│ ✗ test-planning: prompt file not found: prompts/test.md       │
└───────────────────────────────────────────────────────────────┘
│  Enter jump-to-step  Esc close                                │
```

- Use `↑`/`↓` to scroll.
- Press `Enter` to jump to the first referenced step.
- Press `Esc` to close the panel. Fix the errors and press `Ctrl+S` again.

**If there are only warnings**: an acknowledgment dialog opens. Press `Enter` or `y` to proceed with the save, or `Esc` to cancel and review the warnings.

**If validation is clean**: the save proceeds immediately.

### Stage 3: Write to Disk

Companion files (prompt files, scripts) are written first, then `config.json`. Both are written atomically (temp-file + rename). A "Saved at HH:MM:SS" banner appears at the bottom on success.

If the save fails, an error dialog shows the reason (permission denied, disk full, cross-device rename, etc.).

## Handling a Fatal Finding

If the validator finds a fatal error:

1. The findings panel opens automatically.
2. Read the error messages. Each error shows the step name and field.
3. Press `Enter` to jump to the referenced step, or `Esc` to close the panel and navigate manually.
4. Fix the error in the detail pane.
5. Press `Ctrl+S` again. If all fatals are resolved, the save proceeds.

## Handling a File Conflict

A conflict occurs when `config.json` is modified on disk after you loaded or last saved it. This is detected using nanosecond-precision mtime comparison.

When a conflict is detected, a dialog appears:

```
┌─── File Conflict ─────────────────────────────────────────────┐
│  config.json has been modified since you last loaded it.      │
│                                                               │
│  r  reload from disk (discard unsaved changes)               │
│  f  force save (overwrite disk version)                       │
│  Esc  cancel                                                  │
└───────────────────────────────────────────────────────────────┘
```

- Press `r` to reload from disk (your unsaved changes are discarded).
- Press `f` to force-save (the disk version is overwritten).
- Press `Esc` to cancel and decide later.

## Quitting

Press `Ctrl+Q` to quit.

- If there are no unsaved changes, the builder quits immediately.
- If there are unsaved changes, a dialog appears:

```
┌─── Unsaved Changes ───────────────────────────────────────────┐
│  You have unsaved changes.                                    │
│                                                               │
│  s  save and quit                                             │
│  d  discard changes and quit                                  │
│  Esc  cancel                                                  │
└───────────────────────────────────────────────────────────────┘
```

  - Press `s` to save (follows the full save flow above) then quit.
  - Press `d` to discard changes and quit immediately.
  - Press `Esc` to cancel and return to the builder.

## Starting a New Session

Press `Ctrl+N` to start a new session from the default scaffold. If there are unsaved changes, the unsaved-changes dialog appears first.

The scaffold creates a single placeholder shell step with the name `new-step`. It is not written to disk until you press `Ctrl+S`.

## Related Documentation

- [`docs/features/workflow-builder.md`](../features/workflow-builder.md) — Feature reference
- [`docs/how-to/configuring-external-editor-for-workflow-builder.md`](configuring-external-editor-for-workflow-builder.md) — External editor setup
- [`docs/how-to/building-custom-workflows.md`](building-custom-workflows.md) — Step types, variables, and config schema
- [`docs/features/cli-configuration.md`](../features/cli-configuration.md) — Directory resolution

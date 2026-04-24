# workflowedit

The `internal/workflowedit` package is the Bubble Tea model for the workflow builder TUI. It wires together the outline panel, detail pane, menu bar, dialogs, findings panel, path picker, and session-event log formatters.

- **Last Updated:** 2026-04-24
- **Authors:**
  - River Bailey

## Overview

- `Model` is the top-level `tea.Model` for the workflow builder TUI
- `SaveFS` and `EditorRunner` are injected at construction time for testability
- The update routing order is: help modal → dialog → global key → edit view (D-9)
- `Del` key is scoped to the outline panel only (D-10)
- Two-viewport layout: outline (40% of width, min 20 cols, max 40 cols) + detail pane (remaining width) (D-12)
- Session-event log formatters in `session_log.go` follow F-113 (no containerEnv/env/prompt-file content in logs)

Key files:

- `src/internal/workflowedit/model.go` — `Model`, `New`, `WithSharedInstallWarning`, `Update`, `View`, global key routing
- `src/internal/workflowedit/dialogs.go` — `DialogKind` constants, `dialogState`, D-8 single-slot composition
- `src/internal/workflowedit/outline.go` — step list panel with viewport, cursor, collapsible sections, reorder mode
- `src/internal/workflowedit/detail.go` — field-editing pane with masked sensitive values, revealed-field toggle, auto-scroll
- `src/internal/workflowedit/menu.go` — menu bar + File dropdown with F10 toggle
- `src/internal/workflowedit/footer.go` — `ShortcutLine()` delegates to focused widget
- `src/internal/workflowedit/findings.go` — `findingsPanel`, `findingResult`, `buildFindingsPanel`, `syncViewport`
- `src/internal/workflowedit/pathpicker.go` — `pathPickerModel`, `completePath`, `scanMatches`
- `src/internal/workflowedit/session_log.go` — D-27 log-line formatters
- `src/internal/workflowedit/editor.go` — `EditorRunner` interface, `ExecCallback` type
- `src/cmd/pr9k/editor.go` — `realEditorRunner`, `resolveEditor`, `makeExecCallback`

## Core Types

```go
// Model is the top-level Bubble Tea model for the workflow-builder TUI.
type Model struct { /* unexported fields */ }

// New constructs a Model with the provided dependency injections.
func New(saveFS workflowio.SaveFS, editor EditorRunner, projectDir, workflowDir string) Model

// WithSharedInstallWarning returns a copy of the model with the shared-install
// warning banner set (D-43). Empty string clears the warning.
func (m Model) WithSharedInstallWarning(msg string) Model
```

## EditorRunner Interface

```go
// EditorRunner opens a file in an external editor and returns a tea.Cmd that
// resolves to a message when the editor exits.
type EditorRunner interface {
    Run(file string) tea.Cmd
}

// ExecCallback is the function type used as a process-exit callback.
// It is called with the error returned by exec.Cmd.Wait().
type ExecCallback func(err error) tea.Msg
```

The production implementation lives in `src/cmd/pr9k/editor.go`:

```go
// realEditorRunner resolves the editor binary from $VISUAL/$EDITOR and
// returns an EditorRunner. resolveEditor word-splits via shlex and rejects
// shell metacharacters (D-22, D-33).
type realEditorRunner struct { ... }
```

## Dialog System

The builder uses a single-slot dialog composition (D-8): at most one dialog is visible at a time. The active dialog is stored in `dialogState.kind`.

| DialogKind | Trigger | Keys |
|------------|---------|------|
| `DialogNone` | — | — |
| `DialogPathPicker` | `Ctrl+O` / File→Open | `Tab`/`Shift+Tab` cycle completions, `Enter` load bundle, `Esc` cancel |
| `DialogNewChoice` | File→New | `e` create empty doc, `c` copy from default bundle, `Esc` cancel |
| `DialogUnsavedChanges` | `Ctrl+Q` when dirty | `s` save+quit, `d` discard, `Esc` cancel |
| `DialogQuitConfirm` | `Ctrl+Q` when clean | `y` confirm quit, `Esc` cancel |
| `DialogFindingsPanel` | Fatal errors after validation | scrollable findings, `Enter` jump, `Esc` close |
| `DialogAcknowledgeFindings` | Warnings-only after validation | `Enter`/`y` proceed, `Esc` cancel |
| `DialogError` | Save error | `Esc` / `Enter` dismiss |
| `DialogFileConflict` | mtime mismatch on save | `r` reload, `f` force-save, `Esc` cancel |
| `DialogSaveInProgress` | `Ctrl+Q` during save/validate | `Esc` cancel |
| `DialogRemoveConfirm` | `d` in outline | `d` confirm, `Esc` cancel |
| `DialogCrashTempNotice` | Orphaned `.*.tmp` files detected at open | `Esc` dismiss |
| `DialogFirstSaveConfirm` | First save to external or symlinked workflow | confirm / cancel |
| `DialogRecovery` | Malformed `config.json` detected on load | recovery view display, `Esc` close |
| `DialogExternalEditorOpening` | External editor launching | — (auto-dismiss on editor exit) |

## Async Save Flow

`Ctrl+S` triggers a three-stage state machine:

```
idle
  → validateInProgress (makeValidateCmd dispatched)
  → handleValidateComplete
      → if fatals:   DialogFindingsPanel  → idle on Esc
      → if warnings: DialogAcknowledgeFindings → Enter → saveInProgress
      → if clean:    saveInProgress (makeSaveCmd dispatched)
  → handleSaveResult
      → on success: saveBanner set, saveSnapshot updated
      → on error:   DialogError with kind-specific message
  → idle
```

During `validateInProgress` or `saveInProgress`, `Ctrl+Q` shows `DialogSaveInProgress` instead of `DialogUnsavedChanges`.

## Path Picker

The path picker (`pathPickerModel`) provides async tab completion:

- `Tab` with no cached matches dispatches `completePath` (async scan)
- `Tab` with cached matches cycles forward synchronously
- `Shift+Tab` cycles backward
- Rune input and backspace clear the match cache so the next `Tab` rescans
- `Enter` dispatches `makeLoadCmd` which calls `workflowio.Load` on the selected path; the result is delivered as `openFileResultMsg` and populates `m.doc` and `m.companions`
- Hidden files are excluded unless the prefix starts with `.`
- Matches are sorted alphabetically; directories get a trailing `/`
- An empty prefix is treated as the current working directory

## Session Log Formatters

`session_log.go` contains D-27 log-line formatters. All return a plain string with no field values from the document:

```go
func fmtSessionStart(workflowDir string) string
func fmtWorkflowSaved(path string) string
func fmtSaveFailed(kind workflowio.SaveErrorKind) string  // closed enum switch
func fmtEditorOpened(editorCmd string) string              // first token only
func fmtEditorSigint() string
func fmtQuitClean() string
func fmtQuitDiscarded() string
func fmtQuitCancelled() string
func fmtSharedInstallDetected() string
```

`fmtSaveFailed` uses an exhaustive switch over `SaveErrorKind` values — unrecognized kinds produce a compile error, not a runtime panic.

`fmtEditorOpened` logs only the binary name (last path component before the first space). A full command like `/opt/Sublime Text/subl --wait` logs `subl`.

## Update Routing

The `Update` method routes messages in this order (D-9):

1. `helpOpen` → `updateHelpModal`
2. `dialog.kind != DialogNone` → `updateDialog` (dispatches to per-dialog handler)
3. `isGlobalKey(msg)` (F10, Ctrl+N, Ctrl+O, Ctrl+S, Ctrl+Q) → `handleGlobalKey`
4. default → `updateEditView` (routes to outline, detail, or menu based on focus)

`Del` is handled only inside `updateEditView` (D-10) — it is not in the global key list. This prevents accidental step deletion while a dialog is open.

## Synchronization

`workflowedit` is a pure Bubble Tea model: all state mutations happen inside `Update`, which is called synchronously by the Bubble Tea runtime. No goroutines or mutexes are used within the package. Async operations (validation, save, path completion, editor launch) are dispatched as `tea.Cmd` values and their results arrive as `tea.Msg` values on the next `Update` call.

## Testing

- `src/internal/workflowedit/model_test.go` — 47+ tests (mode tests Part A–E, update routing, Del scoping, gap tests)
- `src/internal/workflowedit/save_flow_test.go` — 16 tests (async save state machine, conflict detection, Ctrl+N reset)
- `src/internal/workflowedit/findings_panel_test.go` — 5 tests (panel construction, scroll, ack tracking)
- `src/internal/workflowedit/pathpicker_test.go` — 11 tests (cycling, hidden files, tilde, backspace, char input, enter)
- `src/internal/workflowedit/session_log_test.go` — 10 tests (format functions, F-113 no-leak guarantees, exact return values)
- `src/internal/workflowedit/shared_install_test.go` — 2 tests (banner present for different UID, absent for same UID; unix-only)
- Additional render tests: `outline_render_test.go`, `detail_pane_render_test.go`, `menu_bar_render_test.go`, `dialogs_render_test.go`, `footer_test.go`, `viewport_test.go`

## Related Documentation

- [`docs/features/workflow-builder.md`](../features/workflow-builder.md) — User-facing feature reference
- [`docs/how-to/using-the-workflow-builder.md`](../how-to/using-the-workflow-builder.md) — Usage walkthrough
- [`docs/code-packages/workflowmodel.md`](workflowmodel.md) — `WorkflowDoc` held by the model
- [`docs/code-packages/workflowio.md`](workflowio.md) — Load and Save operations
- [`docs/code-packages/workflowvalidate.md`](workflowvalidate.md) — Validation bridge
- [`docs/adr/20260424120000-workflow-builder-save-durability.md`](../adr/20260424120000-workflow-builder-save-durability.md) — Save durability decisions

# TUI Mode Coverage: Workflow Builder

<!--
The 28 distinct TUI modes / focus states / overlay combinations the
workflow builder supports, each with a committed observable-behavior
test case. Produced during iterative-plan-review R4 in response to
F-100 (the earlier test-plan artifact at /tmp was unreachable).

Every entry specifies: (1) starting state — mode plus any overlay
currently active; (2) input — keyboard / mouse event delivered;
(3) expected next state; (4) expected observable change the test
asserts.

The corresponding test lives at `src/internal/workflowedit/model_test.go`
with function name prefix `TestModel_Mode_*`.
-->

## Part A — Empty-editor modes (no workflow loaded)

| # | Starting state | Input | Expected next state | Observable change |
|---|----------------|-------|---------------------|-------------------|
| 1 | `EmptyEditor`, no overlay | `Ctrl+N` | `EmptyEditor + DialogNewChoice` | dialog renders "Copy / Empty / Cancel" |
| 2 | `EmptyEditor + DialogNewChoice` | `Esc` | `EmptyEditor`, no overlay | dialog closes, cursor unchanged |
| 3 | `EmptyEditor`, no overlay | `Ctrl+O` | `EmptyEditor + DialogPathPicker` | path picker pre-filled with `<projectDir>/.pr9k/workflow/config.json` |
| 4 | `EmptyEditor`, no overlay | `F10` | `EmptyEditor + menu open` | File menu dropdown visible |
| 5 | `EmptyEditor`, no overlay | `?` | `EmptyEditor + helpOpen` | help modal renders |

## Part B — Edit-view core modes

| # | Starting state | Input | Expected next state | Observable change |
|---|----------------|-------|---------------------|-------------------|
| 6 | `EditView`, outline focus on step row, no overlay | `↓` | `EditView`, outline focus advances | next row highlighted |
| 7 | `EditView`, outline focus on step row | `Tab` | `EditView`, focus moves to detail pane first field | detail-pane field highlighted |
| 8 | `EditView`, outline focus on step row | `Del` | `EditView + DialogStepRemoveConfirm` | confirm dialog "Delete step <name>? Delete / Cancel" |
| 9 | `EditView`, outline focus on step row | `Alt+↑` | `EditView`, step moves up one row | outline order updates; step index decremented |
| 10 | `EditView`, outline focus on step row (Alt blocked) | `r` | `EditView + reorder mode` | footer shows reorder shortcuts |
| 11 | `EditView + reorder mode` | `↑` | `EditView + reorder mode`, step moved | outline shows step at new position |
| 12 | `EditView + reorder mode` | `Enter` | `EditView`, reorder commits | `dirty` state true |
| 13 | `EditView + reorder mode` | `Esc` | `EditView`, reorder cancels | step restored to original position |
| 14 | `EditView`, detail-pane focus on choice-list field | `Enter` | `EditView + dropdown open` | choice list visible |
| 15 | `EditView + dropdown open` | typed char | `EditView + dropdown open`, selection jumps | next matching option highlighted |
| 16 | `EditView`, detail-pane focus on masked `containerEnv` value | `r` | `EditView`, value revealed | masked → plain, footer shows `r mask` |
| 17 | `EditView`, detail-pane focus on masked value | focus leaves field | value re-masked | plain → `••••••••` |

## Part C — Save-flow modes

| # | Starting state | Input | Expected next state | Observable change |
|---|----------------|-------|---------------------|-------------------|
| 18 | `EditView`, no overlay, saveInProgress=false | `Ctrl+S` with valid doc | `EditView + validateInProgress` → `EditView + saveInProgress` → `EditView` | banner `Saved at HH:MM:SS`; `dirty=false`; `saveSnapshot` populated |
| 19 | `EditView`, no overlay, saveInProgress=true | `Ctrl+S` | `EditView`, save gated | no new goroutine spawned; state unchanged |
| 20 | `EditView`, no overlay, saveInProgress=true | `Ctrl+Q` | `EditView + DialogSaveInProgress`, pendingQuit=true | dialog shows "Save in progress — please wait" |
| 21 | `EditView + DialogSaveInProgress`, pendingQuit=true | `saveCompleteMsg` arrives | standard quit-flow enters (QuitConfirm or unsaved-changes) | dialog transitions based on post-save state |
| 22 | `EditView`, valid doc | `Ctrl+S`, validator returns fatals | `EditView + DialogFindingsPanel` | panel renders fatals; save aborted; `dirty` unchanged |
| 23 | `EditView + DialogFindingsPanel` | `?` | `EditView + DialogFindingsPanel + helpOpen` | help modal over findings panel (only coexistence case) |

## Part D — Quit-flow modes

| # | Starting state | Input | Expected next state | Observable change |
|---|----------------|-------|---------------------|-------------------|
| 24 | `EditView`, no unsaved, no overlay | `Ctrl+Q` | `EditView + DialogQuitConfirm` | two-option `Yes / No` with No default |
| 25 | `EditView + DialogQuitConfirm` | `y` | program exit | `program.Quit` returned |
| 26 | `EditView`, unsaved changes | `Ctrl+Q` | `EditView + DialogUnsavedChanges` | three-option `Save / Cancel / Discard` with Cancel default |
| 27 | `EditView + DialogUnsavedChanges` | `s` (Save) with fatals | `EditView + DialogFindingsPanel` | quit canceled; findings panel open |

## Part E — Load / recovery modes

| # | Starting state | Input | Expected next state | Observable change |
|---|----------------|-------|---------------------|-------------------|
| 28 | `EmptyEditor`, `--workflow-dir` targets malformed config.json | launch | `EditView + DialogRecovery` | recovery view renders ANSI-stripped (StripAll), 8 KiB capped raw bytes; four actions (Open-in-editor / Reload / Discard / Cancel) |

## Cross-cutting assertions applied to every mode test

- `updateModel` returns a new `Model` (immutable pattern); the caller's Model is never mutated in place.
- Every non-`EmptyEditor` test threads a minimal populated `WorkflowDoc` through the Model.
- Tests pass `-race` cleanly per `docs/coding-standards/testing.md`.
- Tests use the `fakeFS` and `fakeEditorRunner` doubles — no real filesystem or TTY access.
- Focus-restoration is asserted on every transition that opens or closes an overlay (`prevFocus` == `currentFocus` before overlay, per D-55).

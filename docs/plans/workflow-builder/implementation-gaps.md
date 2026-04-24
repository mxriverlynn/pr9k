# Gap Analysis: Workflow Builder Plans vs Current Implementation

## Comparison Direction

- **Current state:** `src/` Go source tree at `/Users/mxriverlynn/dev/mxriverlynn/pr9k/src/` (and related repo files) as of branch `workflow-builder-mode`, commit `a84d697`.
- **Desired state:** `docs/plans/workflow-builder/feature-specification.md` (spec) and `docs/plans/workflow-builder/feature-implementation-plan.md` (plan).
- **Direction:** current state → desired state (current checked for gaps against what the two planning documents require).

## Scope

Comparison areas analyzed:

- CLI subcommand wiring (`cmd/pr9k/workflow.go` and its integration into `main.go` / `cli.Execute`).
- TUI runtime wiring (construction and execution of `workflowedit.Model`; connection of EditorRunner, SaveFS, logger, and detect helpers).
- `workflowedit` package behavior (menu bar, session header/banners, outline sections/affordances, detail-pane field editors, dialogs, findings panel, save flow, external editor handoff, path picker flows, session logging, help modal).
- `workflowmodel` package coverage of the `config.json` schema (top-level env/containerEnv, unknown fields, statusLine round-trip, DefaultModel).
- `workflowio` package (load/save/detect/crashtemp) and its integration with the TUI load pipeline.
- `atomicwrite` and `ansi` packages (referenced by plan as inner-ring dependencies).
- Validator extensions (`ValidateDoc`, `safePromptPath`, `validateCommandPath`) — OI-1 hardening.
- Logger extension (`NewLoggerWithPrefix`).
- Documentation obligations (feature doc, how-tos, ADR, coding standard, code-package docs).

Explicitly out of scope for this analysis: implementation quality of test coverage itself (only presence of required tests noted); release-logistics items OI-3/OI-4 (version-bump PR structure and single-vs-split PR).

## Summary

The workflow-builder plan committed to a single-PR ship of a user-facing `pr9k workflow` subcommand whose TUI opens, edits, validates, and saves workflow bundles. The inner-ring Go packages (`atomicwrite`, `ansi`, `workflowmodel`, `workflowio`, `workflowvalidate`, validator/logger extensions) are largely present and tested, but the entire user-facing surface — the cobra command is hidden, returns `nil` without constructing a `tea.Program`, and the `workflowedit.Model` has no caller. A dozen `workflowedit` widgets exist as code but are never reachable at runtime, and several spec behaviors (session header with priority banners, outline sections for env/containerEnv/statusLine/phase grouping, path picker in File > New, external editor invocation, unknown-field warn-and-drop, crash-temp notice, file-conflict/first-save/recovery actions, load-time banner ordering) are either missing entirely or stubbed. Documentation is present and tracked by doc-integrity tests.

| Category | Count | Description |
|----------|-------|-------------|
| Missing | 22 | Elements in desired state with no current state correspondence |
| Partial | 14 | Elements present in both but incompletely covered |
| Divergent | 3 | Elements addressing same concern in incompatible ways |
| Implicit | 2 | Assumed capabilities neither confirmed nor denied |

Full analysis written to: `/Users/mxriverlynn/dev/mxriverlynn/pr9k/docs/plans/workflow-builder/implementation-gaps.md`

## Findings

### 1. Subcommand wiring — the user-facing surface

**GAP-001: `pr9k workflow` subcommand does not launch any TUI**
- **Category:** Missing
- **Feature/Behavior:** Spec Primary Flow step 1 — "The user launches `pr9k workflow`. The subcommand accepts `--workflow-dir` and `--project-dir`… Otherwise the builder enters an empty-editor state…" Plan WU-2 and WU-7 deliver a cobra subcommand that constructs and runs `workflowedit.Model` via `tea.NewProgram`.
- **Current State:** `src/cmd/pr9k/workflow.go:74` sets up a logger, a signal goroutine, then returns `nil` with the comment `// TUI (workflowedit.Model) not yet wired — stub exits cleanly.` No `tea.NewProgram`, no `workflowedit.New(...)`, no call into `workflowedit` at all. The command is marked `Hidden: true` at `workflow.go:28`.
- **Desired State:** `feature-specification.md` Primary Flow §2 describes the edit-view layout and empty-editor state that must appear when the subcommand runs; `feature-implementation-plan.md` §"Architecture and Integration Points" (line 96) specifies `cli.Execute(newSandboxCmd(), newWorkflowCmd())` wiring and Definition of Done "`pr9k workflow` subcommand available and listed under `pr9k --help`".

**GAP-002: Subcommand is hidden from `pr9k --help`**
- **Category:** Divergent
- **Feature/Behavior:** Plan Definition of Done line 347: "`pr9k workflow` subcommand available and listed under `pr9k --help`".
- **Current State:** `src/cmd/pr9k/workflow.go:28` sets `Hidden: true`. The subcommand will not appear in help output.
- **Desired State:** Plan DoD explicitly requires the subcommand to be listed; spec §"Versioning" calls adding it "a backwards-compatible addition to the CLI surface" that "becomes part of pr9k's public API from the moment it ships."

**GAP-003: Signal handler does not call `program.Send(quitMsg)` or manage the `tea.Program` lifecycle**
- **Category:** Partial
- **Feature/Behavior:** Plan §"Runtime Behavior" (line 188): "On SIGINT/SIGTERM, the handler calls `program.Send(quitMsg{})` and `cancel()`; it **unconditionally** does not call `program.Kill()` for 10 seconds."
- **Current State:** `src/cmd/pr9k/workflow.go:61-72` notifies signals and cancels the context but does not hold a `program` reference (there is no `tea.Program`). The comment at `workflow.go:66` reads "program.Send(quitMsg{}) — wired when workflowedit is added."
- **Desired State:** Plan D-34 signal handler semantics require a live `tea.Program` to be receiving a graceful `quitMsg`.

### 2. TUI layout — menu bar, session header, banners

**GAP-004: Menu bar redesign — `File` menu dropdown items (`New`/`Open`/`Save`/`Quit`) with inline shortcut labels**
- **Category:** Partial
- **Feature/Behavior:** Spec §"Primary Flow" step 2 and D64–D67 describe a persistent `File` menu with four items whose inline shortcut labels (`Ctrl+N`/`Ctrl+O`/`Ctrl+S`/`Ctrl+Q`) are rendered next to each item, opened via `F10`, `Alt+F`, or mouse click, with keyboard `↑/↓ navigate  Enter select  Esc close` in the footer while open.
- **Current State:** `src/internal/workflowedit/menu.go` renders only `File` (closed) or `[ File  New  Open  Save  Quit ]` (open) with no per-item shortcut labels, no dropdown cursor, and no mouse activation or `Alt+F` binding. `model.go:495-498` handles only `tea.KeyF10`; there is no `Alt+F`, no click handler, no item-highlight state, and no keyboard dispatch within the open menu.
- **Desired State:** D65 "menu bar rendering," D66 "menu activation model," D67 "menu item keyboard shortcuts."

**GAP-005: Session header with target path, unsaved-changes indicator, and priority banners**
- **Category:** Missing
- **Feature/Behavior:** Spec Primary Flow §5 bullet 2 describes a session header on the third row that always shows the target path and unsaved-changes indicator and shows at most one warning banner at a time selected by priority (read-only, external-workflow, symlink, shared-install, unknown-field) plus a `[N more warnings]` affordance.
- **Current State:** `src/internal/workflowedit/model.go:130-154` `View()` renders menu → optional `sharedInstallWarning` line → dialog/empty/edit view → optional save banner → shortcut footer. No persistent session header exists; `sharedInstallWarning` is the only banner slot and it has no priority resolution. There is no target-path display, no unsaved-indicator glyph, no `[N more warnings]` affordance, no read-only or external-workflow or symlink or unknown-field banner.
- **Desired State:** Spec D49 "session header banner priority."

**GAP-006: Read-only browse-only mode**
- **Category:** Missing
- **Feature/Behavior:** Spec §"Read-only target (any source)": the builder must detect read-only destination at load time, show a prominent indicator, grey out File > Save, and disable dirty-tracking. D4/D30.
- **Current State:** `workflowio.DetectReadOnly` exists (`src/internal/workflowio/detect.go:31`), but no caller in `workflowedit` ever invokes it. The model has no `readOnly` flag, the menu does not grey Save, and `handleGlobalKey` `Ctrl+S` (model.go:506) has no read-only short-circuit.
- **Desired State:** Spec D4, D30; Edit-view "browse-only mode" behavior.

**GAP-007: External-workflow banner and first-save confirmation**
- **Category:** Missing
- **Feature/Behavior:** Spec §"External-workflow session": session-long banner plus first-save confirmation dialog when target lies outside project and home.
- **Current State:** `workflowio.DetectExternalWorkflow` exists (`src/internal/workflowio/detect.go:51`), but no caller. `DialogFirstSaveConfirm` is declared (`dialogs.go:17`) but has no Update handler and is never opened; `handleSaveResult` (`model.go:771`) never prompts for first-save confirmation.
- **Desired State:** Spec D22.

**GAP-008: Symlink banner and first-save confirmation**
- **Category:** Missing
- **Feature/Behavior:** Spec §"Symlinked target or companion file": symlink banner at load and confirmation at first save.
- **Current State:** `workflowio.DetectSymlink` exists and `Load` returns `IsSymlink`/`SymlinkTarget` (`src/internal/workflowio/load.go:71`), but `handleOpenFileResult` (`model.go:793-812`) discards these fields — no banner is ever rendered and no confirmation is triggered.
- **Desired State:** Spec D17.

**GAP-009: Unknown-field warn-on-load/drop-on-save**
- **Category:** Missing
- **Feature/Behavior:** Spec §"Unknown-field warning" + D18: parse must record unrecognized fields, banner must warn on load, save silently drops them.
- **Current State:** `workflowmodel.WorkflowDoc` has no `UnknownFields` field (plan §"Data Model" line 117 mandates one); `parseConfig` (`src/internal/workflowmodel/scaffold.go:76`) uses `json.Unmarshal` without `DisallowUnknownFields` and does not record unknowns; no banner code path exists.
- **Desired State:** Plan §"Data Model" and spec D18.

### 3. Outline — section grouping, affordances, reorder

**GAP-010: Outline does not group steps by phase; no env/containerEnv/statusLine sections**
- **Category:** Divergent
- **Feature/Behavior:** Spec Primary Flow §5 bullet 3: outline shows collapsible sections for env passthrough, containerEnv, statusLine, and initialize/iteration/finalize phases, each with an always-visible item count and ending in a `+ Add <item-type>` affordance row.
- **Current State:** `src/internal/workflowedit/outline.go:37-58` renders a flat list of `m.doc.Steps` with no section headers, no phase grouping, no env or containerEnv or statusLine sections, no item-count glyph, no collapse state, no `+ Add` row, and no scroll indicator.
- **Desired State:** Spec D28 (collapsible sections), D29 (outline scrollability), D46 (`+ Add` affordance), D51 (section summary content).

**GAP-011: Step rows do not show gripper glyph in normal (non-reorder) state**
- **Category:** Partial
- **Feature/Behavior:** Spec §"User Interactions — Outline" and D34: every step row in the outline carries a persistent gripper glyph (`⋮⋮`) on its left edge, regardless of reorder mode.
- **Current State:** `outline.go:47-54` shows `GlyphGripper` only when `reorderMode && i == cursor`; all other rows use `> ` or blank prefixes. The gripper is not a persistent signifier.
- **Desired State:** Spec Primary Flow §7 "Step order within a phase… Every step row in the outline carries a persistent gripper glyph (⋮⋮)."

**GAP-012: Step reorder never commits a dirty flag on cancel, and phase-boundary stop is unimplemented**
- **Category:** Partial
- **Feature/Behavior:** Spec D34: cross-phase drag is not supported; "dragging a step past a phase boundary visibly drops it at the phase's edge."
- **Current State:** `doMoveStepUp`/`doMoveStepDown` (`model.go:641-666`) swap across the flat `Steps` slice with no phase awareness — a step can be moved from iteration into initialize's slot freely, and marshal/save would re-bucket by `step.Phase` (which never changes), silently erasing the visual move. No phase-boundary stop is enforced.
- **Desired State:** Spec D34 and "Alternate Flows — … cross-phase drag is not supported."

**GAP-013: `+ Add <item-type>` affordance and `a`/`Enter` shortcuts absent**
- **Category:** Missing
- **Feature/Behavior:** Spec D46: adding a step/env/containerEnv entry is triggered from a visible `+ Add` row at the end of each section; `a` on a section header is equivalent.
- **Current State:** No `+ Add` row exists in `outline.go`. `handleOutlineKey` (`model.go:574-618`) has no `a` shortcut. `handleGlobalKey` does not include any "add step" binding.
- **Desired State:** Spec D46.

### 4. Detail pane — field editors and choice lists

**GAP-014: Detail pane renders only a static summary — no editable input boxes, no constraint hints, no invalid-input markers**
- **Category:** Partial
- **Feature/Behavior:** Spec Primary Flow §7 first and second bullets: plain-text fields rendered as input boxes with inline constraint hints; input sanitized at input time (newlines stripped, ANSI escapes stripped, soft length cap warning).
- **Current State:** `src/internal/workflowedit/detail.go:41-63` renders labels and values as plain lipgloss text (`fmt.Fprintf(&sb, "Name: %s\n", step.Name)`), with no input widget, no cursor, no keystroke routing to mutate the step, and no sanitization pipeline. `handleDetailKey` (`model.go:668-691`) moves a `detail.cursor` counter but never mutates any field of the selected step.
- **Desired State:** Spec Primary Flow §7 plain-text-field and input-sanitization requirements; D42 structured-field input sanitization.

**GAP-015: Choice-list fields for constrained enums (captureMode, onTimeout, statusLine type, isClaude) have no dropdown UI and no keyboard contract**
- **Category:** Missing
- **Feature/Behavior:** Spec Primary Flow §7 choice-list bullet and D12/D45: constrained fields render as choice lists with `▾` indicator, `Enter`/`Space` opens, `↑`/`↓` navigates, `Enter` confirms, `Escape` dismisses, typing-a-character jumps.
- **Current State:** `detailPane.dropdownOpen` bool exists (`detail.go:16`) and `handleDropdownKey` (`model.go:693-701`) handles only `Esc`/`Enter` closing. There is no enumeration of options, no option list render, no confirmation of selection into the model, no `▾` indicator, and no character-jump.
- **Desired State:** Spec D12, D45.

**GAP-016: Numeric fields have no numeric-only input handling or range enforcement**
- **Category:** Missing
- **Feature/Behavior:** Spec Primary Flow §7 numeric-field bullet and D62: non-numeric characters silently ignored, pasted input sanitized at first non-digit with a visible message, range enforced at input time.
- **Current State:** No numeric input path exists in `detail.go`; `timeoutSeconds` and `refreshIntervalSeconds` are rendered via the generic fmt path and cannot be edited.
- **Desired State:** Spec D62.

**GAP-017: Secret masking for containerEnv secrets has no `r` toggle in focus-leave path**
- **Category:** Partial
- **Feature/Behavior:** Spec D20/D47: containerEnv values whose key matches a secret pattern (`_TOKEN`, `_SECRET`, `_KEY`, `_PASSWORD`, `_PASSPHRASE`, `_CREDENTIAL`, `_APIKEY`) render as `••••••••` with `[press r to reveal]` label; `r` while focused toggles; field re-masks on focus-leave.
- **Current State:** `detail.go:53-62` renders every `env.Value` masked when `d.revealedField != i`, regardless of key pattern — non-secret literals are also masked. The `r` key sets `revealedField` (`model.go:687`) and `detail.dropdownOpen`-close path clears it (`model.go:676`), but no `[press r to reveal]` label is shown and no key-pattern matching gates masking.
- **Desired State:** Spec D20, D47.

**GAP-018: Model-field free-text with suggestion list is not wired to the detail pane**
- **Category:** Missing
- **Feature/Behavior:** Spec D12/D58 and plan D-42: the step's `model` field is rendered as free-text input with a suggestion list drawn from `workflowmodel.ModelSuggestions`.
- **Current State:** `ModelSuggestions` exists (`src/internal/workflowmodel/modelsuggestions.go:10-14`), but no code in `workflowedit` references it. The detail pane shows `Model: <string>` with no suggestion overlay.
- **Desired State:** Plan D-42 "scaffold model defaults and model suggestion list."

### 5. Dialogs and save flow

**GAP-019: Path picker is never shown for File > New**
- **Category:** Divergent
- **Feature/Behavior:** Spec Primary Flow §3(d) and D71: after a New choice (Copy / Empty), the builder shows a path-picker dialog pre-filled with `<projectDir>/.pr9k/workflow/` and waits for user confirmation before loading the workflow into the edit view.
- **Current State:** `updateDialogNewChoice` (`model.go:260-286`) on `e` or `c` writes directly into `m.doc` with no path picker, no destination selection, and no write-path setup. The workflow is loaded into memory against `m.workflowDir`, the ambient path, without a user-chosen target.
- **Desired State:** Spec D71 "path picker design" including tab-completion, inline warnings for existing `config.json` or directory, and Cancel default.

**GAP-020: Unsaved-changes interception missing for File > New and File > Open**
- **Category:** Missing
- **Feature/Behavior:** Spec Primary Flow §3(a)/§4(a) and D72: invoking File > New or File > Open with unsaved edits triggers the three-way Save/Cancel/Discard dialog; after Save-success or Discard the pending action resumes automatically.
- **Current State:** `handleGlobalKey` `Ctrl+N` (`model.go:498-501`) and `Ctrl+O` (`model.go:502-505`) open their respective dialogs unconditionally. The dirty flag is not consulted; there is no `pendingAction` field, no auto-resume after save, and no resume after Discard.
- **Desired State:** Spec D72, §"Unsaved-changes interception (Quit, File > New, File > Open)".

**GAP-021: Pre-copy integrity check for File > New > Copy-from-default missing**
- **Category:** Missing
- **Feature/Behavior:** Spec Primary Flow §3(c) and D61: before copying the default bundle, the builder must verify the default's internal references; a broken default triggers a Copy-anyway / Cancel dialog.
- **Current State:** `updateDialogNewChoice` calls `workflowmodel.CopyFromDefault` directly (`model.go:275`) without any integrity check; a broken bundle surfaces only as a generic parse error from `CopyFromDefault`, not as a Copy-anyway-vs-Cancel prompt.
- **Desired State:** Spec D61.

**GAP-022: `DialogFirstSaveConfirm` has no Update handler or trigger**
- **Category:** Missing
- **Feature/Behavior:** Spec §"External-workflow session" and §"Symlinked target or companion file" first-save confirmation; plan §"Runtime Behavior" save-flow step 6 implies a pre-save confirmation path for external/symlink targets.
- **Current State:** `DialogFirstSaveConfirm` kind is declared (`dialogs.go:17`) and renders a one-line placeholder (`model.go:194-195`), but `updateDialog` (`model.go:230-258`) has no `case DialogFirstSaveConfirm` and no code path sets `m.dialog.kind = DialogFirstSaveConfirm`.
- **Desired State:** Spec D22, D17.

**GAP-023: `DialogFileConflict` has no Update handler**
- **Category:** Partial
- **Feature/Behavior:** Spec Edge Cases row "Configuration file is modified on disk since the builder loaded it" and D41: overwrite / reload / cancel choices.
- **Current State:** `handleGlobalKey` opens `DialogFileConflict` on mtime mismatch (`model.go:515-520`) but `updateDialog` does not list it; any keystroke during this dialog reaches the default branch which only closes on `Esc`. There is no overwrite path, no reload path.
- **Desired State:** Spec D41.

**GAP-024: `DialogCrashTempNotice` has no Update handler or load-time trigger**
- **Category:** Missing
- **Feature/Behavior:** Spec §"Crash-era temporary file on open" and D42-a: non-blocking notice on open offering delete-silently or leave.
- **Current State:** `DialogCrashTempNotice` is declared (`dialogs.go:16`) and rendered as a one-line placeholder (`model.go:192-193`). `workflowio.DetectCrashTempFiles` exists (`src/internal/workflowio/crashtemp.go:37`), but `handleOpenFileResult` (`model.go:793`) never calls it; the notice is never shown.
- **Desired State:** Spec D42-a.

**GAP-025: `DialogRecovery` has no reload / open-in-editor / discard actions**
- **Category:** Partial
- **Feature/Behavior:** Spec §"Parse-error recovery": four actions — open raw file in external editor, reload, discard, cancel. After successful open-in-editor, attempt reload.
- **Current State:** `updateDialogRecovery` (`model.go:480-490`) handles only `Esc`/`c` (cancel). There is no reload, no open-in-editor, no discard branch; the raw bytes payload is shown inline but cannot be acted upon.
- **Desired State:** Spec D36.

**GAP-026: No-op save (zero findings + no dirty changes) does not short-circuit the save flow**
- **Category:** Partial
- **Feature/Behavior:** Spec Primary Flow §9 third bullet and D63: if there are no findings and no in-memory changes, the save is a no-op with `No changes to save` feedback; file is not rewritten.
- **Current State:** The check lives only inside `workflowio.Save` (`src/internal/workflowio/save.go:63`), which short-circuits if `IsDirty` is false. The TUI still calls `makeValidateCmd` then `makeSaveCmd`, and `handleSaveResult` always renders `Saved at HH:MM:SS` on success (`model.go:785`). The `No changes to save` string never appears.
- **Desired State:** Spec D63.

**GAP-027: Findings panel acknowledgment-dialog-only suppression not implemented**
- **Category:** Missing
- **Feature/Behavior:** Spec Primary Flow §9 second bullet and D23: a warning acknowledged during a session is not surfaced again at the acknowledgment dialog for the remainder of the session, though it continues to appear in the findings panel the user can open manually.
- **Current State:** `buildFindingsPanel` (`src/internal/workflowedit/findings.go:38-75`) carries an `ackSet` but it is not consulted anywhere in `updateDialogAcknowledgeFindings` (`model.go:411-426`), which re-shows every warning on every save without filtering by ack state.
- **Desired State:** Spec D23.

**GAP-028: Acknowledgment dialog short-circuits to save without user acknowledgment feedback**
- **Category:** Partial
- **Feature/Behavior:** Spec Primary Flow §9 second bullet: "the save proceeds after the user acknowledges the findings panel."
- **Current State:** `updateDialogAcknowledgeFindings` on `y`/Enter calls `makeSaveCmd` directly (`model.go:420-423`), without first persisting the acknowledged finding keys into `m.findingsPanel.ackSet`. The ack-record step is absent, so a subsequent save re-shows the same warnings.
- **Desired State:** Spec D23 "per-session warning suppression."

### 6. External editor handoff

**GAP-029: External editor invocation is never triggered from the TUI**
- **Category:** Missing
- **Feature/Behavior:** Spec Primary Flow §7 "Multi-line content — prompt files and scripts — is edited by handing control of the terminal to the user's configured external editor" and spec §"External-editor invocation"; plan D-26 two-step render (DialogExternalEditorOpening then tea.Tick then editorRunner.Run).
- **Current State:** `EditorRunner` interface exists (`src/internal/workflowedit/editor.go:11`), `realEditorRunner` production impl exists (`src/cmd/pr9k/editor.go:34`), and `DialogExternalEditorOpening` is declared — but no code path in `workflowedit` calls `m.editor.Run(...)`. There is no `openEditorMsg`, no `launchEditorMsg`, no focus-on-promptFile-field → `Enter` pipeline, no footer shortcut to open the editor, and no `editorExitMsg`/`editorSigintMsg` handler in `Update`.
- **Desired State:** Spec D5/D33; plan D-6, D-7, D-26.

**GAP-030: Recovery view offers no "open raw in external editor" action**
- **Category:** Missing
- **Feature/Behavior:** Spec §"Parse-error recovery": "open the raw file in the external editor… After a successful open-in-editor invocation from this view, the builder automatically attempts to reload."
- **Current State:** `updateDialogRecovery` (`model.go:480-490`) has no editor-invocation branch; even if it did, no editor hookup exists.
- **Desired State:** Spec D36.

**GAP-031: `editorExitMsg` / `editorSigintMsg` / `editorRestoreFailedMsg` produced by `cmd/pr9k/editor.go` are never consumed by the Model**
- **Category:** Missing
- **Feature/Behavior:** Plan §"Runtime Behavior" external-editor-handoff step 4: a three-way type switch routes the callback outcomes back into `Update` to re-read the file, surface quit-confirm, or show a RestoreTerminal-failure dialog.
- **Current State:** These message types are declared in `src/cmd/pr9k/editor.go:22-32` but `workflowedit.Model.Update` (`model.go:102-113`) and `updateEditView` (`model.go:542-558`) have no case for any of them. The `openFileResultMsg` is the only application-defined message currently handled.
- **Desired State:** Plan F-107 three-way type switch; D-7 SIGINT branch.

### 7. Help, session logging, shared-install

**GAP-032: `?` help modal content is a one-line placeholder, not a per-mode keyboard-shortcut listing**
- **Category:** Partial
- **Feature/Behavior:** Spec Primary Flow §8: "`?` opens a help modal listing every keyboard shortcut for the current mode."
- **Current State:** `renderHelpModal` (`model.go:158-160`) returns a literal one-line string `"Help: Ctrl+N new  Ctrl+O open  Ctrl+S save  Ctrl+Q quit  ?  close help"`. There is no per-mode shortcut listing, no reorder-mode help, no dropdown-open help, no findings-panel help.
- **Desired State:** Spec D24.

**GAP-033: Session-event logging helpers exist but are never called from the Model**
- **Category:** Missing
- **Feature/Behavior:** Plan §"Runtime Behavior"/§"External Interfaces"/D-27: session_start, workflow_saved, save_failed, editor_opened, editor_sigint, quit_clean, quit_discarded_changes, quit_cancelled, shared_install_detected events must be logged via `log.Log("workflow-builder", line)`.
- **Current State:** Format helpers (`fmtSessionStart`, `fmtWorkflowSaved`, `fmtSaveFailed`, `fmtEditorOpened`, `fmtEditorSigint`, `fmtQuitClean`, `fmtQuitDiscarded`, `fmtQuitCancelled`, `fmtSharedInstallDetected`) exist in `src/internal/workflowedit/session_log.go`. No call site inside `Update` invokes them, no logger is injected into the Model (the Model has no logger dependency), and `runWorkflowBuilder` constructs a logger but never hands it to a Model.
- **Desired State:** Plan D-27 field-exclusion contract.

**GAP-034: `WithSharedInstallWarning` exists but no detection path calls it**
- **Category:** Partial
- **Feature/Behavior:** Spec §"Edge Cases" row "User is editing the bundled default on a writable shared install" and D43.
- **Current State:** The builder (`workflow.go:runWorkflowBuilder`) does not call `workflowio.DetectSharedInstall` nor `WithSharedInstallWarning`. The `sharedInstallWarning` slot is always empty in production.
- **Desired State:** Spec D39, plan D-43.

### 8. Load pipeline — ordering, banners, companion handling

**GAP-035: Load-pipeline ordering (symlink detect → banner → parse → recovery view) not implemented on the UI side**
- **Category:** Partial
- **Feature/Behavior:** Plan §"Decomposition" WU-10: "load-pipeline ordering (symlink detect → banner → parse → recovery view)."
- **Current State:** `workflowio.Load` performs the staged reads in order (`src/internal/workflowio/load.go:62-156`), but the TUI's `handleOpenFileResult` (`model.go:793-812`) ignores `result.IsSymlink`, `result.SymlinkTarget`, and any detection-layer banners. A symlinked config.json loads silently into the edit view.
- **Desired State:** Plan D-23.

**GAP-036: Companion file key convention in load is `prompts/<file>` but validator expects bare filename for in-memory hit**
- **Category:** Partial
- **Feature/Behavior:** Plan test §"validate_test.go" line 305: "`ValidateDoc` given a full-path key (`prompts/step-1.md`) treats it as a cache miss and reads from disk; given a bare filename key (`step-1.md`) it uses the in-memory bytes." This is the F-121 convention.
- **Current State:** `workflowio.Load` builds `companions[relKey] = data` where `relKey = filepath.Join("prompts", step.PromptFile)` (`src/internal/workflowio/load.go:119, 149`). These are full-path keys. When `workflowedit.Model.runValidate` passes them to `ValidateDoc`, each will miss the in-memory cache and re-read from disk per F-121. The TUI therefore never exercises the in-memory validation path plan T3 requires.
- **Desired State:** Plan T3 "the validator sees exactly the state the save will write — no subset, no superset."

**GAP-037: Load does not load scripts (only `promptFile`-referenced companions)**
- **Category:** Partial
- **Feature/Behavior:** Spec D15 "companion file copy scope": configuration, every referenced prompt, every referenced script, and the statusLine script when present.
- **Current State:** `workflowio.Load` iterates only `doc.Steps` `PromptFile` fields (`src/internal/workflowio/load.go:115-150`). Shell-step `Command[0]` script references and the `statusLine.Command` script are not loaded as companions.
- **Desired State:** Spec D15 and spec §"Copy from default."

### 9. Schema coverage in model and serialization

**GAP-038: `WorkflowDoc` has no top-level `env` or `containerEnv`; marshal/parse drop them**
- **Category:** Missing
- **Feature/Behavior:** Spec Primary Flow §5 bullet 3 describes outline sections for top-level env passthrough and containerEnv; plan §"Data Model" references "EnvEntry" at the top level; validator Category 10 checks env passthrough.
- **Current State:** `src/internal/workflowmodel/model.go:64-68` `WorkflowDoc` contains only `DefaultModel`, `StatusLine`, and `Steps`. `parseConfig` (`src/internal/workflowmodel/scaffold.go:76-105`) never reads an `env` or `containerEnv` array; `marshalDoc` (`src/internal/workflowio/marshal.go`) never writes them. Loading a config.json that contains top-level `env` or `containerEnv` silently drops those fields, and saving will never emit them.
- **Desired State:** Spec D46, existing config.json schema (`docs/code-packages/steps.md`), validator Category 10.

**GAP-039: `DefaultModel` field exists but parse and marshal never read or write it**
- **Category:** Divergent
- **Feature/Behavior:** Plan §"Data Model" `WorkflowDoc` types include `DefaultModel`.
- **Current State:** `workflowmodel.WorkflowDoc.DefaultModel` is declared (`model.go:65`) and referenced in `deepCopyDoc` (`workflowedit/model.go:939`), but `parseConfig` never deserializes it from JSON and `marshalDoc` never serializes it. A workflow's `defaultModel` (if that field exists in the schema) will round-trip to the empty string and vanish on save.
- **Desired State:** Plan line 117 "Steps carry a companion `IsClaudeSet` bool… Unknown fields encountered on load are recorded in `WorkflowDoc.UnknownFields`."

**GAP-040: Step-level `env` and `containerEnv` are not parsed or marshaled**
- **Category:** Missing
- **Feature/Behavior:** Spec Primary Flow §7 (container-environment secret masking) and config.json schema: each step may carry `env` and `containerEnv` fields.
- **Current State:** `Step` has an `Env []EnvEntry` field (`model.go:53`), but `convertStep` (`scaffold.go:107-136`) never populates it from the parsed JSON — `rawStep` has no `Env`/`ContainerEnv` fields at all. `marshalDoc` similarly omits the field on output. Any step env entries loaded from disk are silently dropped and subsequent saves will erase them.
- **Desired State:** Config.json schema, spec D20.

**GAP-041: Unknown-field capture has no model support**
- **Category:** Missing
- **Feature/Behavior:** Plan §"Data Model" line 117: "Unknown fields encountered on load are recorded in `WorkflowDoc.UnknownFields`."
- **Current State:** No `UnknownFields` field exists in `WorkflowDoc` (`model.go`) and no parse code records unknowns.
- **Desired State:** Plan §"Data Model," spec D18.

### 10. Validator (OI-1) — observations

**GAP-042: `validateCommandPath` applies EvalSymlinks only for relative command paths**
- **Category:** Implicit
- **Feature/Behavior:** Plan §"External Interfaces" "OI-1 containment precision": "`safePromptPath` and the new `validateCommandPath` guard apply `EvalSymlinks` to BOTH the candidate path AND the bundle root."
- **Current State:** `validator.go:745-776` applies EvalSymlinks only for relative command paths; absolute and `$PATH`-resolved commands bypass the containment check. This may be intentional (absolute commands on `$PATH` are trusted), but the plan text does not draw this distinction explicitly.
- **Desired State:** Plan line 198 (may be implicit — the plan does not state whether absolute commands are exempt).

### 11. Documentation — doc-integrity

**GAP-043: Doc-integrity tests for workflow-builder exist but the associated implementation behaviors are not verified at runtime**
- **Category:** Implicit
- **Feature/Behavior:** Plan WU-11 and Definition of Done line 351: "Doc-integrity tests DI-1 through DI-8 pass."
- **Current State:** `src/cmd/pr9k/doc_integrity_test.go` contains tests at lines 1001, 1010, 1024, 1074, 1139 that assert the doc files exist and link correctly. The docs are present at `docs/features/workflow-builder.md`, `docs/how-to/using-the-workflow-builder.md`, `docs/how-to/configuring-external-editor-for-workflow-builder.md`, `docs/adr/20260424120000-workflow-builder-save-durability.md`, `docs/coding-standards/file-writes.md`, plus the five code-package docs. This evidence pair shows the documentation obligation surface is satisfied textually — whether the docs accurately describe runtime behavior is out of scope for doc-integrity tests.
- **Desired State:** Plan D-32, D-38.

### 12. Bundle integration — the default workflow

**GAP-044: `bundle_builder_integration_test.go` smoke-tests loading through `workflowio` but not through the end-to-end builder**
- **Category:** Partial
- **Feature/Behavior:** Plan Definition of Done line 350: "`bundle_integration_test.go` smoke passes: default bundle loads, validates, and saves via the builder without fatal findings." Plan line 303: "new file… smoke-tests that the bundled default workflow opens cleanly in the builder."
- **Current State:** `src/cmd/pr9k/bundle_builder_integration_test.go` exists and exercises `workflowio.Load` + `workflowvalidate.Validate` against the shipped bundle, but does not drive the `workflowedit.Model` (which cannot be driven end-to-end because the subcommand does not launch it). The "loads cleanly in the builder" claim is only indirectly verified.
- **Desired State:** Plan WU-12 bundle-builder integration test.

### 13. Miscellaneous

**GAP-045: Quit confirmation dialog does not support the spec's spatial order or Cancel default precisely**
- **Category:** Partial
- **Feature/Behavior:** Spec §"Quit confirmation (no unsaved changes)" and D73: `Quit the workflow builder? (Yes / No)`, `No` default, `Enter` cancels, `Esc` equivalent to No.
- **Current State:** `updateDialogQuitConfirm` (`model.go:428-441`) treats `Enter` as Yes (quits) and `Esc` as No (cancels) — the opposite of the spec's `No` default. Spec says "pressing `Enter` cancels the quit."
- **Desired State:** Spec D73 / Dialog Conventions "the safe option is the keyboard default."

**GAP-046: Unsaved-changes dialog has no spatial Save/Cancel/Discard rendering**
- **Category:** Partial
- **Feature/Behavior:** Spec §"Unsaved-changes interception" and Dialog Conventions: three options in spatial order Save / Cancel / Discard, Cancel is keyboard default, `Enter` activates Cancel, `Esc` equivalent to Cancel.
- **Current State:** `renderDialog` for `DialogUnsavedChanges` returns one line `"Unsaved changes: Save / Cancel / Discard"` (`model.go:171-172`). `updateDialogUnsavedChanges` (`model.go:443-469`) treats only `s`/`d`/`c`/`Esc`; there is no visible keyboard cursor / spatial default / Enter-on-Cancel behavior.
- **Desired State:** Spec Dialog Conventions section.

**GAP-047: Reorder mode entered via `r` conflicts with detail-pane `r` secret-reveal when focus moves mid-edit**
- **Category:** Partial
- **Feature/Behavior:** Spec D47 `r` is "secret-reveal keyboard binding" only in detail pane; D34 `r` is "enter-reorder-mode" only in outline.
- **Current State:** The two bindings are correctly scoped to `handleOutlineKey` and `handleDetailKey` respectively. However, there is no footer-shortcut disclosure of "`r` reveal" or "`r` reorder" when focus is ambiguous; the outline footer advertises `r` reorder at all times and the detail footer advertises `r` reveal at all times — but neither is context-sensitive to whether the focused row actually supports the action. Low-confidence finding.
- **Desired State:** Spec D24 shortcut-footer requirement "updated on focus change."

**GAP-048: `statusLine.RefreshIntervalSeconds` round-trip — zero means "default"; marshal uses `omitempty`**
- **Category:** Partial
- **Feature/Behavior:** Spec §"User Interactions" statusLine summary; validator Category 11 treats 0 as "use default."
- **Current State:** `marshalDoc` writes `RefreshIntervalSeconds` with `,omitempty` (`src/internal/workflowio/marshal.go:29`), which elides any zero. But `rawStatusLine.RefreshIntervalSeconds` uses `*int` (`src/internal/workflowmodel/scaffold.go:61`), which distinguishes "not set" from zero. On save the distinction is lost; a workflow that explicitly sets 0 cannot round-trip.
- **Desired State:** Spec D18 "unknown-field warn-and-drop" does not authorize known-field data loss; config.json schema authority.

**GAP-049: No shutdown / cleanup of active goroutines if the Model is ever used**
- **Category:** Implicit
- **Feature/Behavior:** Plan §"Runtime Behavior" "Goroutine lifecycle" (D-33): every non-stdlib goroutine receives `ctx` and selects on `ctx.Done()`.
- **Current State:** `workflowedit.Model` spawns no goroutines itself (the `tea.Cmd` closures in `makeValidateCmd`/`makeSaveCmd`/`makeLoadCmd`/`completePath` are run by the Bubble Tea runtime, not by the Model). They do not take `ctx` as plan D-33 requires, so long-running scans in `scanMatches` cannot be cancelled. The plan expects context-aware cancellation; the implementation uses fire-and-forget closures. Whether this is a gap depends on whether `context.Context` was meant to cover bubbletea-owned goroutines.
- **Desired State:** Plan D-33.

**GAP-050: `bundle_integration_test.go` (preexisting) vs plan's claim that builder smoke test is a new file**
- **Category:** Partial
- **Feature/Behavior:** Plan line 302: "the existing `bundle_integration_test.go` is unchanged"; plan line 303: "`bundle_builder_integration_test.go` — new file (separate from the existing bundle-layout `bundle_integration_test.go`)."
- **Current State:** Both files exist: `src/cmd/pr9k/bundle_integration_test.go` (pre-existing) and `src/cmd/pr9k/bundle_builder_integration_test.go` (new, dated Apr 24). However, the new file does not "smoke-test that the bundled default workflow opens cleanly in the builder" — it tests only `workflowio.Load` and `workflowvalidate.Validate` on the bundle, with no Model driving.
- **Desired State:** Plan line 303.

**GAP-051: Rename-guard tests assert literal `"workflow"` string appears in `workflow.go` but not that the TUI is wired**
- **Category:** Partial
- **Feature/Behavior:** Plan line 304: "`src/cmd/pr9k/rename_guard_test.go` — extended to assert `workflow-` filename prefix and `pr9k workflow` command name do not collide."
- **Current State:** The guard tests (`rename_guard_test.go:282-326`) assert `NewLoggerWithPrefix` and the literal `"workflow"` string appear in `workflow.go`. They do not assert the command is un-hidden or that a `tea.NewProgram` call exists. The F-116 guard therefore passes against the current stub implementation.
- **Desired State:** Plan F-116.

### 14. Out-of-the-box observations (not gaps relative to plan — included for completeness)

**GAP-052: Version has not been bumped for the feature**
- **Category:** Implicit
- **Feature/Behavior:** Plan Definition of Done line 354: "Version bump lands as the first commit of the feature PR."
- **Current State:** `src/internal/version/version.go:7` is at `0.7.2`. Plan OI-3 acknowledges the version-bump logistics are user-choice; the current version is unchanged from main. Without knowing whether this PR is pre-merge this cannot be classified as a gap.
- **Desired State:** Plan D-18 / OI-3.

## Areas Needing Separate Analysis

- **Validator semantics drift.** The validator changes for OI-1 hardening appear in place (`ValidateDoc`, `safePromptPath` EvalSymlinks + containment, `validateCommandPath`). A focused pass should confirm the full plan-required test matrix exists and the `TestValidate_ProductionStepsJSON` assertion that production config still passes unchanged. Out of scope here because the validator is not the primary gap surface.
- **`workflowedit.Model.Update` keyboard coverage.** The plan commits to 28 TUI mode-coverage entries; the current `model_test.go` (25,479 bytes) appears to cover a subset but a systematic mode-by-mode diff needs dedicated analysis.
- **Dialog rendering fidelity.** Every dialog kind currently renders a single-line placeholder. A separate UX-level analysis should catalog the full-fidelity rendering each dialog needs (multi-line bodies, option cursors, help text, focus-indicator glyphs) and produce a side-by-side spec-versus-impl diff.
- **Editor runner tests.** `editor_test.go` covers resolver cases; a separate pass should check that `ExecCallback` branches (F-107) are exercised end-to-end through a fake `tea.Program`, not just unit-tested in isolation.
- **Atomicwrite + crashtemp interplay.** `atomicwrite.Write` emits temp files whose names `crashtemp.DetectCrashTempFiles` globs. A focused pass should verify the two naming schemes match exactly and round-trip through `parsePIDFromTempName`.
- **Doc-integrity completeness.** DI-1 through DI-8 are all referenced in `doc_integrity_test.go`. A separate audit should confirm each DI test's asserted claim matches the plan's D-32 enumeration.

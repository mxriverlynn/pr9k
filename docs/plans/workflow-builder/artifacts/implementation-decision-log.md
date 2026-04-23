# Implementation Decision Log: Workflow Builder

<!--
This file records every implementation decision committed while planning the
`pr9k workflow` subcommand. Behavioral and implementation statements live in
[../feature-implementation-plan.md](../feature-implementation-plan.md). Round-
by-round history lives in [implementation-iteration-history.md](implementation-iteration-history.md).

Cross-referencing invariants:
- `Driven by rounds:` — R# IDs from [implementation-iteration-history.md](implementation-iteration-history.md).
- `Dependent decisions:` — D# IDs of later decisions that rest on this one.
- `Referenced in plan:` — sections of [../feature-implementation-plan.md](../feature-implementation-plan.md)
  that cite this decision with an inline parenthetical link.

Any time a decision is added or edited in this file, update the matching entries
in implementation-iteration-history.md and ../feature-implementation-plan.md so
the three files stay in sync.
-->

## D-1: Package decomposition — five new `src/internal/` packages plus one cmd file

- **Question:** How is the builder's code organized across Go packages?
- **Decision:** Ship six new compilation units: `cmd/pr9k/workflow.go` (cobra subcommand + DI composition root), plus five new `src/internal/` packages: `workflowedit` (Bubble Tea model), `workflowmodel` (mutable in-memory `WorkflowDoc`), `workflowio` (load / save / detect), `workflowvalidate` (bridge to validator), and `atomicwrite` (shared helper). `workflowmodel` has no dependencies on any other new package; `workflowedit` depends on `workflowmodel`, `workflowvalidate`, and `workflowio` (via interfaces); `workflowio` depends on `workflowmodel` and `atomicwrite`; `workflowvalidate` depends on `workflowmodel` and `internal/validator`.
- **Rationale:** Five distinct reasons-to-change (schema field → `workflowmodel`; save durability → `workflowio`; validator rule → `workflowvalidate` / `internal/validator`; dialog or widget → `workflowedit`; CLI flag → `cmd/pr9k/workflow.go`) map to five distinct modules (SRP). The existing codebase refuses to couple `internal/ui` to `internal/validator` directly — `main.go` wires them — and the builder follows the same pattern. All new packages live under `src/internal/` which the existing `Makefile:18` `GOFMT_PATHS` already covers.
- **Evidence:** Architect findings R1 and amended in Round 2 (see `/tmp/wb-arch-findings.md` section R1 and `/tmp/wb-arch-round2.md` section 3). DevOps DOR-010 (GOFMT coverage). Existing precedent: `internal/ui` never imports `internal/validator`; `internal/statusline` is a sibling peer of `internal/ui`.
- **Rejected alternatives:**
  - Single `internal/workflowbuilder` package — rejected because greenfield single-package builders accumulate all change pressures in one file (architect R1); test coverage for the I/O layer becomes entangled with TUI state.
  - Place atomic-save helper inside `internal/logger` — rejected; the logger is a domain-specific append-only writer with its own lifecycle and should not carry a general-purpose durability primitive (architect R4; DevOps DOR-001).
  - A seventh package `internal/ansi` for `stripANSI` — rejected in Round 2: `internal/statusline.Sanitize` already exists and is exported; promoting a new package with one caller is the anti-pattern the architecture process rejects. Revisit only when a second caller exists (architect Round 2 §3.2).
- **Specialist owner:** `software-architect`
- **Revisit criterion:** Reopen if implementation discovers that a builder-owned symbol needs to be imported from two of `workflowedit`, `workflowio`, `workflowvalidate` simultaneously (suggests the split is wrong); or if a second consumer of `atomicwrite` surfaces a shape that demands a different API.
- **Dissent (if any):** —
- **Driven by rounds:** R1, R2
- **Dependent decisions:** D-2, D-3, D-4, D-5, D-6, D-7, D-8, D-9
- **Referenced in plan:** Implementation Approach — Architecture and Integration Points; Decomposition and Sequencing (WU-1 through WU-11)

## D-2: In-memory workflow model lives in `workflowmodel.WorkflowDoc`, distinct from `vFile` and `steps.StepFile`

- **Question:** Does the builder reuse the validator's `vFile` struct (or `steps.StepFile`) for its in-memory model, or define its own type?
- **Decision:** `workflowmodel.WorkflowDoc` is a new mutable struct with value-typed fields (`IsClaude bool`, not `*bool`). Presence-sensitive fields track their presence with explicit companion booleans where the validator needs the distinction (for example, `Step.IsClaudeSet bool` — required because the validator treats absent `isClaude` as fatal but the builder must allow a partially-entered step). The builder serializes `WorkflowDoc` to JSON via its own marshaler in `workflowio`; it does not share `vFile`.
- **Rationale:** The validator's `vFile` / `vStep` use pointer-indirected fields to distinguish absent-from-explicit-false for strict JSON validation. The runtime's `steps.StepFile` uses value types because runtime has no need for the absent-vs-present distinction. The builder needs a third view shaped for mutation. Sharing `vFile` would require exporting it (breaking encapsulation) or placing builder code in the `validator` package (violating SRP). Three views of the same schema for three different consumers is the correct decomposition; the precedent (`vFile` + `steps.StepFile` already existing side by side) validates it.
- **Evidence:** Architect R2 (`/tmp/wb-arch-findings.md`). Validator source at `src/internal/validator/validator.go:78-114` (`vFile`/`vStep`). Existing `src/internal/steps/steps.go` (`StepFile`/`Step` value-typed).
- **Rejected alternatives:**
  - Reuse `vFile` — rejected because adding editing-convenience fields would force the validator's private struct to change (OCP violation).
  - Reuse `steps.StepFile` — rejected because it collapses mutable-editing semantics into the runtime-execution view; `CaptureAs *string` in `vStep` vs `CaptureAs string` in `steps.Step` already signals these are intentionally different types.
- **Specialist owner:** `software-architect`
- **Revisit criterion:** Reopen if the builder needs a fourth view for an unforeseen consumer (e.g., a diff tool that compares two workflows); at that point consider extracting a shared core.
- **Dissent (if any):** —
- **Driven by rounds:** R1
- **Dependent decisions:** D-3, D-5, D-6
- **Referenced in plan:** Implementation Approach — Data Model and Persistence

## D-3: Validator extension — add `ValidateDoc(doc, workflowDir, companionFiles)` alongside the existing `Validate(workflowDir)`

- **Question:** How does the builder feed its in-memory state to the validator without writing a temp file first (T3 contract)?
- **Decision:** Add a new exported function to `internal/validator`: `func ValidateDoc(doc workflowmodel.WorkflowDoc, workflowDir string, companionFiles map[string][]byte) []Error`. The `companionFiles` map keys are `promptFile` values from the step (e.g., `"step-1.md"`, not the full `"prompts/step-1.md"` path); when a key is present, `ValidateDoc` uses the in-memory bytes for existence checks and token scanning instead of calling `os.Stat` or `os.ReadFile`. Two new private helpers (`readCompanionOrDisk`, `statCompanionOrDisk`) centralize the nil-check. `Validate(workflowDir)` is preserved unchanged and internally calls `ValidateDoc(doc, workflowDir, nil)` — passing `nil` means "use disk for everything," preserving behavior exactly. `safePromptPath` (disk-free) and `validateCommandPath` (scripts must always exist on disk; builder may edit them but they still run from disk at workflow time) are untouched.
- **Rationale:** Approach (a) from T3. The architect's Round 2 validator code audit confirmed only five functions in `validator.go` take `workflowDir`; only four disk-touching operations sit inside `validatePhase`, and all are interceptable by the `companionFiles` map. Approach (b) (scratch directory) is retained as a named fallback only — it incurs per-save I/O proportional to companion-file size, complicates cleanup (crash residue), and raises new security surface (world-readable scratch files on shared hosts, per security NEW-2). Approach (a) eliminates all three concerns at bounded cost (two new helpers, one new signature parameter on two private functions).
- **Evidence:** T3 (`feature-technical-notes.md`). Architect Round 2 §1.2, §1.3, §2. Validator audit at `src/internal/validator/validator.go:154` (`Validate` entry), `:691-705` (`safePromptPath`), `:641` (`validateCommandPath`). Production integration test `TestValidate_ProductionStepsJSON` unaffected (architect Round 2 §1.4).
- **Rejected alternatives:**
  - Approach (b) — scratch directory + existing `Validate` — rejected as primary; retained as fallback. Security NEW-2 raised world-readable file exposure on shared hosts; JrF-14 raised collision with D42-a crash-era scan; architect Round 2 eliminates both by choosing approach (a).
  - Inline the validator logic inside `workflowvalidate` — rejected; duplicates D13 validation rules and violates narrow-reading (schema knowledge lives in `internal/validator`).
- **Specialist owner:** `software-architect`
- **Revisit criterion:** Reopen only if implementation discovers additional disk-touching helpers added to the validator after this audit (Round 2) that cannot accept the `companionFiles` map cleanly; in that case fall back to approach (b) and treat this decision as superseded.
- **Dissent (if any):** —
- **Driven by rounds:** R1, R2
- **Dependent decisions:** D-4, D-5, D-13
- **Referenced in plan:** Implementation Approach — External Interfaces; Runtime Behavior (save path); Testing Strategy (T3 fixtures)

## D-4: `workflowvalidate` bridge is a thin type-conversion layer

- **Question:** What does the new `internal/workflowvalidate` package contain?
- **Decision:** One file, `validate.go`, with one exported entry: a function (not an interface) that converts `workflowmodel.WorkflowDoc` to the shape `validator.ValidateDoc` needs, performs no validation logic itself, and delegates. The bridge exists to keep `workflowedit` from importing `internal/validator` directly — preserving the existing convention that the TUI package does not depend on the validator package.
- **Rationale:** DIP. `workflowedit` depends on an abstraction (the bridge's function signature), not on the validator package's internal types. The validator may later change its internal representation; `workflowedit` is unaffected. Per `internal/ui` / `internal/validator` precedent (main.go wires them; `internal/ui` does not import `internal/validator`), the builder follows the same layering.
- **Evidence:** Architect R1, R3. Existing separation between `internal/ui` and `internal/validator`.
- **Rejected alternatives:**
  - Expose an interface for the validator — rejected as premature abstraction; one implementation, no churn, no behavioral autonomy (the "one-implementation interface" anti-pattern from architect R8).
  - Call `validator.ValidateDoc` directly from `workflowedit` — rejected; couples TUI package to validator internal type conversion logic.
- **Specialist owner:** `software-architect`
- **Revisit criterion:** Reopen if a second consumer of the bridge emerges (e.g., a CLI validation subcommand) — at that point consider whether the bridge is still the right shape or should become a method on the model.
- **Dissent (if any):** —
- **Driven by rounds:** R1
- **Dependent decisions:** —
- **Referenced in plan:** Implementation Approach — External Interfaces

## D-5: Atomic-save helper lives in a new `internal/atomicwrite` package with signature `Write(path, data, mode) error`

- **Question:** What is the canonical API and location for the durable write helper T1 requires?
- **Decision:** New package `internal/atomicwrite`. Single exported function: `func Write(path string, data []byte, mode os.FileMode) error`. The helper: (1) resolves `path` via `filepath.EvalSymlinks` at save time so the temp file lands in the same directory as the symlink's real target (T1 correction); (2) creates the temp file with `os.CreateTemp(realDir, "*"+TempFileSuffix)` — `os.CreateTemp` internally uses `O_RDWR|O_CREATE|O_EXCL` and mode `0o600`; (3) writes `data`; (4) calls `file.Sync()` (maps to `fsync(2)`) before close; (5) calls `os.Rename(tempPath, realPath)`; (6) on any error, removes the temp file via `os.Remove` before returning. Exported constant `TempFileSuffix = ".pr9k-wb-<pid>-<epoch-ns>.tmp"` (actual pattern rendered per-call) enables `workflowio` to scan for crash-era leftovers.
- **Rationale:** SRP — one package, one reason to change (atomic-write strategy). High cohesion — the symlink-resolution correction (T1) and the `O_EXCL`+`0o600` commitment (security Finding 4) live in the one place every caller inherits. Placing this in `internal/logger` would couple the logger to an unrelated concern. The function signature matches DevOps OQ1's candidate verbatim. The mode parameter exists so callers can distinguish config files (0o644 by convention) from prompt files; the temp file itself is always 0o600 regardless of the caller's target mode.
- **Evidence:** Architect R4 and Round 2 §3.1. DevOps DOR-001, DOR-002, DOR-004, OQ1. Security Finding 4. T1 (feature-technical-notes.md). Go `os.CreateTemp` stdlib source (documents `O_EXCL` + `0o600`).
- **Rejected alternatives:**
  - Extend `internal/logger` — rejected (DOR-001, architect R4). Two unrelated reasons to change.
  - Inline in `workflowio` as private helper — rejected; the coding standard D59 commits to a canonical API that future callers can reuse, which requires a shared package.
  - Use `os.OpenFile(..., O_CREATE|O_EXCL|O_WRONLY, 0o600)` directly instead of `os.CreateTemp` — acceptable but `os.CreateTemp` gives the same guarantees plus a collision-resistant random suffix managed by stdlib.
- **Specialist owner:** `software-architect` (package author) / `devops-engineer` (operational verification)
- **Revisit criterion:** Reopen if a caller needs to write across filesystems (at which point the same-directory temp-file guarantee breaks and EXDEV surfaces — see D-19); or if `os.CreateTemp` semantics change (version-pin verified on Go 1.26.2).
- **Dissent (if any):** —
- **Driven by rounds:** R1, R2
- **Dependent decisions:** D-18, D-19, D-20
- **Referenced in plan:** Implementation Approach — Data Model and Persistence; Security Posture; Decomposition and Sequencing (WU-1)

## D-6: `EditorRunner` is a one-method interface; editor resolution is private to the production impl

- **Question:** What is the shape of the DI seam for `tea.ExecProcess`?
- **Decision:** In `internal/workflowedit/editor.go`:
  ```go
  type EditorRunner interface {
      Run(filePath string, exitCallback func(err error)) tea.Cmd
  }
  ```
  Production implementation in `cmd/pr9k/workflow.go` is `realEditorRunner` with a private package-level function `resolveEditor() (bin string, args []string, err error)` that handles `$VISUAL`/`$EDITOR` lookup, metacharacter rejection (D33), and `exec.LookPath`. `Run` calls `resolveEditor()` internally; on resolution error, returns a `tea.Cmd` that immediately delivers the error via `exitCallback`; on resolution success, returns `tea.ExecProcess(cmd, func(err) tea.Msg { exitCallback(err); return editorExitMsg{err} })`. Test double substitutes a `tea.Cmd` that writes known bytes to `filePath` and returns a no-op message without touching any TTY.
- **Rationale:** ISP — one method is sufficient. A separate `Resolve()` method would fragment the contract (tests could resolve without running; production would always do both). Resolution errors and editor exit errors both flow through the same `exitCallback`, so `workflowedit.Model` handles them identically. `resolveEditor` is a package-level function (not a method on `realEditorRunner`) so unit tests in `cmd/pr9k/` can exercise it directly without instantiating the interface.
- **Evidence:** Architect R5 and Round 2 §4. Security Finding 7 (SIGINT branching — see D-7). T2 (feature-technical-notes.md). Test plan `wfeditor_test.go` matrix.
- **Rejected alternatives:**
  - Two-method interface (`Resolve` + `Run`) — rejected in Round 2; resolution internal to production impl is cleaner and doesn't expose test doubles to D33 details they don't need to simulate.
  - Function-typed DI seam (`editorRun func(...) tea.Cmd`) — rejected; interface gives a named type for logging and introspection and matches the `sandboxCreateDeps.dockerRun` precedent's shape (though that uses a function type, the method form is preferred here because `tea.ExecProcess` takes a callback, which is itself a function, making a function-of-function-of-function harder to read).
- **Specialist owner:** `software-architect`
- **Revisit criterion:** Reopen if the interface accumulates a second method (suggests the one-method design is wrong); or if the test double shape needs to distinguish "editor resolution failed" from "editor ran and exited with error" externally (at that point the interface might need a typed error, not just `error`).
- **Dissent (if any):** —
- **Driven by rounds:** R1, R2
- **Dependent decisions:** D-7, D-26
- **Referenced in plan:** Implementation Approach — External Interfaces; Runtime Behavior (external editor handoff); Testing Strategy (T2)

## D-7: `ExecCallback` branches on exit code 130 — SIGINT routes to quit-confirm, not re-read

- **Question:** When the external editor exits due to SIGINT (Ctrl+C), does the builder re-read the file and return to edit view (the normal path) or enter the quit-confirm flow?
- **Decision:** Inside `realEditorRunner.Run`'s `ExecCallback`, inspect the child exit code. Exit code 130 (SIGINT on POSIX) or an `*exec.ExitError` whose `ExitCode()` returns 130 routes to a new message type `editorSigintMsg{}`; `workflowedit.Model.Update` on receipt transitions to the quit-confirm dialog (the unsaved-changes three-option dialog if the in-memory state is dirty, otherwise the two-option quit-confirm). All other exit codes (zero and non-zero non-130) route to the normal `editorExitMsg{err, exitCode}` which triggers the re-read + edit-view-return path.
- **Rationale:** Without this branching, Ctrl+C during editor invocation produces a spurious unsaved-changes dialog (because the re-read updates the in-memory state; the in-memory state is now dirty; the subsequent quit signal races against the file-re-read message). Security Finding 7 traces the exact race. The spec's Edge Cases table already commits to the behavior ("enters the normal SIGINT/quit flow"); this decision names the implementation mechanism.
- **Evidence:** Security Finding 7 (`/tmp/wb-sec-findings.md`). Concurrency C3. Test plan `TestEditorRunner_SIGINTDuringReleasedWindow_QuitFlowEntered`. Spec Edge Cases table (SIGINT during released window row).
- **Rejected alternatives:**
  - No branching; treat all editor exits uniformly — rejected (security Finding 7); produces the spurious unsaved-dialog race.
  - Track SIGINT via an `os/signal` handler separate from `ExecCallback` — rejected; the signal is delivered to the child process group, not to the builder directly, so inspecting the child's exit code is the only reliable signal.
- **Specialist owner:** `adversarial-security-analyst`
- **Revisit criterion:** Reopen if a platform other than POSIX (e.g., Windows, if support is ever added) uses a different SIGINT exit-code convention.
- **Dissent (if any):** —
- **Driven by rounds:** R1, R2
- **Dependent decisions:** —
- **Referenced in plan:** Runtime Behavior (external editor handoff); Security Posture

## D-8: Dialog composition uses one `dialogState` slot + `helpOpen bool` + depth-1 `prevFocus`

- **Question:** How are overlapping overlays (findings panel, help modal, dialogs, menu) composed in the Bubble Tea model without breaking focus restoration?
- **Decision:** In `internal/workflowedit`:
  ```go
  type DialogKind int
  const (
      DialogNone DialogKind = iota
      DialogPathPicker
      DialogNewChoice
      DialogUnsavedChanges
      DialogQuitConfirm
      DialogExternalEditorOpening
      DialogFindingsPanel
      DialogError
      DialogCrashTempNotice
      DialogFirstSaveConfirm   // external-workflow or symlink first-save
      DialogRemoveConfirm      // Del / trash-glyph confirmation per D48
      DialogCopyBrokenRef      // D61 broken-default dialog
      DialogFileConflict       // D41 mtime-mismatch overwrite/reload/cancel
      DialogEditorError        // D16 not-configured / D33 rejected-value
  )
  type dialogState struct { kind DialogKind; payload interface{} }
  type Model struct {
      dialog     dialogState   // at most one active dialog
      helpOpen   bool          // second-layer overlay; only legal when dialog.kind==DialogFindingsPanel
      prevFocus  focusTarget   // depth-1 restoration address (set when dialog or panel opens)
      // ...
  }
  ```
  Modal exclusion: at most one `dialogState` is active; opening a second dialog replaces the first (the UX commitment from R2-C5 and D48 — Escape pops one layer at a time, not a stack of dialogs). The findings panel is the **only** dialog kind over which the help modal may open; `helpOpen` flips `true` only when `m.dialog.kind == DialogFindingsPanel` and `?` is pressed, and is silently suppressed for all other dialog kinds (R2-C1). `prevFocus` is captured when the first overlay opens and cleared when the last overlay closes.
- **Rationale:** R6 modal-exclusion is simpler than a stack and correct for every coexistence the spec documents. The depth-1 `prevFocus` covers all three scenarios UX traced in Round 2 (primary coexistence, empty-editor, path-picker). A full focus-stack adds complexity for a coexistence case that does not exist. The enum is closed-set but OCP-friendly: adding a new dialog adds a constant and a handler; existing handlers are untouched.
- **Evidence:** Architect R6. UX Round 2 question-1 trace (`/tmp/wb-ux-round2.md`). UX IP-001, IP-002. Spec Primary Flow step 8 (help over findings). Spec D48 (dialog conventions).
- **Rejected alternatives:**
  - Focus stack (slice) — rejected (UX Round 2); the spec documents exactly one coexistence case (help over findings), which a stack over-engineers.
  - Mode-per-dialog on a single enum — rejected (architect R6); fat Mode enum recapitulates the existing TUI's `updateShortcutLineLocked` switch smell at greater scale.
  - Dialog queue (opening a dialog over a dialog queues the second) — rejected; unsupported by the spec's flow diagrams and not requested by any specialist.
- **Specialist owner:** `software-architect` + `user-experience-designer`
- **Revisit criterion:** Reopen if the spec adds a second coexistence case where two non-findings-panel surfaces must be simultaneously visible; or if `prevFocus` is ever overwritten before being consumed (suggests depth > 1 needed).
- **Dissent (if any):** —
- **Driven by rounds:** R1, R2
- **Dependent decisions:** D-9, D-10, D-14, D-21
- **Referenced in plan:** Implementation Approach — Runtime Behavior (dialog state machine); Decomposition and Sequencing (WU-7)

## D-9: Update routing — overlay-first ordering with global-key intercept nested inside

- **Question:** In what order does `Update` dispatch `tea.KeyMsg` between overlays, the global-key intercept (F10, Ctrl+N/O/S/Q), and widgets?
- **Decision:** The exact ordering committed in `workflowedit.Model.Update`:
  ```
  (1) helpOpen check      → updateHelpModal(msg)
  (2) dialog.kind check   → updateDialog(msg)
  (3) isGlobalKey check   → handleGlobalKey(msg)
  (4) fall-through        → updateEditView(msg) (outline / detail / menu / empty-editor per state)
  ```
  Global shortcuts (F10, Ctrl+N, Ctrl+O, Ctrl+S, Ctrl+Q) are **not** the outermost guard. When any overlay (help modal or any `dialog.kind != DialogNone`) is active, global shortcuts are either consumed by the overlay's handler or silently suppressed. Specifically: Ctrl+S while a D48 confirmation dialog (e.g., remove-step confirm) is active is silently suppressed; it is not treated as a save trigger.
- **Rationale:** The UX Round 2 trace showed the "global keys first" ordering (the original IP-007 remediation phrasing) silently destroys in-progress path-picker input when Ctrl+N is pressed mid-dialog and dismisses quit-confirm when F10 is pressed. Overlay-first ordering respects the modal dialog convention (D48) that the user's current overlay controls the shortcut space.
- **Evidence:** UX Round 2 question-3 (`/tmp/wb-ux-round2.md` R2-C5). UX IP-007. Spec D48 dialog conventions.
- **Rejected alternatives:**
  - Global-key first — rejected (UX Round 2); produces the destructive side effects enumerated above.
  - Widget-by-widget intercept — rejected (UX IP-007); each widget would duplicate the check and at least one would miss it.
- **Specialist owner:** `user-experience-designer`
- **Revisit criterion:** Reopen if a new dialog type needs to escape the suppression (e.g., a transient progress dialog that must not block Ctrl+Q) — in that case the dialog's `updateDialog` handler opts in to forwarding specific keys.
- **Dissent (if any):** —
- **Driven by rounds:** R1, R2
- **Dependent decisions:** —
- **Referenced in plan:** Runtime Behavior (key dispatch)

## D-10: `Del` key scoped to outline-focus; not a global key

- **Question:** How does `Del` scope to step removal without destroying in-progress text in the detail pane?
- **Decision:** `Del` is **not** in the global-key intercept set. When focus is on an outline row (the active focus target is the outline and the row is a step), `Del` opens the D48 remove-step confirmation dialog (`Delete step "<name>"? (Delete / Cancel)`). When focus is anywhere else — any detail-pane text field, any choice list, the path picker, a dialog — `Del` is forwarded to the focused widget as forward-delete.
- **Rationale:** The global-key intercept is reserved for shortcuts that have the same meaning everywhere (menu, new, open, save, quit). `Del`'s meaning is context-dependent: destructive in the outline, text-editing in a field. Without this scoping, `Del` pressed while mid-edit destroys a step. UX Round 2 names this as the highest-urgency commitment (R2-C2).
- **Evidence:** UX Round 2 R2-C2 (`/tmp/wb-ux-round2.md`). OQ-3 Round 1 resolution (shortcut footer shows `Del  remove` when outline row focused).
- **Rejected alternatives:**
  - Put `Del` in the global-key intercept set — rejected (UX R2-C2); makes text-field forward-delete impossible.
  - Require `Shift+Del` or `Ctrl+Del` for removal — rejected; footer-driven discoverability (`Del  remove`) already sufficient, and adding a modifier fragments keyboard muscle memory.
- **Specialist owner:** `user-experience-designer`
- **Revisit criterion:** Reopen if user testing surfaces a confusion pattern around `Del` at the outline/detail-pane boundary.
- **Dissent (if any):** —
- **Driven by rounds:** R2
- **Dependent decisions:** —
- **Referenced in plan:** Runtime Behavior (key dispatch)

## D-11: Widget-owned shortcut-footer content via `ShortcutLine() string`

- **Question:** How does the shortcut footer stay correct across nine focus-context states without a growing `switch mode` statement?
- **Decision:** Every focusable widget type in `workflowedit` (outline row types, detail-pane field types, path-picker text input, dialog kinds) exposes a `ShortcutLine() string` method. The root `View()` queries the currently-focused widget and renders its return value in the footer slot. Focus-change events do not send a separate `FocusChangedMsg` to update a central shortcut string; rendering derives the footer from the focus target at render time.
- **Rationale:** Decouples footer content from the Mode enum and eliminates the `updateShortcutLineLocked` switch (UX IP-005). The render-time query keeps the footer in sync automatically; there is no "forgot to send FocusChangedMsg" failure mode. Per-widget ownership follows SRP — each widget knows its own shortcuts.
- **Evidence:** UX IP-003, IP-005 (`/tmp/wb-ux-findings.md`). Existing `src/internal/ui/ui.go:205-232` (`updateShortcutLineLocked` 8-case switch — the pattern being avoided).
- **Rejected alternatives:**
  - Static per-mode constants — rejected (UX IP-003); grows combinatorially with focus states.
  - Central `FocusChangedMsg` that writes a cached string — rejected; requires every navigation key to remember to send the message; one missed send produces a stale footer.
- **Specialist owner:** `user-experience-designer`
- **Revisit criterion:** Reopen if rendering-time computation surfaces a measurable perf issue (unlikely at TUI scale).
- **Dissent (if any):** —
- **Driven by rounds:** R1
- **Dependent decisions:** —
- **Referenced in plan:** Runtime Behavior (rendering)

## D-12: Two independently scrollable viewports with pointer-side mouse routing

- **Question:** How are the outline (left) and detail pane (right) sized and mouse-routed?
- **Decision:** Two `bubbles/viewport` instances: `outlineViewport` (fixed width of 40 columns for terminals ≥80 cols, proportional 40% for narrower terminals, minimum 20 cols; auto-height from chrome calculation) and `detailViewport` (remaining width, same height). Wheel events inspect `msg.X`: if `X < outlineRightEdge` → outline viewport; otherwise → detail viewport. Focus-change within the detail pane triggers a viewport auto-scroll: on receipt of a `FocusChangedMsg` carrying the newly focused field's line index, `detailViewport.SetYOffset(...)` is called before the next render.
- **Rationale:** D29, D52 are committed spec decisions. Two viewports is the only feasible shape for independent scroll. The column-split rule (40 cols or 40%) mirrors common IDE layouts and gives the outline enough room for step names without crowding the detail pane.
- **Evidence:** UX IP-004 (`/tmp/wb-ux-findings.md`). Spec D29, D52. Existing `src/internal/ui/log_panel.go:39` (single-viewport precedent).
- **Rejected alternatives:**
  - Single viewport with mode switching — rejected; D52 commits to independent scroll.
  - 50/50 split — rejected; detail pane needs more room for field labels and hints.
- **Specialist owner:** `user-experience-designer`
- **Revisit criterion:** Reopen if user feedback shows 40-col outline is too wide or too narrow in common terminal widths.
- **Dissent (if any):** —
- **Driven by rounds:** R1
- **Dependent decisions:** —
- **Referenced in plan:** Runtime Behavior (rendering); Decomposition and Sequencing (WU-6)

## D-13: Save is async via `tea.Cmd`; save-in-progress flag + snapshot refresh on callback

- **Question:** How does the builder run validation and the atomic save off the Update goroutine without breaking the "exactly what will be written" contract (T3) or allowing double-save races?
- **Decision:** The save sequence on `Ctrl+S` / `File > Save` is:
  1. In `Update` (synchronous), check the D41 mtime+size snapshot against on-disk state — if it changed, open the conflict dialog (overwrite/reload/cancel) and return.
  2. In `Update` (synchronous), check `saveInProgress bool` — if true, silently consume the key (no second save starts).
  3. In `Update` (synchronous), call `workflowvalidate.ValidateDoc(deepCopy(m.doc), m.workflowDir, snapshotCompanions(m))`. Validation is synchronous because (per D-3) the in-memory variant makes no disk calls for companion files already in the map and `safePromptPath`'s containment check is disk-free. Script existence (`validateCommandPath`) does a bounded `os.Stat`, accepted as synchronous because stat is sub-millisecond on local filesystems.
  4. If fatals: open findings panel; return. If warnings/info only: open findings panel, user acknowledges, fall through.
  5. Set `saveInProgress = true`; return a `tea.Cmd` that runs the atomic save (`workflowio.Save` → `atomicwrite.Write`) on a goroutine.
  6. The goroutine completes and delivers a `saveCompleteMsg{result}` to `Update`; on receipt, `Update` (synchronous) refreshes the D41 snapshot via `os.Stat(resolvedRealPath)` and captures `ModTime()` + `Size()`, clears `saveInProgress = false`, and dispatches UI feedback (banner, indicator clear).
- **Rationale:** Validation is called synchronously with a deep copy of the `WorkflowDoc`, which guarantees the validated state is identical to the state the save goroutine writes — T3 contract satisfied. `saveInProgress` prevents a second `Ctrl+S` from dispatching a concurrent save (C2). Snapshot refresh in `Update` (not in the goroutine) avoids the data race on the snapshot field (C5). `time.Time` comparisons use `Equal()` on nanosecond-precision `ModTime()` values from `os.Stat`, not seconds-truncated (C8).
- **Evidence:** Concurrency C2, C4, C5, C8 (`/tmp/wb-concurrency-findings.md`). T3. Spec D41, D63. Architect Round 2 §1 (synchronous in-memory validator feasibility).
- **Rejected alternatives:**
  - Fully synchronous save on `Update` — rejected (concurrency standard); file I/O blocks the TUI render loop.
  - Fully async validation + save — rejected (C4); window between validate-dispatch and save-dispatch allows user keystrokes to mutate state.
  - Shared snapshot read/write across goroutines without a lock — rejected (C2, C5); data race, fails the race detector.
- **Specialist owner:** `concurrency-analyst` + `software-architect`
- **Revisit criterion:** Reopen if `validateCommandPath`'s synchronous `os.Stat` produces measurable TUI hang on slow filesystems (NFS, FUSE); at that point move validation entirely async with a deep-copy safeguard.
- **Dissent (if any):** —
- **Driven by rounds:** R1
- **Dependent decisions:** D-14
- **Referenced in plan:** Runtime Behavior (save path); Security Posture (snapshot precision)

## D-14: D41 mtime snapshot uses nanosecond-precision `time.Time` from post-rename `os.Stat`

- **Question:** What exactly does the D41 file-change snapshot store, and when is it refreshed?
- **Decision:** The snapshot stores `{ModTime time.Time, Size int64}` captured from `os.Stat(resolvedRealPath)` **after** `os.Rename` returns successfully. `time.Time` values are compared via `Equal()` (monotonic stripped via `.Round(0)` before comparison, per Go docs on cross-process time comparison). The snapshot is never derived from `time.Now()`; never truncated to Unix seconds; never captured from the temp file before rename.
- **Rationale:** APFS and ext4 store nanosecond-precision mtime; truncating to seconds silently misses sub-second external saves (C8). Using `time.Now()` on the builder side would drift from the actual filesystem mtime by up to a clock tick, producing false-positive conflict dialogs (C5).
- **Evidence:** Concurrency C5, C8. Spec D41.
- **Rejected alternatives:**
  - `time.Now()` as snapshot — rejected (C5).
  - Unix-second truncation — rejected (C8).
- **Specialist owner:** `concurrency-analyst`
- **Revisit criterion:** Reopen if a supported filesystem other than APFS/ext4 has different mtime semantics the team learns about.
- **Dissent (if any):** —
- **Driven by rounds:** R1
- **Dependent decisions:** —
- **Referenced in plan:** Runtime Behavior (save path)

## D-15: Log filename prefix `workflow-` (builder) vs. `ralph-` (run)

- **Question:** How do builder session logs and run logs coexist in the same `.pr9k/logs/` directory without collision?
- **Decision:** The builder creates its logger via a new constructor (`logger.NewLoggerWithPrefix(projectDir, "workflow")`) or parameterizes the existing `NewLogger` with a prefix argument; filenames become `workflow-YYYY-MM-DD-HHMMSS.mmm.log`. The `ralph-` prefix used by the main run loop is preserved. Two concurrent sessions — one `pr9k`, one `pr9k workflow` — produce files whose prefixes distinguish their origin.
- **Rationale:** Postmortem of a workflow-builder session requires identifying the right log file; time-window-only discrimination fails at scale (DOR-006). Distinct prefixes fix this at zero runtime cost.
- **Evidence:** DevOps DOR-006, OQ2 (`/tmp/wb-devops-findings.md`). Concurrency C6 (`/tmp/wb-concurrency-findings.md`). Existing `src/internal/logger/logger.go:33` hardcodes `ralph-` prefix.
- **Rejected alternatives:**
  - Shared `ralph-` prefix — rejected (DOR-006); indistinguishable from run logs.
  - `wb-` prefix — rejected; less discoverable than `workflow-`; matches the subcommand name.
- **Specialist owner:** `devops-engineer`
- **Revisit criterion:** Reopen if a third subcommand is added that also writes to `.pr9k/logs/`; naming convention may need a broader rule.
- **Dissent (if any):** —
- **Driven by rounds:** R1
- **Dependent decisions:** —
- **Referenced in plan:** Operational Readiness — Observability; Decomposition and Sequencing (WU-2)

## D-16: Temp-file naming `config.json.<pid>-<epoch-ns>.tmp` with PID-liveness rule

- **Question:** What is the temp-file naming convention for crash-era detection (D42-a)?
- **Decision:** `atomicwrite.Write` names the temp file `<targetBasename>.<pid>-<epoch-ns>.tmp` using the creator's PID and epoch nanoseconds. `workflowio.DetectCrashTempFiles(workflowDir)` globs `<workflowDir>/*.*.*.tmp` and filters to files whose name matches the builder pattern. For each match, the detector parses the `<pid>` token and calls `syscall.Kill(pid, 0)` (or equivalent OS-specific liveness check); a live PID means the file belongs to a running session (not crash-era) and is excluded. A dead or foreign PID classifies the file as crash-era, and the user is offered "delete / leave" per D42-a.
- **Rationale:** PID liveness discriminates "crashed" from "another running session," closing the DevOps DOR-004 trap where a concurrent session's active temp file would otherwise be misclassified and deleted. Including epoch nanoseconds in the name avoids collisions when the same PID creates multiple temps rapidly (unlikely in practice; defense-in-depth).
- **Evidence:** DevOps DOR-004, OQ3. Spec D42-a.
- **Rejected alternatives:**
  - Mtime-only classification — rejected (DOR-004); a slow NFS save looks like a crash.
  - `<basename>.<timestamp>.tmp` without PID — rejected; doesn't distinguish concurrent sessions.
- **Specialist owner:** `devops-engineer`
- **Revisit criterion:** Reopen if `syscall.Kill` with signal 0 becomes unavailable (non-POSIX port).
- **Dissent (if any):** —
- **Driven by rounds:** R1
- **Dependent decisions:** —
- **Referenced in plan:** Runtime Behavior (crash-era scan); Decomposition and Sequencing (WU-3)

## D-17: `docs/coding-standards/file-writes.md` scope — four rules

- **Question:** What exactly does the new coding-standards file (D59) contain?
- **Decision:** Four required sections:
  1. **Trigger rule:** atomic-rename required for any file that is the user's primary editing artifact (config.json, prompt files, script files). `O_TRUNC` is acceptable for ephemeral files re-created on every process start (streaming outputs, append-only logs) — name the existing exempt call sites explicitly.
  2. **Naming convention:** `<basename>.<pid>-<epoch-ns>.tmp` sibling of the target; points at D-16 for PID-liveness classification.
  3. **Cleanup responsibility:** the helper removes the temp file on rename failure; the caller is not responsible. On success, there is no temp file to clean up (the rename moved it).
  4. **EXDEV handling:** helper returns a wrapped error identifiable by `errors.Is(err, syscall.EXDEV)`; the caller surfaces the condition via the D56 four-element error template naming "target on different filesystem" as the `why`.
- **Rationale:** DevOps DOR-007 identified four ad-hoc decisions that had to be codified or two implementers would answer them differently. The coding standard pins the canonical answers so future contributors inherit them.
- **Evidence:** DevOps DOR-007. Spec D59. Architect R4.
- **Rejected alternatives:**
  - A larger scope including async-write patterns — rejected; outside the `atomicwrite` package's remit.
  - Smaller scope (trigger rule only) — rejected (DOR-007); leaves the other three decisions ad-hoc.
- **Specialist owner:** `devops-engineer`
- **Revisit criterion:** Reopen if a caller surfaces a fifth concern not covered by the four rules.
- **Dissent (if any):** —
- **Driven by rounds:** R1
- **Dependent decisions:** —
- **Referenced in plan:** Definition of Done; Decomposition and Sequencing (WU-1); Operational Readiness

## D-18: Version bump is the first commit of the feature PR, merged with `--no-ff`

- **Question:** How does the version bump satisfy versioning.md's "bump is its own commit" rule given the feature PR's size?
- **Decision:** The patch version bump (`version.Version`) is the **first commit** of the feature PR (the commit introducing the `workflow` subcommand, before the implementation lands in subsequent commits). The PR is merged with `--no-ff` (merge commit preserved) rather than squash-merged, so the version bump commit remains independently identifiable in git history. A one-line note in the PR description calls this out; the project's branch-protection rules already permit merge commits. Separately landing the version bump as a prior PR is also acceptable but not required.
- **Rationale:** DevOps DOR-005 identified the squash-merge gap between D37 and `docs/coding-standards/versioning.md`. The `--no-ff` path preserves the version-bump commit for downstream tooling auditing, while keeping the bump paired with the feature PR so reviewers see the whole change together.
- **Evidence:** DevOps DOR-005, OQ4. `docs/coding-standards/versioning.md:42`. Spec D37.
- **Rejected alternatives:**
  - Squash-merge bundling version bump with feature commits — rejected (DOR-005); violates the coding standard.
  - Separate PR always — acceptable but not required; `--no-ff` preserves audit trail equally well.
- **Specialist owner:** `devops-engineer`
- **Revisit criterion:** Reopen if the project adopts an automated changelog tool that requires squash-merge semantics.
- **Dissent (if any):** —
- **Driven by rounds:** R1
- **Dependent decisions:** —
- **Referenced in plan:** Operational Readiness — Rollout; Definition of Done

## D-19: EXDEV cross-device rename error surfaced via D56 template

- **Question:** What happens when `os.Rename` returns `EXDEV` during save?
- **Decision:** `atomicwrite.Write` wraps the EXDEV error (`err = fmt.Errorf("atomicwrite: rename cross-device: %w", err)`) so callers can detect via `errors.Is(err, syscall.EXDEV)`. `workflowio.Save` surfaces the condition via the D56 four-element template: **what happened** "Save failed — target is on a different filesystem than the builder's scratch directory"; **why** "POSIX atomic rename requires the temp file and target to share a filesystem"; **in-memory state commitment** "Your edits are still in memory. No changes were written to disk."; **action** "Retry / Cancel" (retry re-attempts the save after the user fixes the mount situation; Cancel returns to edit view).
- **Rationale:** Edge case PH-1 / AS-1 (P0) identified EXDEV as a third distinct OS-error class not enumerated in the spec's error taxonomy. In practice the helper always places the temp file in the target's real directory (post-EvalSymlinks), so EXDEV should never surface — but a refactor that moves the temp to `os.TempDir()` would cause silent failure without this named response.
- **Evidence:** Edge case PH-1, AS-1. Spec D56 error template. T1 "same directory" commitment.
- **Rejected alternatives:**
  - Opaque error passed through — rejected (PH-1); user sees a cryptic OS error and cannot self-diagnose.
  - Fallback copy-then-rename across filesystems — rejected; changes the durability guarantee and adds complexity.
- **Specialist owner:** `devops-engineer`
- **Revisit criterion:** Reopen if a user reports EXDEV in normal operation (would indicate the same-directory invariant is not holding).
- **Dissent (if any):** —
- **Driven by rounds:** R1
- **Dependent decisions:** —
- **Referenced in plan:** Runtime Behavior (save path errors)

## D-20: Companion-file save ordering — companions first, config last

- **Question:** In what order does a save write the bundle's files?
- **Decision:** `workflowio.Save` writes every dirty companion file (new or modified prompts and scripts) via `atomicwrite.Write` before writing `config.json`. Companion writes proceed sequentially; on any companion failure, the already-written companion files are **not** rolled back — they are left on disk (see D-24 for crash-recovery stance). `config.json` is the last write; if it fails, the companion files on disk may reference paths the old `config.json` does not know about. This is acceptable because the user can retry save (the atomic-rename primitive is idempotent) or quit without writing.
- **Rationale:** Spec D60 commits to this ordering so a crash mid-save leaves the configuration file in its prior state; the old config doesn't reference the new companion files, so the user's runtime is unaffected. Rolling back partial companion writes would require a more complex transactional layer (two-phase commit) whose complexity outweighs the benefit for a developer tool.
- **Evidence:** Spec D60. Edge case AS-4 (companion partial failure). Test plan `TestSave_CompanionFilesWrittenBeforeConfig`.
- **Rejected alternatives:**
  - Transactional rollback of companions on failure — rejected; complexity exceeds value for a developer tool; spec D60 explicitly permits the current shape.
  - Config first then companions — rejected (D60); leaves the config referencing not-yet-written companion files, which fails validation on next load.
- **Specialist owner:** `software-architect`
- **Revisit criterion:** Reopen if orphaned companion files become a reported user-visible problem (at scale they are disk clutter, not data loss).
- **Dissent (if any):** —
- **Driven by rounds:** R1
- **Dependent decisions:** —
- **Referenced in plan:** Runtime Behavior (save path)

## D-21: Create-on-editor-open applies EvalSymlinks + containment before `os.Create`

- **Question:** When the user opens the external editor on a placeholder `promptFile` that does not exist (OQ-4 scaffold case), and the file is created on disk before the editor spawns, how does the create-path avoid a symlink escape?
- **Decision:** The create-on-editor-open path in `workflowedit` (or `workflowio.CreateEmptyCompanion`): (1) joins `workflowDir + "prompts/" + promptFile`; (2) calls `filepath.EvalSymlinks` on the full resolved path — if it returns an error **other** than `os.IsNotExist` (the expected case for a not-yet-created file), returns the error as D56 fatal; (3) calls `filepath.EvalSymlinks(filepath.Dir(path))` on the parent directory and asserts the resolved parent is a prefix of `filepath.Join(workflowDir, "prompts/")` (containment check on the directory, not the not-yet-existent file); (4) on containment success, creates the file via `os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)` — **not** `atomicwrite.Write` for a zero-byte file, because atomic-rename of an empty file through a symlink is unnecessary for the create case; `O_EXCL` prevents race overwrite.
- **Rationale:** Security NEW-1 identified this path as HIGH because the handoff brief was ambiguous about whether OQ-4's "builder creates it empty" happened in memory or on disk. The editor needs a real file to open. The containment check must apply to the parent directory since the target doesn't exist yet; `EvalSymlinks` on the parent resolves any symlinks in the intermediate path components. `O_EXCL` prevents a TOCTOU race where an attacker pre-creates the path.
- **Evidence:** Security Round 2 NEW-1. OQ-4 Round 1 resolution. Existing `src/internal/cli/args.go` EvalSymlinks precedent.
- **Rejected alternatives:**
  - Pass a non-existent path to the editor and let the editor create it — rejected; editors that don't create files fail; and the builder's subsequent re-read from disk may miss the file the editor wrote at a different path (Edge case "Save As" row).
  - Create without containment check — rejected (NEW-1); attacker-controlled `promptFile` value creates an empty file outside the bundle tree.
- **Specialist owner:** `adversarial-security-analyst`
- **Revisit criterion:** Reopen if a user workflow legitimately needs prompt files outside `<workflowDir>/prompts/` (unsupported by current convention; would require spec change).
- **Dissent (if any):** —
- **Driven by rounds:** R2
- **Dependent decisions:** —
- **Referenced in plan:** Security Posture; Runtime Behavior (editor open)

## D-22: `$VISUAL`/`$EDITOR` word-splitting via `github.com/google/shlex`

- **Question:** What Go function splits `$VISUAL`/`$EDITOR` into command + args?
- **Decision:** Add `github.com/google/shlex` as a new module dependency. In `cmd/pr9k/workflow.go`'s `resolveEditor`, call `shlex.Split(os.Getenv("VISUAL"))` to produce `[]string` tokens; first token is the command, remaining are arguments. Same logic applies to `$EDITOR` fallback. Handles single-quoted tokens (`VISUAL='code --wait'` → `["code", "--wait"]`), double-quoted tokens, and backslash-escaped spaces. Paths containing spaces without quotes (`VISUAL=/Applications/Sublime Text.app/... --wait`, edge case EE-1) still split on the unquoted space; the plan commits to the error message naming this case: "Editor path contains an unquoted space — quote the path in your shell's `~/.profile` (e.g., `export VISUAL='/Applications/Sublime Text.app/.../subl --wait'`)."
- **Rationale:** Security Finding 1 identified the gap. `google/shlex` is ubiquitous (used by Docker, google/cadvisor, many others), minimal (~300 LOC), MIT-licensed. A hand-rolled shlex adds code to maintain without benefit. The committed test case (`VISUAL='code --wait'` → `command=code, args=[--wait]`) pins the quoting behavior.
- **Evidence:** Security Finding 1, Round 2 resolution (`/tmp/wb-round2-resolutions.md`). Edge case EE-1.
- **Rejected alternatives:**
  - `strings.Fields` — rejected (security Finding 1); breaks single-quoted tokens.
  - Hand-rolled shlex — rejected; maintenance surface without benefit over a battle-tested library.
  - `os/exec`'s `LookPath` with shell wrapper — rejected; reintroduces shell-injection risk.
- **Specialist owner:** `adversarial-security-analyst`
- **Revisit criterion:** Reopen if `google/shlex` is ever deprecated or has a security advisory.
- **Dissent (if any):** —
- **Driven by rounds:** R1, R2
- **Dependent decisions:** D-6
- **Referenced in plan:** Security Posture; External Interfaces

## D-23: `stripANSI` reuse via `statusline.Sanitize`; recovery view capped at 8 KiB; symlink banner before recovery-view render

- **Question:** How does the parse-error recovery view defend against ANSI injection and SSH-private-key-style content exposure?
- **Decision:** Three commitments:
  1. Reuse `statusline.Sanitize` from `internal/statusline` (already exported). `workflowio.loadConfigRaw` / the recovery-view renderer in `workflowedit` calls `statusline.Sanitize` on the raw bytes before display.
  2. Cap displayed raw bytes at **8 KiB** (matching the existing `statusline` 8 KiB stdout cap). Files larger than 8 KiB have the first 8 KiB shown with an explicit truncation note; the rest is not rendered.
  3. Load pipeline ordering: **symlink-detect → symlink-banner render → parse attempt → recovery-view render**. The symlink banner is always rendered before any recovery-view content from a symlinked target. On a symlink-detect failure (error other than "not a symlink"), the load aborts and the user sees the banner without ever entering the recovery view.
- **Rationale:** Security Findings 2 and 8 identified the terminal injection surface (OSC 8 hyperlinks, window-title exfiltration) and the SSH-private-key content-exposure case. `statusline.Sanitize` already exists and is tested; promoting it to `internal/ansi` is premature (architect Round 2 §3.2 — one-caller package anti-pattern). The 8 KiB cap limits exposure; combined with ANSI strip, the terminal-injection threat is fully closed. Content exposure is mitigated but not fully closed (a complete SSH key fits in 8 KiB); security Round 2 explicitly documents this as accepted residual.
- **Evidence:** Security Findings 2, 8; security Round 2 `/tmp/wb-sec-round2.md`. Architect Round 2 §3.2. Existing `src/internal/statusline/sanitize.go`.
- **Rejected alternatives:**
  - New `internal/ansi` package — rejected in Round 2 (one-caller).
  - No cap (render full raw bytes) — rejected (Finding 2); exposes arbitrary binary content.
  - Content-type filter (reject non-UTF-8 raw) — rejected as additional scope; the 8 KiB + ANSI strip is accepted residual per security Round 2.
- **Specialist owner:** `adversarial-security-analyst`
- **Revisit criterion:** Reopen if a second caller for ANSI stripping outside `statusline` and the recovery view emerges — then extract to `internal/ansi`.
- **Dissent (if any):** —
- **Driven by rounds:** R1, R2
- **Dependent decisions:** —
- **Referenced in plan:** Security Posture; Runtime Behavior (load pipeline)

## D-24: Orphaned-companion-file crash residue is an accepted limitation; no detection in v1

- **Question:** Does the builder detect companion files written before a crash-era config-rename failure (edge case AS-4)?
- **Decision:** No detection in v1. The save sequence writes companions first, config last (D-20). On crash between companion writes and config rename, the orphaned companion files remain on disk; the old `config.json` does not reference them; the user's runtime is unaffected. Detection would require a separate transaction log or a second `D42-a`-style scan looking for "files referenced by the in-memory model after load but not yet written at crash time" — a complexity the spec did not commit to. v1 documents this as an accepted limitation in the ADR.
- **Rationale:** Edge case AS-4 (P0 prioritized by edge-case-explorer) traces the scenario. Spec D60 is silent on rollback; the rollback discussion is deferred to v1 explicitly ("the user can retry after freeing space" — implies no automatic cleanup). A transactional layer is out of scope.
- **Evidence:** Edge case AS-4. Spec D60.
- **Rejected alternatives:**
  - Add a manifest file listing pending writes — rejected (complexity); a third durable file introduces new crash windows.
  - Scan on open for companion files unreferenced by config.json — rejected (false positives for user-added files the builder's model doesn't know about).
- **Specialist owner:** `devops-engineer`
- **Revisit criterion:** Reopen if users report disk bloat from orphaned companions or if a second-generation durability layer is added.
- **Dissent (if any):** —
- **Driven by rounds:** R1
- **Dependent decisions:** —
- **Referenced in plan:** Out-of-scope; Risks

## D-25: Filesystem tab-completion is a custom minimal implementation (not a new dependency)

- **Question:** How is path-picker tab-completion implemented (JrF-6 scope concern)?
- **Decision:** A custom, minimal `pathcomplete` helper inside `internal/workflowedit/pathpicker.go`. Contract: on `Tab`, dispatch a `tea.Cmd` that calls `os.ReadDir(prefixDir)` (off the Update goroutine), sorts matches lexicographically, filters to names starting with the typed basename prefix (case-sensitive on POSIX), and returns a `pathCompletionMsg{matches []string}`. `Update` on receipt: exactly one match → auto-complete; multiple matches → cycle on repeated Tab; zero matches → no-op (ring bell). Hidden (`.`-prefixed) files shown only when the typed prefix starts with `.`. `~` is expanded to `os.UserHomeDir()` before `os.ReadDir`. Multi-byte UTF-8 is preserved; no NFD/NFC normalization in v1 (edge case PH-2 is a known limitation documented in the how-to guide).
- **Rationale:** JrF-6 flagged this as an unscoped implementation cost. No existing `bubbles` component provides this; a new dependency is heavier than the ~100 LOC custom implementation. Async `tea.Cmd` dispatch satisfies UX OQ-3 (no filesystem I/O on the Update goroutine).
- **Evidence:** Junior JrF-6. UX OQ-3. OQ-8 Round 1 resolution (hidden-file rule). Edge case PH-2 (normalization), PH-4 (`~` expansion).
- **Rejected alternatives:**
  - New `bubbles` dependency or third-party completion widget — rejected; the implementation scope is bounded and a dependency adds maintenance surface.
  - Synchronous `os.ReadDir` on `Update` — rejected (UX OQ-3); TUI stalls on slow filesystems.
  - Full filesystem autocomplete library — rejected (scope).
- **Specialist owner:** `user-experience-designer` + `software-architect`
- **Revisit criterion:** Reopen if user feedback shows NFD/NFC mismatch on macOS is a common pain point; then add Unicode NFC normalization before comparison.
- **Dissent (if any):** —
- **Driven by rounds:** R1
- **Dependent decisions:** —
- **Referenced in plan:** Runtime Behavior (path picker); Decomposition and Sequencing (WU-8)

## D-26: External-editor handoff two-step: "Opening editor…" pre-render, then ExecProcess Cmd

- **Question:** How does the "Opening editor…" message render before `tea.ExecProcess` releases the terminal?
- **Decision:** A two-cycle handoff. Cycle 1 (`Update` receives `openEditorMsg`): set `dialog.kind = DialogExternalEditorOpening`, return `tea.Tick(10*time.Millisecond, func(...) tea.Msg { return launchEditorMsg{filePath: ...} })`. Cycle 1 renders with the "Opening editor…" notice visible. Cycle 2 (`Update` receives `launchEditorMsg`): call `editorRunner.Run(filePath, exitCallback)` to obtain the `tea.Cmd`, return it. The Bubble Tea runtime executes the cmd, calls `ReleaseTerminal`, spawns the editor. The 10 ms tick gives the renderer time to paint the notice; the exact duration is not load-bearing (any positive tick suffices; 10 ms is imperceptible).
- **Rationale:** JrF-4 identified the single-cycle gap. `tea.ExecProcess` calls `ReleaseTerminal` immediately; without an intervening render cycle, the notice never reaches the screen.
- **Evidence:** Junior JrF-4. T2. Bubble Tea source `bubbletea@v1.3.10/exec.go:103`.
- **Rejected alternatives:**
  - Single-cycle (return the ExecProcess cmd directly) — rejected (JrF-4); notice silently dropped.
  - Print the notice to stderr before returning the cmd — rejected; breaks Bubble Tea's ownership of the output stream and produces visible artifacts in alt-screen mode.
- **Specialist owner:** `user-experience-designer` + `software-architect`
- **Revisit criterion:** Reopen if the two-cycle latency becomes a user-visible delay (would need a tick longer than 10 ms to notice).
- **Dissent (if any):** —
- **Driven by rounds:** R1
- **Dependent decisions:** —
- **Referenced in plan:** Runtime Behavior (external editor handoff)

## D-27: Session-event logging uses `logger.Log` with fixed stepName `"workflow-builder"` and a field-exclusion contract

- **Question:** What is the contract for D39 session-event log entries?
- **Decision:** `workflowedit.Model` holds a `*logger.Logger` injected via `workflowDeps`. Session events are logged via `log.Log("workflow-builder", line)` with `line` being a pre-formatted human-readable string. The event catalog: `session_start target=<path>`, `save_ok validators=<fatal:N warn:N info:N> confirmations=<external|symlink|none>`, `save_failed reason=<short>`, `editor_invoked binary=<first-token-of-VISUAL> exit_code=<int> duration_ms=<int>`, `quit unsaved=<bool>`. **No** `containerEnv` key or value; **no** `env` entry value; **no** prompt-file content; **no** full `$VISUAL` argument list. The first-token-only rule for editor binary is the defense against credential-in-args leakage.
- **Rationale:** Security Findings 6 and 9 identified the secret-leak path through logger events. Closed-form event catalog with explicit exclusions removes the shape of the risk. No new logger method is needed in v1; the `Log(stepName, line)` call shape is sufficient given the explicit formatting commitment.
- **Evidence:** Security Findings 6, 9. Spec D39. Existing `src/internal/logger/logger.go:67-85`.
- **Rejected alternatives:**
  - Structured JSON Lines logger with field map — acceptable but adds API surface; rejected for v1 in favor of human-readable lines.
  - Log full editor command — rejected (Finding 9); credentials in `$VISUAL` would leak.
  - Log full save-outcome validator findings — rejected; finding `Problem` text could grow to include sensitive content in future validator changes.
- **Specialist owner:** `adversarial-security-analyst` + `devops-engineer`
- **Revisit criterion:** Reopen if an operator needs machine-readable log analysis (at that point add a `LogEvent` structured method and migrate).
- **Dissent (if any):** —
- **Driven by rounds:** R1
- **Dependent decisions:** —
- **Referenced in plan:** Operational Readiness; Security Posture

## D-28: `env`/`containerEnv` entry editing: detail-pane fields + D48 remove dialog + no reorder

- **Question:** What do `env` and `containerEnv` entry add / edit / remove flows look like?
- **Decision:** An `env` entry is a single plain-text field (the name) in the detail pane. A `containerEnv` entry is two plain-text fields (key, value) with D20 secret masking on values matched by the secret pattern. Uniqueness of keys within a section is enforced by the validator at save (not at input time). Removal uses the D48 two-option confirmation dialog (`Delete entry "<name>"? (Delete / Cancel)`, Cancel default) consistent with step removal. Reorder is **not** supported for `env` or `containerEnv` in v1 (order is not behaviorally significant — Docker last-wins for containerEnv; env-passthrough order is cosmetic). Unnamed entries render the placeholder label `(unnamed)` in dimmed style in the outline (R2-C3).
- **Rationale:** OQ-1 Round 1 resolution plus UX Round 2 unnamed-entry commitment. The placeholder label prevents visually-identical rows for two pending-name entries.
- **Evidence:** OQ-1 Round 1 resolution. UX Round 2 R2-C3. JrF-7.
- **Rejected alternatives:**
  - Reorder support — rejected; no behavioral significance.
  - Input-time uniqueness check — rejected; inconsistent with D18/D23 validator posture (validator owns duplicate detection).
- **Specialist owner:** `user-experience-designer`
- **Revisit criterion:** Reopen if users request env reorder for documentation-readability reasons.
- **Dissent (if any):** —
- **Driven by rounds:** R1, R2
- **Dependent decisions:** —
- **Referenced in plan:** Runtime Behavior (detail-pane editing)

## D-29: `statusLine` add affordance is `+ Add statusLine block` row; defaults to `refreshIntervalSeconds=0` with inline hint

- **Question:** How does the user add a `statusLine` block when none is configured (JrF-12)?
- **Decision:** When no `statusLine` block exists, the outline renders a `statusLine` section header followed by a single affordance row `+ Add statusLine block`. Activating the row (Enter, or `a` shortcut when the section header is focused) creates an in-memory statusLine with defaults: `type="command"`, `refreshIntervalSeconds=0`, `command=""`. Focus moves to the `command` field. The detail pane renders an inline hint adjacent to the `refreshIntervalSeconds` field: `0 disables automatic refresh` in dimmed style (R2-C4). Removal follows the same D48 two-option confirmation dialog.
- **Rationale:** OQ-2 Round 1 + UX Round 2 R2-C4. The `+ Add` affordance matches D46's pattern even though the statusLine is a single object (not a list). The inline hint prevents the first-time-user confusion where the default (0) silently disables refresh.
- **Evidence:** OQ-2 Round 1 resolution. UX Round 2 R2-C4. JrF-12. Spec D46, D51.
- **Rejected alternatives:**
  - No `+ Add` row; require File > preferences menu — rejected; inconsistent with the outline-as-surface pattern.
  - Default `refreshIntervalSeconds=5` — rejected; silently runs a user's empty command every 5 seconds once they set one.
- **Specialist owner:** `user-experience-designer`
- **Revisit criterion:** Reopen if users confuse the dimmed hint with dimmed invalid state.
- **Dissent (if any):** —
- **Driven by rounds:** R1, R2
- **Dependent decisions:** —
- **Referenced in plan:** Runtime Behavior (outline structure)

## D-30: Step removal uses D48 two-option dialog `Delete step "<name>"? (Delete / Cancel)`

- **Question:** What does the step-remove confirmation affordance look like (JrF-15)?
- **Decision:** `Del` when an outline step row is focused (D-10), or click on the focused row's `🗑` glyph, opens a D48-conformant two-option dialog: `Delete step "<name>"? (Delete / Cancel)`. Cancel is the keyboard default. The shortcut footer shows `Del  remove` when an outline step row is focused. Post-removal focus falls to next step below, then above, then to the phase's `+ Add step` row (Edge Cases table — "Focused step is removed").
- **Rationale:** OQ-3 Round 1 resolution. Spec JrF-15 identified this as underspecified. The D48 convention + Cancel default prevents accidental destructive confirmation via Enter.
- **Evidence:** OQ-3 Round 1 resolution. JrF-15. Spec D48.
- **Rejected alternatives:**
  - No confirmation (immediate remove) — rejected (spec line: "Removing a step requires a confirmation affordance").
  - Two-step Discard-style confirmation — rejected (R3 simplified away two-step for unsaved-changes; consistency argues for the same simplification here).
- **Specialist owner:** `user-experience-designer`
- **Revisit criterion:** Reopen if users report misfires under the single-step confirmation.
- **Dissent (if any):** —
- **Driven by rounds:** R1
- **Dependent decisions:** D-10
- **Referenced in plan:** Runtime Behavior (outline editing)

## D-31: Findings panel is independently scrollable with preserved state across rebuild

- **Question:** Is the findings panel scrollable independently of the outline and detail pane (OQ-5)?
- **Decision:** Yes. The findings panel uses its own `bubbles/viewport` instance. Arrow-key navigation within the panel auto-scrolls to keep the focused finding visible (matching D29/D52 pattern). When the help modal opens over the findings panel (D-8), the panel is visually dimmed but its scroll state is preserved for restoration. The panel's rebuild logic (on each save attempt, per spec D35) preserves scroll offset when the rebuild produces at least one finding whose identity matches the previously-focused one; otherwise scroll resets to top.
- **Rationale:** OQ-5 Round 1 resolution. Consistent with D29/D52 scrollability precedent and the test plan's mode-coverage expectation of scrollable findings.
- **Evidence:** OQ-5 Round 1 resolution. Spec D35. Edge case VI-3.
- **Rejected alternatives:**
  - Non-scrollable panel — rejected (edge case VI-3 — 600+ findings overflow terminal height).
  - Scroll reset on every rebuild — rejected; loses user's place when they jump to a field and return.
- **Specialist owner:** `user-experience-designer`
- **Revisit criterion:** Reopen if the "preserve scroll across rebuild when focused finding identity persists" heuristic proves confusing.
- **Dissent (if any):** —
- **Driven by rounds:** R1
- **Dependent decisions:** —
- **Referenced in plan:** Runtime Behavior (findings panel)

## D-32: D38 doc obligations extended with doc-integrity test list

- **Question:** Which doc-integrity tests must pass for D38's documentation obligations?
- **Decision:** Eight new tests appended to `src/cmd/pr9k/doc_integrity_test.go` (following the existing `TestDocIntegrity_CLAUDEmd_*` pattern): (DI-1) `docs/features/workflow-builder.md` exists and is linked from `CLAUDE.md`; (DI-2) `docs/how-to/using-the-workflow-builder.md` exists, linked, contains `"Ctrl+N"` and `"Ctrl+O"`; (DI-3) `docs/how-to/configuring-external-editor-for-workflow-builder.md` exists, linked, contains `"$VISUAL"`, `"$EDITOR"`, `"code --wait"`, and does **not** contain a fallback-to-`vi` recommendation; (DI-4) ADR for save-durability exists under `docs/adr/`, contains `"atomic"` and `"rename"`, linked from `CLAUDE.md`'s ADR section; (DI-5) `docs/coding-standards/file-writes.md` exists, linked, contains `"temp"` and `"rename"`; (DI-6) one `docs/code-packages/<pkg>.md` per new internal package (`atomicwrite`, `workflowmodel`, `workflowio`, `workflowvalidate`, `workflowedit`), each linked from `CLAUDE.md`; (DI-7) `docs/features/cli-configuration.md` contains `"workflow"`, `"--workflow-dir"`, `"--project-dir"`, and does **not** contain `"--iterations"` in the workflow-subcommand section; (DI-8) external-editor how-to contains `"tmux"` or `"Alt"` and references reorder mode `"r"` fallback.
- **Rationale:** DevOps DOR-008 identified the gap. Test-engineer DI-1 through DI-8 names the exact shape. The tests are mechanical enforcement of the documentation obligations D38 commits to.
- **Evidence:** DevOps DOR-008. Test plan section 8 (`/tmp/wb-test-findings.md`). Spec D38.
- **Rejected alternatives:**
  - Defer doc-integrity tests to a follow-up PR — rejected (`docs/coding-standards/documentation.md:18`: "docs not updated for the new X is always a medium-severity issue").
- **Specialist owner:** `test-engineer`
- **Revisit criterion:** Reopen if a doc obligation is renamed or deleted.
- **Dissent (if any):** —
- **Driven by rounds:** R1
- **Dependent decisions:** —
- **Referenced in plan:** Definition of Done; Testing Strategy

## D-33: Goroutine lifecycle for the builder subcommand uses `context.Context` cancellation

- **Question:** What prevents goroutine leaks from the builder subcommand (main.go-style heartbeat, statusline, signal handler) when `program.Run()` returns?
- **Decision:** Every goroutine spawned by `runWorkflowBuilder` receives a `context.Context` derived from a `ctx, cancel := context.WithCancel(cmd.Context())`. `cancel()` is `defer`red at the top of `runWorkflowBuilder`. Goroutines select on `ctx.Done()` alongside their work channels. Signal handler uses the same context; on signal, it calls `cancel()` and lets the goroutines unwind. The "terminates with the process" pattern from `main.go:204-212` is explicitly **not** copied — that comment is specific to the main binary which calls `os.Exit` after `program.Run`; the `workflow` subcommand returns control to cobra and then to `main.go`, so a leaked goroutine continues running.
- **Rationale:** Concurrency C7. The existing heartbeat goroutine pattern is process-lifetime-bound; a subcommand that returns needs explicit lifecycle management.
- **Evidence:** Concurrency C7 (`/tmp/wb-concurrency-findings.md`). Existing `src/cmd/pr9k/main.go:204-212`.
- **Rejected alternatives:**
  - Copy the `main.go` heartbeat pattern verbatim — rejected (C7); leaks on subcommand return.
  - Global shutdown channel — rejected; `context.Context` is the idiomatic Go primitive for this and integrates with `cobra.Command.Context()`.
- **Specialist owner:** `concurrency-analyst`
- **Revisit criterion:** Reopen if a goroutine surfaces that cannot take a `context.Context` (would indicate a library-boundary issue).
- **Dissent (if any):** —
- **Driven by rounds:** R1
- **Dependent decisions:** —
- **Referenced in plan:** Runtime Behavior (lifecycle); Risks (leak)

## D-34: Signal handler does not call `program.Kill()` during `tea.ExecProcess` window

- **Question:** How does the builder's signal handler avoid bypassing `RestoreTerminal` when SIGINT arrives during the editor window?
- **Decision:** `runWorkflowBuilder`'s signal handler sends a `quitMsg{}` via `program.Send` and does not call `program.Kill()` within the first N seconds. If `program.Run()` has not returned after a grace period (e.g., 10 s), the handler logs and exits the process via `os.Exit(130)` — accepting a corrupted terminal in the extreme edge case where the editor never exits and the user's only recourse was already `reset`. The ordinary path: SIGINT → `quitMsg` → Bubble Tea runtime ensures the editor exits (SIGINT is delivered to the foreground process group) → `RestoreTerminal` fires → `ExecCallback` runs with exit code 130 (D-7) → builder enters quit-confirm flow.
- **Rationale:** Concurrency C3. The existing `main.go:270-279` `program.Kill()` 2-second timeout pattern cannot be copied because `program.Kill()` bypasses `RestoreTerminal`, corrupting the terminal when SIGINT arrives mid-ExecProcess.
- **Evidence:** Concurrency C3. Spec Edge Cases table (SIGINT row). Existing `src/cmd/pr9k/main.go:270-279`.
- **Rejected alternatives:**
  - `program.Kill()` after 2 s — rejected (C3); corrupts terminal.
  - No grace period / immediate `program.Kill()` — rejected; same corruption at zero delay.
  - Track "ExecProcess active" state explicitly and force `RestoreTerminal` — rejected as added complexity; letting Bubble Tea's built-in ExecProcess completion handle it is sufficient for every non-pathological case.
- **Specialist owner:** `concurrency-analyst`
- **Revisit criterion:** Reopen if a user reports a stuck builder after editor hang — may need a force-kill fallback that's aware of the ExecProcess state.
- **Dissent (if any):** —
- **Driven by rounds:** R1
- **Dependent decisions:** D-7
- **Referenced in plan:** Runtime Behavior (signals); Risks

## D-35: Builder constants file collects all affordance signifiers

- **Question:** Where do the `▾`, `⋮⋮`, `+ Add …`, `(unnamed)`, `[press r to reveal]` strings live?
- **Decision:** A new file `src/internal/workflowedit/constants.go` collects every user-visible affordance string:
  - `GripperGlyph = "⋮⋮"` — D34 rationale (avoid Braille-font risk)
  - `ChoiceListIndicator = "▾"` — D27
  - `AddRowPrefix = "+ Add "` — D46
  - `PlaceholderLabel = "(unnamed)"` — D-28 / UX R2-C3
  - `SecretRevealHint = "[press r to reveal]"` — D20 / D47
  - `MoreWarningsFormat = "[%d more warnings]"` — D49
  - Plus item-type strings: `"step"`, `"env variable"`, `"container env entry"`, `"statusLine block"`
- **Rationale:** UX IP-006. Without a constants file, glyph-and-string rationale (especially D34's Braille-font decision) becomes disconnected from the code and risks future reversion. Single-location makes terminal-font regression debugging one-grep away.
- **Evidence:** UX IP-006. Spec D27, D34, D46, D47, D49.
- **Rejected alternatives:**
  - Inline strings at use sites — rejected (IP-006); rationale lost.
- **Specialist owner:** `user-experience-designer`
- **Revisit criterion:** Reopen if i18n requirements land and strings need externalization.
- **Dissent (if any):** —
- **Driven by rounds:** R1
- **Dependent decisions:** —
- **Referenced in plan:** Decomposition and Sequencing (WU-6, WU-7)

## D-36: `pr9k workflow` wires as a peer subcommand to `pr9k sandbox`; bypasses `startup()`

- **Question:** How is `pr9k workflow` registered in `main.go` and wired relative to the existing `startup()` pre-run sequence?
- **Decision:** `cli.Execute(newSandboxCmd(), newWorkflowCmd())` in `main.go`. `newWorkflowCmd()` lives in `cmd/pr9k/workflow.go`. `runWorkflowBuilder(deps, workflowDir, projectDir)` is called directly from `cmd.RunE` — **not** through `startup()`. The builder creates its own `*logger.Logger` (with `workflow-` prefix per D-15), resolves `workflowDir` (via reused `cli.resolveWorkflowDirWith`) and `projectDir` (via reused `cli.resolveProjectDir`), and skips the Docker preflight, step loading, and D13 full-workflow validation that `startup()` performs for the run loop.
- **Rationale:** Architect R7. `startup()` satisfies orchestrator needs that don't apply to the builder — requiring Docker to launch, validating a not-yet-chosen workflow, creating an artifact directory that may never be used. The sandbox subcommand pattern proves the clean DI seam works.
- **Evidence:** Architect R7 (`/tmp/wb-arch-findings.md`). Existing `src/cmd/pr9k/sandbox_create.go:19` pattern.
- **Rejected alternatives:**
  - Wire through `startup()` — rejected (R7); couples to Docker, step loading.
  - Inline in main.go without a cobra subcommand — rejected; contradicts D1 and breaks `--help` discovery.
- **Specialist owner:** `software-architect`
- **Revisit criterion:** Reopen if startup state (Docker sandbox profile, logger) becomes something the builder legitimately needs.
- **Dissent (if any):** —
- **Driven by rounds:** R1
- **Dependent decisions:** —
- **Referenced in plan:** Architecture and Integration Points; External Interfaces (CLI)

## D-37: Production integration test `TestValidate_ProductionStepsJSON` must pass unchanged

- **Question:** Does extending the validator with `ValidateDoc` risk breaking the existing production-config integration test?
- **Decision:** No. `Validate(workflowDir string) []Error` keeps its signature and its behavior verbatim. Internally it calls `ValidateDoc(doc, workflowDir, nil)` after deserializing `config.json` from disk. The nil `companionFiles` means the new `readCompanionOrDisk` and `statCompanionOrDisk` helpers fall through to `os.ReadFile` and `os.Stat` exactly as before. `TestValidate_ProductionStepsJSON` does not change; it is the integration pin confirming the delegation doesn't break the path-based entry.
- **Rationale:** Architect Round 2 §1.4 audited the internal structure and confirmed the wrapping. This is both a decision and a Definition-of-Done criterion.
- **Evidence:** Architect Round 2 §1.4. Test plan section 9 (`/tmp/wb-test-findings.md`). Spec D14.
- **Rejected alternatives:** —
- **Specialist owner:** `test-engineer`
- **Revisit criterion:** Reopen only if the test starts failing during implementation (signal that the refactoring is going wrong).
- **Dissent (if any):** —
- **Driven by rounds:** R1, R2
- **Dependent decisions:** D-3
- **Referenced in plan:** Testing Strategy; Definition of Done

## D-38: OI-1 validator hardening lands in the same PR as the builder

- **Question:** Does the validator's `safePromptPath` hardening (spec OI-1) land in this PR, in a prior PR, or deferred?
- **Decision:** Lands in the same PR. Two changes to `internal/validator`: (1) `safePromptPath` adds `filepath.EvalSymlinks` before the containment `HasPrefix` check; on error (including escape beyond the `prompts/` tree) return an error; (2) `validateCommandPath` receives an analogous `EvalSymlinks` + containment check for script paths. The existing production integration test covers the behavior with non-symlink files; a new test covers the symlink-escape rejection case.
- **Rationale:** The builder adds a **write** path (open-in-external-editor) through `safePromptPath`'s resolution. Security Finding 5 escalates OI-1's severity from "read exposure" to "write-arbitrary-file" because the builder hands the resolved path to the editor, which writes back. Landing OI-1 and the builder atomically closes this exploit class entirely.
- **Evidence:** Security Finding 5 (`/tmp/wb-sec-findings.md`), Round 2 (closed). Spec OI-1. Architect R3 (workflowvalidate bridge is downstream of this fix).
- **Rejected alternatives:**
  - Ship builder before OI-1 — rejected (Finding 5); the builder materially expands the exploit path.
  - Defer builder until OI-1 ships separately — rejected; same PR accomplishes both with less coordination cost and matches OI-1's own recommendation ("the implementation plan should address in the same PR").
- **Specialist owner:** `adversarial-security-analyst` + `software-architect`
- **Revisit criterion:** Reopen only if implementation surfaces an unanticipated regression from the EvalSymlinks addition (would indicate OI-1's scope is wrong, not the decision to land it here).
- **Dissent (if any):** —
- **Driven by rounds:** R1
- **Dependent decisions:** —
- **Referenced in plan:** Security Posture; Definition of Done; Open Items

## D-39: Pre-copy integrity check and the copy operation each apply `EvalSymlinks` + containment per-open

- **Question:** How does the D61 pre-copy integrity check and the subsequent file-copy step defend against path traversal and TOCTOU?
- **Decision:** Both the pre-copy integrity check (D61) and the copy loop (`workflowio.CopyDefaultBundle`) apply `filepath.EvalSymlinks` + containment immediately **before** each `os.Stat` / `os.Open`. The integrity check does **not** cache the resolved path for use by the copy; the copy calls `EvalSymlinks` again at its own open time. Per-open guards close the TOCTOU window (security Round 2 Finding 3 Partially Closed note).
- **Rationale:** Security Finding 3 and Round 2. The integrity check and the copy are separated in time by milliseconds; an attacker with filesystem access (or a malicious default-bundle pre-copy check surface) can swap a symlink between check and open. Per-open guards close the race.
- **Evidence:** Security Finding 3, Round 2 (`/tmp/wb-sec-round2.md`). Spec D61.
- **Rejected alternatives:**
  - Cache-then-use — rejected (Round 2 TOCTOU note).
  - Single pre-scan guard — rejected (same reason).
- **Specialist owner:** `adversarial-security-analyst`
- **Revisit criterion:** Reopen if implementation introduces a third code path that reads companion files (third per-open guard would be needed).
- **Dissent (if any):** —
- **Driven by rounds:** R1, R2
- **Dependent decisions:** —
- **Referenced in plan:** Security Posture; Runtime Behavior (File > New Copy)

## D-40: Scaffold placeholder step conventions (OQ-4)

- **Question:** What is the exact shape of the "start with empty workflow" placeholder iteration step (OQ-4)?
- **Decision:** Name `"step-1"`, `isClaude: true`, `model: "claude-sonnet-4-6"` (a sensible default from D58's suggestion list), `promptFile: "step-1.md"`, `command` omitted. The referenced `prompts/step-1.md` does **not** exist on disk at scaffold creation time. It is created empty (mode 0o600) on first open-in-external-editor per D-21, or written via atomic save on first File > Save. The validator (via `ValidateDoc` with `{"step-1.md": {}}` in the companion map per D-3) reports no fatal for missing-prompt until the map is cleared — i.e., after the first save the file exists on disk and the map is empty. Until then, the workflow is "in-progress scaffold" — validator fatals are expected and acceptable session state (D61 copy-anyway precedent).
- **Rationale:** OQ-4 Round 1 resolution.
- **Evidence:** OQ-4 Round 1 resolution (`/tmp/wb-round1-resolutions.md`).
- **Rejected alternatives:**
  - Name `"feature-work"` or Ralph-specific — rejected (narrow-reading ADR; the builder is generic).
  - Create the prompt file eagerly at scaffold time — rejected; commits to disk before user intent.
- **Specialist owner:** `user-experience-designer`
- **Revisit criterion:** Reopen when the D58 model suggestion list is updated (the scaffold default follows the list).
- **Dissent (if any):** —
- **Driven by rounds:** R1
- **Dependent decisions:** D-21
- **Referenced in plan:** Runtime Behavior (scaffold)

## D-41: D69 inline reference to superseded D54 is an OI for spec author

- **Question:** D69's step-(1) text references "D54 three-way dialog ... Discard two-step confirmation" — but D54 was superseded by D7 in R3 and the two-step confirmation was removed. How does the plan handle this?
- **Decision:** The implementation plan cites **D7** as the authoritative source for the unsaved-changes dialog (single-step three-option). The spec author is asked to update D69's inline text to reference D7 and remove the "two-step confirmation" language. The implementation follows D7 behavior regardless of D69's stale text.
- **Rationale:** JrF-10 flagged the actionable inconsistency. The authoritative decision is clear (D7, not D54); the stale reference is a documentation issue that does not affect implementation, because the implementer should read D7 (which has the current behavior) before implementing.
- **Evidence:** JrF-10. Spec D7, D54 (SUPERSEDED), D69. Spec R3 simplification notes.
- **Rejected alternatives:**
  - Implement D69's stale "two-step" text literally — rejected (JrF-10); contradicts R3 user direction.
  - Block the plan on spec correction — rejected; the intent is unambiguous and spec correction is a fast-follow.
- **Specialist owner:** `project-manager` (handoff to spec author)
- **Revisit criterion:** Closed when the spec D69 text is corrected.
- **Dissent (if any):** —
- **Driven by rounds:** R1
- **Dependent decisions:** —
- **Referenced in plan:** Open Items

## D-42: Scaffold model defaults / model suggestion list lives in `workflowedit/modelsuggestions.go`

- **Question:** Where does the D58 hardcoded model-suggestion list live in Go source (JrF-8)?
- **Decision:** A new file `src/internal/workflowedit/modelsuggestions.go` owns the suggestion list as a package-level `var ModelSuggestions []string`. The file's doc comment cites D58 (hardcoded snapshot, may go stale) and points to `docs/how-to/using-the-workflow-builder.md` for the user-facing note. The validator package does **not** own this list (the validator intentionally does not constrain the `model` field per D12); placing it in the validator would couple UI suggestion to schema enforcement. Placing it in `workflowmodel` would couple suggestion policy to the mutable document; placing it in `workflowedit` puts it where the detail pane reads it.
- **Rationale:** JrF-8. The list is a UI-layer concern; per narrow-reading ADR, it should not leak into validator code.
- **Evidence:** Junior JrF-8. Spec D58, D12.
- **Rejected alternatives:**
  - In `internal/validator` — rejected; validator explicitly does not constrain the field.
  - In `workflowmodel` — rejected; model is the document, not UI presentation policy.
  - In a data file at runtime — rejected; D58 commits to "hardcoded snapshot at ship time."
- **Specialist owner:** `user-experience-designer`
- **Revisit criterion:** Reopen on every pr9k release to refresh the list if Anthropic publishes new model names.
- **Dissent (if any):** —
- **Driven by rounds:** R1
- **Dependent decisions:** —
- **Referenced in plan:** Decomposition and Sequencing (WU-6)

## D-43: Shared-install detection uses `syscall.Stat_t.Uid` (Unix-only)

- **Question:** How does the builder detect "writable shared install" for D39's banner (JrF-5)?
- **決定 (Decision):** `workflowio.DetectSharedInstall(path)` calls `os.Stat(path)` and inspects `info.Sys().(*syscall.Stat_t).Uid`. If the Uid does not match `os.Getuid()` and the path is writable by the current user, the "shared install" banner fires. An "assumes Unix" comment per `docs/coding-standards/api-design.md` documents the platform scope; if the codebase ever adds Windows support, this check becomes a build-tagged function.
- **Rationale:** JrF-5 noted the absence of ownership-check code in the current codebase. Using `syscall.Stat_t` is the idiomatic POSIX approach; the existing `sandbox.HostUIDGID()` is a Docker-UID helper with different semantics.
- **Evidence:** Junior JrF-5. Spec D39. `docs/coding-standards/api-design.md` (Unix assumption).
- **Rejected alternatives:**
  - `os.Stat` + file mode heuristic — rejected; mode doesn't identify the owner.
  - Cross-platform abstraction layer — rejected; the codebase is Unix-only.
- **Specialist owner:** `devops-engineer`
- **Revisit criterion:** Reopen if Windows support becomes a goal.
- **Dissent (if any):** —
- **Driven by rounds:** R1
- **Dependent decisions:** —
- **Referenced in plan:** Security Posture; Runtime Behavior (load-time detection)

## D-44: Log directory under `projectDir/.pr9k/logs/`; external-workflow case inherits the same directory

- **Question:** Where do builder logs land when the user edits an external workflow (outside projectDir and home) — JrF-9?
- **Decision:** Logs always write to `projectDir/.pr9k/logs/` regardless of the chosen workflow target. `projectDir` is resolved at startup via `cli.resolveProjectDir` (`os.Getwd()` or `--project-dir`). If `projectDir` cannot be resolved (theoretical Linux case: CWD deleted mid-process), the builder falls back to `os.UserConfigDir()+"/.pr9k/logs/"` and logs a one-line notice. External-workflow sessions still log into the user's primary project directory — the external workflow is what's being edited, not where the logs belong.
- **Rationale:** JrF-9. Log destination tied to the operator's project keeps logs local to the person running the tool; tying logs to the workflow's directory would spray logs across arbitrary workflow locations.
- **Evidence:** Junior JrF-9. Spec D39.
- **Rejected alternatives:**
  - Logs beside the workflow — rejected; external workflows spread log pollution.
  - No logs for external-workflow sessions — rejected; loses observability for the sessions most likely to go wrong.
- **Specialist owner:** `devops-engineer`
- **Revisit criterion:** Reopen if users report the fallback directory is unacceptable in CI environments.
- **Dissent (if any):** —
- **Driven by rounds:** R1
- **Dependent decisions:** D-15
- **Referenced in plan:** Operational Readiness — Observability

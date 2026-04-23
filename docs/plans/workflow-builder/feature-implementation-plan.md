# Feature Implementation Plan: Workflow Builder (`pr9k workflow`)

Ship the interactive `pr9k workflow` TUI subcommand as a single pull request that lands the new binary surface, the five new internal packages, the validator hardening (OI-1), the new atomic-save primitive, the coding-standards entry, and the full documentation set. The feature is additive (no flag needed) and protected by a patch version bump delivered as the first commit of the PR with a `--no-ff` merge ([D-18](artifacts/implementation-decision-log.md#d-18-version-bump-is-the-first-commit-of-the-feature-pr-merged-with---no-ff)).

<!--
Behavioral and rationale details for non-obvious choices live in
[artifacts/implementation-decision-log.md](artifacts/implementation-decision-log.md).
Round-by-round history lives in
[artifacts/implementation-iteration-history.md](artifacts/implementation-iteration-history.md).
Inline parenthetical `([D-N])` links mark each non-obvious claim.
-->

## Source Specification

- **Feature specification:** [feature-specification.md](feature-specification.md)
- **Specification decision log:** [artifacts/decision-log.md](artifacts/decision-log.md)
- **Specification team findings:** [artifacts/team-findings.md](artifacts/team-findings.md)
- **Specification technical notes:** [artifacts/feature-technical-notes.md](artifacts/feature-technical-notes.md) (T1 atomic save, T2 terminal handoff, T3 in-memory validation)
- **Specification decisions this plan inherits:** D1–D73 (73 decisions; 5 superseded: D2, D8, D31, D50, D54). The live set is the 68 non-superseded decisions.
- **Specification open items this plan must respect or resolve:** OI-1 (validator `safePromptPath` symlink-containment hardening; **landing in this PR per [D-38](artifacts/implementation-decision-log.md#d-38-oi-1-validator-hardening-lands-in-the-same-pr-as-the-builder)**).

## Outcome

When this PR lands, the `pr9k` binary exposes a new `workflow` cobra subcommand that opens an interactive Bubble Tea TUI for authoring and editing workflow bundles (config.json + referenced prompts + referenced scripts) against a menu-bar-driven File / New / Open / Save / Quit flow. The same validator that `pr9k` runs at startup evaluates the in-memory state before the save lands; saves are durable against crash and signal via a new `internal/atomicwrite` package ([D-5](artifacts/implementation-decision-log.md#d-5-atomic-save-helper-lives-in-a-new-internalatomicwrite-package-with-signature-writepath-data-mode-error)). Terminal handoff to the user's external editor is clean on entry and exit, including SIGINT during the handoff window ([D-7](artifacts/implementation-decision-log.md#d-7-execcallback-branches-on-exit-code-130--sigint-routes-to-quit-confirm-not-re-read)). The validator's long-standing OI-1 symlink-escape hardening lands atomically with the builder ([D-38](artifacts/implementation-decision-log.md#d-38-oi-1-validator-hardening-lands-in-the-same-pr-as-the-builder)).

## Context

- **Driving constraint:** pr9k users editing workflows today hand-write JSON in external editors, with no validation until `pr9k` startup and no protection against partial-write corruption. The builder closes both gaps in one ship.
- **Stakeholders:**
  - *Workflow authors* (pr9k operators tailoring their own loop): success means editing steps without touching JSON and having validation feedback before save.
  - *pr9k maintainers* updating the bundled default workflow: success means the same tool handles their case without special paths.
  - *Documentation consumers*: success means the feature doc, two how-tos, one ADR, one coding-standards entry, and five code-package docs all ship with the feature.
  - *Future contributors* writing file-write code elsewhere: success means `internal/atomicwrite` is the canonical, documented pattern they reuse ([D-17](artifacts/implementation-decision-log.md#d-17-docscoding-standardsfile-writesmd-scope--four-rules)).
- **Future-state concern:** The `internal/atomicwrite` package is the seed of a codebase-wide durability pattern. It must be correct on day one because future callers will follow it. The `workflowedit` TUI introduces the first `tea.ExecProcess` usage in the codebase — its signal and terminal-handoff semantics become precedent for any future interactive subprocess.
- **Out-of-scope boundary:** Running workflows from within the builder; importing workflows from URL/git; diffing workflows; multi-user locking; syntax highlighting in prompt bodies; version-control operations; cross-phase step drag; step templates; Windows support. See spec "Out of Scope" for the full enumeration.

## Team Composition and Participation

| Specialist | Status | Key Input |
|------------|--------|-----------|
| `project-manager` | Coordinator | Facilitated R1 and R2; synthesized the plan; accepted OQ-1 through OQ-5 per evidence-grounded defaults (R1 resolution summary) |
| `user-experience-designer` | Active R1, R2 | Seven R1 findings (IP-001–IP-007); five R2 commitments (R2-C1 through R2-C5) — dialog composition ([D-8](artifacts/implementation-decision-log.md#d-8-dialog-composition-uses-one-dialogstate-slot--helpopen-bool--depth-1-prevfocus)), Update routing ([D-9](artifacts/implementation-decision-log.md#d-9-update-routing--overlay-first-ordering-with-global-key-intercept-nested-inside)), Del scoping ([D-10](artifacts/implementation-decision-log.md#d-10-del-key-scoped-to-outline-focus-not-a-global-key)), widget-owned footer ([D-11](artifacts/implementation-decision-log.md#d-11-widget-owned-shortcut-footer-content-via-shortcutline-string)), dual-viewport rules ([D-12](artifacts/implementation-decision-log.md#d-12-two-independently-scrollable-viewports-with-pointer-side-mouse-routing)) |
| `adversarial-security-analyst` | Active R1, R2 | Nine R1 findings; R2 verdicts + NEW-1 (OQ-4 create-on-editor-open containment — [D-21](artifacts/implementation-decision-log.md#d-21-create-on-editor-open-applies-evalsymlinks--containment-before-oscreate)); named `google/shlex` ([D-22](artifacts/implementation-decision-log.md#d-22-visualeditor-word-splitting-via-githubcomgoogleshlex)), SIGINT branch ([D-7](artifacts/implementation-decision-log.md#d-7-execcallback-branches-on-exit-code-130--sigint-routes-to-quit-confirm-not-re-read)), recovery-view stripping ([D-23](artifacts/implementation-decision-log.md#d-23-stripansi-reuse-via-statuslinesanitize-recovery-view-capped-at-8-kib-symlink-banner-before-recovery-view-render)), per-open TOCTOU guard ([D-39](artifacts/implementation-decision-log.md#d-39-pre-copy-integrity-check-and-the-copy-operation-each-apply-evalsymlinks--containment-per-open)), logger exclusion contract ([D-27](artifacts/implementation-decision-log.md#d-27-session-event-logging-uses-loggerlog-with-fixed-stepname-workflow-builder-and-a-field-exclusion-contract)) |
| `devops-engineer` | Active R1 | Ten findings (DOR-001 through DOR-010) driving: atomic-save package + signature ([D-5](artifacts/implementation-decision-log.md#d-5-atomic-save-helper-lives-in-a-new-internalatomicwrite-package-with-signature-writepath-data-mode-error)), log prefix ([D-15](artifacts/implementation-decision-log.md#d-15-log-filename-prefix-workflow--builder-vs-ralph--run)), PID-liveness temp detection ([D-16](artifacts/implementation-decision-log.md#d-16-temp-file-naming-configjsonpid-epoch-nstmp-with-pid-liveness-rule)), version-bump commit position ([D-18](artifacts/implementation-decision-log.md#d-18-version-bump-is-the-first-commit-of-the-feature-pr-merged-with---no-ff)), coding-standards scope ([D-17](artifacts/implementation-decision-log.md#d-17-docscoding-standardsfile-writesmd-scope--four-rules)), doc-integrity test list ([D-32](artifacts/implementation-decision-log.md#d-32-d38-doc-obligations-extended-with-doc-integrity-test-list)) |
| `software-architect` | Active R1, R2 | Eight R1 recommendations (R1–R8); R2 validator-code audit and package-count amendment — package decomposition ([D-1](artifacts/implementation-decision-log.md#d-1-package-decomposition--five-new-srcinternal-packages-plus-one-cmd-file)), `WorkflowDoc` model ([D-2](artifacts/implementation-decision-log.md#d-2-in-memory-workflow-model-lives-in-workflowmodelworkflowdoc-distinct-from-vfile-and-stepsstepfile)), `ValidateDoc` signature ([D-3](artifacts/implementation-decision-log.md#d-3-validator-extension--add-validatedocdoc-workflowdir-companionfiles-alongside-the-existing-validateworkflowdir)), `EditorRunner` interface ([D-6](artifacts/implementation-decision-log.md#d-6-editorrunner-is-a-one-method-interface-editor-resolution-is-private-to-the-production-impl)), subcommand wiring ([D-36](artifacts/implementation-decision-log.md#d-36-pr9k-workflow-wires-as-a-peer-subcommand-to-pr9k-sandbox-bypasses-startup)) |
| `concurrency-analyst` | Active R1 | Eight findings (C1–C8) driving: async save + in-progress flag ([D-13](artifacts/implementation-decision-log.md#d-13-save-is-async-via-teacmd-save-in-progress-flag--snapshot-refresh-on-callback)), nanosecond mtime snapshot ([D-14](artifacts/implementation-decision-log.md#d-14-d41-mtime-snapshot-uses-nanosecond-precision-timetime-from-post-rename-osstat)), goroutine lifecycle ([D-33](artifacts/implementation-decision-log.md#d-33-goroutine-lifecycle-for-the-builder-subcommand-uses-contextcontext-cancellation)), signal handler ([D-34](artifacts/implementation-decision-log.md#d-34-signal-handler-does-not-call-programkill-during-teaexecprocess-window)) |
| `test-engineer` | Active R1 | Test inventory per package; T1/T2/T3 test matrices; 28 TUI mode-coverage entries; 12 edge-case tests; eight doc-integrity tests ([D-32](artifacts/implementation-decision-log.md#d-32-d38-doc-obligations-extended-with-doc-integrity-test-list)); production-config pin ([D-37](artifacts/implementation-decision-log.md#d-37-production-integration-test-testvalidate_productionstepsjson-must-pass-unchanged)) |
| `edge-case-explorer` | Active R1 | 30 findings (four P0: EXDEV, orphaned companions, `$VISUAL`-with-space, negative viewport) — drove EXDEV taxonomy ([D-19](artifacts/implementation-decision-log.md#d-19-exdev-cross-device-rename-error-surfaced-via-d56-template)), orphaned-companion acceptance ([D-24](artifacts/implementation-decision-log.md#d-24-orphaned-companion-file-crash-residue-is-an-accepted-limitation-no-detection-in-v1)) |
| `junior-developer` | Active R1 | 15 findings (JrF-1 through JrF-15) — created reframing pressure on "feasible" (JrF-1 → R2 architect audit), location of model suggestions ([D-42](artifacts/implementation-decision-log.md#d-42-scaffold-model-defaults--model-suggestion-list-lives-in-workfloweditmodelsuggestionsgo)), shared-install detection ([D-43](artifacts/implementation-decision-log.md#d-43-shared-install-detection-uses-syscallstat_tuid-unix-only)), log-dir fallback ([D-44](artifacts/implementation-decision-log.md#d-44-log-directory-under-projectdirpr9klogs-external-workflow-case-inherits-the-same-directory)), D69 stale reference ([D-41](artifacts/implementation-decision-log.md#d-41-d69-inline-reference-to-superseded-d54-is-an-oi-for-spec-author)) |

## Implementation Approach

### Architecture and Integration Points

The builder ships as six new compilation units grouped per a six-package decomposition ([D-1](artifacts/implementation-decision-log.md#d-1-package-decomposition--five-new-srcinternal-packages-plus-one-cmd-file)):

```
cmd/pr9k/
    workflow.go          — cobra subcommand, workflowDeps, realEditorRunner (D-36)

src/internal/
    workflowedit/        — Bubble Tea Model + Update + View for the editor TUI
        model.go         — the top-level tea.Model
        dialogs.go       — DialogKind enum + dialogState; per-kind handlers (D-8)
        outline.go       — outline panel (viewport, rows, collapsible sections)
        detail.go        — detail pane (viewport, field rendering, auto-scroll)
        menu.go          — menu bar + File dropdown
        footer.go        — widget-owned ShortcutLine (D-11)
        pathpicker.go    — path-picker dialog + async tab-completion (D-25)
        editor.go        — EditorRunner interface (D-6), editorExitMsg, editorSigintMsg
        messages.go      — typed tea.Msg values
        constants.go     — affordance glyphs and strings (D-35)
        modelsuggestions.go — D58 snapshot (D-42)

    workflowmodel/       — WorkflowDoc mutable in-memory representation (D-2)
        model.go         — WorkflowDoc, Step, EnvEntry, StatusLineBlock, UnknownFields
        diff.go          — dirty-state comparison (disk snapshot == in-memory?)
        scaffold.go      — empty-scaffold and copy-from-default constructors (D-40)

    workflowio/          — load, save, detect (D-1)
        load.go          — parse config.json, symlink-detect first, recovery-view hooks (D-23)
        save.go          — companion-first ordering (D-20), WriteFileAtomic delegation
        detect.go        — readonly / symlink / external-workflow / shared-install checks (D-43)
        crashtemp.go     — DetectCrashTempFiles with PID-liveness (D-16)

    workflowvalidate/    — bridge: WorkflowDoc → ValidateDoc (D-4)
        validate.go      — type conversion; delegates to validator.ValidateDoc

    atomicwrite/         — WriteFileAtomic canonical helper (D-5)
        write.go         — EvalSymlinks + O_EXCL + 0o600 + fsync + rename
```

Wiring: `main.go` registers both subcommands in one `cli.Execute` call — `cli.Execute(newSandboxCmd(), newWorkflowCmd())` — matching the existing sandbox pattern ([D-36](artifacts/implementation-decision-log.md#d-36-pr9k-workflow-wires-as-a-peer-subcommand-to-pr9k-sandbox-bypasses-startup)). The `workflow` subcommand does **not** call `startup()`; it owns its own logger creation, `workflowDir`/`projectDir` resolution (via reused `internal/cli` helpers), and goroutine lifecycle.

Dependency layering (inner rings have no outward dependencies):

```
atomicwrite       workflowmodel        (inner — zero deps on new packages)
       \             /     |
        \          /       |
   workflowio    workflowvalidate
              \      /
           workflowedit
                |
         cmd/pr9k/workflow.go
```

`workflowedit` imports `workflowmodel`, `workflowio`, `workflowvalidate`. `workflowio` imports `workflowmodel` and `atomicwrite`. `workflowvalidate` imports `workflowmodel` and `internal/validator`. `internal/ansi` is explicitly **not** introduced; the builder's single ANSI-stripping call site reuses `internal/statusline.Sanitize` ([D-23](artifacts/implementation-decision-log.md#d-23-stripansi-reuse-via-statuslinesanitize-recovery-view-capped-at-8-kib-symlink-banner-before-recovery-view-render)).

### Data Model and Persistence

**In-memory model.** `workflowmodel.WorkflowDoc` is a value-typed mutable struct ([D-2](artifacts/implementation-decision-log.md#d-2-in-memory-workflow-model-lives-in-workflowmodelworkflowdoc-distinct-from-vfile-and-stepsstepfile)). Steps carry a companion `IsClaudeSet bool` to distinguish "new step with no kind chosen yet" from "shell step." Unknown fields encountered on load are recorded in `WorkflowDoc.UnknownFields` (spec D18) and are not written back on save.

**Save path.** Every save — config.json and every dirty companion file — uses `atomicwrite.Write(path, data, mode)` ([D-5](artifacts/implementation-decision-log.md#d-5-atomic-save-helper-lives-in-a-new-internalatomicwrite-package-with-signature-writepath-data-mode-error)). The helper:

1. `realPath, err = filepath.EvalSymlinks(path)` — resolves at save time (spec T1 correction).
2. `realDir = filepath.Dir(realPath)`.
3. `f, err = os.CreateTemp(realDir, filepath.Base(realPath)+".<pid>-<epoch-ns>.tmp")` — stdlib guarantees `O_RDWR|O_CREATE|O_EXCL` and mode `0o600` ([D-5](artifacts/implementation-decision-log.md#d-5-atomic-save-helper-lives-in-a-new-internalatomicwrite-package-with-signature-writepath-data-mode-error)).
4. Write `data`; `f.Sync()` (fsync) before `f.Close()`.
5. `os.Rename(tempPath, realPath)`.
6. On any error after step 3, `os.Remove(tempPath)` before returning the error.
7. EXDEV wrapped so callers can detect via `errors.Is(err, syscall.EXDEV)` and surface the D56 template ([D-19](artifacts/implementation-decision-log.md#d-19-exdev-cross-device-rename-error-surfaced-via-d56-template)).

Save ordering in `workflowio.Save`: companion files first, config.json last ([D-20](artifacts/implementation-decision-log.md#d-20-companion-file-save-ordering--companions-first-config-last)). Orphaned companions on mid-save crash are an accepted limitation documented in the ADR ([D-24](artifacts/implementation-decision-log.md#d-24-orphaned-companion-file-crash-residue-is-an-accepted-limitation-no-detection-in-v1)).

**D41 snapshot.** After `os.Rename` returns, `workflowio.Save` calls `os.Stat(realPath)` and returns `{ModTime, Size}`. `workflowedit.Model.Update` stores this in a `saveSnapshot` field. Subsequent save invocations compare via `time.Time.Equal()` on the monotonic-stripped values ([D-14](artifacts/implementation-decision-log.md#d-14-d41-mtime-snapshot-uses-nanosecond-precision-timetime-from-post-rename-osstat)).

**Crash-era detection.** On File > Open / `--workflow-dir` auto-open, `workflowio.DetectCrashTempFiles(workflowDir)` globs `<workflowDir>/*.*.*.tmp`, parses the `<pid>` token in each name, and classifies via `syscall.Kill(pid, 0)` liveness check ([D-16](artifacts/implementation-decision-log.md#d-16-temp-file-naming-configjsonpid-epoch-nstmp-with-pid-liveness-rule)).

### Runtime Behavior

**Bubble Tea model.** `workflowedit.Model` implements `tea.Model`. `Update` routing order ([D-9](artifacts/implementation-decision-log.md#d-9-update-routing--overlay-first-ordering-with-global-key-intercept-nested-inside)):

```
(1) m.helpOpen        → updateHelpModal(msg)
(2) m.dialog.kind !=
    DialogNone        → updateDialog(msg)
(3) isGlobalKey(msg)  → handleGlobalKey(msg)   // F10, Ctrl+N/O/S/Q only; not Del
(4) otherwise         → updateEditView(msg)    // outline / detail / menu / empty-editor
```

`Del` is **not** in `isGlobalKey`; step removal via `Del` dispatches only inside `updateEditView` when the outline has focus ([D-10](artifacts/implementation-decision-log.md#d-10-del-key-scoped-to-outline-focus-not-a-global-key)).

**Dialog state.** A single `dialogState` slot + `helpOpen bool` + depth-1 `prevFocus` ([D-8](artifacts/implementation-decision-log.md#d-8-dialog-composition-uses-one-dialogstate-slot--helpopen-bool--depth-1-prevfocus)). Fourteen `DialogKind` constants cover every modal the spec documents. `?` sets `helpOpen=true` only when `dialog.kind == DialogFindingsPanel`; for all other kinds it is silently suppressed (UX Round 2 R2-C1). The shortcut footer suppresses the `?  help` hint while non-findings-panel dialogs are active.

**Rendering.** Two `bubbles/viewport` instances — outline (40 cols or 40% minimum 20) and detail pane (remaining width) ([D-12](artifacts/implementation-decision-log.md#d-12-two-independently-scrollable-viewports-with-pointer-side-mouse-routing)). Wheel events route by pointer X. The shortcut footer is rendered by calling `ShortcutLine()` on the currently focused widget ([D-11](artifacts/implementation-decision-log.md#d-11-widget-owned-shortcut-footer-content-via-shortcutline-string)), eliminating the mode-switch pattern from the existing `internal/ui`. The findings panel has its own viewport with preserved scroll state ([D-31](artifacts/implementation-decision-log.md#d-31-findings-panel-is-independently-scrollable-with-preserved-state-across-rebuild)).

**Save flow.** On `Ctrl+S` / `File > Save`:

1. (sync) D41 snapshot compare → if mismatch, open `DialogFileConflict`.
2. (sync) If `saveInProgress` is true, silently consume; return.
3. (sync) Call `workflowvalidate.Validate(deepCopy(m.doc), m.workflowDir, snapshotCompanions(m))` ([D-13](artifacts/implementation-decision-log.md#d-13-save-is-async-via-teacmd-save-in-progress-flag--snapshot-refresh-on-callback)).
4. (sync) Classify findings; on fatal → open `DialogFindingsPanel`; on warn/info only → open acknowledgment dialog; on zero → fall through.
5. (sync) Set `saveInProgress = true`; return a `tea.Cmd` that runs `workflowio.Save(...)` on a goroutine.
6. (async) Goroutine completes; sends `saveCompleteMsg{result, snapshot}` via `tea.Msg` channel.
7. (sync on receipt) Refresh D41 snapshot from `saveCompleteMsg.snapshot`; clear `saveInProgress`; dispatch UI feedback (banner, indicator).

**External editor handoff.** Two-cycle render ([D-26](artifacts/implementation-decision-log.md#d-26-external-editor-handoff-two-step-opening-editor-pre-render-then-execprocess-cmd)):

1. `openEditorMsg` → set `dialog.kind = DialogExternalEditorOpening`; return `tea.Tick(10ms, launchEditorMsg{filePath})`.
2. (10 ms later, "Opening editor…" now on screen) `launchEditorMsg` → call `editorRunner.Run(filePath, exitCallback)`; return its `tea.Cmd`.
3. Bubble Tea runtime calls `ReleaseTerminal`, spawns editor, waits, calls `RestoreTerminal`, delivers `exitCallback(err)`.
4. `exitCallback` inspects exit code: 130 → returns `editorSigintMsg` → `Update` opens quit-confirm dialog ([D-7](artifacts/implementation-decision-log.md#d-7-execcallback-branches-on-exit-code-130--sigint-routes-to-quit-confirm-not-re-read)). Other → returns `editorExitMsg{err}` → `Update` re-reads the file from disk, updates dirty state.

**Create-on-editor-open.** When the user opens the editor on a `promptFile` that does not exist, `workflowio.CreateEmptyCompanion(workflowDir, promptFile)` applies `EvalSymlinks` to the parent directory and the containment check, then creates the file via `os.OpenFile(..., O_CREATE|O_EXCL|O_WRONLY, 0o600)` ([D-21](artifacts/implementation-decision-log.md#d-21-create-on-editor-open-applies-evalsymlinks--containment-before-oscreate)).

**Signal handler.** `runWorkflowBuilder` establishes `ctx, cancel := context.WithCancel(cmd.Context())`; defers `cancel()`. On SIGINT/SIGTERM, the handler calls `program.Send(quitMsg{})` and `cancel()`; it does **not** call `program.Kill()` during the `tea.ExecProcess` window ([D-34](artifacts/implementation-decision-log.md#d-34-signal-handler-does-not-call-programkill-during-teaexecprocess-window)). A hard fallback to `os.Exit(130)` kicks in only after a 10-second grace period.

**Goroutine lifecycle.** Every non-stdlib goroutine spawned by the builder receives `ctx` and selects on `ctx.Done()` ([D-33](artifacts/implementation-decision-log.md#d-33-goroutine-lifecycle-for-the-builder-subcommand-uses-contextcontext-cancellation)). The "terminates with the process" `main.go:204-212` heartbeat pattern is explicitly **not** copied.

### External Interfaces

**CLI surface.** `pr9k workflow [--workflow-dir PATH] [--project-dir PATH]`. No `--iterations`, no other run-specific flags (spec D19). Subcommand registered via `cli.Execute(newSandboxCmd(), newWorkflowCmd())` ([D-36](artifacts/implementation-decision-log.md#d-36-pr9k-workflow-wires-as-a-peer-subcommand-to-pr9k-sandbox-bypasses-startup)).

**Validator.** `internal/validator` gains `func ValidateDoc(doc workflowmodel.WorkflowDoc, workflowDir string, companionFiles map[string][]byte) []Error` ([D-3](artifacts/implementation-decision-log.md#d-3-validator-extension--add-validatedocdoc-workflowdir-companionfiles-alongside-the-existing-validateworkflowdir)). Existing `Validate(workflowDir) []Error` preserved; internally calls `ValidateDoc(doc, workflowDir, nil)` after disk deserialization. `TestValidate_ProductionStepsJSON` unaffected ([D-37](artifacts/implementation-decision-log.md#d-37-production-integration-test-testvalidate_productionstepsjson-must-pass-unchanged)).

**Logger.** `internal/logger` gains a prefix parameter (via new `NewLoggerWithPrefix` or extended `NewLogger`); builder constructs its logger with prefix `"workflow"` producing `workflow-YYYY-MM-DD-HHMMSS.mmm.log` filenames ([D-15](artifacts/implementation-decision-log.md#d-15-log-filename-prefix-workflow--builder-vs-ralph--run)). Session events logged via `log.Log("workflow-builder", line)` with the field-exclusion contract ([D-27](artifacts/implementation-decision-log.md#d-27-session-event-logging-uses-loggerlog-with-fixed-stepname-workflow-builder-and-a-field-exclusion-contract)).

**New dependency.** `github.com/google/shlex` added to `go.mod` for `$VISUAL`/`$EDITOR` word splitting ([D-22](artifacts/implementation-decision-log.md#d-22-visualeditor-word-splitting-via-githubcomgoogleshlex)).

## Decomposition and Sequencing

| # | Work Unit | Delivers | Depends On | Verification |
|---|-----------|----------|------------|--------------|
| 1 | `internal/atomicwrite` + coding standard + `O_TRUNC` audit | `atomicwrite.Write`, `docs/coding-standards/file-writes.md` (four rules [D-17](artifacts/implementation-decision-log.md#d-17-docscoding-standardsfile-writesmd-scope--four-rules)), audit of existing `O_TRUNC` sites (`rawwriter.go`, `iterationlog.go`) — documented as exempt with rationale | — | `wfatomicwrite_test.go` (T1 matrix); doc-integrity tests DI-4, DI-5 ([D-32](artifacts/implementation-decision-log.md#d-32-d38-doc-obligations-extended-with-doc-integrity-test-list)); `make ci` green |
| 2 | CLI subcommand wiring + logger prefix | `cmd/pr9k/workflow.go`, `newWorkflowCmd()`, logger prefix support ([D-15](artifacts/implementation-decision-log.md#d-15-log-filename-prefix-workflow--builder-vs-ralph--run)), `--workflow-dir`/`--project-dir` flags ([D-36](artifacts/implementation-decision-log.md#d-36-pr9k-workflow-wires-as-a-peer-subcommand-to-pr9k-sandbox-bypasses-startup)) | — | `workflow_cmd_test.go` (WG-1 registration, WG-2 no-stale-flags), updated `logger_test.go` `runStampRe` |
| 3 | `internal/workflowmodel` + diff + scaffold | `WorkflowDoc`, `Step`, `EnvEntry`, `StatusLineBlock`, `diff.IsDirty`, `scaffold.Empty`, `scaffold.CopyFromDefault` ([D-2](artifacts/implementation-decision-log.md#d-2-in-memory-workflow-model-lives-in-workflowmodelworkflowdoc-distinct-from-vfile-and-stepsstepfile), [D-40](artifacts/implementation-decision-log.md#d-40-scaffold-placeholder-step-conventions-oq-4)) | — | `wfmodel_test.go` unit tests; input-immutability tests |
| 4 | `internal/workflowio` load + save + detect | `Load`, `Save`, `DetectSymlink`, `DetectReadOnly`, `DetectExternalWorkflow`, `DetectSharedInstall` ([D-43](artifacts/implementation-decision-log.md#d-43-shared-install-detection-uses-syscallstat_tuid-unix-only)), `DetectCrashTempFiles` ([D-16](artifacts/implementation-decision-log.md#d-16-temp-file-naming-configjsonpid-epoch-nstmp-with-pid-liveness-rule)), `CreateEmptyCompanion` ([D-21](artifacts/implementation-decision-log.md#d-21-create-on-editor-open-applies-evalsymlinks--containment-before-oscreate)), companion-first save sequence ([D-20](artifacts/implementation-decision-log.md#d-20-companion-file-save-ordering--companions-first-config-last)) | 1, 3 | `wfsave_test.go` (T1 matrix including symlink, rename-failure, companion rollback, EXDEV, crash-era classification); `wfload_test.go` (parse-error, symlink-first ordering) |
| 5 | Validator hardening (OI-1) + `ValidateDoc` + `workflowvalidate` bridge | `safePromptPath` EvalSymlinks + containment; `validateCommandPath` parallel guard ([D-38](artifacts/implementation-decision-log.md#d-38-oi-1-validator-hardening-lands-in-the-same-pr-as-the-builder)); new exported `ValidateDoc` ([D-3](artifacts/implementation-decision-log.md#d-3-validator-extension--add-validatedocdoc-workflowdir-companionfiles-alongside-the-existing-validateworkflowdir)); `workflowvalidate.Validate` bridge ([D-4](artifacts/implementation-decision-log.md#d-4-workflowvalidate-bridge-is-a-thin-type-conversion-layer)) | 3 | `wfvalidate_test.go` (T3 matrix); new symlink-escape rejection tests for `safePromptPath`/`validateCommandPath`; existing `TestValidate_ProductionStepsJSON` continues to pass unchanged ([D-37](artifacts/implementation-decision-log.md#d-37-production-integration-test-testvalidate_productionstepsjson-must-pass-unchanged)) |
| 6 | `EditorRunner` + production impl + `google/shlex` integration | `EditorRunner` interface ([D-6](artifacts/implementation-decision-log.md#d-6-editorrunner-is-a-one-method-interface-editor-resolution-is-private-to-the-production-impl)); `realEditorRunner` + private `resolveEditor` using `google/shlex` ([D-22](artifacts/implementation-decision-log.md#d-22-visualeditor-word-splitting-via-githubcomgoogleshlex)); `ExecCallback` SIGINT branch ([D-7](artifacts/implementation-decision-log.md#d-7-execcallback-branches-on-exit-code-130--sigint-routes-to-quit-confirm-not-re-read)) | 2 | `wfeditor_test.go` (T2 matrix); async-pattern guard test; `TestResolveEditor_*` for D33 rejection cases |
| 7 | `internal/workflowedit` core: model, dialogs, outline, detail, menu, footer, constants | `workflowedit.Model` implementing `tea.Model`; `dialogState` + 14 `DialogKind` ([D-8](artifacts/implementation-decision-log.md#d-8-dialog-composition-uses-one-dialogstate-slot--helpopen-bool--depth-1-prevfocus)); Update routing ([D-9](artifacts/implementation-decision-log.md#d-9-update-routing--overlay-first-ordering-with-global-key-intercept-nested-inside)); widget-owned footer ([D-11](artifacts/implementation-decision-log.md#d-11-widget-owned-shortcut-footer-content-via-shortcutline-string)); two viewports ([D-12](artifacts/implementation-decision-log.md#d-12-two-independently-scrollable-viewports-with-pointer-side-mouse-routing)); `constants.go` ([D-35](artifacts/implementation-decision-log.md#d-35-builder-constants-file-collects-all-affordance-signifiers)); model suggestions ([D-42](artifacts/implementation-decision-log.md#d-42-scaffold-model-defaults--model-suggestion-list-lives-in-workfloweditmodelsuggestionsgo)) | 3, 4, 5, 6 | `wfmodel_test.go` (TUI 28-mode coverage); `*_render_test.go`; edge-case tests EC-1–EC-12 |
| 8 | Path picker + async tab-completion | `pathpicker.go` with `pathCompletionMsg` async `tea.Cmd` ([D-25](artifacts/implementation-decision-log.md#d-25-filesystem-tab-completion-is-a-custom-minimal-implementation-not-a-new-dependency)); `~` expansion; hidden-file rule | 7 | Unit tests for tab-completion, hidden-file rule, cycling behavior |
| 9 | Findings panel + save flow + validation-feedback integration | Findings panel with viewport + preserved scroll ([D-31](artifacts/implementation-decision-log.md#d-31-findings-panel-is-independently-scrollable-with-preserved-state-across-rebuild)); async save + `saveInProgress` + nanosecond mtime snapshot ([D-13](artifacts/implementation-decision-log.md#d-13-save-is-async-via-teacmd-save-in-progress-flag--snapshot-refresh-on-callback), [D-14](artifacts/implementation-decision-log.md#d-14-d41-mtime-snapshot-uses-nanosecond-precision-timetime-from-post-rename-osstat)) | 4, 5, 7 | Model-update tests for save matrix; findings-panel rebuild-preserves-acknowledgment tests; TUI VI-1–VI-4 edge cases |
| 10 | Session-event logging + shared-install banner + symlink-banner ordering | `log.Log("workflow-builder", line)` at every D39 trigger point per field-exclusion contract ([D-27](artifacts/implementation-decision-log.md#d-27-session-event-logging-uses-loggerlog-with-fixed-stepname-workflow-builder-and-a-field-exclusion-contract)); load-pipeline ordering (symlink detect → banner → parse → recovery view) ([D-23](artifacts/implementation-decision-log.md#d-23-stripansi-reuse-via-statuslinesanitize-recovery-view-capped-at-8-kib-symlink-banner-before-recovery-view-render)); shared-install detection ([D-43](artifacts/implementation-decision-log.md#d-43-shared-install-detection-uses-syscallstat_tuid-unix-only)); log-dir fallback ([D-44](artifacts/implementation-decision-log.md#d-44-log-directory-under-projectdirpr9klogs-external-workflow-case-inherits-the-same-directory)) | 2, 4 | Unit tests on log-line shape (regex pin on excluded fields); integration test: symlink target loads, banner fires first |
| 11 | Doc obligations + doc-integrity tests + ADR + CLAUDE.md updates | `docs/features/workflow-builder.md`; `docs/how-to/using-the-workflow-builder.md`; `docs/how-to/configuring-external-editor-for-workflow-builder.md`; ADR under `docs/adr/`; `docs/coding-standards/file-writes.md` ([D-17](artifacts/implementation-decision-log.md#d-17-docscoding-standardsfile-writesmd-scope--four-rules)); code-package docs for the five new internal packages; `CLAUDE.md` updated with links; `docs/features/cli-configuration.md` updated; `docs/architecture.md` updated; DI-1–DI-8 tests added to `doc_integrity_test.go` ([D-32](artifacts/implementation-decision-log.md#d-32-d38-doc-obligations-extended-with-doc-integrity-test-list)) | 1–10 | `TestDocIntegrity_WorkflowBuilder*` suite; manual read-through of CLAUDE.md section headings |
| 12 | Version bump commit + integration + polish | `internal/version` bumped (patch); PR structured as version-bump-first with `--no-ff` merge ([D-18](artifacts/implementation-decision-log.md#d-18-version-bump-is-the-first-commit-of-the-feature-pr-merged-with---no-ff)); integration smoke with the default bundle | 1–11 | `make ci` green; race detector clean; manual smoke against the bundled default workflow |

## RAID Log

### Risks

| ID | Risk | Likelihood | Severity | Blast Radius | Reversibility | Owner | Mitigation |
|----|------|------------|----------|--------------|---------------|-------|------------|
| R1 | Concurrent saves on rapid `Ctrl+S` produce races on `saveSnapshot` or on-disk temp files | Medium | High (silent data inconsistency) | One workflow bundle | Reversible (next save fixes) | concurrency-analyst | `saveInProgress` flag + post-rename snapshot refresh ([D-13](artifacts/implementation-decision-log.md#d-13-save-is-async-via-teacmd-save-in-progress-flag--snapshot-refresh-on-callback), [D-14](artifacts/implementation-decision-log.md#d-14-d41-mtime-snapshot-uses-nanosecond-precision-timetime-from-post-rename-osstat)); PID-tagged temp filenames ([D-16](artifacts/implementation-decision-log.md#d-16-temp-file-naming-configjsonpid-epoch-nstmp-with-pid-liveness-rule)) |
| R2 | SIGHUP or power loss during save leaves orphaned companion files unreferenced by `config.json` | Low | Low (disk clutter, no data loss) | One workflow bundle | Manual cleanup (user deletes orphan files) | devops-engineer | Accepted limitation in v1; ADR documents ([D-24](artifacts/implementation-decision-log.md#d-24-orphaned-companion-file-crash-residue-is-an-accepted-limitation-no-detection-in-v1)) |
| R3 | Symlink `safePromptPath` escape — OI-1 exploit through builder's editor-open write path | High (pre-OI-1) / Low (post) | Critical (arbitrary file write as user) | Any user-writable file outside the bundle | Closed by OI-1 fix | adversarial-security-analyst | OI-1 lands in same PR; `safePromptPath` + `validateCommandPath` EvalSymlinks + containment ([D-38](artifacts/implementation-decision-log.md#d-38-oi-1-validator-hardening-lands-in-the-same-pr-as-the-builder)); editor refuses escaping paths |
| R4 | EXDEV surfaces cryptically on a target whose real directory is on a different filesystem than the parent of the symlink | Low (only if implementation diverges from same-dir) | Medium (opaque error) | One save attempt | Automatic retry succeeds after user moves target | devops-engineer | Helper always places temp in real target's directory (post-EvalSymlinks); D56 template applied ([D-19](artifacts/implementation-decision-log.md#d-19-exdev-cross-device-rename-error-surfaced-via-d56-template)) |
| R5 | SIGINT during `tea.ExecProcess` window leaves terminal in released-but-not-reclaimed state | Low (with D-34) / High (if naive signal handler) | Critical (terminal unusable until `reset`) | User's terminal session | Reversible (`reset`) | concurrency-analyst | Signal handler uses `program.Send(quitMsg)`, not `program.Kill`; Bubble Tea runtime guarantees `RestoreTerminal` after editor exit ([D-34](artifacts/implementation-decision-log.md#d-34-signal-handler-does-not-call-programkill-during-teaexecprocess-window)); ExecCallback branches on exit 130 ([D-7](artifacts/implementation-decision-log.md#d-7-execcallback-branches-on-exit-code-130--sigint-routes-to-quit-confirm-not-re-read)) |
| R6 | ANSI-injection through parse-error recovery view (OSC 8 hyperlinks, window-title exfiltration) | Medium (attacker-controlled config.json is plausible) | Medium (terminal-local exploit) | One terminal session | Closed by stripANSI + 8 KiB cap | adversarial-security-analyst | Reuse `statusline.Sanitize` + 8 KiB cap + banner-before-recovery-view ordering ([D-23](artifacts/implementation-decision-log.md#d-23-stripansi-reuse-via-statuslinesanitize-recovery-view-capped-at-8-kib-symlink-banner-before-recovery-view-render)) |
| R7 | `containerEnv` secret leak via save-outcome log | Medium (easy for a naive implementation to fall into) | High (credentials in plaintext log) | `.pr9k/logs/` dir | Reversible via log rotation | adversarial-security-analyst | Closed-form event catalog; first-token-only editor binary; explicit exclusion of all workflow content from logs ([D-27](artifacts/implementation-decision-log.md#d-27-session-event-logging-uses-loggerlog-with-fixed-stepname-workflow-builder-and-a-field-exclusion-contract)) |
| R8 | Heartbeat/statusline goroutine leak after `pr9k workflow` subcommand returns | Medium (if main.go pattern copied) | Low (memory + misdirected `program.Send`) | One process, until next run | Reversible (`pr9k` restart) | concurrency-analyst | `context.Context` cancellation on subcommand return ([D-33](artifacts/implementation-decision-log.md#d-33-goroutine-lifecycle-for-the-builder-subcommand-uses-contextcontext-cancellation)) |
| R9 | OQ-4 create-on-editor-open creates a file outside the bundle tree for an attacker-controlled `promptFile` | Medium (malicious config.json) | High (arbitrary file creation) | Any user-writable path | Reversible (user deletes file) | adversarial-security-analyst | EvalSymlinks on parent + containment + `O_EXCL` ([D-21](artifacts/implementation-decision-log.md#d-21-create-on-editor-open-applies-evalsymlinks--containment-before-oscreate)) |

### Assumptions

| ID | Assumption | What Changes If Wrong | Verifier | Status |
|----|------------|-----------------------|----------|--------|
| A1 | `os.CreateTemp` uses `O_EXCL` + mode `0o600` on Go 1.26.2 (stdlib guaranteed) | Must switch to explicit `os.OpenFile(..., O_CREATE\|O_EXCL\|O_WRONLY, 0o600)` | Go stdlib docs verified at plan time; test asserts temp file mode | Verified |
| A2 | APFS and ext4 preserve nanosecond `ModTime` through Go's `os.FileInfo` | D41 conflict detection would miss sub-second external saves | Concurrency-analyst C8 cites stdlib behavior; integration test on macOS verifies | Verified |
| A3 | Bubble Tea v1.3.10 `ExecProcess` calls `ReleaseTerminal` → spawn → `RestoreTerminal` even if the callback returns an error | Terminal left corrupted on editor error | Bubble Tea source `exec.go:103-129` inspected at plan time | Verified |
| A4 | Editor receives SIGINT delivered to foreground process group and exits with code 130 when it handles the signal | Spurious unsaved-changes dialog after SIGINT | POSIX convention; integration test | Verified (POSIX); Windows untested |
| A5 | `google/shlex` correctly handles single-quoted, double-quoted, and backslash-escaped tokens | `$VISUAL` parsing silently fails on common configurations | `google/shlex` is used in production by Docker and others; unit tests pin the behavior | Verified |
| A6 | `syscall.Kill(pid, 0)` is available and non-privileged on all supported platforms | Crash-era temp-file PID-liveness misclassifies | POSIX (macOS, Linux) — available and non-privileged for processes in the same UID | Verified |

### Issues

| ID | Issue | Owner | Next Step |
|----|-------|-------|-----------|
| I1 | D69 inline text in the spec references superseded D54's "two-step Discard confirmation" | spec author (out-of-team) | Fast-follow spec update; implementation follows D7 authoritative rule ([D-41](artifacts/implementation-decision-log.md#d-41-d69-inline-reference-to-superseded-d54-is-an-oi-for-spec-author)) |

### Dependencies

| ID | Dependency | Owner | Status |
|----|------------|-------|--------|
| Dep1 | `github.com/google/shlex` added to `go.mod` | software-architect | Pending — added in WU-6 |
| Dep2 | OI-1 validator hardening (`safePromptPath`, `validateCommandPath`) | adversarial-security-analyst + software-architect | Pending — delivered in WU-5 ([D-38](artifacts/implementation-decision-log.md#d-38-oi-1-validator-hardening-lands-in-the-same-pr-as-the-builder)) |
| Dep3 | Existing `internal/logger` extension for filename prefix | devops-engineer | Pending — delivered in WU-2 |
| Dep4 | `docs/coding-standards/file-writes.md` new file | devops-engineer | Pending — delivered in WU-1 alongside the helper |

## Testing Strategy

**Observable behaviors to test:** Every mode transition in the 28-entry TUI mode coverage (test plan §5); every alternate flow (spec "Alternate Flows"); every edge-case row in the spec's Edge Cases table; all T1/T2/T3 matrices (test plan §§2-4); all doc-integrity assertions ([D-32](artifacts/implementation-decision-log.md#d-32-d38-doc-obligations-extended-with-doc-integrity-test-list)); the production-config integration test `TestValidate_ProductionStepsJSON` ([D-37](artifacts/implementation-decision-log.md#d-37-production-integration-test-testvalidate_productionstepsjson-must-pass-unchanged)).

**Test doubles posture:** Two DI seams — `workflowio.SaveFS` interface (inject `fakeFS` capturing per-method calls with own counters per `docs/coding-standards/testing.md`) and `workflowedit.EditorRunner` interface ([D-6](artifacts/implementation-decision-log.md#d-6-editorrunner-is-a-one-method-interface-editor-resolution-is-private-to-the-production-impl)) allowing a test `tea.Cmd` that writes known bytes and returns without touching TTY. Every fake method captures calls (no silent no-ops); every fake has per-method counters; fakes shared across goroutines carry `sync.Mutex` even if single-goroutine today.

**Test files and coverage:**

- `src/internal/atomicwrite/write_test.go` — T1 matrix: first save, replaces existing, symlink preservation (`TestSave_SymlinkedTarget_SymlinkPreserved` per [D-5](artifacts/implementation-decision-log.md#d-5-atomic-save-helper-lives-in-a-new-internalatomicwrite-package-with-signature-writepath-data-mode-error)), rename failure rolls back temp, temp-creation failure, fsync-called-before-rename ordering, cross-device rename error taxonomy ([D-19](artifacts/implementation-decision-log.md#d-19-exdev-cross-device-rename-error-surfaced-via-d56-template)). `fakeFS` following the `sandbox_create_test.go` pattern.
- `src/internal/workflowio/save_test.go` — companion-files-written-before-config ordering ([D-20](artifacts/implementation-decision-log.md#d-20-companion-file-save-ordering--companions-first-config-last)); no-op save (`TestSave_NoOp_FileNotRewritten`); conflict-dialog precondition (mtime mismatch at save).
- `src/internal/workflowio/load_test.go` — parse error → recovery view; symlink-detect-before-parse ordering ([D-23](artifacts/implementation-decision-log.md#d-23-stripansi-reuse-via-statuslinesanitize-recovery-view-capped-at-8-kib-symlink-banner-before-recovery-view-render)); crash-era temp classification with PID-liveness ([D-16](artifacts/implementation-decision-log.md#d-16-temp-file-naming-configjsonpid-epoch-nstmp-with-pid-liveness-rule)).
- `src/internal/workflowmodel/diff_test.go`, `scaffold_test.go` — dirty-state tracking, scaffold defaults ([D-40](artifacts/implementation-decision-log.md#d-40-scaffold-placeholder-step-conventions-oq-4)).
- `src/internal/workflowvalidate/validate_test.go` — T3 matrix: `ValidateDoc` equivalence with `Validate`; in-memory companion map handling; does-not-touch-on-disk assertion; empty-scaffold `{"step-1.md": {}}` pass.
- `src/internal/validator/validator_test.go` — OI-1 hardening: symlink-escape rejection for both `safePromptPath` and `validateCommandPath` ([D-38](artifacts/implementation-decision-log.md#d-38-oi-1-validator-hardening-lands-in-the-same-pr-as-the-builder)).
- `src/cmd/pr9k/workflow_cmd_test.go` — cobra registration (WG-1); absent `--iterations` flag (WG-2); editor resolution via `google/shlex` (single-quoted, double-quoted, space-in-path error).
- `src/cmd/pr9k/editor_test.go` — `resolveEditor` D33 rejection; `ExecCallback` exit-code-130 branching ([D-7](artifacts/implementation-decision-log.md#d-7-execcallback-branches-on-exit-code-130--sigint-routes-to-quit-confirm-not-re-read)); not-configured dialog path.
- `src/internal/workflowedit/model_test.go` — 28 TUI mode-coverage entries; Update routing order with dialog/help overlay combinations ([D-9](artifacts/implementation-decision-log.md#d-9-update-routing--overlay-first-ordering-with-global-key-intercept-nested-inside)); Del scoped to outline focus ([D-10](artifacts/implementation-decision-log.md#d-10-del-key-scoped-to-outline-focus-not-a-global-key)); save flow state machine ([D-13](artifacts/implementation-decision-log.md#d-13-save-is-async-via-teacmd-save-in-progress-flag--snapshot-refresh-on-callback)); saveInProgress guards double-save; findings-panel rebuild preserves acknowledgment; the 12 edge-case rows EC-1–EC-12.
- `src/internal/workflowedit/outline_render_test.go`, `detail_pane_render_test.go`, `menu_bar_render_test.go`, `dialogs_render_test.go`, `findings_panel_test.go`.
- `src/internal/workflowedit/pathpicker_test.go` — tab-complete async pattern; hidden-file rule; `~` expansion; empty-match cycling.
- `src/cmd/pr9k/doc_integrity_test.go` — DI-1 through DI-8 per [D-32](artifacts/implementation-decision-log.md#d-32-d38-doc-obligations-extended-with-doc-integrity-test-list).
- `src/cmd/pr9k/bundle_integration_test.go` — smoke that the bundled default workflow opens cleanly in the builder.

**Edge cases requiring coverage:** P0 edge cases from `/tmp/wb-edge-findings.md` (EXDEV [AS-1/PH-1], orphaned companions [AS-4] — as regression-probe unit test for companion ordering, `$VISUAL`-with-space [EE-1], terminal-height-1 [TUI-1]); 22 P1 edge cases per test plan §6 and edge-case findings section; concurrency C1–C8 each with a targeted test per `/tmp/wb-concurrency-findings.md`.

**Test levels:** Unit (per-package `_test.go` files per `docs/coding-standards/testing.md` conventions). Integration (`bundle_integration_test.go` — round-trip against `workflow/config.json`). The spec explicitly does not require end-to-end tests against a real TTY ([D-6](artifacts/implementation-decision-log.md#d-6-editorrunner-is-a-one-method-interface-editor-resolution-is-private-to-the-production-impl); `docs/coding-standards/testing.md` "do not test assembly-only code" pattern). `make ci` runs `go test -race ./...` ([D41-b](artifacts/decision-log.md#d41-b-test-strategy-for-t1-t2-and-tui-modes)).

## Security Posture

The builder adds seven concrete mitigations to threats the specialists identified, plus one validator hardening already scoped:

1. **Path traversal via symlinks (OI-1, Finding 5, Finding 3).** `safePromptPath` and `validateCommandPath` add `filepath.EvalSymlinks` before the containment check ([D-38](artifacts/implementation-decision-log.md#d-38-oi-1-validator-hardening-lands-in-the-same-pr-as-the-builder)). Pre-copy integrity check and the copy operation each apply the guard per-open, closing the TOCTOU window ([D-39](artifacts/implementation-decision-log.md#d-39-pre-copy-integrity-check-and-the-copy-operation-each-apply-evalsymlinks--containment-per-open)).

2. **Create-on-editor-open containment (NEW-1).** When the builder creates an empty companion file for a not-yet-existing `promptFile`, it applies EvalSymlinks on the parent directory + containment check + `O_EXCL|0o600` create ([D-21](artifacts/implementation-decision-log.md#d-21-create-on-editor-open-applies-evalsymlinks--containment-before-oscreate)).

3. **ANSI injection in recovery view (Finding 8).** Raw bytes pass through `statusline.Sanitize` before render; display capped at 8 KiB; symlink banner renders before recovery view in the load pipeline ([D-23](artifacts/implementation-decision-log.md#d-23-stripansi-reuse-via-statuslinesanitize-recovery-view-capped-at-8-kib-symlink-banner-before-recovery-view-render)).

4. **Temp-file race + permission (Finding 4).** `atomicwrite.Write` uses `os.CreateTemp` (stdlib `O_EXCL` + `0o600`) followed by `fsync` before rename ([D-5](artifacts/implementation-decision-log.md#d-5-atomic-save-helper-lives-in-a-new-internalatomicwrite-package-with-signature-writepath-data-mode-error)).

5. **`$VISUAL` word-splitting (Finding 1).** `github.com/google/shlex` ([D-22](artifacts/implementation-decision-log.md#d-22-visualeditor-word-splitting-via-githubcomgoogleshlex)), with explicit error message when an unquoted path contains a space.

6. **SIGINT during editor window (Finding 7).** `ExecCallback` branches on exit code 130 and routes to quit-confirm, not re-read ([D-7](artifacts/implementation-decision-log.md#d-7-execcallback-branches-on-exit-code-130--sigint-routes-to-quit-confirm-not-re-read)). Signal handler refuses `program.Kill` during the ExecProcess window ([D-34](artifacts/implementation-decision-log.md#d-34-signal-handler-does-not-call-programkill-during-teaexecprocess-window)).

7. **Session-event logging field exclusion (Findings 6, 9).** Logger contract explicitly excludes every `containerEnv` value, every `env` entry value, every prompt-file content, and the full `$VISUAL` argument list — only the editor binary's first token is logged ([D-27](artifacts/implementation-decision-log.md#d-27-session-event-logging-uses-loggerlog-with-fixed-stepname-workflow-builder-and-a-field-exclusion-contract)).

**Accepted residual (Finding 2 content exposure):** A user who mis-autocompletes a path to a private key in the path picker, then enters the recovery view, sees up to 8 KiB of ANSI-stripped raw bytes. This is sufficient to contain a complete SSH key. Security Round 2 documents this as accepted residual given the 8 KiB cap is the defense and a content-type filter is out of scope for v1. The how-to guide calls this out.

## Operational Readiness

- **Observability:** Session-event lines in `.pr9k/logs/workflow-<timestamp>.log` at session start, save outcomes, editor invocations (binary + exit + duration), quit events ([D-15](artifacts/implementation-decision-log.md#d-15-log-filename-prefix-workflow--builder-vs-ralph--run), [D-27](artifacts/implementation-decision-log.md#d-27-session-event-logging-uses-loggerlog-with-fixed-stepname-workflow-builder-and-a-field-exclusion-contract)). The `workflow-` prefix distinguishes these from run loop's `ralph-` logs. Log-directory fallback on `projectDir` resolution failure lands in `os.UserConfigDir()+"/.pr9k/logs/"` ([D-44](artifacts/implementation-decision-log.md#d-44-log-directory-under-projectdirpr9klogs-external-workflow-case-inherits-the-same-directory)).
- **SLO impact:** None. The builder is a local interactive dev tool with no SLO touchpoints. "Responsiveness" is bounded by Bubble Tea's event loop; file I/O is always in `tea.Cmd` closures per concurrency standard.
- **Feature flag:** None. Additive subcommand; `pr9k workflow` does not alter any existing behavior. Versioning discipline is the rollout control ([D-18](artifacts/implementation-decision-log.md#d-18-version-bump-is-the-first-commit-of-the-feature-pr-merged-with---no-ff)).
- **Rollout:** Standard single-PR merge against `main`. PR structured as version-bump first commit, then WU-1 through WU-12 commits; merged with `--no-ff` to preserve the version-bump commit boundary ([D-18](artifacts/implementation-decision-log.md#d-18-version-bump-is-the-first-commit-of-the-feature-pr-merged-with---no-ff)).
- **Rollback:** Revert the PR merge commit. No data migration; no state carried forward between `pr9k workflow` sessions beyond the workflow files the user already owns. Previous `pr9k` version continues to read any `config.json` the builder produced (schema is unchanged).
- **Cost and scale:** Zero runtime cost (local binary). No cloud resources, no network calls.

## Definition of Done

- [ ] `pr9k workflow` subcommand available and listed under `pr9k --help`; `--workflow-dir` and `--project-dir` flags present; `--iterations` not present ([D-36](artifacts/implementation-decision-log.md#d-36-pr9k-workflow-wires-as-a-peer-subcommand-to-pr9k-sandbox-bypasses-startup)).
- [ ] `make ci` passes: `go test -race ./...`, `golangci-lint`, `gofmt`, `go vet`, `govulncheck`, `go mod tidy`, `go build`.
- [ ] `TestValidate_ProductionStepsJSON` continues to pass unchanged ([D-37](artifacts/implementation-decision-log.md#d-37-production-integration-test-testvalidate_productionstepsjson-must-pass-unchanged)).
- [ ] `bundle_integration_test.go` smoke passes: default bundle loads, validates, and saves via the builder without fatal findings.
- [ ] Doc-integrity tests DI-1 through DI-8 pass ([D-32](artifacts/implementation-decision-log.md#d-32-d38-doc-obligations-extended-with-doc-integrity-test-list)).
- [ ] All new doc files present and linked from `CLAUDE.md`: `docs/features/workflow-builder.md`, `docs/how-to/using-the-workflow-builder.md`, `docs/how-to/configuring-external-editor-for-workflow-builder.md`, `docs/adr/<datestamp>-workflow-builder-save-durability.md`, `docs/coding-standards/file-writes.md` ([D-17](artifacts/implementation-decision-log.md#d-17-docscoding-standardsfile-writesmd-scope--four-rules)), five code-package docs (`atomicwrite`, `workflowmodel`, `workflowio`, `workflowvalidate`, `workflowedit`). `docs/features/cli-configuration.md` updated with the new subcommand. `docs/architecture.md` updated if any top-level package structure is reflected.
- [ ] OI-1 validator hardening present in `src/internal/validator/validator.go` with rejection tests ([D-38](artifacts/implementation-decision-log.md#d-38-oi-1-validator-hardening-lands-in-the-same-pr-as-the-builder)).
- [ ] Version bump lands as the first commit of the feature PR; PR merged with `--no-ff` ([D-18](artifacts/implementation-decision-log.md#d-18-version-bump-is-the-first-commit-of-the-feature-pr-merged-with---no-ff)).
- [ ] Race detector clean on the full test suite.
- [ ] Post-ship owner named: the `software-architect` and `devops-engineer` specialists jointly own post-merge followup for `internal/atomicwrite` call-site migrations (tracked deferral if not in this PR).

## Specialist Handoffs for Implementation

- **`adversarial-security-analyst`** — dispatch before merge for a final pass verifying all seven security mitigations (paths, create-on-editor, ANSI strip, temp perms, shlex, SIGINT branch, logger exclusion) are present with tests; needs the implemented diff.
- **`test-engineer`** — dispatch after WU-9 completes to confirm the 28-mode coverage table has a corresponding test case per entry; needs `wfmodel_test.go` final file.
- **`concurrency-analyst`** — dispatch if the race detector surfaces any flake; needs the failing test plus the Update method's state at that point.
- **`user-experience-designer`** — dispatch after WU-7 for a final shortcut-footer walkthrough against the nine focus-context states; needs running builder or screenshots.
- **`devops-engineer`** — dispatch for the `make ci` sign-off and to verify the version-bump-first PR structure before merge; needs the final PR URL.
- **`software-architect`** — dispatch if WU-5's `ValidateDoc` threading proves more invasive than architect Round 2 audited (fallback to approach b decision point).
- **`gap-analyzer`** — dispatch after WU-11 to confirm every Edge Cases row and every spec "Alternate Flow" has a code path; needs the completed implementation.

## Open Items

- **OI-1 (spec):** Validator `safePromptPath` symlink-containment hardening (spec open item).
  - **Resolves when:** WU-5 lands; `safePromptPath` and `validateCommandPath` apply `EvalSymlinks` + containment; rejection tests pass ([D-38](artifacts/implementation-decision-log.md#d-38-oi-1-validator-hardening-lands-in-the-same-pr-as-the-builder)).
  - **Blocks implementation:** No — the mitigation is scheduled into the same PR as the builder itself; the implementation plan commits to both landing together.

- **OI-2 (spec text):** D69 step-(1) contains a stale reference to superseded D54's "two-step Discard confirmation" ([D-41](artifacts/implementation-decision-log.md#d-41-d69-inline-reference-to-superseded-d54-is-an-oi-for-spec-author)).
  - **Resolves when:** The spec author updates D69 to cite D7 and remove "two-step confirmation" language.
  - **Blocks implementation:** No — the implementation follows D7's authoritative single-step flow; D69's inline text is a documentation inconsistency only.

## Summary

- **Outcome delivered:** Interactive `pr9k workflow` TUI subcommand for authoring and editing workflow bundles, with durable save, in-memory validation, and security-hardened external-editor handoff.
- **Team size:** 9 specialists — see [artifacts/implementation-iteration-history.md](artifacts/implementation-iteration-history.md).
- **Rounds of facilitation:** 2 — see [artifacts/implementation-iteration-history.md](artifacts/implementation-iteration-history.md).
- **Decisions committed:** 44 — see [artifacts/implementation-decision-log.md](artifacts/implementation-decision-log.md).
- **Decisions settled by evidence:** 43 — see [artifacts/implementation-decision-log.md](artifacts/implementation-decision-log.md).
- **Decisions settled by junior-developer reframing:** 1 (D-41, D69 stale-reference identified via JrF-10).
- **Decisions settled by user input:** 0 (all OQ resolutions were accepted under auto-mode per evidence-grounded PM recommendations).
- **Rejected alternatives recorded:** 65 — see [artifacts/implementation-decision-log.md](artifacts/implementation-decision-log.md).
- **Open items remaining:** 2 — OI-1 (validator hardening, scheduled into this PR) and OI-2 (spec text correction, non-blocking).
- **Recommendation:** **Ship as planned.** All 44 decisions have specialist owners and revisit criteria; no blocking open questions remain; the security residual (Finding 2 content exposure under 8 KiB) is explicitly accepted and documented.

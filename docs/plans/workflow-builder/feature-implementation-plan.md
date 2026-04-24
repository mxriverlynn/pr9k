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
        write.go         — EvalSymlinks-with-ENOENT-walkback + O_EXCL + 0o600 + fsync + rename + parent-dir fsync

    ansi/                — StripAll: strict ANSI scrubber (including OSC 8) (D-45)
        strip.go         — new canonical stripper for untrusted bytes
```

Wiring: `main.go` registers both subcommands in one `cli.Execute` call — `cli.Execute(newSandboxCmd(), newWorkflowCmd())` — matching the existing sandbox pattern ([D-36](artifacts/implementation-decision-log.md#d-36-pr9k-workflow-wires-as-a-peer-subcommand-to-pr9k-sandbox-bypasses-startup)). The `workflow` subcommand does **not** call `startup()`; it owns its own logger creation, `workflowDir`/`projectDir` resolution (via reused `internal/cli` helpers), and goroutine lifecycle.

Dependency layering (inner rings have no outward dependencies):

```
atomicwrite   ansi   workflowmodel        (inner — zero deps on new packages)
       \       \         /     |
        \       \      /       |
         workflowio         workflowvalidate
                  \            /
                   workflowedit
                         |
                  cmd/pr9k/workflow.go
```

`workflowio` imports `ansi` for the recovery-view stripper ([F-94](artifacts/review-findings.md#f94-statuslinesanitize-preserves-osc-8-hyperlinks-plan-blocking)). `workflowedit` does not import `ansi` directly — it receives already-stripped content from `workflowio.Load`.

`workflowedit` imports `workflowmodel`, `workflowio`, `workflowvalidate`. `workflowio` imports `workflowmodel`, `atomicwrite`, and the new `internal/ansi` package. `workflowvalidate` imports `workflowmodel` and `internal/validator`. `internal/ansi.StripAll` is introduced because `statusline.Sanitize` deliberately preserves OSC 8 hyperlinks for the status-line use case and is therefore unsafe as the sole defense against a malicious `config.json` rendered in the recovery view ([F-94](artifacts/review-findings.md#f94-statuslinesanitize-preserves-osc-8-hyperlinks-plan-blocking)); `workflowedit` never imports `statusline` ([F-99](artifacts/review-findings.md#f99-workflowedit--internalstatusline-coupling-architecture)).

### Data Model and Persistence

**In-memory model.** `workflowmodel.WorkflowDoc` is a value-typed mutable struct ([D-2](artifacts/implementation-decision-log.md#d-2-in-memory-workflow-model-lives-in-workflowmodelworkflowdoc-distinct-from-vfile-and-stepsstepfile)). Steps carry a companion `IsClaudeSet bool` to distinguish "new step with no kind chosen yet" from "shell step." Unknown fields encountered on load are recorded in `WorkflowDoc.UnknownFields` (spec D18) and are not written back on save.

**Save path.** Every save — config.json and every dirty companion file — uses `atomicwrite.Write(path, data, mode)` ([D-5](artifacts/implementation-decision-log.md#d-5-atomic-save-helper-lives-in-a-new-internalatomicwrite-package-with-signature-writepath-data-mode-error)). The helper:

1. Resolve the real target via `filepath.EvalSymlinks(path)`. On `ENOENT` — expected for every first-save — walk back to the lowest existing ancestor directory, `EvalSymlinks` that, then append the unresolved suffix. `realPath` is the final resolved path (file may not exist yet); `realDir` is the resolved existing parent directory, guaranteed same-filesystem as the rename target ([F-95](artifacts/review-findings.md#f95-filepathevalsymlinks-enoent-on-non-existent-paths-plan-blocking)).
2. `tempPath = filepath.Join(realDir, fmt.Sprintf("%s.%d-%d.tmp", filepath.Base(realPath), os.Getpid(), time.Now().UnixNano()))`. Explicit naming — NOT `os.CreateTemp`, whose random-suffix convention is incompatible with the D-16 crash-era glob ([F-110](artifacts/review-findings.md#f110-temp-file-naming-scheme-inconsistent-across-decisions)).
3. `f, err = os.OpenFile(tempPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)` — explicit flags and mode.
4. Write `data`; `f.Sync()` (file fsync) before `f.Close()`.
5. `os.Rename(tempPath, realPath)`; then open `realDir` and `Sync()` it (parent-directory fsync — best-effort; POSIX requires this for the rename entry to survive power loss) ([F-108](artifacts/review-findings.md#f108-parent-directory-fsync-missing-after-rename)).
6. On any error after step 3, `os.Remove(tempPath)` before returning the error.
7. EXDEV detected via `errors.Is(err, syscall.EXDEV)` inside `workflowio.Save`, which sets `SaveResult.Kind = SaveErrorEXDEV` — the TUI never imports `syscall` ([D-47](artifacts/implementation-decision-log.md#d-47-saveresult-typed-error-kind-enum), [F-103](artifacts/review-findings.md#f103-saveresultsaveerrorkind-typed-enum-architecture)).

Save ordering in `workflowio.Save`: companion files first, config.json last ([D-20](artifacts/implementation-decision-log.md#d-20-companion-file-save-ordering--companions-first-config-last)). Orphaned companions on mid-save crash are an accepted limitation documented in the ADR ([D-24](artifacts/implementation-decision-log.md#d-24-orphaned-companion-file-crash-residue-is-an-accepted-limitation-no-detection-in-v1)).

**D41 snapshot.** After `os.Rename` returns, `workflowio.Save` calls `os.Stat(realPath)` and returns `SaveSnapshot{ModTime, Size}`. `workflowedit.Model.Update` stores this in a `saveSnapshot *SaveSnapshot` field — nil-by-default, set on first successful save ([F-98](artifacts/review-findings.md#f98-savesnapshot-not-reset-on-session-transition-plan-blocking)). Session transitions (File > Open, File > New) reset `saveSnapshot` to `nil`. Save flow step 1: if `saveSnapshot == nil`, the conflict check is skipped (no prior save this session, no conflict possible). Subsequent comparisons use `time.Time.Equal()` ([D-14](artifacts/implementation-decision-log.md#d-14-d41-mtime-snapshot-uses-nanosecond-precision-timetime-from-post-rename-osstat)).

**Crash-era detection.** On File > Open / `--workflow-dir` auto-open, `workflowio.DetectCrashTempFiles(workflowDir)` globs `<basename>.*.tmp` per target file (narrower than `*.*.*.tmp` to avoid matching unrelated temp files in the directory) ([F-110](artifacts/review-findings.md#f110-temp-file-naming-scheme-inconsistent-across-decisions)), parses the `<pid>` token, and classifies via `syscall.Kill(pid, 0)` liveness check ([D-16](artifacts/implementation-decision-log.md#d-16-temp-file-naming-configjsonpid-epoch-nstmp-with-pid-liveness-rule)). Known limitation: PID reuse produces false negatives — a long-dead crash-era temp whose PID has been reused appears "active." Accepted: the temp file resurfaces once the reusing process ends ([F-117](artifacts/review-findings.md#f117-pid-reuse-in-crash-era-detection--accepted-limitation)).

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

**Save flow.** Three-stage state machine — `idle` → `validating` → `saving` → `idle` ([F-105](artifacts/review-findings.md#f105-synchronous-validation-violates-concurrency-standard-on-nfsfuse), [F-97](artifacts/review-findings.md#f97-ctrlq-during-save-in-flight-discards-savecompletemsg-plan-blocking)):

1. (sync) On `Ctrl+S` / `File > Save`: if `validateInProgress || saveInProgress`, silently consume the keystroke.
2. (sync) D41 snapshot compare: if `saveSnapshot != nil` and mismatch, open `DialogFileConflict` (overwrite / reload / cancel); return.
3. (sync) Set `validateInProgress = true`. Return a `tea.Cmd` running `workflowvalidate.Validate(deepCopy(m.doc), m.workflowDir, snapshotCompanions(m))` on a goroutine. Deep copy is also required for T3 correctness. Validation is async because post-OI-1 `safePromptPath` performs `os.Lstat` per companion file — on NFS/FUSE a 100-step workflow otherwise freezes the Update goroutine for seconds.
4. (async) Validation goroutine completes; sends `validateCompleteMsg{findings}`.
5. (sync on receipt) Clear `validateInProgress`. Classify findings; on fatal → open `DialogFindingsPanel`; on warn/info only → open acknowledgment dialog; on zero → fall through to step 6.
6. (sync) Set `saveInProgress = true`. Return a `tea.Cmd` running `workflowio.Save(...)` on a goroutine.
7. (async) Save goroutine completes atomically (uninterruptible by design — the rename must finish or roll back cleanly); sends `saveCompleteMsg{result SaveResult, snapshot *SaveSnapshot}`.
8. (sync on receipt) Clear `saveInProgress`. If `result.Kind != SaveErrorNone`, open the appropriate error dialog via D56 template based on `Kind`. If success, set `saveSnapshot = snapshot`, fire the "Saved at HH:MM:SS" banner, clear the unsaved indicator.
9. (sync) If `pendingQuit` is set, re-trigger the quit flow now that save feedback has been delivered ([F-97](artifacts/review-findings.md#f97-ctrlq-during-save-in-flight-discards-savecompletemsg-plan-blocking)).

**Quit interaction with save-in-flight.** `handleGlobalKey` on `Ctrl+Q`: if `saveInProgress || validateInProgress`, set `pendingQuit = true` and open `DialogSaveInProgress` ("Save in progress — please wait"). On `saveCompleteMsg` with `pendingQuit == true` (step 9 above), re-route to `handleGlobalKey(Ctrl+Q)` which now sees `saveInProgress == false` and proceeds normally — QuitConfirm (if save succeeded, clean exit) or unsaved-changes dialog (if save failed and left dirty state). Invariant: every save produces user-visible feedback before process exit.

**External editor handoff.** Two-cycle render ([D-26](artifacts/implementation-decision-log.md#d-26-external-editor-handoff-two-step-opening-editor-pre-render-then-execprocess-cmd)):

1. `openEditorMsg` → set `dialog.kind = DialogExternalEditorOpening`; return `tea.Tick(10ms, launchEditorMsg{filePath})`.
2. (10 ms later, "Opening editor…" now on screen) `launchEditorMsg` → call `editorRunner.Run(filePath, exitCallback)`; return its `tea.Cmd`.
3. Bubble Tea runtime calls `ReleaseTerminal`, spawns editor, waits, calls `RestoreTerminal`, delivers `exitCallback(err)`.
4. `exitCallback` performs a **three-way type switch** on the error ([F-107](artifacts/review-findings.md#f107-execcallback-restoreterminal-failure-branch-unspecified)):
   - `err == nil` — editor exited 0 → `editorExitMsg{ok: true}` → `Update` re-reads file, updates dirty state.
   - `err` is `*exec.ExitError` with code 130 → `editorSigintMsg` → `Update` opens quit-confirm dialog ([D-7](artifacts/implementation-decision-log.md#d-7-execcallback-branches-on-exit-code-130--sigint-routes-to-quit-confirm-not-re-read)).
   - `err` is `*exec.ExitError` with other code → `editorExitMsg{ok: false, code}` → `Update` re-reads file (editor may have written partial content) and surfaces a non-blocking notice.
   - `err` is any other type → `editorRestoreFailedMsg{err}` → `Update` opens `DialogError` with the D56 template: "The terminal may be degraded — if the display is garbled, run `reset` in your shell." The builder does NOT re-read the file on this branch — the file state is intact, but the terminal is not.

**Create-on-editor-open.** When the user opens the editor on a `promptFile` that does not exist, `workflowio.CreateEmptyCompanion(workflowDir, promptFile)`:

1. Apply `EvalSymlinks` with ENOENT walkback ([F-95](artifacts/review-findings.md#f95-filepathevalsymlinks-enoent-on-non-existent-paths-plan-blocking)) — resolves to a real absolute path plus an unresolved suffix.
2. Check containment: resolved parent must be `HasPrefix(resolvedWorkflowDir)`.
3. `os.MkdirAll(resolvedParentDir, 0o700)` to create any missing intermediate directories (e.g., `prompts/` on a brand-new workflow), inheriting the containment check.
4. Verify the final target path does not already exist and would not resolve to a non-regular file.
5. `os.OpenFile(..., O_CREATE|O_EXCL|O_WRONLY, 0o600)` to create the empty file atomically ([D-21](artifacts/implementation-decision-log.md#d-21-create-on-editor-open-applies-evalsymlinks--containment-before-oscreate)).

**Regular-file check on load and create.** `workflowio.Load` and `CreateEmptyCompanion` verify the EvalSymlinks-resolved target has `FileInfo.Mode().IsRegular()`. FIFOs, sockets, block/char devices, and directories are rejected with the D56 template — the check prevents opening the editor on a FIFO (which would block indefinitely waiting for a writer) ([F-109](artifacts/review-findings.md#f109-fifos--named-pipes-in-bundle-pass-containment-block-editor)).

**Signal handler.** `runWorkflowBuilder` establishes `ctx, cancel := context.WithCancel(cmd.Context())`; defers `cancel()`. On SIGINT/SIGTERM, the handler calls `program.Send(quitMsg{})` and `cancel()`; it **unconditionally** does not call `program.Kill()` for 10 seconds (no ExecProcess-state tracking; the protection is passive) ([D-34](artifacts/implementation-decision-log.md#d-34-signal-handler-does-not-call-programkill-during-teaexecprocess-window), [F-111](artifacts/review-findings.md#f111-signal-handler-10-second-fallback-rationale-and-dod-gap)). A hard fallback to `os.Exit(130)` kicks in only after the 10-second grace period. Rationale: 10s accommodates slow-starting editors (vim with heavy init, VS Code with plugin load on spinning disk) that may be mid-launch inside the ExecProcess window; the existing main-loop 2s is insufficient for cold VS Code. Trade-off: a stuck builder at edit-view level (no ExecProcess active) also waits 10s before OS-forced exit. Accepted given editor launch is the common interactive case and Bubble Tea `quitMsg` usually responds within a frame.

**Goroutine lifecycle.** Every non-stdlib goroutine spawned by the builder receives `ctx` and selects on `ctx.Done()` ([D-33](artifacts/implementation-decision-log.md#d-33-goroutine-lifecycle-for-the-builder-subcommand-uses-contextcontext-cancellation)). The "terminates with the process" `main.go:204-212` heartbeat pattern is explicitly **not** copied.

### External Interfaces

**CLI surface.** `pr9k workflow [--workflow-dir PATH] [--project-dir PATH]`. No `--iterations`, no other run-specific flags (spec D19). Subcommand registered via `cli.Execute(newSandboxCmd(), newWorkflowCmd())` ([D-36](artifacts/implementation-decision-log.md#d-36-pr9k-workflow-wires-as-a-peer-subcommand-to-pr9k-sandbox-bypasses-startup)).

**Validator.** `internal/validator` gains `func ValidateDoc(doc workflowmodel.WorkflowDoc, workflowDir string, companionFiles map[string][]byte) []Error` ([D-3](artifacts/implementation-decision-log.md#d-3-validator-extension--add-validatedocdoc-workflowdir-companionfiles-alongside-the-existing-validateworkflowdir)). Existing `Validate(workflowDir) []Error` preserved; internally calls `ValidateDoc(doc, workflowDir, nil)` after disk deserialization.

**OI-1 containment precision.** `safePromptPath` and the new `validateCommandPath` guard apply `EvalSymlinks` to BOTH the candidate path AND the bundle root, then check `HasPrefix(resolvedCandidate, resolvedBundleRoot)`. A directory-level symlink that keeps content INSIDE the resolved bundle passes (e.g., `<tempdir>/prompts -> <repo>/workflow/prompts` resolves to `<repo>/workflow/prompts/...`; resolved bundle root is `<repo>/workflow`; check passes). A symlink that ESCAPES the bundle fails. This preserves the existing `TestValidate_ProductionStepsJSON` behavior — the test's `assembleWorkflowDir` uses directory-level symlinks and must continue to work ([F-96](artifacts/review-findings.md#f96-testvalidate_productionstepsjson-breaks-under-oi-1-plan-blocking)). `TestValidate_ProductionStepsJSON` unaffected ([D-37](artifacts/implementation-decision-log.md#d-37-production-integration-test-testvalidate_productionstepsjson-must-pass-unchanged)).

**Logger.** `internal/logger` gains a new exported `NewLoggerWithPrefix(projectDir, prefix string) (*Logger, error)`. The existing `NewLogger(projectDir)` is preserved unchanged and internally calls `NewLoggerWithPrefix(projectDir, "ralph")` — no existing call site changes ([F-104](artifacts/review-findings.md#f104-newloggerwithprefix-api-shape-uncommitted)). The builder constructs its logger with prefix `"workflow"` producing `workflow-YYYY-MM-DD-HHMMSS.mmm.log` filenames ([D-15](artifacts/implementation-decision-log.md#d-15-log-filename-prefix-workflow--builder-vs-ralph--run)). Session events logged via `log.Log("workflow-builder", line)` with the field-exclusion contract ([D-27](artifacts/implementation-decision-log.md#d-27-session-event-logging-uses-loggerlog-with-fixed-stepname-workflow-builder-and-a-field-exclusion-contract)). The `save_failed reason=<short>` event uses a closed enumeration — `validator_fatals | permission_error | disk_full | cross_device | conflict_detected | symlink_escape | target_not_regular_file | parse_error | other` — and never inlines free-form text ([F-113](artifacts/review-findings.md#f113-logger-reasonshort-value-enumeration-underspecified)).

**New dependency.** `github.com/google/shlex` added to `go.mod` for `$VISUAL`/`$EDITOR` word splitting ([D-22](artifacts/implementation-decision-log.md#d-22-visualeditor-word-splitting-via-githubcomgoogleshlex)).

## Decomposition and Sequencing

| # | Work Unit | Delivers | Depends On | Verification |
|---|-----------|----------|------------|--------------|
| 1 | `internal/atomicwrite` + `internal/ansi` + coding standard + `O_TRUNC` audit | `atomicwrite.Write` (with ENOENT walkback, explicit `O_EXCL|0o600`, parent-dir fsync), `internal/ansi.StripAll` ([D-45](artifacts/implementation-decision-log.md#d-45-internalansistripall-strict-ansi-stripper-for-untrusted-bytes)), `docs/coding-standards/file-writes.md` (four rules [D-17](artifacts/implementation-decision-log.md#d-17-docscoding-standardsfile-writesmd-scope--four-rules)), audit of existing `O_TRUNC` sites (`rawwriter.go`, `iterationlog.go`) — documented as exempt with rationale | — | `src/internal/atomicwrite/write_test.go` (T1 matrix); `src/internal/ansi/strip_test.go` (OSC 8 stripped, CSI stripped, bare ESC stripped); doc-integrity tests DI-4, DI-5 ([D-32](artifacts/implementation-decision-log.md#d-32-d38-doc-obligations-extended-with-doc-integrity-test-list)); `make ci` green |
| 2 | CLI subcommand wiring + logger prefix | `cmd/pr9k/workflow.go`, `newWorkflowCmd()`, `logger.NewLoggerWithPrefix` added; existing `NewLogger` preserved ([D-15](artifacts/implementation-decision-log.md#d-15-log-filename-prefix-workflow--builder-vs-ralph--run)); `--workflow-dir`/`--project-dir` flags ([D-36](artifacts/implementation-decision-log.md#d-36-pr9k-workflow-wires-as-a-peer-subcommand-to-pr9k-sandbox-bypasses-startup)) | — | `src/cmd/pr9k/workflow_cmd_test.go` (WG-1 registration, WG-2 no-stale-flags); updated `logger_test.go` `runStampRe` covering both prefixes |
| 3 | `internal/workflowmodel` + diff + scaffold | `WorkflowDoc`, `Step`, `EnvEntry`, `StatusLineBlock`, `diff.IsDirty`, `scaffold.Empty`, `scaffold.CopyFromDefault` ([D-2](artifacts/implementation-decision-log.md#d-2-in-memory-workflow-model-lives-in-workflowmodelworkflowdoc-distinct-from-vfile-and-stepsstepfile), [D-40](artifacts/implementation-decision-log.md#d-40-scaffold-placeholder-step-conventions-oq-4)) | — | `src/internal/workflowmodel/{diff,scaffold}_test.go` unit tests; input-immutability tests |
| 4 | `internal/workflowio` load + save + detect + `SaveFS` interface | `SaveFS` interface ([D-46](artifacts/implementation-decision-log.md#d-46-savefs-interface-for-testability), [F-102](artifacts/review-findings.md#f102-savefs-interface-shape-uncommitted-architecture)); `SaveResult` typed error kind ([D-47](artifacts/implementation-decision-log.md#d-47-saveresult-typed-error-kind-enum)); `realSaveFS` production impl; `Load`, `Save`, `DetectSymlink`, `DetectReadOnly`, `DetectExternalWorkflow`, `DetectSharedInstall` ([D-43](artifacts/implementation-decision-log.md#d-43-shared-install-detection-uses-syscallstat_tuid-unix-only)), `DetectCrashTempFiles` ([D-16](artifacts/implementation-decision-log.md#d-16-temp-file-naming-configjsonpid-epoch-nstmp-with-pid-liveness-rule)), `CreateEmptyCompanion` (with MkdirAll + regular-file check [F-109](artifacts/review-findings.md#f109-fifos--named-pipes-in-bundle-pass-containment-block-editor)), companion-first save sequence ([D-20](artifacts/implementation-decision-log.md#d-20-companion-file-save-ordering--companions-first-config-last)); applies `ansi.StripAll` to recovery-view bytes before returning to TUI ([F-94](artifacts/review-findings.md#f94-statuslinesanitize-preserves-osc-8-hyperlinks-plan-blocking)) | 1, 3 | `src/internal/workflowio/{save,load,detect,crashtemp}_test.go` — T1 matrix including symlink, first-save ENOENT walkback, rename-failure, companion rollback, EXDEV, crash-era classification with PID-liveness, regular-file rejection (FIFO), recovery-view ANSI stripping with OSC 8 dropped |
| 5 | Validator hardening (OI-1) + `ValidateDoc` + `workflowvalidate` bridge | `safePromptPath` EvalSymlinks + containment; `validateCommandPath` parallel guard ([D-38](artifacts/implementation-decision-log.md#d-38-oi-1-validator-hardening-lands-in-the-same-pr-as-the-builder)); new exported `ValidateDoc` ([D-3](artifacts/implementation-decision-log.md#d-3-validator-extension--add-validatedocdoc-workflowdir-companionfiles-alongside-the-existing-validateworkflowdir)); `workflowvalidate.Validate` bridge ([D-4](artifacts/implementation-decision-log.md#d-4-workflowvalidate-bridge-is-a-thin-type-conversion-layer)) | 3 | `wfvalidate_test.go` (T3 matrix); new symlink-escape rejection tests for `safePromptPath`/`validateCommandPath`; existing `TestValidate_ProductionStepsJSON` continues to pass unchanged ([D-37](artifacts/implementation-decision-log.md#d-37-production-integration-test-testvalidate_productionstepsjson-must-pass-unchanged)) |
| 6 | `EditorRunner` + production impl + `google/shlex` integration | `EditorRunner` interface ([D-6](artifacts/implementation-decision-log.md#d-6-editorrunner-is-a-one-method-interface-editor-resolution-is-private-to-the-production-impl)); `realEditorRunner` + private `resolveEditor` using `google/shlex` ([D-22](artifacts/implementation-decision-log.md#d-22-visualeditor-word-splitting-via-githubcomgoogleshlex)); `ExecCallback` SIGINT branch ([D-7](artifacts/implementation-decision-log.md#d-7-execcallback-branches-on-exit-code-130--sigint-routes-to-quit-confirm-not-re-read)) | 2 | `wfeditor_test.go` (T2 matrix); async-pattern guard test; `TestResolveEditor_*` for D33 rejection cases |
| 7 | `internal/workflowedit` core: model, dialogs, outline, detail, menu, footer, constants | `workflowedit.Model` implementing `tea.Model`, holding `workflowio.SaveFS` + `EditorRunner` interfaces injected at construction; `dialogState` + 14 `DialogKind` ([D-8](artifacts/implementation-decision-log.md#d-8-dialog-composition-uses-one-dialogstate-slot--helpopen-bool--depth-1-prevfocus)); Update routing ([D-9](artifacts/implementation-decision-log.md#d-9-update-routing--overlay-first-ordering-with-global-key-intercept-nested-inside)); widget-owned footer ([D-11](artifacts/implementation-decision-log.md#d-11-widget-owned-shortcut-footer-content-via-shortcutline-string)); two viewports ([D-12](artifacts/implementation-decision-log.md#d-12-two-independently-scrollable-viewports-with-pointer-side-mouse-routing)); `constants.go` ([D-35](artifacts/implementation-decision-log.md#d-35-builder-constants-file-collects-all-affordance-signifiers)); model suggestions ([D-42](artifacts/implementation-decision-log.md#d-42-scaffold-model-defaults--model-suggestion-list-lives-in-workfloweditmodelsuggestionsgo)) | 3, 4, 5, 6 | `src/internal/workflowedit/{model,outline,detail,menu,dialogs}_test.go` — 28 TUI mode-coverage entries per [`artifacts/tui-mode-coverage.md`](tui-mode-coverage.md); edge-case tests EC-1–EC-12 |
| 8 | Path picker + async tab-completion | `pathpicker.go` with `pathCompletionMsg` async `tea.Cmd` ([D-25](artifacts/implementation-decision-log.md#d-25-filesystem-tab-completion-is-a-custom-minimal-implementation-not-a-new-dependency)); `~` expansion; hidden-file rule | 7 | Unit tests for tab-completion, hidden-file rule, cycling behavior |
| 9 | Findings panel + save flow + validation-feedback integration | Findings panel with viewport + preserved scroll ([D-31](artifacts/implementation-decision-log.md#d-31-findings-panel-is-independently-scrollable-with-preserved-state-across-rebuild)); async save + `saveInProgress` + nanosecond mtime snapshot ([D-13](artifacts/implementation-decision-log.md#d-13-save-is-async-via-teacmd-save-in-progress-flag--snapshot-refresh-on-callback), [D-14](artifacts/implementation-decision-log.md#d-14-d41-mtime-snapshot-uses-nanosecond-precision-timetime-from-post-rename-osstat)) | 4, 5, 7 | Model-update tests for save matrix; findings-panel rebuild-preserves-acknowledgment tests; TUI VI-1–VI-4 edge cases |
| 10 | Session-event logging + shared-install banner + symlink-banner ordering | `log.Log("workflow-builder", line)` at every D39 trigger point per field-exclusion contract ([D-27](artifacts/implementation-decision-log.md#d-27-session-event-logging-uses-loggerlog-with-fixed-stepname-workflow-builder-and-a-field-exclusion-contract)); load-pipeline ordering (symlink detect → banner → parse → recovery view) ([D-23](artifacts/implementation-decision-log.md#d-23-stripansi-reuse-via-statuslinesanitize-recovery-view-capped-at-8-kib-symlink-banner-before-recovery-view-render)); shared-install detection ([D-43](artifacts/implementation-decision-log.md#d-43-shared-install-detection-uses-syscallstat_tuid-unix-only)); log-dir fallback ([D-44](artifacts/implementation-decision-log.md#d-44-log-directory-under-projectdirpr9klogs-external-workflow-case-inherits-the-same-directory)) | 2, 4 | Unit tests on log-line shape (regex pin on excluded fields); integration test: symlink target loads, banner fires first |
| 11 | Doc obligations + doc-integrity tests + ADR + CLAUDE.md updates | `docs/features/workflow-builder.md`; `docs/how-to/using-the-workflow-builder.md`; `docs/how-to/configuring-external-editor-for-workflow-builder.md`; ADR under `docs/adr/`; `docs/coding-standards/file-writes.md` ([D-17](artifacts/implementation-decision-log.md#d-17-docscoding-standardsfile-writesmd-scope--four-rules)); code-package docs for the six new internal packages (includes `atomicwrite` and `ansi`); `CLAUDE.md` updated with links; `docs/features/cli-configuration.md` updated; `docs/architecture.md` updated; DI-1–DI-8 tests added to `doc_integrity_test.go` ([D-32](artifacts/implementation-decision-log.md#d-32-d38-doc-obligations-extended-with-doc-integrity-test-list)) | 1–10 | `TestDocIntegrity_WorkflowBuilder*` suite; manual read-through of CLAUDE.md section headings |
| 12 | Version bump commit + rename-guard + integration + polish | `internal/version` bumped (patch); PR structure per [D-18](artifacts/implementation-decision-log.md#d-18-version-bump-is-the-first-commit-of-the-feature-pr-merged-with---no-ff) (subject to OI-3); `rename_guard_test.go` extended to assert `"workflow-"` log prefix and `"pr9k workflow"` command name don't collide ([F-116](artifacts/review-findings.md#f116-rename-guard-test-commitment-missing)); new `bundle_builder_integration_test.go` (separate from the existing bundle-layout test, [F-115](artifacts/review-findings.md#f115-evidence-drift--line-numbers-and-file-reference-gaps)) smoke-tests the bundled default workflow loading in the builder | 1–11 | `make ci` green; race detector clean; manual smoke against the bundled default workflow |

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
| A2 | APFS and ext4 preserve nanosecond `ModTime` through Go's `os.FileInfo` | D41 conflict detection degrades to second-precision on filesystems without nanosecond mtime — FAT32 (1s), exFAT (10ms), NFS v3 (1s), HFS+ (1s variants) — known limitation ([F-106](artifacts/review-findings.md#f106-mtime-precision-on-fat32--exfat--nfs-v3-silently-defeats-detector)) | Concurrency-analyst C8 cites stdlib behavior; integration test on macOS (APFS) verifies nanosecond case; filesystem-specific degradation documented in Operational Readiness | Verified bounds |
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

**Test files and coverage** (all paths co-located per `docs/coding-standards/testing.md` — normalized in R4 per [F-101](artifacts/review-findings.md#f101-test-file-naming-inconsistency-across-plan-sections)):

- `src/internal/atomicwrite/write_test.go` — T1 matrix of 11 enumerated cases ([F-118](artifacts/review-findings.md#f118-t1t2-test-matrix-enumeration-gap)):
  1. `TestWrite_FirstSave_NonExistentTarget_Succeeds` — ENOENT walkback on new file ([F-95](artifacts/review-findings.md#f95-filepathevalsymlinks-enoent-on-non-existent-paths-plan-blocking)).
  2. `TestWrite_ReplacesExistingFile` — basic replace.
  3. `TestWrite_SymlinkedTarget_SymlinkPreserved` — resolves through symlink; symlink entry untouched.
  4. `TestWrite_SymlinkTargetOnDifferentFS_ReturnsEXDEV` — EXDEV detection.
  5. `TestWrite_RenameFailure_RollsBackTempFile` — fault injection on rename.
  6. `TestWrite_TempFileCreationFailure_ReturnsError` — fault injection on OpenFile.
  7. `TestWrite_FsyncCalledBeforeRename` — call-order assertion.
  8. `TestWrite_ParentDirSyncCalled` — parent-directory fsync per [F-108](artifacts/review-findings.md#f108-parent-directory-fsync-missing-after-rename).
  9. `TestWrite_TempFileUsesExplicitO_EXCLAndMode0o600` — permission bits and exclusivity.
  10. `TestWrite_TempFileNameMatchesGlobPattern` — crash-era glob alignment ([F-110](artifacts/review-findings.md#f110-temp-file-naming-scheme-inconsistent-across-decisions)).
  11. `TestWrite_CrossDeviceRenameSurfacedAsEXDEV` — wrapped error chain through `errors.Is`.
  DI seam: `fakeFS` following the `sandbox_create_test.go` pattern; implements the `writeFS` internal interface used by `atomicwrite`.
- `src/internal/workflowio/save_test.go` — companion-files-written-before-config ordering ([D-20](artifacts/implementation-decision-log.md#d-20-companion-file-save-ordering--companions-first-config-last)); no-op save (`TestSave_NoOp_FileNotRewritten`); conflict-dialog precondition (mtime mismatch at save).
- `src/internal/workflowio/load_test.go` — parse error → recovery view; symlink-detect-before-parse ordering ([D-23](artifacts/implementation-decision-log.md#d-23-stripansi-reuse-via-statuslinesanitize-recovery-view-capped-at-8-kib-symlink-banner-before-recovery-view-render)); crash-era temp classification with PID-liveness ([D-16](artifacts/implementation-decision-log.md#d-16-temp-file-naming-configjsonpid-epoch-nstmp-with-pid-liveness-rule)).
- `src/internal/workflowmodel/diff_test.go`, `scaffold_test.go` — dirty-state tracking, scaffold defaults ([D-40](artifacts/implementation-decision-log.md#d-40-scaffold-placeholder-step-conventions-oq-4)).
- `src/internal/workflowvalidate/validate_test.go` — T3 matrix: `ValidateDoc` equivalence with `Validate`; in-memory companion map handling; does-not-touch-on-disk assertion; empty-scaffold `{"step-1.md": {}}` pass.
- `src/internal/validator/validator_test.go` — OI-1 hardening: symlink-escape rejection for both `safePromptPath` and `validateCommandPath` ([D-38](artifacts/implementation-decision-log.md#d-38-oi-1-validator-hardening-lands-in-the-same-pr-as-the-builder)).
- `src/cmd/pr9k/workflow_cmd_test.go` — cobra registration (WG-1); absent `--iterations` flag (WG-2); editor resolution via `google/shlex` (single-quoted, double-quoted, space-in-path error).
- `src/cmd/pr9k/editor_test.go` — T2 matrix of 8 enumerated cases ([F-118](artifacts/review-findings.md#f118-t1t2-test-matrix-enumeration-gap)):
  1. `TestResolveEditor_VisualWithDoubleQuotes_Parses` — `VISUAL="code --wait"`.
  2. `TestResolveEditor_VisualWithSingleQuotedPath_Parses` — `VISUAL='/Applications/Sublime Text/subl'`.
  3. `TestResolveEditor_VisualWithShellMetacharacters_Rejected` — backticks, `;`, `|`, newline.
  4. `TestResolveEditor_EditorNotOnPath_ReturnsSpecificError` — relative path with no match.
  5. `TestResolveEditor_NeitherVisualNorEditorSet_ReturnsGuidance` — documented fallback dialog.
  6. `TestExecCallback_ExitCode130_ReturnsSigintMsg` — `*exec.ExitError` with code 130.
  7. `TestExecCallback_RestoreTerminalFailure_ReturnsRestoreFailedMsg` — non-`*exec.ExitError` error ([F-107](artifacts/review-findings.md#f107-execcallback-restoreterminal-failure-branch-unspecified)).
  8. `TestExecCallback_EditorCleanExit_ReturnsExitMsgAndTriggersReread`.
- `src/internal/workflowedit/model_test.go` — 28 TUI mode-coverage entries enumerated at [`artifacts/tui-mode-coverage.md`](artifacts/tui-mode-coverage.md) ([F-100](artifacts/review-findings.md#f100-28-entry-tui-mode-coverage-table-lives-in-tmp-unreachable)); Update routing order with dialog/help overlay combinations ([D-9](artifacts/implementation-decision-log.md#d-9-update-routing--overlay-first-ordering-with-global-key-intercept-nested-inside)); Del scoped to outline focus ([D-10](artifacts/implementation-decision-log.md#d-10-del-key-scoped-to-outline-focus-not-a-global-key)); save flow three-stage state machine (`idle → validating → saving → idle`) ([D-13](artifacts/implementation-decision-log.md#d-13-save-is-async-via-teacmd-save-in-progress-flag--snapshot-refresh-on-callback), [F-105](artifacts/review-findings.md#f105-synchronous-validation-violates-concurrency-standard-on-nfsfuse)); `saveInProgress` and `validateInProgress` guards gate double-save; `Ctrl+Q` during save sets `pendingQuit` and surfaces `DialogSaveInProgress` then re-enters quit flow on `saveCompleteMsg` ([F-97](artifacts/review-findings.md#f97-ctrlq-during-save-in-flight-discards-savecompletemsg-plan-blocking)); `saveSnapshot == nil` on session transition skips conflict check ([F-98](artifacts/review-findings.md#f98-savesnapshot-not-reset-on-session-transition-plan-blocking)); findings-panel rebuild preserves acknowledgment; the 12 edge-case rows EC-1–EC-12.
- `src/internal/workflowedit/outline_render_test.go`, `detail_pane_render_test.go`, `menu_bar_render_test.go`, `dialogs_render_test.go`, `findings_panel_test.go`.
- `src/internal/workflowedit/pathpicker_test.go` — tab-complete async pattern; hidden-file rule; `~` expansion; empty-match cycling.
- `src/cmd/pr9k/doc_integrity_test.go` — DI-1 through DI-8 per [D-32](artifacts/implementation-decision-log.md#d-32-d38-doc-obligations-extended-with-doc-integrity-test-list); the existing `bundle_integration_test.go` is unchanged ([F-115](artifacts/review-findings.md#f115-evidence-drift--line-numbers-and-file-reference-gaps)).
- `src/cmd/pr9k/bundle_builder_integration_test.go` — new file (separate from the existing bundle-layout `bundle_integration_test.go`); smoke-tests that the bundled default workflow opens cleanly in the builder.
- `src/cmd/pr9k/rename_guard_test.go` — extended to assert `"workflow-"` filename prefix and `"pr9k workflow"` command name do not collide with existing `ralph-` log-file names or existing subcommand help text ([F-116](artifacts/review-findings.md#f116-rename-guard-test-commitment-missing)).
- `src/internal/workflowvalidate/validate_test.go` — one test pins the companion-file key convention: `ValidateDoc` given a full-path key (`"prompts/step-1.md"`) treats it as a cache miss and reads from disk; given a bare filename key (`"step-1.md"`) it uses the in-memory bytes ([F-121](artifacts/review-findings.md#f121-validatedoc-companion-file-key-convention--documentation-only)).

**Edge cases requiring coverage:** P0 edge cases from `/tmp/wb-edge-findings.md` (EXDEV [AS-1/PH-1], orphaned companions [AS-4] — as regression-probe unit test for companion ordering, `$VISUAL`-with-space [EE-1], terminal-height-1 [TUI-1]); 22 P1 edge cases per test plan §6 and edge-case findings section; concurrency C1–C8 each with a targeted test per `/tmp/wb-concurrency-findings.md`.

**Test levels:** Unit (per-package `_test.go` files per `docs/coding-standards/testing.md` conventions). Integration (`bundle_integration_test.go` — round-trip against `workflow/config.json`). The spec explicitly does not require end-to-end tests against a real TTY ([D-6](artifacts/implementation-decision-log.md#d-6-editorrunner-is-a-one-method-interface-editor-resolution-is-private-to-the-production-impl); `docs/coding-standards/testing.md` "do not test assembly-only code" pattern). `make ci` runs `go test -race ./...` ([D41-b](artifacts/decision-log.md#d41-b-test-strategy-for-t1-t2-and-tui-modes)).

## Security Posture

The builder adds seven concrete mitigations to threats the specialists identified, plus one validator hardening already scoped:

1. **Path traversal via symlinks (OI-1, Finding 5, Finding 3).** `safePromptPath` and `validateCommandPath` add `filepath.EvalSymlinks` before the containment check ([D-38](artifacts/implementation-decision-log.md#d-38-oi-1-validator-hardening-lands-in-the-same-pr-as-the-builder)). Pre-copy integrity check and the copy operation each apply the guard per-open, closing the TOCTOU window ([D-39](artifacts/implementation-decision-log.md#d-39-pre-copy-integrity-check-and-the-copy-operation-each-apply-evalsymlinks--containment-per-open)).

2. **Create-on-editor-open containment (NEW-1).** When the builder creates an empty companion file for a not-yet-existing `promptFile`, it applies EvalSymlinks on the parent directory + containment check + `O_EXCL|0o600` create ([D-21](artifacts/implementation-decision-log.md#d-21-create-on-editor-open-applies-evalsymlinks--containment-before-oscreate)).

3. **ANSI injection in recovery view (Finding 8; amended by [F-94](artifacts/review-findings.md#f94-statuslinesanitize-preserves-osc-8-hyperlinks-plan-blocking)).** `statusline.Sanitize` deliberately preserves OSC 8 hyperlinks for the status-line use case. That preservation is wrong for untrusted `config.json` content. The builder introduces a new `internal/ansi.StripAll(bytes) []byte` that strips EVERY ANSI escape including OSC 8, and `workflowio.Load` applies it before returning recovery content ([D-45](artifacts/implementation-decision-log.md#d-45-internalansistripall-strict-ansi-stripper-for-untrusted-bytes)). Display also capped at 8 KiB; symlink banner renders before recovery view in the load pipeline.

4. **Temp-file race + permission (Finding 4; amended by [F-110](artifacts/review-findings.md#f110-temp-file-naming-scheme-inconsistent-across-decisions)).** `atomicwrite.Write` uses explicit `os.OpenFile(tempPath, O_CREATE|O_EXCL|O_WRONLY, 0o600)` with `tempPath = "<basename>.<pid>-<epoch-ns>.tmp"` in the resolved real directory. `fsync` on the file before rename; `fsync` on the parent directory after rename per POSIX durability ([F-108](artifacts/review-findings.md#f108-parent-directory-fsync-missing-after-rename)). ([D-5](artifacts/implementation-decision-log.md#d-5-atomic-save-helper-lives-in-a-new-internalatomicwrite-package-with-signature-writepath-data-mode-error))

5. **`$VISUAL` word-splitting (Finding 1).** `github.com/google/shlex` ([D-22](artifacts/implementation-decision-log.md#d-22-visualeditor-word-splitting-via-githubcomgoogleshlex)), with explicit error message when an unquoted path contains a space.

6. **SIGINT during editor window + RestoreTerminal failure (Finding 7; amended by [F-107](artifacts/review-findings.md#f107-execcallback-restoreterminal-failure-branch-unspecified)).** `ExecCallback` performs a three-way type switch: `nil` → clean-exit re-read; `*exec.ExitError` with code 130 → `editorSigintMsg` → quit-confirm; other `*exec.ExitError` → re-read + non-blocking notice; any other error type → `editorRestoreFailedMsg` → `DialogError` ("terminal may be degraded — run `reset`") with NO re-read ([D-7](artifacts/implementation-decision-log.md#d-7-execcallback-branches-on-exit-code-130--sigint-routes-to-quit-confirm-not-re-read)). Signal handler unconditionally refuses `program.Kill` for 10s ([D-34](artifacts/implementation-decision-log.md#d-34-signal-handler-does-not-call-programkill-during-teaexecprocess-window)).

7. **Session-event logging field exclusion (Findings 6, 9).** Logger contract explicitly excludes every `containerEnv` value, every `env` entry value, every prompt-file content, and the full `$VISUAL` argument list — only the editor binary's first token is logged. Save-failure `reason` is drawn from a closed enumeration ([F-113](artifacts/review-findings.md#f113-logger-reasonshort-value-enumeration-underspecified)). ([D-27](artifacts/implementation-decision-log.md#d-27-session-event-logging-uses-loggerlog-with-fixed-stepname-workflow-builder-and-a-field-exclusion-contract))

8. **FIFO / non-regular-file rejection on load and create ([F-109](artifacts/review-findings.md#f109-fifos--named-pipes-in-bundle-pass-containment-block-editor)).** `workflowio.Load` and `CreateEmptyCompanion` verify `FileInfo.Mode().IsRegular()` on the EvalSymlinks-resolved target. Non-regular files (FIFOs, sockets, block/char devices, directories) are rejected with the D56 template, preventing indefinite-block attacks on the editor spawn.

9. **First-save path-resolution containment ([F-95](artifacts/review-findings.md#f95-filepathevalsymlinks-enoent-on-non-existent-paths-plan-blocking)).** `atomicwrite.Write` and `CreateEmptyCompanion` use `EvalSymlinks`-with-ENOENT-walkback to resolve the lowest existing ancestor. Containment is then re-asserted on the resolved parent plus the unresolved suffix — a non-existent `promptFile` like `../../../etc/evil` fails containment at the resolved-parent step.

**Accepted residual (Finding 2 content exposure):** A user who mis-autocompletes a path to a private key in the path picker, then enters the recovery view, sees up to 8 KiB of ANSI-stripped raw bytes. This is sufficient to contain a complete SSH key. Accepted residual given the 8 KiB cap is the primary defense and a content-type filter is out of scope for v1; the how-to guide calls out the risk. Users driving the path picker are treated as authoritative about their intent ([F-119](artifacts/review-findings.md#f119-path-picker-tab-completion--not-a-vulnerability)).

## Operational Readiness

- **Observability:** Session-event lines in `.pr9k/logs/workflow-<timestamp>.log` at session start, save outcomes, editor invocations (binary + exit + duration), quit events ([D-15](artifacts/implementation-decision-log.md#d-15-log-filename-prefix-workflow--builder-vs-ralph--run), [D-27](artifacts/implementation-decision-log.md#d-27-session-event-logging-uses-loggerlog-with-fixed-stepname-workflow-builder-and-a-field-exclusion-contract)). The `workflow-` prefix distinguishes these from run loop's `ralph-` logs. Log-directory fallback on `projectDir` resolution failure lands in `os.UserConfigDir()+"/.pr9k/logs/"` ([D-44](artifacts/implementation-decision-log.md#d-44-log-directory-under-projectdirpr9klogs-external-workflow-case-inherits-the-same-directory)). Save-failure events use a closed `reason=<short>` enumeration — `validator_fatals | permission_error | disk_full | cross_device | conflict_detected | symlink_escape | target_not_regular_file | parse_error | other` — with no free-form detail inlined ([F-113](artifacts/review-findings.md#f113-logger-reasonshort-value-enumeration-underspecified)).
- **Filesystem-precision limitation:** Conflict detection relies on `ModTime` precision. On APFS and ext4 — the development and production targets — nanosecond precision is preserved and the detector is reliable. On FAT32, exFAT, NFS v3, and some HFS+ configurations, `ModTime` degrades to seconds precision; same-second external saves are undetected. Documented in the how-to guide; users editing workflow bundles on those filesystems should expect degraded conflict detection ([F-106](artifacts/review-findings.md#f106-mtime-precision-on-fat32--exfat--nfs-v3-silently-defeats-detector)).
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
- [ ] **SIGINT responsiveness (aspirational, not enforced in v1)**: SIGINT response latency ≤ 10 s in any state; ≤ 2 s when no external editor is pending launch ([F-111](artifacts/review-findings.md#f111-signal-handler-10-second-fallback-rationale-and-dod-gap)).
- [ ] `internal/ansi.StripAll` test includes an OSC 8 hyperlink input and asserts the output contains no `\x1b]` byte ([F-94](artifacts/review-findings.md#f94-statuslinesanitize-preserves-osc-8-hyperlinks-plan-blocking)).
- [ ] `TestWrite_FirstSave_NonExistentTarget_Succeeds` and `TestCreateEmptyCompanion_MissingPromptsDir_CreatesItContained` added and passing ([F-95](artifacts/review-findings.md#f95-filepathevalsymlinks-enoent-on-non-existent-paths-plan-blocking)).

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

- **OI-3 (PR structure — version bump):** D-18 commits the version bump as the first commit of the feature PR merged with `--no-ff`. `docs/coding-standards/versioning.md:42` says "bump is its own commit, not a drive-by edit in a feature PR," and the prior 0.7.1 bump was its own PR. The plan's current structure satisfies the letter of the rule but is weaker than the precedent ([F-112](artifacts/review-findings.md#f112-version-bump-first-commit-interpretation-of-versioningmd)).
  - **Resolves when:** The user chooses between (a) land the version bump as a separate PR before the feature PR (matches precedent), or (b) accept the intra-PR interpretation and document it on D-18 explicitly.
  - **Blocks implementation:** No — implementation can proceed under either interpretation; this is a release-logistics choice.

- **OI-4 (PR scope — single vs split):** The feature PR is estimated at 8–14K LOC across 6 new packages plus validator and logger changes, 8 doc files, and tests. `docs/coding-standards/` does not mandate a split, but typical pr9k PRs are <1000 LOC. Recommended split at a natural boundary: **PR 1** delivers WU-1..WU-5 plus version bump (atomic-save, CLI subcommand wiring, `workflowmodel`, `workflowio`, `workflowvalidate`, OI-1 validator hardening, coding-standards entry, ADR); **PR 2** delivers WU-6..WU-12 (`EditorRunner`, `workflowedit`, findings panel, session logging, docs, integration polish). After PR 1, `pr9k workflow` runs and shows the empty-editor state; PR 2 lights up the edit machinery ([F-114](artifacts/review-findings.md#f114-single-pr-scope-at-8-14k-loc)).
  - **Resolves when:** The user chooses single-PR or split-PR ship. If split, the PR 1 / PR 2 boundary is as described above.
  - **Blocks implementation:** No — implementation can proceed under either choice; this is a review-logistics decision.

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
- **Recommendation:** **Ship as planned** pending user choice on OI-3 (version-bump PR structure) and OI-4 (single-PR vs split-PR ship). All 44 original decisions plus 3 added (D-45 `internal/ansi.StripAll`, D-46 `SaveFS` interface, D-47 `SaveResult`/`SaveErrorKind`) have specialist owners and revisit criteria. Five plan-blocking defects surfaced in the iterative-plan-review R4 pass ([F-94](artifacts/review-findings.md#f94-statuslinesanitize-preserves-osc-8-hyperlinks-plan-blocking), [F-95](artifacts/review-findings.md#f95-filepathevalsymlinks-enoent-on-non-existent-paths-plan-blocking), [F-96](artifacts/review-findings.md#f96-testvalidate_productionstepsjson-breaks-under-oi-1-plan-blocking), [F-97](artifacts/review-findings.md#f97-ctrlq-during-save-in-flight-discards-savecompletemsg-plan-blocking), [F-98](artifacts/review-findings.md#f98-savesnapshot-not-reset-on-session-transition-plan-blocking)) have been resolved into the plan.

## Review History

- **Review mode:** team (7 specialists: junior-developer, evidence-based-investigator, adversarial-validator, software-architect, adversarial-security-analyst, concurrency-analyst, test-engineer).
- **Iterations or rounds completed:** 1 (R4 in the shared iteration log — following R1/R2/R3 on the source specification) — see [artifacts/review-iteration-history.md](artifacts/review-iteration-history.md).
- **Team composition:**
  - `junior-developer` — generalist stress test; raised 15 R1 findings on unstated prerequisites and convention conflicts.
  - `evidence-based-investigator` — verified every codebase citation in the plan against actual code and git history.
  - `adversarial-validator` — falsified plan claims; surfaced 10 attacks including V7 (OSC 8 preserved by Sanitize) and V9 (production test breaks under OI-1).
  - `software-architect` — stress-tested package decomposition and interface shapes; Item 8 identified the `workflowedit → statusline` coupling.
  - `adversarial-security-analyst` — verified the 7 committed mitigations; SV-03 confirmed V7's OSC 8 exploit against shipped `sanitize.go`.
  - `concurrency-analyst` — 10 R1 findings; CV-02 (Ctrl+Q during save) and CV-09 (saveSnapshot not reset) were plan-blocking.
  - `test-engineer` — 12 findings on file naming and matrix traceability; TV-05 flagged the unreachable 28-mode table.
- **Findings raised:** 29 new (F-94 through F-122); added to [artifacts/review-findings.md](artifacts/review-findings.md). Breakdown by resolution: 24 plan-edit, 2 deferred-to-user (OI-3 and OI-4), 3 accepted/documented-limitation (F-117 PID reuse, F-120 orphaned companions already-accepted, F-122 expected-absent pending items), plus F-119 and F-121 evaluated as "not a plan change" after evidence.
- **Assumptions challenged across all passes:** `statusline.Sanitize` usefulness as a safety boundary (F-94 refuted); `filepath.EvalSymlinks` precondition on path existence (F-95 refuted); D-37 "unchanged pass" under OI-1 (F-96 refuted); D-14 "nanosecond precision" verification scope (F-106 bounded); plan's "ship as planned" single-PR claim (F-114 deferred to user).
- **Consolidations made:** Dialog ANSI handling centralized in `workflowio.Load` (removes `workflowedit → statusline` edge); `internal/ansi` added as the canonical stripper for untrusted bytes; `SaveResult`/`SaveErrorKind` centralize the EXDEV + permission + disk-full + conflict taxonomy so TUI never imports `syscall`.
- **Ambiguities resolved, and how:** Temp-file naming aligned across D-5 and D-16 (explicit `<basename>.<pid>-<ns>.tmp` scheme, not `os.CreateTemp`); test-file naming normalized to co-located convention; `NewLoggerWithPrefix` shape committed as additive (preserves `NewLogger`); `reason=<short>` enumerated closed set.
- **Open items remaining:** 4 — OI-1 (validator hardening, scheduled into the PR), OI-2 (spec D69 stale reference, non-blocking), OI-3 (version-bump PR structure — user choice), OI-4 (single-PR vs split-PR ship — user choice).

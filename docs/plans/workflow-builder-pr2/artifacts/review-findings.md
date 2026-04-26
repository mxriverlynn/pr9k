# Review Findings: Workflow Builder PR-2 Implementation Plan

Companion to `../feature-implementation-plan.md`. Round 1 of team-mode iterative review.

## Round 1 (R1) — six-specialist team review

**Date:** 2026-04-24
**Mode:** team
**Specialists engaged:** junior-developer, evidence-based-investigator, adversarial-validator, concurrency-analyst, adversarial-security-analyst, test-engineer
**Findings produced:** F-PR2-1 through F-PR2-58

---

### F-PR2-1 — IsDirty + UnknownFields sequencing breaks make-ci between WU-PR2-2 and WU-PR2-6

- **Agent:** junior-developer (JrF-PR2-003) + adversarial-validator (AdV-PR2-002)
- **Category:** Plan sequencing / standards conflict
- **Finding:** WU-PR2-2 adds `UnknownFields map[string]json.RawMessage` to `WorkflowDoc`. WU-PR2-6 fixes `IsDirty` to ignore `UnknownFields`. Between them, `IsDirty` (`reflect.DeepEqual` at `diff.go:7`) returns true after a clean load when the input config.json has any unknown fields, breaking the WU-PR2-2 round-trip test. D-PR2-2 commits to "every intermediate commit green under make ci" — this sequencing violates the invariant.
- **Evidence considered:** `src/internal/workflowmodel/diff.go:7`; D-PR2-18 in implementation-decision-log.md; D-PR2-2 commit-graph contract.
- **Resolution:** Move D-PR2-18's `IsDirty` fix into WU-PR2-2 so the schema addition and the comparison fix land atomically. Update D-PR2-18's `Driven by rounds` to include R1.
- **Resolved by:** edit applied to plan (Decomposition/Sequencing WU-PR2-2 description)
- **Raised in round:** R1
- **Changed in plan:** Decomposition and Sequencing > WU-PR2-2; Decomposition and Sequencing > WU-PR2-6 (gap closure); implementation-decision-log D-PR2-18 (Driven by rounds extended)

### F-PR2-2 — fakeFS lacks sync.Mutex until WU-PR2-12 but async paths begin in WU-PR2-6

- **Agent:** junior-developer (JrF-PR2-007)
- **Category:** Sequencing / standards conflict
- **Finding:** Per `docs/coding-standards/testing.md` §"Test doubles with shared state need mutexes," `fakeFS.WriteAtomic` called inside a Bubble Tea goroutine requires mutex protection. The plan defers fakeFS extensions to WU-PR2-12, but WU-PR2-6 runs save tests that exercise async paths under `-race`. `make ci -race` will fail at WU-PR2-6.
- **Evidence considered:** `docs/coding-standards/testing.md`; backup `helpers_test.go`; D-PR2-2 per-commit green guarantee.
- **Resolution:** Move fakeFS mutex + per-method counter additions into WU-PR2-3 (cherry-pick skeleton), so the cherry-pick lands with mutex-safe doubles. WU-PR2-12 retains responsibility only for documentation polish.
- **Resolved by:** edit applied to plan (Decomposition/Sequencing WU-PR2-3 and WU-PR2-12)
- **Raised in round:** R1
- **Changed in plan:** Decomposition and Sequencing > WU-PR2-3 description; > WU-PR2-12 description; Testing Strategy.

### F-PR2-3 — D-PR2-22 spawn() WaitGroup architecture is not implementable as described

- **Agent:** concurrency-analyst (CV-PR2-PR-001) + adversarial-validator (AdV-PR2-006)
- **Category:** Concurrency / synchronization
- **Finding:** D-PR2-22 says `runWorkflowBuilder` "holds a `sync.WaitGroup` registered with every closure spawn." But `tea.Cmd` closures are invoked by the Bubble Tea runtime, not by `runWorkflowBuilder`. `runWorkflowBuilder` cannot call `wg.Add` because it has no hook into when Update returns a closure. If `wg.Add(1)` and `defer wg.Done()` both live in the closure, the WG is always at zero when `wg.Wait()` is called. If the WG lives in `Model`, then `Model` becomes a `sync.WaitGroup`-bearing struct that `go vet copylocks` flags. The plan does not specify ownership.
- **Evidence considered:** Bubble Tea runtime model; `sync.WaitGroup` copylocks; backup `model.go` Update signature is value receiver.
- **Resolution:** D-PR2-22 must specify: (a) `*sync.WaitGroup` lives in `workflowDeps` (passed by pointer to Model on construction); (b) `Model.Update` calls `m.wg.Add(1)` immediately before returning a `tea.Cmd`-wrapped closure, and the closure calls `defer m.wg.Done()` as its first statement; (c) `runWorkflowBuilder` receives the same WG pointer via `workflowDeps` and calls `wg.Wait()` after `cancel()`; (d) `wg.Wait()` is safe because Bubble Tea's event loop has already exited when `program.Run()` returned.
- **Resolved by:** edit applied to plan (Runtime Behavior > Goroutine lifecycle, decision log D-PR2-22)
- **Raised in round:** R1
- **Changed in plan:** Implementation Approach > Runtime Behavior > Goroutine lifecycle; implementation-decision-log D-PR2-22.

### F-PR2-4 — D-PR2-22 wg.Wait has no timeout; can hang runWorkflowBuilder indefinitely on NFS

- **Agent:** concurrency-analyst (CV-PR2-PR-005) + concurrency-analyst (CV-PR2-PR-004)
- **Category:** Deadlock potential / concurrency
- **Finding:** `sync.WaitGroup.Wait()` has no timeout. `docs/coding-standards/concurrency.md` §"Wait for background goroutines after program.Run() returns" prescribes the bounded `select { case <-done: case <-time.After(4*time.Second): }` pattern. Combined with the fact that `select <-ctx.Done()` cannot interrupt blocking syscalls (NFS-hung `os.Rename`), the unbounded `wg.Wait()` can hang `runWorkflowBuilder` forever — the binary appears to exit but the process holds an NFS lock until `kill -9`.
- **Evidence considered:** `docs/coding-standards/concurrency.md`; D-PR2-13 NFS rationale.
- **Resolution:** Replace `wg.Wait()` with bounded-wait pattern: spawn a goroutine that calls `wg.Wait()` and closes a done channel; use `select { case <-done: case <-time.After(4*time.Second): }`. Document in D-PR2-13 that the ctx propagation does NOT cancel in-progress blocking syscalls — that is an accepted residual handled by the bounded drain.
- **Resolved by:** edit applied to plan (Runtime Behavior, RAID Assumptions A-PR2-3 NFS limitation; decision log D-PR2-22 + D-PR2-13)
- **Raised in round:** R1
- **Changed in plan:** Runtime Behavior > Goroutine lifecycle; RAID > Assumptions; D-PR2-13 rationale (correct NFS claim); D-PR2-22 (bounded wait).

### F-PR2-5 — D-PR2-19 pre-dispatch tier excludes quitMsg; SIGINT silently swallowed during DialogSaveInProgress

- **Agent:** concurrency-analyst (CV-PR2-PR-006)
- **Category:** Async error / control flow
- **Finding:** D-PR2-19 says "validateCompleteMsg and saveCompleteMsg (and only those two types)" pre-dispatch before tier-2 (dialog). The signal handler sends `quitMsg{}` via `program.Send`, which is also async-delivered to Update. When `DialogSaveInProgress` is open and SIGINT fires, `quitMsg` enters tier-2 and `updateDialog(DialogSaveInProgress)` does not type-switch on it — silently dropped. The save-in-progress dialog is the highest-likelihood state to interrupt with SIGINT.
- **Evidence considered:** D-PR2-21 signal-handler design; D-PR2-14 editorInProgress contract; backup `model.go updateDialogSaveInProgress`.
- **Resolution:** Extend D-PR2-19's pre-dispatch tier to include `quitMsg` alongside `validateCompleteMsg` and `saveCompleteMsg`. Update plan's Update routing to: tier (0) — typed-message pre-dispatch for `validateCompleteMsg | saveCompleteMsg | quitMsg`; tier (1) helpModal; tier (2) dialog; tier (3) globalKey; tier (4) editView.
- **Resolved by:** edit applied to plan (Runtime Behavior > Update routing; decision log D-PR2-19)
- **Raised in round:** R1
- **Changed in plan:** Implementation Approach > Runtime Behavior > Update routing; D-PR2-19 decision text.

### F-PR2-6 — Ctrl+E editor invocation has no containment check for already-existing files

- **Agent:** adversarial-security-analyst (SEC-PR2-002, SEC-PR2-006)
- **Category:** Security / path traversal
- **Finding:** D-PR2-8 commits `Ctrl+E` shortcut to invoke `m.editor.Run(filePath, cb)` on a focused promptFile field. `workflowio.CreateEmptyCompanion` enforces containment only for not-yet-existing paths. When the file already exists (user typed `../../.ssh/authorized_keys`), the editor opens it with no containment check — validator's `safePromptPath` only fires at save time. Editor opens system file, may write back via `:wq`. The original plan's mitigations 1, 2, and 8 (path traversal, create-on-editor-open, FIFO rejection) are inherited but unverified for the Ctrl+E existing-file path.
- **Evidence considered:** `src/internal/validator/validator.go safePromptPath`; backup model.go has no Ctrl+E handler (GAP-029); D-PR2-8.
- **Resolution:** Add D-PR2-24 (new): "Ctrl+E handler applies `validator.safePromptPath(workflowDir, fieldValue)` containment AND `os.Lstat`-based regular-file check before invoking `m.editor.Run`, regardless of whether the file already exists. On containment or non-regular-file failure, opens `DialogError` with the safe-default text." Add to WU-PR2-4 verification: `TestModel_CtrlE_PathTraversal_Rejected`, `TestModel_CtrlE_ExistingFIFO_Rejected`.
- **Resolved by:** new decision D-PR2-24 added to decision log; edit to WU-PR2-4 description
- **Raised in round:** R1
- **Changed in plan:** Decomposition and Sequencing > WU-PR2-4; Security Posture; new D-PR2-24 in implementation-decision-log.

### F-PR2-7 — DialogCrashTempNotice Discard has no containment check before os.Remove

- **Agent:** adversarial-security-analyst (SEC-PR2-001)
- **Category:** Security
- **Finding:** D-PR2-17 commits Discard action that "deletes the temp file" via `os.Remove`. `DetectCrashTempFiles` glob output is not containment-verified. An attacker who controls `workflowDir` can plant a temp-named symlink pointing outside; though `os.Remove` removes the symlink itself (low blast radius), the lack of an explicit guard violates defense-in-depth and should be codified.
- **Evidence considered:** backup `crashtemp.go:48`; D-PR2-17.
- **Resolution:** D-PR2-17 must require: Discard handler verifies path satisfies (a) `filepath.IsAbs`, (b) `EvalSymlinks(path)` resolves inside `EvalSymlinks(workflowDir)`, (c) `os.Lstat` returns regular file. Add `TestDialog_CrashTempDiscard_RejectsPathEscapingWorkflowDir` to WU-PR2-8 verification.
- **Resolved by:** edit applied to plan (decision log D-PR2-17)
- **Raised in round:** R1
- **Changed in plan:** Decomposition and Sequencing > WU-PR2-8; D-PR2-17.

### F-PR2-8 — fmtEditorOpened uses hand-rolled extraction, not shlex tokens[0] — credential leak risk

- **Agent:** adversarial-security-analyst (SEC-PR2-004)
- **Category:** Security / observability
- **Finding:** Plan's External Interfaces section claims `fmtEditorOpened` uses `tokens[0]` from the resolved shlex parse. Backup `session_log.go:60-81` shows it accepts the raw `editorEnv` string and re-parses with last-`/` + first-` ` extraction, which fails on `VISUAL='/Applications/Sublime Text/subl' --wait` (returns `subl' --wait` then strips at space → `subl'`). For `VISUAL="nvim --auth-token=SECRET"`, the `SECRET` value can leak past the space-strip if the token has no leading space.
- **Evidence considered:** backup `session_log.go:60-81`; D-PR2-16 + plan's "tokens[0]" claim.
- **Resolution:** WU-PR2-4 must update `fmtEditorOpened` signature to take `binary string` (the already-resolved `tokens[0]`) instead of raw `editorEnv`. Remove `editorFirstToken` helper. Update `realEditorRunner.Run` to expose `tokens[0]` to the caller (or have the cmd return it as part of `editorExitMsg`). Add `TestFmtEditorOpened_CredentialInArgs_NoLeak`.
- **Resolved by:** edit applied to plan (Runtime Behavior > External editor handoff; D-PR2-16)
- **Raised in round:** R1
- **Changed in plan:** Implementation Approach > Runtime Behavior > External editor handoff; Decomposition and Sequencing > WU-PR2-4; D-PR2-16 trigger-point list.

### F-PR2-9 — D-PR2-16 trigger points omit load-time security signals (symlink/external/read-only/shared-install)

- **Agent:** adversarial-security-analyst (SEC-PR2-003)
- **Category:** Observability / security
- **Finding:** D-PR2-16 lists 9 trigger points; load-time banner conditions (symlink-detected, external-workflow, read-only, shared-install) are not in the list. The shared-install helper exists in backup `session_log.go` but the other three have no helpers and no trigger sites. Operators have no audit trail for the most security-relevant load conditions.
- **Evidence considered:** backup `session_log.go`; D-PR2-16; original plan §Operational Readiness > Observability.
- **Resolution:** Extend D-PR2-16 to include `symlink_detected target=<resolved path>`, `external_workflow_detected workflowDir=<path>`, `read_only_detected workflowDir=<path>`. Add `fmtSymlinkDetected`, `fmtExternalWorkflowDetected`, `fmtReadOnlyDetected` to the WU-PR2-10 deliverable. Wire each at its corresponding load-time banner site.
- **Resolved by:** edit applied to plan (decision log D-PR2-16; WU-PR2-10 description)
- **Raised in round:** R1
- **Changed in plan:** Decomposition and Sequencing > WU-PR2-10; D-PR2-16 trigger-point list; Operational Readiness > Observability.

### F-PR2-10 — fmtEditorOpened merges open+exit; D-PR2-16's "external editor invoked" event never fires

- **Agent:** adversarial-security-analyst (SEC-PR2-007)
- **Category:** Observability
- **Finding:** Backup `fmtEditorOpened(editorEnv, exitCode, duration)` is called only post-exit (it requires `exitCode`). D-PR2-16 enumerates "external editor invoked" and "external editor exit" as distinct events. A hung editor produces no log line until the editor returns — operator cannot diagnose stuck sessions.
- **Evidence considered:** backup `session_log.go`; D-PR2-16.
- **Resolution:** Split into `fmtEditorInvoked(binary string)` (fired when `launchEditorMsg` fires, before tea.ExecProcess) and `fmtEditorExited(binary string, exitCode int, d time.Duration)` (fired in ExecCallback handler). Land in WU-PR2-10. Add `TestFmtEditorEvents_TwoDistinctLogLines`.
- **Resolved by:** edit applied to plan (decision log D-PR2-16; WU-PR2-10)
- **Raised in round:** R1
- **Changed in plan:** Decomposition and Sequencing > WU-PR2-10; D-PR2-16.

### F-PR2-11 — D-PR2-21 program.Send before program.Run causes 10s blocking hang on early SIGINT

- **Agent:** adversarial-validator (AdV-PR2-007) + concurrency-analyst (CV-PR2-PR-002)
- **Category:** Concurrency / control flow
- **Finding:** Bubble Tea's `program.Send` blocks before `program.Run()` is called (no goroutine reading the msgs channel). If SIGINT arrives between `tea.NewProgram` and `program.Run()`, the signal goroutine blocks for up to 10 seconds before the fallback `os.Exit(130)`. D-PR2-21's claim of race-free is correct; the 10-second hang is an unintended side effect not noted. CV-PR2-PR-002 also flags the rationale's "captured by value" wording as Go-incorrect (closures capture variables, not values).
- **Evidence considered:** Bubble Tea v1.3.10 `tea.go:770-779` (Send blocks pre-Run); D-PR2-21 rationale.
- **Resolution:** D-PR2-21 rationale must (a) correct "captured by value" to "captures the variable `program`, which is a pointer initialized before the goroutine starts; the invariant is that `program` is never reassigned after the goroutine spawns"; (b) document the early-SIGINT hang as the designed behavior and confirm the 10-second timer (D-PR2-23) catches it. The hang is acceptable because the user has at most a 10-second wait if they SIGINT in the millisecond window before `program.Run()` actually starts reading.
- **Resolved by:** edit applied to plan (decision log D-PR2-21 rationale)
- **Raised in round:** R1
- **Changed in plan:** D-PR2-21.

### F-PR2-12 — D-PR2-20 generation counter increments only on Tab; should increment on every prefix mutation

- **Agent:** concurrency-analyst (CV-PR2-PR-003)
- **Category:** Race conditions
- **Finding:** D-PR2-20 says counter increments "on every Tab keystroke." But the path prefix can change via character input, backspace, paste — without these incrementing the counter, a stale `pathCompletionMsg` for the prior prefix can overwrite the user's current input.
- **Evidence considered:** Spec §"Path picker"; D-PR2-20.
- **Resolution:** D-PR2-20 must clarify: counter increments on every keystroke that mutates the path prefix (character input, backspace, paste) AND on Tab when a new async lookup is dispatched. It does NOT increment on Tab cycles through already-received completions (no new dispatch).
- **Resolved by:** edit applied to plan (decision log D-PR2-20)
- **Raised in round:** R1
- **Changed in plan:** D-PR2-20.

### F-PR2-13 — WU-PR2-9 is too large (9 gaps in one commit)

- **Agent:** junior-developer (JrF-PR2-005) + adversarial-validator (AdV-PR2-010)
- **Category:** Decomposition
- **Finding:** WU-PR2-9 closes GAP-010, GAP-013, GAP-014, GAP-015, GAP-016, GAP-017, GAP-018, GAP-019 plus path-picker for File>New. Its verification names 9 mode-coverage entries. This is a single bisect dead zone. GAP-014 alone (detail-pane field editing) covers 6 field types. D-PR2-2's "every commit green under make ci" means a single failure holds up all 9 gap closures.
- **Evidence considered:** WU-PR2-9 description; tui-mode-coverage modes 6-19.
- **Resolution:** Split WU-PR2-9 into WU-PR2-9a (outline structure: GAP-010 phase grouping, GAP-013 + Add affordance — structural changes that affect the outline.go file) and WU-PR2-9b (detail-pane input layer: GAP-014 input, GAP-015 choice list, GAP-016 numeric, GAP-017 secret masking, GAP-018 model-suggestion, GAP-019 path picker).
- **Resolved by:** edit applied to plan (Decomposition and Sequencing WU-PR2-9 → WU-PR2-9a + WU-PR2-9b)
- **Raised in round:** R1
- **Changed in plan:** Decomposition and Sequencing > WU-PR2-9.

### F-PR2-14 — Constructor signature ambiguity: New(saveFS, editor, projectDir, workflowDir) vs New(deps workflowDeps)

- **Agent:** junior-developer (JrF-PR2-002) + evidence-based-investigator (EBI-PR2-001) + adversarial-validator (AdV-PR2-001)
- **Category:** Architecture / cherry-pick scope
- **Finding:** Backup's `New` takes 4 positional parameters; PR-2 plan's wiring sequence implies a `workflowDeps` struct. The constructor change is `replace`, not `as-is`/`modify`. Every test helper calling `New(...)` must update. Plan's WU-PR2-3 description ("cherry-pick the workflowedit package skeleton") understates this work. EBI-PR2-001 also notes the backup does NOT reference `UnknownFields`/top-level `Env`/`ContainerEnv`, so D-PR2-4's "compile-blocking" claim (cherry-pick can't compile until WU-PR2-2 lands) is technically false — the cherry-pick compiles against current main. The sequencing rationale for WU-PR2-2 → WU-PR2-3 should rest on data-corruption (EC-PR2-004), not compile dependency.
- **Evidence considered:** backup `model.go:65-85` constructor; backup imports lack `internal/logger`.
- **Resolution:** Update WU-PR2-3 to explicitly include "introduce `workflowDeps` struct (logger + saveFS + editor + workflowDir + projectDir + bundled-default copy-source); refactor `New()` signature to `New(deps workflowDeps)` and update all call sites in helpers_test.go." Update D-PR2-4's rationale to anchor on data-corruption, not compile dependency.
- **Resolved by:** edit applied to plan (WU-PR2-3 description; D-PR2-4 rationale clarification)
- **Raised in round:** R1
- **Changed in plan:** Decomposition and Sequencing > WU-PR2-3; D-PR2-4.

### F-PR2-15 — D-PR2-4 conflates existing fields with genuinely new fields

- **Agent:** evidence-based-investigator (EBI-PR2-002)
- **Category:** Decision log accuracy
- **Finding:** D-PR2-4 lists `DefaultModel string` and per-step `Env []EnvEntry` as new additions. Both already exist on main (`workflowmodel/model.go:53,65`). The actual new struct fields are `UnknownFields` + top-level `Env` + top-level `ContainerEnv` + step-level `ContainerEnv`. The `DefaultModel` and step-level `Env` work is marshal/unmarshal coverage (struct-present but JSON-invisible per GAP-039/040), not field addition.
- **Evidence considered:** `src/internal/workflowmodel/model.go:53,65`; backup tag identical.
- **Resolution:** Update D-PR2-4 decision text and DoD to distinguish: (a) new struct fields (UnknownFields, top-level Env, top-level ContainerEnv, step-level ContainerEnv); (b) existing struct fields needing marshal/unmarshal coverage (DefaultModel, step-level Env).
- **Resolved by:** edit applied to plan (decision log D-PR2-4)
- **Raised in round:** R1
- **Changed in plan:** D-PR2-4; Definition of Done.

### F-PR2-16 — D-PR2-5 says backup types are exported; they are unexported (and Update has no handlers at all)

- **Agent:** evidence-based-investigator (EBI-PR2-003)
- **Category:** Decision log accuracy
- **Finding:** Backup `cmd/pr9k/editor.go:24-31` declares `editorExitMsg`, `editorSigintMsg`, `editorRestoreFailedMsg` (lowercase, unexported). D-PR2-5 cites them as `EditorExitMsg` etc. (uppercase). Backup `model.go` has zero handlers for any of these. The work is more than "move and capitalize" — it is creating new exported types AND adding new Update handlers.
- **Evidence considered:** backup `cmd/pr9k/editor.go:24-31`; backup `model.go` (no editor message cases).
- **Resolution:** Update D-PR2-5 decision text to clarify: (1) declare new exported types `EditorExitMsg`, `EditorSigintMsg`, `EditorRestoreFailedMsg` in `internal/workflowedit/editor.go` (the backup's unexported types are deleted from cmd/pr9k); (2) add Update handlers in `model.go` for each new type (this is net-new code, not a move).
- **Resolved by:** edit applied to plan (D-PR2-5 decision text and rationale)
- **Raised in round:** R1
- **Changed in plan:** D-PR2-5.

### F-PR2-17 — D-PR2-6 silently overrides spec-stage decision D33

- **Agent:** adversarial-validator (AdV-PR2-004)
- **Category:** Spec-stage governance
- **Finding:** Spec D33 lists rejection set as `{backtick, ;, |, &, <, >, newline}` (7 chars). D-PR2-6 commits `{backtick, ;, |, newline}` (4 chars) — drops `$` (correct), drops `&`/`<`/`>` (deviation). D33 is spec-stage ground truth; PR-2 plan inherits "D1..D73 from spec decision-log." Dropping `&`/`<`/`>` is a spec-level deviation that requires either supersession or explicit user acceptance.
- **Evidence considered:** Spec decision-log D33; PR-2 plan inherits-decisions list.
- **Resolution:** Document D-PR2-6 as a deliberate spec-level deviation in the plan's Open Items section: "OI-PR2-1 — Spec D33 rejection set narrowed to direct-exec-relevant chars; spec author should add D74 superseding D33 with the rationale 'characters inert under direct exec are not rejected.' Implementation proceeds under D-PR2-6 pending spec amendment." This frames the override as documented and surfaces it for user-acceptance.
- **Resolved by:** edit applied to plan (Open Items section)
- **Raised in round:** R1
- **Changed in plan:** Open Items.

### F-PR2-18 — Goroutine ownership matrix is absent

- **Agent:** concurrency-analyst (CV-PR2-PR-008)
- **Category:** Documentation gap
- **Finding:** Plan names six goroutine concerns across nine concurrency decisions but never enumerates which goroutines exist, who owns them, or which is in the WG. Without an explicit table, implementors infer the lifecycle separately — the inferences contradict (as the other concurrency findings demonstrate).
- **Resolution:** Add a Goroutine Ownership Matrix table to Runtime Behavior. Columns: name, creator, shared state accessed, exit condition, in WG (yes/no), ctx source. Rows: (a) Bubble Tea event loop, (b) Bubble Tea runtime, (c) save closure, (d) validate closure, (e) tab-complete closure, (f) signal goroutine, (g) ExecProcess goroutine, (h) 10s fallback timer goroutine.
- **Resolved by:** edit applied to plan (Runtime Behavior > Goroutine ownership matrix — new subsection)
- **Raised in round:** R1
- **Changed in plan:** Implementation Approach > Runtime Behavior.

### F-PR2-19 — Backup file count: plan says 13 named; actually 27 (12 test files unnamed)

- **Agent:** evidence-based-investigator (EBI-PR2-008)
- **Category:** Triage scope
- **Finding:** Plan's Architecture section lists 13 backup files explicitly. Backup actually has 27 (15 source + 12 test files). Test files (`detail_pane_render_test.go`, `dialogs_render_test.go`, `findings_panel_test.go`, `footer_test.go`, `menu_bar_render_test.go`, `model_test.go`, `outline_render_test.go`, `pathpicker_test.go`, `save_flow_test.go`, `session_log_test.go`, `shared_install_test.go`, `viewport_test.go`) need triage classification too.
- **Resolution:** Update WU-PR2-0 description to require the triage table classify all 27 backup files (15 source + 12 test). Test files typically classify `as-is` or `modify` per gap analysis findings (e.g., save_flow_test.go mode-21 needs rewrite per F-PR2-23 below).
- **Resolved by:** edit applied to plan (WU-PR2-0 description)
- **Raised in round:** R1
- **Changed in plan:** Decomposition and Sequencing > WU-PR2-0.

### F-PR2-20 — DialogFileConflict Reload error path unspecified

- **Agent:** adversarial-validator (AdV-PR2-008)
- **Category:** Error propagation
- **Finding:** D-PR2-17's Reload action discards in-memory state and re-loads from disk. If the async load fails (parse error after concurrent disk corruption), the model is left with no in-memory doc and no successful disk load. The plan does not specify which dialog opens — `DialogError` (current default) or `DialogRecovery` (the parse-recovery view).
- **Resolution:** D-PR2-17 must specify: "If Reload's `makeLoadCmd` returns `openFileResultMsg` with parse error, the handler routes to `DialogRecovery`; with permission/disk error, routes to `DialogError`." Add `TestModel_DialogFileConflict_Reload_ParseError_ShowsRecovery`.
- **Resolved by:** edit applied to plan (D-PR2-17)
- **Raised in round:** R1
- **Changed in plan:** D-PR2-17.

### F-PR2-21 — D-PR2-10 re-routing has latent cycle if editorInProgress=true at save complete

- **Agent:** adversarial-validator (AdV-PR2-005)
- **Category:** State-machine / control flow
- **Finding:** D-PR2-10's `handleSaveResult → handleGlobalKey(Ctrl+Q)` re-routing assumes `editorInProgress=false` at save-completion time. The plan does not invariantize this; if `editorInProgress=true` (some race or future code path), `handleGlobalKey` sets `pendingAction = pendingActionQuit{}` and returns without opening QuitConfirm — silent state loop.
- **Resolution:** Add invariant to plan Runtime Behavior: "At the time `saveCompleteMsg` is dispatched, `editorInProgress` is always false because `makeSaveCmd` is only spawned from paths that gate on `!editorInProgress` AND `editorInProgress` is cleared synchronously by ExecCallback before any save can be re-spawned." Add `TestModel_PendingQuit_SaveComplete_AssumesEditorInProgressFalse`.
- **Resolved by:** edit applied to plan (Runtime Behavior; D-PR2-10)
- **Raised in round:** R1
- **Changed in plan:** Runtime Behavior; D-PR2-10.

### F-PR2-22 — D-PR2-18 IsDirty UnknownFields comparison underspecified for nested objects

- **Agent:** adversarial-validator (AdV-PR2-011)
- **Category:** Decision precision
- **Finding:** D-PR2-18 says "sorted-key equality of `json.RawMessage` byte slices." For unknown values that are nested JSON objects (e.g., `{"old_field": {"a":1, "b":2}}`), the inner-object key order is byte-sensitive without canonicalization. Round-trip can produce different byte ordering and IsDirty returns true for semantically-equal values.
- **Resolution:** Update D-PR2-18 to specify: "UnknownFields comparison normalizes each `RawMessage` value via `json.Unmarshal → json.Marshal` round-trip before byte comparison. Outer map keys are sorted before comparison." Add `TestIsDirty_UnknownFields_NestedObjectDifferentKeyOrder_False`.
- **Resolved by:** edit applied to plan (D-PR2-18)
- **Raised in round:** R1
- **Changed in plan:** D-PR2-18.

### F-PR2-23 — Some backup tests pin buggy behavior; "reactivate" is wrong; rewrite required

- **Agent:** junior-developer (JrF-PR2-010)
- **Category:** Test correctness
- **Finding:** Backup's mode-21 test (pendingQuit → direct tea.Quit) pins the buggy behavior D-PR2-10 fixes. "Reactivate" preserves the bug; rewrite is required. Same for any test pinning `pendingQuit bool` (D-PR2-15 replaces with discriminated type).
- **Resolution:** Add testing-strategy bullet: "The following backup tests must be rewritten (not reactivated) because they pin behavior PR-2 fixes: (a) `TestSaveComplete_WithPendingQuit_ReentersQuitFlow` — must assert DialogQuitConfirm opens, not direct tea.Quit (D-PR2-10); (b) any tests using `m.pendingQuit` field — replace with `m.pendingAction` type assertions (D-PR2-15)."
- **Resolved by:** edit applied to plan (Testing Strategy)
- **Raised in round:** R1
- **Changed in plan:** Testing Strategy.

### F-PR2-24 — DI-1/DI-2 reactivation: assertions need explicit specification

- **Agent:** test-engineer (TST-PR2-PR-002)
- **Category:** Test specification
- **Finding:** Plan commits to reactivating DI-1 and DI-2 but doesn't enumerate what they assert. Existing DI-3..DI-8 pattern is "file exists AND linked from CLAUDE.md." Plan should align.
- **Resolution:** Add to Testing Strategy: "DI-1: `docs/features/workflow-builder.md` exists AND linked from CLAUDE.md. DI-2: `docs/how-to/using-the-workflow-builder.md` and `docs/how-to/configuring-external-editor-for-workflow-builder.md` both exist AND both linked from CLAUDE.md. DI-5 extended to also include `docs/code-packages/workflowedit.md`. All four file deliverables and CLAUDE.md update land in WU-PR2-11 atomically."
- **Resolved by:** edit applied to plan (Testing Strategy)
- **Raised in round:** R1
- **Changed in plan:** Testing Strategy.

### F-PR2-25 — Bundle smoke test must drive Model.Update; specific message sequence required

- **Agent:** test-engineer (TST-PR2-PR-003)
- **Category:** Test specification
- **Finding:** Current `bundle_builder_integration_test.go` only tests inner ring. PR-2 commits to "drive Model end-to-end" but doesn't specify the message sequence.
- **Resolution:** Add to Testing Strategy / WU-PR2-12 verification: bundle smoke test (1) constructs `workflowedit.New(deps)` with a `fakeFS` capturing WriteAtomic; (2) `model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyCtrlS})`; (3) `msg := cmd()` synchronously; (4) `model, _ = model.Update(msg)`; (5) asserts `fakeFS.writeAtomicCalls == 1` and `model.IsDirty() == false`. No `tea.NewProgram`.
- **Resolved by:** edit applied to plan (Testing Strategy; WU-PR2-12)
- **Raised in round:** R1
- **Changed in plan:** Testing Strategy; Decomposition and Sequencing > WU-PR2-12.

### F-PR2-26 — Test package scope: `package workflowedit` (not `_test`) for access to unexported message types

- **Agent:** test-engineer (TST-PR2-PR-001)
- **Category:** Test specification
- **Finding:** Mode-coverage tests must inject unexported messages (`saveCompleteMsg`, `validateCompleteMsg`, `pathCompletionMsg`). Tests must live in `package workflowedit` (not `workflowedit_test`).
- **Resolution:** Add to Testing Strategy: "All `TestModel_Mode_*` tests live in `package workflowedit` to access unexported message types. The `_test` package suffix is reserved for the public API surface tests (e.g., `TestNew_AcceptsDeps`)."
- **Resolved by:** edit applied to plan (Testing Strategy)
- **Raised in round:** R1
- **Changed in plan:** Testing Strategy.

### F-PR2-27 — fakeFS / fakeEditorRunner method enumeration missing

- **Agent:** test-engineer (TST-PR2-PR-004)
- **Category:** Test specification
- **Finding:** Plan says fakeFS has per-method counters but doesn't enumerate the SaveFS interface methods. Same for fakeEditorRunner.
- **Resolution:** Add to Testing Strategy: "`fakeFS` implements `SaveFS` with: `writeAtomicCalls int`, `writeAtomicArgs []WriteAtomicCall {path, data, mode}`, `statCalls int`, `statPaths []string`. `fakeEditorRunner` implements `EditorRunner` with: `runCalls int`, `lastFilePath string`, `lastCb ExecCallback`. All fields protected by `sync.Mutex`."
- **Resolved by:** edit applied to plan (Testing Strategy)
- **Raised in round:** R1
- **Changed in plan:** Testing Strategy.

### F-PR2-28 — rejectShellMeta test matrix incomplete

- **Agent:** test-engineer (TST-PR2-PR-005)
- **Category:** Test specification
- **Finding:** D-PR2-6 changes set; only `$` accepted is named in tests. `&`/`<`/`>` acceptance and `|`/`;`/backtick/newline rejection have no named tests.
- **Resolution:** Add four named tests: (1) `TestResolveEditor_VisualWithDollar_Accepted`; (2) `TestResolveEditor_VisualWithAmpersand_Accepted`; (3) `TestResolveEditor_VisualWithPipe_Rejected`; (4) `TestResolveEditor_VisualWithSemicolon_Rejected`. Land in WU-PR2-4.
- **Resolved by:** edit applied to plan (Testing Strategy; WU-PR2-4)
- **Raised in round:** R1
- **Changed in plan:** Testing Strategy.

### F-PR2-29 — pendingAction dispatch tests unenumerated

- **Agent:** test-engineer (TST-PR2-PR-006)
- **Category:** Test specification
- **Finding:** D-PR2-15 introduces 4 pendingAction values but Testing Strategy doesn't enumerate per-value tests.
- **Resolution:** Add to Testing Strategy: "`pendingAction` dispatch tests (4 minimum): (1) `TestModel_PendingAction_Quit_OpensQuitConfirm`; (2) `TestModel_PendingAction_New_OpensNewChoice`; (3) `TestModel_PendingAction_Open_OpensPathPicker`; (4) `TestModel_PendingAction_ClearedOnFatals`."
- **Resolved by:** edit applied to plan (Testing Strategy)
- **Raised in round:** R1
- **Changed in plan:** Testing Strategy.

### F-PR2-30 — D-PR2-19 pre-dispatch test for swallow-bug not named

- **Agent:** test-engineer (TST-PR2-PR-007)
- **Category:** Test specification
- **Finding:** Mode 21 covers "saveCompleteMsg arrives during DialogSaveInProgress" but the test must explicitly start with the dialog open and assert the message is NOT swallowed.
- **Resolution:** Add `TestModel_Mode_21_SaveCompleteMsgWhileDialogOpen_RoutesCorrectly` to Testing Strategy: setup `m.dialog.kind = DialogSaveInProgress`, inject `saveCompleteMsg{err: nil}`, assert `m.dialog.kind` transitions away from `DialogSaveInProgress`.
- **Resolved by:** edit applied to plan (Testing Strategy)
- **Raised in round:** R1
- **Changed in plan:** Testing Strategy.

### F-PR2-31 — DialogKind handlers need both render and Update tests

- **Agent:** test-engineer (TST-PR2-PR-010)
- **Category:** Test specification
- **Finding:** WU-PR2-8 verification names `dialogs_render_test.go` (render only). The four new dialog handlers (FileConflict, FirstSaveConfirm, CrashTempNotice, Recovery) need behavioral Update tests for their keyboard inputs.
- **Resolution:** Add to Testing Strategy / WU-PR2-8: per-handler Update tests including (1) `TestModel_DialogFileConflict_Overwrite_SetsBypassFlag`; (2) `TestModel_DialogFileConflict_Reload_ReloadsAndClearsState`; (3) `TestModel_DialogCrashTempNotice_Discard_RemovesTempFile_AssertsContainmentBeforeRemove`; (4) `TestModel_DialogRecovery_OpenInEditor_AttemptsReload`.
- **Resolved by:** edit applied to plan (Testing Strategy; WU-PR2-8)
- **Raised in round:** R1
- **Changed in plan:** Testing Strategy; WU-PR2-8.

### F-PR2-32 — Linter-clean cherry-pick has no explicit gate

- **Agent:** test-engineer (TST-PR2-PR-009)
- **Category:** Standards conflict
- **Finding:** `lint-and-tooling.md` prohibits any `nolint` suppressions. Cherry-picked code may have findings. Plan does not specify when lint hygiene is verified.
- **Resolution:** Add to WU-PR2-0 description: "Triage table annotates each `as-is` symbol with lint-hygiene status (`golangci-lint run` output for cherry-picked files). Findings are corrected in the cherry-pick commit (WU-PR2-3), not via nolint suppressions."
- **Resolved by:** edit applied to plan (WU-PR2-0; WU-PR2-3)
- **Raised in round:** R1
- **Changed in plan:** Decomposition and Sequencing > WU-PR2-0; > WU-PR2-3.

### F-PR2-33 — Coverage threshold not committed

- **Agent:** test-engineer (TST-PR2-PR-008)
- **Category:** Test scope
- **Finding:** `make ci` passes when tests pass; doesn't fail on uncovered code. New 4140 LOC could ship with major paths uncovered.
- **Resolution:** Add to Definition of Done: "test-engineer review of the 28-mode test set is a merge blocker (not advisory). Review confirms (1) every `tui-mode-coverage.md` entry maps to a named test that compiles; (2) bundle smoke test drives at least one save cycle; (3) fakeFS per-method counters are present and asserted; (4) `pendingAction` 4-test set present; (5) D-PR2-19 swallow-bug test present."
- **Resolved by:** edit applied to plan (Definition of Done)
- **Raised in round:** R1
- **Changed in plan:** Definition of Done.

### F-PR2-34 — D-PR2-23 time.After Go-version dependency unrecorded

- **Agent:** concurrency-analyst (CV-PR2-PR-007)
- **Category:** Documentation gap
- **Finding:** `time.After` GC behavior changed in Go 1.23+. Plan uses Go 1.26.2 (works) but pattern is version-dependent and the rationale "timer GC'd" misleads contributors copying it elsewhere.
- **Resolution:** D-PR2-23 rationale must add Go-version annotation: "This pattern relies on Go 1.23+ GC of unreferenced `time.After` timers. Below Go 1.23 use `t := time.NewTimer(10*time.Second); defer t.Stop()` with `<-t.C` in the select."
- **Resolved by:** edit applied to plan (D-PR2-23)
- **Raised in round:** R1
- **Changed in plan:** D-PR2-23.

### F-PR2-35 — A-PR2-2 (backup compiles against main) needs WU-PR2-0 verification before WU-PR2-3

- **Agent:** junior-developer (JrF-PR2-004) + adversarial-validator (AdV-PR2-009)
- **Category:** Risk gate
- **Finding:** Plan recommends "Ship as planned" but A-PR2-2 (backup compiles) is unverified, and WU-PR2-0 triage is the verifier. The triage's only gate is "reviewed by software-architect" with no quantitative criteria. The "30% replace" revisit threshold has no specific denominator.
- **Resolution:** Add to WU-PR2-0 description: "Triage table is reviewed and approved by `software-architect` BEFORE WU-PR2-3 is committed. Approval criteria: (a) every backup symbol has a classification; (b) replace-class function count ÷ total function count is computed; (c) if ratio > 0.30, escalate via Open Items for user-acceptance to continue or revert to rewrite-from-scratch strategy. Software-architect approval is a merge-blocking sign-off."
- **Resolved by:** edit applied to plan (WU-PR2-0; D-PR2-1)
- **Raised in round:** R1
- **Changed in plan:** Decomposition and Sequencing > WU-PR2-0; D-PR2-1 revisit criterion.

### F-PR2-36 — workflowDir flag wiring not assigned to a WU

- **Agent:** junior-developer (JrF-PR2-008)
- **Category:** Scope gap
- **Finding:** PR-1's stub has `_ = workflowDir`. After un-hide, `pr9k workflow --workflow-dir <path>` is a documented behavior that auto-opens. No WU assigns the wiring.
- **Resolution:** Add to WU-PR2-4 description: "`runWorkflowBuilder` consumes `workflowDir` flag: resolves via inherited `internal/cli` two-candidate helpers; passes resolved path to `workflowedit.New(deps)` as auto-open directive; on Init triggers File>Open load pipeline. Removes `_ = workflowDir` stub."
- **Resolved by:** edit applied to plan (WU-PR2-4)
- **Raised in round:** R1
- **Changed in plan:** Decomposition and Sequencing > WU-PR2-4.

### F-PR2-37 — Rollback model needs sentence explaining workflowedit unreachable when WU-PR2-13 reverted

- **Agent:** junior-developer (JrF-PR2-009)
- **Category:** Documentation
- **Finding:** Rollback section says "revert WU-PR2-13 hides the surface" but doesn't state why intermediate-WU rollback is unnecessary: workflowedit has no callers when subcommand is hidden.
- **Resolution:** Add to Operational Readiness Rollback: "WU-PR2-3 through WU-PR2-12 land changes to `internal/workflowedit`, which has no callers when WU-PR2-13 is reverted. Reverting WU-PR2-13 alone is sufficient. If a deeper rollback is needed for a specific WU, revert that WU plus all subsequent commits."
- **Resolved by:** edit applied to plan (Operational Readiness)
- **Raised in round:** R1
- **Changed in plan:** Operational Readiness.

### F-PR2-38 — JrF-PR2-001 D-PR2-3 versioning rationale wording (low-priority polish)

- **Agent:** junior-developer (JrF-PR2-001)
- **Category:** Documentation polish
- **Finding:** D-PR2-3's rationale could be clearer about why PATCH is correct vs. MINOR under 0.y.z rules.
- **Resolution:** Defer to user-driven polish; not blocking.
- **Resolved by:** deferred (not blocking; no plan edit)
- **Raised in round:** R1
- **Changed in plan:** —

### F-PR2-39 — JrF-PR2-006 28-mode "all pass" ambiguity

- **Agent:** junior-developer (JrF-PR2-006)
- **Category:** Test specification
- **Finding:** "All 28 pass" means three things; only one is verifiable. If backup mode tests pin buggy behavior they pass for the wrong reason.
- **Resolution:** Combined with F-PR2-23 (test rewrites). The DoD wording "All 28 TUI mode-coverage tests pass with the post-gap-close behavior" disambiguates.
- **Resolved by:** edit applied to plan (Definition of Done; cross-references F-PR2-23)
- **Raised in round:** R1
- **Changed in plan:** Definition of Done.

### F-PR2-40 — JrF-PR2-011 commit-graph: WU-PR2-0 vs version-bump-first conflict

- **Agent:** junior-developer (JrF-PR2-011)
- **Category:** Sequencing clarity
- **Finding:** D-PR2-3 says version bump is "first commit." D-PR2-2 puts WU-PR2-0 first. Conflict in wording.
- **Resolution:** Update D-PR2-2 commit graph: "Sequence: (0) WU-PR2-0 docs-only triage commit (planning artifact, may land on PR branch's first commit OR be merged separately first); (1) WU-PR2-1 version bump 0.7.2 → 0.7.3; (2) WU-PR2-2 schema additions; (3-13) gap closures + un-hide." The relevant "first" is "first code commit" — the version bump remains the first code-bearing commit per D-PR2-3.
- **Resolved by:** edit applied to plan (D-PR2-2; D-PR2-3)
- **Raised in round:** R1
- **Changed in plan:** D-PR2-2; D-PR2-3.

### F-PR2-41 — JrF-PR2-012 `~30 functional-completeness` references ephemeral /tmp files

- **Agent:** junior-developer (JrF-PR2-012)
- **Category:** Documentation accuracy
- **Finding:** D-PR2-11's functional-completeness list cross-references `/tmp/pr9k-pr2-planning/round1-specialist-outputs.md` (ephemeral). Reviewers cannot verify "all SEC findings closed" from the repo.
- **Resolution:** Move the round-1 specialist output and round-2 outputs into the artifacts/ folder of the planning dir as durable record. Update D-PR2-11 to reference those committed paths.
- **Resolved by:** edit applied to repo (commit `/tmp/pr9k-pr2-planning/*` to `docs/plans/workflow-builder-pr2/artifacts/round1-specialist-outputs.md` and `round2-outputs-and-resolutions.md`); D-PR2-11 updated
- **Raised in round:** R1
- **Changed in plan:** D-PR2-11; new artifacts committed to repo.

### F-PR2-42 — `$()` rationale unrecorded in D-PR2-6

- **Agent:** adversarial-security-analyst (SEC-PR2-005)
- **Category:** Documentation
- **Finding:** Plan does not document why `$()` (command substitution) is safe under the narrowed rejection set; future contributor may reintroduce `$` rejection.
- **Resolution:** Add to D-PR2-6 rationale: "Command-substitution forms (`$()`) are either expanded by the user's shell before pr9k reads `$VISUAL` (inert at read time) or are literal strings `exec.Command` does not interpret. Backtick remains rejected as defense-in-depth footgun-prevention. Revisit criterion: if any future code path reads `$VISUAL` from a file rather than `os.Getenv`, reassess `$()` as an active vector."
- **Resolved by:** edit applied to plan (D-PR2-6)
- **Raised in round:** R1
- **Changed in plan:** D-PR2-6.

### F-PR2-43 — EBI-PR2-006 OI-4 wording overstates PR-1 delivered scope

- **Agent:** evidence-based-investigator (EBI-PR2-006)
- **Category:** Documentation accuracy
- **Finding:** OI-4 said "after PR-1, `pr9k workflow` shows the empty-editor state." PR-1 delivered a stub returning nil. PR-2 plan inherits OI-4 framing.
- **Resolution:** Add to PR-2 plan §Source Specification: "Note: OI-4 anticipated PR-1 would show an empty-editor state; PR-1 delivered a Hidden:true stub returning nil. PR-2 must therefore deliver both the empty-editor state AND the edit machinery."
- **Resolved by:** edit applied to plan (Source Specification)
- **Raised in round:** R1
- **Changed in plan:** Source Specification.

### F-PR2-44 — EBI-PR2-004 (DialogKind 14) verifies clean

- **Agent:** evidence-based-investigator (EBI-PR2-004)
- **Category:** Verification pass
- **Finding:** Backup `dialogs.go` confirmed 14 named DialogKind constants. D-PR2-7 is correct.
- **Resolution:** No plan change.
- **Resolved by:** verified clean; no plan change
- **Raised in round:** R1
- **Changed in plan:** —

### F-PR2-45 — EBI-PR2-005 (companion key convention) verifies clean

- **Agent:** evidence-based-investigator (EBI-PR2-005)
- **Category:** Verification pass
- **Finding:** Validator and load.go both use `filepath.Join("prompts", step.PromptFile)`. D-PR2-12 closure of GAP-036 is correct.
- **Resolution:** No plan change.
- **Resolved by:** verified clean; no plan change
- **Raised in round:** R1
- **Changed in plan:** —

### F-PR2-46 — EBI-PR2-007 (version 0.7.2) verifies clean

- **Agent:** evidence-based-investigator (EBI-PR2-007)
- **Category:** Verification pass
- **Finding:** version.go is at 0.7.2 as claimed.
- **Resolution:** No plan change.
- **Resolved by:** verified clean
- **Raised in round:** R1
- **Changed in plan:** —

### F-PR2-47 — EBI-PR2-009 (CV-PR2-005 framing) — diagnosis correct, framing imprecise

- **Agent:** evidence-based-investigator (EBI-PR2-009)
- **Category:** Verification pass
- **Finding:** D-PR2-19's solution is correct; the gap analysis of "tier-2 silently swallows" is the precise mechanism. No plan change.
- **Resolution:** No change beyond F-PR2-5 already addressing the underlying issue.
- **Resolved by:** verified; covered by F-PR2-5
- **Raised in round:** R1
- **Changed in plan:** —

### F-PR2-48 — EBI-PR2-010 (28 entries) verifies clean

- **Agent:** evidence-based-investigator (EBI-PR2-010)
- **Category:** Verification pass
- **Finding:** tui-mode-coverage.md has exactly 28 entries.
- **Resolution:** No plan change.
- **Resolved by:** verified clean
- **Raised in round:** R1
- **Changed in plan:** —

### F-PR2-49 — AdV-PR2-003 deepCopyDoc must classify as `replace` to copy new fields

- **Agent:** adversarial-validator (AdV-PR2-003)
- **Category:** Cherry-pick scope
- **Finding:** Backup `deepCopyDoc` copies only old fields. After WU-PR2-2 adds new fields, the cherry-picked `deepCopyDoc` silently drops them on the validation copy.
- **Resolution:** Update WU-PR2-3 description: "`deepCopyDoc` MUST be classified `replace` in the WU-PR2-0 triage table — it must be modified to copy `UnknownFields`, top-level `Env`, top-level `ContainerEnv`, and step-level `ContainerEnv`. Add `TestDeepCopyDoc_PreservesAllFields_RoundTrip`."
- **Resolved by:** edit applied to plan (WU-PR2-3; D-PR2-1 triage table notes)
- **Raised in round:** R1
- **Changed in plan:** Decomposition and Sequencing > WU-PR2-3.

### F-PR2-50 — D-PR2-9 spec §8 update timing relative to mode tests

- **Agent:** adversarial-validator (cluster — referenced as remaining risk)
- **Category:** Sequencing
- **Finding:** D-PR2-9 commits to spec §8 wording update. Tests for `?` suppression behavior land in WU-PR2-9. If spec update lands later, spec and test temporarily disagree.
- **Resolution:** Land spec §8 update in WU-PR2-11 (the doc commit) but reference D-PR2-9 in WU-PR2-9's test code with a comment pointing to the pending spec update. Acceptable transient inconsistency.
- **Resolved by:** edit applied to plan (D-PR2-9)
- **Raised in round:** R1
- **Changed in plan:** D-PR2-9.

### F-PR2-51 — F-118 T2 enumeration: VISUAL with unquoted space not covered

- **Agent:** test-engineer (TST-001 in PR-1 R2)
- **Category:** Test specification
- **Finding:** Plan references F-118 T2 8-case matrix but doesn't add a 9th case for unquoted-path-with-space (`VISUAL=/Applications/Sublime Text/subl`).
- **Resolution:** Add `TestResolveEditor_UnquotedPathWithSpace_RejectsWithGuidance` to WU-PR2-4.
- **Resolved by:** edit applied to plan (Testing Strategy; WU-PR2-4)
- **Raised in round:** R1
- **Changed in plan:** Testing Strategy.

### F-PR2-52 — VISUAL/EDITOR both empty: not covered

- **Agent:** test-engineer (TST-002 in PR-1 R2)
- **Category:** Test specification
- **Finding:** "Neither set" case covered; "both set to empty" not.
- **Resolution:** Add `TestResolveEditor_VisualEmptyEditorEmpty_SameGuidanceAsNeitherSet`.
- **Resolved by:** edit applied to plan (Testing Strategy)
- **Raised in round:** R1
- **Changed in plan:** Testing Strategy.

### F-PR2-53 — Pre-implementation: WU-PR2-0 triage table is itself an artifact, not just a planning step

- **Agent:** adversarial-validator (AdV-PR2-009)
- **Category:** Process gate
- **Finding:** Triage table needs to be a reviewable artifact, not just an in-PR commit.
- **Resolution:** WU-PR2-0 commits a `docs/plans/workflow-builder-pr2/artifacts/cherry-pick-triage.md` artifact containing the function-level classification; software-architect reviews via PR comment. Already implicit in WU-PR2-0 description; explicit acknowledgment added.
- **Resolved by:** edit applied to plan (WU-PR2-0)
- **Raised in round:** R1
- **Changed in plan:** Decomposition and Sequencing > WU-PR2-0.

### F-PR2-54 — Mode 25 (`tea.Quit`) test approach must explicitly type-switch on `tea.QuitMsg`

- **Agent:** test-engineer (TST-PR2-PR-001)
- **Category:** Test specification
- **Finding:** Mode 25 asserts "program exit" — that requires invoking the returned `tea.Cmd` and checking the returned message is `tea.QuitMsg`.
- **Resolution:** Add to Testing Strategy: "Mode 25 assertion calls the returned `tea.Cmd` and type-switches on `tea.QuitMsg`. Mode 22/23/28 assertions call `m.View()` and check rendered string for option labels."
- **Resolved by:** edit applied to plan (Testing Strategy)
- **Raised in round:** R1
- **Changed in plan:** Testing Strategy.

### F-PR2-55 — D-PR2-13 NFS rationale incorrectly claims ctx cancellation works

- **Agent:** concurrency-analyst (CV-PR2-PR-004)
- **Category:** Decision precision
- **Finding:** D-PR2-13 says ctx propagation "preserves correctness on slow filesystems where a stuck save closure would otherwise hold a goroutine past program exit." But blocking syscalls (NFS-hung os.Rename) do not respond to ctx.Done — only OS-level fd close interrupts. ctx provides cancellation only at Go select points between syscalls.
- **Resolution:** Update D-PR2-13 rationale: "ctx propagation provides cancellation at Go select points between syscalls — it does NOT interrupt in-progress blocking syscalls on NFS/FUSE. On a hung NFS mount, the goroutine waits for the syscall to return; the bounded `wg.Wait()` (D-PR2-22) provides at most a 4-second extra wait before runWorkflowBuilder returns. This is an accepted residual; documented as RAID Assumption A-PR2-NFS."
- **Resolved by:** edit applied to plan (D-PR2-13; RAID Assumptions)
- **Raised in round:** R1
- **Changed in plan:** D-PR2-13; RAID > Assumptions.

### F-PR2-56 — D-PR2-7 update timing — must land with cherry-pick of dialogs.go

- **Agent:** evidence-based-investigator + software-architect (R2 from prior plan-implementation)
- **Category:** Decision sequencing
- **Finding:** D-PR2-7 updates D-8 to enumerate 14 constants. The update is a docs change to the original planning artifact, not a code change. Plan should clarify it's an artifact-only update.
- **Resolution:** D-PR2-7 description clarifies: "This decision corrects the original D-8's count to 14; the correction lands as a plan-artifact edit, not a code change. The DialogKind constants in `dialogs.go` are cherry-picked as-is in WU-PR2-3."
- **Resolved by:** edit applied to plan (D-PR2-7)
- **Raised in round:** R1
- **Changed in plan:** D-PR2-7.

### F-PR2-57 — Test scope: `TestModel_*` test count budget per WU

- **Agent:** test-engineer (cluster TST-PR2-PR-006/007/008)
- **Category:** Test scope
- **Finding:** Plan commits to many tests across many WUs but doesn't budget the count or assert the total.
- **Resolution:** Add to Testing Strategy: "PR-2 ships approximately 65–80 new tests across `internal/workflowedit` (28 mode tests + 12 EC tests + 14 dialog render+update + 8 pendingAction + 4 D-PR2-19 routing + 4 misc) plus ~10 in `cmd/pr9k/editor_test.go` (T2 matrix + new). Coverage of each named test in this plan is verified by the test-engineer DoD review."
- **Resolved by:** edit applied to plan (Testing Strategy; DoD)
- **Raised in round:** R1
- **Changed in plan:** Testing Strategy; Definition of Done.

### F-PR2-58 — Test for goroutine ownership matrix needs `TestWorkflow_GoroutineLifecycle_*`

- **Agent:** concurrency-analyst (CV-PR2-PR-008)
- **Category:** Test specification
- **Finding:** Goroutine ownership matrix (F-PR2-18) needs at least one test that exercises the drain path under SIGINT.
- **Resolution:** Add `TestWorkflow_SIGINT_DrainsAllGoroutines_WithTimeout` to WU-PR2-5: spawn a fake-blocking save closure, send SIGINT, assert `runWorkflowBuilder` returns within 5 seconds (not hanging).
- **Resolved by:** edit applied to plan (Testing Strategy; WU-PR2-5)
- **Raised in round:** R1
- **Changed in plan:** Testing Strategy.

---

## Summary

- **Findings raised:** 58 (F-PR2-1..58).
- **Resolved by edit applied to plan:** 49.
- **Resolved by verifies clean (no change):** 5 (F-PR2-44..48).
- **Resolved by deferred / non-blocking:** 1 (F-PR2-38).
- **Resolved by repo artifact commit:** 1 (F-PR2-41).
- **Cross-references / consolidated:** 2 (F-PR2-39 cross-refs F-PR2-23; F-PR2-47 cross-refs F-PR2-5).
- **Categories with most findings:** Concurrency (8), Test specification (15), Architecture/sequencing (12), Security (7), Decision precision (8), Documentation (8).
- **Cluster of overlapping findings:** F-PR2-1 (IsDirty+UnknownFields) consolidates JrF-003+AdV-002; F-PR2-3 (WaitGroup architecture) consolidates AdV-006+CV-001; F-PR2-13 (WU-PR2-9 too large) consolidates JrF-005+AdV-010; F-PR2-14 (constructor) consolidates JrF-002+EBI-001+AdV-001.

The plan is structurally sound but has 49 specific edits required before implementation begins. After R1 edits land, an R2 round would primarily produce cosmetic refinements; the convergence threshold (≥80% chance of meaningful structural change) is below the threshold. **Recommend stopping at R1.**

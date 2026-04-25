# PR-2 Workflow Builder — Round 1 Specialist Outputs (verbatim)

This file consolidates the verbatim output of every Round 1 specialist for the project-manager to facilitate over.

---

## UX (user-experience-designer) — 10 findings

**UX-001: Dialog rendering is placeholder-only — D48 uniform contract is never visible.** GAP-045/046, spec#dialog-conventions, D48, D73. Every `DialogKind` is a one-line placeholder. Quit-confirm has `Enter`→Yes/exit (opposite of D73's `Enter`-cancels). Plan must commit to full-fidelity rendering for every DialogKind: multi-line body+title, D56 four-element template, visible option-cursor, spatial primary-safe-first, correct Enter-default wiring; tested per kind.

**UX-002: Focus restoration after overlay close has no verified implementation path.** D55, D-8 `prevFocus focusTarget` depth-1. Plan never specifies the `focusTarget` type, capture site, clear site, or coexistence rule for help-modal-over-findings-panel. Tests would pass trivially with both nil. Plan must define `focusTarget` concretely; specify capture-once / clear-once rule; test the help-over-findings sequence preserves focus throughout.

**UX-003: Session-header banner-priority model (D49) has no committed rendering contract.** GAP-005, spec#primary-flow §5, D49. Risk: implemented as a single string slot rather than priority queue → wrong banner shown. A read-only browse-mode user who doesn't see the read-only banner attempts Save with no feedback. Plan must define the priority data structure (bitmask / ordered slice / struct+ActiveBanner method); specify the `[N more warnings]` panel as a DialogKind; test three-banner-active load shows correct single banner.

**UX-004: Choice-list dropdown (D12, D45) has no rendering specification.** GAP-015. Plan names keyboard contract but never the visual shape. Inline expansion vs floating overlay vs separate viewport unspecified. Mode-coverage rows can't be written. Plan should specify inline-expansion (lower complexity); rendering of option cursor; clip/scroll behavior; commit to specific rendering artifact for mode-coverage row 15.

**UX-005: Secret masking applies to all containerEnv values regardless of key pattern.** GAP-017, D20/D47. Current code masks every value (NODE_ENV=production shows ••••). Re-mask-on-focus-leave is on dropdown-close only, not focus-change. Plan must specify key-pattern matching helper; specific focus-leave event triggering re-mask; `[press r to reveal]` label only for matched keys; test reveal→focus-leave→remasked transition.

**UX-006: Model-field suggestion list (D12/D58) has no committed interaction contract.** GAP-018, D-42. ModelSuggestions exists in workflowmodel but no rendering, navigation, or dismissal specified. Differs from choice-list because doesn't constrain values. Plan must specify trigger condition; navigation (↓ moves into list, Enter picks, Esc collapses); free-text acceptance for unknown values; new mode-coverage row.

**UX-007: Path picker shared by File>New / File>Open with different pre-fills, button labels, inline guidance — no isolation in dialogState.** GAP-019, D71, D-25. New = `<projectDir>/.pr9k/workflow/`+"Create" button; Open = `…/config.json`+"Open" button. Different inline notes. Plan must extend `dialogState.payload` with PickerKind enum; reset input on every open; specify inline-note rendering location.

**UX-008: Phase-section structure, `+ Add` affordance, section-collapse are entirely absent.** Section 3 of gaps, GAP-010, GAP-013, D28, D29, D46, D51. Outline shows flat list with no landmarks. `+ Add` row is the only discoverable extension path. Plan must add mode-coverage rows for: section-header focus, `+ Add` row Enter, section collapse via toggle, `a` shortcut. Section header type must be a distinct outline row kind.

**UX-009: Unsaved-changes auto-resume (D72) not broken into the three triggering flows.** GAP-020, spec#primary-flow §3a/4a/§10. Current code uses bool `pendingQuit` only — Discard from File>New quits the builder. Plan must define `pendingActionKind` discriminated type (`pendingNone/Quit/New/Open`); store picker-context for resume; mode-coverage rows for unsaved-save-success-resumes-New and -Open.

**UX-010: Numeric-field input (D62) absent; three behaviors (digit-filter, range-clamp, paste-sanitized message) need to be separable.** GAP-016, D62. Bubble Tea's paste-event detection is non-portable. If implementation treats multi-char as keystrokes, the "pasted content sanitized" message never fires. Plan must specify paste-detection mechanism (`tea.Paste` if available; else heuristic `len(Runes)>1`); commit to three separately testable behaviors; specify message render location and auto-dismiss.

---

## Security (adversarial-security-analyst) — 11 findings

**SEC-001: ExecCallback three-way switch implemented but disconnected from Model (D-7, GAP-031).** `realEditorRunner.Run` does NOT call `makeExecCallback()` — passes the caller-supplied `cb` directly. `Update` has zero cases for editor message types. Even if wired, Model would see raw error not typed messages. Severity: High — D-7 SIGINT branch silently inoperative; spurious-quit-dialog race uncovered. Plan must commit Run uses makeExecCallback internally, Update has cases for editorExitMsg/editorSigintMsg/editorRestoreFailedMsg.

**SEC-002: realEditorRunner.Run resolution error returns editorRestoreFailedMsg, not editor-binary error dialog.** editor.go:36-41. Resolution errors (pre-TTY-handoff) funneled through same type as post-handoff RestoreTerminal failures — Model shows "terminal may be degraded — run reset" when user just forgot to set $VISUAL. Plan must distinguish editorResolveErrMsg from editorRestoreFailedMsg.

**SEC-003: rejectShellMeta rejects `$` but D33 explicitly forbids that.** editor.go:85-93. Spec D33: "Reject `$`… rejected because direct exec does not interpret `$`. Legitimate `VISUAL='$HOME/bin/myvim'` must work." Implementation adds `$` to rejection set — a false-positive DoS on spec-allowed editor values. Plan must remove `$`; rejection set must be: ` ` `, `;`, `|`, `&`, `<`, `>`, `\n`. Add positive test TestResolveEditor_VisualWithDollar_Accepted.

**SEC-004: CreateEmptyCompanion has zero callers in the TUI; D-21 entirely unimplemented.** GAP-029, GAP-021, D-21. Containment function exists and is correct — the gap is wiring. PR-2 must commit detail-pane editor-open handler calls workflowio.CreateEmptyCompanion before invoking m.editor.Run for non-existent prompt files; test must verify DI seam invoked. Severity: High (R9 is High in RAID).

**SEC-005: ANSI stripping in recovery view — D-23 references statusline.Sanitize but D-45 introduces internal/ansi.StripAll; plan internally inconsistent.** F-94, D-23, D-45. statusline.Sanitize preserves OSC 8; StripAll drops it. Plan internally inconsistent — D-23 still says "Reuse statusline.Sanitize"; the fix lives in D-45. Update D-23 to reference internal/ansi.StripAll; commit TUI loads stripped recovery content from workflowio.Load result; test pins OSC 8 input through full pipeline.

**SEC-006: Signal handler sends `program.Send(quitMsg{})` only in a comment — D-34's protection against terminal corruption is entirely absent.** GAP-003, D-34. workflow.go:64-67 has `// program.Send(quitMsg{}) — wired when workflowedit is added`. After 10s `os.Exit(130)` fires without RestoreTerminal — exactly the R5 Critical scenario. Plan must wire program.Send(quitMsg{}); test must verify signal handler holds live tea.Program reference. Severity: Critical.

**SEC-007: Session-event logging helpers exist; Model has no logger field; no call site invokes any helper.** GAP-033, D-27. Format helpers exist but are dead code. Risk R7 (containerEnv secret leak) unmitigated at runtime. Plan must: (1) add *logger.Logger to Model via workflowDeps; (2-7) wire log.Log("workflow-builder", ...) at every D39 trigger point; (8) regex test no log line contains containerEnv value pattern.

**SEC-008: rejectShellMeta omits `&`, `<`, `>` that D33 includes.** editor.go:85-93. Implementation rejects backtick, `;`, `|`, `$`, `\n` — wrong set. Inert under direct exec but spec-rejected as defense-in-depth signal. Plan must align rejection set to D33; tests TestResolveEditor_VisualWithShellMetacharacters_Rejected covers full D33 set.

**SEC-009: editorFirstToken in session_log.go uses path parsing rather than shlex — inconsistency with how resolveEditor splits.** session_log.go:62-79, D-27. For VISUAL="'/Applications/Sublime Text/subl' --wait" produces `subl'` — log inaccurate (not a leak). Plan: fmtEditorOpened should extract from already-resolved tokens[0] not re-parse raw env.

**SEC-010: validateCommandPath EvalSymlinks applies only to relative command paths; absolute commands bypass containment.** GAP-042. Plan §"OI-1 containment precision" says both apply EvalSymlinks but implementation applies selectively. Could allow `command: ["/etc/cron.d/pwned"]` past validator. Plan must explicitly decide: absolute paths exempt (and document) vs checked. Severity: High.

**SEC-011: Hidden:true with non-functional RunE creates a silent no-op surface.** GAP-002, spec#versioning. Un-hiding subcommand stub creates silent exit-0 on `pr9k workflow --workflow-dir /etc/`. Plan: do not un-hide until tea.NewProgram is wired; rename-guard test must extend to assert Hidden:false after un-hide commit.

---

## DevOps (devops-engineer) — 10 findings

**DOR-001: Version bump is stale.** versioning.md, D37, D-18. PR-1 shipped 0.7.2; PR-2 expands public API. P0: bump to 0.7.3 as first commit of PR-2 branch. P1: CI gate for version-bump-as-first-commit. P2: rename-guard parses version.go and asserts not identical to main if Hidden:false.

**DOR-002: Un-hide is not an atomic, independently reversible commit.** Rollback story = "revert PR merge commit" = 6-8k LOC reversion. Plan should structure un-hide as named commit AFTER WU-7 wiring, before merge — single-commit cherry-pick revert is the rollback boundary. Add WU-13 entry to Decomposition table with verification gate `pr9k --help | grep workflow`.

**DOR-003: Session-event logging trigger points absent.** D-27, GAP-033, GAP-034. Format helpers exist; Model has no logger field; all 9 D27 trigger points unwired. Without `session_start` event, operator cannot distinguish "crashed before save" from "fatal validation" from "user quit." R7 mitigation untested. P0: wire logger via workflowDeps; minimum session_start + quit unsaved=<bool>. P1: regex-pin unit test on log-line shape (R7 mitigation gate).

**DOR-004: D-44 log directory fallback has silent gap for external-workflow case.** D-44, workflow.go:83-94. resolveBuilderLogBaseDir takes only projectDirFlag. After PR-2 wires --workflow-dir, accidentally passing workflowDir to the function would scatter logs across unrelated dirs. Add comment block; rename param to `projectDir`; test workflowDir != logBaseDir.

**DOR-005: --workflow-dir flag accepted by PR-1 but silently ignored (`_ = workflowDir`).** workflow.go:32, GAP-001. After un-hide, `pr9k workflow --workflow-dir /some/path` will appear to work but launch empty-editor state — opposite of D3 spec. Documentation lies. P0: PR-2 must consume workflowDir; resolve via cli helper; pass to Model as auto-open directive; trigger File>Open load pipeline.

**DOR-006: Rename-guard tests pass against current stub — pass trivially after un-hide too.** GAP-051, F-116. P0: extend rename_guard with assertNotContains(content, "Hidden:\ttrue") and assertContains(content, "tea.NewProgram"). P1: DoD checklist must list these guards. P2: companion subprocess test running `pr9k --help` and grep workflow.

**DOR-007: Session-log format helpers and D-27 field-exclusion contract have no test pinning excluded fields — R7 untested.** D-27, RAID R7, plan WU-10. P0: unit tests for every fmt* helper that pass a containerEnv with `_TOKEN` and assert returned string does not contain the value. P1: SaveErrorKind→reason mapping as compile-time exhaustive switch with default fallback; test asserts switch covers every constant.

**DOR-008: workflowedit package does not exist — PR-2's full DoD is cold-start delivery.** OI-4. The internal packages on main exist; workflowedit (WU-7..WU-10) does not. PR-2 must create from scratch: 11 source files, 28 TUI mode-coverage tests, 12 edge-case tests, save state machine, session-log integration, path picker, findings panel. Per OI-4 the split was designed as un-hide gate. P0: structure commit sequence so un-hide and version bump are last two commits — PR can be reviewed/tested with Hidden:true still in place. P1: bundle-builder integration test driven through Model end-to-end.

**DOR-009: Feature flag posture correct but DoD has no automated CI gate that `pr9k --help` lists `workflow`.** P0: TestNewWorkflowCmd_NotHidden asserting cmd.Hidden == false. P1: make ci runs `./bin/pr9k --help` subprocess and greps for `workflow`. P2: tag-creation workflow asserts all subcommands present.

**DOR-010: Cost/scale — no findings. Local interactive dev tool with no SLO/cloud/network. The plan's §Cost and scale and §SLO impact are correctly stated as zero.** No new operational risk.

---

## Software Architecture — 10 findings

**R-PR2-001: File decomposition matches D-1.** Backup tag has model.go (988 lines) plus dedicated outline.go, detail.go, menu.go, footer.go, dialogs.go, pathpicker.go, session_log.go, findings.go, constants.go, editor.go, messages.go. The 988 lines are the tea.Model behavioral core — widget rendering correctly delegates to sub-files. deepCopyDoc and snapshotCompanions are correctly co-located. **Verdict: matches plan; no adjustment.**

**R-PR2-002: EditorRunner interface shape correct; ExecCallback diverges from D-6 pseudocode in beneficial direction.** Backup defines `type ExecCallback func(err error) tea.Msg` and `Run(filePath, cb ExecCallback) tea.Cmd`. Promotes callback to message-producer; three-way switch lives in cmd/pr9k where the message types are defined; workflowedit never references concrete message types. DIP preserved. Plan: update D-6 pseudocode to match shipped form.

**R-PR2-003: SaveFS interface present in PR-1's shipped workflowio — no retroactive refactor needed.** workflowio/save.go on main lines 19-25 declares SaveFS interface with WriteAtomic and Stat; RealSaveFS() returns realSaveFS. Backup tag identical. Confirmed. No plan adjustment needed.

**R-PR2-004: Update routing order matches D-9 exactly.** Backup model.go:102-115 implements helpOpen→dialog→isGlobalKey→default. isGlobalKey passes only F10/Ctrl+N/O/S/Q (no Del — honors D-10). helpOpen=true set at exactly two sites: inside DialogFindingsPanel handler, and inside handleEditKey only when no dialog and !helpOpen. D-8 honored. Recommend updating struct comment from "second-layer overlay; only legal when dialog.kind==DialogFindingsPanel" to also note "or dialog.kind==DialogNone (help from edit view)".

**R-PR2-005: workflowedit does not import internal/statusline.** Confirmed clean. F-99 honored.

**R-PR2-006: go.mod requires one new direct dependency — github.com/google/shlex.** Used exclusively in cmd/pr9k/editor.go (resolveEditor) per D-22 D-33. All other bubbles/viewport, tea.ExecProcess, tea.SetWindowTitle uses satisfied by existing pins.

**R-PR2-007: Public surface of workflowedit correctly minimized.** Exports: Model, New, WithSharedInstallWarning, ShortcutLine, EditorRunner, ExecCallback, DialogKind+15 constants. cmd/pr9k imports workflowedit.New, workflowio.RealSaveFS(), workflowedit.EditorRunner. Concrete editor message types stay in cmd/pr9k. PR-2 must complete signal-handler `program.Send(quitMsg{})` (GAP-003).

**R-PR2-008: Dialog constant set diverges from D-8 spec.** D-8 lists 14; backup has 15. Absent from backup vs D-8: DialogCopyBrokenRef, DialogEditorError. Present in backup vs D-8: DialogSaveInProgress (plan line 165), DialogRecovery, DialogAcknowledgeFindings. The collapse of CopyBrokenRef and EditorError into generic DialogError is safe simplification. Plan: update D-8 to reflect shipped 15-constant set; PR-2 decide whether to complete unhandled stubs (DialogError, DialogCrashTempNotice, DialogFirstSaveConfirm, DialogExternalEditorOpening, DialogFileConflict).

**R-PR2-009: m.editor.Run(...) is never called in the backup — external editor handoff structurally stubbed.** EditorRunner interface and realEditorRunner exist and are wired into New(...) but zero call sites invoke m.editor.Run. DialogExternalEditorOpening has render path but no Update handler. Two-cycle handoff (D-26) not implemented. PR-2 must add openEditorMsg and launchEditorMsg, two-cycle path, editorExitMsg/editorSigintMsg handling. Risk: shipping an interface with zero callers (over-abstraction anti-pattern) if deferred past PR-2.

**R-PR2-010: Update method dispatch size already correctly decomposed.** updateDialog is 30-line dispatch; each dialog kind has own handler (8-30 lines). OCP-compliant. Five fall-through dialog kinds need either complete handlers or render text matching the Esc-only contract (DialogFileConflict currently renders three options with only Esc — UX lie).

---

## Behavioral — 10 findings

**BEH-001: F-97 pendingQuit re-trigger issues save feedback then quits directly — plan specifies re-entry through quit flow.** model.go:771-789. Plan: after saveCompleteMsg with pendingQuit==true, "re-route to handleGlobalKey(Ctrl+Q) which now sees saveInProgress==false and proceeds normally — QuitConfirm or unsaved-changes." Draft does neither. Success path returns tea.Quit directly without quit-confirm. Failure path clears pendingQuit and opens DialogError — quit silently abandoned. Save_flow_test asserts only tea.Quit returned, not DialogQuitConfirm opened first. Plan commitment needed: success path re-enter quit flow (D73) or bypass (implicit confirmation)?

**BEH-002: Editor handoff three-way ExecCallback switch produces message types with no consumer in Model.Update.** updateEditView handles only validateCompleteMsg/saveCompleteMsg/openFileResultMsg/window/key/mouse — no editor messages. Three message types defined in `main` package, package-local. workflowedit.Model.Update would need them in workflowedit (or shared) to type-switch. Plan must commit message ownership boundary.

**BEH-003: Editor invocation has no call site — ExecCallback, openEditorMsg, launchEditorMsg trigger chain doesn't exist.** GAP-029, plan §Runtime Behavior. neither openEditorMsg nor launchEditorMsg declared. m.editor.Run never called. DialogExternalEditorOpening renders placeholder, no Update case. Three call sites needed: detail-pane Enter, footer shortcut, recovery-view "open in external editor".

**BEH-004: Load pipeline discards IsSymlink/SymlinkTarget from workflowio.Load.** GAP-035. workflowio.Load populates fields; makeLoadCmd discards them — openFileResultMsg has no IsSymlink/SymlinkTarget fields. D-23 ordering "symlink detect → banner → parse → recovery view" cannot fire. Plan: openFileResultMsg must carry IsSymlink and SymlinkTarget; handleOpenFileResult must act on them.

**BEH-005: updateDialogAcknowledgeFindings does not write to findingsPanel.ackSet — D23 per-session warning suppression never activates.** GAP-027, GAP-028, model.go:411-426. ackSet populated by buildFindingsPanel across rebuilds, but never written at acknowledgment time. Every save with warning-only findings re-prompts. Test asserts ackSet preserved across rebuilds, not that updateDialog populates it.

**BEH-006: handleOpenFileResult does not reset session-scoped state — reorderMode, findingsPanel.ackSet, helpOpen survive session transitions.** spec D57. handleOpenFileResult resets doc/diskDoc/companions/loaded/dirty/cursor/saveSnapshot. Does NOT reset reorderMode/reorderOrigin/reorderSnapshot/findingsPanel.ackSet/findingsPanel.entries/helpOpen/prevFocus. Cross-session contamination: stale reorderMode on new session routes input keys to reorder handler; stale ackSet suppresses warnings from a different workflow.

**BEH-007: DialogFileConflict and DialogFirstSaveConfirm declared and opened but no Update handlers — Esc-only escape.** GAP-022, GAP-023. handleGlobalKey opens DialogFileConflict on mtime mismatch; updateDialog has no case → falls to default Esc-only handler. Pressing `o` (overwrite) or `r` (reload) has no effect — infinite loop of detect-and-dismiss. Both dialog types need explicit updateDialog cases before being wired.

**BEH-008: workflowmodel.IsDirty is never called — dirty state tracked via ad-hoc m.dirty=true mutations.** Plan §Data Model. Eight scattered mutation sites manually set m.dirty=true. diskDoc maintained alongside doc precisely to enable IsDirty(diskDoc, doc) but never invoked. Two divergable dirty signals. Class of invariant violations: any path that forgets m.dirty=true silently allows dirty workflow to appear clean.

**BEH-009: DialogUnsavedChanges has no pendingAction field — File>New and File>Open after unsaved changes can't resume after Save or Discard.** GAP-020, D72. updateDialogUnsavedChanges handles only Ctrl+Q via existing pendingQuit bool. Discard branch unconditionally calls tea.Quit — Discard from File>New quits the builder. No mechanism to distinguish "user was doing File>New" from "user was doing File>Quit". Plan must add `pendingAction` enum.

**BEH-010: Session-event logging helpers exist but Model has no logger field — D27 event catalog unreachable.** GAP-033. All nine fmt* helpers in session_log.go return correct strings. Model struct has no logger field. No call to any helper anywhere in model.go. New(saveFS, editor, projectDir, workflowDir) signature has no logger. Plan must inject via constructor parameter or WithLogger setter.

---

## Concurrency — 9 findings

**CV-PR2-001: Signal handler does not hold a program reference — race hazard in capture pattern.** workflow.go:62-73. PR-2 must introduce *tea.Program. Three shapes: (a) capture in local var before goroutine launches (safe); (b) shared variable written after launch (race); (c) atomic.Pointer (overkill). Plan must commit explicitly. SIGINT can arrive between goroutine launch and tea.NewProgram assignment.

**CV-PR2-002: ctx-cancellation cannot reach tea.Cmd goroutines — scanMatches and save/validate closures have no cancellation path.** D-33. tea.Cmd closures run in Bubble Tea goroutines; ctx not injected. NFS os.ReadDir blocking 30s keeps goroutine alive after Bubble Tea exit. Plan must commit Option A (accept Bubble Tea-owned goroutines exempt; document) or Option B (pass ctx into closures with select on ctx.Done at blocking points). Backup implements A; D-33 implies B.

**CV-PR2-003: saveInProgress and validateInProgress flags safe only if Update is sole writer — plan must pin invariant explicitly.** model.go (eight read/write sites all in Update handlers). All inside Model.Update which is single-goroutine. Tests directly set m.saveInProgress=true (test-only). Plan must include explicit note: "these flags are read and written exclusively inside Model.Update; no tea.Cmd closure, no goroutine, no signal handler may write them."

**CV-PR2-004: saveSnapshot must be set BEFORE saveInProgress cleared in handleSaveResult — order inversion creates TOCTOU window.** model.go:771-790. D-13 step 8 says "Clear saveInProgress. If success, set saveSnapshot=snapshot" — sequence ambiguous. Backup clears first, sets second. Single-threaded Update means safe across frames; logical hazard if next Ctrl+S arrives in same frame (Bubble Tea serializes — practically safe). Plan should pin ordering explicitly.

**CV-PR2-005: validateCompleteMsg routed via updateEditView — swallowed when any dialog is active during async round-trip.** D-9. updateEditView handles validateCompleteMsg/saveCompleteMsg. Dialog opens between makeValidateCmd and validateCompleteMsg → message reaches updateDialog → silently dropped → validateInProgress stays true permanently → all future saves silently consumed. Plan must commit: pre-dispatch validateCompleteMsg/saveCompleteMsg to handlers regardless of dialog state.

**CV-PR2-006: pendingQuit re-triggers quit by re-entering handleGlobalKey — but handleGlobalKey not reached during DialogSaveInProgress routing tier.** model.go:471-478, 765-790. Plan §Quit interaction says "re-route to handleGlobalKey(Ctrl+Q)". Backup directly returns tea.Quit (line 787-788) — no QuitConfirm shown. Plan must state explicitly: after successful save with pendingQuit=true, show DialogQuitConfirm or exit immediately?

**CV-PR2-007: In-flight pathCompletionMsg not cancelled when user types another character — stale completions overwrite live input.** pathpicker.go:35-43, model.go:288-354. Goroutine reads against OLD prefix; user types char extending input; stale pathCompletionMsg arrives and overwrites picker.input with old-prefix match — typed char discarded. Plan must commit generation-counter approach or "discard stale on char input" pattern.

**CV-PR2-008: Post-program.Run() goroutine drain is unspecified — defer cancel() fires before in-flight tea.Cmd goroutines complete.** Concurrency standard §"Wait for background goroutines after program.Run() returns". Plan must commit drain pattern: select{<-workflowDone: <-time.After(4s):} after program.Run(). Without it, on Ctrl+Q during in-flight save the saveCompleteMsg may be lost.

**CV-PR2-009: Signal handler 10s time.AfterFunc leaks across test invocations.** workflow.go:62-73. Timer is persistent across goroutine exit via ctx.Done(). In tests this calls os.Exit(130) later. Plan must commit timer cancelled if ctx.Done() branch fires; D-34 currently silent.

---

## Test (test-engineer) — 12 findings

**TST-001: T2 Matrix — VISUAL with unquoted space not covered.** Plan F-118 enumerates 8 T2 cases. Backup has 12 tests but no test for `VISUAL=/Applications/Sublime Text/subl` (no outer quotes). Add TestResolveEditor_UnquotedPathWithSpace_RejectsWithGuidance.

**TST-002: T2 Matrix — VISUAL empty string vs absent variable distinction.** Backup G-2 covers VISUAL="" with EDITOR="vi". What's not tested: VISUAL="" with EDITOR="" — should trigger same guidance as "neither set". Add TestResolveEditor_VisualEmptyEditorEmpty_SameGuidanceAsNeitherSet.

**TST-003: 28-mode coverage — all 28 entries present in backup model_test.go (764 lines); current branch contains none.** Restore the full file. Risk of partial restore: 28-entry table partially satisfied without visible gap; DoD line 351 misreports.

**TST-004: fakeFS missing per-method call counters — violates docs/coding-standards/testing.md.** Backup helpers_test.go fakeFS has WriteAtomic and Stat but neither has counter or capture slice. Tests cannot independently assert WriteAtomic was called with right path/data. Extend fakeFS with writeAtomicCalls and statPaths counters before restoring tests.

**TST-005: DI-1 and DI-2 are placeholder comments in current doc_integrity_test.go — must be re-activated.** Lines 1001-1004 have comment block. Backup has TestDocIntegrity_FeatureDocLinked, TestDocIntegrity_HowToGuidesLinked active. PR-2 ships the three doc files — un-comment/restore. Also DI-5 needs workflowedit.md added back; DI-8 needs three doc files added back to the scan.

**TST-006: T3 validator integration — companion key convention untested end-to-end in PR-1's workflowvalidate tests.** F-121, GAP-036. Add TestValidate_BareKeyHitsCache_DiskNotRead and TestValidate_FullPathKeyMissesCacheReadsFromDisk.

**TST-007: save_flow_test.go — saveCompleteMsg with pendingQuit=true re-enter-quit-flow branch missing from PR-1 tests.** Backup has TestSaveComplete_WithPendingQuit_ReentersQuitFlow (mode 21). Restore as part of save_flow_test.go.

**TST-008: dialogs_render_test.go — 14 DialogKind constants; backup covers 13 of 14.** Missing render test for DialogCopyBrokenRef (D-61 broken-default-copy). Add TestDialog_CopyBrokenRef_ContainsOptions asserting Copy-anyway/Cancel option text.

**TST-009: bundle_builder_integration_test.go — smoke test does not drive workflowedit.Model.** GAP-044, plan DoD line 350. "Opens cleanly" means: workflowedit.New(fakeFS, fakeEditorRunner, tmpDir, tmpDir) receives openFileResultMsg{...}, transitions from empty-editor to edit-view, m.loaded==true, no panics. Programmatically deliver load result via m.Update; assert state.

**TST-010: Race detector — fakeFS and fakeEditorRunner in helpers_test.go lack sync.Mutex on shared fields.** Testing standard requires mutex on fakes touching async paths. fakeFS.WriteAtomic called inside Bubble Tea goroutine; fakeEditorRunner.runCount incremented inside tea.Cmd closure. Add sync.Mutex before tests exercise async paths.

**TST-011: EC-11 (TestModel_EC_11_UnknownMessage_NoPanic) is sole test for Update default branch — must preserve, not weaken.** Asserts no panic AND returned model field-equal to input. Don't relax to just "no panic."

**TST-012: save_flow_test.go — TestSave_ValidatorDeepCopyNotSharedWithDoc must pass under -race.** F-98, D-14. Test mutates m.doc.Steps[0].Name then runs deferred validate cmd. Under -race the same memory access pattern is what race detector catches.

---

## Edge Cases — 15 findings

**EC-PR2-001 [P0]: Ctrl+Q during tea.ExecProcess kills builder without RestoreTerminal.** D-34, D-7. No editorInProgress flag — direct Ctrl+Q→quit-confirm→Yes path is not gated. Plan: add editorInProgress flag; handleGlobalKey Ctrl+Q checks it; pendingQuit pattern carries quit across editorExitMsg.

**EC-PR2-002 [P0]: $VISUAL='/Applications/Sublime Text/subl' single-quoted path with space — unclear which characters trigger metacharacter rejection BEFORE shlex.** D33 narrowed to {backtick, ;, |, newline}. Single-quote and double-quote must NOT be in rejection set. Test TestResolveEditor_SingleQuotedAbsPath_NotRejectedAsMetachar.

**EC-PR2-003 [P0]: pendingAction auto-resume for File>New / File>Open after Save with fatals leaves state machine in wrong phase.** D72, save flow step 5. When fatals surface, plan must clear pendingAction at fatals branch — only pendingQuit (from in-flight save Ctrl+Q) carries.

**EC-PR2-004 [P0]: DefaultModel and top-level env/containerEnv silently dropped on parse and save — corrupts existing configs.** GAP-038/039/040, D46. rawConfig/outConfig missing fields; user with defaultModel + top-level env loses fields silently on save. Validator's Cat 10 fires at startup but builder erases env on save; next run has env removed. P0 data corruption. PR-2 must extend rawConfig/outConfig/WorkflowDoc with DefaultModel, Env, ContainerEnv, UnknownFields.

**EC-PR2-005 [P0]: Companion map key convention — workflowio.Load stores `prompts/foo.md`; F-121 says `step-1.md` is in-memory cache key.** GAP-036, T3. Validator misses cache, re-reads from disk. T3's "validator sees exactly the state the save will write" violated. Editor edit→save race window has validator running against pre-edit disk state. Plan must commit single key convention enforced at Load and snapshotCompanions.

**EC-PR2-006 [P1]: Rapid triple Ctrl+S during validating→saving→idle transition has race window.** D-13. Between step 5 (validateInProgress=false) and step 6 (saveInProgress=true) is a single Update invocation. Plan: set both flags in same synchronous Update on validateCompleteMsg before returning tea.Cmd.

**EC-PR2-007 [P1]: Terminal resize during open dropdown leaves dropdown at stale coordinates.** D-12. tea.WindowSizeMsg handler must recompute dropdown render positions unconditionally.

**EC-PR2-008 [P1]: `?` help modal opened while DialogFindingsPanel active suppresses `?` in ALL other dialog states including quit-confirm.** D-8 R2-C1 vs spec §8 "unconditionally reachable from edit view". Plan conflict: accept R2-C1 (require shortcut footer to enumerate full options for every dialog) or escalate.

**EC-PR2-009 [P1]: Reorder mode (`r`) with single step in phase — Up/Down has nowhere to go but boundary-stop is multi-step-only.** D34, GAP-012. Implementation must enforce phase boundary AND provide visible feedback (flash, or "already at top/bottom of phase" message).

**EC-PR2-010 [P1]: IsDirty uses reflect.DeepEqual on WorkflowDoc which includes UnknownFields map[string]json.RawMessage — semantically equal docs compare as dirty due to whitespace.** Spec D63. After UnknownFields added (GAP-041), IsDirty always returns true after load. Defeats no-op save. Plan: IsDirty must skip UnknownFields explicitly.

**EC-PR2-011 [P1]: DetectReadOnly probe at <workflowDir>/.pr9k-write-probe — if dir doesn't yet exist (File>New fresh path), probe ENOENT returned as non-permission error not "writable=true".** detect.go:32-43. ENOENT path not handled; falls to errors.New(...). Plan: DetectReadOnly must handle ENOENT as "directory doesn't exist yet, defer writability check" — return false, nil.

**EC-PR2-012 [P1]: save_failed log event D-27 exclusion contract doesn't cover config.json path itself.** D-27, R7, F-113. SaveResult.Err contains wrapped error with full temp file path. Plan: fmtSaveFailed logs only reason=<enum>, not result.Err.Error(). Test feeds save failure with path-containing error; assert no path in log.

**EC-PR2-013 [P2]: Empty config.json produces JSON parse error but recovery view doesn't say "file is empty" — needs special-case before ParseConfig.** D43. workflowio.Load:98-106 calls ParseConfig without pre-checking. Plan: Load must check len(raw)==0, BOM, utf8.Valid before calling ParseConfig and emit human-readable prefix.

**EC-PR2-014 [P2]: Esc during reorder mode must restore original position — plan doesn't commit to whether intermediate moves are committed or deferred.** D34. doMoveStepDown mutates in place (existing). Plan: snapshot at reorder-mode entry; Esc restores from snapshot; Enter commits and sets dirty.

**EC-PR2-015 [P2]: DetectReadOnly probe `.pr9k-write-probe` static name causes EEXIST collision after SIGKILL.** detect.go:33. EEXIST not handled. Plan: probe name should include PID and timestamp matching atomicwrite pattern; or handle EEXIST by removing stale probe and retrying.

---

## Junior Developer — 11 questions

**JrQ-PR2-001: Is cherry-picking the draft actually cheaper than using it as a reference?** 41 of 51 gaps live inside workflowedit. What fraction of 988 lines can land without rewrite? Walk model.go and mark functions: as-is / modify / replace. If "modify or replace" > half, rewrite-with-backup-as-reference may be lower-risk.

**JrQ-PR2-002: What exactly is "done" for PR-2 — every numbered gap, or just the critical path?** 51 gaps. GAP-052 acknowledged separately. Several "low-confidence" or "Implicit" — possibly out of scope. Plan must classify each as PR-2-required / deferred-to-vNext / accepted-as-is.

**JrQ-PR2-003: GAP-036 companion key convention — is this actually a gap?** Validator's safePromptPath/statCompanionOrDisk/readCompanionOrDisk on main use `filepath.Join("prompts", step.PromptFile)` — same as Load. May be false positive in gap analysis. Re-check.

**JrQ-PR2-004: What does un-hiding the cobra command actually require in test changes?** Rename-guard on main asserts strings appear; doesn't assert Hidden:false or tea.NewProgram presence. Extend rename guard with assertNotContains "Hidden:\ttrue" and assertContains "tea.NewProgram".

**JrQ-PR2-005: Do cherry-picked files compile against PR-1's main?** Backup workflowedit/model.go imports workflowio.SaveFS — exists on main. workflowmodel.WorkflowDoc identical between backup and main. But GAP-038-041 require adding fields. PR-2 ordering: workflowmodel changes must land before/with cherry-pick.

**JrQ-PR2-006: Does backup's workflowmodel have UnknownFields, and which layer owns the addition?** Plan mandates WorkflowDoc.UnknownFields. Neither backup nor main has it. Scope question: PR-2 modifies workflowmodel? Yes — completing the data model PR-1 left as stub. Should be called out explicitly in commit ordering.

**JrQ-PR2-007: The two how-to docs were trimmed in PR-1 — what was trimmed, and does backup version assume live TUI?** Backup pre-dates R2 menu-bar redesign (which replaced landing-page selection with persistent File menu). If backup how-tos describe superseded landing-page flow, they need rewrite not cherry-pick. Diff backup against R2-redesigned spec sections.

**JrQ-PR2-008: Is this 0.7.3 or 0.8.0?** PR-1 shipped 0.7.2 with hidden stub. PR-2 un-hides — patch under 0.y.z rules. D-18 says version bump is first commit — PR-2 needs own version bump (0.7.3) as first commit per spec versioning + standard.

**JrQ-PR2-009: Cherry-pick strategy when files have compile-order dependencies?** Backup workflowedit has 26 files. Same package = no compile-order issue. But commit history matters: one large commit has unreviewable diff; many small commits have intermediate states that may not compile (`make ci` fails on intermediates). Plan needs explicit commit graph: skeleton-restore commit + gap-closing commit series.

**JrQ-PR2-010: What was "explicitly out of scope" in original plan that gap analysis surfaced as missing — does PR-2 inherit deferrals or close them?** Plan author should triage each gap: PR-2-required-primary / PR-2-required-spec / deferred-to-v1.1-with-issue / out-of-scope-per-spec-boundary. Any deferred should be filed before merge.

**JrQ-PR2-011: Does the bundle builder integration test in backup drive workflowedit.Model, and does plan commit to making it meaningful?** Current main test exercises workflowio.Load + workflowvalidate.Validate only. Plan WU-12 says "smoke-tests bundled default workflow loading in builder" but current implementation tests only inner-ring in isolation. PR-2 must construct workflowedit.New, deliver openFileResultMsg, assert m.loaded==true.

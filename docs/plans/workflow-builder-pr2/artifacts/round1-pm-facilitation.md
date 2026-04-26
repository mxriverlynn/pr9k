I now have all the material I need. Let me produce the facilitation summary.

---

# Facilitation Summary: PR-2 Workflow Builder — Round 1 Planning

## Scope

**What was facilitated:** Round 1 specialist review of PR-2 implementation planning for the `pr9k workflow` subcommand. PR-2 is responsible for delivering the full user-facing TUI (`workflowedit.Model` and its wiring) on top of the inner-ring packages already shipped in PR-1. The draft `workflowedit` code exists only in the backup tag `backup/workflow-builder-mode-full-2026-04-24`.

**Artifacts reviewed:** feature-specification.md, feature-implementation-plan.md, implementation-gaps.md (GAP-001..GAP-052), implementation-decision-log.md (D-1..D-47), feature-technical-notes.md (T1..T3), tui-mode-coverage.md (28 entries), decision-log.md (D1..D73), team-findings.md, review-findings.md.

**Specialist outputs consolidated:** 98 findings across 9 specialists (UX-001..010, SEC-001..011, DOR-001..010, R-PR2-001..010, BEH-001..010, CV-PR2-001..009, TST-001..012, EC-PR2-001..015, JrQ-PR2-001..011).

**Date:** 2026-04-24. Participants: project-manager (facilitation), plus all 9 specialist agents above.

---

## Outcome and Context

**Primary outcome (plain language):** PR-2 ships a running `pr9k workflow` subcommand — the user launches it, a Bubble Tea TUI appears, and they can open, edit, validate, and save a workflow bundle without hand-editing JSON. The subcommand is un-hidden and listed in `pr9k --help`.

**Driving constraint:** PR-1 already shipped the inner-ring infrastructure (workflowio, workflowmodel, workflowvalidate, atomicwrite, ansi). The outer ring — the interactive TUI that makes all of it reachable — is the gap PR-2 must close. Without it, the user-facing surface is a silent no-op.

**Stakeholders:** Workflow authors (operators and maintainers), future contributors who will extend the TUI, and operators whose existing config.json files must survive the round-trip without data loss (GAP-038–041).

**Future-state concern:** `internal/atomicwrite` sets a codebase-wide precedent for durable writes. The first `tea.ExecProcess` call (T2) sets a precedent for interactive subprocess handling. Both must be correct on day one.

**Out-of-scope boundary:** Running workflows from the builder; multi-user locking; syntax highlighting; cross-phase step drag; Windows; config migration. Defined in spec "Out of Scope."

---

## Participation Record

| Domain | Specialist | Status | Summary of Input |
|--------|-----------|--------|-----------------|
| UX, accessibility, affordance | `user-experience-designer` | In discussion | 10 findings. Identified gaps in dialog rendering fidelity (UX-001), focus restoration (UX-002), banner-priority structure (UX-003), choice-list visual shape (UX-004), secret-masking granularity (UX-005), model-field suggestion interaction (UX-006), path-picker isolation (UX-007), phase-section structure and `+ Add` affordance (UX-008), unsaved-changes auto-resume discriminated type (UX-009), and numeric-field paste detection (UX-010). |
| Exploit-path security, auth, PII | `adversarial-security-analyst` | In discussion | 11 findings. Identified: ExecCallback disconnected from Model (SEC-001), resolution/restore error type confusion (SEC-002), `$`-rejection violating D33 (SEC-003), CreateEmptyCompanion uncalled (SEC-004), D-23/D-45 ANSI-stripping inconsistency (SEC-005), signal handler missing program reference (SEC-006), session-logging entirely unwired (SEC-007), metachar rejection set incomplete (SEC-008), editorFirstToken re-parsing (SEC-009), absolute-command containment bypass (SEC-010), Hidden:true silent no-op surface (SEC-011). |
| Production readiness, deployment, observability | `devops-engineer` | In discussion | 10 findings. Identified: stale version (DOR-001), un-hide not an independently reversible commit (DOR-002), session-event logging unwired (DOR-003), log-dir fallback ambiguity (DOR-004), --workflow-dir silently ignored (DOR-005), rename-guard trivially satisfied by stub (DOR-006), R7 secret-leak test absent (DOR-007), workflowedit package entirely absent from main (DOR-008), no CI gate for un-hide (DOR-009), confirmed no cloud/SLO risk (DOR-010). |
| Software architecture, module boundaries | `software-architect` | In discussion (as `R-PR2-*`) | 10 findings. Confirmed: file decomposition matches D-1 (R-PR2-001), EditorRunner shape is improved over D-6 pseudocode (R-PR2-002), SaveFS interface on main is correct (R-PR2-003), Update routing matches D-9 (R-PR2-004), no statusline coupling (R-PR2-005), one new direct dependency google/shlex (R-PR2-006), public surface minimized (R-PR2-007), DialogKind set diverges from D-8 (15 vs 14 constants, R-PR2-008), editor.Run never called (R-PR2-009), updateDialog correctly decomposed (R-PR2-010). |
| Runtime behavior, data flow, error propagation | `behavioral-analyst` | In discussion | 10 findings. Identified: pendingQuit re-trigger behavior unspecified (BEH-001), ExecCallback message types have no consumer in Model (BEH-002), editor invocation has no call site at all (BEH-003), load pipeline discards IsSymlink/SymlinkTarget (BEH-004), ackSet never written at acknowledgment time (BEH-005), handleOpenFileResult does not reset session-scoped state (BEH-006), DialogFileConflict/DialogFirstSaveConfirm handlers absent — Esc-only infinite loop (BEH-007), IsDirty never called despite diskDoc existing (BEH-008), DialogUnsavedChanges has no pendingAction field (BEH-009), Model has no logger field (BEH-010). |
| Concurrency, race conditions, deadlock | `concurrency-analyst` | In discussion | 9 findings. Identified: signal handler program-reference race (CV-PR2-001), ctx-cancellation cannot reach tea.Cmd goroutines (CV-PR2-002), saveInProgress invariant undocumented (CV-PR2-003), saveSnapshot ordering ambiguity (CV-PR2-004), validateCompleteMsg swallowed when dialog active (CV-PR2-005), pendingQuit re-trigger unreachable through DialogSaveInProgress routing (CV-PR2-006), stale pathCompletionMsg overwrites live input (CV-PR2-007), post-program.Run goroutine drain unspecified (CV-PR2-008), signal handler timer leaks across test invocations (CV-PR2-009). |
| Test planning | `test-engineer` | In discussion | 12 findings. Identified: T2 matrix gaps (TST-001, TST-002), 28-mode coverage entirely absent in current branch (TST-003), fakeFS lacks per-method counters (TST-004), DI-1/DI-2 placeholder comments not activated (TST-005), companion key convention T6 untested (TST-006), save flow mode 21 missing (TST-007), DialogCopyBrokenRef render test missing (TST-008), bundle integration test does not drive Model (TST-009), fakeFS/fakeEditorRunner lack mutex on shared fields (TST-010), EC-11 assertion requirements (TST-011), TestSave_ValidatorDeepCopyNotSharedWithDoc must pass under -race (TST-012). |
| Edge-case discovery | `edge-case-explorer` | In discussion | 15 findings at P0/P1/P2 priority. P0s: Ctrl+Q during ExecProcess kills builder without RestoreTerminal (EC-PR2-001), single-quoted path space with metachar rejection (EC-PR2-002), pendingAction auto-resume leaving wrong phase on fatals (EC-PR2-003), DefaultModel/top-level env/containerEnv data corruption (EC-PR2-004), companion map key convention T3 violation (EC-PR2-005). P1s (EC-PR2-006..EC-PR2-012). P2s (EC-PR2-013..EC-PR2-015). |
| Generalist stress-test | `junior-developer` | In discussion | 11 questions. Core concerns: cherry-pick vs. rewrite cost analysis (JrQ-PR2-001), gap triage (JrQ-PR2-002), GAP-036 dispute (JrQ-PR2-003), rename-guard gaps (JrQ-PR2-004), compile-against-main ordering (JrQ-PR2-005), UnknownFields ownership (JrQ-PR2-006), how-to doc pre-dates menu-bar redesign (JrQ-PR2-007), version bump question (JrQ-PR2-008), cherry-pick commit ordering (JrQ-PR2-009), gap triage/deferrals (JrQ-PR2-010), bundle integration test scope (JrQ-PR2-011). |

---

## Claim Ledger

Each finding is tagged Evidenced (cites file/line/GAP/D-N), Anecdotal (stated without citation), or Disputed (specialists disagree).

### UX Findings

| Claim | State | Citation | Specialist |
|-------|-------|----------|-----------|
| UX-001: Every DialogKind is a one-line placeholder; Quit-confirm Enter→Yes is opposite of D73 | Evidenced | GAP-045/046, D48, D73; implementation-plan §dialog-conventions | `user-experience-designer` |
| UX-002: focusTarget type never specified; focus restoration unverifiable | Evidenced | D55, D-8 `prevFocus focusTarget` in implementation-decision-log.md | `user-experience-designer` |
| UX-003: D49 banner-priority has no committed rendering contract (single string slot risk) | Evidenced | GAP-005, spec §Primary Flow §5, D49 | `user-experience-designer` |
| UX-004: Choice-list visual shape unspecified (inline vs. floating vs. viewport) | Evidenced | GAP-015; spec D12, D45 | `user-experience-designer` |
| UX-005: Secret masking applies to all containerEnv values regardless of key pattern | Evidenced | GAP-017, D20, D47; detail.go:53-62 | `user-experience-designer` |
| UX-006: Model-field suggestion list has no committed interaction contract | Evidenced | GAP-018, D-42, spec D12/D58 | `user-experience-designer` |
| UX-007: Path picker not isolated by PickerKind (New vs. Open context) | Evidenced | GAP-019, D71, D-25 | `user-experience-designer` |
| UX-008: Phase-section structure and `+ Add` affordance entirely absent from outline | Evidenced | GAP-010, GAP-013, D28, D29, D46, D51 | `user-experience-designer` |
| UX-009: pendingQuit bool insufficient for unsaved-changes auto-resume across New/Open/Quit | Evidenced | GAP-020, spec D72 | `user-experience-designer` |
| UX-010: Numeric-field paste detection mechanism unspecified; three behaviors non-separable | Evidenced | GAP-016, D62; spec §Primary Flow §7 numeric-field bullet | `user-experience-designer` |

### Security Findings

| Claim | State | Citation | Specialist |
|-------|-------|----------|-----------|
| SEC-001: ExecCallback three-way switch disconnected from Model.Update | Evidenced | D-7, GAP-031; editor.go:36-41, model.go UpdateEditView | `adversarial-security-analyst` |
| SEC-002: Resolution errors funneled through editorRestoreFailedMsg | Evidenced | editor.go:36-41 (backup) | `adversarial-security-analyst` |
| SEC-003: rejectShellMeta rejects `$` but D33 explicitly forbids that | Evidenced | editor.go:85-93 (backup); D33 in decision-log.md | `adversarial-security-analyst` |
| SEC-004: CreateEmptyCompanion has zero callers in TUI; D-21 entirely unimplemented | Evidenced | GAP-029, GAP-021, D-21 | `adversarial-security-analyst` |
| SEC-005: D-23 says reuse statusline.Sanitize but D-45 introduces internal/ansi.StripAll — plan internally inconsistent | Evidenced | F-94, D-23, D-45; implementation-decision-log.md D-45 | `adversarial-security-analyst` |
| SEC-006: Signal handler `program.Send(quitMsg{})` is only in a comment | Evidenced | GAP-003, D-34; workflow.go:64-67 | `adversarial-security-analyst` |
| SEC-007: Session-event logging helpers exist; Model has no logger field | Evidenced | GAP-033, D-27; session_log.go format helpers | `adversarial-security-analyst` |
| SEC-008: rejectShellMeta omits `&`, `<`, `>` that D33 includes | Evidenced | editor.go:85-93 (backup); D33 | `adversarial-security-analyst` |
| SEC-009: editorFirstToken re-parses raw env rather than using already-resolved tokens[0] | Evidenced | session_log.go:62-79, D-27 | `adversarial-security-analyst` |
| SEC-010: validateCommandPath applies EvalSymlinks only for relative paths | Evidenced | GAP-042; plan §OI-1 containment precision | `adversarial-security-analyst` |
| SEC-011: Hidden:true with non-functional RunE creates a silent no-op surface | Evidenced | GAP-002; spec §Versioning | `adversarial-security-analyst` |

### DevOps Findings

| Claim | State | Citation | Specialist |
|-------|-------|----------|-----------|
| DOR-001: Version bump is stale at 0.7.2 | Evidenced | versioning.md, D37, D-18; version/version.go:7 | `devops-engineer` |
| DOR-002: Un-hide is not independently reversible | Evidenced | Plan rollback story = revert full PR merge | `devops-engineer` |
| DOR-003: Session-event logging trigger points absent (9 D27 points unwired) | Evidenced | D-27, GAP-033, GAP-034 | `devops-engineer` |
| DOR-004: resolveBuilderLogBaseDir param ambiguity — workflowDir vs. projectDir | Evidenced | D-44, workflow.go:83-94 | `devops-engineer` |
| DOR-005: --workflow-dir silently ignored after stub un-hides | Evidenced | workflow.go:32, GAP-001 | `devops-engineer` |
| DOR-006: Rename-guard tests trivially satisfied by stub | Evidenced | GAP-051, F-116 | `devops-engineer` |
| DOR-007: R7 secret-leak mitigation untested | Evidenced | D-27, RAID R7 | `devops-engineer` |
| DOR-008: workflowedit package does not exist on main — full cold-start delivery | Evidenced | OI-4; backup tag contents documented | `devops-engineer` |
| DOR-009: No automated CI gate that `pr9k --help` lists `workflow` | Evidenced | GAP-002 | `devops-engineer` |
| DOR-010: No cloud/SLO/cost concerns | Evidenced | Plan §Cost and scale correctly states zero | `devops-engineer` |

### Architecture Findings

| Claim | State | Citation | Specialist |
|-------|-------|----------|-----------|
| R-PR2-001: File decomposition matches D-1 | Evidenced | backup tag; D-1 | `software-architect` |
| R-PR2-002: EditorRunner interface shape improved beyond D-6 pseudocode | Evidenced | backup workflowedit/editor.go; D-6 | `software-architect` |
| R-PR2-003: SaveFS interface in PR-1's workflowio — no retroactive refactor needed | Evidenced | workflowio/save.go:19-25 on main | `software-architect` |
| R-PR2-004: Update routing order matches D-9 exactly | Evidenced | backup model.go:102-115; D-9 | `software-architect` |
| R-PR2-005: workflowedit does not import internal/statusline (F-99 honored) | Evidenced | backup import graph | `software-architect` |
| R-PR2-006: One new direct dependency — github.com/google/shlex | Evidenced | D-22; backup cmd/pr9k/editor.go | `software-architect` |
| R-PR2-007: Public surface of workflowedit correctly minimized | Evidenced | backup export list | `software-architect` |
| R-PR2-008: DialogKind set is 15 constants vs. D-8's 14 | Evidenced | backup dialogs.go; implementation-decision-log.md D-8 | `software-architect` |
| R-PR2-009: m.editor.Run is never called in the backup | Evidenced | GAP-029; backup model.go call-site scan | `software-architect` |
| R-PR2-010: updateDialog dispatch correctly decomposed; five fall-through kinds need handlers | Evidenced | backup model.go updateDialog; GAP-022, GAP-023 | `software-architect` |

### Behavioral Findings

| Claim | State | Citation | Specialist |
|-------|-------|----------|-----------|
| BEH-001: After save with pendingQuit, backup returns tea.Quit directly — no QuitConfirm | Evidenced | model.go:771-789 (backup); save flow step 9 in implementation-plan | `behavioral-analyst` |
| BEH-002: ExecCallback message types have no consumer in Model.Update | Evidenced | updateEditView message type list; cmd/pr9k editor message type declarations | `behavioral-analyst` |
| BEH-003: openEditorMsg and launchEditorMsg not declared; m.editor.Run never called | Evidenced | GAP-029; backup model.go | `behavioral-analyst` |
| BEH-004: makeLoadCmd discards IsSymlink/SymlinkTarget from workflowio.Load result | Evidenced | GAP-035; backup makeLoadCmd / openFileResultMsg struct | `behavioral-analyst` |
| BEH-005: ackSet never written during acknowledgment — per-session suppression never activates | Evidenced | GAP-027, GAP-028; backup model.go:411-426 | `behavioral-analyst` |
| BEH-006: handleOpenFileResult does not reset reorderMode, ackSet, helpOpen | Evidenced | spec D57; backup model.go:793-812 | `behavioral-analyst` |
| BEH-007: DialogFileConflict and DialogFirstSaveConfirm Esc-only — infinite loop on overwrite/reload press | Evidenced | GAP-022, GAP-023; backup updateDialog | `behavioral-analyst` |
| BEH-008: IsDirty never called; dirty tracked by scattered m.dirty=true mutations | Evidenced | backup model.go 8 mutation sites; workflowmodel.IsDirty existence | `behavioral-analyst` |
| BEH-009: DialogUnsavedChanges Discard unconditionally calls tea.Quit — kills builder on File>New | Evidenced | GAP-020, D72; backup updateDialogUnsavedChanges | `behavioral-analyst` |
| BEH-010: Model has no logger field — D27 event catalog unreachable | Evidenced | GAP-033; backup New() signature | `behavioral-analyst` |

### Concurrency Findings

| Claim | State | Citation | Specialist |
|-------|-------|----------|-----------|
| CV-PR2-001: Signal handler does not hold a program reference — race hazard | Evidenced | workflow.go:62-73; D-34 signal-handler semantics | `concurrency-analyst` |
| CV-PR2-002: ctx-cancellation cannot reach tea.Cmd goroutines | Evidenced | D-33; backup scanMatches/save/validate closures | `concurrency-analyst` |
| CV-PR2-003: saveInProgress/validateInProgress invariant not pinned | Anecdotal | Observation from backup code; no concrete failure scenario cited with line numbers | `concurrency-analyst` |
| CV-PR2-004: saveSnapshot ordering ambiguity creates logical TOCTOU window | Evidenced | model.go:771-790 (backup); D-13 step 8 sequence ambiguity | `concurrency-analyst` |
| CV-PR2-005: validateCompleteMsg swallowed when dialog active during async round-trip | Evidenced | D-9 routing tier; backup updateEditView message handling | `concurrency-analyst` |
| CV-PR2-006: pendingQuit re-trigger unreachable through DialogSaveInProgress routing tier | Evidenced | model.go:471-478, 765-790 (backup); D-9 routing order | `concurrency-analyst` |
| CV-PR2-007: Stale pathCompletionMsg overwrites live input | Evidenced | pathpicker.go:35-43, model.go:288-354 (backup) | `concurrency-analyst` |
| CV-PR2-008: Post-program.Run goroutine drain unspecified | Evidenced | docs/coding-standards/concurrency.md §"Wait for background goroutines" | `concurrency-analyst` |
| CV-PR2-009: Signal handler 10s timer leaks across test invocations | Evidenced | workflow.go:62-73; D-34 timer behavior | `concurrency-analyst` |

### Test Findings

| Claim | State | Citation | Specialist |
|-------|-------|----------|-----------|
| TST-001: T2 matrix missing VISUAL with unquoted space | Evidenced | Plan F-118 T2 case enumeration; backup test list | `test-engineer` |
| TST-002: T2 matrix missing VISUAL="" with EDITOR="" case | Evidenced | backup T2 test list; spec D16 guidance behavior | `test-engineer` |
| TST-003: 28-mode coverage entirely absent in current branch | Evidenced | tui-mode-coverage.md 28 entries; model_test.go absent from main | `test-engineer` |
| TST-004: fakeFS lacks per-method call counters — violates testing.md | Evidenced | docs/coding-standards/testing.md §"Each fake method must have its own call counter"; backup helpers_test.go fakeFS | `test-engineer` |
| TST-005: DI-1/DI-2 are placeholder comments — must be re-activated | Evidenced | doc_integrity_test.go lines 1001-1004 | `test-engineer` |
| TST-006: Companion key convention untested end-to-end (T3 contract gap) | Evidenced | F-121; GAP-036; T3 spec | `test-engineer` |
| TST-007: Mode 21 (pendingQuit + saveCompleteMsg re-enter-quit-flow) missing from save_flow_test.go | Evidenced | tui-mode-coverage.md mode 21; backup TestSaveComplete_WithPendingQuit | `test-engineer` |
| TST-008: DialogCopyBrokenRef render test missing from dialogs_render_test.go | Evidenced | D-61; DialogCopyBrokenRef constant in backup | `test-engineer` |
| TST-009: bundle_builder_integration_test.go does not drive workflowedit.Model | Evidenced | GAP-044; plan DoD line 350 | `test-engineer` |
| TST-010: fakeFS and fakeEditorRunner lack sync.Mutex | Evidenced | docs/coding-standards/testing.md §"Test doubles with shared state need mutexes" | `test-engineer` |
| TST-011: EC-11 must assert field-equality not just no-panic | Anecdotal | "Don't relax" stated without citing a prior test regression that proved this matters | `test-engineer` |
| TST-012: TestSave_ValidatorDeepCopyNotSharedWithDoc must pass under -race | Evidenced | docs/coding-standards/testing.md §race; F-98, D-14 | `test-engineer` |

### Edge Case Findings

| Claim | State | Citation | Specialist |
|-------|-------|----------|-----------|
| EC-PR2-001 [P0]: Ctrl+Q during ExecProcess kills builder without RestoreTerminal | Evidenced | D-34, D-7; backup — no editorInProgress flag | `edge-case-explorer` |
| EC-PR2-002 [P0]: Single-quoted path with space triggers metachar rejection before shlex | Evidenced | D33 rejection set narrowed to {backtick, ;, |, newline} | `edge-case-explorer` |
| EC-PR2-003 [P0]: pendingAction auto-resume leaves state machine in wrong phase when save has fatals | Evidenced | D72 save flow step 5; spec §Unsaved-changes interception | `edge-case-explorer` |
| EC-PR2-004 [P0]: DefaultModel/top-level env/containerEnv silently dropped — data corruption | Evidenced | GAP-038/039/040, D46; workflowmodel/model.go:64-68; validator Category 10 | `edge-case-explorer` |
| EC-PR2-005 [P0]: Companion map key convention — T3 "exactly what will be saved" violated | Disputed — see JrQ-PR2-003 below | GAP-036, T3; workflowio/load.go:119,149 (full-path key); validator readCompanionOrDisk (bare key) | `edge-case-explorer` |
| EC-PR2-006 [P1]: Rapid triple Ctrl+S race window between validateInProgress clear and saveInProgress set | Evidenced | D-13 steps 5-6; implementation-plan §Save flow | `edge-case-explorer` |
| EC-PR2-007 [P1]: Terminal resize during open dropdown leaves dropdown at stale coordinates | Evidenced | D-12; spec §Terminal resize edge case | `edge-case-explorer` |
| EC-PR2-008 [P1]: `?` help modal in non-findings dialog states — D-8 R2-C1 vs. spec §8 conflict | Evidenced | D-8 R2-C1; spec Primary Flow §8 "unconditionally reachable from edit view" | `edge-case-explorer` |
| EC-PR2-009 [P1]: Reorder mode with single step — boundary feedback absent | Evidenced | D34; GAP-012 | `edge-case-explorer` |
| EC-PR2-010 [P1]: IsDirty uses reflect.DeepEqual; UnknownFields map causes false-dirty after load | Evidenced | spec D63; GAP-041; UnknownFields type map[string]json.RawMessage | `edge-case-explorer` |
| EC-PR2-011 [P1]: DetectReadOnly ENOENT not handled — non-permission error on fresh path | Evidenced | detect.go:32-43; GAP-006 | `edge-case-explorer` |
| EC-PR2-012 [P1]: save_failed log event inclusion contract does not cover config.json path itself | Evidenced | D-27, R7, F-113; fmtSaveFailed | `edge-case-explorer` |
| EC-PR2-013 [P2]: Empty config.json recovery view needs special-case before ParseConfig | Evidenced | D43; workflowio/load.go:98-106 | `edge-case-explorer` |
| EC-PR2-014 [P2]: Esc during reorder must restore original position | Evidenced | D34; tui-mode-coverage.md mode 13 | `edge-case-explorer` |
| EC-PR2-015 [P2]: DetectReadOnly probe static name — EEXIST collision after SIGKILL | Evidenced | detect.go:33; D-16 PID-liveness pattern | `edge-case-explorer` |

### Junior Developer Questions

| Claim | State | Citation | Specialist |
|-------|-------|----------|-----------|
| JrQ-PR2-001: Cherry-pick vs. reference-rewrite cost unknown | Anecdotal | No line-count or function-level analysis provided | `junior-developer` |
| JrQ-PR2-002: "Done" definition — 51 gaps, which are PR-2-required vs. deferred | Anecdotal | No triage table exists | `junior-developer` |
| JrQ-PR2-003: GAP-036 may be a false positive — validator uses same key convention as Load | Disputed | Counters EC-PR2-005 claim; JrQ-PR2-003 cites validator.go `filepath.Join("prompts", step.PromptFile)` matching Load; requires code verification | `junior-developer` |
| JrQ-PR2-004: Rename-guard extension requirements | Evidenced | GAP-051, F-116 | `junior-developer` |
| JrQ-PR2-005: Cherry-picked files may not compile against PR-1's main without workflowmodel additions | Evidenced | GAP-038/039/041; workflowmodel/model.go:64-68 | `junior-developer` |
| JrQ-PR2-006: UnknownFields ownership — neither backup nor main has it | Evidenced | GAP-041; plan §Data Model | `junior-developer` |
| JrQ-PR2-007: How-to docs in backup predate R2 menu-bar redesign | Anecdotal | No specific diff cited; claim is plausible given timeline of R2 redesign | `junior-developer` |
| JrQ-PR2-008: Version bump is 0.7.3 (patch under 0.y.z rules) | Evidenced | versioning.md; D-18; spec §Versioning | `junior-developer` |
| JrQ-PR2-009: Cherry-pick commit graph needs explicit ordering to keep `make ci` green on every commit | Evidenced | D-1 package dependencies; same-package compilation | `junior-developer` |
| JrQ-PR2-010: Gap triage — each gap must be classified as PR-2-required / deferred / accepted | Anecdotal | No enumeration provided | `junior-developer` |
| JrQ-PR2-011: bundle_builder_integration_test currently only tests inner-ring, not Model | Evidenced | GAP-044; backup bundle_builder_integration_test.go analysis | `junior-developer` |

---

## RAID Log

### Risks

| ID | Risk | Likelihood | Severity | Blast Radius | Reversibility | Owner | Mitigation |
|----|------|------------|----------|--------------|---------------|-------|------------|
| R-PR2-A | Cherry-pick approach introduces merge conflicts or compile failures if workflowmodel additions (GAP-038/041) are not sequenced before workflowedit cherry-pick | High | High (CI breaks on intermediates) | PR-2 entire timeline | Recoverable by reordering commits | `software-architect` | Explicit commit-graph planning before cherry-pick; validate `make ci` on each commit |
| R-PR2-B | Backup how-to docs describe superseded landing-page flow (JrQ-PR2-007); shipping them as-is produces documentation that describes a non-existent UX | Medium | Medium (user confusion) | Two how-to docs | Reversible (update docs) | `information-architect` / `user-experience-designer` | Diff backup how-tos against R2-redesigned spec before cherry-pick; assume rewrite needed |
| R-PR2-C | GAP-036 disputed claim: if JrQ-PR2-003 is correct (validator uses same key convention as Load), EC-PR2-005 is a false positive and T3 is satisfied | Medium | Low (no fix needed if dispute resolves in favor of JrQ-PR2-003) | T3 compliance claim | N/A — benefit if false positive confirmed | `behavioral-analyst` | Code verification against validator.go readCompanionOrDisk |
| R-PR2-D | ExecCallback message types declared in cmd/pr9k (package-local) cannot be type-switched in workflowedit.Model — requires message ownership resolution before M.editor.Run can be wired | High (this is a real blocker for external-editor handoff) | High | External editor feature | Requires decision now | `software-architect` | R-PR2-002 notes DIP is preserved; message ownership boundary must be explicitly committed (BEH-002) |
| R-PR2-E | dialogState constant divergence (15 vs. D-8's 14) means D-8 documentation is stale; unhandled stub dialogs (DialogError, DialogCrashTempNotice, DialogFirstSaveConfirm, DialogExternalEditorOpening, DialogFileConflict) create Esc-only traps | High | High (user cannot resolve conflicts; Esc-only dialogs are UX dead-ends) | Five dialog flows | Requires explicit handlers before un-hide | `software-architect` + `user-experience-designer` | R-PR2-008 flags this; BEH-007 confirms; must decide which constants to complete vs. defer |
| R-PR2-F | Backup `workflowedit` is 4,140 LOC across 27 files but has ~50 gaps — unknown ratio of "cherry-pick as-is" to "requires significant rewrite" (JrQ-PR2-001) | Medium | Medium (timeline risk) | Entire PR-2 scope | Recoverable via rewrite-with-reference | `software-architect` | Function-level triage before implementation begins |
| R-PR2-G | Missing pendingAction discriminated type (UX-009, BEH-009) means File>New and File>Open after unsaved changes quit the builder instead of resuming the pending action | High | High (data loss equivalent — in-memory state destroyed when user chose New, not Quit) | Every unsaved-changes→New/Open flow | Requires pendingAction enum before un-hide | `user-experience-designer` / `behavioral-analyst` | Committed before any cherry-pick of updateDialogUnsavedChanges |

Existing plan RAID R1..R9 carry forward from the implementation plan RAID log (all still applicable).

### Assumptions

| ID | Assumption | What Changes If Wrong | Verifier | Status |
|----|------------|-----------------------|----------|--------|
| A-PR2-1 | Backup tag `backup/workflow-builder-mode-full-2026-04-24` is intact and accessible | PR-2 loses its reference implementation | Any team member | Unverified — no specialist confirmed checkout |
| A-PR2-2 | workflowedit package in backup compiles against PR-1's main after workflowmodel additions (GAP-038/041) | Compile failures block cherry-pick | `software-architect` | Unverified — JrQ-PR2-005 raises this |
| A-PR2-3 | GAP-036 is a real gap (companion key convention mismatch) — validator uses bare filename key, Load uses `prompts/<file>` key | If JrQ-PR2-003 is correct, T3 is already satisfied and the fix is not needed | `behavioral-analyst` via code inspection | Disputed — requires verification |
| A-PR2-4 | How-to docs in backup predate the R2 menu-bar redesign and require rewrite, not cherry-pick | If backup docs already reflect R2 redesign, cherry-pick is sufficient | `user-experience-designer` | Unverified — JrQ-PR2-007 raises this |
| A-PR2-5 | ExecCallback message types must move to workflowedit (or a shared package) to resolve BEH-002 | Message ownership boundary decision is needed; alternative: callback receives typed workflowedit messages from cmd/pr9k | `software-architect` | Open — requires decision |
| A-PR2-6 | D-8's 14 constants are authoritative and backup's 15th (one of: DialogSaveInProgress, DialogRecovery, DialogAcknowledgeFindings) is an undocumented extension that plan should acknowledge | If the 15th constant is valid and plan simply failed to number it, D-8 needs updating, not the implementation | `software-architect` | Plan-level — must update D-8 before synthesis |

### Issues

| ID | Issue | Owner | Next Step |
|----|-------|-------|-----------|
| I-PR2-1 | No gap-triage table exists: 51 gaps not classified as PR-2-required vs. deferred vs. accepted | Plan author | Produce gap-triage table before synthesis; JrQ-PR2-002/010 |
| I-PR2-2 | workflowedit entirely absent from main — PR-2 is a full cold-start delivery, not a gap-closure patch | `devops-engineer` | Structure cherry-pick + commit graph explicitly (DOR-008, JrQ-PR2-009) |
| I-PR2-3 | EC-PR2-005 disputed by JrQ-PR2-003 — companion key convention may not be a gap | `behavioral-analyst` | Code inspection against validator.go readCompanionOrDisk and workflowio/load.go:119 |
| I-PR2-4 | ExecCallback message ownership boundary (BEH-002, R-PR2-D) is undecided — blocks external editor wiring | `software-architect` | Decide before cherry-pick of workflowedit/editor.go and cmd/pr9k/editor.go |
| I-PR2-5 | rejectShellMeta rejection set conflicts with D33 in two directions: wrongly rejects `$` (SEC-003), wrongly omits `&`, `<`, `>` (SEC-008) | `adversarial-security-analyst` | Fix rejection set before cherry-pick of editor.go |

### Dependencies

All existing plan dependencies (Dep1..Dep4) carry forward. New PR-2-specific dependencies:

| ID | Dependency | Owner | Status |
|----|------------|-------|--------|
| Dep5 | workflowmodel.WorkflowDoc gains UnknownFields, top-level Env/ContainerEnv, DefaultModel round-trip | `software-architect` | Must land before or with workflowedit cherry-pick |
| Dep6 | Gap-triage table classifying all 51 gaps | Plan author | Blocks synthesis |
| Dep7 | ExecCallback message ownership boundary decision | `software-architect` | Blocks external editor wiring |
| Dep8 | Backup how-to doc diff against R2-redesigned spec | `user-experience-designer` | Determines whether cherry-pick or rewrite |

---

## Scope and Definition-of-Done Check

**What the plan says is done:**
- `pr9k workflow` listed in `--help`, not hidden
- workflowedit.Model implemented and running via tea.NewProgram
- 28 TUI mode-coverage tests pass
- bundle_builder_integration_test drives the Model end-to-end
- doc-integrity tests DI-1..DI-8 pass
- version bump at 0.7.3

**What specialists say is missing from the DoD:**

- No DoD criterion for pendingAction enum (BEH-009 / UX-009) — without it, File>New/Open after unsaved changes silently quits
- No DoD criterion for message ownership boundary (BEH-002 / R-PR2-D) — without it, ExecCallback three-way switch is a compile error
- No DoD criterion for gap-triage classification — the plan cannot commit to "done" without knowing which gaps are PR-2-required
- No DoD criterion for editorInProgress flag (EC-PR2-001) — Ctrl+Q during ExecProcess is a terminal-corruption scenario, not a UX nicety
- No DoD criterion for rejectShellMeta correction (SEC-003 / SEC-008)
- No DoD criterion for "backup how-to docs reviewed against R2 spec"
- No DoD criterion for post-program.Run goroutine drain (CV-PR2-008)
- DOR-002: un-hide should be a named, independently reversible commit — not bundled with TUI wiring

**What the plan says is explicitly out of scope that specialists are not disputing:** Running workflows, importing from URL, diffing, multi-user locking, syntax highlighting, cross-phase step drag, Windows.

**What is ambiguously in between (requires user input):**

- EC-PR2-008: `?` help modal while non-findings dialog is active — spec §8 says "unconditionally reachable from edit view" but D-8 R2-C1 says silently suppressed for non-findings-panel dialogs. This is a spec-vs-implementation-plan contradiction.
- JrQ-PR2-010: Which of the 51 gaps are deferred to vNext and which must land in PR-2.
- EC-PR2-001 vs. "out of scope": whether the editorInProgress flag is a PR-2 requirement or acceptable to defer if ExecProcess is not fully wired in PR-2.

**Smallest viable slice concern:** DOR-002 / JrQ-PR2-009 both flag that the current plan has no articulated commit graph, making it impossible to land partial work without breaking `make ci`. The un-hide commit in particular should be the last named commit, making partial review possible.

---

## Inconsistencies and Standards Conflicts

| Inconsistency | Location of Standard | Conflicting Plan Element | Resolution Path |
|---------------|---------------------|--------------------------|-----------------|
| SEC-003: backup rejectShellMeta rejects `$` | decision-log.md D33: "Reject `$`… rejected because direct exec does not interpret `$`. Legitimate `VISUAL='$HOME/bin/myvim'` must work." | editor.go:85-93 (backup) adds `$` to rejection set | Remove `$` from rejection set; plan must acknowledge |
| SEC-008: backup rejectShellMeta omits `&`, `<`, `>` | decision-log.md D33 rejection set specification | editor.go:85-93 (backup) rejection set incomplete | Add missing characters before cherry-pick |
| SEC-005: D-23 references statusline.Sanitize, D-45 introduces internal/ansi.StripAll | implementation-decision-log.md D-23 and D-45 (same document, contradictory) | Plan internally inconsistent | Update D-23 to reference internal/ansi.StripAll; one plan, one stripper |
| TST-004: fakeFS lacks per-method call counters | docs/coding-standards/testing.md §"Each fake method must have its own call counter" | backup helpers_test.go fakeFS has no counters | Must add counters before restoring tests |
| TST-010: fakeFS/fakeEditorRunner lack sync.Mutex | docs/coding-standards/testing.md §"Test doubles with shared state need mutexes" | backup helpers_test.go fakes | Must add mutex before tests exercise async paths |
| EC-PR2-008: `?` help modal reachability | spec §Primary Flow §8 "unconditionally reachable from edit view" | D-8 R2-C1 says silently suppressed for non-findings-panel dialogs | Requires user input — spec-vs-plan conflict |
| R-PR2-008: DialogKind 15 constants vs D-8's 14 | implementation-decision-log.md D-8 (14 constants enumerated) | backup dialogs.go has 15 | Update D-8 to acknowledge the 15-constant set with rationale |
| CV-PR2-002: D-33 implies ctx-cancellation for all goroutines; backup implements fire-and-forget | implementation-decision-log.md D-33: "every non-stdlib goroutine receives ctx and selects on ctx.Done()" | tea.Cmd closures in backup do not take ctx | Must explicitly decide: Option A (Bubble Tea goroutines exempt; document) or Option B (pass ctx) |
| BEH-001: Implementation-plan save flow step 9 says "re-route to handleGlobalKey(Ctrl+Q)"; backup returns tea.Quit directly | implementation-decision-log.md D-13 step 9 / implementation-plan §Quit interaction | backup model.go:787-788 | Plan must commit: re-enter quit flow (show QuitConfirm) or bypass (implicit confirmation) |

---

## Future-State Scan

| Concern | Domain Owner | Resolving Question |
|---------|-------------|-------------------|
| internal/atomicwrite is now the canonical durable-write pattern for the codebase. A second caller (e.g., logger, status-line persistence) will follow it. Any bug in the pattern becomes a codebase-wide bug. | `devops-engineer` + `software-architect` | Are all T1 test cases (11 enumerated in implementation-plan §Testing) present and passing under `-race`? |
| tea.ExecProcess usage sets the codebase precedent for interactive subprocess handling. The SIGINT branch (D-7) and RestoreTerminal error handling (editorRestoreFailedMsg) become copy-paste templates. | `software-architect` + `concurrency-analyst` | Is the ExecCallback three-way switch tested end-to-end through a fake tea.Program (not just unit-tested in isolation)? |
| workflowedit.Model is the first TUI component that spawns async tea.Cmd goroutines with results that must survive program.Run() return. The drain pattern (CV-PR2-008) is novel for this codebase. | `concurrency-analyst` | Is the post-program.Run drain pattern documented in the concurrency coding standard, or at minimum in a code comment, so future TUI contributors follow it? |
| DefaultModel/top-level env/containerEnv are data-model additions to workflowmodel (EC-PR2-004). As the builder gains a second caller of WorkflowDoc (e.g., a future CLI validator subcommand), the correctness of round-trip serialization matters more. | `structural-analyst` | Are round-trip marshal tests added for every new WorkflowDoc field per docs/coding-standards/testing.md §"Add round-trip tests for phase-structured marshaling"? |
| DetectReadOnly probe leaves a `.pr9k-write-probe` file at a static name — EEXIST collision after SIGKILL (EC-PR2-015). If a future caller also calls DetectReadOnly, collision probability doubles. | `devops-engineer` | Should the probe use PID+timestamp in its name (matching atomicwrite pattern from D-16) before PR-2 ships? |

---

## Spec-Maturity Classification

Each finding is classified as:
- `plan-level` — concerns internal PR-2 decisions (wiring, sequencing, test coverage) that do not affect the spec
- `spec-level` — reveals a gap or ambiguity in the feature spec itself
- `T#-contradiction` — directly contradicts one of T1, T2, or T3 (load-bearing technical notes)

| Finding | Classification | Rationale |
|---------|---------------|-----------|
| UX-001..010 | plan-level | Dialog rendering gaps, focus restoration, and banner structures are all plan-implementation choices, not spec gaps. The spec specifies behaviors; the plan specifies how to achieve them. |
| SEC-001..011 | plan-level | All are wiring gaps or implementation bugs in the backup code. Spec is not touched. |
| DOR-001..009 | plan-level | Operational, versioning, and CI concerns. |
| DOR-010 | plan-level | Confirmation of zero cloud/SLO risk. |
| R-PR2-001..010 | plan-level | Architecture validation and divergence notes. R-PR2-008 (15 vs 14 DialogKind constants) is plan-level — requires D-8 update. |
| BEH-001..010 | plan-level | All wiring and state-machine gaps in backup implementation. |
| CV-PR2-001..009 | plan-level | All concurrency gaps in backup implementation or unspecified plan choices. |
| TST-001..012 | plan-level | Test coverage and tooling gaps. |
| EC-PR2-001..015 | plan-level (13 of 15) | Behavioral edge cases that the plan must handle. |
| **EC-PR2-008** | **spec-level** | Spec §8 says `?` is "unconditionally reachable from edit view"; D-8 R2-C1 says silently suppressed for non-findings-panel dialogs. These are contradictory statements in the spec and implementation-decision-log respectively. Requires user clarification. |
| **BEH-001** | **spec-level** | Save flow step 9 in implementation-plan says "re-route to handleGlobalKey(Ctrl+Q) which now sees saveInProgress==false and proceeds normally — QuitConfirm or unsaved-changes." But spec D73 says quit always confirms. The ambiguity is whether "implicit confirmation" (bypassing QuitConfirm after a successful save in pendingQuit path) is a spec violation or an intentional simplification. Requires user clarification. |
| JrQ-PR2-001..011 | plan-level (10 of 11) | Process and sequencing questions. |
| **JrQ-PR2-003 / EC-PR2-005 dispute** | plan-level (contested) | The companion key convention dispute is about whether GAP-036 is real. This is a code-inspection question, not a spec ambiguity. |

**Spec-maturity gate assessment:**

- T# contradictions: **0 confirmed**. SEC-005 (D-23 vs D-45 inconsistency) is a plan-internal inconsistency, not a contradiction of T1/T2/T3 themselves. T1 (atomic save) is unaffected. T2 (terminal handoff) is confirmed correct by R-PR2-002. T3 (in-memory validation) has a disputed claim (EC-PR2-005 / JrQ-PR2-003) but no confirmed contradiction.
- Spec-level findings: **2 confirmed** (EC-PR2-008, BEH-001), raised by 2 distinct specialists (`edge-case-explorer` and `behavioral-analyst`).

Gate thresholds:
- `≥2 T# contradictions raised by ≥2 distinct specialists` — NOT tripped. Zero T# contradictions.
- `≥5 spec-level findings raised by ≥3 distinct specialists` — NOT tripped. Two spec-level findings by two specialists.

**Gate verdict: Does not trip. The spec is sufficiently mature to proceed.** However, the two spec-level findings (EC-PR2-008 and BEH-001) require user input before synthesis can commit to decisions.

---

## Open Questions

### Group A: Evidence-based resolution (can be settled by reading the codebase or docs)

**OQ-PR2-1: Is GAP-036 a real gap? (EC-PR2-005 vs. JrQ-PR2-003)**
- **Question:** Does the validator's `readCompanionOrDisk` / `statCompanionOrDisk` use `filepath.Join("prompts", step.PromptFile)` as the cache key (matching Load's full-path key), or does it use a bare `step.PromptFile` key (requiring Load to store bare keys for T3 to be satisfied)?
- **Raised by:** `edge-case-explorer` (EC-PR2-005, asserts real gap), `junior-developer` (JrQ-PR2-003, disputes).
- **Evidence considered:** GAP-036 notes workflowio/load.go:119 stores `filepath.Join("prompts", step.PromptFile)` as key. JrQ-PR2-003 cites validator.go `filepath.Join("prompts", step.PromptFile)` as the key used by `statCompanionOrDisk` — which would make the keys match.
- **Recommended answer:** Read `src/internal/validator/validator.go` at the `readCompanionOrDisk` and `statCompanionOrDisk` implementations. If both use `filepath.Join("prompts", step.PromptFile)`, JrQ-PR2-003 is correct, GAP-036 is a false positive, and EC-PR2-005 is closed. If the validator uses bare filename keys, the gap is real.
- **Blocks synthesis:** Yes — T3 compliance claim depends on the answer.

**OQ-PR2-2: What is the exact rejection set in the backup's rejectShellMeta, and does it conflict with D33?**
- **Question:** Two specialists flag contradictions in opposite directions (SEC-003 wrongly adds `$`; SEC-008 wrongly omits `&`, `<`, `>`). Are both correct?
- **Raised by:** `adversarial-security-analyst` (SEC-003, SEC-008).
- **Evidence considered:** editor.go:85-93 (backup). D33 rejection set: backtick, `;`, `|`, `&`, `<`, `>`, `\n`. No `$`.
- **Recommended answer:** Read backup's editor.go:85-93. If both observations are confirmed, the fix is: remove `$`, add `&`, `<`, `>`. One test added per SEC-003's TestResolveEditor_VisualWithDollar_Accepted and SEC-008's full D33 set test.
- **Blocks synthesis:** No — it is a clear fix with evidenced resolution path. But must be in PR-2 commit before cherry-pick of editor.go.

**OQ-PR2-3: Do the backup's how-to docs describe the superseded landing-page flow?**
- **Question:** Do the two how-to docs in the backup (`using-the-workflow-builder.md`, `configuring-external-editor-for-workflow-builder.md`) reference the pre-R2-redesign landing page, or do they already reflect the persistent File menu?
- **Raised by:** `junior-developer` (JrQ-PR2-007).
- **Evidence considered:** R2 menu-bar redesign replaced landing-page selection with persistent File menu (decision-log.md R2 history). Backup pre-dates R2 spec-revision in the PR-1 timeline.
- **Recommended answer:** Diff both how-to docs against the R2 spec primary flow. If they describe a landing page, they require rewrite before cherry-pick.
- **Blocks synthesis:** Yes — if rewrite is needed, it must be in the DoD.

**OQ-PR2-4: What is the companion key in validator.ValidateDoc's companionFiles map — bare filename or `prompts/<file>`?**
- This is the same investigation as OQ-PR2-1, restated from D-3's perspective: D-3 says "map keys are `promptFile` values from the step (e.g., `step-1.md`, not the full `prompts/step-1.md` path)." If D-3 commits to bare keys, then Load must store bare keys, and GAP-036 is real. Read `src/internal/validator/validator.go` D-3 implementation.
- **Blocks synthesis:** Yes (same as OQ-PR2-1).

### Group B: Needs junior-developer reframing

**OQ-PR2-5: What fraction of backup's 988-line model.go and 27 files can be cherry-picked as-is vs. requires rewrite?**
- **Question:** JrQ-PR2-001 asks this but provides no triage. The answer determines whether the implementation approach is "cherry-pick + gap-close" or "rewrite with backup as reference."
- **Raised by:** `junior-developer` (JrQ-PR2-001).
- **Evidence considered:** 51 gaps identified; R-PR2-001 confirms file decomposition is correct; specialists have mapped gaps to specific functions (e.g., BEH-003 = editor.Run caller, BEH-005 = ackSet write).
- **Recommended answer:** A function-level triage of model.go against the gap list is the right pre-implementation step. Gaps that require modifying existing backup functions (BEH-001, BEH-005, BEH-006, BEH-008) are "modify" candidates; gaps that require new code paths (BEH-003, BEH-009) are "add" candidates; gaps that require replacing a function entirely (GAP-010 outline rendering, GAP-014 detail pane editing) are "replace" candidates.
- **Blocks synthesis:** No — the plan can commit to "cherry-pick + gap-close" as the strategy with the triage as a pre-implementation work unit. But the gap-triage table (I-PR2-1) must exist before the DoD is finalized.

**OQ-PR2-6: What is the commit graph for PR-2 such that every intermediate commit passes `make ci`?**
- **Question:** JrQ-PR2-009 raises this; DOR-002 reinforces it. The un-hide commit should be last and independently revertable.
- **Raised by:** `junior-developer` (JrQ-PR2-009), `devops-engineer` (DOR-002).
- **Recommended answer for the plan to commit to:** (1) Version bump commit (first, per D-18). (2) workflowmodel additions (UnknownFields, top-level Env/ContainerEnv, DefaultModel) with tests. (3) workflowedit cherry-pick skeleton (compiles against main, all tests present but some failing — or alternatively, all tests present and passing if gaps are addressed per-file). (4) Gap-closure commit series (one commit per gap cluster, each green). (5) Un-hide + rename-guard extensions (last named commit, WU-13). This is a recommendation; the team may restructure but must specify a graph.
- **Blocks synthesis:** No — but it is a required output of synthesis.

### Group C: Requires user input

**OQ-PR2-7 (SPEC-LEVEL): Does `?` help modal suppress in non-findings-panel dialogs? (EC-PR2-008)**
- **Question:** Spec §8 says the help modal is "unconditionally reachable from the edit view regardless of any other configuration." D-8 R2-C1 says `helpOpen` flips `true` only when `dialog.kind == DialogFindingsPanel` and silently suppresses for all other dialog kinds. These are contradictory.
- **Raised by:** `edge-case-explorer` (EC-PR2-008).
- **Evidence considered:** Spec Primary Flow §8; D-8 R2-C1 in implementation-decision-log.md. Both are authoritative for their respective domains.
- **Options:** (A) Accept D-8 R2-C1: `?` is suppressed during non-findings dialogs; update spec §8 to add "from edit view when no non-findings dialog is active." (B) Accept spec §8: `?` works during all dialogs; every dialog's `updateDialog` handler must forward `?` to `updateHelpModal`. Option A is lower implementation cost. Option B is strictly spec-compliant but adds a handler requirement to every dialog.
- **Recommended answer:** Option A. The UX rationale in D-8 (modal exclusion, Escape-pops-one-layer) is sound. Spec §8's phrasing "edit view regardless of other configuration" was written before D-8 established the overlay model. Update spec §8 to read "from edit view or the findings panel, regardless of other configuration."
- **Blocks synthesis:** Yes — the recommendation handles it, but requires user acknowledgment to update the spec.

**OQ-PR2-8 (SPEC-LEVEL): After successful save with pendingQuit, does the builder show QuitConfirm or exit directly? (BEH-001)**
- **Question:** Implementation-plan save flow step 9 says "re-route to handleGlobalKey(Ctrl+Q) which now sees saveInProgress==false and proceeds normally." Spec D73 says quit always confirms. Backup returns tea.Quit directly without QuitConfirm. TST-007 / save_flow_test mode 21 title says "reenterQuitFlow" but spec §Quit says "confirmation always required." If save succeeded and there are no unsaved changes, the no-unsaved-changes QuitConfirm dialog should appear. Backup skips it.
- **Raised by:** `behavioral-analyst` (BEH-001).
- **Evidence considered:** D73 ("The builder always confirms before exiting. Two dialog shapes."); backup model.go:787-788.
- **Recommended answer:** Follow D73. After a successful save with pendingQuit=true, the post-save state is "no unsaved changes"; the two-option QuitConfirm (Yes/No, No default) must appear before exit. Backup's direct tea.Quit is a D73 violation. Update the backup's handleSaveResult to re-route to handleGlobalKey(Ctrl+Q) as the plan specifies.
- **Blocks synthesis:** Yes — requires user acknowledgment to confirm D73 is authoritative over the backup behavior.

**OQ-PR2-9: Which of the 51 gaps are PR-2-required vs. deferred to vNext? (JrQ-PR2-002/010)**
- **Question:** No triage table exists. Without one, the DoD cannot be finalized.
- **Raised by:** `junior-developer` (JrQ-PR2-002, JrQ-PR2-010).
- **Evidence considered:** Gap list in implementation-gaps.md. Specialist P0/P1/P2 priority tags from edge-case-explorer. Specialist "P0 blocker" labels from security and behavioral.
- **Recommended starter classification (for user approval):**
  - **PR-2-required (P0 blockers):** GAP-001 (TUI wiring), GAP-002 (un-hide), GAP-003 (signal handler), GAP-020 (unsaved-changes interception), GAP-029 (editor invocation), GAP-033 (session logging), GAP-038/039/040/041 (data model), EC-PR2-001 (editorInProgress), EC-PR2-003 (pendingAction), EC-PR2-004 (data corruption), SEC-006 (signal handler).
  - **PR-2-required (functional completeness):** GAP-005 (session header), GAP-006 (read-only), GAP-007 (external-workflow), GAP-008 (symlink), GAP-009 (unknown-field), GAP-010 (outline sections), GAP-013 (Add affordance), GAP-014 (detail pane input), GAP-015 (choice lists), GAP-016 (numeric fields), GAP-017 (secret masking), GAP-018 (model suggestions), GAP-019 (path picker), GAP-021 (pre-copy integrity), GAP-022/023 (dialog handlers), GAP-024/025 (crash-temp/recovery actions).
  - **Deferred to vNext candidates (with issues):** GAP-011 (gripper always-visible), GAP-026 (no-op save feedback), GAP-032 (help modal per-mode detail), GAP-047 (footer context-sensitivity), GAP-048 (statusLine RefreshIntervalSeconds 0 vs omitempty), EC-PR2-015 (probe name collision after SIGKILL).
  - **Implicitly resolved or accepted:** GAP-043 (doc-integrity tests present), GAP-050 (file naming clarified), GAP-052 (version bump is PR-2 first commit).
- **Blocks synthesis:** Yes — without this classification the plan has no bounded scope.

---

## Standards-Conflict Check

No standards library (CLAUDE.md-level) conflicts were found that the plan is silently violating. All conflicts are flagged in the Inconsistencies section above. Confirmed standards issues requiring fixes before merge:

1. **docs/coding-standards/testing.md §"Each fake method must have its own call counter"** — fakeFS violates this (TST-004).
2. **docs/coding-standards/testing.md §"Test doubles with shared state need mutexes"** — fakeFS and fakeEditorRunner violate this (TST-010).
3. **docs/coding-standards/lint-and-tooling.md §"Never suppress lints"** — no suppressions found in the plan, but the rule must be respected as backup code is cherry-picked (no nolint comments).
4. **docs/coding-standards/file-writes.md (D-17 four rules)** — plan commits to this; atomicwrite.Write must be the path for all external file writes.

---

## Specialist Handoffs for Round 2

Only specialists whose new context would meaningfully change their findings are re-engaged.

| Specialist | Question for Round 2 | Evidence Needed |
|-----------|---------------------|-----------------|
| `behavioral-analyst` | Read `src/internal/validator/validator.go` — specifically `readCompanionOrDisk` and `statCompanionOrDisk` implementations. Do they use `filepath.Join("prompts", step.PromptFile)` (full-path key, matching Load) or bare `step.PromptFile` (bare key)? | validator.go source; D-3 implementation decision |
| `software-architect` | (1) Read backup's `cmd/pr9k/editor.go` ExecCallback implementation — confirm it produces package-local message types. Then decide: should those message types move to `workflowedit` package, live in a new `cmd/pr9k/messages.go`, or stay in editor.go with the workflowedit.Model receiving them via interface? (2) Confirm backup's 15th DialogKind constant and update D-8. | backup cmd/pr9k/editor.go; backup workflowedit/dialogs.go |
| `adversarial-security-analyst` | Read backup's `editor.go:85-93`. Confirm: (a) `$` is in the rejection set (confirming SEC-003 as a real bug), (b) `&`, `<`, `>` are absent (confirming SEC-008 as a real gap). Produce the correct rejection set. | backup editor.go:85-93; D33 |
| `user-experience-designer` | Read the backup's `docs/how-to/using-the-workflow-builder.md` and `docs/how-to/configuring-external-editor-for-workflow-builder.md`. Do they describe the pre-R2 landing-page flow or the current File-menu flow? Produce a list of sections requiring rewrite. | backup docs; R2 spec primary flow; D64-D72 |

Specialists **not** re-engaged in Round 2, with reason:

- `devops-engineer` — DOR findings are clear and actionable; no new context changes the findings. All actions are in the implementation plan.
- `concurrency-analyst` — CV findings are clear and actionable. CV-PR2-002 Option A/B decision is the one open item; it can be resolved in synthesis by deferring to the plan's D-33 intent (Option B preferred, Option A acceptable with documentation).
- `test-engineer` — TST findings are clear; fakeFS/mutex fixes are unambiguous; 28-mode restore is unambiguous.
- `edge-case-explorer` — EC findings are clear. P0s are unambiguous. EC-PR2-008 is elevated to OQ-PR2-7 for user input.
- `junior-developer` — All questions either resolve through evidence-based OQs above or through the gap-triage user input (OQ-PR2-9). No new code-reading task assigned.

---

## Next Step for the Conversation

**Continue facilitation — blocked pending:**

1. **User input on OQ-PR2-7** (spec vs. D-8 on `?` help modal reachability) and **OQ-PR2-8** (D73 QuitConfirm after successful save with pendingQuit).
2. **User input or PM-mediated gap triage for OQ-PR2-9** (which gaps are PR-2-required vs. deferred).
3. **Round 2 specialist handoffs** for `behavioral-analyst` (OQ-PR2-1/4), `software-architect` (message ownership + DialogKind), `adversarial-security-analyst` (rejection set confirmation), and `user-experience-designer` (how-to doc audit).

These four handoffs are targeted and will produce the missing evidence to close the four blocking OQs. After those close, the plan is ready for synthesis.

---

## Summary

Round 1 facilitation is complete across all 9 specialist domains (98 findings). The inner-ring packages from PR-1 are confirmed correct; the gap is entirely in the outer ring — `workflowedit.Model` and its wiring. The spec-maturity gate does not trip (zero T# contradictions; two spec-level findings by two specialists, below the five-by-three threshold). The plan is structurally sound but has four categories of blocking items before synthesis can proceed: (1) two spec-level ambiguities requiring user input (EC-PR2-008, BEH-001), (2) an evidence dispute about companion key convention (OQ-PR2-1), (3) a missing gap-triage table (OQ-PR2-9), and (4) ExecCallback message ownership boundary unresolved (R-PR2-D).

| Log category | Count |
|---|---|
| Evidenced / Anecdotal / Disputed claims | 85 / 9 / 4 |
| Risks / Assumptions / Issues | 7 new + 9 carry-forward / 6 / 5 |
| Decisions committed | 0 (facilitation mode — no decisions yet) |
| Open Questions | 9 |
| Specialist handoffs | 4 |

Next step: Continue facilitation — dispatch 4 specialist handoffs in parallel, await user input on OQ-PR2-7, OQ-PR2-8, and OQ-PR2-9, then go to synthesis.
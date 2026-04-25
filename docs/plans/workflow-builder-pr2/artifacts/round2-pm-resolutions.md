# PR-2 Workflow Builder — Round 2 Evidence and OQ Resolutions

## Round 2 specialist evidence (verbatim summaries)

### Behavioral analyst (R2): GAP-036 / OQ-PR2-1 / OQ-PR2-4 — companion key convention

**Verdict: GAP-036 is a false positive. JrQ-PR2-003 is correct.**

- Validator (`src/internal/validator/validator.go` lines 644, 667, 802) and load.go (`src/internal/workflowio/load.go` line 119) both use the same key format: `filepath.Join("prompts", step.PromptFile)` (e.g., `"prompts/step-1.md"`).
- The validator's doc comment on `ValidateDoc` (lines 166-168) explicitly states the contract: "Companion files keyed by path relative to workflowDir … Keys must use the full relative path — bare filenames like 'step-1.md' are cache misses and fall through to disk (F-121)."
- T3 (in-memory validation) is **satisfied**. Validator reads companion contents via `readCompanionOrDisk` using the same `relKey` format the loader writes.

**Disposition:** Close GAP-036 and EC-PR2-005. No PR-2 work needed in this area.

### Software architect (R2): ExecCallback message ownership boundary

**Verdict: Recommend Option A — move EditorExitMsg, EditorSigintMsg, EditorRestoreFailedMsg into `internal/workflowedit/editor.go`.**

- The current backup design has these three message types in `cmd/pr9k/editor.go` (`package main`), preventing `workflowedit.Model.Update` from type-switching on them.
- This is a DIP violation: `workflowedit` (high-level module) cannot handle its own domain events because the types are defined in `package main` (entry-point package).
- Option B (wrap in generic envelope) is double indirection. Option C (interface with concrete types in cmd/pr9k) is the same problem re-labeled.
- Option A is minimal and correct: types become `workflowedit.EditorExitMsg`, `workflowedit.EditorSigintMsg`, `workflowedit.EditorRestoreFailedMsg`. `cmd/pr9k/editor.go`'s `makeExecCallback` already imports `workflowedit`, so it constructs them by package-qualified names.

**DialogKind count confirmed:** 14 named non-zero DialogKind constants (15 iota values including DialogNone). All present from D-8 plus DialogSaveInProgress, DialogRecovery, DialogAcknowledgeFindings (added during plan iteration; D-8 should be updated in synthesis).

### Adversarial security analyst (R2): rejectShellMeta correctness

**Confirmed observations:**

- Backup `cmd/pr9k/editor.go:84` rejection set: `` ` ``, `;`, `|`, `$`, `\n` (5 chars).
- `$` is in the set: SEC-003 claim confirmed (D33 violation).
- `&`, `<`, `>` are absent: SEC-008 claim partially confirmed but **downgraded to non-issue** — under direct exec.Command (no shell), these characters are inert; their absence is intentional simplification, not a security gap.
- Single-quote and double-quote are correctly absent (they are valid quoting characters for shlex).

**Recommended corrected set:** `` ` ``, `;`, `|`, `\n` (4 chars — drop `$`, do NOT add `&`/`<`/`>`).

**Tests to update:**
- Remove the test asserting `$` is rejected.
- Add `TestResolveEditor_VisualWithDollar_Accepted`.
- No tests needed for `&`/`<`/`>` absence (intentional and inert).

**Security impact:** Removing `$` — no impact; exec.Command is not a shell. Plan-level decision: codify the rejection set as `{backtick, semicolon, pipe, newline}` and treat the spec D33 list as defense-in-depth that the implementation simplifies because exec.Command bypasses shell interpretation entirely.

### UX designer (R2): how-to docs alignment with R2 spec

**`docs/how-to/using-the-workflow-builder.md` (backup):**
- No landing-page or splash-screen references. Already R2-aligned.
- ASCII layout shows File menu bar; F10/Ctrl+N/Ctrl+O/Ctrl+S/Ctrl+Q all present. Empty-editor hint state matches D68.
- **One defect:** "Opening a Companion File in External Editor" section binds the action to `Ctrl+O`, which in R2 (D70) is File > Open. This is a key-binding collision.

**`docs/how-to/configuring-external-editor-for-workflow-builder.md` (backup):**
- No landing-page references.
- Same `Ctrl+O` collision in the "Verifying Your Configuration" section.
- All other sections (env-var lookup, shlex, metacharacter rejection, SIGINT, examples, session log entries) are independent of the menu model.

**Rewrite scope:** Two targeted paragraphs (one per doc). Everything else lands as-is. Both are blocked on a product decision: which key invokes the external editor for a focused prompt/script field?

---

## OQ Resolutions (PM-recommended, accepted under auto-mode authority)

### OQ-PR2-1 / OQ-PR2-4 (companion key convention) — RESOLVED BY EVIDENCE

**Resolution:** GAP-036 and EC-PR2-005 are false positives. Validator and load.go use matching key format `filepath.Join("prompts", step.PromptFile)`. T3 is satisfied. Close both findings.

### OQ-PR2-2 (rejectShellMeta set) — RESOLVED BY EVIDENCE

**Resolution:** The corrected rejection set is `` ` ``, `;`, `|`, `\n`. Drop `$` (SEC-003). Do NOT add `&`/`<`/`>` (SEC-008 downgraded — they are inert under direct exec). Update editor.go and tests in PR-2.

### OQ-PR2-3 (how-to docs) — RESOLVED BY EVIDENCE

**Resolution:** Both backup docs are R2-aligned with no landing-page references. Rewrite is two targeted paragraphs (one per doc) addressing the Ctrl+O collision for the external-editor invocation. Otherwise they land as-is.

**Sub-decision needed:** Which key invokes the external editor for a focused prompt/script field? Per spec §7 last bullet, it is "a visible shortcut in the footer when a prompt-or-script-path field is focused" — the spec does not pin a specific key.

**Recommended answer:** `Ctrl+E` (mnemonic: "edit"). It does not collide with any other global shortcut (Ctrl+N/O/S/Q are all File-menu actions; Ctrl+E is unused). The docs' two paragraphs are rewritten to use Ctrl+E in PR-2.

### OQ-PR2-5 (cherry-pick triage table) — RESOLVED BY PM RECOMMENDATION

**Resolution:** The plan adopts "cherry-pick + gap-close" as the strategy. A pre-implementation triage work unit (WU-PR2-0) produces a function-level table classifying every backup function as as-is / modify / replace before the cherry-pick lands. The triage table itself becomes part of the PR-2 plan artifacts.

### OQ-PR2-6 (commit graph) — RESOLVED BY PM RECOMMENDATION

**Resolution:** PR-2 commit sequence: (1) Version bump 0.7.2 → 0.7.3 as first commit (per D-18). (2) workflowmodel additions (UnknownFields, top-level Env, top-level ContainerEnv, DefaultModel) with tests. (3) workflowedit cherry-pick skeleton (compiles against main; widget files restored). (4) Gap-closure commits, one per gap cluster, each green under `make ci`. (5) Un-hide + rename-guard extensions as the LAST named commit (WU-13), making the un-hide independently revertable.

### OQ-PR2-7 (`?` help modal in dialogs) — RESOLVED BY PM-RECOMMENDED SPEC UPDATE

**Resolution:** Accept D-8 R2-C1: `?` is silently suppressed during non-findings-panel dialogs. The spec §8 sentence "unconditionally reachable from the edit view regardless of any other configuration" is updated to read "from the edit view or the findings panel, regardless of any other configuration." The shortcut footer for every dialog state must enumerate the available options in full so users have guidance without needing `?`. This becomes decision **D-PR2-OQ7**.

### OQ-PR2-8 (QuitConfirm after pendingQuit successful save) — RESOLVED BY PM RECOMMENDATION

**Resolution:** Follow D73. After a successful save with `pendingQuit=true`, the post-save state is "no unsaved changes"; the two-option QuitConfirm dialog (Yes / No, No default) must appear before exit. The backup's direct `tea.Quit` is a D73 violation that PR-2 must fix in `handleSaveResult`. Re-route to `handleGlobalKey(Ctrl+Q)` which now sees `saveInProgress==false` and `dirty==false`, opening the no-unsaved-changes QuitConfirm. This becomes decision **D-PR2-OQ8**.

### OQ-PR2-9 (gap triage) — RESOLVED BY PM RECOMMENDATION

**Resolution:** Adopt the PM's starter classification:

**PR-2-required (P0 blockers — must close to ship):**
- GAP-001 (TUI wiring), GAP-002 (un-hide), GAP-003 (signal handler), GAP-020 (unsaved-changes interception), GAP-029 (editor invocation), GAP-033 (session logging), GAP-038 (top-level env), GAP-039 (defaultModel), GAP-040 (step-level env/containerEnv), GAP-041 (UnknownFields), EC-PR2-001 (editorInProgress flag), EC-PR2-003 (pendingAction clears on fatals), EC-PR2-004 (data-corruption from missing schema fields), SEC-006 (signal handler program.Send wiring), SEC-001 (ExecCallback wired to Update), SEC-007 (logger injected into Model).

**PR-2-required (functional completeness):**
- GAP-005 (session header banner priority), GAP-006 (read-only browse mode), GAP-007 (external-workflow banner + first-save confirm), GAP-008 (symlink banner + first-save confirm), GAP-009 (unknown-field warn-on-load), GAP-010 (outline phase sections), GAP-013 (+ Add affordance), GAP-014 (detail pane input editing), GAP-015 (choice-list dropdown), GAP-016 (numeric fields), GAP-017 (secret masking key-pattern gate), GAP-018 (model-suggestion list), GAP-019 (path picker for File>New), GAP-021 (pre-copy integrity check), GAP-022 (DialogFirstSaveConfirm handler), GAP-023 (DialogFileConflict handler), GAP-024 (DialogCrashTempNotice handler), GAP-025 (DialogRecovery actions), GAP-027 (ackSet write), GAP-028 (ackSet write — same as 027), GAP-030 (recovery view → editor), GAP-031 (editor message handlers in Update), GAP-034 (shared-install detection wiring), GAP-035 (load pipeline forwarding), GAP-037 (script + statusLine companion loading), GAP-046 (unsaved-changes dialog rendering), GAP-045 (QuitConfirm Enter cancels — fixes wrong default), all P1 EC findings (EC-PR2-006 through EC-PR2-014), all SEC findings (SEC-002 through SEC-005, SEC-008 through SEC-011), all R-PR2 findings, all BEH findings, all CV-PR2 findings, all TST findings.

**Deferred to vNext (file issues; do not block PR-2):**
- GAP-011 (gripper always-visible glyph), GAP-026 (no-op save feedback "No changes to save"), GAP-032 (help modal per-mode detail; minimal version is fine for v1), GAP-047 (footer context-sensitivity refinement), GAP-048 (statusLine.RefreshIntervalSeconds 0 vs omitempty), EC-PR2-015 (probe name PID collision after SIGKILL).

**Implicitly resolved or accepted:**
- GAP-043 (doc-integrity tests — present after un-hide adds DI-1/DI-2 back), GAP-050 (file naming clarified — bundle_builder_integration_test.go exists), GAP-052 (version bump is PR-2 first commit).

**False positive (closed):**
- GAP-036 (companion key convention — see OQ-PR2-1 resolution).

This becomes decision **D-PR2-OQ9**.

### OQ-PR2-10 (CV-PR2-002 ctx propagation) — DEFERRED TO PLAN

**Resolution:** PM noted the CV-PR2-002 Option A vs Option B decision is acceptable in synthesis. **Choose Option B** (pass ctx into closures with select on ctx.Done at blocking points) for `scanMatches` and the validator/save closures, matching D-33 intent. Document that Bubble Tea-owned goroutines outside the builder's own closures (the runtime's tick scheduler, etc.) are exempt by construction. This becomes decision **D-PR2-OQ10**.

### Additional sub-decisions surfaced in R2

**D-PR2-R2-1: External-editor shortcut key.** Use `Ctrl+E` for "open prompt/script in external editor" when a prompt/script-path field is focused. Updates both how-to docs.

**D-PR2-R2-2: Move ExecCallback message types into `workflowedit`.** Make EditorExitMsg, EditorSigintMsg, EditorRestoreFailedMsg exported types in `internal/workflowedit/editor.go`. cmd/pr9k/editor.go's makeExecCallback constructs them by package-qualified names.

**D-PR2-R2-3: D-8 is updated to reflect the 14 named DialogKind constants.** Decision-log entry adjusted in synthesis. The 14 are: DialogPathPicker, DialogNewChoice, DialogUnsavedChanges, DialogQuitConfirm, DialogExternalEditorOpening, DialogFindingsPanel, DialogError, DialogCrashTempNotice, DialogFirstSaveConfirm, DialogRemoveConfirm, DialogFileConflict, DialogSaveInProgress, DialogRecovery, DialogAcknowledgeFindings.

**D-PR2-R2-4: Rejection set in rejectShellMeta is `{backtick, semicolon, pipe, newline}`.** Plan adjusts D33 implementation note to match. Test changes per OQ-PR2-2.

---

## Round 2 facilitation status

All 9 specialist Round 1 findings (98 total) are now either closed by evidence, resolved by PM recommendation, or scheduled into the PR-2 plan. The spec-maturity gate did not trip in Round 1 and remains untripped in Round 2 (the two spec-level findings — EC-PR2-008 and BEH-001 — are resolved by D-PR2-OQ7 and D-PR2-OQ8 with explicit spec-text updates committed as decisions).

**Next step: project-manager synthesis mode.** Authoritative inputs for synthesis are this file, the Round 1 specialist outputs, the Round 1 facilitation summary, and the source-spec materials.

# Implementation Iteration History: Workflow Builder PR-2

<!--
This file records how the PR-2 implementation plan evolved across two
discussion rounds plus a synthesis pass. Committed decisions live in
[implementation-decision-log.md](implementation-decision-log.md) and the
primary plan lives in [../feature-implementation-plan.md](../feature-implementation-plan.md).

PR-2 inherits the R1..R4 history of the original feature-implementation-plan
([../../workflow-builder/artifacts/implementation-iteration-history.md](../../workflow-builder/artifacts/implementation-iteration-history.md));
the R1/R2 rounds below are PR-2-specific and append to (rather than replace)
the inherited history.

Cross-referencing invariants:
- `Decisions produced:` — D# IDs from [implementation-decision-log.md](implementation-decision-log.md)
  that this round added or changed. `—` if the round produced no new or changed decisions.
- `Changed in plan:` — sections of [../feature-implementation-plan.md](../feature-implementation-plan.md)
  that this round updated. `—` if nothing in the plan changed.
-->

## R1: Parallel specialist review across the gap analysis (2026-04-24)

- **Specialists engaged:** `project-manager` (coordinator), `junior-developer`, `user-experience-designer`, `adversarial-security-analyst`, `devops-engineer`, `software-architect`, `behavioral-analyst`, `concurrency-analyst`, `test-engineer`, `edge-case-explorer`. All 9 specialist domains were activated; no specialist stood down.
- **New input provided:** The PR-1 merge state on `main` (commit `cbf7753`); the backup tag `backup/workflow-builder-mode-full-2026-04-24` preserving ~4,140 LOC across 27 `internal/workflowedit/` files plus `cmd/pr9k/editor.go`; the gap analysis at `../../workflow-builder/implementation-gaps.md` enumerating 51 numbered gaps (GAP-001..GAP-052; GAP-036 disputed); the inherited spec, plan, decision logs, technical notes, team findings, review findings, and the 28-entry TUI mode coverage table.
- **Questions raised:**
  - **OQ-PR2-1** — Is GAP-036 a real gap? (EC-PR2-005 vs JrQ-PR2-003 dispute on companion key convention)
  - **OQ-PR2-2** — What is the exact rejection set in `rejectShellMeta` and does it conflict with D33? (SEC-003 vs SEC-008)
  - **OQ-PR2-3** — Do the backup how-to docs describe the superseded landing-page flow? (JrQ-PR2-007)
  - **OQ-PR2-4** — Same as OQ-PR2-1 restated from D-3's perspective (validator companion key convention)
  - **OQ-PR2-5** — What fraction of backup's 988-line `model.go` and 27 files can be cherry-picked as-is vs requires rewrite? (JrQ-PR2-001)
  - **OQ-PR2-6** — What is the commit graph for PR-2 such that every intermediate commit passes `make ci`? (JrQ-PR2-009, DOR-002)
  - **OQ-PR2-7** — Does `?` help modal suppress in non-findings-panel dialogs? (EC-PR2-008 spec-level)
  - **OQ-PR2-8** — After successful save with `pendingQuit`, does the builder show QuitConfirm or exit directly? (BEH-001 spec-level)
  - **OQ-PR2-9** — Which of the 51 gaps are PR-2-required vs deferred to vNext? (JrQ-PR2-002, JrQ-PR2-010)
  - **OQ-PR2-10** — CV-PR2-002 ctx-propagation Option A vs Option B
- **Resolution source:**
  - **OQ-PR2-1, OQ-PR2-4** — deferred to R2 (behavioral-analyst code inspection of validator)
  - **OQ-PR2-2** — deferred to R2 (security-analyst code inspection of backup editor.go)
  - **OQ-PR2-3** — deferred to R2 (UX designer audit of backup how-to docs)
  - **OQ-PR2-5** — deferred to synthesis (PM-recommended adoption of cherry-pick + WU-PR2-0 triage table)
  - **OQ-PR2-6** — deferred to synthesis (PM-recommended commit graph)
  - **OQ-PR2-7** — deferred to synthesis (PM-recommended spec §8 update accepting D-8 R2-C1)
  - **OQ-PR2-8** — deferred to synthesis (PM-recommended D73 authoritative; backup is bug)
  - **OQ-PR2-9** — deferred to synthesis (PM-recommended starter classification)
  - **OQ-PR2-10** — deferred to synthesis (PM-recommended Option B with Bubble Tea-runtime exemption)
- **Decisions produced:** — (R1 produced findings and OQs only; no decisions committed yet — facilitation mode)
- **Changed in plan:** — (R1 was facilitation; no plan file written yet)
- **Project-manager next-step recommendation:** Continue facilitation — dispatch 4 targeted Round-2 specialist handoffs in parallel (`behavioral-analyst`, `software-architect`, `adversarial-security-analyst`, `user-experience-designer`); apply PM-recommended resolutions for OQ-PR2-5 through OQ-PR2-10 under auto-mode authority; then go to synthesis.

## R2: Targeted evidence handoffs and OQ resolutions (2026-04-24)

- **Specialists engaged:** `project-manager` (coordinator), `behavioral-analyst` (re-engaged), `software-architect` (re-engaged), `adversarial-security-analyst` (re-engaged), `user-experience-designer` (re-engaged). Five specialists were **not** re-engaged — `devops-engineer`, `concurrency-analyst`, `test-engineer`, `edge-case-explorer`, `junior-developer` — because R1 findings from those domains were clear, actionable, and had no new context that would change them.
- **New input provided:** R1 facilitation summary at `/tmp/pr9k-pr2-planning/round1-pm-facilitation.md` (claim ledger, RAID, OQs, classifications); R1 specialist outputs at `/tmp/pr9k-pr2-planning/round1-specialist-outputs.md`; targeted code-inspection assignments per specialist (validator key convention, backup editor.go rejection set, backup ExecCallback message types, backup how-to docs).
- **Questions raised:** No new questions; R2 was scoped to closing the four code-inspection OQs (OQ-PR2-1/2/3/4) and accepting PM-recommended resolutions on the synthesis-bound OQs (OQ-PR2-5..10).
- **Resolution source:**
  - **OQ-PR2-1, OQ-PR2-4 (companion key convention)** — RESOLVED BY EVIDENCE. Behavioral-analyst code inspection confirmed `src/internal/validator/validator.go:644,667,802` and `src/internal/workflowio/load.go:119` both use `filepath.Join("prompts", step.PromptFile)`. T3 satisfied; GAP-036 / EC-PR2-005 closed as false positive.
  - **OQ-PR2-2 (rejectShellMeta)** — RESOLVED BY EVIDENCE. Security re-engagement confirmed backup editor.go:84 set is `` ` `` `;` `|` `$` `\n`. Corrected set: `` ` `` `;` `|` `\n` (drop `$`; do not add `&`/`<`/`>` since exec.Command bypasses shell).
  - **OQ-PR2-3 (how-to docs)** — RESOLVED BY EVIDENCE. UX re-engagement confirmed both backup how-tos are R2-aligned with no landing-page references. Defect: both bind external-editor invocation to `Ctrl+O`, colliding with File > Open. Recommended `Ctrl+E`.
  - **OQ-PR2-5 (cherry-pick triage table)** — RESOLVED BY PM RECOMMENDATION (cherry-pick + gap-close strategy with WU-PR2-0 as artifact-only pre-implementation work unit).
  - **OQ-PR2-6 (commit graph)** — RESOLVED BY PM RECOMMENDATION (version-bump-first, schema additions, cherry-pick skeleton, gap-closure series, un-hide-last).
  - **OQ-PR2-7 (`?` help modal)** — RESOLVED BY PM-RECOMMENDED SPEC UPDATE (accept D-8 R2-C1; spec §8 sentence updated).
  - **OQ-PR2-8 (QuitConfirm after pendingQuit)** — RESOLVED BY PM RECOMMENDATION (D73 authoritative; backup direct `tea.Quit` is a bug).
  - **OQ-PR2-9 (gap triage)** — RESOLVED BY PM RECOMMENDATION (starter classification: 16 P0 blockers, ~30 functional-completeness, 6 deferred to vNext, 3 implicit, 1 false-positive).
  - **OQ-PR2-10 (ctx propagation)** — RESOLVED BY PM RECOMMENDATION (Option B with Bubble Tea-runtime exemption).
  - **R2 sub-decisions surfaced and accepted:**
    - **D-PR2-R2-1** — External-editor shortcut = `Ctrl+E`
    - **D-PR2-R2-2** — Move `EditorExitMsg` / `EditorSigintMsg` / `EditorRestoreFailedMsg` into `internal/workflowedit/editor.go`
    - **D-PR2-R2-3** — D-8 updated to enumerate 14 named DialogKind constants
    - **D-PR2-R2-4** — Rejection set in rejectShellMeta is `{backtick, semicolon, pipe, newline}`
- **Decisions produced (committed in synthesis):** D-PR2-5 (editor message types in workflowedit), D-PR2-6 (rejectShellMeta set), D-PR2-7 (D-8 14 constants), D-PR2-8 (Ctrl+E external-editor shortcut), D-PR2-9 (`?` suppression), D-PR2-10 (QuitConfirm after pendingQuit), D-PR2-11 (gap triage), D-PR2-12 (GAP-036 false positive), D-PR2-13 (ctx Option B).
- **Changed in plan:** Implementation Approach > Architecture and Integration Points (added: editor message ownership flip; D-8 constant enumeration); Implementation Approach > Runtime Behavior (added: Ctrl+E binding, `?` suppression rule, QuitConfirm-after-pendingQuit, ctx propagation to closures); Decomposition and Sequencing (committed: WU-PR2-0 triage, version-bump-first, un-hide-last); Security Posture (corrected rejection set); RAID Log (closed R-PR2-C as false positive; closed R-PR2-D via D-PR2-5; closed R-PR2-E via D-PR2-7 + D-PR2-17; closed R-PR2-G via D-PR2-15); Definition of Done (added: cherry-pick triage table; un-hide as last commit; spec §8 update).
- **Project-manager next-step recommendation:** Go to synthesis. All 9 R1 specialist domains have either resolved findings or scheduled them into the PR-2 plan. Spec-maturity gate did not trip in R1 and remains untripped in R2.

## Synthesis: Plan, decision log, and history committed (2026-04-24)

- **Specialists engaged:** `project-manager` (synthesis only — no new specialist invitations).
- **New input provided:** R1 facilitation summary, R2 evidence-and-resolutions document, the inherited plan/spec/decision-log/technical-notes/tui-mode-coverage artifacts, and the gap analysis classifications resolved during R2.
- **Questions raised:** None. Synthesis is the close-out pass; no new questions are raised by definition.
- **Resolution source:** All open questions from R1 and R2 are committed to decisions per the resolution-source table in R2 above.
- **Decisions produced:** D-PR2-1 through D-PR2-23 — 23 PR-2-specific decisions inheriting D-1..D-47 from the original plan unchanged. Specifically:
  - **R1-driven (committed in synthesis):** D-PR2-14 (editorInProgress flag), D-PR2-15 (pendingAction discriminated type), D-PR2-16 (logger injected via workflowDeps), D-PR2-17 (conflict-detection dialog handlers), D-PR2-18 (workflowmodel.IsDirty replaces ad-hoc dirty), D-PR2-19 (routing pre-dispatch for async messages), D-PR2-20 (path-completion generation counter), D-PR2-21 (capture *tea.Program before signal goroutine), D-PR2-22 (drain pattern after program.Run), D-PR2-23 (10s signal-timer cancellation), D-PR2-2 (commit graph), D-PR2-3 (version bump 0.7.3), D-PR2-4 (workflowmodel schema additions).
  - **R2-driven (committed in synthesis):** D-PR2-5, D-PR2-6, D-PR2-7, D-PR2-8, D-PR2-9, D-PR2-10, D-PR2-11, D-PR2-12, D-PR2-13.
  - **Synthesis-driven:** D-PR2-1 (cherry-pick + gap-close strategy as the umbrella decision, formalizing the approach the prior rounds converged on).
- **Changed in plan:** All sections of `../feature-implementation-plan.md` written for the first time as part of synthesis (the file did not exist before this pass).
- **Project-manager next-step recommendation:** **Ship as planned.** All 23 PR-2-specific decisions plus the inherited 47 have specialist owners and revisit criteria. Recommend proceeding to WU-PR2-0 (cherry-pick triage table) as the first implementation work unit, followed by WU-PR2-1 (version bump) and WU-PR2-2 (workflowmodel schema additions). The un-hide commit (WU-PR2-13) is the last named commit and the rollback boundary.

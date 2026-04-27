# Implementation Iteration History: Workflow-Builder TUI Visual-Design

<!--
This file records how the implementation plan for the workflow-builder TUI
visual-design layer evolved across discussion rounds. Committed decisions live
in [implementation-decision-log.md](implementation-decision-log.md) and the
primary plan lives in [../feature-implementation-plan.md](../feature-implementation-plan.md).

The iteration loop is capped at four rounds. The visual-design implementation
planning concluded in a single round (R-1) because the spec-maturity gate did
not trip and the project-manager auto-resolved all four open questions under
auto-mode authority.
-->

## R-1: Parallel specialist review — visual-design implementation planning

- **Specialists engaged:** `software-architect`, `structural-analyst`, `behavioral-analyst`, `concurrency-analyst`, `edge-case-explorer`, `test-engineer`, `user-experience-designer`, `devops-engineer`, `risk-analyst`, `junior-developer`. The `project-manager` coordinated facilitation and synthesis. `adversarial-validator` was named for the post-synthesis gate (handoff queued, not yet engaged in R-1). `gap-analyzer`, `information-architect`, and behavioral specialists outside this list were stood down — gaps were already resolved in the visual spec's team-findings F1–F25, and the implementation does not change information-architecture surfaces.
- **New input provided:** The shared brief at `/tmp/visual-design-brief.md` (mission, source spec citations, behavioral ground truth, current implementation state at `src/internal/workflowedit/`, project context including ADRs and coding standards). The visual spec at `docs/plans/workflow-builder-tui-design/feature-specification.md` (51 D# decisions, 25 mockups). The visual decision-log, team-findings, and visual-gaps artifacts. The behavioral spec and its implementation plan and decision log. The PR-2 cherry-pick + gap-close plan.
- **Questions raised:**
  - Q from junior-developer: `internal/uichrome` extraction vs re-export from `internal/ui` (Q1). Settled by evidence and software-architect A1, A3 → became D-2.
  - Q from junior-developer: Does extraction violate the narrow-reading ADR? (Q2). Settled by evidence (ADR governs Go-vs-config separation, not intra-Go structure) → no decision needed.
  - Q from junior-developer: Timer pattern — `tea.Tick` vs `tea.After` vs lazy timestamp (Q4). Settled by concurrency-analyst C2/C3 reasoning (View must be pure) → became D-7.
  - Q from junior-developer: Time injection seam (Q9). Settled by test-engineer recommendation → became D-8.
  - Q from junior-developer: D43 vs HintEmpty (Q6). Settled by structural-analyst S12 + edge-case-explorer EC19 → became D-26.
  - Q from junior-developer: `m.dirty` vs `IsDirty()` (Q10). Settled by behavioral-analyst B12 + UX#3 → became D-11.
  - Q from junior-developer: `Ctrl+E` from detail pane (Q8). Settled by behavioral-analyst B8 + visual-spec D32 → became D-16.
  - Q from junior-developer: Build-order / commit graph (Q12). Settled by risk-analyst ordering hazards → became D-9.
  - Q from junior-developer: WindowSizeMsg routing fix (Q13). Settled by behavioral-analyst B13 + concurrency-analyst C1 → became D-14.
  - Q from junior-developer: Smallest shippable slice (Q14). Settled by test-engineer file-by-file triage → became D-6, D-9.
  - Q from PM: Does any D# contradict another D#? (spec-maturity gate test). Resolved: 9 risk-analyst escalations re-classified as gap-fill (placeholder code superseded by spec); zero true D#-vs-D# contradictions.
  - OQ-1: Does `fieldKindMultiLine` exist already? Resolved post-facilitation by code search (`grep -n "fieldKind" /Users/mxriverlynn/dev/mxriverlynn/pr9k/src/internal/workflowedit/detail.go` shows enum has only 5 kinds, no multi-line) → became D-16.
  - OQ-2: Will `funlen`/`gocyclo` fire on render functions? Resolved post-facilitation by `.golangci.yml` inspection (no project-level config; defaults include gocyclo at threshold 30 but not funlen/mnd; A4 decomposition is sufficient) → influenced D-3 (decomposition retained as good practice).
  - OQ-3: Bubble Tea ExecProcess resize gap — undocumented. Recorded as open item with default = document as known limitation.
  - OQ-4: Single-PR vs multi-PR delivery. Recorded as open item with default = single PR with 13 commits per D-9.
- **Resolution source:** evidence (28 of 36 claims resolved by code citations); junior-developer reframing (Q1, Q4, Q6, Q9, Q10, Q12 reframed away from premature solution-seeking); PM auto-decisions (4: OD-1 timer pattern, OD-2 time injection, OD-3 uichrome extraction, OD-4 commit graph) under auto-mode authority; specialist domain expertise (architecture, structure, behavior, concurrency, edge cases, testing, UX, DevOps, risk).
- **Decisions produced:** D-1, D-2, D-3, D-4, D-5, D-6, D-7, D-8, D-9, D-10, D-11, D-12, D-13, D-14, D-15, D-16, D-17, D-18, D-19, D-20, D-21, D-22, D-23, D-24, D-25, D-26.
- **Changed in plan:** Source Specification (linked to `feature-specification.md`, `artifacts/decision-log.md`, `artifacts/team-findings.md`, `artifacts/visual-gaps.md`, `artifacts/facilitation-summary.md`); Outcome (full bordered TUI rendering all D1–D51 invariants); Context (driving constraint, stakeholders, future-state concern, out-of-scope); Team Composition and Participation (10 specialists + PM + adversarial-validator); Implementation Approach → Architecture and Integration Points (uichrome, render file map, dialog grammar, field grammar); Implementation Approach → Runtime Behavior (render-time geometry, banner timer, phase-boundary, revealedField reset, WindowSizeMsg routing); Implementation Approach → External Interfaces (Ctrl+E wiring, fieldKindMultiLine, browse-only signals); Decomposition and Sequencing (13-commit table); RAID Log (R1–R10, A1–A7, I1–I3, DEP-1..DEP-6 carried forward from facilitation); Testing Strategy (ANSI-stripped substring + structural assertions, nowFn time injection, generation-counter sync tests, file triage); Security Posture (secret-mask leak fix, no new external-input surfaces); Operational Readiness (no version bump, session-event logging, render perf, documentation in same PR); Definition of Done (8 acceptance criteria carried from facilitation summary); Specialist Handoffs (adversarial-validator, concurrency-analyst, test-engineer); Open Items (OQ-3, OQ-4, deferred P2/P3 risks).
- **Project-manager next-step recommendation:** Go to synthesis. The spec-maturity gate did not trip. All 10 specialists were heard. Four open questions did not block synthesis (all answerable in implementation or by post-facilitation evidence). Synthesis produces `feature-implementation-plan.md` and the two artifacts (this file + `implementation-decision-log.md`). After synthesis, dispatch `adversarial-validator` for the post-synthesis gate before merge.

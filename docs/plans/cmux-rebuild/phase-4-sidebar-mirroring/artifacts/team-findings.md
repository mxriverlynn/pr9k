# Team Findings: Phase 4 — Sidebar Mirroring

Review-team findings on the [Phase 4 feature specification](../feature-specification.md) and how each was resolved. Each finding is assigned a stable `F#` identifier; the same counter is shared across major and minor classes.

**Size classification (per the plan-a-feature skill, Step 5.5).** Small. Phase 4 is a single-subsystem extension — pr9k's existing cmux client gains two surface mutators, the existing in-pane orchestrator's header-update adapter gains a parallel sidebar projection, and there is no auth surface, no PII, no migration, and no cross-service work. Team cap: 2 (`junior-developer` + 1 specialist).

**Specialists consulted:** `junior-developer` (always-on generalist) and `user-experience-designer` (the only domain question is whether the operator-facing sidebar entries are useful and clear).

## Major findings

### F1: "Same cadence as header" is undefined

- **Raised by:** `junior-developer`
- **Location:** `feature-specification.md#outcome`, `feature-specification.md#primary-flow` (step 4), `decision-log.md` D10
- **Category:** Behavioral gap
- **Description:** the spec stated "for every header push there is exactly one corresponding sidebar push" but the header push cadence is not defined in this spec or in the parent. If the header pane is updated on a richer cadence than just step transitions (e.g., intra-step heartbeats), the same rule would produce many sidebar pushes per step.
- **Resolution: evidence.** Tightened the cadence rule in the Outcome and Primary Flow to anchor sidebar pushes to two specific events — a step-name change or an iteration-counter advance — and to no other event. Updated D10 to reflect the refined rule.
- **Affected decisions:** D10
- **Affected tech-notes:** —
- **Changed in spec:** Outcome, Primary Flow

### F2: First-push timing and ordering ambiguity

- **Raised by:** `junior-developer`
- **Location:** `feature-specification.md#primary-flow` (steps 2–3)
- **Category:** Behavioral gap
- **Description:** the spec did not state whether the initial status and progress pushes happen as part of the same first step transition, what happens if one succeeds and the other times out, and whether D6's cleanup covers an abort during the initial-push sequence.
- **Resolution: evidence.** Rewrote Primary Flow steps 2 and 3 to state that both initial pushes occur as part of the first step's state event, that the two pushes are independent (a non-timeout failure of one does not block the other), and that a cmux timeout follows parent D15 with the abort path clearing whichever entry was successfully pushed.
- **Affected decisions:** D6 (clarified scope)
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow

### F3: Iteration counter starting value (N=0 or N=1) is undefined

- **Raised by:** `junior-developer`
- **Location:** `feature-specification.md#primary-flow` (step 3), `decision-log.md` D3
- **Category:** Behavioral gap
- **Description:** the spec said the progress fraction is `N / M` but did not state what N is at run start. The edge case table said "from `1/M`" but never named what point in the iteration lifecycle (begin or complete) the counter advances at.
- **Resolution: evidence.** Stated explicitly in Primary Flow step 3 that `N` is the iteration currently running, so the first visible value is `1 / M` from the start of iteration 1, and `K / M` while iteration K is currently running.
- **Affected decisions:** D3 (sharpened)
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow

### F4: "Every run-end path" has no inventory; crash-path cleanup scoping unstated

- **Raised by:** `junior-developer`
- **Location:** `feature-specification.md#primary-flow` (step 7), `decision-log.md` D6
- **Category:** Behavioral gap / Missing coordination
- **Description:** D6 said "cleared on every run-end path" but the spec also said failure modes are out of scope for Phase 4. This produces a coverage gap between Phase 4 and Phase 6 that an operator using Phase 4 alone would experience as stale sidebar entries after a crash.
- **Resolution: evidence.** Scoped D6 to **graceful** run-end paths and named the failure-path handoff to Phase 6 explicitly. Updated the spec Outcome, Primary Flow step 8, and D6 itself; renamed D6 to "Sidebar entries are cleared on every **graceful** run-end path".
- **Affected decisions:** D6 (scope tightened); D17 (new — names the Phase 6 integration point, see F6)
- **Affected tech-notes:** —
- **Changed in spec:** Outcome, Primary Flow, Edge Cases and Failure Modes

### F6: Phase 6 integration point is assumed, not specified

- **Raised by:** `junior-developer`
- **Location:** `feature-specification.md#edge-cases-and-failure-modes` (last rows)
- **Category:** Missing coordination
- **Description:** the build-phase-outline said Phase 6 "Builds on Phase 4 — the sidebar entries this phase clears during failure handling are the entries Phase 4 pushes." The Phase 4 spec referred to this integration but did not state whether Phase 4 must expose a hook or whether Phase 6 calls the same sidebar-clear operations directly.
- **Resolution: evidence.** Added D17 (trivial): Phase 6 integrates by calling the same sidebar-clear operations the orchestrator's normal shutdown sequence calls; no new interface is required from Phase 4. Linked from Primary Flow step 8.
- **Affected decisions:** D17 (new, trivial)
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow, Edge Cases and Failure Modes

### F8: `pr9k.step` key string leaks into the decision log as an implementation detail

- **Raised by:** `junior-developer`
- **Location:** `decision-log.md` D2
- **Category:** Mechanics leaking into spec
- **Description:** D2 named the specific key string `pr9k.step` parenthetically. The spec-level commitment is "stable and pr9k-prefixed"; the specific key is an implementation choice.
- **Resolution: evidence.** Removed the `pr9k.step` parenthetical from D2; D2 now commits only to "stable and pr9k-prefixed". The implementation plan pins the exact string.
- **Affected decisions:** D2 (cleaned)
- **Affected tech-notes:** —
- **Changed in spec:** (no change — the leak was in D2 only)

### F9: `internal/cmuxctl.CmuxClient` and file path named in decision-log evidence

- **Raised by:** `junior-developer`
- **Location:** `decision-log.md` D1 (Evidence section, last bullet)
- **Category:** Mechanics leaking into spec
- **Description:** D1's evidence section ended with a bullet naming the Go interface `internal/cmuxctl.CmuxClient` and the file `src/internal/cmuxctl/client.go` as the natural extension point. Internal Go package/type/file names belong in the implementation plan, not the decision log.
- **Resolution: evidence.** Rewrote the evidence bullet behaviorally: "The orchestrator already holds an active cmux v2 client used for workspace lifecycle and other cmux calls; the same client handles the sidebar surface — no new client infrastructure is required."
- **Affected decisions:** D1 (evidence cleaned)
- **Affected tech-notes:** —
- **Changed in spec:** (no change — the leak was in D1's evidence only)

### F12: Capability check for new sidebar methods not mentioned in preconditions

- **Raised by:** `junior-developer`
- **Location:** `feature-specification.md#actors-and-triggers` (Preconditions)
- **Category:** Open question / Missing coordination
- **Description:** Phase 1 ships a startup capability check (parent D18) that aborts the launch if a method pr9k will call is missing. Phase 4 calls two new cmux methods. Without adding them to the capability check, a cmux build missing those methods would produce silent runtime failures swallowed by D5's "log and continue" path.
- **Resolution: evidence.** Added D15 (trivial): the Phase 1 capability check is extended to cover Phase 4's sidebar methods at Phase 4 implementation time. Added a precondition bullet referencing D15 and an error-state bullet in User Interactions naming the "missing method aborts launch" path.
- **Affected decisions:** D15 (new, trivial)
- **Affected tech-notes:** —
- **Changed in spec:** Actors and Triggers (Preconditions), User Interactions (Error states)

### F14: Error mode invisible on the monitoring surface Phase 4 introduces (UX-F1)

- **Raised by:** `user-experience-designer`
- **Location:** `feature-specification.md#alternate-flows-and-states` ("The run ends in error mode without operator action"), `feature-specification.md#out-of-scope` ("A failure-specific sidebar signal")
- **Category:** Behavioral gap (UX — Visibility of system status, Perceptible Information)
- **Description:** Phase 3 commits the run to blocking indefinitely in error mode. Phase 5's notification can be dismissed; if it is, the operator monitoring from another workspace has no glanceable signal that the run is paused — the sidebar pill still shows the failing step's name, indistinguishable from a healthy mid-step state. A Claude step can run for many minutes legitimately; the operator cannot distinguish "still working" from "paused 35 minutes ago" from the sidebar alone.
- **Resolution: user input.** Adopted UX-F1's recommendation: when the orchestrator enters error mode, the status pill's value gains a stable error-mode marker (e.g., `Feature work — awaiting input`); the marker reverts on the next normal step push or at run-end clear. New full decision D13.
- **Affected decisions:** D13 (new, full)
- **Affected tech-notes:** T1 (Supports decisions updated to include D13)
- **Changed in spec:** Outcome, Primary Flow (step 6 added), Alternate Flows and States (the error-mode alternate flow now describes the marker behavior), User Interactions, Out of Scope (removed the "no failure-specific sidebar signal in Phase 4" line because Phase 4 now ships exactly that signal in textual form), Summary

## Minor edits

- **F5: Error-mode alternate flow conflated paused-state with terminal-state.** Raised by `junior-developer`. Renamed the heading "The run ends in error mode without operator action" to "The run is paused in error mode awaiting the operator's decision" and rewrote the entry condition to be unambiguous about the workflow still being alive. — Affected decisions: D13 (covers the rewritten flow). — Changed in spec: Alternate Flows and States.
- **F7: `M / M` visibility at iteration loop boundary unspecified.** Raised by `junior-developer`. Updated Primary Flow step 7 and the iteration-loop-ends alternate flow to state explicitly that the orchestrator pushes the terminal iteration value (`M / M` for natural completion, the last-completed iteration's value for an early break) before clearing the progress entry at the finalization transition. — Affected decisions: D7 (sharpened). — Changed in spec: Primary Flow, Alternate Flows and States.
- **F10: "Operator switches to a different cmux workspace" alternate flow added no new behavior (YAGNI symmetry).** Raised by `junior-developer`. Removed the standalone alternate flow; its content (workspace-targeted pushes are focus-independent) is already implied by the Outcome paragraph and D1's commitment to workspace-handle scoping. — Affected decisions: —. — Changed in spec: Alternate Flows and States (section removed).
- **F11: No spec for progress push at `breakLoopIfEmpty` break point.** Raised by `junior-developer`. Added one sentence to the Edge Cases row for early loop exit: "no additional progress push is made at the break point." — Affected decisions: —. — Changed in spec: Edge Cases and Failure Modes.
- **F13: Initialize-phase steps not mentioned in sidebar coverage.** Raised by `junior-developer`. Stated explicitly in Primary Flow step 2 that all workflow steps participate — initialize, iteration, and finalization phases. Added D14 (trivial). — Affected decisions: D14 (new, trivial). — Changed in spec: Primary Flow.
- **F15: UX-F2 — finalization ambiguity.** Raised by `user-experience-designer`. User declined adding a finalization marker; sharpened the Deferred (YAGNI) section to name the reopen trigger ("operators report that they routinely misread 'finalization step' as 'still iterating', or workflow authors begin shipping finalization steps whose names collide with iteration step names"). — Affected decisions: —. — Changed in spec: Deferred (YAGNI).
- **F16: UX-F3 — default pill styling provides no guaranteed visual distinction.** Raised by `user-experience-designer`. Sharpened the YAGNI reopen condition for icon/color/priority overrides to name the monitoring-from-another-workspace case specifically ("operators report that they cannot identify the pr9k pill at a glance when other tools' pills are also present"). — Affected decisions: —. — Changed in spec: Deferred (YAGNI).
- **F17: UX-F5 — forced-kill edge case incorrectly defers stale-entry cleanup to Phase 7.** Raised by `user-experience-designer`. Corrected the forced-kill row in the Edge Cases table: Phase 7 prints a startup advisory naming orphan workspaces; it does not itself clear stale sidebar entries. Stale entries remain until the operator dismisses the orphan workspace. — Affected decisions: —. — Changed in spec: Edge Cases and Failure Modes.
- **F18: UX-F4 — no defined starting state for the sidebar between launch and first step.** Raised by `user-experience-designer`. Added one sentence to Primary Flow step 1 plus a new trivial decision D16: "Between launch and the first step transition the pr9k workspace's sidebar row carries no pr9k entries — this is an expected, bounded gap, not a missing-state bug." — Affected decisions: D16 (new, trivial). — Changed in spec: Primary Flow.

## Synthesis corrections

- **F19: CLI verb names in D1's Decision statement violated the spec-content rule.** Raised by: `project-manager (synthesis)`. Category: Mechanics leaking into spec. The D1 Decision field named the specific cmux CLI verb names `set-status`, `clear-status`, `set-progress`, `clear-progress` inline as parentheticals. Per the operating principle, implementation identifiers belong only in `Evidence:` blocks, not in the Decision statement itself. The same identifiers were already fully documented in the Evidence and Rationale sections. — Resolution: removed the parenthetical CLI verb clauses from the Decision statement; the behavioral commitment ("the step name entry is pushed to cmux's sidebar status-entry surface") stands without them. Evidence is unchanged. — Affected decisions: D1 (Decision field cleaned). — Changed in spec: (no change — the violation was in the decision log only).
- **F20: Internal Go adapter name in spec's Open Items sentence.** Raised by: `project-manager (synthesis)`. Category: Mechanics leaking into spec. The Open Items section stated "…the existing `cmuxHeader` adapter" — naming an internal Go type directly in a behavioral spec sentence. — Resolution: removed the implementation identifier; the sentence now reads "…parent decisions D5 / D-R1 / D-R2 / D15 / D17" without naming a Go type. — Affected decisions: —. — Changed in spec: Open Items.
- **F21: D5 "Referenced in spec" field was incomplete.** Raised by: `project-manager (synthesis)`. Category: Cross-reference invariant. D5 is directly cited in the Actors and Triggers (Preconditions) section and in Primary Flow step 3, but the "Referenced in spec" field listed only "Edge Cases and Failure Modes, Coordinations, User Interactions." — Resolution: updated D5's "Referenced in spec" to include "Actors and Triggers" and "Primary Flow." — Affected decisions: D5 (metadata corrected). — Changed in spec: (no change — the gap was in the decision log's metadata only).

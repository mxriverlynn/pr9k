# Feature Specification: Phase 4 — Sidebar Mirroring

Phase 4 of the [pr9k cmux mode build outline](../build-phase-outline.md). While a workflow runs in a pr9k cmux workspace, two live entries appear in cmux's persistent sidebar against that workspace: the current step name as a status entry, and the iteration counter as a progress entry. Both entries update on the same cadence as the header pane, are scoped to the pr9k workspace handle so they remain visible from any other cmux workspace, and are cleared when the run ends.

This phase lands the "monitor a workspace from elsewhere" benefit of cmux mode in its smallest demoable form. Without it, the operator who has switched to a different cmux workspace has no in-cmux signal that pr9k is making progress; they have to come back to the pr9k workspace to look. Phase 4 closes that gap behaviorally; the complementary out-of-workspace interrupt — cmux notifications at completion, failure, and error-mode — ships in Phase 5.

Parent artifacts:

- [Parent feature specification](../feature-specification.md)
- [Parent decision log](../artifacts/decision-log.md)
- [Parent feature technical notes](../artifacts/feature-technical-notes.md)

Builds on [Phase 2 — First Real Workflow Runs End-to-End in Cmux](../phase-2-real-workflow-runs/feature-implementation-plan.md): the running workflow, the orchestrator's authoritative knowledge of the current step and iteration counter, and the workspace handle the orchestrator owns ([parent D-R1](../artifacts/decision-log.md#d-r1-orchestrator-is-the-in-pane-pr9k-process-no-hidden-orchestrator-pane-supersedes-d13d-3d-4-hidden-pane-parts), [parent D-R2](../artifacts/decision-log.md#d-r2-cmux-v2-protocol-is-the-contract-of-record)) all exist; this phase mirrors that state into cmux's sidebar surfaces.

## Outcome

When the workflow is running in a pr9k cmux workspace, an operator looking at any cmux workspace — the pr9k workspace itself, the workspace they were in before launching, or any other — sees two live entries in cmux's sidebar against the pr9k workspace's row: a **status entry** showing the current step name and a **progress entry** showing the iteration counter in `N / M` form ([D1](artifacts/decision-log.md#d1-sidebar-entries-map-to-cmuxs-status-pill-and-progress-bar-surfaces), [parent D5](../artifacts/decision-log.md#d5-mirror-key-state-into-cmux-sidebar)). The two entries update on the same cadence as the header pane — every time the header sees a step transition or an iteration tick, the sidebar entries see the same transition — subject to the small, bounded cross-surface desynchronization Phase 2 already accepts ([parent T4](../artifacts/feature-technical-notes.md#t4-display-processes-update-independently)). The entries are owned by pr9k under a stable, pr9k-prefixed key so they coexist cleanly with status pills set by other tools the operator may also be running in that workspace ([D2](artifacts/decision-log.md#d2-pr9k-owns-its-sidebar-entries-under-a-stable-pr9k-prefixed-key), [T1](artifacts/feature-technical-notes.md#t1-cmux-sidebar-surface-shape)).

When the run ends — successfully, unsuccessfully, or by operator quit — both entries are cleared as part of the orchestrator's normal shutdown sequence so the operator's sidebar is not left with stale pr9k state pointing at a workspace that is no longer running ([D6](artifacts/decision-log.md#d6-sidebar-entries-are-cleared-on-every-run-end-path)).

Phase 4 introduces no new on-disk artifacts and no change to the header, log, or footer panes. The header pane continues to show the same checkbox grid and iteration line it shows in Phase 2; the sidebar entries are a parallel, workspace-scoped projection of that same state, intended for operators whose attention is elsewhere ([parent D17](../artifacts/decision-log.md#d17-log-file-artifacts-are-unchanged-in-cmux-mode)).

## Actors and Triggers

- **Primary actor:** the human operator running pr9k in cmux mode against a target repository, from a shell inside a cmux session — the same actor and trigger Phases 1–3 establish.
- **Trigger:** the workflow has started inside a pr9k cmux workspace (Phase 2's readiness handshake has completed and the first step has begun). The sidebar entries are pushed throughout the run and cleared when the run ends.
- **Preconditions:**
  - All Phase 1 and Phase 2 preconditions hold: cmux is reachable in a mode that admits pr9k, the workspace exists, the three display panes are connected, and the readiness handshake has completed ([parent D16](../artifacts/decision-log.md#d16-per-launch-readiness-handshake-before-workflow-starts)).
  - The orchestrator holds the pr9k workspace's cmux handle so every sidebar update can target that specific workspace ([parent D-R1](../artifacts/decision-log.md#d-r1-orchestrator-is-the-in-pane-pr9k-process-no-hidden-orchestrator-pane-supersedes-d13d-3d-4-hidden-pane-parts), [parent D-R2](../artifacts/decision-log.md#d-r2-cmux-v2-protocol-is-the-contract-of-record)).

## Primary Flow

1. The workflow begins as in Phase 2: the workspace exists, the three display panes are rendering, the readiness handshake has completed, and the orchestrator starts executing steps.
2. As soon as the first step starts running, the orchestrator pushes the **step status entry** to the pr9k workspace's sidebar: a single-line label whose value is the step's display name and whose key is a stable, pr9k-prefixed identifier so the entry can be addressed for later updates and for the run-end cleanup ([D1](artifacts/decision-log.md#d1-sidebar-entries-map-to-cmuxs-status-pill-and-progress-bar-surfaces), [D2](artifacts/decision-log.md#d2-pr9k-owns-its-sidebar-entries-under-a-stable-pr9k-prefixed-key), [T1](artifacts/feature-technical-notes.md#t1-cmux-sidebar-surface-shape)).
3. When the iteration counter is known at launch — the operator passed `-n M` for a finite number of iterations — the orchestrator also pushes the **iteration progress entry** to the sidebar: a progress bar whose fraction is the current iteration divided by the total and whose label is the human-readable `N / M` ([D1](artifacts/decision-log.md#d1-sidebar-entries-map-to-cmuxs-status-pill-and-progress-bar-surfaces), [D3](artifacts/decision-log.md#d3-progress-entry-uses-cmuxs-fraction-plus-label-progress-surface), [T1](artifacts/feature-technical-notes.md#t1-cmux-sidebar-surface-shape)).
4. Whenever the running step changes — including the transition between iterations and the transition from one step to the next within an iteration — the orchestrator pushes a new status entry value with the same key, overwriting the prior value at cmux's sidebar ([D1](artifacts/decision-log.md#d1-sidebar-entries-map-to-cmuxs-status-pill-and-progress-bar-surfaces)). Whenever the iteration counter advances, the orchestrator pushes a new progress entry with the updated fraction and label ([D3](artifacts/decision-log.md#d3-progress-entry-uses-cmuxs-fraction-plus-label-progress-surface)). Updates are pushed on the same cadence as the header pane updates — for every header push there is exactly one corresponding sidebar push for whichever of the two entries changed ([parent D5](../artifacts/decision-log.md#d5-mirror-key-state-into-cmux-sidebar)).
5. The two entries are scoped to the pr9k workspace's cmux handle, so an operator who switches to any other cmux workspace continues to see them in the sidebar's row for the pr9k workspace ([parent D-R2](../artifacts/decision-log.md#d-r2-cmux-v2-protocol-is-the-contract-of-record), [D1](artifacts/decision-log.md#d1-sidebar-entries-map-to-cmuxs-status-pill-and-progress-bar-surfaces)).
6. When the iteration loop ends and the workflow advances into the finalization phase (code review, fix review items, final CI check, update docs, etc.), the orchestrator clears the progress entry from the sidebar — finalization is a single sequence, not an iteration loop, and a half-full progress bar there would be misleading. The status entry continues to update to show the current finalization step's name ([D7](artifacts/decision-log.md#d7-progress-entry-is-cleared-at-the-end-of-the-iteration-loop-status-entry-continues-through-finalization)).
7. When the run ends — by successful completion, by an unrecoverable abort, by operator-initiated quit, or by any of the failure modes Phase 6 will harden — the orchestrator clears both the status entry and (if it was set) the progress entry as part of its shutdown sequence, so the operator's sidebar does not show stale pr9k state against a workspace that is no longer running ([D6](artifacts/decision-log.md#d6-sidebar-entries-are-cleared-on-every-run-end-path)).
8. The on-disk log artifacts that Phase 2 writes — the per-run log file and per-step artifacts under the target repository's standard log directory — are unchanged by Phase 4; sidebar mirroring does not produce any new artifact ([parent D17](../artifacts/decision-log.md#d17-log-file-artifacts-are-unchanged-in-cmux-mode)).

## Alternate Flows and States

### The iteration count is unknown at launch (no `-n`)

- **Entry condition:** the operator launches pr9k without specifying an iteration cap — the workflow runs until its own break condition fires (for example, the default workflow exits the loop when no more issues labeled "ralph" remain).
- **Sequence:** the orchestrator pushes the step status entry as in the primary flow, but it does **not** push a progress entry — cmux's progress surface is a 0.0–1.0 fraction, and there is no honest fraction to push when the total iteration count is not known in advance ([D4](artifacts/decision-log.md#d4-unbounded-iteration-count-suppresses-the-progress-entry-the-status-entry-continues-unchanged), [T1](artifacts/feature-technical-notes.md#t1-cmux-sidebar-surface-shape)). The step status entry updates on every step transition exactly as in the primary flow.
- **Exit:** the run ends as usual; only the status entry needs to be cleared ([D6](artifacts/decision-log.md#d6-sidebar-entries-are-cleared-on-every-run-end-path)).

### Operator switches to a different cmux workspace

- **Entry condition:** the workflow is running; the operator focuses any cmux workspace other than the pr9k workspace.
- **Sequence:** cmux continues to show the pr9k workspace in its sidebar listing with the two pr9k-owned entries against that row. The entries update on the same cadence as the header pane regardless of which workspace has focus — the orchestrator's pushes are workspace-targeted, not focus-dependent ([parent D-R2](../artifacts/decision-log.md#d-r2-cmux-v2-protocol-is-the-contract-of-record)).
- **Exit:** when the operator returns focus to the pr9k workspace, the in-workspace header pane shows the same step and iteration information that was already visible in the sidebar; there is no fast-forward or replay needed because the sidebar values are a parallel projection of the header's authoritative state ([D1](artifacts/decision-log.md#d1-sidebar-entries-map-to-cmuxs-status-pill-and-progress-bar-surfaces)).

### Iteration loop ends and finalization begins

- **Entry condition:** the last iteration of the iteration loop has completed; the orchestrator advances into the finalization phase.
- **Sequence:** the orchestrator clears the progress entry from the sidebar. The status entry continues to be updated as the finalization phase advances — each finalization step's name shows in the pill in turn ([D7](artifacts/decision-log.md#d7-progress-entry-is-cleared-at-the-end-of-the-iteration-loop-status-entry-continues-through-finalization)). The status entry continues to track the running step until the run ends.
- **Exit:** the run ends and both entries (status, and progress if it had been re-pushed) are cleared per [D6](artifacts/decision-log.md#d6-sidebar-entries-are-cleared-on-every-run-end-path).

### The run ends in error mode without operator action

- **Entry condition:** a step has failed and the run is paused at pr9k's continue / retry / quit error prompt ([Phase 3](../phase-3-interactive-error-recovery/feature-specification.md)) without the operator having answered.
- **Sequence:** Phase 4 does not change the sidebar entries while the run is paused — the status entry still shows the failing step's name and the progress entry still shows the iteration the failure occurred in. In Phase 4 there is no failure-specific sidebar signal (no error pill, no failed-state color) — that decoration belongs to a future phase if evidence later supports it. The directing notification that closes the attention gap for an unattended error prompt ships in Phase 5 ([parent D6](../artifacts/decision-log.md#d6-fire-cmux-notifications-at-named-lifecycle-moments)).
- **Exit:** when the operator answers the error prompt, the run resumes (or aborts) and the sidebar entries update or clear according to the primary flow.

## Edge Cases and Failure Modes

| Condition | Required Behavior |
|-----------|-------------------|
| The operator launches pr9k with an iteration cap (`-n M`). | Both entries are pushed; the progress entry's fraction advances from `1/M` through `M/M` as iterations complete, and the label tracks `N / M` ([D3](artifacts/decision-log.md#d3-progress-entry-uses-cmuxs-fraction-plus-label-progress-surface)). |
| The operator launches pr9k without an iteration cap. | Only the status entry is pushed; the progress entry is suppressed entirely ([D4](artifacts/decision-log.md#d4-unbounded-iteration-count-suppresses-the-progress-entry-the-status-entry-continues-unchanged)). |
| The iteration loop exits early (the configured break condition fires before reaching the cap). | The progress entry's last visible value reflects the last completed iteration. When the iteration loop ends and finalization begins, the progress entry is cleared per [D7](artifacts/decision-log.md#d7-progress-entry-is-cleared-at-the-end-of-the-iteration-loop-status-entry-continues-through-finalization). |
| The orchestrator pushes status updates faster than cmux renders them. | cmux's sidebar surfaces are "latest-wins per key" — pushing a new value for the same status key overwrites the previous value at cmux's side; pushing a new progress value overwrites the previous progress for the same workspace ([T1](artifacts/feature-technical-notes.md#t1-cmux-sidebar-surface-shape)). No queueing, throttling, or coalescing logic is required in pr9k. |
| A status pill set by a different tool (e.g., `claude_code`, `build`) is already present on the pr9k workspace. | pr9k's entries are addressed by a stable, pr9k-prefixed key that does not collide with any well-known tool's key. pr9k does not list, inspect, or modify other tools' pills. pr9k's entry and the other tool's entries coexist side-by-side ([D2](artifacts/decision-log.md#d2-pr9k-owns-its-sidebar-entries-under-a-stable-pr9k-prefixed-key), [T1](artifacts/feature-technical-notes.md#t1-cmux-sidebar-surface-shape)). |
| A sidebar update call returns a non-timeout error (e.g., the workspace handle was rejected, the value was malformed, the call temporarily failed). | The error is logged to the per-run log file under `<projectDir>/.pr9k/logs/<stamp>/` and the workflow continues unaffected. Sidebar mirroring is a best-effort projection of state that already exists authoritatively in the header pane and the on-disk log artifacts ([D5](artifacts/decision-log.md#d5-non-timeout-sidebar-errors-are-logged-and-the-run-continues-timeouts-remain-fatal-per-parent-d15)). |
| A sidebar update call times out (cmux's socket accepted the request but did not return a response within the configured per-call timeout). | The timeout is treated as fatal, identical to any other cmux call timeout, and triggers the same run-abort path as parent [D15](../artifacts/decision-log.md#d15-cmux-api-per-call-timeout-is-fatal) — sidebar calls do not get a special exemption ([D5](artifacts/decision-log.md#d5-non-timeout-sidebar-errors-are-logged-and-the-run-continues-timeouts-remain-fatal-per-parent-d15)). |
| The operator's prior workspace was torn down while the run was still going. | Phase 4 makes no change here; this is the failure-mode territory of Phase 6. Phase 4's sidebar-clear calls at run-end may receive a non-timeout error if the workspace handle is already gone; that error is logged and ignored per [D5](artifacts/decision-log.md#d5-non-timeout-sidebar-errors-are-logged-and-the-run-continues-timeouts-remain-fatal-per-parent-d15). |
| The orchestrator is killed without a chance to run its shutdown sequence (forced kill, host shutdown). | The sidebar entries are not cleared by pr9k and may remain visible until the operator dismisses the orphaned workspace. The operator's startup advisory in [Phase 7](../build-phase-outline.md#phase-7) is the broader handling for orphaned pr9k workspaces; Phase 4 does not attempt to "self-clean" sidebar entries pre-emptively. |
| A pane is lost mid-run, the interaction channel stalls, or any other Phase-2-or-Phase-6 failure mode fires. | Out of scope for Phase 4. The orchestrator continues to own the sidebar entries through its shutdown sequence; Phase 6's failure-handling work integrates the sidebar-clear into the abort path so the clean-abort surface stays consistent. |

## User Interactions

- **Affordances:**
  - **cmux sidebar (visible workspace-wide):** the pr9k workspace's row in cmux's sidebar shows a status entry (the current step name) and, when the iteration count is known, a progress entry (`N / M` form). Both are managed by cmux's existing chrome — pr9k does not draw the sidebar itself; it only pushes values to cmux's surfaces ([D1](artifacts/decision-log.md#d1-sidebar-entries-map-to-cmuxs-status-pill-and-progress-bar-surfaces), [T1](artifacts/feature-technical-notes.md#t1-cmux-sidebar-surface-shape)).
  - **Header pane (visible inside the pr9k workspace):** continues to show the same checkbox grid and iteration line Phase 2 established; the sidebar entries are a parallel projection, not a replacement.
  - **Other tools' status pills:** if the operator is running other tools that also push status pills, those pills appear in the same workspace row, side-by-side with pr9k's. pr9k owns only its prefixed key ([D2](artifacts/decision-log.md#d2-pr9k-owns-its-sidebar-entries-under-a-stable-pr9k-prefixed-key)).
- **Feedback:**
  - On a step transition: the header pane checkbox advances and the sidebar status entry text updates, near-simultaneously and subject to the small bounded desync Phase 2 accepts ([parent T4](../artifacts/feature-technical-notes.md#t4-display-processes-update-independently)).
  - On an iteration transition (when `-n M` is known): the header iteration line advances and the sidebar progress entry advances by `1/M`, near-simultaneously and subject to the same bounded desync.
  - On run end: the sidebar entries clear and the workspace's row no longer shows pr9k state. The workspace itself remains open until the operator dismisses it per Phase 2 ([parent D14](../artifacts/decision-log.md#d14-workspace-closure-is-operator-initiated)).
- **Error states:**
  - A failing sidebar update (non-timeout): no operator-visible change; the failure is recorded in the per-run log file only ([D5](artifacts/decision-log.md#d5-non-timeout-sidebar-errors-are-logged-and-the-run-continues-timeouts-remain-fatal-per-parent-d15)).
  - A sidebar update timeout: the run aborts with the same abort path display-loss takes; the abort is itself visible through the standard pr9k abort surfaces (parent D15, Phase 6 surfaces).

## Coordinations

| Counterpart | Direction | Interactions | Constraints |
|-------------|-----------|--------------|-------------|
| cmux (the multiplexer's sidebar surfaces) | outbound | Push a workspace-scoped status entry under a stable pr9k-prefixed key; push a workspace-scoped progress entry with a 0.0–1.0 fraction and a `N / M` label; clear both entries on run end. | Each call is sequential per orchestrator. The per-call wall-clock timeout from parent [D15](../artifacts/decision-log.md#d15-cmux-api-per-call-timeout-is-fatal) applies unchanged — a timeout is fatal. Other (non-timeout) errors are logged and the run continues ([D5](artifacts/decision-log.md#d5-non-timeout-sidebar-errors-are-logged-and-the-run-continues-timeouts-remain-fatal-per-parent-d15)). cmux's sidebar surfaces are latest-wins per key, so pr9k does not coalesce or throttle updates ([T1](artifacts/feature-technical-notes.md#t1-cmux-sidebar-surface-shape)). |
| Header pane (inside the pr9k workspace) | inbound (parallel) | The same orchestrator state that drives the header pane drives the sidebar entries. Phase 4 introduces no new dependency from the header pane on the sidebar or vice versa. | Bounded cross-surface desynchronization, identical in shape to Phase 2's per-pane independence ([parent T4](../artifacts/feature-technical-notes.md#t4-display-processes-update-independently)). |
| On-disk log artifacts | unchanged | Phase 4 introduces no new artifact and writes nothing to disk it would not have written in Phase 2 ([parent D17](../artifacts/decision-log.md#d17-log-file-artifacts-are-unchanged-in-cmux-mode)). | — |

## Out of Scope

- **A sidebar log entry stream.** cmux's sidebar log surface (a per-workspace append-log) is not used by Phase 4. The log pane inside the pr9k workspace is the authoritative log surface; mirroring the log into the sidebar would duplicate the surface without an evidence-supported operator need. This stays out of scope until the parent spec's deferral reopens it ([parent Deferred (YAGNI)](../feature-specification.md#deferred-yagni)).
- **A failure-specific sidebar signal.** Phase 4 does not add a "failure" color, icon, or alternate label to the status entry when a step fails. The directing surface for unattended failures is the cmux notification that ships in Phase 5; the sidebar's role in Phase 4 is passive progress-mirroring only.
- **Operator-configurable sidebar styling.** Phase 4 does not expose icon, color, or priority overrides to the operator. cmux's sidebar status-entry surface accepts those parameters; pr9k does not surface them in this release because no operator-described need supports it.
- **Sidebar mirroring of the full status line.** Already deferred at the parent spec level ([parent Deferred (YAGNI)](../feature-specification.md#deferred-yagni)) — the status-line script continues to run in the footer pane unchanged.
- **Mirroring sidebar state into the on-disk log artifacts.** The artifacts remain unchanged; sidebar state is ephemeral by design.

## Deferred (YAGNI)

### A "failure" decoration on the status entry when a step fails

- **Why deferred:** the spec's evidence test prefers Phase 5's notification surface — which already directs the operator to the footer pane — as the failure-attention signal. Adding a failure pill on top would be a second, redundant signal supported by no operator-described need.
- **Reopen when:** operators report that they regularly miss the failure-mode notification and want a second, sidebar-resident, persistent visual signal that survives notification dismissal.

### Status-pill priority, icon, or color overrides

- **Why deferred:** cmux's sidebar status surface accepts these parameters, but pr9k has no operator-described need for them and no design rationale for one default over another. Setting them now is symmetry/completeness, which fails the YAGNI evidence test.
- **Reopen when:** operators report that pr9k's pill is sort-ordered behind other tools' pills in a way that makes it hard to find, or that the default styling is not visually distinguishable from another tool's pill.

### Mirroring the iteration progress into Phase 5's notifications

- **Why deferred:** Phase 5's notification surface is for interruptive signals (completion, failure, error-mode prompt awaiting input), not for ongoing progress mirroring. Pushing iteration progress to the notification surface would produce notification fatigue and conflicts with the Phase 5 scope.
- **Reopen when:** operators report that they want a non-sidebar progress signal — e.g., they run cmux without the sidebar visible — and would tolerate the notification noise as a trade-off.

## Open Items

None — every Phase 4 decision is settled by evidence (cmux v0.64.7's sidebar surface, parent decisions D5 / D-R1 / D-R2 / D15 / D17, and the existing `cmuxHeader` adapter) or by user judgment captured in [the decision log](artifacts/decision-log.md). The Phase 4 preconditions in the build outline that mentioned OQ-5 (the sidebar method) are resolved by [D1](artifacts/decision-log.md#d1-sidebar-entries-map-to-cmuxs-status-pill-and-progress-bar-surfaces) and [T1](artifacts/feature-technical-notes.md#t1-cmux-sidebar-surface-shape).

## Summary

- **Outcome delivered:** a workflow running in a pr9k cmux workspace now mirrors its current step name and iteration progress into cmux's sidebar against the pr9k workspace's row, visible from any other cmux workspace, on the same cadence as the header pane, and cleared cleanly on every run-end path.
- **Actor:** the human operator running pr9k in cmux mode.
- **Decisions settled:** 7 (1–7 in [the decision log](artifacts/decision-log.md), of which the OQ-5 mapping is settled by D1 + T1).
- **Technical notes captured:** 1 ([T1](artifacts/feature-technical-notes.md)).
- **Reviewers consulted:** (filled in after Step 6).
- **Key adjustments:** (filled in after Step 7).

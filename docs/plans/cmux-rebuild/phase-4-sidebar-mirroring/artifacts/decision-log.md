# Decision Log: Phase 4 — Sidebar Mirroring

Decisions specific to [Phase 4 — Sidebar Mirroring](../feature-specification.md). Decisions established by the [parent feature specification](../../feature-specification.md) and [parent decision log](../../artifacts/decision-log.md) — including [D5](../../artifacts/decision-log.md#d5-mirror-key-state-into-cmux-sidebar) (mirror step + iteration only), [D15](../../artifacts/decision-log.md#d15-cmux-api-per-call-timeout-is-fatal), [D17](../../artifacts/decision-log.md#d17-log-file-artifacts-are-unchanged-in-cmux-mode), [D-R1](../../artifacts/decision-log.md#d-r1-orchestrator-is-the-in-pane-pr9k-process-no-hidden-orchestrator-pane-supersedes-d13d-3d-4-hidden-pane-parts), and [D-R2](../../artifacts/decision-log.md#d-r2-cmux-v2-protocol-is-the-contract-of-record) — apply here unchanged and are not re-litigated.

## Full decisions

### D1: Sidebar entries map to cmux's status-pill and progress-bar surfaces

- **Question:** [OQ-5 in the build outline](../../build-phase-outline.md#oq-5) — which cmux surfaces realize the two sidebar entries the parent spec commits to?
- **Decision:** the **step name** entry is pushed to cmux's sidebar status-entry surface (CLI verb `set-status` / `clear-status` in cmux v0.64.7, the corresponding RPC under the hood). The **iteration counter** entry is pushed to cmux's sidebar progress surface (CLI verb `set-progress` / `clear-progress`). Both calls target the pr9k workspace by its workspace handle ([parent D-R2](../../artifacts/decision-log.md#d-r2-cmux-v2-protocol-is-the-contract-of-record)). The caller is the in-pane orchestrator process ([parent D-R1](../../artifacts/decision-log.md#d-r1-orchestrator-is-the-in-pane-pr9k-process-no-hidden-orchestrator-pane-supersedes-d13d-3d-4-hidden-pane-parts)) — there is no hidden orchestrator pane.
- **Rationale:** Rework R's standard was to verify every cmux integration claim against the pinned cmux source rather than against an assumed surface. cmux v0.64.7 actually exposes two distinct sidebar surfaces — status entries (pills, keyed) and a single progress bar (fraction + label) per workspace. The parent spec's two outcomes (step name as a status entry, iteration counter as a progress entry) map one-to-one onto these surfaces. Verified against `cmux --help`, `cmux set-status --help`, `cmux set-progress --help`, and [docs/cli-contract.md](https://raw.githubusercontent.com/manaflow-ai/cmux/main/docs/cli-contract.md) for cmux 0.64.7.
- **Evidence:**
  - User confirmation on 2026-05-19 that cmux 0.64.7 is the pinned version.
  - `cmux set-status --help` and `cmux set-progress --help` output on cmux 0.64.7 (reproducible against the installed cmux binary).
  - The cmux CLI contract reference at <https://raw.githubusercontent.com/manaflow-ai/cmux/main/docs/cli-contract.md>, lines documenting `set-status`, `clear-status`, `list-status`, `set-progress`, `clear-progress`.
  - [Parent T1](../../artifacts/feature-technical-notes.md#t1-cmux-programmatic-interface-shape) listing "sidebar status / progress / log" as one of cmux's six API categories.
  - The orchestrator already holds an active cmux v2 client used for workspace lifecycle and other cmux calls; the same client handles the sidebar surface — no new client infrastructure is required ([parent D-R1](../../artifacts/decision-log.md#d-r1-orchestrator-is-the-in-pane-pr9k-process-no-hidden-orchestrator-pane-supersedes-d13d-3d-4-hidden-pane-parts), [parent D-R2](../../artifacts/decision-log.md#d-r2-cmux-v2-protocol-is-the-contract-of-record)).
- **Rejected alternatives:**
  - **`workspace.rename` + free-form text in the workspace title** — rejected because pr9k's workspace title is also the input to Phase 7's orphan-detection filter (the `pr9k-` prefix), and rewriting the title on every step transition would either collide with that filter or require an extra parsing pass.
  - **`feed.push` / cmux's per-workspace feed surface** — the feed surface is a per-pane append-only log, not the sidebar; using it for status mirroring would put the values in the wrong cmux chrome.
  - **`notification.create*` per step change** — rejected because notifications are the wrong concept for a passive monitoring surface (notification fatigue) and they are Phase 5's surface.
  - **"There is no dedicated sidebar API; defer Phase 4"** — rejected because cmux 0.64.7 does expose a dedicated sidebar surface with exactly the right shape.
- **Linked technical notes:** T1
- **Driven by findings:** —
- **Dependent decisions:** D2, D3, D6, D7
- **Referenced in spec:** Outcome, Primary Flow, Alternate Flows and States, User Interactions, Coordinations

### D2: pr9k owns its sidebar entries under a stable, pr9k-prefixed key

- **Question:** cmux's sidebar status surface keys each pill by a string supplied by the caller; multiple tools can set pills on the same workspace. What key does pr9k use, and how does it ensure it does not collide with other tools' pills?
- **Decision:** pr9k uses a single stable key for its step status entry, prefixed `pr9k.`. The exact key string is an implementation choice the implementation plan pins; the spec-level commitment is "stable and pr9k-prefixed", which is the operator-visible invariant. pr9k does not list, inspect, or mutate any pill whose key is not `pr9k.*`. The progress bar is one-per-workspace by cmux's contract, so it does not need a key.
- **Rationale:** cmux's documented examples show other tools using keys like `claude_code`, `codex`, `build`, `deploy`. Using a pr9k-prefixed key isolates pr9k's pill from those tools' pills on the operator's screen — they coexist as separate pills in the workspace row, and pr9k can address its own pill for updates and cleanup without affecting anyone else's.
- **Evidence:**
  - `cmux set-status --help` example: `cmux set-status build "compiling" --icon hammer --color "#ff9500" --priority 80` — confirms the keyed model.
  - cmux binary strings show `set_status claude_code` and `set_status codex` as references — confirms other tools use top-level keys, so a prefix isolates us.
- **Rejected alternatives:**
  - **A non-prefixed key like `step`** — rejected because the key is operator-visible (it scopes the pill identity for other CLI calls) and a non-prefixed key would invite collision with other tools.
  - **Reading other tools' pills before pushing and avoiding collisions dynamically** — rejected as overkill; the static prefix gives the same isolation with no runtime cost.
- **Linked technical notes:** T1
- **Driven by findings:** —
- **Dependent decisions:** D6
- **Referenced in spec:** Outcome, Primary Flow, Edge Cases and Failure Modes, User Interactions

### D3: Progress entry uses cmux's fraction-plus-label progress surface

- **Question:** cmux's sidebar progress surface takes a 0.0–1.0 fraction and an optional label. How does pr9k populate them from its iteration counter?
- **Decision:** the **fraction** is `N / M` (current iteration over total). The **label** is the human-readable form `"N / M"`. Both are recomputed and pushed every time the iteration counter advances.
- **Rationale:** the parent spec commits to the iteration counter being "a progress entry with a numeric `N / M` form" (parent D5). cmux's progress surface is the right shape for that — the fraction drives the bar's fill and the label gives the operator the exact integer pair. No further translation is needed.
- **Evidence:**
  - `cmux set-progress --help`: `Usage: cmux set-progress <0.0-1.0> [flags]` with `--label <text>`.
  - [Parent D5](../../artifacts/decision-log.md#d5-mirror-key-state-into-cmux-sidebar) commits to the `N / M` form.
- **Rejected alternatives:**
  - **Push only the label, fraction always 0.0** — visually wrong; the operator's progress bar would always appear empty.
  - **Push only the fraction, no label** — loses the exact integer pair; the operator can't read `N / M` directly.
- **Linked technical notes:** T1
- **Driven by findings:** —
- **Dependent decisions:** D4, D7
- **Referenced in spec:** Outcome, Primary Flow, Edge Cases and Failure Modes

### D4: Unbounded iteration count suppresses the progress entry; the status entry continues unchanged

- **Question:** what does the progress entry show when the operator launches pr9k without an iteration cap (no `-n`, so the workflow runs until a break condition fires)?
- **Decision:** the progress entry is **not pushed at all**. The step status entry is pushed exactly as in the bounded case. When the run ends, the cleanup path for the progress entry is a no-op (there is nothing to clear).
- **Rationale:** cmux's progress surface is a 0.0–1.0 fraction. With no known total, there is no honest fraction to publish. The dishonest alternatives (always 0.0, or self-normalizing) would mislead the operator more than the absence does. The step pill alone still gives the operator the "current step" signal that is the core value of sidebar mirroring.
- **Evidence:**
  - The default workflow's break-condition pattern (`breakLoopIfEmpty`) lets the loop end before reaching any specified iteration cap, and the Ralph use case routinely runs without `-n` (the operator wants to process every issue labeled `ralph`).
  - cmux's progress surface is by-design fractional per `cmux set-progress --help`.
- **Rejected alternatives:**
  - **Push fraction 0.0 with a label like `Iteration N`** — rejected because the progress bar visually says "0% done" which is misleading; the absence of a bar is more honest.
  - **Self-normalizing bar (e.g., `iteration / max(iteration, 1)`)** — rejected because the bar would always look full and would not advance meaningfully.
- **Linked technical notes:** T1
- **Driven by findings:** —
- **Dependent decisions:** D6
- **Referenced in spec:** Alternate Flows and States, Edge Cases and Failure Modes

### D5: Non-timeout sidebar errors are logged and the run continues; timeouts remain fatal per parent D15

- **Question:** how does pr9k handle a sidebar update call that fails for a non-timeout reason (workspace handle rejected, value malformed, transient cmux error)?
- **Decision:** non-timeout sidebar errors are recorded in the per-run log file (under `<projectDir>/.pr9k/logs/<stamp>/`) and the workflow continues. Timeouts remain fatal under [parent D15](../../artifacts/decision-log.md#d15-cmux-api-per-call-timeout-is-fatal) — sidebar calls do not get a special exemption from the parent's timeout policy.
- **Rationale:** the sidebar is a passive projection of state that already exists authoritatively in the header pane (which the operator can return to) and on disk (which never depends on the sidebar). A non-timeout failure to push a sidebar value is an annoyance, not a correctness problem — aborting the workflow over it would trade significant operator value for a non-load-bearing surface. Timeouts are a different signal: they indicate cmux itself is unhealthy, which is exactly the case parent D15 designed for.
- **Evidence:**
  - [Parent D15](../../artifacts/decision-log.md#d15-cmux-api-per-call-timeout-is-fatal): "Every cmux API call has a configured per-call wall-clock timeout. A timeout is treated as fatal."
  - [Parent D17](../../artifacts/decision-log.md#d17-log-file-artifacts-are-unchanged-in-cmux-mode): the on-disk log artifacts are the authoritative record; sidebar state is a projection.
- **Rejected alternatives:**
  - **Treat all sidebar errors (including non-timeouts) as fatal** — symmetrical with D15 but trades the wrong way: a malformed value or a stale workspace handle does not warrant aborting a long-running pr9k workflow.
  - **Suppress logging for non-timeout sidebar errors** — rejected because the operator needs a record when the sidebar goes silent so they can debug why.
- **Linked technical notes:** —
- **Driven by findings:** —
- **Dependent decisions:** D6
- **Referenced in spec:** Edge Cases and Failure Modes, Coordinations, User Interactions

### D6: Sidebar entries are cleared on every graceful run-end path

- **Question:** when does pr9k clear the sidebar entries, and what about paths that bypass the shutdown sequence?
- **Decision:** the orchestrator clears both entries (the pr9k-prefixed status entry and the progress bar, when it was pushed) as part of its shutdown sequence on every **graceful** run-end path — successful completion, unrecoverable abort that still reaches shutdown, and operator-initiated quit. The clear calls are best-effort under [D5](#d5-non-timeout-sidebar-errors-are-logged-and-the-run-continues-timeouts-remain-fatal-per-parent-d15): a non-timeout error is logged but does not change the orchestrator's exit code. Cleanup on non-graceful paths — a display-pane loss, channel stall, orchestrator crash, host shutdown, forced kill — is out of Phase 4's scope; Phase 6's failure-handling work integrates the same sidebar-clear operations into the abort path ([D17](#d17-phase-6-integrates-with-phase-4-by-calling-the-same-sidebar-clear-operations-no-new-interface-is-required)).
- **Rationale:** without a clear on run-end, the operator's sidebar would show stale pr9k state pointing at a workspace that is no longer doing anything — actively misleading. Clearing on every graceful exit path (rather than only success) is the simplest invariant for the operator and the implementer. Scoping the commitment explicitly to graceful paths avoids the false-completeness implied by "every run-end path" — non-graceful paths exist and Phase 4 acknowledges them as Phase 6's territory.
- **Evidence:**
  - Parent [D5](../../artifacts/decision-log.md#d5-mirror-key-state-into-cmux-sidebar) commits to "sidebar entries clear" at run end as part of the workspace transition to done.
  - Parent [D14](../../artifacts/decision-log.md#d14-workspace-closure-is-operator-initiated) commits to the workspace persisting after the run ends — meaning the workspace's sidebar row remains visible, so its contents matter even after pr9k exits.
- **Rejected alternatives:**
  - **Clear only on success; leave entries visible after a failure so the operator can see what was running** — rejected because the on-disk log and the header pane (still visible inside the persisted workspace) already give that record; the sidebar's last value is a poor place to leave diagnostic state.
  - **Have cmux auto-clear when the workspace is dismissed** — cmux does clear workspace-scoped state when the workspace is destroyed, but pr9k's process exits before the operator dismisses the workspace, and there is no reason to leave the sidebar stale during the window where the operator is reviewing the persisted workspace.
- **Linked technical notes:** T1
- **Driven by findings:** —
- **Dependent decisions:** —
- **Referenced in spec:** Outcome, Primary Flow, Alternate Flows and States, Edge Cases and Failure Modes

### D7: Progress entry is cleared at the end of the iteration loop; the status entry continues through finalization

- **Question:** the pr9k workflow has two phases — the iteration loop (one iteration per issue/work item) and the finalization phase (code review, fix review items, etc., which runs once over the full set of changes). The progress bar's `N / M` model fits the iteration loop. What does the progress bar show during finalization?
- **Decision:** the progress entry is cleared when the iteration loop ends. The step status entry continues to update through the finalization phase, showing each finalization step's name in turn. When the run ends, the status entry is cleared along with the (already-cleared) progress entry per [D6](#d6-sidebar-entries-are-cleared-on-every-graceful-run-end-path).
- **Rationale:** finalization is a single sequence, not an iteration loop. There is no honest `N / M` to publish during finalization. Pushing `1.0` would falsely claim the run is done; keeping the last iteration value (`M / M`) would be stale; recomputing as a finalization-step fraction would conflate two different concepts (iteration progress and step progress). Clearing the bar acknowledges that the iteration-progress signal has run its course, while the step pill — which is already step-level, not iteration-level — keeps working unchanged.
- **Evidence:**
  - The default workflow's finalization phase is a single-pass sequence (per `workflow/config.json` — code review → check verdict → fix review items → final CI check → update docs → deferred work → lessons learned → git push) with no iteration counter at that layer.
  - [Parent D5](../../artifacts/decision-log.md#d5-mirror-key-state-into-cmux-sidebar) names the iteration counter specifically, not a generic "any-phase progress indicator".
- **Rejected alternatives:**
  - **Show the progress bar at 1.0 during finalization** — rejected; misleading because the run is not over.
  - **Keep the last iteration progress value visible during finalization** — rejected; stale, and the operator's natural reading is "still on iteration M / M" when in fact the iteration loop has ended.
  - **Reuse the progress bar to show finalization-step progress (`finalization step k / total finalization steps`)** — rejected; conflates iteration and step granularity in a single visual surface and parent D5 specifically names the iteration counter as the source.
- **Linked technical notes:** T1
- **Driven by findings:** —
- **Dependent decisions:** D6
- **Referenced in spec:** Outcome, Primary Flow, Alternate Flows and States, Edge Cases and Failure Modes

### D13: Error mode suffixes a stable marker onto the status entry value

- **Question:** when the orchestrator enters error mode (a step failure paused waiting for the operator's continue / retry / quit decision), how does an operator monitoring from another cmux workspace know the run is paused — given that Phase 5's cmux notification can be dismissed by the operator?
- **Decision:** while the orchestrator is in error mode, the status pill's value is the failing step's name combined with a stable error-mode marker (e.g., the value transitions from `Feature work` to `Feature work — awaiting input`). The marker is appended to the value of the existing pill under the same pr9k-prefixed key — there is no separate pill, no icon override, no color override. When the operator resolves the prompt (continue, retry, quit), the marker is dropped on the next normal status push (continue / successful retry) or the run aborts gracefully and the pill clears per [D6](#d6-sidebar-entries-are-cleared-on-every-graceful-run-end-path) (quit).
- **Rationale:** Phase 5's notification surface fires once when error mode is entered and re-fires on a cadence until the operator answers ([parent D6](../../artifacts/decision-log.md#d6-fire-cmux-notifications-at-named-lifecycle-moments), [parent D19](../../artifacts/decision-log.md#d19-error-mode-notifications-direct-the-operator-to-the-control-pane)). But a notification the operator dismisses (intending to "I'll come back to it later") gives no glanceable signal that the run is still paused — the sidebar shows the same step name it did before the failure. Without an in-sidebar signal, the operator monitoring from another workspace could see the step name "Feature work" in the pill for many minutes and not know whether pr9k is still working or has been paused waiting for them. A stable suffix on the existing pill closes that gap with no new cmux surface and no icon/color override, staying within the simpler-version test.
- **Evidence:**
  - Phase 3 spec ([../phase-3-interactive-error-recovery/feature-specification.md](../../phase-3-interactive-error-recovery/feature-specification.md), Alternate Flows "Operator's focus is away from the footer when the step fails"): "The orchestrator blocks indefinitely — there is no auto-timeout. In Phase 3 there is no notification and no sidebar failure label, so the only indication that the run needs attention is the footer pane's rendered error prompt, which the operator may not currently be looking at."
  - Phase 5 (build-phase-outline.md §Phase 5) commits to a re-firing notification, but notifications can be dismissed and remain dismissed.
  - Review-team finding [F14](team-findings.md#f14-error-mode-invisible-on-monitoring-surface) (UX): "An operator who dismissed the Phase 5 notification once and then looks at the sidebar for a secondary confirmation finds no difference from a healthy run."
- **Rejected alternatives:**
  - **Push a separate error-state pill under a second pr9k-prefixed key** — rejected because two pills doubles the visual surface area for one piece of information, and the operator would need to read both to assemble the run state. The single-pill-with-suffix form is denser and uses an existing entry.
  - **Override the pill's icon or color to a failure styling** — rejected because [D12](#trivial-decisions) (pr9k does not override optional pill parameters in this release) covers this and the suffix is sufficient without crossing into icon/color territory.
  - **Defer all error-mode signaling to Phase 5's notification surface** — rejected because Phase 5's signal can be dismissed; the sidebar signal is the persistent complement.
- **Linked technical notes:** T1
- **Driven by findings:** F14 (UX-F1)
- **Dependent decisions:** D6
- **Referenced in spec:** Outcome, Primary Flow, Alternate Flows and States, User Interactions

## Trivial decisions

- **D8:** Sidebar updates target the pr9k workspace by its cmux handle (workspace_id / workspace_ref). — Follows directly from [parent D-R2](../../artifacts/decision-log.md#d-r2-cmux-v2-protocol-is-the-contract-of-record). — Referenced in spec: Primary Flow, Coordinations.
- **D9:** The orchestrator (the in-pane pr9k process) is the caller of sidebar update RPCs. — Follows directly from [parent D-R1](../../artifacts/decision-log.md#d-r1-orchestrator-is-the-in-pane-pr9k-process-no-hidden-orchestrator-pane-supersedes-d13d-3d-4-hidden-pane-parts) (there is no hidden pane to delegate to). — Referenced in spec: Primary Flow, Coordinations.
- **D10:** Sidebar pushes happen exactly when the running step changes or the iteration counter advances — one sidebar push per such event for whichever of the two entries changed. Intra-step log output and cosmetic header re-renders do not trigger sidebar pushes. — Refines parent D5 ("update on the same cadence as the header pane") to a specific event set after F1. — Referenced in spec: Outcome, Primary Flow.
- **D11:** Phase 4 introduces no new on-disk artifacts. — Follows directly from [parent D17](../../artifacts/decision-log.md#d17-log-file-artifacts-are-unchanged-in-cmux-mode). — Referenced in spec: Outcome, Coordinations.
- **D12:** pr9k does not override the optional sidebar status-entry parameters (icon, color, priority). — Follows YAGNI: no operator-described need, and the default styling is honest. — Referenced in spec: Out of Scope.
- **D14:** Every workflow step transition pushes the status pill — including initialize-phase, iteration-phase, and finalization-phase steps. — Follows from D1 (cadence event = step-name change); the workflow's three phases all produce step transitions and the spec applies the same rule to all of them. Resolves [F13](team-findings.md#f13-initialize-phase-steps-not-mentioned). — Referenced in spec: Primary Flow.
- **D15:** The Phase 1 capability check is extended to cover Phase 4's sidebar methods. — Follows from [parent D18](../../artifacts/decision-log.md#d18-startup-capability-check) (the capability check covers every method pr9k calls). Without this, a missing method would be swallowed by D5's "log and continue" path, producing a silent operator-visible failure ("the sidebar just doesn't update"). Resolves [F12](team-findings.md#f12-capability-check-for-new-sidebar-methods-not-mentioned-in-preconditions). — Referenced in spec: Actors and Triggers (Preconditions), User Interactions (Error states).
- **D16:** No pr9k sidebar entries exist between launch and the first step transition. — Follows from D1 (the cadence event is a step transition; the first transition is the first push). This is an expected bounded gap, not a missing-state bug. Resolves [F18](team-findings.md#f18-no-defined-starting-state-for-the-sidebar-between-launch-and-first-step). — Referenced in spec: Primary Flow.
- **D17:** Phase 6 integrates with Phase 4 by calling the same sidebar-clear operations; no new interface is required. — Follows from the existing orchestrator owning the sidebar-clear operations; Phase 6's abort path adds a call to the same operations. This is named explicitly so the Phase 6 implementer does not look for a separate adapter. Resolves [F6](team-findings.md#f6-phase-6-integration-point-is-assumed-not-specified). — Referenced in spec: Primary Flow (step 8), Edge Cases and Failure Modes.

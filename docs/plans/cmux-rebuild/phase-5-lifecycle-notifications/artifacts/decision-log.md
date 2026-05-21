# Decision Log: Phase 5 — Lifecycle Notifications

Decisions specific to [Phase 5 — Lifecycle Notifications](../feature-specification.md). Decisions established by the [parent feature specification](../../feature-specification.md) and [parent decision log](../../artifacts/decision-log.md) — including [D6](../../artifacts/decision-log.md#d6-fire-cmux-notifications-at-named-lifecycle-moments) (three notification moments), [D19](../../artifacts/decision-log.md#d19-error-mode-notifications-direct-the-operator-to-the-control-pane) (error-mode directs to control pane and persists), [D15](../../artifacts/decision-log.md#d15-cmux-api-per-call-timeout-is-fatal), [D17](../../artifacts/decision-log.md#d17-log-file-artifacts-are-unchanged-in-cmux-mode), [D18](../../artifacts/decision-log.md#d18-startup-capability-check), [D-R1](../../artifacts/decision-log.md#d-r1-orchestrator-is-the-in-pane-pr9k-process-no-hidden-orchestrator-pane-supersedes-d13d-3d-4-hidden-pane-parts), and [D-R2](../../artifacts/decision-log.md#d-r2-cmux-v2-protocol-is-the-contract-of-record) — apply here unchanged and are not re-litigated.

## Full decisions

### D1: Three notification classes and their targeting

- **Question:** which lifecycle moments fire notifications, how many fire per moment, and where does the notification's "activate" gesture take the operator?
- **Decision:** three notification classes, all targeting the pr9k workspace by its cmux workspace handle ([parent D-R2](../../artifacts/decision-log.md#d-r2-cmux-v2-protocol-is-the-contract-of-record)). **Completion** fires exactly once when the run finishes successfully. **Run aborted** fires exactly once when the orchestrator's shutdown sequence runs with a non-zero outcome — including operator quit, error-mode quit, cmux per-call timeout, and (once Phase 6 lands) display-loss / pane-close paths. **Error-mode** fires on entry to the continue / retry / quit prompt and persists per [D3](#d3-error-mode-notification-fires-on-entry-then-re-fires-every-60-seconds) and [D4](#d4-dismissal-does-not-suppress-re-fire-pr9k-owns-the-cadence). All three target the workspace handle so the activation gesture brings the workspace into focus ([D9](#d9-click-to-focus-brings-the-pr9k-workspace-into-view)).
- **Rationale:** parent [D6](../../artifacts/decision-log.md#d6-fire-cmux-notifications-at-named-lifecycle-moments) committed to the three moments and to "no per-step / per-iteration" notifications; this decision pins the per-phase commitments — exact count per moment (one for completion, one for abort, persistent for error-mode) and the targeting handle. Targeting the workspace handle (rather than a specific surface) keeps Phase 5 within the cmux v2 methods already verified at [parent D-R2](../../artifacts/decision-log.md#d-r2-cmux-v2-protocol-is-the-contract-of-record); per-surface targeting would require an additional method verification and capability check for marginal operator value ([Deferred (YAGNI) — per-pane click-to-focus targeting](../feature-specification.md#deferred-yagni)).
- **Evidence:** parent [D6](../../artifacts/decision-log.md#d6-fire-cmux-notifications-at-named-lifecycle-moments), parent [D19](../../artifacts/decision-log.md#d19-error-mode-notifications-direct-the-operator-to-the-control-pane), parent [D-R1](../../artifacts/decision-log.md#d-r1-orchestrator-is-the-in-pane-pr9k-process-no-hidden-orchestrator-pane-supersedes-d13d-3d-4-hidden-pane-parts), parent [D-R2](../../artifacts/decision-log.md#d-r2-cmux-v2-protocol-is-the-contract-of-record); the build-outline Phase 5 entry naming the cmux v2 notification methods verified at commit `2f96c15c2`.
- **Rejected alternatives:**
  - **Per-step or per-iteration notifications** — rejected by parent [D6](../../artifacts/decision-log.md#d6-fire-cmux-notifications-at-named-lifecycle-moments) as noise; reaffirmed here.
  - **Separate notification classes for sub-reasons of abort** (operator quit vs. timeout vs. display loss) — rejected as scope creep that fails the simpler-version test; the operator reads the per-run log file for sub-reason. Deferred and reopenable ([Deferred (YAGNI)](../feature-specification.md#deferred-yagni)).
  - **Per-pane targeting** for the error-mode notification — deferred for the same reason ([Deferred (YAGNI)](../feature-specification.md#deferred-yagni)).
- **Linked technical notes:** —
- **Driven by findings:** —
- **Dependent decisions:** D2, D3, D6, D7, D8, D9
- **Referenced in spec:** Outcome, Primary Flow, Alternate Flows and States, Edge Cases and Failure Modes

### D2: Notification text is repo name plus lifecycle verb; error text incorporates the control-pane directive

- **Question:** what does each notification say?
- **Decision:** all three classes include the target repository's basename in their text. The completion notification reads as `pr9k run completed in <repo-basename>` (or its implementation-pinned equivalent). The run-aborted notification reads as `pr9k run aborted in <repo-basename>`. The error-mode notification reads as `<step name> failed in <repo-basename> — Focus the pr9k control pane to respond` and incorporates the directive verbatim per parent [D19](../../artifacts/decision-log.md#d19-error-mode-notifications-direct-the-operator-to-the-control-pane). The spec commits to the *content shape*; the implementation plan pins the exact wording.
- **Rationale:** the operator may run multiple parallel pr9k sessions against different repositories. Without the repo basename, the operator cannot tell which run produced the alert without activating it. The basename is already part of the pr9k workspace title ([parent D29](../../artifacts/decision-log.md#d29-workspace-name-pattern)), so it is cheap to surface in the notification text. The error-mode directive is mandated by parent [D19](../../artifacts/decision-log.md#d19-error-mode-notifications-direct-the-operator-to-the-control-pane) and the spec must incorporate it.
- **Evidence:**
  - User confirmation on 2026-05-21 selecting "repo-name + lifecycle verb" over "lifecycle verb only" and "workspace title as the prefix".
  - Parent [D19](../../artifacts/decision-log.md#d19-error-mode-notifications-direct-the-operator-to-the-control-pane) mandating "Focus the pr9k control pane to respond" in error-mode notification text.
  - Parent [D29](../../artifacts/decision-log.md#d29-workspace-name-pattern) establishing the repo basename as part of every pr9k workspace's identity.
- **Rejected alternatives:**
  - **Lifecycle verb only, no repo** — rejected; loses disambiguation across parallel runs.
  - **Full workspace title as prefix** — rejected; the workspace title also includes a high-resolution timestamp ([parent D29](../../artifacts/decision-log.md#d29-workspace-name-pattern)) which is operator-visible in the workspace list and would make notification text long and noisy.
- **Linked technical notes:** —
- **Driven by findings:** —
- **Dependent decisions:** —
- **Referenced in spec:** Outcome, Primary Flow, Edge Cases and Failure Modes, User Interactions

### D3: Error-mode notification fires on entry, then re-fires every 60 seconds

- **Question:** when does the error-mode notification first fire, and at what cadence does it re-fire?
- **Decision:** the error-mode notification fires once immediately on entry to the error prompt, then re-fires every **60 seconds** thereafter until the operator answers the prompt (continue / retry / quit). The cadence is fixed in this release and not operator-configurable.
- **Rationale:** the build outline's Phase 5 precondition explicitly calls for the team to settle the cadence before work starts so it is not litigated in review. 60 seconds is long enough that an operator reading the failing step's log is not spammed and short enough that an operator who walked away discovers the strand within a minute. The cadence is well above the 5–10 second per-call cmux timeout from [parent D15](../../artifacts/decision-log.md#d15-cmux-api-per-call-timeout-is-fatal), so it cannot collide with an in-flight notification call. Firing immediately on entry (rather than waiting one full cadence) gives the operator instant signal that the run is paused.
- **Evidence:**
  - User confirmation on 2026-05-21 selecting "60 seconds (Recommended)" over 30 seconds, 2 minutes, and 5 minutes.
  - Build outline Phase 5 preconditions: "Confirm the cadence at which the error-mode notification re-fires — the spec calls this an implementation-internal value but the team should agree on a number before the work starts so it is not litigated during review."
  - Parent [D15](../../artifacts/decision-log.md#d15-cmux-api-per-call-timeout-is-fatal) sets the per-call timeout at 5–10 seconds — 60 seconds is safely above any single-call window.
- **Rejected alternatives:**
  - **30 seconds** — too aggressive when the operator is deliberately reading the failure log; could feel spammy.
  - **2 minutes / 5 minutes** — too slow; doubles or quintuples the worst-case strand interval for the operator who silenced the first ping.
  - **Operator-configurable cadence** — rejected; no operator-described need, would add a configuration surface without evidence anyone wants a different number (same reasoning as [parent D27](../../artifacts/decision-log.md#d27-cmux-per-call-timeout-value)).
- **Linked technical notes:** —
- **Driven by findings:** —
- **Dependent decisions:** D4, D5
- **Referenced in spec:** Outcome, Primary Flow, Alternate Flows and States, Edge Cases and Failure Modes

### D4: Dismissal does not suppress re-fire; pr9k owns the cadence

- **Question:** if the operator dismisses an error-mode notification through cmux's notification chrome, does that suppress the next 60-second re-fire?
- **Decision:** **no**. pr9k owns the cadence and re-fires unconditionally every 60 seconds while the error prompt is unresolved, regardless of whether prior instances were dismissed by the operator or by the operating system's notification chrome. pr9k does not query dismissal state.
- **Rationale:** the whole reason the error-mode notification persists ([parent D6](../../artifacts/decision-log.md#d6-fire-cmux-notifications-at-named-lifecycle-moments)) is to prevent a dismissed notification from silently stranding the orchestrator. If dismissal suppressed re-fire, the operator could dismiss once, forget, and miss the prompt indefinitely — the exact failure mode the persistence is designed to prevent. Operators have a perfectly good "stop telling me about this" affordance: answer the prompt.
- **Evidence:**
  - Parent [D6](../../artifacts/decision-log.md#d6-fire-cmux-notifications-at-named-lifecycle-moments)'s persistence rationale: "A one-shot notification for an error prompt can be missed; persistence ensures the blocking state does not silently strand the run."
  - Build outline Phase 5 demo step 4: "The operator dismisses it; some seconds later the notification re-appears." Confirms dismissal-resistance is the intended behavior.
- **Rejected alternatives:**
  - **Dismissal suppresses the next re-fire** — rejected; defeats the purpose of persistence.
  - **Dismissal extends the cadence** (e.g., next re-fire is 5 minutes out instead of 60 seconds) — rejected; adds complexity without evidence anyone wants it.
  - **Pr9k offers an explicit "snooze" affordance** — rejected as out of scope; the only intended "snooze" is answering the prompt.
- **Linked technical notes:** —
- **Driven by findings:** —
- **Dependent decisions:** —
- **Referenced in spec:** Outcome, Primary Flow, Edge Cases and Failure Modes

### D5: Resolution of the error prompt stops re-fire immediately

- **Question:** when the operator answers the continue / retry / quit prompt, does the re-fire timer stop synchronously with the answer, or does one more notification slip out?
- **Decision:** the orchestrator stops the re-fire timer **immediately** on processing the operator's answer, before doing any other resolution work. Future scheduled re-fires are cancelled. A re-fire call that was already in flight at the moment the answer was processed is allowed to complete to keep cmux's state consistent — pr9k does not abort an in-flight cmux call mid-RPC.
- **Rationale:** the operator just told pr9k they are paying attention; any further notification would be noise. Cancelling future re-fires immediately is straightforward — the timer is a behavioral artifact pr9k owns. Aborting an in-flight cmux call would risk leaving cmux's notification surface in an inconsistent state; allowing it to complete is the simpler and safer policy.
- **Evidence:**
  - Build outline Phase 5 demo step 5: "presses the quit shortcut. The error-mode notification stops re-firing, the run aborts, and a 'run aborted' notification appears once." Confirms the synchronous-from-the-operator's-perspective semantics.
  - Operational principle (parent and Phase 4 D5): pr9k does not interfere with in-flight cmux calls.
- **Rejected alternatives:**
  - **Allow one more re-fire after the answer to confirm cancellation** — rejected as unnecessary; the answer itself is the confirmation.
  - **Abort any in-flight notification RPC** — rejected; risks half-applied state on cmux's side for no operator-visible benefit.
- **Linked technical notes:** —
- **Driven by findings:** —
- **Dependent decisions:** D6
- **Referenced in spec:** Outcome, Primary Flow, Alternate Flows and States, Edge Cases and Failure Modes

### D6: Quit from error mode stops re-fire, then fires run-aborted once

- **Question:** when the operator answers an error prompt with quit, do they see one notification (run aborted) or two (error mode plus run aborted)? In what order?
- **Decision:** **exactly one** "run aborted" notification fires after a quit-from-error-mode. The orchestrator stops the error-mode re-fire timer first (per [D5](#d5-resolution-of-the-error-prompt-stops-re-fire-immediately)), runs its abort shutdown, and fires the run-aborted notification once at the end of shutdown. There is no overlap: the operator does not see the error-mode notification re-appear after answering, and they see exactly one run-aborted alert.
- **Rationale:** without the explicit ordering commitment, an implementation might naively let one final re-fire happen after the abort shutdown begins, producing a confusing two-notification sequence. Pinning the order in the spec keeps the operator's experience clean.
- **Evidence:**
  - Build outline Phase 5 demo step 5 commits to this sequence: "error-mode notification stops re-firing, the run aborts, and a 'run aborted' notification appears once."
- **Rejected alternatives:**
  - **Skip the run-aborted notification when the abort came from an error-mode quit** (the operator just chose this themselves, so they "know") — rejected; the run-aborted notification is the canonical signal that the orchestrator is gone, and operators consistent at-completion alerts. Skipping it adds branching to the spec for no operator value.
  - **Allow one final error-mode notification after the answer for symmetry** — rejected per [D5](#d5-resolution-of-the-error-prompt-stops-re-fire-immediately).
- **Linked technical notes:** —
- **Driven by findings:** —
- **Dependent decisions:** —
- **Referenced in spec:** Outcome, Primary Flow, Alternate Flows and States

### D7: Non-timeout notification errors are logged and the run continues; timeouts remain fatal per parent D15

- **Question:** how does pr9k handle a notification call that fails for a non-timeout reason (workspace handle rejected, malformed value, transient cmux error)?
- **Decision:** non-timeout notification errors are recorded in the per-run log file (under `<projectDir>/.pr9k/logs/<stamp>/`) and the workflow continues. For the error-mode notification specifically, the next scheduled 60-second re-fire is attempted normally; a failure to fire one re-fire does not cancel the timer. Timeouts remain fatal under [parent D15](../../artifacts/decision-log.md#d15-cmux-api-per-call-timeout-is-fatal) and trigger the standard abort path — and within that abort path, the run-aborted notification's own call is attempted once; if it too times out, the timeout is logged but does not recurse into another abort cycle.
- **Rationale:** notifications are an *interrupt* surface, but the underlying run state is authoritatively held in the orchestrator and on disk. A non-timeout failure to fire one notification instance is an annoyance, not a correctness problem — losing one error-mode notification still leaves the next 60-second re-fire as a backstop, and losing the completion or abort notification just means the operator has to look at the workspace directly to see the terminal state. Timeouts are a different signal: they indicate cmux itself is unhealthy, which is exactly the case parent [D15](../../artifacts/decision-log.md#d15-cmux-api-per-call-timeout-is-fatal) designed for. The "log but don't recurse" rule for an abort-path notification timeout prevents the only obvious failure-cycle hazard.
- **Evidence:**
  - Phase 4's parallel decision [Phase 4 D5](../../phase-4-sidebar-mirroring/artifacts/decision-log.md#d5-non-timeout-sidebar-errors-are-logged-and-the-run-continues-timeouts-remain-fatal-per-parent-d15) established the same policy for sidebar calls; consistent treatment across cmux surface families keeps the operator's mental model simple.
  - Parent [D15](../../artifacts/decision-log.md#d15-cmux-api-per-call-timeout-is-fatal) commits cmux call timeouts to be fatal.
- **Rejected alternatives:**
  - **Treat any notification failure as fatal** — rejected; an annoyance failure should not abort an otherwise-healthy workflow.
  - **Retry failed notification calls in-place** — rejected as YAGNI; the error-mode notification's existing 60-second cadence is already a built-in retry, and no operator has reported missing completion/abort notifications.
  - **Recurse on abort-notification timeout** (try to fire the run-aborted abort cycle's run-aborted notification) — rejected; obvious infinite-loop hazard.
- **Linked technical notes:** —
- **Driven by findings:** —
- **Dependent decisions:** —
- **Referenced in spec:** Outcome, Primary Flow, Alternate Flows and States, Edge Cases and Failure Modes, User Interactions, Coordinations

### D8: The Phase 1 capability check is extended to cover Phase 5's notification methods

- **Question:** the Phase 1 startup capability check ([parent D18](../../artifacts/decision-log.md#d18-startup-capability-check)) verifies that the cmux build exposes every method pr9k will use. Should Phase 5 extend that check to cover the notification methods?
- **Decision:** **yes**. The Phase 1 capability check is extended at Phase 5 implementation time to include exactly the cmux v2 notification methods Phase 5 calls. Missing methods abort the launch with the same actionable, method-named error parent [D18](../../artifacts/decision-log.md#d18-startup-capability-check) already produces.
- **Rationale:** without extending the check, a cmux build that lacks the notification methods would launch normally and then start logging non-timeout errors per [D7](#d7-non-timeout-notification-errors-are-logged-and-the-run-continues-timeouts-remain-fatal-per-parent-d15) for every lifecycle event — a silent failure mode that operators would have to discover by inspecting log files. Failing fast at preflight with a method-named error gives the operator an actionable diagnostic ("upgrade cmux to a build that exposes X / Y / Z") that they can resolve before they next launch. This mirrors [Phase 4 D15](../../phase-4-sidebar-mirroring/artifacts/decision-log.md#d15-the-phase-1-capability-check-is-extended-to-cover-phase-4s-sidebar-methods)'s reasoning verbatim.
- **Evidence:**
  - Parent [D18](../../artifacts/decision-log.md#d18-startup-capability-check) commits to method-presence-based capability checks.
  - [Phase 4 D15](../../phase-4-sidebar-mirroring/artifacts/decision-log.md#d15-the-phase-1-capability-check-is-extended-to-cover-phase-4s-sidebar-methods) sets the precedent for extending the check per phase.
- **Rejected alternatives:**
  - **Skip the check; rely on D7's non-timeout-error logging** — rejected; trades a clear preflight failure for a silent runtime degradation.
- **Linked technical notes:** —
- **Driven by findings:** —
- **Dependent decisions:** —
- **Referenced in spec:** Actors and Triggers, Edge Cases and Failure Modes, User Interactions

### D9: Click-to-focus brings the pr9k workspace into view

- **Question:** when the operator activates a notification from cmux's notification chrome, where does focus land?
- **Decision:** activating the notification brings the **pr9k workspace** into focus. cmux's normal focus rules pick the active pane inside the workspace. The error-mode notification's text directs the operator to the footer pane ([parent D19](../../artifacts/decision-log.md#d19-error-mode-notifications-direct-the-operator-to-the-control-pane)); pr9k does not attempt per-pane targeting in this release.
- **Rationale:** all three notification classes share the same target (the pr9k workspace handle), which keeps the cmux v2 method set Phase 5 depends on small and consistent with what parent [D-R2](../../artifacts/decision-log.md#d-r2-cmux-v2-protocol-is-the-contract-of-record) already verified. Per-pane targeting would require an additional cmux v2 method ([deferred per Deferred (YAGNI)](../feature-specification.md#deferred-yagni)). The operator's hop from "activate notification" to "focus footer pane" is one focus change inside cmux — a small ergonomic cost that does not justify a stronger Phase 5 dependency now.
- **Evidence:**
  - User confirmation on 2026-05-21 selecting "Pr9k workspace, no specific pane (Recommended)" over "Footer pane directly" and "Defer to implementation plan".
  - Build outline Phase 5 demo step 2: "The operator clicks it and is taken to the pr9k workspace."
  - Parent [D-R2](../../artifacts/decision-log.md#d-r2-cmux-v2-protocol-is-the-contract-of-record)'s standard of verifying every cmux v2 method against the pinned source — sticking to workspace-scoped targeting minimizes the verification surface Phase 5 adds.
- **Rejected alternatives:**
  - **Per-pane targeting for the error-mode notification** — deferred ([Deferred (YAGNI) — per-pane click-to-focus targeting](../feature-specification.md#deferred-yagni)); reopens when operators report the inter-pane hop is friction worth an extra dependency.
  - **Defer the click target entirely to the implementation plan** — rejected; the build-outline demo commits to a specific behavior, and leaving it open invites a divergence between spec and demo.
- **Linked technical notes:** —
- **Driven by findings:** —
- **Dependent decisions:** —
- **Referenced in spec:** Outcome, Primary Flow, User Interactions

## Trivial decisions

- **D10:** No per-step or per-iteration notifications — inherited verbatim from [parent D6](../../artifacts/decision-log.md#d6-fire-cmux-notifications-at-named-lifecycle-moments); reaffirmed here so a reader scanning the Phase 5 spec does not have to chase the parent. — Referenced in spec: Outcome, Out of Scope.
- **D11:** The orchestrator is the in-pane pr9k process and is the sole caller of cmux's notification surface — inherited from [parent D-R1](../../artifacts/decision-log.md#d-r1-orchestrator-is-the-in-pane-pr9k-process-no-hidden-orchestrator-pane-supersedes-d13d-3d-4-hidden-pane-parts). — Referenced in spec: Outcome, Actors and Triggers, Primary Flow.
- **D12:** cmux v2 protocol of record applies to every notification call — inherited from [parent D-R2](../../artifacts/decision-log.md#d-r2-cmux-v2-protocol-is-the-contract-of-record). — Referenced in spec: Outcome, Coordinations.
- **D13:** Phase 5 introduces no on-disk artifact and no change to the existing per-run log file beyond adding non-timeout notification-error entries — inherited from [parent D17](../../artifacts/decision-log.md#d17-log-file-artifacts-are-unchanged-in-cmux-mode). — Referenced in spec: Outcome, Coordinations.
- **D14:** Notifications and sidebar updates are independent side-by-side effects of the same orchestrator lifecycle event — the orchestrator emits both for error-mode entry / exit, and neither blocks or coalesces with the other. — Referenced in spec: Outcome, Coordinations.
- **D15:** The error-mode notification text incorporates "Focus the pr9k control pane to respond" verbatim — inherited from [parent D19](../../artifacts/decision-log.md#d19-error-mode-notifications-direct-the-operator-to-the-control-pane). — Referenced in spec: Outcome, User Interactions.
- **D16:** Failure modes that bypass orchestrator shutdown (display loss, channel stall, forced kill, host shutdown) are Phase 6 scope — Phase 5 exposes the run-aborted notification through the orchestrator surface Phase 6 will call. — Referenced in spec: Outcome, Edge Cases and Failure Modes, Coordinations.

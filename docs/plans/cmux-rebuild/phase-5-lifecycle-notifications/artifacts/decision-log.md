# Decision Log: Phase 5 — Lifecycle Notifications

Decisions specific to [Phase 5 — Lifecycle Notifications](../feature-specification.md). Decisions established by the [parent feature specification](../../feature-specification.md) and [parent decision log](../../artifacts/decision-log.md) — including [D6](../../artifacts/decision-log.md#d6-fire-cmux-notifications-at-named-lifecycle-moments) (three notification moments), [D19](../../artifacts/decision-log.md#d19-error-mode-notifications-direct-the-operator-to-the-control-pane) (error-mode directs operator to control pane and persists), [D15](../../artifacts/decision-log.md#d15-cmux-api-per-call-timeout-is-fatal), [D17](../../artifacts/decision-log.md#d17-log-file-artifacts-are-unchanged-in-cmux-mode), [D18](../../artifacts/decision-log.md#d18-startup-capability-check), [D-R1](../../artifacts/decision-log.md#d-r1-orchestrator-is-the-in-pane-pr9k-process-no-hidden-orchestrator-pane-supersedes-d13d-3d-4-hidden-pane-parts), and [D-R2](../../artifacts/decision-log.md#d-r2-cmux-v2-protocol-is-the-contract-of-record) — apply here unchanged and are not re-litigated, except where Phase 5 narrows or supersedes their wording (see [D2](#d2-notification-text-is-repo-name-plus-lifecycle-verb-error-text-incorporates-the-footer-pane-directive) for the "footer pane" wording that supersedes [parent D19](../../artifacts/decision-log.md#d19-error-mode-notifications-direct-the-operator-to-the-control-pane)'s "control pane" phrasing).

## Full decisions

### D1: Three notification classes and their targeting

- **Question:** which lifecycle moments fire notifications, how many fire per moment, and where does the notification's "activate" gesture take the operator?
- **Decision:** three notification classes, all targeting the pr9k workspace by its cmux workspace handle ([parent D-R2](../../artifacts/decision-log.md#d-r2-cmux-v2-protocol-is-the-contract-of-record)). **Completion** fires exactly once when the run finishes successfully. **Run aborted** fires exactly once when the orchestrator's shutdown sequence runs with a non-zero outcome — including operator quit, error-mode quit, cmux per-call timeout, and (once Phase 6 lands) display-loss / pane-close paths. **Error-mode** fires on entry to the continue / retry / quit prompt and persists per [D3](#d3-error-mode-notification-fires-on-entry-then-re-fires-every-60-seconds) and [D4](#d4-dismissal-does-not-suppress-re-fire-pr9k-owns-the-cadence). All three target the workspace handle so the activation gesture brings the workspace into focus ([D9](#d9-click-to-focus-brings-the-pr9k-workspace-into-view)).
- **Rationale:** parent [D6](../../artifacts/decision-log.md#d6-fire-cmux-notifications-at-named-lifecycle-moments) committed to the three moments and to "no per-step / per-iteration" notifications; this decision pins the per-phase commitments — exact count per moment (one for completion, one for abort, persistent for error-mode) and the targeting handle. Targeting the workspace handle (rather than a specific surface) keeps Phase 5 within the cmux v2 methods already verified at [parent D-R2](../../artifacts/decision-log.md#d-r2-cmux-v2-protocol-is-the-contract-of-record); per-surface targeting would require an additional method verification and capability check for marginal operator value ([Deferred (YAGNI) — per-pane click-to-focus targeting](../feature-specification.md#deferred-yagni)).
- **Evidence:** parent [D6](../../artifacts/decision-log.md#d6-fire-cmux-notifications-at-named-lifecycle-moments), parent [D19](../../artifacts/decision-log.md#d19-error-mode-notifications-direct-the-operator-to-the-control-pane), parent [D-R1](../../artifacts/decision-log.md#d-r1-orchestrator-is-the-in-pane-pr9k-process-no-hidden-orchestrator-pane-supersedes-d13d-3d-4-hidden-pane-parts), parent [D-R2](../../artifacts/decision-log.md#d-r2-cmux-v2-protocol-is-the-contract-of-record); the build-outline Phase 5 entry naming the cmux v2 notification methods verified at commit `2f96c15c2`.
- **Rejected alternatives:**
  - **Per-step or per-iteration notifications** — rejected by parent [D6](../../artifacts/decision-log.md#d6-fire-cmux-notifications-at-named-lifecycle-moments) as noise; reaffirmed here.
  - **Separate notification classes for sub-reasons of abort** (operator quit vs. timeout vs. display loss) — rejected as scope creep that fails the simpler-version test; the operator reads the per-run log file for sub-reason. Deferred and reopenable ([Deferred (YAGNI) — distinct notification text per abort sub-reason](../feature-specification.md#deferred-yagni)).
  - **Per-pane targeting** for the error-mode notification — deferred for the same reason ([Deferred (YAGNI) — per-pane click-to-focus targeting](../feature-specification.md#deferred-yagni)).
- **Linked technical notes:** —
- **Driven by findings:** —
- **Dependent decisions:** D2, D3, D6, D7, D8, D9, D11, D12, D13
- **Referenced in spec:** Outcome, Primary Flow, Alternate Flows and States, Edge Cases and Failure Modes

### D2: Notification text is repo name plus lifecycle verb; error text incorporates the footer-pane directive

- **Question:** what does each notification say, and which vocabulary names the pane the operator should focus during error-mode?
- **Decision:** all three classes include the target repository's sanitized basename in their text. The implementation plan pins the exact strings; the spec-level commitments are:
  1. The text includes the repo basename (the same `SanitizeBasename`-processed string used in the pr9k workspace title per [parent D29](../../artifacts/decision-log.md#d29-workspace-name-pattern); when the sanitizer falls back to its placeholder ("repo") for an all-special-character directory name, the placeholder is what appears).
  2. The text includes a lifecycle verb identifying the moment.
  3. The error-mode text additionally includes the directive "Focus the footer pane to respond" verbatim.
- **Wording supersedes parent D19's "control pane" phrasing.** [Parent D19](../../artifacts/decision-log.md#d19-error-mode-notifications-direct-the-operator-to-the-control-pane) committed to the directive "Focus the pr9k control pane to respond". The shipped operator-facing how-to (`docs/how-to/setting-up-cmux.md`) and every other user-visible surface refer to the three panes as **header / log / footer**; "control pane" is internal architectural shorthand that does not appear in the how-to or in pane labels. Phase 5 supersedes the exact phrasing of parent D19 (still satisfying its behavioral intent: "the directive names the pane to focus") to maintain vocabulary consistency for the operator.
- The expected canonical strings are:
  - Completion: `pr9k run completed in <repo-basename>`
  - Run aborted: `pr9k run aborted in <repo-basename>`
  - Error-mode: `<step name> failed in <repo-basename> — Focus the footer pane to respond`
- **Rationale:** the operator may run multiple parallel pr9k sessions against different repositories. Without the repo basename, the operator cannot tell which run produced the alert without activating it. The basename is already part of the pr9k workspace title ([parent D29](../../artifacts/decision-log.md#d29-workspace-name-pattern)), so it is cheap to surface in the notification text. The directive vocabulary update is a copy correction, not a behavioral change: the operator who is told to focus the footer pane will look at the same pane parent D19 intended them to look at, while the wording matches what the how-to and shipped UI have always called that pane. For all-special-character directory names where the sanitizer falls back to "repo", multiple such runs are not textually distinguishable in the notification chrome — the workspace title carries the high-resolution timestamp for that case ([parent D29](../../artifacts/decision-log.md#d29-workspace-name-pattern)).
- **Evidence:**
  - User confirmation on 2026-05-21 selecting "repo-name + lifecycle verb" over "lifecycle verb only" and "workspace title as the prefix".
  - Parent [D19](../../artifacts/decision-log.md#d19-error-mode-notifications-direct-the-operator-to-the-control-pane) mandating a directive that names the pane to focus.
  - Parent [D29](../../artifacts/decision-log.md#d29-workspace-name-pattern) establishing the sanitized repo basename as part of every pr9k workspace's identity.
  - `docs/how-to/setting-up-cmux.md` (lines 89, 97, 101, 115 per UX-review finding F1) consistently uses "footer pane" in operator-facing instructions.
  - `internal/cmuxctl.SanitizeBasename` falls back to "repo" for all-special-character names; the same fallback applies to the notification text.
- **Rejected alternatives:**
  - **Lifecycle verb only, no repo** — rejected; loses disambiguation across parallel runs.
  - **Full workspace title as prefix** — rejected; the workspace title also includes a high-resolution timestamp ([parent D29](../../artifacts/decision-log.md#d29-workspace-name-pattern)) which is operator-visible in the workspace list and would make notification text long and noisy.
  - **Keep parent D19's "control pane" wording verbatim** — rejected; introduces vocabulary that does not appear anywhere else in the operator-facing product. The directive's *intent* (name the pane to focus) is preserved; only the phrasing is updated to match shipped naming.
- **Linked technical notes:** —
- **Driven by findings:** F1 (UX), F11 (junior-developer — spec content clarity), F15 (edge-case — sanitizer fallback)
- **Dependent decisions:** —
- **Referenced in spec:** Outcome, Primary Flow, Edge Cases and Failure Modes, User Interactions

### D3: Error-mode notification fires on entry, then re-fires every 60 seconds

- **Question:** when does the error-mode notification first fire, and at what cadence does it re-fire?
- **Decision:** the error-mode notification fires once immediately on entry to the error prompt (after the footer-pane mode-broadcast and the state-bit/timer setup per [D12](#d12-error-mode-effects-are-ordered-footer-broadcast-state-bit-and-timer-notification-call)), then re-fires every **60 seconds** thereafter until the operator answers the prompt (continue / retry / quit). The cadence is fixed in this release and not operator-configurable.
- **Rationale:** the build outline's Phase 5 precondition explicitly calls for the team to settle the cadence before work starts so it is not litigated in review. 60 seconds is long enough that an operator reading the failing step's log is not spammed and short enough that an operator who walked away discovers the strand within a minute. The cadence is well above the 5–10 second per-call cmux timeout from [parent D15](../../artifacts/decision-log.md#d15-cmux-api-per-call-timeout-is-fatal), so it cannot collide with an in-flight notification call under normal conditions. Firing immediately on entry (rather than waiting one full cadence) gives the operator instant signal that the run is paused.
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
- **Rationale:** the whole reason the error-mode notification persists ([parent D6](../../artifacts/decision-log.md#d6-fire-cmux-notifications-at-named-lifecycle-moments)) is to prevent a dismissed notification from silently stranding the orchestrator. If dismissal suppressed re-fire, the operator could dismiss once, forget, and miss the prompt indefinitely — the exact failure mode the persistence is designed to prevent. Operators have a perfectly good "stop telling me about this" affordance: answer the prompt. The UX-review finding F2 noted this is a coercive constraint (no in-pr9k escape hatch short of answering) and the appropriate remediation is documentation in the how-to, not a behavior change.
- **Evidence:**
  - Parent [D6](../../artifacts/decision-log.md#d6-fire-cmux-notifications-at-named-lifecycle-moments)'s persistence rationale: "A one-shot notification for an error prompt can be missed; persistence ensures the blocking state does not silently strand the run."
  - Build outline Phase 5 demo step 4: "The operator dismisses it; some seconds later the notification re-appears." Confirms dismissal-resistance is the intended behavior.
- **Rejected alternatives:**
  - **Dismissal suppresses the next re-fire** — rejected; defeats the purpose of persistence.
  - **Dismissal extends the cadence** (e.g., next re-fire is 5 minutes out instead of 60 seconds) — rejected; adds complexity without evidence anyone wants it.
  - **Pr9k offers an explicit "snooze" affordance** — rejected as out of scope; the only intended "snooze" is answering the prompt.
- **Linked technical notes:** —
- **Driven by findings:** F2 (UX — coercive constraint, remediated via how-to disclosure)
- **Dependent decisions:** —
- **Referenced in spec:** Outcome, Primary Flow, Edge Cases and Failure Modes

### D5: Resolution of the error prompt stops re-fire immediately

- **Question:** which operator keystroke stops the re-fire timer, and what happens to a notification call that was already in flight when that keystroke arrives?
- **Decision:** the orchestrator stops the re-fire timer **synchronously** at the moment it processes the operator's first resolution keystroke — `c` for continue, `r` for retry, or `q` (the first quit keystroke; *not* `y` confirmation). The stop signal precedes any other resolution work the orchestrator does. The operator may see at most one additional error-mode notification instance if a call was already in flight at the stop moment; no further calls are issued after the stop. The text of that trailing instance is identical to prior instances (same step name, same directive) — pr9k does not annotate it as "trailing".
- The quit flow specifically: if the operator presses `q` and then cancels the confirmation (Escape or a non-`y` keystroke), the timer is **restarted** from 0 so the operator continues to receive the periodic prompt until they actually answer.
- **Rationale:** the operator just told pr9k they are paying attention; any further notification call would be noise. The build-outline demo committed to "stops re-firing" the moment the quit shortcut is pressed; pinning the stop to the *first* keystroke rather than the confirmation keystroke preserves the demoable behavior even when the confirmation takes some time. Restarting the timer on a cancelled quit avoids stranding the operator who started to quit, changed their mind, and now has no re-fire because the timer was already stopped.
- **Evidence:**
  - Build outline Phase 5 demo step 5: "presses the quit shortcut. The error-mode notification stops re-firing, the run aborts, and a 'run aborted' notification appears once." Confirms the synchronous-from-the-operator's-perspective semantics.
  - Junior-developer finding F1 surfaced the two-step quit ambiguity; resolved here in favor of first-keystroke.
  - Junior-developer finding F6 (mechanics leaking) requested behavioral rather than mechanical wording; the spec sentence now expresses the operator-observable commitment ("at most one additional notification") rather than the implementation mechanic ("inflight call allowed to complete").
- **Rejected alternatives:**
  - **Stop the timer on the confirmation keystroke (`y`)** — rejected; a re-fire could land while the quit-confirm dialog is on screen, producing a confusing notification arriving after the operator already pressed `q`.
  - **Abort any in-flight notification RPC at the stop moment** — rejected; risks half-applied state on cmux's side for no operator-visible benefit.
  - **Leave the timer stopped after a cancelled quit confirmation** — rejected; would strand the operator who began quitting and reconsidered. The timer restart is the simpler-correct behavior.
- **Linked technical notes:** —
- **Driven by findings:** F1, F6, F12 (junior-developer); F7 (UX); EC2 (edge-case-explorer — stop-signal contract)
- **Dependent decisions:** D6, D12, D16
- **Referenced in spec:** Outcome, Primary Flow, Alternate Flows and States, Edge Cases and Failure Modes

### D6: Quit from error mode stops re-fire, then fires run-aborted once

- **Question:** when the operator answers an error prompt with quit, do they see one notification (run aborted) or two (error mode plus run aborted)? In what order?
- **Decision:** at most **two** notification surfaces visible to the operator: (1) optionally, one trailing in-flight error-mode notification if a call was mid-flight at the moment they pressed `q` (per [D5](#d5-resolution-of-the-error-prompt-stops-re-fire-immediately)); (2) exactly one "run aborted" notification at the end of shutdown ([D11](#d11-run-aborted-and-completion-notifications-fire-before-workspaceclose-while-the-workspace-handle-is-still-valid)). No re-fires happen after the operator presses `q`. The operator does not see overlapping persistent error-mode and run-aborted notifications.
- **Rationale:** without the explicit ordering commitment, an implementation might naively let one final re-fire happen after the abort shutdown begins, producing a confusing two-notification sequence beyond the trailing-instance case. Pinning the order in the spec keeps the operator's experience clean. The trailing instance is acknowledged in [D5](#d5-resolution-of-the-error-prompt-stops-re-fire-immediately) as an unavoidable artifact of allowing in-flight calls to complete.
- **Evidence:**
  - Build outline Phase 5 demo step 5 commits to this sequence: "error-mode notification stops re-firing, the run aborts, and a 'run aborted' notification appears once."
  - UX-review finding F7 noted the trailing-instance case warrants explicit documentation of "what the operator should expect next" — addressed here.
- **Rejected alternatives:**
  - **Skip the run-aborted notification when the abort came from an error-mode quit** (the operator just chose this themselves, so they "know") — rejected; the run-aborted notification is the canonical signal that the orchestrator is gone, and operators benefit from consistent at-completion alerts. Skipping it adds branching to the spec for no operator value.
  - **Allow one final error-mode notification after the answer for symmetry** — rejected per [D5](#d5-resolution-of-the-error-prompt-stops-re-fire-immediately).
- **Linked technical notes:** —
- **Driven by findings:** F7 (UX), F9 (UX — inflight race documentation completeness)
- **Dependent decisions:** D11
- **Referenced in spec:** Outcome, Primary Flow, Alternate Flows and States

### D7: Non-timeout notification errors are logged and the run continues; timeouts remain fatal per parent D15

- **Question:** how does pr9k handle a notification call that fails for a non-timeout reason (workspace handle rejected, malformed value, transient cmux error)?
- **Decision:** non-timeout notification errors are recorded in the per-run log file (under `<projectDir>/.pr9k/logs/<stamp>/`) and the workflow continues. For the error-mode notification specifically, the next scheduled 60-second re-fire is attempted normally; a failure to fire one re-fire does not cancel the timer. Timeouts remain fatal under [parent D15](../../artifacts/decision-log.md#d15-cmux-api-per-call-timeout-is-fatal) and trigger the standard abort path — **except** when the timing-out call is itself part of the abort path or was in flight when the operator answered an error prompt, in which case the timeout is non-fatal ([D10](#d10-abort-path-notification-calls-treat-every-failure-including-timeout-as-non-fatal-explicit-exception-to-parent-d15), [D16](#d16-in-flight-call-completing-after-an-error-mode-answer-is-treated-as-non-fatal-regardless-of-outcome)). Sustained non-timeout failures across many consecutive re-fires are an acceptable degraded state per [D17](#d17-sustained-non-timeout-notification-failures-are-acceptable-the-sidebar-pill-is-the-backstop).
- **Rationale:** notifications are an *interrupt* surface, but the underlying run state is authoritatively held in the orchestrator and on disk. A non-timeout failure to fire one notification instance is an annoyance, not a correctness problem — losing one error-mode notification still leaves the next 60-second re-fire as a backstop, and losing the completion or abort notification just means the operator has to look at the workspace directly to see the terminal state. Timeouts are a different signal: they indicate cmux itself is unhealthy, which is exactly the case parent [D15](../../artifacts/decision-log.md#d15-cmux-api-per-call-timeout-is-fatal) designed for. The two named exceptions (abort path, post-answer in-flight) prevent the only obvious failure-cycle hazards.
- **Evidence:**
  - Phase 4's parallel decision [Phase 4 D5](../../phase-4-sidebar-mirroring/artifacts/decision-log.md#d5-non-timeout-sidebar-errors-are-logged-and-the-run-continues-timeouts-remain-fatal-per-parent-d15) established the same policy for sidebar calls; consistent treatment across cmux surface families keeps the operator's mental model simple.
  - Parent [D15](../../artifacts/decision-log.md#d15-cmux-api-per-call-timeout-is-fatal) commits cmux call timeouts to be fatal.
  - Junior-developer finding F4 raised the "every re-fire fails non-fatally" case; resolved by sustaining the run with the sidebar as backstop ([D17](#d17-sustained-non-timeout-notification-failures-are-acceptable-the-sidebar-pill-is-the-backstop)).
  - Junior-developer finding F7 / EC8 (edge-case) requested explicit completion-call-timeout handling — added to the Edge Cases table.
- **Rejected alternatives:**
  - **Treat any notification failure as fatal** — rejected; an annoyance failure should not abort an otherwise-healthy workflow.
  - **Retry failed notification calls in-place** — rejected as YAGNI; the error-mode notification's existing 60-second cadence is already a built-in retry, and no operator has reported missing completion/abort notifications.
  - **Add a threshold of N consecutive non-timeout re-fire failures after which abort fires** — rejected because the sidebar pill is the explicit backstop ([D17](#d17-sustained-non-timeout-notification-failures-are-acceptable-the-sidebar-pill-is-the-backstop)); adding a threshold introduces operator-visible failure of an interrupt surface for no behavior the sidebar does not already provide.
- **Linked technical notes:** —
- **Driven by findings:** F4 (junior-developer — every re-fire fails), F7 (junior-developer — abort-path symmetry), EC6 (edge-case-explorer — sustained non-timeout failures), EC8 (edge-case-explorer — completion-call-timeout)
- **Dependent decisions:** D10, D16, D17
- **Referenced in spec:** Outcome, Primary Flow, Alternate Flows and States, Edge Cases and Failure Modes, User Interactions, Coordinations

### D8: The Phase 1 capability check is extended to cover Phase 5's notification methods

- **Question:** the Phase 1 startup capability check ([parent D18](../../artifacts/decision-log.md#d18-startup-capability-check)) verifies that the cmux build exposes every method pr9k will use. Should Phase 5 extend that check to cover the notification methods?
- **Decision:** **yes**. The Phase 1 capability check is extended at Phase 5 implementation time to include exactly the cmux v2 notification methods Phase 5 calls. Missing methods abort the launch with the same actionable, method-named error parent [D18](../../artifacts/decision-log.md#d18-startup-capability-check) already produces. The check is a startup-only gate; methods that disappear mid-run (e.g., cmux is upgraded while pr9k is running) fall through to the non-timeout-error path per [D7](#d7-non-timeout-notification-errors-are-logged-and-the-run-continues-timeouts-remain-fatal-per-parent-d15).
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
- **Decision:** activating the notification brings the **pr9k workspace** into focus. cmux's normal focus rules pick the active pane inside the workspace. The error-mode notification's text directs the operator to the footer pane ([D2](#d2-notification-text-is-repo-name-plus-lifecycle-verb-error-text-incorporates-the-footer-pane-directive)); pr9k does not attempt per-pane targeting in this release.
- **Rationale:** all three notification classes share the same target (the pr9k workspace handle), which keeps the cmux v2 method set Phase 5 depends on small and consistent with what parent [D-R2](../../artifacts/decision-log.md#d-r2-cmux-v2-protocol-is-the-contract-of-record) already verified. Per-pane targeting would require an additional cmux v2 method ([deferred per Deferred (YAGNI)](../feature-specification.md#deferred-yagni)). The operator's hop from "activate notification" to "focus footer pane" is one focus change inside cmux — a small ergonomic cost that does not justify a stronger Phase 5 dependency now. The UX-review finding F5 noted this hop should be made discoverable in how-to documentation; that addition is tracked in the implementation work-items.
- **Evidence:**
  - User confirmation on 2026-05-21 selecting "Pr9k workspace, no specific pane (Recommended)" over "Footer pane directly" and "Defer to implementation plan".
  - Build outline Phase 5 demo step 2: "The operator clicks it and is taken to the pr9k workspace."
  - Parent [D-R2](../../artifacts/decision-log.md#d-r2-cmux-v2-protocol-is-the-contract-of-record)'s standard of verifying every cmux v2 method against the pinned source — sticking to workspace-scoped targeting minimizes the verification surface Phase 5 adds.
- **Rejected alternatives:**
  - **Per-pane targeting for the error-mode notification** — deferred ([Deferred (YAGNI) — per-pane click-to-focus targeting](../feature-specification.md#deferred-yagni)); reopens when operators report the inter-pane hop is friction worth an extra dependency.
  - **Defer the click target entirely to the implementation plan** — rejected; the build-outline demo commits to a specific behavior, and leaving it open invites a divergence between spec and demo.
- **Linked technical notes:** —
- **Driven by findings:** F5 (UX — pane discoverability after click)
- **Dependent decisions:** —
- **Referenced in spec:** Outcome, Primary Flow, User Interactions

### D10: Abort-path notification calls treat every failure (including timeout) as non-fatal — explicit exception to parent D15

- **Question:** [parent D15](../../artifacts/decision-log.md#d15-cmux-api-per-call-timeout-is-fatal) makes every cmux call timeout fatal. The run-aborted notification's own call is fired as part of the abort path. If it times out, does the abort recurse?
- **Decision:** **no**. Abort-path notification calls — most prominently the run-aborted notification itself, but also any notification call that pr9k issues from within an already-running abort sequence — treat every failure (success, non-timeout error, *or* timeout) as non-fatal. The failure is logged to the per-run log file; the abort sequence continues to completion. The exception to parent [D15](../../artifacts/decision-log.md#d15-cmux-api-per-call-timeout-is-fatal) is explicit, scoped narrowly to abort-path calls, and documented as a first-class rule so an implementer following parent D15 alone does not produce a recursive abort.
- **Rationale:** the obvious failure-cycle hazard is "abort fires run-aborted notification, notification times out, timeout triggers abort, abort fires run-aborted notification, …". Without an explicit exception, an implementer could reasonably apply parent D15 verbatim and produce this loop. Naming the exception in its own decision makes the rule visible and prevents the recursion.
- **Evidence:**
  - Junior-developer finding F7 surfaced this asymmetry and requested it be a stated rule rather than inferrable from D7's rationale.
  - The Primary Flow already commits to "the run-aborted notification fires once" and "if it too times out, the timeout is logged but does not recurse" — this decision is the explicit naming of that rule.
- **Rejected alternatives:**
  - **Recurse on abort-notification timeout** — obvious infinite-loop hazard.
  - **Skip the run-aborted notification when cmux is unhealthy** — rejected; the operator still benefits from the notification even if it sometimes fails, and the failure is logged.
- **Linked technical notes:** —
- **Driven by findings:** F7 (junior-developer)
- **Dependent decisions:** —
- **Referenced in spec:** Outcome, Primary Flow, Alternate Flows and States, Edge Cases and Failure Modes, User Interactions, Coordinations

### D11: Run-aborted and completion notifications fire before workspace.close, while the workspace handle is still valid

- **Question:** the orchestrator's shutdown sequence includes both firing the terminal notification and closing the workspace (`workspace.close`, which reclaims the handle). In which order?
- **Decision:** the terminal notification (completion or run-aborted) fires **before** the `workspace.close` call. The workspace handle is still valid at the moment the notification call is issued; cmux can route the notification to the still-active workspace.
- **Rationale:** the edge-case-explorer finding EC1 surfaced that the current `runTeardown` (`src/internal/cmuxctl/runphase1.go:205–228`) calls `WorkspaceClose` near the end of shutdown. If the notification fires after `WorkspaceClose`, the workspace handle is reclaimed and the call returns a non-timeout error — per [D7](#d7-non-timeout-notification-errors-are-logged-and-the-run-continues-timeouts-remain-fatal-per-parent-d15) the error is logged and the run continues to exit, but the operator never receives the notification. The simple ordering rule — fire the notification while the workspace is still alive — eliminates that failure mode without introducing new mechanisms.
- **Evidence:**
  - Edge-case-explorer finding EC1, with the file/line citation to `runTeardown`.
  - Edge-case-explorer finding EC5: the relative ordering of `WorkspaceDone` broadcast and the notification call is also pinned by this decision — the notification call sequences before workspace teardown begins, which is also before `WorkspaceDone` is broadcast and the display panes ack the final state.
- **Rejected alternatives:**
  - **Fire notification after `WorkspaceClose`** — produces silent failure if cmux rejects calls against closed handles.
  - **Use a different cmux handle that survives close** — no such handle exists in cmux v2 ([parent D-R2](../../artifacts/decision-log.md#d-r2-cmux-v2-protocol-is-the-contract-of-record)).
- **Linked technical notes:** —
- **Driven by findings:** EC1, EC5 (edge-case-explorer)
- **Dependent decisions:** D6
- **Referenced in spec:** Primary Flow, Alternate Flows and States, Coordinations

### D12: Error-mode effects are ordered (footer broadcast → state bit and timer → notification call)

- **Question:** at error-mode entry the orchestrator must (a) broadcast the footer-pane mode switch, (b) set the error-mode state bit and start the re-fire timer, and (c) fire the initial notification call. In which order?
- **Decision:** the orchestrator performs these effects in exactly the order above:
  1. **Broadcast the footer-pane mode switch first** so when the operator activates a notification and arrives at the workspace, the footer pane is already showing the error-mode shortcut hints.
  2. **Set the error-mode state bit and start the re-fire timer second** so the timer is always stoppable regardless of how long the subsequent notification call takes (including a call that ends up timing out per [D7](#d7-non-timeout-notification-errors-are-logged-and-the-run-continues-timeouts-remain-fatal-per-parent-d15)).
  3. **Fire the notification call third.**
- **Rationale:** the UX-review finding F3 noted that without an ordering commitment, an operator could arrive at the workspace via the notification before the footer-pane mode-switch has rendered. The junior-developer finding F3 and the edge-case-explorer finding EC2/EC3 noted the timer-stop signal can only work reliably if the state bit is set before the notification call begins (otherwise an operator who answers during a long-running first notification call has no state bit to flip). The three-step ordering resolves both concerns and is consistent with the "orchestrator is the single source of truth for error-mode active" coordination commitment.
- **Evidence:**
  - UX finding F3 (three-surface ordering).
  - Junior-developer finding F3 (state bit / timer / notification call ordering).
  - Edge-case-explorer finding EC2 (stop-signal contract requires timer-state to be readable before the call) and EC3 (initial-call timeout requires timer-already-started semantics).
- **Rejected alternatives:**
  - **Fire the notification first, then broadcast / set state bit** — rejected; produces the F3 UX gap.
  - **Defer the ordering to the implementation plan** — rejected; the ordering is load-bearing for behavioral correctness (the timer-stop contract depends on it).
- **Linked technical notes:** —
- **Driven by findings:** F3 (UX), F3 (junior-developer), EC2, EC3 (edge-case-explorer)
- **Dependent decisions:** D5
- **Referenced in spec:** Primary Flow, User Interactions, Coordinations

### D13: Step name is snapshotted at error-mode entry, not recomputed at each re-fire

- **Question:** the error-mode notification text includes the failing step's name. Is the name captured at error-mode entry and reused for every re-fire, or recomputed at each cadence tick?
- **Decision:** **snapshotted at entry**. The orchestrator captures the current step name at the moment of error-mode entry (the same moment the state bit is set and the timer starts, per [D12](#d12-error-mode-effects-are-ordered-footer-broadcast-state-bit-and-timer-notification-call)). Every re-fire for that same prompt uses the snapshotted name. A subsequent error prompt — after the operator answered the first with continue/retry — snapshots its own name afresh.
- **Rationale:** the edge-case-explorer finding EC4 noted that `cmuxSidebar.lastStepName` (the candidate source) is a mutable field updated on every step transition. While the orchestrator is paused in error mode, no step transitions happen, so the field should be stable — but pr9k makes no general commitment that other code paths cannot mutate it. Snapshotting at entry makes the notification text invariant for the duration of one error-mode session, eliminating the class of bugs where the name changes mid-prompt.
- **Evidence:**
  - Edge-case-explorer finding EC4 with file/line citation to `src/cmd/pr9k/cmux_sidebar.go:62, 122`.
  - The orchestrator is sequential — only one error prompt is active at a time (Phase 3 spec confirms) — so snapshotting at entry gives a single authoritative value per prompt.
- **Rejected alternatives:**
  - **Recompute the step name at each re-fire** — exposes the spec to mutation hazards in `lastStepName` for marginal benefit (the name is already stable while error mode is active).
  - **Use a separate "currently-failing-step" field that pr9k writes only at error-mode entry** — would work but is a heavier mechanism than snapshotting at entry; the implementation can choose whichever it prefers as long as the spec-level commitment ("text reflects the step that was failing at entry") holds.
- **Linked technical notes:** —
- **Driven by findings:** EC4 (edge-case-explorer)
- **Dependent decisions:** —
- **Referenced in spec:** Primary Flow, Alternate Flows and States, User Interactions, Edge Cases and Failure Modes

## Trivial decisions

- **D14:** Notifications and sidebar updates are independent side-by-side effects of the same orchestrator lifecycle event — the orchestrator emits both for error-mode entry / exit, and neither blocks or coalesces with the other. — Referenced in spec: Outcome, Coordinations.
- **D15:** Failure modes that bypass orchestrator shutdown (display loss, channel stall, forced kill, host shutdown) are Phase 6 scope — Phase 5 exposes the run-aborted notification through the orchestrator surface Phase 6 will call. — Referenced in spec: Outcome, Edge Cases and Failure Modes, Coordinations.
- **D16:** A notification call that was already in flight at the moment the operator answered an error-mode prompt is treated as non-fatal regardless of outcome (success, non-timeout error, or timeout) — the error-mode state is already resolved by the operator's answer, so any subsequent abort would be spurious. Derived from [D5](#d5-resolution-of-the-error-prompt-stops-re-fire-immediately) and the operator-observable commitment "no further notifications after answering, except at most one trailing in-flight instance". — Referenced in spec: Primary Flow, Edge Cases and Failure Modes, Coordinations.
- **D17:** Sustained non-timeout notification failures are acceptable; the Phase 4 sidebar pill is the backstop — when every notification call fails non-fatally, the workflow remains paused at the error prompt, the per-run log file records each failure, and the operator's persistent signal is the sidebar's "— awaiting input" marker ([Phase 4 D13](../../phase-4-sidebar-mirroring/artifacts/decision-log.md#d13-error-mode-suffixes-a-stable-marker-onto-the-status-entry-value)). No threshold or escalation; the sidebar pill was designed as this backstop. — Referenced in spec: Outcome, Alternate Flows and States, User Interactions.
- **D18:** Step names are non-empty and control-character-free; this is a workflow-config validator invariant. Phase 5 does not define a fallback for empty step names. — Referenced in spec: Actors and Triggers.

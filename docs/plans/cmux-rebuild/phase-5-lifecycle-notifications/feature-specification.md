# Feature Specification: Phase 5 — Lifecycle Notifications

Phase 5 of the [pr9k cmux mode build outline](../build-phase-outline.md). While a workflow runs in a pr9k cmux workspace, cmux's notification surface fires at three named lifecycle moments: when the full run completes successfully, when the run aborts unsuccessfully, and when an error-mode prompt is awaiting the operator's continue / retry / quit decision. The error-mode notification persists — it re-fires on a regular cadence until the operator answers — so an operator whose attention is on another workspace is not silently stranded by a single dismissed notification.

This phase closes the "monitor from outside" story Phase 4 started. [Phase 4 — Sidebar Mirroring](../phase-4-sidebar-mirroring/feature-specification.md) gives the operator passive at-a-glance signal from any other cmux workspace; Phase 5 gives the *interrupt* — the operator's attention is pulled back when the run reaches a terminal state or needs a decision. The two surfaces are independent at the orchestrator's call site (Phase 4 pushes sidebar entries; Phase 5 fires notifications) and complementary at the operator's surface (passive pill + active alert).

Parent artifacts:

- [Parent feature specification](../feature-specification.md)
- [Parent decision log](../artifacts/decision-log.md)
- [Parent feature technical notes](../artifacts/feature-technical-notes.md)

Builds on:

- [Phase 2 — First Real Workflow Runs End-to-End in Cmux](../phase-2-real-workflow-runs/feature-specification.md): the orchestrator's authoritative knowledge of run completion and the workspace handle it owns ([parent D-R1](../artifacts/decision-log.md#d-r1-orchestrator-is-the-in-pane-pr9k-process-no-hidden-orchestrator-pane-supersedes-d13d-3d-4-hidden-pane-parts), [parent D-R2](../artifacts/decision-log.md#d-r2-cmux-v2-protocol-is-the-contract-of-record)).
- [Phase 3 — Interactive Error Recovery](../phase-3-interactive-error-recovery/feature-specification.md): the error-mode entry and exit events that drive the error-mode notification.

## Outcome

During a pr9k cmux run, cmux's notification surface receives exactly three classes of notification, all originating from the in-pane orchestrator process ([parent D-R1](../artifacts/decision-log.md#d-r1-orchestrator-is-the-in-pane-pr9k-process-no-hidden-orchestrator-pane-supersedes-d13d-3d-4-hidden-pane-parts)) and all targeting the pr9k workspace so that activating the notification brings the operator to it ([D9](artifacts/decision-log.md#d9-click-to-focus-brings-the-pr9k-workspace-into-view)):

- **Completion** — fires exactly once when the full run finishes successfully ([parent D6](../artifacts/decision-log.md#d6-fire-cmux-notifications-at-named-lifecycle-moments), [D1](artifacts/decision-log.md#d1-three-notification-classes-and-their-targeting)).
- **Run aborted** — fires exactly once when the run ends unsuccessfully through any path the orchestrator can run shutdown for: operator-initiated quit, escalated failure, cmux call timeout, or display-loss / pane-close abort once Phase 6 integrates them ([parent D6](../artifacts/decision-log.md#d6-fire-cmux-notifications-at-named-lifecycle-moments), [D1](artifacts/decision-log.md#d1-three-notification-classes-and-their-targeting), [D6 of this phase](artifacts/decision-log.md#d6-quit-from-error-mode-stops-re-fire-then-fires-run-aborted-once)).
- **Error-mode awaiting input** — fires on entry to pr9k's continue / retry / quit error prompt and re-fires every 60 seconds until the operator answers, even across dismissals ([parent D19](../artifacts/decision-log.md#d19-error-mode-notifications-direct-the-operator-to-the-control-pane), [D3](artifacts/decision-log.md#d3-error-mode-notification-fires-on-entry-then-re-fires-every-60-seconds), [D4](artifacts/decision-log.md#d4-dismissal-does-not-suppress-re-fire-pr9k-owns-the-cadence)). Its text directs the operator to focus the footer pane ([parent D19](../artifacts/decision-log.md#d19-error-mode-notifications-direct-the-operator-to-the-control-pane), [D2](artifacts/decision-log.md#d2-notification-text-is-repo-name-plus-lifecycle-verb-error-text-incorporates-the-control-pane-directive)).

No notifications fire at per-step or per-iteration boundaries — those would be noise ([parent D6](../artifacts/decision-log.md#d6-fire-cmux-notifications-at-named-lifecycle-moments)). The persistent re-fire is unique to the error-mode notification; the completion and abort notifications each fire exactly once.

The notification surface is the *interrupt* counterpart to [Phase 4](../phase-4-sidebar-mirroring/feature-specification.md)'s sidebar entries. The two are independent: at error-mode entry, the sidebar status pill gains its "— awaiting input" marker ([Phase 4 D13](../phase-4-sidebar-mirroring/artifacts/decision-log.md#d13-error-mode-suffixes-a-stable-marker-onto-the-status-entry-value)) at the same moment the notification fires ([D14](artifacts/decision-log.md#d14-notifications-and-sidebar-updates-are-independent-side-by-side-effects-of-the-same-event)). The two effects are emitted side-by-side by the orchestrator off the same lifecycle event, and the sidebar marker is the persistent passive signal that survives notification dismissal ([Phase 4 D13](../phase-4-sidebar-mirroring/artifacts/decision-log.md#d13-error-mode-suffixes-a-stable-marker-onto-the-status-entry-value)).

Notification text is consistent across the three classes and identifies the target repository so an operator monitoring multiple parallel pr9k runs can tell which run produced which alert ([D2](artifacts/decision-log.md#d2-notification-text-is-repo-name-plus-lifecycle-verb-error-text-incorporates-the-control-pane-directive)).

Phase 5 introduces no on-disk artifact and no change to the header, log, or footer panes; the pr9k log file under `<projectDir>/.pr9k/logs/<stamp>/` is unchanged ([parent D17](../artifacts/decision-log.md#d17-log-file-artifacts-are-unchanged-in-cmux-mode)). A notification call that fails for a non-timeout reason is logged to that file and the run continues; a notification call that times out is fatal per [parent D15](../artifacts/decision-log.md#d15-cmux-api-per-call-timeout-is-fatal) and triggers the run-abort path ([D7](artifacts/decision-log.md#d7-non-timeout-notification-errors-are-logged-and-the-run-continues-timeouts-remain-fatal-per-parent-d15)).

## Actors and Triggers

- **Primary actor:** the human operator running pr9k in cmux mode against a target repository, from a shell inside a cmux session — the same actor and trigger Phases 1–4 establish.
- **Triggers** (three, one per notification class):
  - **Completion trigger:** the orchestrator's run loop reaches the end of the workflow's finalization phase successfully — the same terminal state Phase 2's run loop already detects.
  - **Run-aborted trigger:** the orchestrator's shutdown sequence runs with a non-zero outcome — operator-initiated quit, escalated failure, cmux per-call timeout, or Phase 6 abort paths once integrated.
  - **Error-mode trigger:** a step exits in a way that triggers pr9k's existing continue / retry / quit error prompt — the same event [Phase 3](../phase-3-interactive-error-recovery/feature-specification.md) uses to drive the footer pane's mode switch and [Phase 4 D13](../phase-4-sidebar-mirroring/artifacts/decision-log.md#d13-error-mode-suffixes-a-stable-marker-onto-the-status-entry-value)'s sidebar marker.
- **Preconditions:**
  - All Phase 1 / Phase 2 preconditions hold: cmux is reachable in a mode that admits pr9k, the workspace exists, and the three display panes are connected ([parent D16](../artifacts/decision-log.md#d16-per-launch-readiness-handshake-before-workflow-starts)).
  - The orchestrator holds the pr9k workspace's cmux handle so every notification call can target the right workspace ([parent D-R1](../artifacts/decision-log.md#d-r1-orchestrator-is-the-in-pane-pr9k-process-no-hidden-orchestrator-pane-supersedes-d13d-3d-4-hidden-pane-parts), [parent D-R2](../artifacts/decision-log.md#d-r2-cmux-v2-protocol-is-the-contract-of-record)).
  - The Phase 1 startup capability check ([parent D18](../artifacts/decision-log.md#d18-startup-capability-check)) is extended at Phase 5 implementation time to include the cmux notification methods Phase 5 calls; missing methods abort the launch with the same actionable, method-named error parent D18 produces ([D8](artifacts/decision-log.md#d8-the-phase-1-capability-check-is-extended-to-cover-phase-5s-notification-methods)).

## Primary Flow

1. The workflow runs as in Phase 2 — the workspace exists, the three display panes are rendering, the orchestrator advances steps and iterations. No notifications fire during normal progress; the only notification surfaces in play are the three triggered classes ([parent D6](../artifacts/decision-log.md#d6-fire-cmux-notifications-at-named-lifecycle-moments)).
2. When the run reaches its terminal state and the orchestrator's shutdown sequence runs, it fires exactly one notification before its other shutdown work completes:
   - **Successful completion:** a single completion notification with text identifying the target repository and the success verb ([D1](artifacts/decision-log.md#d1-three-notification-classes-and-their-targeting), [D2](artifacts/decision-log.md#d2-notification-text-is-repo-name-plus-lifecycle-verb-error-text-incorporates-the-control-pane-directive)).
   - **Unsuccessful abort:** a single "run aborted" notification with text identifying the target repository and the abort verb ([D1](artifacts/decision-log.md#d1-three-notification-classes-and-their-targeting), [D2](artifacts/decision-log.md#d2-notification-text-is-repo-name-plus-lifecycle-verb-error-text-incorporates-the-control-pane-directive)). The same abort path is followed by an operator-initiated quit, a step failure the operator chose to quit through ([D6 of this phase](artifacts/decision-log.md#d6-quit-from-error-mode-stops-re-fire-then-fires-run-aborted-once)), a cmux per-call timeout ([parent D15](../artifacts/decision-log.md#d15-cmux-api-per-call-timeout-is-fatal)), and (once [Phase 6](../build-phase-outline.md#phase-6) integrates them) every other display-loss or pane-close failure path.
3. The completion and run-aborted notifications target the pr9k workspace by its cmux handle so activating the notification from the operating system's notification surface brings the pr9k workspace into focus ([D9](artifacts/decision-log.md#d9-click-to-focus-brings-the-pr9k-workspace-into-view)). The notification's surfacing chrome — banner, badge, sound, system notification panel — is owned by cmux and the host operating system; pr9k does not draw it ([T1](artifacts/feature-technical-notes.md#t1-cmux-notification-surface-shape) if captured, otherwise discoverable from the cmuxctl client).
4. When a step exits in a way that triggers pr9k's continue / retry / quit error prompt, the orchestrator fires the **error-mode notification** as part of the same lifecycle event that switches the footer pane into error mode ([Phase 3](../phase-3-interactive-error-recovery/feature-specification.md)) and adds the "— awaiting input" suffix to the sidebar status pill ([Phase 4 D13](../phase-4-sidebar-mirroring/artifacts/decision-log.md#d13-error-mode-suffixes-a-stable-marker-onto-the-status-entry-value)). The notification text identifies the failing step's name, the target repository, and the directive "Focus the pr9k control pane to respond" ([D2](artifacts/decision-log.md#d2-notification-text-is-repo-name-plus-lifecycle-verb-error-text-incorporates-the-control-pane-directive), [parent D19](../artifacts/decision-log.md#d19-error-mode-notifications-direct-the-operator-to-the-control-pane)). The notification targets the pr9k workspace; activating it brings the workspace into focus, and cmux's normal focus rules pick the active pane ([D9](artifacts/decision-log.md#d9-click-to-focus-brings-the-pr9k-workspace-into-view)).
5. The error-mode notification re-fires every 60 seconds from the moment it was first issued, until the operator answers the error prompt ([D3](artifacts/decision-log.md#d3-error-mode-notification-fires-on-entry-then-re-fires-every-60-seconds)). The re-fire is unconditional with respect to whether the operator has dismissed any prior instance — pr9k owns the cadence, and dismissal by the operator (or by the operating system's notification chrome) does not suppress the next firing ([D4](artifacts/decision-log.md#d4-dismissal-does-not-suppress-re-fire-pr9k-owns-the-cadence)). The cadence is fixed in this release and not operator-configurable ([D3](artifacts/decision-log.md#d3-error-mode-notification-fires-on-entry-then-re-fires-every-60-seconds)).
6. When the operator answers the error prompt — continue, retry, or quit — the orchestrator stops the error-mode re-fire timer before doing anything else ([D5](artifacts/decision-log.md#d5-resolution-of-the-error-prompt-stops-re-fire-immediately)). The next notification, if any, depends on which answer was given:
   - **Continue or retry:** no additional notification fires from the resolution event; the run resumes and the next notification is whichever lifecycle moment occurs next (completion at successful run end, a new error-mode prompt if another step fails, or run-aborted if a subsequent path aborts the run).
   - **Quit:** the error-mode re-fire stops immediately, the orchestrator runs its abort shutdown, and the **run-aborted** notification fires exactly once at the end of that shutdown ([D6 of this phase](artifacts/decision-log.md#d6-quit-from-error-mode-stops-re-fire-then-fires-run-aborted-once)). The operator does not see overlapping error-mode and run-aborted notifications.
7. If a notification call returns a non-timeout error (the workspace handle was rejected, the call temporarily failed, the cmux process returned a malformed response), the error is recorded in the per-run log file under `<projectDir>/.pr9k/logs/<stamp>/` and the workflow continues; for the persistent error-mode notification, the next 60-second re-fire is attempted normally ([D7](artifacts/decision-log.md#d7-non-timeout-notification-errors-are-logged-and-the-run-continues-timeouts-remain-fatal-per-parent-d15)).
8. If a notification call times out (cmux's socket accepted the request but did not return a response within the configured per-call timeout from [parent D15](../artifacts/decision-log.md#d15-cmux-api-per-call-timeout-is-fatal)), the timeout is fatal and triggers the same abort path display-loss takes ([D7](artifacts/decision-log.md#d7-non-timeout-notification-errors-are-logged-and-the-run-continues-timeouts-remain-fatal-per-parent-d15)). The notification call that produced the timeout is *not* itself a candidate for a follow-up run-aborted notification — the abort path is responsible for firing run-aborted exactly once, and the timed-out call is treated as an error for that call only.
9. On-disk artifacts (per-run log file, per-step `.jsonl` files, iteration log) are unchanged by Phase 5 — notifications produce no new artifact ([parent D17](../artifacts/decision-log.md#d17-log-file-artifacts-are-unchanged-in-cmux-mode)).

## Alternate Flows and States

### Operator quits successfully mid-run (no failing step in progress)

- **Entry condition:** the operator presses the quit shortcut and confirms (existing two-step `q` then `y`), with no failing step pending.
- **Sequence:** the orchestrator runs its abort shutdown. At the end of shutdown, the **run-aborted** notification fires exactly once with the repo name and the abort verb ([D1](artifacts/decision-log.md#d1-three-notification-classes-and-their-targeting), [D2](artifacts/decision-log.md#d2-notification-text-is-repo-name-plus-lifecycle-verb-error-text-incorporates-the-control-pane-directive)). The completion notification does not fire — completion is reserved for successful run end. No error-mode notification fires because no error prompt was open.
- **Exit:** the workspace remains open per [parent D14](../artifacts/decision-log.md#d14-workspace-closure-is-operator-initiated); the operator dismisses it to restore the prior workspace.

### Operator answers an error-mode prompt with quit

- **Entry condition:** the run is paused at an error prompt; the persistent error-mode notification has fired at least once and may have re-fired one or more times.
- **Sequence:** the operator presses the quit shortcut in the footer pane. The orchestrator stops the error-mode re-fire timer immediately ([D5](artifacts/decision-log.md#d5-resolution-of-the-error-prompt-stops-re-fire-immediately)), then runs its abort shutdown, then fires the **run-aborted** notification exactly once at the end of shutdown ([D6 of this phase](artifacts/decision-log.md#d6-quit-from-error-mode-stops-re-fire-then-fires-run-aborted-once)). The operator never sees an error-mode and a run-aborted notification surfaced simultaneously; the cancellation is synchronous from the operator's perspective.
- **Exit:** workspace remains open; operator dismisses it.

### Operator answers an error-mode prompt with continue or retry

- **Entry condition:** the run is paused at an error prompt; the persistent error-mode notification has fired at least once.
- **Sequence:** the operator presses continue or retry. The orchestrator stops the error-mode re-fire timer ([D5](artifacts/decision-log.md#d5-resolution-of-the-error-prompt-stops-re-fire-immediately)) and resumes the workflow. No notification fires as part of the resolution itself — the next notification is whichever lifecycle moment occurs next (a subsequent error prompt, the final completion notification at successful run end, or the run-aborted notification if a later failure aborts the run).
- **Exit:** the run continues until its next lifecycle moment.

### Multiple successive error prompts in the same run

- **Entry condition:** the operator resolves one error prompt with continue or retry, and a later step in the same run also fails and triggers a fresh error prompt.
- **Sequence:** the orchestrator treats each error-mode entry as a separate lifecycle event. A new error-mode notification fires on entry to the new prompt (with that step's name in the text), and the re-fire timer is restarted from 0 — there is no carryover from the prior prompt's re-fire schedule. The text reflects the new failing step.
- **Exit:** resolution proceeds as in the relevant single-prompt flow above.

### Error prompt is open at the moment cmux itself becomes unreachable

- **Entry condition:** the error-mode notification is re-firing on its cadence, and a re-fire call times out (cmux became unresponsive while the run was paused).
- **Sequence:** the timeout is fatal per [parent D15](../artifacts/decision-log.md#d15-cmux-api-per-call-timeout-is-fatal). The orchestrator stops the re-fire timer and runs its abort shutdown. The run-aborted notification's *call* may itself fail or time out — the orchestrator attempts it once; non-timeout errors are logged and shutdown continues; a timeout on the abort notification is logged as an error for that call but does not recurse into a further abort cycle ([D7](artifacts/decision-log.md#d7-non-timeout-notification-errors-are-logged-and-the-run-continues-timeouts-remain-fatal-per-parent-d15)).
- **Exit:** the orchestrator exits non-zero; the orchestrator's stderr and the per-run log file under `<projectDir>/.pr9k/logs/<stamp>/` contain the timeout diagnostic. The workspace state is whatever cmux preserved at the moment it became unresponsive.

### Run completes successfully with no error prompts along the way

- **Entry condition:** the workflow runs from start to finalization without any failing step.
- **Sequence:** no notifications fire during the run. At the end of finalization, the **completion** notification fires exactly once with the repo name and the success verb.
- **Exit:** workspace remains open; the operator dismisses it.

## Edge Cases and Failure Modes

| Condition | Required Behavior |
|-----------|-------------------|
| Notification call returns a non-timeout error. | Error is logged to the per-run log file under `<projectDir>/.pr9k/logs/<stamp>/`; workflow continues. For the error-mode notification, the next scheduled 60-second re-fire is attempted normally ([D7](artifacts/decision-log.md#d7-non-timeout-notification-errors-are-logged-and-the-run-continues-timeouts-remain-fatal-per-parent-d15)). |
| Notification call times out. | Treated identically to any other cmux call timeout per [parent D15](../artifacts/decision-log.md#d15-cmux-api-per-call-timeout-is-fatal) — fatal, triggers abort. The run-aborted notification is attempted once at shutdown; if it too times out the timeout is logged but does not recurse ([D7](artifacts/decision-log.md#d7-non-timeout-notification-errors-are-logged-and-the-run-continues-timeouts-remain-fatal-per-parent-d15)). |
| The operator dismisses the error-mode notification at the operating system / cmux notification chrome. | Dismissal is observed by the host's notification chrome only; pr9k does not query dismissal state. The next 60-second cadence tick fires a fresh notification regardless of whether prior instances were dismissed ([D4](artifacts/decision-log.md#d4-dismissal-does-not-suppress-re-fire-pr9k-owns-the-cadence)). |
| The operator answers the error prompt during the brief interval between a re-fire call being initiated and cmux returning a response. | The orchestrator may emit one final notification instance whose call was already inflight at the moment the answer was processed. The cancellation stops *future* re-fires immediately; an inflight call is allowed to complete to keep cmux's state consistent ([D5](artifacts/decision-log.md#d5-resolution-of-the-error-prompt-stops-re-fire-immediately)). |
| Operator launches pr9k with an iteration cap (`-n M`) and the workflow completes all iterations and the full finalization phase successfully. | One completion notification fires at the end of finalization. No per-iteration notifications fire ([parent D6](../artifacts/decision-log.md#d6-fire-cmux-notifications-at-named-lifecycle-moments)). |
| Operator launches pr9k without an iteration cap and the workflow's break condition fires partway through the run. | Treated as a successful completion — the workflow ended at its own configured break point with no failure. One completion notification fires at the end of finalization ([D1](artifacts/decision-log.md#d1-three-notification-classes-and-their-targeting)). |
| Two pr9k cmux runs are active concurrently against different target repositories. | Each run fires notifications targeting its own workspace handle. The repo basename in the notification text disambiguates which run produced which alert ([D2](artifacts/decision-log.md#d2-notification-text-is-repo-name-plus-lifecycle-verb-error-text-incorporates-the-control-pane-directive)). |
| A failure mode that bypasses the orchestrator's shutdown sequence fires (display-pane loss, channel stall, orchestrator forced kill, host shutdown). | Phase 5 makes no commitment about a notification firing on these paths. [Phase 6](../build-phase-outline.md#phase-6) integrates the run-aborted notification into its abort handling for the paths it can recover; a forced kill or host shutdown leaves the orphan workspace per [Phase 6](../build-phase-outline.md#phase-6) and is the input to [Phase 7](../build-phase-outline.md#phase-7)'s startup advisory. |
| The cmux capability check at startup finds that the notification methods are not available on the cmux build. | The launch aborts with the same actionable, method-named error [parent D18](../artifacts/decision-log.md#d18-startup-capability-check) produces, listing exactly which notification methods are missing ([D8](artifacts/decision-log.md#d8-the-phase-1-capability-check-is-extended-to-cover-phase-5s-notification-methods)). |
| A second error prompt fires in the same run after the first was resolved with continue or retry. | A fresh error-mode notification fires on entry to the second prompt; the re-fire timer starts from 0 — there is no carryover from the prior prompt's schedule. |

## User Interactions

- **Affordances:**
  - **Cmux notification surface (visible system-wide, owned by cmux and the host operating system):** the pr9k workspace is the target; activating the notification brings the pr9k workspace into focus ([D9](artifacts/decision-log.md#d9-click-to-focus-brings-the-pr9k-workspace-into-view)). pr9k does not draw the notification chrome; pr9k supplies the title text and the target workspace handle.
  - **Notification text — completion:** identifies the target repository basename and uses the success verb (for example, `pr9k run completed in <repo-basename>`). The exact wording is the implementation plan's to pin; the spec-level commitment is "repo basename, lifecycle verb, no other content" ([D2](artifacts/decision-log.md#d2-notification-text-is-repo-name-plus-lifecycle-verb-error-text-incorporates-the-control-pane-directive)).
  - **Notification text — run aborted:** identifies the target repository basename and uses the abort verb (for example, `pr9k run aborted in <repo-basename>`). Same commitment shape as completion.
  - **Notification text — error-mode:** identifies the failing step's name, the target repository basename, and incorporates "Focus the pr9k control pane to respond" per [parent D19](../artifacts/decision-log.md#d19-error-mode-notifications-direct-the-operator-to-the-control-pane). For example, `<step name> failed in <repo-basename> — Focus the pr9k control pane to respond.`
- **Feedback:**
  - At the completion or run-aborted moment, a single notification surfaces in cmux's notification chrome; no in-pane visual change is owned by Phase 5 (the panes continue to show whatever Phase 2/3 already shows at those terminal states).
  - At error-mode entry, the notification surfaces in parallel with [Phase 4 D13](../phase-4-sidebar-mirroring/artifacts/decision-log.md#d13-error-mode-suffixes-a-stable-marker-onto-the-status-entry-value)'s sidebar marker and [Phase 3](../phase-3-interactive-error-recovery/feature-specification.md)'s footer mode switch — the operator sees a coordinated three-surface signal: interrupt (notification), passive monitor (sidebar pill), and inline shortcut hints (footer pane).
  - On each 60-second re-fire, the notification re-appears as a fresh instance in cmux's chrome regardless of whether any prior instance was dismissed.
- **Error states:**
  - A notification call that fails for a non-timeout reason produces no operator-visible change; the failure is recorded in the per-run log file only ([D7](artifacts/decision-log.md#d7-non-timeout-notification-errors-are-logged-and-the-run-continues-timeouts-remain-fatal-per-parent-d15)).
  - A notification call that times out triggers the standard abort path (parent D15, Phase 6 surfaces).
  - A missing cmux notification method at preflight: the launch aborts with a [parent D18](../artifacts/decision-log.md#d18-startup-capability-check)-style actionable error naming the missing methods ([D8](artifacts/decision-log.md#d8-the-phase-1-capability-check-is-extended-to-cover-phase-5s-notification-methods)).

## Coordinations

| Counterpart | Direction | Interactions | Constraints |
|-------------|-----------|--------------|-------------|
| cmux (notification surface) | outbound | Fire a workspace-targeted notification with a string title at three lifecycle moments. | Each call is sequential per orchestrator. The per-call wall-clock timeout from [parent D15](../artifacts/decision-log.md#d15-cmux-api-per-call-timeout-is-fatal) applies unchanged — a timeout is fatal. Other (non-timeout) errors are logged and the run continues ([D7](artifacts/decision-log.md#d7-non-timeout-notification-errors-are-logged-and-the-run-continues-timeouts-remain-fatal-per-parent-d15)). |
| Phase 3 error-mode entry / exit | inbound | The same lifecycle event that switches the footer pane into error mode triggers the error-mode notification's first fire and the re-fire timer; the event that resolves the prompt stops the timer. | The orchestrator must be the single source of truth for "error-mode is active". Phase 5's re-fire timer is keyed on that state, not on a separate notification-internal flag. |
| Phase 4 sidebar updates | parallel (same trigger) | The error-mode entry event fires the notification *and* causes [Phase 4 D13](../phase-4-sidebar-mirroring/artifacts/decision-log.md#d13-error-mode-suffixes-a-stable-marker-onto-the-status-entry-value)'s sidebar pill marker. Resolution clears both. | Notifications and sidebar pushes are independent calls — neither blocks or coalesces with the other ([D14](artifacts/decision-log.md#d14-notifications-and-sidebar-updates-are-independent-side-by-side-effects-of-the-same-event)). |
| Phase 6 failure handling | outbound (delegated) | Phase 6's abort path is responsible for invoking Phase 5's run-aborted notification on display-loss, channel-stall, and pane-close failures. | Phase 5 does not handle those paths itself; it exposes the run-aborted firing through the same orchestrator surface Phase 6 calls ([D16](artifacts/decision-log.md#d16-failure-modes-that-bypass-orchestrator-shutdown-are-phase-6-scope)). |
| On-disk log artifacts | unchanged | Phase 5 introduces no new artifact and writes nothing to disk it would not have written in Phase 2 ([parent D17](../artifacts/decision-log.md#d17-log-file-artifacts-are-unchanged-in-cmux-mode)). | — |

## Out of Scope

- **Per-step and per-iteration notifications.** Already deferred at the parent spec level ([parent D6](../artifacts/decision-log.md#d6-fire-cmux-notifications-at-named-lifecycle-moments)) — would produce notification fatigue. The sidebar pill is the per-step / per-iteration signal ([Phase 4](../phase-4-sidebar-mirroring/feature-specification.md)).
- **Operator-configurable re-fire cadence.** The 60-second cadence is fixed in this release ([D3](artifacts/decision-log.md#d3-error-mode-notification-fires-on-entry-then-re-fires-every-60-seconds)). Same reasoning as [parent D27](../artifacts/decision-log.md#d27-cmux-per-call-timeout-value) for the cmux per-call timeout: exposing a configuration surface invites bikeshedding without evidence anyone wants a different number.
- **Operator-configurable notification icon, color, urgency, or sound.** cmux's notification surface accepts those parameters; pr9k does not surface them in this release because no operator-described need supports it.
- **Sound or vibration control.** Notification chrome (sound, vibration, banner persistence) is owned by cmux and the host operating system; pr9k does not attempt to override.
- **Snooze / mute controls inside pr9k.** The operator can dismiss individual notifications through cmux's chrome; pr9k offers no "silence for N minutes" affordance because the persistent re-fire is the safety net against an unattended error prompt ([D4](artifacts/decision-log.md#d4-dismissal-does-not-suppress-re-fire-pr9k-owns-the-cadence)).
- **Per-pane click targeting.** Activating a notification brings the pr9k workspace into focus; cmux's normal focus rules pick the active pane. Per-pane targeting (e.g., "click on the error-mode notification focuses the footer pane directly") is deferred — see Deferred (YAGNI) below.
- **Notification history surface inside the pr9k workspace.** cmux's notification list is the operator's history. pr9k does not duplicate it in a pane.
- **Mirroring sidebar progress into the notification surface.** Already addressed at the [Phase 4 spec level](../phase-4-sidebar-mirroring/feature-specification.md#deferred-yagni); explicitly out of Phase 5 scope.

## Deferred (YAGNI)

### Per-pane click-to-focus targeting

- **Why deferred:** the build-outline demo step commits only to "click takes the operator to the pr9k workspace"; cmux's normal focus rules pick the active pane inside the workspace, and the error-mode notification text already directs the operator to the footer pane ([parent D19](../artifacts/decision-log.md#d19-error-mode-notifications-direct-the-operator-to-the-control-pane)). Targeting the footer pane directly on click would add an extra cmux v2 method dependency and an extra capability check, for the marginal benefit of saving the operator one focus change.
- **Reopen when:** operators report that the inter-pane focus hop after activating the error-mode notification is friction they would pay an extra dependency to remove, *and* the cmux v2 method for per-surface notification targeting is verified against the pinned cmux source the way [parent D-R2](../artifacts/decision-log.md#d-r2-cmux-v2-protocol-is-the-contract-of-record) verified all other methods.

### Configurable error-mode re-fire cadence

- **Why deferred:** no operator has asked for it. The 60-second cadence ([D3](artifacts/decision-log.md#d3-error-mode-notification-fires-on-entry-then-re-fires-every-60-seconds)) is short enough to surface a forgotten error prompt within a minute and long enough not to spam an operator who is reading the failing step's log. Exposing a setting would invite bikeshedding without evidence anyone wants a different number — the same reasoning [parent D27](../artifacts/decision-log.md#d27-cmux-per-call-timeout-value) applied to the cmux per-call timeout.
- **Reopen when:** operators report that 60 seconds is either spammy or too slow in their specific workflow.

### Distinct notification text per abort sub-reason

- **Why deferred:** the run-aborted notification fires for several sub-paths (operator quit, error-mode quit, cmux timeout, display loss once Phase 6 lands). Distinguishing them in the notification text would let the operator decide whether to investigate from the notification alone. Today the operator gets a single abort line and reads the per-run log file under `<projectDir>/.pr9k/logs/<stamp>/` for details. The simpler version satisfies the same evidence — the operator's primary post-mortem surface remains the log file ([parent D17](../artifacts/decision-log.md#d17-log-file-artifacts-are-unchanged-in-cmux-mode)).
- **Reopen when:** operators report that they routinely need to triage abort cause from the notification chrome without opening the log file — for instance, because they run multiple parallel pr9k sessions and the volume of aborts makes log-diving expensive.

### Notification severity / urgency mapping

- **Why deferred:** cmux's notification surface accepts urgency/severity parameters that influence banner persistence, sound, and operating system surfacing rules. Picking defaults now is symmetry/completeness, which fails the YAGNI evidence test. cmux's default urgency is appropriate for all three Phase 5 notification classes.
- **Reopen when:** operators report that the error-mode notification is too easy to miss in their notification chrome (suggesting it should be higher urgency) or that the completion notification is too intrusive (suggesting it should be lower urgency).

## Open Items

None — every Phase 5 decision is settled either by evidence (parent decisions D6, D19, D-R1, D-R2, D15, D17, D18; Phase 4 D5, D13, D15) or by user judgment captured in [the decision log](artifacts/decision-log.md) (the 60-second cadence, the notification text shape, the workspace-only click target).

## Summary

- **Outcome delivered:** during a pr9k cmux run, cmux's notification surface fires exactly once at successful completion, exactly once at unsuccessful abort, and persistently (every 60 seconds, surviving dismissal) when an error-mode prompt is awaiting the operator's decision. Each notification identifies the target repository in its text; the error-mode notification additionally identifies the failing step and directs the operator to the footer pane.
- **Actor:** the human operator running pr9k in cmux mode.
- **Decisions settled:** see [the decision log](artifacts/decision-log.md) — 9 full decisions (D1–D9) and 7 trivial decisions (D10–D16), of which the build-outline precondition on the re-fire cadence is settled by D3 and the click target is settled by D9.
- **Technical notes captured:** none — every load-bearing mechanic is discoverable from `internal/cmuxctl` once the notification methods are added, and the behavioral commitments stand on their own without a tech-note dependency.
- **Reviewers to consult:** `junior-developer`, `user-experience-designer`, `edge-case-explorer` (see [team-findings.md](artifacts/team-findings.md)).

# Feature Specification: Phase 3 — Interactive Error Recovery

Phase 3 of the [pr9k cmux mode build outline](../build-phase-outline.md). When a workflow step fails in a way that triggers pr9k's existing continue / retry / quit error prompt, the operator sees that prompt inside the cmux footer pane and resolves it from there — with exactly the same semantics as the standard pr9k display — while the header pane marks the failing step and the log pane shows the error. Control keys pressed while a non-control pane is focused are absorbed silently.

This phase makes a cmux-mode run interactively recoverable. Without it, Phase 2's happy-path run cannot survive a single failing step — the operator's only option would be to quit and re-launch. Phase 3 closes that gap by reusing the footer-pane keyboard state machine and the orchestrator-to-footer intent channel that Phase 2 already established; it adds no new cmux calls and no new on-disk artifacts.

Parent artifacts:
- [Parent feature specification](../feature-specification.md)
- [Parent decision log](../artifacts/decision-log.md)
- [Parent feature technical notes](../artifacts/feature-technical-notes.md)

Builds on [Phase 2 — First Real Workflow Runs End-to-End in Cmux](../phase-2-real-workflow-runs/feature-implementation-plan.md): the running workflow, the footer-pane keyboard state machine, and the orchestrator-to-footer intent channel all exist; this phase extends their use to cover the error-mode prompt.

## Outcome

When a workflow step exits non-zero in a way that triggers pr9k's existing error mode — the same condition that triggers it in the standard display, unchanged — the cmux workspace becomes interactively recoverable instead of stranded. The footer pane switches to the continue / retry / quit shortcut hints, the header pane marks the failing step with the same failure indicator the standard display uses ([D2](artifacts/decision-log.md#d2-header-marks-the-failed-step-with-the-standard-failure-indicator)), and the log pane shows the step's error output as ordinary streamed content ([D3](artifacts/decision-log.md#d3-log-pane-shows-error-output-and-retry-separator-as-ordinary-streamed-content)). The operator focuses the footer pane and chooses continue, retry, or quit; the run then proceeds with exactly the same behavior it would have in the standard display — continue advances past the failed step, retry re-runs the step with the same resolved command and prompt and writes a retry separator to the log, and quit goes through the existing two-step confirmation ([D1](artifacts/decision-log.md#d1-error-recovery-reuses-the-inherited-workflow-error-mode-loop-unchanged)). A retry that fails again returns to the same prompt; the workflow stays paused with no auto-timeout until the operator decides. The on-disk log artifacts continue to be produced exactly as Phase 2 produces them; Phase 3 introduces no new artifacts.

Phase 3 deliberately does not add any out-of-workspace signal that an error is waiting. The cmux notification that fires on an error-mode prompt — and its "Focus the pr9k control pane to respond" directive — ships in Phase 5 ([parent D6](../artifacts/decision-log.md#d6-fire-cmux-notifications-at-named-lifecycle-moments), [parent D19](../artifacts/decision-log.md#d19-error-mode-notifications-direct-the-operator-to-the-control-pane)). In Phase 3 the error prompt is visible only inside the footer pane; an operator whose focus is on another pane or another cmux workspace must notice that the run is blocked on their own ([D4](artifacts/decision-log.md#d4-control-keys-on-non-control-panes-are-absorbed-silently-with-no-notification-in-phase-3)).

## Actors and Triggers

- **Primary actor:** the human operator running pr9k in cmux mode against a target repository, from a shell inside a cmux session.
- **Trigger:** a workflow step exits non-zero in a way that triggers pr9k's existing error mode (continue / retry / quit) — the identical trigger condition the standard display uses ([parent D1](../artifacts/decision-log.md#d1-cmux-mode-is-an-alternate-presentation-not-a-new-workflow)). Soft-fail-on-timeout (a step configured `onTimeout: "continue"` whose time cap fires), a user-initiated skip of a running step, and step-preparation failures (e.g., a missing prompt file) do **not** trigger error mode — that inherited behavior is unchanged in cmux mode.
- **Preconditions:**
  - A workflow is running inside a Phase 2 cmux workspace: all Phase 1 and Phase 2 preconditions hold, the four panes are connected, and the readiness handshake has completed ([parent D16](../artifacts/decision-log.md#d16-per-launch-readiness-handshake-before-workflow-starts)).
  - The operator drives pr9k by focusing the footer pane, exactly as in Phase 2 ([parent D8](../artifacts/decision-log.md#d8-footer-pane-owns-keyboard-input)).

## Primary Flow

1. The workflow runs as in Phase 2 until a step exits non-zero in a way that triggers the existing error mode.
2. The orchestrator enters its existing error mode — the same state the standard display enters — pausing the workflow and waiting for the operator's decision. There is no auto-timeout; the run stays paused indefinitely until the operator answers ([D1](artifacts/decision-log.md#d1-error-recovery-reuses-the-inherited-workflow-error-mode-loop-unchanged)).
3. The orchestrator marks the failing step as failed; the header pane reflects the failure indicator on that step's checkbox, identical to the marker the standard display uses ([D2](artifacts/decision-log.md#d2-header-marks-the-failed-step-with-the-standard-failure-indicator)).
4. The failing step's error output appears in the log pane as ordinary streamed content — the same bytes the standard display's log body would show, with no separate error panel or special formatting ([D3](artifacts/decision-log.md#d3-log-pane-shows-error-output-and-retry-separator-as-ordinary-streamed-content)).
5. The footer pane switches to the error-mode shortcut hints (continue / retry / quit) ([parent D20](../artifacts/decision-log.md#d20-keyboard-state-machine-is-owned-by-the-footer-pane)).
6. The operator focuses the footer pane — the only pane that drives pr9k ([parent D8](../artifacts/decision-log.md#d8-footer-pane-owns-keyboard-input)) — and presses continue, retry, or quit. A choice made the instant the prompt appears is not lost ([D1](artifacts/decision-log.md#d1-error-recovery-reuses-the-inherited-workflow-error-mode-loop-unchanged)).
7. The footer forwards the resolved choice to the orchestrator, which acts on it with the same semantics as the standard display: continue advances past the failed step; retry re-runs the step with the same resolved command and prompt and writes a retry separator to the log; quit runs the existing two-step confirmation and then aborts the run ([D1](artifacts/decision-log.md#d1-error-recovery-reuses-the-inherited-workflow-error-mode-loop-unchanged)).
8. On continue or a successful retry, the footer returns to its normal shortcut hints, the header advances past the resolved step, and the workflow proceeds. A retry that fails again re-enters this flow at step 2.
9. The orchestrator writes the same on-disk log artifacts Phase 2 writes — the per-run log file and per-step artifacts under the target repository's standard log directory — unchanged. Phase 3 introduces no new artifacts and no new cmux calls ([parent D17](../artifacts/decision-log.md#d17-log-file-artifacts-are-unchanged-in-cmux-mode)).

## Alternate Flows and States

### Retry fails again

- **Entry condition:** the operator chose retry in the error prompt and the retried step exits non-zero in a way that again triggers error mode.
- **Sequence:** a retry separator was written to the log before the retry began; the retried step's output streams to the log pane; the step exits non-zero again; the orchestrator re-enters error mode; the header re-marks the step as failed; the footer re-shows the continue / retry / quit hints. The behavior is identical to the standard display's retry loop ([D1](artifacts/decision-log.md#d1-error-recovery-reuses-the-inherited-workflow-error-mode-loop-unchanged)).
- **Exit:** the loop continues until the operator chooses continue (advance) or quit (abort), or a retry succeeds.

### Operator chooses quit from the error prompt

- **Entry condition:** the operator focuses the footer pane during an error prompt and presses the quit shortcut.
- **Sequence:** the footer pane runs the existing two-step quit confirmation inline (the same `q` then confirm flow Phase 2 established). If the operator confirms, the footer forwards a quit intent and the orchestrator aborts the run with the same semantics as the standard display. If the operator cancels the confirmation, the footer returns to the error prompt — the continue / retry / quit hints are shown again and the workflow remains paused ([D1](artifacts/decision-log.md#d1-error-recovery-reuses-the-inherited-workflow-error-mode-loop-unchanged), [parent D20](../artifacts/decision-log.md#d20-keyboard-state-machine-is-owned-by-the-footer-pane)).
- **Exit:** on confirmed quit, the run aborts and the workspace remains open for inspection per Phase 2's operator-initiated-closure behavior; on cancel, the run stays in the error prompt.

### Operator presses a control key while a non-control pane is focused

- **Entry condition:** the header pane or the log pane has focus and the operator presses one of the control keys (continue, retry, quit, help, etc.), whether or not the run is currently in error mode.
- **Sequence:** the keystroke is absorbed by the focused pane without effect — no error, no terminal bell, and no notification ([D4](artifacts/decision-log.md#d4-control-keys-on-non-control-panes-are-absorbed-silently-with-no-notification-in-phase-3), [parent D8](../artifacts/decision-log.md#d8-footer-pane-owns-keyboard-input)). If the run is in error mode, it stays in error mode; the footer pane continues to show the error prompt. The log pane still honors its pane-local keys (scrollback) per Phase 2.
- **Exit:** when the operator returns focus to the footer pane, the control shortcuts are live again and the error prompt can be answered.

### Operator's focus is away from the footer when the step fails

- **Entry condition:** a step fails and triggers error mode while the operator's focus is on the header pane, the log pane, or an entirely different cmux workspace.
- **Sequence:** the orchestrator enters error mode and the footer pane switches to the error prompt regardless of which pane has focus (the state push is focus-independent, as in Phase 2). The orchestrator blocks indefinitely — there is no auto-timeout. In Phase 3 there is no notification and no sidebar failure label, so the only indication that the run needs attention is the footer pane's rendered error prompt, which the operator may not currently be looking at ([D4](artifacts/decision-log.md#d4-control-keys-on-non-control-panes-are-absorbed-silently-with-no-notification-in-phase-3)). The operator must notice the stalled run and focus the footer pane to respond. The directing cmux notification that closes this gap ships in Phase 5.
- **Exit:** the operator focuses the footer pane and answers the prompt; the run resumes per the chosen action.

### Operator initiates quit then sends a conflicting intent in rapid succession

- **Entry condition:** during an error prompt the operator presses the quit shortcut and then, before the orchestrator has acted, presses a conflicting key (continue or retry).
- **Sequence:** the footer pane's local keyboard state machine sequences the keystrokes into a single ordered intent stream. The orchestrator processes intents in arrival order and acts on the first quit confirmation it sees; conflicting follow-ups are ignored once shutdown has begun. This is the inherited Phase 2 / parent behavior, unchanged ([parent D20](../artifacts/decision-log.md#d20-keyboard-state-machine-is-owned-by-the-footer-pane)).
- **Exit:** the run aborts on the confirmed quit; the conflicting intent has no effect.

## Edge Cases and Failure Modes

| Condition | Required Behavior |
|-----------|-------------------|
| The operator presses a control key (`q`, `c`, `r`, `?`, etc.) while the header or log pane is focused, in normal or error mode. | The keystroke is absorbed by the focused pane without effect — no error, no terminal bell, no notification. If the orchestrator is in error mode it stays in error mode; the footer pane continues to show the error prompt ([D4](artifacts/decision-log.md#d4-control-keys-on-non-control-panes-are-absorbed-silently-with-no-notification-in-phase-3), [parent D8](../artifacts/decision-log.md#d8-footer-pane-owns-keyboard-input)). |
| A step times out and its config sets `onTimeout: "continue"`. | No error prompt appears. The step soft-fails, a one-line banner is written to the log pane, and the workflow advances — unchanged from the standard display ([parent D1](../artifacts/decision-log.md#d1-cmux-mode-is-an-alternate-presentation-not-a-new-workflow)). |
| The operator initiates a skip of a running step (the existing skip path). | No error prompt appears. The step is treated as a successful user-initiated termination and the workflow advances — unchanged from the standard display. |
| Step preparation fails (e.g., a referenced prompt file does not exist). | No error prompt appears. The preparation error is logged to the log pane and the step is skipped at the orchestrator level — unchanged from the standard display ([parent D1](../artifacts/decision-log.md#d1-cmux-mode-is-an-alternate-presentation-not-a-new-workflow)). |
| The operator presses quit during an error prompt, then presses continue or retry before the orchestrator acts. | The footer sequences the keystrokes into a single ordered intent stream; the orchestrator acts on the first confirmed quit and ignores the conflicting follow-up once shutdown has begun ([parent D20](../artifacts/decision-log.md#d20-keyboard-state-machine-is-owned-by-the-footer-pane)). |
| The operator presses quit during an error prompt and then cancels the quit confirmation (`n` or `esc`). | The footer returns to the error prompt; the continue / retry / quit hints are shown again and the workflow remains paused, identical to the standard display ([D1](artifacts/decision-log.md#d1-error-recovery-reuses-the-inherited-workflow-error-mode-loop-unchanged)). |
| A step fails while the operator's focus is on a non-control pane or a different cmux workspace. | The orchestrator enters error mode and blocks indefinitely (no auto-timeout). The footer pane shows the error prompt, but with no notification and no sidebar label in Phase 3 the operator may not see it; the operator must notice the stalled run and focus the footer pane. The directing notification ships in Phase 5 ([D4](artifacts/decision-log.md#d4-control-keys-on-non-control-panes-are-absorbed-silently-with-no-notification-in-phase-3)). |
| A footer keystroke is pressed in the instant between the error prompt appearing and the orchestrator being ready to accept the choice. | The choice is not lost; the operator's intents are ordered and delivered so the orchestrator acts on the first one — the same race-window handling Phase 2 established for the error mode ([D1](artifacts/decision-log.md#d1-error-recovery-reuses-the-inherited-workflow-error-mode-loop-unchanged)). |
| A display pane or the orchestrator pane is lost, the interaction channel stalls, or a cmux call hangs while the run is in error mode. | Out of scope for Phase 3. Phase 3 does not change Phase 2's behavior for these conditions; generalized failure handling (clean run-abort, "run aborted" notification, sidebar cleanup) ships in Phase 6 ([parent D10](../artifacts/decision-log.md#d10-display-pane-loss-aborts-the-run)). |

## User Interactions

- **Affordances:**
  - **Footer pane (visible, bottom):** the control surface. On a step failure it shows the error-mode shortcut hints (continue / retry / quit). The operator focuses this pane to respond; the quit confirmation renders inline as established in Phase 2 ([parent D20](../artifacts/decision-log.md#d20-keyboard-state-machine-is-owned-by-the-footer-pane)). The persistent hint that shortcuts work only when this pane has focus, established in Phase 2, continues to apply ([parent D8](../artifacts/decision-log.md#d8-footer-pane-owns-keyboard-input)).
  - **Header pane (visible, top):** read-only. The failing step's checkbox shows the same failure indicator the standard display uses; on retry the step returns to its in-progress state when the retry begins, matching the standard display's retry-marking behavior ([D2](artifacts/decision-log.md#d2-header-marks-the-failed-step-with-the-standard-failure-indicator)).
  - **Log pane (visible, middle):** read-only. The failing step's error output and any retry separator appear as ordinary streamed content; pane-local scrollback remains available per Phase 2 ([D3](artifacts/decision-log.md#d3-log-pane-shows-error-output-and-retry-separator-as-ordinary-streamed-content)).
  - **Non-control panes:** the header and log panes absorb control keys without effect; only the footer pane drives pr9k ([D4](artifacts/decision-log.md#d4-control-keys-on-non-control-panes-are-absorbed-silently-with-no-notification-in-phase-3)).
- **Feedback:**
  - On a step failure: the header marks the step failed, the log shows the error, and the footer switches to the error prompt — three simultaneous in-workspace signals.
  - On continue or a successful retry: the footer returns to its normal shortcut hints and the header advances past the resolved step.
  - No notifications and no sidebar failure label are produced in Phase 3 — those are Phases 5 and 4 respectively.
- **Error states:**
  - Step failure: described above — header failure indicator, error output in the log, error-mode hints in the footer.
  - Loss of a display pane, the orchestrator, or the interaction channel during error mode: unchanged from Phase 2; generalized handling ships in Phase 6.

## Coordinations

| Coordinating System | Direction | Interaction | Ordering / Consistency Requirement |
|---------------------|-----------|-------------|-----------------------------------|
| Display panes (header / log / footer) | outbound (state push) | On a step failure the orchestrator pushes the failed-step state to the header, the error output to the log, and the error-mode state to the footer over the existing Phase 2 interaction channel. No new message kinds and no new cmux calls are introduced. | Eventual per-pane consistency, identical to Phase 2's contract; small cross-pane desync at the failure moment is acceptable ([parent D10](../artifacts/decision-log.md#d10-display-pane-loss-aborts-the-run)). |
| Display panes (footer only) | inbound (intent) | The footer pane forwards the operator's continue / retry / quit choice to the orchestrator using the existing Phase 2 intent stream. | Intents are serial and ordered by arrival; the orchestrator acts on them in arrival order and ignores conflicting follow-ups once an intent has been acted on ([parent D20](../artifacts/decision-log.md#d20-keyboard-state-machine-is-owned-by-the-footer-pane)). |
| cmux (the multiplexer) | unchanged | Phase 3 makes no new cmux calls. Notifications and sidebar updates that would accompany an error are deliberately deferred to Phases 5 and 4. | Unchanged from Phase 2. |
| Target repository, `config.json`, `.pr9k/logs/` artifacts | unchanged | Same workflow execution, same step semantics, same on-disk artifacts as Phase 2 — error recovery does not alter what is written ([parent D17](../artifacts/decision-log.md#d17-log-file-artifacts-are-unchanged-in-cmux-mode)). | Unchanged. |

## Out of Scope

- **Error-mode notification.** No cmux notification fires on an error prompt in Phase 3, and the "Focus the pr9k control pane to respond" directive and its persistent re-fire cadence are not present. These ship in Phase 5 ([parent D6](../artifacts/decision-log.md#d6-fire-cmux-notifications-at-named-lifecycle-moments), [parent D19](../artifacts/decision-log.md#d19-error-mode-notifications-direct-the-operator-to-the-control-pane)).
- **Sidebar failure label.** The sidebar status entry is not flipped to a failure label on a step failure; sidebar mirroring (including its behavior at failure) ships in Phase 4.
- **Generalized failure handling.** Display-pane loss, orchestrator-pane loss, interaction-channel stalls, hung cmux calls, and operator pane-close gestures during error mode are handled no differently than in Phase 2; the clean run-abort path with its "run aborted" notification and sidebar cleanup ships in Phase 6 ([parent D10](../artifacts/decision-log.md#d10-display-pane-loss-aborts-the-run)).
- **Any change to the existing error-mode semantics.** The set of recovery choices (continue / retry / quit), the no-auto-timeout behavior, the retry mechanics (same resolved command and prompt, retry separator in the log), and the conditions that do *not* enter error mode (soft-fail-on-timeout, skip, preparation failure) are inherited unchanged. Phase 3 surfaces the existing prompt in cmux; it does not redesign it ([parent D1](../artifacts/decision-log.md#d1-cmux-mode-is-an-alternate-presentation-not-a-new-workflow)).
- **Keyboard-driven log selection.** The standard display's `v` select mode is dropped in cmux mode; cmux's native per-pane mouse selection replaces it. Phase 3 does not reintroduce it ([parent D20](../artifacts/decision-log.md#d20-keyboard-state-machine-is-owned-by-the-footer-pane)).
- **`pr9k workflow` (the workflow builder).** Cmux mode applies to the run loop only; the workflow builder is unaffected ([parent D9](../artifacts/decision-log.md#d9-cmux-mode-applies-to-the-run-loop-only-not-the-workflow-builder)).

## Open Items

- **OI-1: CI testing strategy for cmux mode (inherited, non-blocking).** cmux is a graphical application; the standard pr9k CI runners cannot launch it, so Phase 3's error-recovery path — like Phases 1 and 2 — cannot be exercised end-to-end against a live cmux in CI. The parent build outline's recommendation is to mock cmux's programmatic interface for Phases 1 through 5 and revisit for Phase 6. Tracked in the parent build outline as [OQ-2](../build-phase-outline.md#oq-2) and in Phase 1 as OI-2.
  - **Resolves when:** the team commits to a testing approach for cmux mode (the recommendation stands: mock the cmux interface for Phase 3; the footer state machine, key adapter, and orchestrator error path are unit-testable with the existing Phase 2 test doubles without a live cmux).
  - **Blocks implementation:** No — Phase 3 can be implemented and manually demoed without a live-cmux CI strategy; the durability of the Phase 3 test suite depends on the mocked approach already used for Phase 2.

## Summary

- **Outcome delivered:** a failing workflow step in cmux mode is interactively recoverable from inside the workspace — the footer pane shows the continue / retry / quit prompt, the header marks the failed step, the log shows the error, and the operator resolves it with the same semantics as the standard display — with control keys on non-control panes absorbed silently.
- **Primary actors:** the human operator running pr9k in cmux mode against a target repository from inside a cmux session.
- **Decisions settled by evidence:** 4 — see [artifacts/decision-log.md](artifacts/decision-log.md). (Parent decisions D1, D6, D8, D9, D10, D16, D17, D19, D20 are inherited unchanged.)
- **Decisions settled by user input:** 0 — every Phase 3 decision was resolved against evidence (the parent spec/decision-log, the build-phase outline, the implemented Phase 1/Phase 2 infrastructure, and existing error-recovery documentation) — see [artifacts/decision-log.md](artifacts/decision-log.md).
- **Sub-agents consulted:** _(pending review pass — see [artifacts/team-findings.md](artifacts/team-findings.md))_
- **Key adjustments from review:** _(pending review pass — see [artifacts/team-findings.md](artifacts/team-findings.md))_
- **Remaining open items:** 1 (inherited CI testing strategy for cmux mode; non-blocking for the implementation).

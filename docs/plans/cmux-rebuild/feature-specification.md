# Feature Specification: pr9k Cmux Mode

A new opt-in launch mode in which pr9k drives [cmux](https://cmux.com/) to render its workflow run UI as a native cmux workspace with three coordinated panes — a step header, a streaming log, and a control footer — instead of drawing a single-process TUI in the user's terminal. See [investigation.md](investigation.md) for the prior analysis that ruled out embedding cmux inside pr9k and motivated this inverted shape.

## Outcome

When the operator launches pr9k in cmux mode against a target repository, a fresh cmux workspace appears containing three panes laid out top-to-bottom: a small **header pane** showing per-step checkbox progress and the current iteration counter; a tall **log pane** showing streaming output from the current step; and a small **footer pane** showing the status line and keyboard shortcuts. A fourth pane — the **orchestrator pane** — is created but kept hidden by default; it hosts the workflow engine and is the diagnostic surface when something goes wrong ([D13](artifacts/decision-log.md#d13-orchestrator-runs-in-a-hidden-cmux-pane)). Throughout the run, cmux's native sidebar reflects the current step name as a status entry and the iteration counter as a progress entry, and cmux notifications fire on completion, failure, and error-recovery prompts. The operator also keeps the existing per-run log artifacts — `.pr9k/logs/<run>.log` and the per-step `.jsonl` files — unchanged in the target repository, regardless of which presentation mode they launched ([D17](artifacts/decision-log.md#d17-log-file-artifacts-are-unchanged-in-cmux-mode)).

When the run ends — whether by successful completion, error escalation, or operator quit — the workspace remains open until the operator explicitly dismisses it, matching the existing TUI's "done state" behavior ([D14](artifacts/decision-log.md#d14-workspace-closure-is-operator-initiated)). The outcome is functionally equivalent to today's single-window TUI: same step sequencing, same iteration semantics, same log content, same error handling, same exit codes. What changes is the *presentation surface* — cmux owns the chrome (borders, splits, focus, sidebar, notifications, native selection) instead of pr9k drawing it ([D1](artifacts/decision-log.md#d1-cmux-mode-is-an-alternate-presentation-not-a-new-workflow)).

## Actors and Triggers

- **Primary actor:** the human operator running pr9k against a target repository.
- **Trigger:** the operator launches pr9k with the `--cmux` flag on the existing run invocation ([D25](artifacts/decision-log.md#d25-launch-surface-is-the-cmux-flag); the opt-in nature itself is committed in [D2](artifacts/decision-log.md#d2-cmux-mode-is-opt-in-at-launch)).
- **Preconditions:**
  - cmux is installed on the host and its programmatic interface is reachable ([T1](artifacts/feature-technical-notes.md#t1-cmux-programmatic-interface-shape)).
  - The pr9k invocation is itself running inside a cmux session so it is permitted to connect to cmux's interface ([T2](artifacts/feature-technical-notes.md#t2-cmux-access-model)).
  - cmux is reachable, responsive, and exposes the API capabilities pr9k needs ([D18](artifacts/decision-log.md#d18-startup-capability-check)).
  - All existing preconditions for a pr9k run still apply (target repository present, Docker available for claude steps, `config.json` present, etc.).

## Primary Flow

1. The operator launches pr9k in cmux mode, pointing at a target repository.
2. pr9k records which cmux workspace the operator was in before launch so that workspace can be returned to focus on cleanup ([D22](artifacts/decision-log.md#d22-prior-workspace-is-captured-and-restored)).
3. pr9k probes cmux: confirms it is reachable, that pr9k is permitted to drive it, and that the API exposes every method pr9k will call ([D18](artifacts/decision-log.md#d18-startup-capability-check)). If any check fails, pr9k aborts with a precondition error that names the specific failure condition (see [Edge Cases](#edge-cases-and-failure-modes) for the four distinguishable cases) ([D3](artifacts/decision-log.md#d3-cmux-availability-is-a-hard-precondition-not-a-fallback)).
4. pr9k counts how many existing cmux workspaces still carry pr9k's `pr9k-` naming prefix from prior runs; if any remain, pr9k prints a one-line advisory listing the orphan workspace names and continues without waiting for acknowledgement ([D28](artifacts/decision-log.md#d28-orphan-advisory-fires-when-any-orphan-exists)).
5. pr9k asks cmux to create a fresh workspace whose name follows the `pr9k-<repo-basename>-<nanosecond-timestamp>` pattern, ensuring two concurrent or rapid launches do not collide and the operator can later distinguish workspaces ([D29](artifacts/decision-log.md#d29-workspace-name-pattern)). pr9k prints the workspace name to the launching terminal so the operator has a paper trail.
6. pr9k asks cmux to lay out four panes in the new workspace — the orchestrator pane (kept hidden), the header pane on top, the log pane in the middle, the footer pane at the bottom — and to spawn one pr9k process inside each pane ([D4](artifacts/decision-log.md#d4-three-pane-vertical-layout), [D13](artifacts/decision-log.md#d13-orchestrator-runs-in-a-hidden-cmux-pane)). The three visible panes are focused display renderers; the hidden pane is the workflow orchestrator.
7. The orchestrator pane waits for all three display panes to signal readiness before starting the workflow, so the first step's state reaches the operator's screen without dropped events ([D16](artifacts/decision-log.md#d16-per-launch-readiness-handshake-before-workflow-starts)).
8. The orchestrator runs the configured workflow (the existing `initialize` / `iteration` / `finalize` phase semantics from the narrow-reading principle apply unchanged).
9. As the orchestrator advances, it publishes step-state and log-line updates to the three display panes over a local interaction channel ([T3](artifacts/feature-technical-notes.md#t3-cmux-surfaces-are-processes-not-paintable-canvases), [T4](artifacts/feature-technical-notes.md#t4-display-processes-update-independently)). The header pane redraws on each step-state change; the log pane appends streamed bytes; the footer pane updates the status line on its existing cadence and shows the current shortcut hints.
10. In parallel, the orchestrator pushes the current step name as a cmux **sidebar status entry** (a single-line label that appears in cmux's persistent sidebar against this workspace) and the iteration counter as a **sidebar progress entry** (a progress indicator with a numeric `N / M` form). The two entries update on the same cadence as the header pane so the operator can monitor run state from outside the workspace ([D5](artifacts/decision-log.md#d5-mirror-key-state-into-cmux-sidebar)).
11. When a Claude step finishes, when the full run finishes, or when an error is escalated to the operator, the orchestrator fires a cmux **notification**. Error-mode notifications include the text "Focus the pr9k control pane to respond" so an operator whose focus is elsewhere has a clear next step ([D6](artifacts/decision-log.md#d6-fire-cmux-notifications-at-named-lifecycle-moments), [D19](artifacts/decision-log.md#d19-error-mode-notifications-direct-the-operator-to-the-control-pane)).
12. The orchestrator also writes the same on-disk artifacts pr9k always produces: the per-run log file under the target repository's `.pr9k/logs/` directory, the per-step `.jsonl` files, the iteration log. These are unchanged in cmux mode and remain the primary post-mortem surface ([D17](artifacts/decision-log.md#d17-log-file-artifacts-are-unchanged-in-cmux-mode)).
13. When the run completes (or aborts), the orchestrator marks the workspace as done, clears sidebar entries, and waits — the workspace stays open until the operator explicitly closes it via the footer pane or via cmux's own workspace controls ([D14](artifacts/decision-log.md#d14-workspace-closure-is-operator-initiated)). On dismissal, pr9k restores the operator's prior workspace to focus.

## Alternate Flows and States

### Operator quits mid-run

- **Entry condition:** operator focuses the footer pane and presses the quit shortcut, or sends SIGINT/SIGTERM to the pr9k process tree.
- **Sequence:** the footer pane forwards a quit-requested intent to the orchestrator. The orchestrator confirms with the operator (existing two-step `q`/`y` confirmation, rendered inside the footer pane), then aborts the current step, fires a "run quit" notification, clears sidebar entries, and transitions the workspace to its done state. The workspace remains open per [D14](artifacts/decision-log.md#d14-workspace-closure-is-operator-initiated) so the operator can review the final screen.
- **Exit:** the operator dismisses the workspace when finished; the prior workspace is restored.

### Step fails and prompts for operator decision

- **Entry condition:** a step exits non-zero in a way that triggers pr9k's existing error mode (continue / retry / quit).
- **Sequence:** the orchestrator pushes the error state to the header (which marks the current step as failed), to the log (which shows the error), and to the footer (which switches to the error-mode shortcut hints). A cmux notification fires with the "focus the pr9k control pane" directive ([D19](artifacts/decision-log.md#d19-error-mode-notifications-direct-the-operator-to-the-control-pane)). The footer pane awaits the operator's `c` / `r` / `q` choice and forwards the intent to the orchestrator. If the operator has focus on another pane, the orchestrator stays blocked in error mode indefinitely — there is no auto-timeout — and the notification persists ([D20](artifacts/decision-log.md#d20-keyboard-state-machine-is-owned-by-the-footer-pane)).
- **Exit:** orchestrator continues, retries, or quits per the operator's choice — same semantics as today.

### Skip current step

- **Entry condition:** operator focuses the footer pane during a running step and presses the skip shortcut.
- **Sequence:** the footer pane forwards a skip-step intent to the orchestrator, which terminates the current step's subprocess and treats the step as completed-with-skip per the existing workflow runner semantics. The next step (or the next phase) begins. The header, log, and footer redraw accordingly ([D20](artifacts/decision-log.md#d20-keyboard-state-machine-is-owned-by-the-footer-pane)).
- **Exit:** workflow continues.

### Operator resizes a cmux pane

- **Entry condition:** the operator drags a pane border in cmux to resize the header, log, or footer pane.
- **Sequence:** the affected display pane receives one or more window-size updates and coalesces them so the redraw runs at most once per short interval; visible content re-wraps to fit. Transient display artifacts (mid-drag flicker, partial wraps) are acceptable. Other panes are unaffected and the orchestrator is not involved.
- **Exit:** the pane settles at its new dimensions; orchestrator state is untouched.

### Operator focuses a non-control pane

- **Entry condition:** the operator clicks or keybinds focus to the header pane or the log pane.
- **Sequence:** keystrokes go to whichever pane has focus. Only the footer pane forwards control intents to the orchestrator. Header and log panes accept pane-local keys (scrollback in the log) and ignore control shortcuts. Cmux's native per-pane mouse selection is the path for copying content out of any pane ([D8](artifacts/decision-log.md#d8-footer-pane-owns-keyboard-input), [D12](artifacts/decision-log.md#d12-drop-cross-pane-mouse-selection)).
- **Exit:** when the operator returns focus to the footer, the global shortcut surface is again live.

### Help is requested

- **Entry condition:** operator focuses the footer pane and presses the help shortcut.
- **Sequence:** the footer pane shows the help modal inline (same content the existing TUI shows). Unlike the existing TUI, help is available regardless of whether a status-line script is configured. The orchestrator is not involved ([D20](artifacts/decision-log.md#d20-keyboard-state-machine-is-owned-by-the-footer-pane)).
- **Exit:** the operator dismisses the modal; the footer returns to its prior state.

## Edge Cases and Failure Modes

| Condition | Required Behavior |
|-----------|-------------------|
| cmux is not installed on the host. | pr9k aborts before creating any workspace, prints "cmux is not installed" with a pointer to the cmux setup how-to, and exits non-zero ([D3](artifacts/decision-log.md#d3-cmux-availability-is-a-hard-precondition-not-a-fallback)). |
| cmux is installed but its socket is not reachable (cmux is not running). | pr9k aborts and prints "cmux is installed but not running; start cmux and try again." |
| cmux is running but pr9k is not a descendant of a cmux process (default access mode rejects it). | pr9k aborts and prints "cmux mode must be launched from inside a cmux session" plus the offending socket path ([T2](artifacts/feature-technical-notes.md#t2-cmux-access-model)). |
| cmux is reachable but the socket is explicitly disabled by configuration (`Off` access mode). | pr9k aborts and prints "cmux socket is disabled in cmux configuration; re-enable it and try again." |
| cmux is reachable but does not expose every API method pr9k requires (version skew, breaking change). | pr9k aborts and prints "cmux version is incompatible with pr9k cmux mode" plus the missing method names, so the operator knows what to update ([D18](artifacts/decision-log.md#d18-startup-capability-check)). |
| A cmux API call has not returned a response within the per-call timeout (cmux is hung or otherwise unresponsive). | pr9k treats the timeout as equivalent to display-process loss: aborts the run, fires a "run aborted" notification, marks the workspace as failed, and exits non-zero. The timeout is fixed in the 5–10 second range and not operator-configurable in the initial release ([D15](artifacts/decision-log.md#d15-cmux-api-per-call-timeout-is-fatal), [D27](artifacts/decision-log.md#d27-cmux-per-call-timeout-value), [T1](artifacts/feature-technical-notes.md#t1-cmux-programmatic-interface-shape)). The operator's prior workspace is restored on dismissal. |
| cmux's API rejects a request mid-flow (workspace cannot be created, pane cannot be split, display process cannot be spawned). | pr9k tears down anything it already created in cmux, prints the failing call's diagnostic, and exits with a non-zero status. The operator's prior cmux session is restored without surprises. |
| A display pane dies while the orchestrator is still running. | The orchestrator finishes the cmux API call it is currently making (so cmux is left in a consistent state), then aborts the run, fires a "run aborted" notification, marks the workspace as failed, and exits non-zero ([D10](artifacts/decision-log.md#d10-display-process-loss-aborts-the-run)). |
| The operator closes one of the pr9k panes using cmux's own close-pane gesture. | Treated identically to display-process loss: the run aborts. The cmux setup how-to and the in-run help modal both call this out as a known constraint of cmux mode: closing a pane closes the run ([D24](artifacts/decision-log.md#d24-operator-pane-close-is-treated-as-display-loss)). |
| The orchestrator pane dies but display panes are still alive. | Each display pane detects the loss of the orchestrator's interaction channel within the configured stall threshold, renders a "run aborted" line on its own pane, and exits. The workspace stays open so the operator can read the residual state and can examine the orchestrator pane (cmux retains its exit status) ([D13](artifacts/decision-log.md#d13-orchestrator-runs-in-a-hidden-cmux-pane), [T5](artifacts/feature-technical-notes.md#t5-cmux-workspace-persists-after-pane-exit)). |
| The local interaction channel between orchestrator and a display pane stops delivering events (channel full, socket error, but processes still alive). | Treated identically to display-process loss after the configured stall threshold. ([D10](artifacts/decision-log.md#d10-display-process-loss-aborts-the-run)) |
| The orchestrator is killed with SIGKILL (cannot run cleanup). | No cleanup runs. Display panes detect the loss via the channel and exit. The workspace persists as an orphan; the prior workspace is *not* restored automatically. The operator dismisses the workspace manually. The per-run log file may be truncated short of the SIGKILL moment but the prior content is intact ([D17](artifacts/decision-log.md#d17-log-file-artifacts-are-unchanged-in-cmux-mode)). |
| Operator initiates a `q` quit and then sends a conflicting intent (`c` continue, `r` retry) in rapid succession. | The footer pane's local keyboard state machine sequences the operator's keystrokes into a single ordered intent stream sent to the orchestrator. The orchestrator processes intents in arrival order and acts on the first quit confirmation it sees; subsequent conflicting intents are ignored once the orchestrator has begun shutdown ([D20](artifacts/decision-log.md#d20-keyboard-state-machine-is-owned-by-the-footer-pane)). |
| Operator presses a control key (`q`, `c`, `r`, `?`, etc.) while focus is on the header or log pane. | The keystroke is absorbed by the focused pane without effect. No error, no notification. If the orchestrator is in error mode awaiting input, the persistent error-mode notification continues to direct the operator to the footer pane ([D19](artifacts/decision-log.md#d19-error-mode-notifications-direct-the-operator-to-the-control-pane)). |
| The two-step `q`/`y` quit confirmation is initiated but the operator does nothing for a long period. | Existing pr9k behavior carries over — no auto-timeout; pr9k continues running until the operator confirms or cancels. |
| The operator launches pr9k cmux mode while a prior pr9k cmux workspace from a crashed run still exists. | A fresh workspace is created (uniquely named per [D21](artifacts/decision-log.md#d21-workspace-name-format)). The startup advisory tells the operator how many orphan workspaces remain ([D23](artifacts/decision-log.md#d23-orphan-workspace-startup-advisory)). |

## User Interactions

- **Affordances:**
  - **Header pane:** read-only display of step checkboxes and iteration counter.
  - **Log pane:** read-only display of streamed output. Pane-local scrollback is available when the operator focuses this pane; the operator can scroll back through the same volume of output the standard TUI retains. Keyboard-driven text selection (the standard TUI's `v` select mode) is *not* available in cmux mode — cmux's native per-pane mouse selection is the path for copying content out ([D12](artifacts/decision-log.md#d12-drop-cross-pane-mouse-selection), [D20](artifacts/decision-log.md#d20-keyboard-state-machine-is-owned-by-the-footer-pane)).
  - **Footer pane:** the control surface. Same key bindings as today's single-pane TUI footer: quit, continue, retry, skip step, help. The operator focuses this pane to drive pr9k. A persistent hint in the footer reminds the operator that shortcuts only work when this pane has focus ([D8](artifacts/decision-log.md#d8-footer-pane-owns-keyboard-input)).
  - **Orchestrator pane:** hidden by default. The operator can reveal it via cmux's own pane-show controls if they want to inspect orchestrator state during or after a run. When the orchestrator exits (successfully or by crash), this pane shows the exit status and the last lines of orchestrator output, which is the diagnostic surface for unexpected failures ([D13](artifacts/decision-log.md#d13-orchestrator-runs-in-a-hidden-cmux-pane)).
  - **Cmux sidebar:** a passive surface showing one status entry (the current step name) and one progress entry (the iteration counter `N / M`) for this workspace. Visible from any cmux workspace; updates as the run advances.
  - **Cmux notifications:** alert-on-event surface for completion, failure, and error-prompt moments. Notifications can be dismissed individually; the run's error-mode prompt re-fires the notification on a regular cadence until the operator answers it ([D19](artifacts/decision-log.md#d19-error-mode-notifications-direct-the-operator-to-the-control-pane)).

- **Feedback:**
  - Step state changes are visible in the header pane and in the sidebar status entry simultaneously (with the small cross-pane desync per [T4](artifacts/feature-technical-notes.md#t4-display-processes-update-independently)).
  - Each new log line appears in the log pane within the existing streaming latency budget.
  - Status-line script output appears in the footer pane on its existing cadence.
  - Error and quit-confirmation prompts appear inside the footer pane and as cmux notifications.

- **Error states:**
  - Step failure: header marks the failing step; log shows the error; footer surfaces the error-mode shortcut hints; sidebar status flips to a failure label; a notification fires.
  - Loss of any display, the orchestrator, or the interaction channel: workspace transitions to a failed state; a "run aborted" notification fires; the workspace stays open for inspection.
  - Loss of cmux itself or its API: pr9k exits with a clear error; nothing in cmux is half-built.

## Coordinations

| Coordinating System | Direction | Interaction | Ordering / Consistency Requirement |
|---------------------|-----------|-------------|-----------------------------------|
| cmux (the multiplexer) | outbound | Workspace creation, pane splitting, spawning processes, sidebar status / progress updates, notifications, workspace focus changes, workspace closure. | Calls are sequential per workspace lifecycle. Each call has a configured per-call wall-clock timeout; on timeout the run aborts ([D15](artifacts/decision-log.md#d15-cmux-api-per-call-timeout-is-fatal)). Sidebar updates may coalesce (latest wins) without affecting correctness ([T1](artifacts/feature-technical-notes.md#t1-cmux-programmatic-interface-shape)). |
| Display panes (header / log / footer) | outbound (state push) | The orchestrator pushes step-state updates, log lines, status-line output, and error states. | The behavioral guarantee is *eventual* per-pane consistency — each pane converges to the orchestrator's latest state. Small cross-pane desynchronization (a few milliseconds) at moments like "iteration ticks over" is acceptable ([T3](artifacts/feature-technical-notes.md#t3-cmux-surfaces-are-processes-not-paintable-canvases), [T4](artifacts/feature-technical-notes.md#t4-display-processes-update-independently)). |
| Display panes (footer only) | inbound (intent) | The footer pane forwards quit / continue / retry / skip / help intents from the operator. | Intents are serial and ordered by arrival; the orchestrator acts on them in arrival order and ignores conflicting follow-ups once an intent has been acted on. |
| Local interaction channel (orchestrator ↔ display panes) | bidirectional | Carries state pushes from orchestrator to displays and intents from the footer to the orchestrator. Detects orchestrator and display loss via stall threshold. | Time-bounded: a pane that has not received an expected message within the stall threshold treats the channel as lost and exits; the orchestrator does the same in reverse for the footer's intent stream. |
| Target repository | unchanged | Same git / gh / docker / claude interactions as today. | Unchanged. |
| Workflow `config.json` | unchanged | Same step definitions, same phase semantics, same variable substitution. | Unchanged. |
| `.pr9k/logs/` and per-step `.jsonl` artifacts | outbound (file writes) | The orchestrator writes the per-run log file and the per-step artifacts as in the standard TUI. The orchestrator's working directory is the target repository so the artifact paths match the standard TUI's paths exactly. | Unchanged ([D17](artifacts/decision-log.md#d17-log-file-artifacts-are-unchanged-in-cmux-mode)). |

## Out of Scope

- **`pr9k workflow` (the workflow builder).** Cmux mode applies only to the run loop. The workflow builder retains its existing single-window TUI. ([D9](artifacts/decision-log.md#d9-cmux-mode-applies-to-the-run-loop-only-not-the-workflow-builder))
- **Default behavior.** Cmux mode does not replace the existing standard-terminal TUI; it is an opt-in alternate launch mode. The standard TUI continues to work in any terminal. ([D2](artifacts/decision-log.md#d2-cmux-mode-is-opt-in-at-launch))
- **Cmux-aware status line on the standard TUI.** The "smaller starter step" sketched in the investigation — keeping today's TUI but pushing sidebar entries when pr9k happens to run inside cmux — is *not* this feature. It is a competing approach and is deferred until the larger architecture proves itself or is dropped.
- **A cmux pane for the resulting PR / browser integration.** cmux has a browser surface; using it for the resulting PR is a follow-on enhancement, not in this feature.
- **Cross-pane mouse drag selection.** ([D12](artifacts/decision-log.md#d12-drop-cross-pane-mouse-selection))
- **Keyboard-driven log selection (`v` select mode).** The standard TUI's keyboard select mode is dropped in cmux mode; cmux's native per-pane mouse selection replaces it ([D20](artifacts/decision-log.md#d20-keyboard-state-machine-is-owned-by-the-footer-pane)).
- **Reattaching to a prior workspace after pr9k restarts.** Each invocation creates a fresh workspace; old ones are not adopted. ([D11](artifacts/decision-log.md#d11-orphan-workspaces-from-crashes-are-not-auto-cleaned))
- **Fallback to standard TUI when cmux is unavailable.** ([D3](artifacts/decision-log.md#d3-cmux-availability-is-a-hard-precondition-not-a-fallback))
- **Automatic cleanup of orphan workspaces.** ([D11](artifacts/decision-log.md#d11-orphan-workspaces-from-crashes-are-not-auto-cleaned), [D23](artifacts/decision-log.md#d23-orphan-workspace-startup-advisory))

## Deferred (YAGNI)

### Sidebar mirroring of the full status line

- **Why deferred:** the simpler-version test prefers running the existing status-line script in the footer pane unchanged. Routing the status line into cmux's sidebar would duplicate the information without observable benefit until the footer pane is itself dropped.
- **Reopen when:** the operator regularly wants pr9k's status while pr9k's workspace is not in focus, *beyond* what the existing step / iteration sidebar entries already provide.
- **Source:** investigation.md Round 2.

### Auto-cleanup of orphaned workspaces from prior crashed runs

- **Why deferred:** evidence test fails. The startup advisory ([D23](artifacts/decision-log.md#d23-orphan-workspace-startup-advisory)) gives the operator visibility into accumulation; manual dismissal is straightforward; auto-cleanup would discard forensic evidence the operator might want.
- **Reopen when:** operator reports orphan-workspace accumulation as a real problem, or a documented incident shows an orphan workspace caused confusion.
- **Source:** spec interview; reinforced by review-team finding on accumulation visibility.

### Distributed keyboard control (every pane forwards its own intents)

- **Why deferred:** the simpler-version test prefers a single control pane. The distributed model would require re-implementing the existing keyboard state machine as a distributed protocol with no observable user benefit.
- **Reopen when:** operators report the single-control-pane model is materially worse than expected (specifically: they regularly try to drive pr9k from a non-control pane and find the experience worse than tmux-style "focus the pane to drive its program").
- **Source:** investigation.md Round 2; affirmed in [D20](artifacts/decision-log.md#d20-keyboard-state-machine-is-owned-by-the-footer-pane).

### A cmux pane for the resulting PR / Playwright browser surface

- **Why deferred:** evidence test fails. Speculative use of a cmux capability with no named user need.
- **Reopen when:** the operator names a concrete PR-review-in-cmux workflow they want from inside pr9k.
- **Source:** investigation.md Round 2.

## Open Items

None. All open items from the initial draft have been settled — the original OI-1 (orchestrator location) and OI-5 (launch ancestry) were closed by the review pass into [D13](artifacts/decision-log.md#d13-orchestrator-runs-in-a-hidden-cmux-pane); the five operator-input items that remained after review (launch flag, status-line routing, timeout value, orphan-advisory threshold, workspace-name pattern) are now committed as [D25](artifacts/decision-log.md#d25-launch-surface-is-the-cmux-flag) through [D29](artifacts/decision-log.md#d29-workspace-name-pattern).

## Summary

- **Outcome delivered:** an opt-in launch mode that renders pr9k's run UI as a native cmux workspace with header, log, footer, and hidden-orchestrator panes, plus sidebar and notification integration.
- **Primary actors:** the human operator running pr9k against a target repository.
- **Decisions settled by evidence:** 24 — see [artifacts/decision-log.md](artifacts/decision-log.md)
- **Decisions settled by user input:** 5 (D25–D29; operator accepted the spec's recommended provisional answers for every open item) — see [artifacts/decision-log.md](artifacts/decision-log.md)
- **Sub-agents consulted:** junior-developer, devops-engineer, edge-case-explorer — see [artifacts/team-findings.md](artifacts/team-findings.md)
- **Key adjustments from review:** orchestrator moved into a hidden cmux pane (closes original OI-1); workspace stays open until operator dismisses it (replaces grace period); explicit per-call cmux timeout, capability check, IPC stall threshold, and SIGKILL behavior added; 7-of-8 keyboard modes survive (the `v` select mode is dropped); workspace name format and orphan-workspace advisory added; log-file artifacts explicitly preserved; cmux pane-close gesture treated as display loss with documentation called out. — see [artifacts/team-findings.md](artifacts/team-findings.md)
- **Remaining open items:** 0
- **Technical notes:** 5 — see [artifacts/feature-technical-notes.md](artifacts/feature-technical-notes.md)

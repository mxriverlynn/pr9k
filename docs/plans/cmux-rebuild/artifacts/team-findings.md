# Team Findings: pr9k Cmux Mode

Review-team findings raised against the initial spec draft, and how each was resolved. Behavioral outcomes live in [../feature-specification.md](../feature-specification.md); decisions affected live in [decision-log.md](decision-log.md); load-bearing mechanics affected live in [feature-technical-notes.md](feature-technical-notes.md).

Three review agents were dispatched in parallel against the initial spec draft (D1–D12, T1–T4): `junior-developer` (generalist stress-test), `devops-engineer` (production-readiness), `edge-case-explorer` (boundary / failure exploration). The findings below are the consolidated, deduplicated set.

## Major findings

### F1: Four distinguishable cmux-unreachable conditions need distinct error messages

- **Agent:** junior-developer (F-15), edge-case-explorer (F-9)
- **Finding:** the original spec collapsed "cmux not installed," "cmux not running," "pr9k not a cmux child," and "cmux socket disabled by configuration" into a single edge-case row with a single error message. Each condition has a different corrective action, so the operator needs to know which one fired.
- **Resolution:** [D3](decision-log.md#d3-cmux-availability-is-a-hard-precondition-not-a-fallback) updated to require distinguishable error messages; the [Edge Cases and Failure Modes](../feature-specification.md#edge-cases-and-failure-modes) table now has four separate rows, one per condition, each with its own required error text.
- **Resolved by:** evidence
- **Affected decisions:** D3
- **Affected tech-notes:** —
- **Changed in spec:** Edge Cases and Failure Modes

### F2: Sidebar entries were used as if standard cmux vocabulary the operator already knows

- **Agent:** junior-developer (F-05)
- **Finding:** the original spec used "sidebar status entry" and "sidebar progress entry" without explaining what the operator actually sees in cmux. A reader of the spec alone could not picture the result.
- **Resolution:** [D5](decision-log.md#d5-mirror-key-state-into-cmux-sidebar) extended to call out the form (single-line label per workspace, progress in `N / M` form); [Primary Flow](../feature-specification.md#primary-flow) step 10 and [User Interactions](../feature-specification.md#user-interactions) describe both entries' visual form and where they appear in cmux.
- **Resolved by:** evidence
- **Affected decisions:** D5
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow, User Interactions

### F3: Error-mode notifications must persist to avoid stranding a blocked orchestrator

- **Agent:** edge-case-explorer (F-4)
- **Finding:** if the operator's focus is on a non-control pane when an error-mode prompt fires and they dismiss the one-shot notification, the orchestrator is silently stranded blocking on operator input.
- **Resolution:** [D6](decision-log.md#d6-fire-cmux-notifications-at-named-lifecycle-moments) and [D19](decision-log.md#d19-error-mode-notifications-direct-the-operator-to-the-control-pane) commit to persistent, re-firing notifications for the error-mode prompt with explicit "focus the pr9k control pane" directive text. The [Alternate Flows](../feature-specification.md#alternate-flows-and-states) "Step fails and prompts for operator decision" entry now states the indefinite block and the persistent-notification behavior.
- **Resolved by:** evidence
- **Affected decisions:** D6, D19
- **Affected tech-notes:** —
- **Changed in spec:** Alternate Flows and States, User Interactions, Primary Flow

### F4: Non-control-pane keystrokes have no behavioral commitment

- **Agent:** edge-case-explorer (F-3)
- **Finding:** the original spec said the footer pane is the sole intent forwarder but never said what the operator observes when a control key (`q`, `c`, `r`, `?`) is pressed on a non-control pane.
- **Resolution:** [D20](decision-log.md#d20-keyboard-state-machine-is-owned-by-the-footer-pane) and the [Edge Cases](../feature-specification.md#edge-cases-and-failure-modes) table commit to "absorbed by the focused pane without effect, no error, no notification." The persistent error-mode notification from F3 covers the case where the operator is actively blocked.
- **Resolved by:** evidence
- **Affected decisions:** D20
- **Affected tech-notes:** —
- **Changed in spec:** Edge Cases and Failure Modes, Alternate Flows and States

### F5: Display loss vs in-flight cmux call ordering was undefined

- **Agent:** edge-case-explorer (F-1)
- **Finding:** [D10](decision-log.md#d10-display-pane-loss-aborts-the-run) commits to "shut down the workspace" on display loss, but doing so requires a cmux API call. If a display dies while a sidebar/notification call is in flight, the spec did not say whether the call completes first or is cancelled.
- **Resolution:** the [Edge Cases](../feature-specification.md#edge-cases-and-failure-modes) "display pane dies" row commits to "finishes the cmux API call it is currently making, then aborts." [D10](decision-log.md#d10-display-pane-loss-aborts-the-run) rationale extended.
- **Resolved by:** evidence
- **Affected decisions:** D10
- **Affected tech-notes:** —
- **Changed in spec:** Edge Cases and Failure Modes

### F6: Orphan workspace accumulation needed operator visibility, not auto-cleanup

- **Agent:** devops-engineer (F-6)
- **Finding:** the original spec deferred auto-cleanup but also deferred *awareness* — operators would accumulate orphans across crashes with no signal.
- **Resolution:** new [D23](decision-log.md#d23-orphan-workspace-startup-advisory) commits to a one-line startup advisory listing orphan workspace names. Auto-cleanup remains deferred per YAGNI but visibility is now in place. [Primary Flow](../feature-specification.md#primary-flow) step 4 and the orphan edge-case row updated.
- **Resolved by:** evidence
- **Affected decisions:** D23, D11
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow, Edge Cases and Failure Modes, Out of Scope, Deferred (YAGNI)

### F7: Keyboard `v` select mode is redundant in cmux mode

- **Agent:** edge-case-explorer (F-10)
- **Finding:** [D12](decision-log.md#d12-drop-cross-pane-and-keyboard-driven-selection) dropped the custom mouse-drag selection layer but the spec did not say what happens to the keyboard `v` select mode, which the standard TUI offers as a no-mouse alternative.
- **Resolution:** [D12](decision-log.md#d12-drop-cross-pane-and-keyboard-driven-selection) extended to also drop keyboard select mode. Rationale: the keyboard alternative existed because the standard TUI's alt-screen + mouse-cell-motion mode breaks native drag selection; in cmux mode each pane has native selection, so the keyboard mode is no longer needed. [Out of Scope](../feature-specification.md#out-of-scope) updated.
- **Resolved by:** evidence
- **Affected decisions:** D12, D20
- **Affected tech-notes:** —
- **Changed in spec:** User Interactions, Out of Scope

### F8: Detached-orchestrator option had the worst diagnosability

- **Agent:** devops-engineer (F-5)
- **Finding:** OI-1 (orchestrator location) recommended "detached background process" without analyzing failure semantics. A detached process crash leaves no on-screen output, no captured stderr, possibly an incomplete log file (panic vs clean exit, flush behavior), and no shell holding the exit code. The hidden-pane option is meaningfully more diagnosable.
- **Resolution:** new [D13](decision-log.md#d13-orchestrator-runs-in-a-hidden-cmux-pane) closes the original OI-1 with the hidden-pane choice — best diagnosability of the three options (cmux shows the pane exit status; operator can show the pane to inspect orchestrator output). Detached option rejected explicitly.
- **Resolved by:** evidence
- **Affected decisions:** D13, D4
- **Affected tech-notes:** T2, T5
- **Changed in spec:** Outcome, Primary Flow, Edge Cases and Failure Modes, User Interactions, Open Items (OI-1 closed; renumbered)

### F9: Detached-orchestrator ancestry conflict with cmux's access model

- **Agent:** junior-developer (F-04)
- **Finding:** cmux's default access mode restricts socket connections to descendants of cmux. A detached process that double-forks loses ancestry and cannot connect to cmux's socket. The original spec's OI-1 (detached process) and OI-5 (require launch from inside cmux) were in silent conflict.
- **Resolution:** new [D13](decision-log.md#d13-orchestrator-runs-in-a-hidden-cmux-pane) closes both original open items — the orchestrator runs in a hidden cmux pane (a cmux child by construction), so ancestry is preserved without operator effort. T2 expanded to call out the ancestry constraint and how the hidden-pane choice respects it.
- **Resolved by:** evidence
- **Affected decisions:** D13
- **Affected tech-notes:** T2
- **Changed in spec:** Preconditions, Open Items (OI-1, OI-5 closed)

### F10: "Brief grace period" was not a testable commitment

- **Agent:** junior-developer (F-02), devops-engineer (F-4), edge-case-explorer (F-6)
- **Finding:** the original [D7](decision-log.md#d7-workspace-cleanup-on-completion-superseded-by-d14) committed to "a brief grace period" before workspace auto-close on success. "Brief" is not testable, has no operator visibility, and regresses the standard TUI's `ModeDone` (which stays open until the operator quits).
- **Resolution:** D7 superseded by new [D14](decision-log.md#d14-workspace-closure-is-operator-initiated): workspace stays open until the operator dismisses it, matching `ModeDone`. [Primary Flow](../feature-specification.md#primary-flow) step 13 and the quit/error flows updated.
- **Resolved by:** evidence
- **Affected decisions:** D7 (superseded), D14
- **Affected tech-notes:** T5
- **Changed in spec:** Outcome, Primary Flow, Alternate Flows and States

### F11: Hung cmux socket had no timeout behavioral commitment

- **Agent:** devops-engineer (F-1)
- **Finding:** the original spec covered cmux-socket-goes-away but not cmux-socket-alive-but-unresponsive. A hung cmux would leave the orchestrator blocked indefinitely while Docker / git / claude state continued to diverge from the operator's view.
- **Resolution:** new [D15](decision-log.md#d15-cmux-api-per-call-timeout-is-fatal) commits to a configured per-call wall-clock timeout treated as fatal (same teardown as display loss). [Edge Cases](../feature-specification.md#edge-cases-and-failure-modes) row added; [Coordinations](../feature-specification.md#coordinations) row for cmux updated to call out the timeout. [Open Items](../feature-specification.md#open-items) OI-3 surfaces the exact timeout value for operator input.
- **Resolved by:** evidence
- **Affected decisions:** D15, D10
- **Affected tech-notes:** T1
- **Changed in spec:** Coordinations, Edge Cases and Failure Modes, Open Items

### F12: Startup ordering between orchestrator and displays was unspecified

- **Agent:** junior-developer (F-06)
- **Finding:** the spec spawned three display processes and immediately started the workflow with no statement about whether displays could miss the first events while still starting up.
- **Resolution:** new [D16](decision-log.md#d16-per-launch-readiness-handshake-before-workflow-starts) requires a readiness handshake on the local interaction channel; the orchestrator waits for all three displays to signal readiness before starting the workflow. [Primary Flow](../feature-specification.md#primary-flow) step 7 added.
- **Resolved by:** evidence
- **Affected decisions:** D16
- **Affected tech-notes:** T4
- **Changed in spec:** Primary Flow

### F13: Log file artifact continuity was silent

- **Agent:** devops-engineer (F-3)
- **Finding:** the spec said "same log content" but only referred to the log pane. The existing on-disk artifacts under `.pr9k/logs/` and the per-step `.jsonl` files (the primary post-mortem surface) were not addressed; with the orchestrator process location changing, the artifact path was not guaranteed to remain stable.
- **Resolution:** new [D17](decision-log.md#d17-log-file-artifacts-are-unchanged-in-cmux-mode) explicitly preserves the on-disk artifacts; orchestrator's working directory must be the target repository so paths match the standard TUI's. [Coordinations](../feature-specification.md#coordinations) row added; [Primary Flow](../feature-specification.md#primary-flow) step 12 added.
- **Resolved by:** evidence
- **Affected decisions:** D17
- **Affected tech-notes:** —
- **Changed in spec:** Outcome, Primary Flow, Coordinations, Edge Cases and Failure Modes

### F14: cmux API version compatibility had no surface

- **Agent:** devops-engineer (F-7)
- **Finding:** if cmux ships a breaking change to its API, pr9k's calls fail with cryptic JSON-RPC errors that give the operator no remediation guidance.
- **Resolution:** new [D18](decision-log.md#d18-startup-capability-check) commits to a startup capability check using cmux's `system.capabilities` / `system.identify` methods; failure produces an actionable error naming the missing methods. [Primary Flow](../feature-specification.md#primary-flow) step 3 updated; [Preconditions](../feature-specification.md#actors-and-triggers) updated.
- **Resolved by:** evidence
- **Affected decisions:** D18
- **Affected tech-notes:** T1
- **Changed in spec:** Actors and Triggers, Primary Flow, Edge Cases and Failure Modes

### F15: Error-mode notification text needed to point the operator at the control pane

- **Agent:** edge-case-explorer (F-4)
- **Finding:** an operator who has never seen a multi-pane pr9k workspace may not know which pane to focus when an error-mode prompt fires.
- **Resolution:** new [D19](decision-log.md#d19-error-mode-notifications-direct-the-operator-to-the-control-pane) commits to including the directive "Focus the pr9k control pane to respond" in the notification body. [Primary Flow](../feature-specification.md#primary-flow) step 11 and [Alternate Flows](../feature-specification.md#alternate-flows-and-states) updated.
- **Resolved by:** evidence
- **Affected decisions:** D19
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow, Alternate Flows and States, User Interactions

### F16: Keyboard state-machine ownership was ambiguous

- **Agent:** edge-case-explorer (F-5)
- **Finding:** rapid contradictory intents (e.g., `q` then `c`) raced unsafely if mode state was split between the footer process and the orchestrator.
- **Resolution:** new [D20](decision-log.md#d20-keyboard-state-machine-is-owned-by-the-footer-pane) commits to the footer pane owning the state machine locally; intents are serialized and the orchestrator ignores conflicting follow-ups once it has acted on a quit. Edge-case row added.
- **Resolved by:** evidence
- **Affected decisions:** D20
- **Affected tech-notes:** —
- **Changed in spec:** Edge Cases and Failure Modes

### F17: `ModeNextConfirm` (skip step) had no disposition in cmux mode

- **Agent:** edge-case-explorer (F-14)
- **Finding:** the standard TUI's "skip current step" mode requires the footer to trigger a subprocess termination owned by the orchestrator. The spec did not say whether `n` is supported in cmux mode or how the footer signals subprocess termination.
- **Resolution:** [D20](decision-log.md#d20-keyboard-state-machine-is-owned-by-the-footer-pane) enumerates the seven surviving modes (including NextConfirm) and clarifies that the footer forwards a skip intent to the orchestrator which then terminates the subprocess. [Alternate Flows](../feature-specification.md#alternate-flows-and-states) "Skip current step" added.
- **Resolved by:** evidence
- **Affected decisions:** D20
- **Affected tech-notes:** —
- **Changed in spec:** Alternate Flows and States, User Interactions

### F18: Help mode's `StatusLineActive` gate was inappropriate in cmux mode

- **Agent:** edge-case-explorer (F-11)
- **Finding:** in the standard TUI, `?` only opens help when a status-line script is configured. This gate is a coupling artifact that would confuse cmux-mode operators who naturally expect help to be always available.
- **Resolution:** [D20](decision-log.md#d20-keyboard-state-machine-is-owned-by-the-footer-pane) explicitly removes the `StatusLineActive` gate in cmux mode — help is always accessible in the footer. [Alternate Flows](../feature-specification.md#alternate-flows-and-states) "Help is requested" added.
- **Resolved by:** evidence
- **Affected decisions:** D20
- **Affected tech-notes:** —
- **Changed in spec:** Alternate Flows and States

### F19: Workspace uniqueness mechanism was unspecified

- **Agent:** edge-case-explorer (F-7)
- **Finding:** the original spec said "uniquely-named workspace" with only a timestamp example. Same-second launches would collide; the spec did not say what cmux does on name collision.
- **Resolution:** new [D21](decision-log.md#d21-workspace-name-format) commits to a fixed prefix + repo basename + high-resolution timestamp pattern that avoids same-second collisions. [Open Items](../feature-specification.md#open-items) OI-5 surfaces the exact pattern for operator input.
- **Resolved by:** evidence
- **Affected decisions:** D21
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow, Open Items

### F20: Workspace name needed to be visible to the operator

- **Agent:** junior-developer (F-08)
- **Finding:** D11 says orphans must be dismissed manually, but the original spec gave no commitment about how the operator learns the workspace's name to identify it.
- **Resolution:** [Primary Flow](../feature-specification.md#primary-flow) step 5 commits to printing the workspace name to the launching terminal at startup; [D21](decision-log.md#d21-workspace-name-format) gives the operator a recognizable form (prefix + repo + timestamp).
- **Resolved by:** evidence
- **Affected decisions:** D21
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow

### F21: Prior workspace restoration on dismissal was assumed not committed

- **Agent:** edge-case-explorer (F-13)
- **Finding:** the spec said "the operator's prior cmux session returns to focus" without committing to a mechanism. cmux's auto-focus behavior on workspace close is unspecified, so leaving it implicit would produce inconsistent UX.
- **Resolution:** new [D22](decision-log.md#d22-prior-workspace-is-captured-and-restored) commits to capturing the current workspace at startup and explicitly restoring it on dismissal via cmux's `workspace.select`. [Primary Flow](../feature-specification.md#primary-flow) step 2 added.
- **Resolved by:** evidence
- **Affected decisions:** D22
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow

### F22: Orphan workspace appearance was undescribed

- **Agent:** edge-case-explorer (F-12)
- **Finding:** the spec told the operator to dismiss orphans manually but did not describe how an orphan looks in the cmux workspace list (name, residual content, distinguishability from a live workspace).
- **Resolution:** [D21](decision-log.md#d21-workspace-name-format) (prefix-based naming) lets pr9k detect orphans and lets the operator find them in cmux's list. [D23](decision-log.md#d23-orphan-workspace-startup-advisory) commits to a startup advisory listing orphan names so the operator can identify which workspaces to dismiss.
- **Resolved by:** evidence
- **Affected decisions:** D21, D23
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow, Edge Cases and Failure Modes

### F23: Operator pane-close gesture was an unstated catastrophic-abort path

- **Agent:** devops-engineer (F-2)
- **Finding:** every cmux user has pane-close in muscle memory. The original [D10](decision-log.md#d10-display-pane-loss-aborts-the-run) made pane closure a fatal run abort without acknowledgement; an accidental keypress would destroy a multi-step workflow's in-flight git / Docker state.
- **Resolution:** new [D24](decision-log.md#d24-operator-pane-close-is-treated-as-display-loss) makes the behavior explicit and commits to documenting it in the cmux-mode setup how-to and the in-run help modal — operators are told up front that closing a pane closes the run. Confirmation-on-close rejected because cmux does not natively expose a pane-close hook.
- **Resolved by:** evidence
- **Affected decisions:** D24, D10
- **Affected tech-notes:** —
- **Changed in spec:** Edge Cases and Failure Modes

### F24: SIGKILL on the orchestrator had no specified behavior

- **Agent:** edge-case-explorer (F-8)
- **Finding:** SIGKILL bypasses any defer or signal handler. The spec did not say what state remains in cmux or what the operator must do to recover.
- **Resolution:** [Edge Cases](../feature-specification.md#edge-cases-and-failure-modes) row added: no cleanup runs, displays detect the channel loss and exit, the workspace persists as an orphan, prior workspace is not restored automatically, log file is truncated at the SIGKILL moment but prior content is intact.
- **Resolved by:** evidence
- **Affected decisions:** —
- **Affected tech-notes:** T5
- **Changed in spec:** Edge Cases and Failure Modes

### F25: Pane-resize-during-streaming behavior was unspecified

- **Agent:** edge-case-explorer (F-2)
- **Finding:** a continuous border-drag fires many `WindowSizeMsg` events; each could trigger a full re-wrap of the 2000-line buffer, producing CPU spikes and display tearing.
- **Resolution:** [Alternate Flows](../feature-specification.md#alternate-flows-and-states) "Operator resizes a cmux pane" updated to commit to debouncing resize events (redraw at most once per short interval); transient mid-drag artifacts are acceptable.
- **Resolved by:** evidence
- **Affected decisions:** —
- **Affected tech-notes:** —
- **Changed in spec:** Alternate Flows and States

### F26: IPC channel was not a named coordinating system

- **Agent:** junior-developer (F-06), devops-engineer (F-8), edge-case-explorer (F-1)
- **Finding:** the local interaction channel between orchestrator and display panes is a failure point independent of both the orchestrator and the displays — its failure was not enumerated in the [Coordinations](../feature-specification.md#coordinations) table or the [Edge Cases](../feature-specification.md#edge-cases-and-failure-modes) table.
- **Resolution:** Coordinations row added for the local interaction channel with its stall-detection contract. Edge-case row added for "channel stops delivering events"; the row commits to the same teardown as display loss after a stall threshold. [D10](decision-log.md#d10-display-pane-loss-aborts-the-run) and [D15](decision-log.md#d15-cmux-api-per-call-timeout-is-fatal) cross-reference the same teardown path.
- **Resolved by:** evidence
- **Affected decisions:** D10, D15
- **Affected tech-notes:** T4
- **Changed in spec:** Coordinations, Edge Cases and Failure Modes

### F27: T3 / T4 read as behavioral commitments rather than implementation context

- **Agent:** devops-engineer (F-9)
- **Finding:** T3 and T4 described mechanism (process per pane, IPC channel) in terms that implementors could read as required design rather than implementation context.
- **Resolution:** [feature-technical-notes.md](feature-technical-notes.md) updated with a top-level scope note clarifying that all T# entries are implementation context — the behavioral commitments live in the spec, and the implementation plan is free to choose any mechanism that respects the named constraints.
- **Resolved by:** evidence
- **Affected decisions:** —
- **Affected tech-notes:** T1, T2, T3, T4, T5
- **Changed in spec:** —

### F28: Versioning / documentation impact of the new CLI surface was unflagged

- **Agent:** junior-developer (F-10)
- **Finding:** OI-4 (now OI-1) defers the launch form, but whichever form is chosen is a new public CLI surface that requires a MINOR version bump and matching feature-doc updates per the versioning and documentation standards.
- **Resolution:** OI-1 (formerly OI-4) extended with a "Versioning / docs impact" line explicitly naming the version-bump and doc-update requirements.
- **Resolved by:** evidence
- **Affected decisions:** D2
- **Affected tech-notes:** —
- **Changed in spec:** Open Items

## Minor edits

- F29: removed the "External editor is invoked" alternate-flow stub that was already out of scope — it created the impression that cmux mode might partially handle it. — junior-developer — Alternate Flows and States
- F30: replaced the "2000-line ring buffer" implementation detail with behavioral language ("the same scrollback as the standard TUI") so the spec does not pin to a specific number. — junior-developer — User Interactions
- F31: removed the "Operator focuses a non-control pane" alternate-flow ceremony (Entry / Sequence / Exit) and merged the substance into User Interactions and the Edge Cases table. — junior-developer — Alternate Flows and States, User Interactions
- F32: closed original OI-5 (require launch from inside cmux) by evidence — T2 is sufficient now that OI-1 is committed as the hidden-pane choice. — junior-developer — Open Items
- F33: dropped "operator's prior cmux workspace returns to focus" phrasing in the quit flow exit line in favor of explicit "prior workspace is restored on dismissal" referencing D22. — edge-case-explorer — Alternate Flows and States
- F34: clarified that the orchestrator pane is the diagnostic surface for unexpected crashes and that operators can reveal it via cmux's pane-show controls. — devops-engineer — User Interactions
- F35: removed an unspecified exit-code reference from the orchestrator-dies edge-case row; the existing exit-code contract from docs/features/signal-handling.md applies unchanged. — junior-developer — Edge Cases and Failure Modes

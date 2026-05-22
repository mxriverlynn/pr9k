# Team Findings: Phase 6 — Robust Failure Handling

Review findings raised by the specialist sub-agents against [Phase 6 — Robust Failure Handling](../feature-specification.md). Each finding is classified as **major** (changes behavioral commitments, security/auth/PII, coordinations, edge cases, captured `T#` notes, or "mechanics leaking into spec") or **minor** (wording, naming, formatting, citation cleanup). Resolutions either cite evidence (`Resolved by: evidence`) or capture the user judgment used (`Resolved by: user input`).

Reviewers consulted:

- `junior-developer` — generalist stress-test for hidden assumptions, scope creep, and clarity gaps.
- `edge-case-explorer` — boundary conditions and races between the failure modes (simultaneous failures, in-flight calls, the quiet-vs-stalled distinction, the abort-path recursion guard, pre-run / post-run windows).
- `devops-engineer` — operational readiness: exit codes, diagnostics, orphan-workspace handling, target-repository side-effects, observability of failures.
- `test-engineer` — which observable failure behaviors the spec commits the system to making testable.

## Major findings

### F1: The liveness signal was committed in D3 without naming who emits it or that it must run in every phase

- **Raised by:** `junior-developer` (F-001, F-010), `edge-case-explorer` (EC2, EC5), `test-engineer` (F2).
- **Category:** Coordination / mechanics-clarity.
- **Original location:** [`feature-specification.md` "The interaction channel stalls"](../feature-specification.md#alternate-flows-and-states); [`decision-log.md` D3](decision-log.md#d3-a-healthy-but-quiet-channel-is-never-mistaken-for-a-stalled-one).
- **Concern:** D3 committed to a content-independent liveness signal but left unstated which process emits it, and did not commit that it must continue during a Phase 3 error-mode pause — a run paused at an error prompt for more than 45 seconds would otherwise self-abort. The behavioral guarantee was also not checkable: "healthy" was defined only by the absence of an unspecified mechanism.
- **Resolution:** D3 rewritten to commit (a) the orchestrator is the emitter, (b) the signal is content-independent, (c) it runs in *every* workflow phase including error-mode pauses and between steps, (d) it must be running before the stall timer counts, and (e) a pane treats the channel as healthy if it received any message within the threshold window. The cadence value stays implementation-owned. Spec "interaction channel stalls" flow, the paused-error-prompt edge-case row, and the interaction-channel Coordinations row updated to match.
- **Resolved by:** evidence (parent T4; Phase 3 / Phase 5 paused-run behavior; build-outline OQ-4).
- **Affected decisions:** D3 (full rewrite).
- **Affected tech-notes:** —
- **Changed in spec:** Actors and Triggers (Preconditions), Alternate Flows and States ("The interaction channel stalls"), Edge Cases and Failure Modes (quiet-step and paused-error-prompt rows), Coordinations (interaction-channel row), Summary.

### F2: An edge-case row implied the orchestrator closes the workspace during the abort, contradicting parent D14

- **Raised by:** `junior-developer` (F-005, blocker), `test-engineer` (F8).
- **Category:** Scope / contradiction.
- **Original location:** [`feature-specification.md` Edge Cases](../feature-specification.md#edge-cases-and-failure-modes) — "workspace-close call fails during dismissal cleanup".
- **Concern:** the Primary Flow and Outcome said the workspace stays open and the operator dismisses it (parent D14), but an edge-case row described a workspace-close call "during dismissal cleanup", reading as the orchestrator closing the workspace as part of the abort. Separately, pre-run failures (handshake timeout, workspace/pane creation rejected) left a half-initialized workspace whose handling was not named.
- **Resolution:** added D14, which states (a) pre-run failures are Phase 1 lifecycle-teardown scope, not Phase 6, and (b) the workspace-close call is not part of the abort sequence — the abort leaves the workspace open, and the close call runs only in the post-dismissal teardown after the operator dismisses; a failed close leaves an orphan. The edge-case row was reworded to "fails during the post-dismissal teardown", and a new row was added for pre-run failures. Preconditions section updated to scope Phase 6 to "a run is underway".
- **Resolved by:** evidence (parent D14, D22; parent spec "cmux's API rejects a request mid-flow" edge-case row; code survey of the RunPhase1 teardown sequence).
- **Affected decisions:** D14 (new).
- **Affected tech-notes:** —
- **Changed in spec:** Actors and Triggers (Preconditions), Primary Flow (step 8), Edge Cases and Failure Modes (workspace-close row reworded, pre-run-failure row added), Out of Scope (pre-run failure handling), Summary.

### F3: "First detected failure wins" depended on a single abort guard the spec did not describe

- **Raised by:** `junior-developer` (F-002), `edge-case-explorer` (EC11).
- **Category:** Coordination.
- **Original location:** [`feature-specification.md` Primary Flow](../feature-specification.md#primary-flow) step 1; "Multiple failure modes" alternate flow; [`decision-log.md` D9](decision-log.md#d9-the-run-aborted-notification-fires-exactly-once-per-aborted-run).
- **Concern:** the run-aborted "exactly once" guarantee (D9) requires every failure-detection path — cmux-call, channel-stall, display-loss — to check one shared abort-in-progress gate. Two near-simultaneous detections on independent paths could each believe they were first and each run the sequence.
- **Resolution:** D1 rewritten to commit a single one-way transition into an aborting state, checked by every detection path, made at most once per run; the detection that wins the transition is named in the diagnostic and later detections are absorbed. Primary Flow step 1 and the "Multiple failure modes" alternate flow updated to state the guard explicitly.
- **Resolved by:** evidence (Phase 5 D1 exactly-once commitment; standard concurrency-guard reasoning).
- **Affected decisions:** D1 (rewritten), D9 (Driven by findings updated).
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow (step 1), Alternate Flows and States ("Multiple failure modes"), Edge Cases and Failure Modes (simultaneous-failures row), Summary.

### F4: The spec did not cover the orchestrator dying partway through its own abort sequence

- **Raised by:** `edge-case-explorer` (EC3).
- **Category:** Edge case.
- **Original location:** [`feature-specification.md` Outcome](../feature-specification.md#outcome); [`decision-log.md` D4](decision-log.md#d4-orchestrator-process-death-is-the-documented-exception-to-the-clean-abort-outcome).
- **Concern:** D4 framed orchestrator death as "death before any cleanup runs". An orchestrator can be force-killed mid-sequence — after firing run-aborted but before clearing the sidebar — producing a partial-abort state that is neither the documented clean abort nor the documented orphan.
- **Resolution:** D4 extended to name partial abort as an explicit sub-case: the abort sequence is not transactional and cannot be made atomic against SIGKILL; whatever steps completed are what the operator observes, handled identically to full orchestrator death (orphan workspace, manual dismissal). New alternate flow "The orchestrator dies partway through its own abort sequence" and a matching edge-case row added.
- **Resolved by:** evidence (no in-process design can make a multi-step cmux interaction atomic against a force-kill).
- **Affected decisions:** D4 (extended).
- **Affected tech-notes:** —
- **Changed in spec:** Alternate Flows and States (new "partway through abort" flow), Edge Cases and Failure Modes (new row), Summary.

### F5: The "finish the in-flight cmux call" rule (D10) had no time bound and an unresolved timeout race

- **Raised by:** `junior-developer` (F-003), `edge-case-explorer` (EC1), `devops-engineer` (DOR-005), `test-engineer` (F6).
- **Category:** Edge case / coordination.
- **Original location:** [`feature-specification.md` Primary Flow](../feature-specification.md#primary-flow) step 2; [`decision-log.md` D10](decision-log.md#d10-the-orchestrator-finishes-any-in-flight-cmux-call-before-running-the-abort-sequence).
- **Concern:** D10 said the orchestrator waits for an in-flight call before aborting, but did not bound that wait. If the in-flight call (a steady-state call, normally fatal-on-timeout per D8) itself times out during the wait, it was ambiguous whether that timeout became a second fatal trigger or just a "proceed" signal.
- **Resolution:** D10 clarified: the wait is bounded by the in-flight call's own per-call timeout (not extended); the call's resolution — success, error, or its own timeout — only signals "the call is done, proceed"; a timeout after a failure has already been detected is not a second fatal trigger and does not change the named diagnostic cause. Primary Flow step 2 and a new edge-case row updated to match.
- **Resolved by:** evidence (parent D15/D27 per-call timeout already bounds every cmux call; D1's single abort guard makes the first detection authoritative).
- **Affected decisions:** D10 (clarified).
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow (step 2), Edge Cases and Failure Modes (in-flight-call-timeout row), Coordinations (steady-state cmux-call row), Summary.

### F6: On a clean abort the panes would linger up to 45 seconds with no outcome broadcast

- **Raised by:** `edge-case-explorer` (EC10).
- **Category:** Coordination / behavioral gap.
- **Original location:** [`feature-specification.md` Primary Flow](../feature-specification.md#primary-flow) step 6 (original).
- **Concern:** the clean-abort sequence fired the notification, cleared the sidebar, and exited — but never told the display panes the run had ended. The panes would only notice when their own 45-second stall threshold fired, leaving them alive and stale for up to 45 seconds after a "clean" abort, contradicting D2's prompt-disconnect intent.
- **Resolution:** added D13 — the clean-abort sequence broadcasts the aborted outcome to the three panes over the interaction channel so they render a final state and exit promptly. The existing cmux orchestration code already broadcasts a workspace-done signal with an exit code on the graceful path; the abort path routes through the same broadcast.
- **Resolved by:** evidence (code survey: the graceful path already sends a workspace-done broadcast with an exit code).
- **Affected decisions:** D13 (new).
- **Affected tech-notes:** —
- **Changed in spec:** Outcome, Primary Flow (step 7), Edge Cases and Failure Modes (stall-timer-vs-completion race row), Coordinations (display-panes row), Summary.

### F7: A failure detected in the post-run window could fire a spurious run-aborted notification after a successful completion

- **Raised by:** `edge-case-explorer` (EC7, EC9).
- **Category:** Edge case / race.
- **Original location:** [`feature-specification.md` Edge Cases](../feature-specification.md#edge-cases-and-failure-modes) — "display pane dies after the run completed".
- **Concern:** between the workflow finishing and the terminal outcome being broadcast, a pane death could be observed by the new abort-detection logic and fire a spurious run-aborted notification on top of a successful completion.
- **Resolution:** D9 clarified — the run-completed state is recorded the moment the workflow finishes, before any terminal notification or broadcast, so a failure observed in the post-run window is measured against an already-completed run and does not trigger an abort. New edge-case rows added for the post-run pane-death window and the stall-timer-vs-completion cosmetic race.
- **Resolved by:** evidence (the completion path already exists; the gate ordering is a sequencing commitment).
- **Affected decisions:** D9 (clarified).
- **Affected tech-notes:** —
- **Changed in spec:** Edge Cases and Failure Modes (post-run-window and stall-vs-completion rows), Summary.

### F8: The spec said nothing about target-repository side-effects (Docker, git) when a run aborts

- **Raised by:** `devops-engineer` (DOR-001).
- **Category:** Coordination / behavioral gap.
- **Original location:** [`feature-specification.md` Out of Scope](../feature-specification.md#out-of-scope); [`feature-specification.md` Coordinations](../feature-specification.md#coordinations).
- **Concern:** pr9k makes git commits, pushes branches, and runs Docker sandboxes. The spec described how the cmux/display side aborts but said nothing about the running workflow step's subprocess (a Docker container, a git push) when a cmux/display failure aborts the run.
- **Resolution:** added D15 — the abort terminates the running step's subprocess through pr9k's existing subprocess-termination path; the target repository is left in whatever state the terminated step reached, identical to a standard-display abort (parent D1: cmux mode changes only the presentation surface). New Coordinations row, an Outcome/Primary-Flow sentence, and an Out of Scope item (pr9k does not roll back target-repo side-effects).
- **Resolved by:** evidence (parent D1; pr9k's existing subprocess-termination behavior, documented in the subprocess-execution feature doc).
- **Affected decisions:** D15 (new).
- **Affected tech-notes:** —
- **Changed in spec:** Outcome, Primary Flow (step 3), Coordinations (workflow-step-subprocess row), Out of Scope, Summary.

### F9: "Workspace transitions to a failed state" named no observable signal

- **Raised by:** `test-engineer` (F3), `devops-engineer` (DOR-003).
- **Category:** Mechanics-clarity / testability.
- **Original location:** [`feature-specification.md` Outcome](../feature-specification.md#outcome); Primary Flow step 6 (original).
- **Concern:** "transitions the workspace to a failed state" implied a cmux workspace-level property that does not exist — cmux exposes no workspace "failed" flag. The phrase was non-assertable.
- **Resolution:** the phrase was replaced throughout with the concrete observable end state: the combination of the run-aborted notification, the cleared sidebar entries, the panes' final content, and pr9k's non-zero exit. The Outcome section now states explicitly that there is no persistent cmux-side "aborted" badge. The User Interactions feedback section names what an away operator returns to on the orchestrator-death path.
- **Resolved by:** evidence (code survey: no cmux workspace-state mutation API; the workspace simply persists).
- **Affected decisions:** —
- **Affected tech-notes:** —
- **Changed in spec:** Outcome, Primary Flow (step 8), User Interactions (Feedback).

### F10: D5 committed to "accurate classification" without enumerating classes or defining an unknown-code fallback

- **Raised by:** `test-engineer` (F5), `devops-engineer` (DOR-007).
- **Category:** Edge case / testability.
- **Original location:** [`decision-log.md` D5](decision-log.md#d5-mid-run-cmux-failures-are-classified-to-an-accurate-operator-diagnostic); [`feature-specification.md` "A cmux API call returns a structured or plaintext error mid-run"](../feature-specification.md#alternate-flows-and-states).
- **Concern:** D5 committed to classifying mid-run cmux errors but did not name the recognized classes or say what happens for an unrecognized error code — making the commitment non-falsifiable and risking a worse message than a raw code for any cmux error the classifier predates.
- **Resolution:** D5 extended to name the recognized classes (access denied, named missing method, authentication failure — the same classes the startup check recognizes) and to commit an unknown-code fallback: an unrecognized code is reported as an "unclassified cmux error" with the raw code preserved verbatim in the diagnostic and the log. New edge-case row added for the unrecognized-code case. The exact diagnostic strings remain implementation-plan work.
- **Resolved by:** evidence (parent D-R2; code survey: the startup preflight already classifies these codes).
- **Affected decisions:** D5 (extended).
- **Affected tech-notes:** —
- **Changed in spec:** Alternate Flows and States (structured-error flow), Edge Cases and Failure Modes (unrecognized-code row), Summary.

### F11: A partial sidebar clear (one entry cleared, one not) was an unnamed degraded state

- **Raised by:** `edge-case-explorer` (EC4).
- **Category:** Edge case.
- **Original location:** [`feature-specification.md` Primary Flow](../feature-specification.md#primary-flow) step 5 (original).
- **Concern:** the abort clears two independent sidebar entries; D6 makes each clear non-fatal on failure, so one can succeed and the other fail, leaving one stale entry — a state the spec did not name.
- **Resolution:** added an edge-case row stating a partial sidebar clear is an acceptable degraded state under D6 (failure logged, shutdown continues, operator may see one stale entry); the per-run log file is the authoritative record. D6's trivial-decision text updated to note the partial-clear case.
- **Resolved by:** evidence (D6 / Phase 5 D10 abort-path-calls-non-fatal rule already covers it; the row makes the consequence explicit).
- **Affected decisions:** D6 (text updated).
- **Affected tech-notes:** —
- **Changed in spec:** Edge Cases and Failure Modes (partial-sidebar-clear row).

### F12: Threshold-boundary inclusivity and truncated-log distinguishability were imprecise

- **Raised by:** `test-engineer` (F7), `junior-developer` (F-008).
- **Category:** Edge case / testability precision.
- **Original location:** [`decision-log.md` D2](decision-log.md#d2-the-interaction-channel-stall-threshold-is-45-seconds), [`decision-log.md` D12](decision-log.md#d12-the-orchestrators-diagnostic-surface-on-its-own-death-is-the-launching-terminal-and-the-per-run-log-file).
- **Concern:** D2 did not say whether the 45-second boundary is inclusive or exclusive — a test injecting exactly 45 seconds of silence could not assert correctly. D12 said a force-killed log "may be truncated" but did not say whether a truncated log is distinguishable from a complete one.
- **Resolution:** D2 pinned to an inclusive boundary ("at or after 45 seconds"). D12 extended to state that a truncated log carries no completion marker and so cannot always be distinguished from a log that ended for another reason — a documented limitation of the force-kill path. Spec edge-case rows and the "interaction channel stalls" flow updated to "45 seconds or more".
- **Resolved by:** evidence (test-determinism; the per-run log format has no end-of-run sentinel guarantee on the force-kill path).
- **Affected decisions:** D2 (boundary pinned), D12 (truncation note added).
- **Affected tech-notes:** —
- **Changed in spec:** Alternate Flows and States ("interaction channel stalls"), Edge Cases and Failure Modes (stall and force-kill rows).

### F13: A per-failure-class exit-code taxonomy was raised, evaluated, and deferred under YAGNI

- **Raised by:** `devops-engineer` (DOR-002).
- **Category:** YAGNI candidate.
- **Original location:** [`feature-specification.md` User Interactions](../feature-specification.md#user-interactions) ("exits non-zero").
- **Concern:** every Phase 6 failure path exits "non-zero" with no per-class distinction, so a script wrapping pr9k cannot branch on *why* a cmux run aborted.
- **Resolution:** evaluated against the YAGNI evidence test and deferred. pr9k's exit-code contract is "non-zero on failure", inherited unchanged from the standard display (parent D1); no operator or automation has been described that needs to branch on the abort class, and the classified cause is already in the per-run log diagnostic (D5). Added to the spec's Deferred (YAGNI) section with a named reopening trigger (an operator or wrapping automation reports needing a programmatic decision keyed on abort class). The User Interactions section notes the "non-zero, no per-class distinction" contract and points to the deferral.
- **Resolved by:** evidence (parent D1; YAGNI evidence test — no named consumer).
- **Affected decisions:** —
- **Affected tech-notes:** —
- **Changed in spec:** User Interactions (Error states), Deferred (YAGNI) (new entry).

## Minor edits

- **F14:** Clean-disconnect detection was described as "immediately" with no bound — reworded to "promptly, within the channel's normal read cycle, far below the stall threshold" so the clean-disconnect path is distinguishable from a near-miss stall. — `test-engineer` — Alternate Flows and States, Edge Cases and Failure Modes, Coordinations.
- **F15:** The orchestrator-death flow did not state that all three panes render the "run aborted" line — added "the operator sees the same line in each of the three panes, rendered without coordination between them". — `devops-engineer` — Alternate Flows and States ("orchestrator process dies").
- **F16:** The "run aborted" pane line had no committed content — User Interactions now commits that each pane's final rendered line contains the token "run aborted", with exact wording/styling left to the implementation plan. — `test-engineer` — User Interactions (Affordances).
- **F17:** The structured/plaintext-error alternate flow read as a distinct behavior — reworded to state explicitly that it is an instance of D5's general classification rule, not a separate commitment; speculative entry-condition examples trimmed. — `junior-developer` — Alternate Flows and States.
- **F18:** The OQ-2 live-cmux CI test-strategy question was recorded in Open Items with a concrete recommendation (continue mocked tests + mandatory manual in-pane gate, plus a manual test script exercising the quiet-step non-abort, the deliberate channel wedge, and the near-simultaneous multi-failure case); flagged as a team decision for implementation planning, not a spec behavior. — `devops-engineer`, `edge-case-explorer` — Open Items.
- **F19:** The transient window where the sidebar shows stale step/iteration values between the abort trigger and the sidebar clear was unnamed — User Interactions now notes it as a brief, expected transient. — `junior-developer` — User Interactions (Feedback).

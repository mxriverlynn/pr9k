# Team Findings: Phase 5 — Lifecycle Notifications

Review findings raised by the specialist sub-agents against [Phase 5 — Lifecycle Notifications](../feature-specification.md). Each finding is classified as **major** (changes behavioral commitments, security/auth/PII, coordinations, edge cases, captured `T#` notes, or "mechanics leaking into spec") or **minor** (wording, naming, formatting, citation cleanup). Resolutions either cite evidence (`Resolved by: evidence`) or capture the user judgment used (`Resolved by: user input`).

Reviewers consulted:

- `junior-developer` — generalist stress-test for hidden assumptions, scope creep, and clarity gaps.
- `user-experience-designer` — notification text, persistent re-fire UX, dismissal-resistance, and fatigue.
- `edge-case-explorer` — interaction between the persistent re-fire timer and the abort/quit paths, concurrent error prompts, the cmux-unreachable-during-error-mode case.

## Major findings

### F1: "Control pane" jargon mismatch with shipped operator-facing vocabulary

- **Raised by:** `user-experience-designer` (Finding 1).
- **Category:** Vocabulary / consistency.
- **Original location:** [`feature-specification.md` Outcome, Primary Flow, User Interactions](../feature-specification.md); [`artifacts/decision-log.md` D2, D15](decision-log.md); parent [D19](../../artifacts/decision-log.md#d19-error-mode-notifications-direct-the-operator-to-the-control-pane) (origin of the phrase).
- **Concern:** the notification text committed `Focus the pr9k control pane to respond`, but every user-facing pane label (`docs/how-to/setting-up-cmux.md` lines 89, 97, 101, 115) refers to the same surface as the "footer pane." Operators reading the notification see vocabulary they have never seen before.
- **Resolution:** updated [D2](decision-log.md#d2-notification-text-is-repo-name-plus-lifecycle-verb-error-text-incorporates-the-footer-pane-directive) to commit Phase 5 to "Focus the footer pane to respond" and explicitly supersede parent D19's phrasing while preserving its behavioral intent (the directive names the pane to focus). Updated every spec section that referenced "control pane" to use "footer pane." Removed the trivial-decision entry that referenced control-pane phrasing.
- **Resolved by:** evidence (shipped how-to vocabulary).
- **Affected decisions:** D2 (full rewrite), trivial-decision list (removed prior D15 about control-pane phrasing).
- **Affected tech-notes:** —
- **Changed in spec:** Outcome (last paragraph of error-mode bullet), Primary Flow step 5, User Interactions — Affordances (error-mode text), Summary (key adjustments).

### F2: Persistent re-fire has no in-pr9k escape hatch short of answering — coercive constraint not disclosed to operators

- **Raised by:** `user-experience-designer` (Finding 2).
- **Category:** YAGNI / disclosure.
- **Original location:** [`feature-specification.md` Out of Scope](../feature-specification.md#out-of-scope); [`D4`](decision-log.md#d4-dismissal-does-not-suppress-re-fire-pr9k-owns-the-cadence).
- **Concern:** the design rationale for dismissal-resistance is sound (preventing silent strands), but the trade-off — the operator who needs to step away cannot silence the re-fire short of answering or workspace dismissal — is not documented anywhere the operator would read before encountering it.
- **Resolution:** confirmed via evidence test that the behavior commitment (dismissal-resistance) is correct and YAGNI-justified; remediation is documentation, not behavior change. Added a note in `Out of Scope` that the persistent re-fire behavior warrants a how-to documentation update, which is tracked as an implementation work-item, not a spec change.
- **Resolved by:** evidence + work-item handoff.
- **Affected decisions:** D4 (added `Driven by findings: F2`); no behavioral change.
- **Affected tech-notes:** —
- **Changed in spec:** Out of Scope (added the how-to handoff bullet).

### F3: Three-surface signal ordering at error-mode entry was unspecified

- **Raised by:** `user-experience-designer` (Finding 3), `junior-developer` (Finding 3), `edge-case-explorer` (EC2, EC3).
- **Category:** Coordination / behavioral commitment.
- **Original location:** [`feature-specification.md` Outcome, Coordinations](../feature-specification.md); [`artifacts/decision-log.md`](decision-log.md) — no decision had been written.
- **Concern:** the notification, the sidebar pill, and the footer-pane mode-switch all fire on the same lifecycle event, but the spec said "independent" with no ordering. If the notification reaches the operator before the footer pane has rendered error-mode hints, the operator activates the notification and arrives at a workspace whose footer still looks normal. Edge-case-explorer surfaced the same problem from the other direction: the timer-stop contract is only reliable if the orchestrator's state bit is set before the notification call begins.
- **Resolution:** added a new full decision [D12](decision-log.md#d12-error-mode-effects-are-ordered-footer-broadcast-state-bit-and-timer-notification-call) committing the orchestrator to three-step ordering: (1) broadcast footer mode-switch, (2) set state bit and start timer, (3) fire notification.
- **Resolved by:** user input (design judgment — footer-broadcast first preserves the UX commitment; state-bit-second preserves the timer-stop contract).
- **Affected decisions:** new D12, updated D5's `Dependent decisions` to include D12.
- **Affected tech-notes:** —
- **Changed in spec:** Outcome, Primary Flow step 4, User Interactions — Feedback (three-surface signal description), Coordinations table (Phase 3 row), Summary.

### F4: Sustained non-timeout re-fire failures were an undocumented infinite-loop

- **Raised by:** `junior-developer` (Finding 4), `edge-case-explorer` (EC6).
- **Category:** Edge case / failure mode.
- **Original location:** [`feature-specification.md` Edge Cases](../feature-specification.md#edge-cases-and-failure-modes); [`D7`](decision-log.md#d7-non-timeout-notification-errors-are-logged-and-the-run-continues-timeouts-remain-fatal-per-parent-d15).
- **Concern:** D7 said "next re-fire is attempted normally" with no upper bound. If every re-fire fails non-fatally (e.g., cmux notification surface degraded, socket closed, OS permission revoked), the workflow stays paused forever with silent log-only failures and no operator interrupt.
- **Resolution:** added a new trivial decision [D17](decision-log.md#trivial-decisions) acknowledging the sustained-failure case as an explicit acceptable degraded state, with the Phase 4 sidebar pill as the backstop. Updated D7's rejected-alternatives to record that a failure-count threshold was considered and rejected (the sidebar pill is the backstop; a threshold would manufacture an operator-visible escalation for no behavior the sidebar does not already provide). Added a new alternate flow row to the spec covering this case.
- **Resolved by:** evidence (Phase 4 D13 sidebar pill exists exactly to be the persistent passive signal).
- **Affected decisions:** D7 (added rejected alternative, `Dependent decisions: D17`); new D17.
- **Affected tech-notes:** —
- **Changed in spec:** Outcome (sidebar-backstop sentence added), Primary Flow step 8, Alternate Flows and States (new row "Error prompt is open and every re-fire fails non-fatally"), Edge Cases ("cmux loses notification methods mid-run" row), User Interactions — Error States, Coordinations (Phase 4 row).

### F5: Click-to-focus inter-pane hop is undocumented for operators

- **Raised by:** `user-experience-designer` (Finding 5).
- **Category:** Documentation handoff.
- **Original location:** [`feature-specification.md` User Interactions](../feature-specification.md#user-interactions); [`D9`](decision-log.md#d9-click-to-focus-brings-the-pr9k-workspace-into-view).
- **Concern:** after activating the notification, the operator arrives at the pr9k workspace with cmux's default-focused pane active (likely the log pane), reads "Focus the footer pane to respond" in the notification, and must perform a separate cmux gesture they may not know to reach the footer pane.
- **Resolution:** D9's design (workspace-only targeting) stands; the YAGNI deferral for per-pane targeting is correct. Remediation is documentation — the cmux-mode how-to should describe how to focus a specific pane after activating a notification. Tracked as an implementation work-item.
- **Resolved by:** evidence + work-item handoff.
- **Affected decisions:** D9 (added `Driven by findings: F5`).
- **Affected tech-notes:** —
- **Changed in spec:** no spec change; tracked as a how-to follow-up in Out of Scope.

### F6: Same-repo concurrent-run disambiguation gap

- **Raised by:** `user-experience-designer` (Finding 4).
- **Category:** Edge case / behavioral disclosure.
- **Original location:** [`feature-specification.md` Edge Cases](../feature-specification.md#edge-cases-and-failure-modes); [`D2`](decision-log.md#d2-notification-text-is-repo-name-plus-lifecycle-verb-error-text-incorporates-the-footer-pane-directive).
- **Concern:** D2 commits to repo basename as the disambiguation token, but two concurrent runs against the same repo produce textually identical notifications. The spec's edge-cases table covered only "different repositories."
- **Resolution:** added a same-repo edge-case row to the spec acknowledging that notifications alone do not disambiguate; the workspace title (with timestamp suffix per parent D29) and the per-run log file path remain the operator's authoritative disambiguation. Added a new Deferred (YAGNI) entry ("Same-repo concurrent-run disambiguation in notification text") with reopen trigger.
- **Resolved by:** evidence (workspace title already disambiguates; simpler-version test favors the documentation route).
- **Affected decisions:** D2 (added `Driven by findings: F6` and qualifying language).
- **Affected tech-notes:** —
- **Changed in spec:** Outcome (sanitizer-fallback sentence), Edge Cases (new "Two pr9k cmux runs are active concurrently against the same target repository" row), Deferred (YAGNI) (new entry).

### F7: "Aborted" verb reads as failure for clean operator-initiated quit

- **Raised by:** `user-experience-designer` (Finding 8).
- **Category:** Copy / disambiguation.
- **Original location:** [`feature-specification.md` Alternate Flows and States — Operator quits successfully mid-run](../feature-specification.md#alternate-flows-and-states); [`D6`](decision-log.md#d6-quit-from-error-mode-stops-re-fire-then-fires-run-aborted-once).
- **Concern:** "pr9k run aborted in `<repo>`" fires for clean operator-initiated quit as well as unexpected aborts. "Aborted" connotes failure; the operator who chose to stop may read the notification in their history later and think the run failed.
- **Resolution:** kept the single "run aborted" wording for this release (operator knows they just quit moments earlier; the wording mismatch is minor). Added a Deferred (YAGNI) entry ("Distinct text for operator-initiated quit vs. unexpected abort") with reopen trigger. Updated the "Operator quits successfully mid-run" alternate flow to cross-reference the deferral.
- **Resolved by:** user judgment (simpler-version satisfies the same evidence).
- **Affected decisions:** D6 (no body change; added `Driven by findings: F7`).
- **Affected tech-notes:** —
- **Changed in spec:** Alternate Flows and States (clean-quit row gains a cross-reference to deferral); Deferred (YAGNI) (new entry).

### F8: Run-aborted notification fires after `workspace.close`, against a closed handle

- **Raised by:** `edge-case-explorer` (EC1).
- **Category:** Critical edge case / integration boundary.
- **Original location:** [`feature-specification.md` Primary Flow](../feature-specification.md#primary-flow); [`D6`](decision-log.md#d6-quit-from-error-mode-stops-re-fire-then-fires-run-aborted-once).
- **Concern:** the spec said the run-aborted notification fires "at the end of shutdown" without pinning whether shutdown means before or after `workspace.close`. The existing `runTeardown` (`src/internal/cmuxctl/runphase1.go:205–228`) calls `WorkspaceClose` near the end of shutdown. If the notification fires after that, the handle is reclaimed and the call silently fails per D7.
- **Resolution:** added a new full decision [D11](decision-log.md#d11-run-aborted-and-completion-notifications-fire-before-workspaceclose-while-the-workspace-handle-is-still-valid) pinning that both terminal notifications fire before `workspace.close`, while the workspace handle is still valid. Updated the Primary Flow, Alternate Flows, and Coordinations sections to reference D11.
- **Resolved by:** evidence (the file-line citation to `runTeardown` made the failure path concrete and the resolution obvious).
- **Affected decisions:** new D11, D6 (`Dependent decisions: D11`).
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow step 2, Alternate Flows and States (clean-quit row, error-mode-quit row, successful-completion row), Coordinations (new Phase 1 workspace lifecycle row), Summary.

### F9: Step name was mutable across re-fires

- **Raised by:** `edge-case-explorer` (EC4).
- **Category:** State dependency.
- **Original location:** [`feature-specification.md` Primary Flow, Alternate Flows](../feature-specification.md); decision log had no commitment.
- **Concern:** `cmuxSidebar.lastStepName` is updated on every step transition. While the orchestrator is paused in error mode no transitions happen — but the spec made no commitment that the value used in re-fires was the value at error-mode entry. An implementation that recomputed at each re-fire would be exposed to any future code path that mutates `lastStepName` outside the normal step-transition flow.
- **Resolution:** added a new full decision [D13](decision-log.md#d13-step-name-is-snapshotted-at-error-mode-entry-not-recomputed-at-each-re-fire) committing the orchestrator to snapshot the step name at error-mode entry and reuse it for every re-fire of that prompt.
- **Resolved by:** evidence (file-line citation made the candidate source and its mutability concrete).
- **Affected decisions:** new D13.
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow step 5, Alternate Flows and States (multiple-prompts row), User Interactions — Affordances (error-mode text), Edge Cases.

### F10: Run-aborted call site relative to `WorkspaceDone` broadcast was unspecified

- **Raised by:** `edge-case-explorer` (EC5).
- **Category:** Coordination / sequencing.
- **Original location:** [`feature-specification.md` Coordinations](../feature-specification.md#coordinations).
- **Concern:** the existing post-workflow sequence at `src/cmd/pr9k/cmux_pane.go:203–210` includes `WorkspaceDone` broadcast to display panes and the ack wait. The spec did not pin where the run-aborted notification call goes in this sequence.
- **Resolution:** subsumed by [D11](decision-log.md#d11-run-aborted-and-completion-notifications-fire-before-workspaceclose-while-the-workspace-handle-is-still-valid) (notification fires before `workspace.close`). The implementation plan picks whether the notification fires before or after the `WorkspaceDone` ack cycle — both placements satisfy D11 because both are before `WorkspaceClose`. The spec does not need finer-grained sequencing.
- **Resolved by:** evidence (D11 makes the relative ordering moot for the workspace-handle validity concern).
- **Affected decisions:** D11.
- **Affected tech-notes:** —
- **Changed in spec:** Coordinations (Phase 1 workspace-lifecycle row covers this implicitly).

### F11: Notification text examples read like authoritative strings but were marked "for example"

- **Raised by:** `junior-developer` (Finding 5).
- **Category:** Spec content clarity.
- **Original location:** [`feature-specification.md` User Interactions — Affordances](../feature-specification.md#user-interactions); [`D2`](decision-log.md#d2-notification-text-is-repo-name-plus-lifecycle-verb-error-text-incorporates-the-footer-pane-directive).
- **Concern:** `for example, pr9k run completed in <repo-basename>` invites disagreement between spec and implementation about how much of the example is binding.
- **Resolution:** rewrote D2 and the User Interactions section to (1) state the spec-level commitments as explicit numbered invariants the implementation must satisfy, (2) call the example strings "expected canonical wording" so the spec's intent is visible without forcing the implementation to deviate gratuitously.
- **Resolved by:** evidence (spec content rule).
- **Affected decisions:** D2 (substantial rewrite).
- **Affected tech-notes:** —
- **Changed in spec:** User Interactions — Affordances (rewritten).

### F12: "In-flight call allowed to complete" was an implementation mechanic in the spec

- **Raised by:** `junior-developer` (Finding 6).
- **Category:** Mechanics leaking into spec.
- **Original location:** [`feature-specification.md` Primary Flow, Edge Cases](../feature-specification.md).
- **Concern:** the spec described the timer-stop semantics via the implementation mechanic ("inflight call allowed to complete") rather than the operator-observable commitment.
- **Resolution:** rewrote the relevant sentences to express the operator-observable commitment: "The operator may see at most one additional error-mode notification if a call was already mid-flight at the stop moment." The implementation chooses its own mechanism for that guarantee.
- **Resolved by:** evidence (spec content rule).
- **Affected decisions:** D5 (rewritten).
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow step 7, Edge Cases ("operator answers during the brief interval" row), Alternate Flows and States (operator-quit-from-error-mode flow).

### F13: In-flight call timeout after answer — fatal-vs-non-fatal was unresolved

- **Raised by:** `junior-developer` (Finding 2).
- **Category:** Edge case.
- **Original location:** [`feature-specification.md` Edge Cases](../feature-specification.md#edge-cases-and-failure-modes); [`D5`](decision-log.md#d5-resolution-of-the-error-prompt-stops-re-fire-immediately).
- **Concern:** an in-flight notification call whose timeout fires after the operator has answered the prompt could trigger the fatal-abort path under a literal reading of parent D15, even though the error-mode state is already resolved.
- **Resolution:** added a new trivial decision [D16](decision-log.md#trivial-decisions) committing that any in-flight call whose outcome (including timeout) lands after an error-mode answer is non-fatal. This is derived from D5 and the operator-observable commitment.
- **Resolved by:** evidence (derives from D5's operator-observable commitment).
- **Affected decisions:** new D16; D5 (`Dependent decisions: D16`); D7 (added explicit cross-reference to the new exception).
- **Affected tech-notes:** —
- **Changed in spec:** Primary Flow step 9, Edge Cases ("operator answers during the brief interval" row), Coordinations (cmux notification surface row).

### F14: Abort-path notification timeout exception was load-bearing but buried in D7's rationale

- **Raised by:** `junior-developer` (Finding 7).
- **Category:** Spec content / clarity.
- **Original location:** [`feature-specification.md` Edge Cases](../feature-specification.md#edge-cases-and-failure-modes); [`D7`](decision-log.md#d7-non-timeout-notification-errors-are-logged-and-the-run-continues-timeouts-remain-fatal-per-parent-d15).
- **Concern:** D7's "log but don't recurse" sentence carried an implicit exception to parent D15's "timeout is fatal" rule. An implementer reading parent D15 first could miss the exception and produce a recursive abort.
- **Resolution:** added a new full decision [D10](decision-log.md#d10-abort-path-notification-calls-treat-every-failure-including-timeout-as-non-fatal-explicit-exception-to-parent-d15) that names the exception as a first-class rule, scoped narrowly to abort-path calls. Updated D7 and the Edge Cases table to cite D10 explicitly.
- **Resolved by:** evidence (the exception was already implicit; making it first-class prevents misreading).
- **Affected decisions:** new D10; D7 (added explicit cross-reference).
- **Affected tech-notes:** —
- **Changed in spec:** Outcome (last paragraph), Primary Flow step 9, Edge Cases (new "abort-path call times out" row), Coordinations (cmux notification surface row).

### F15: Completion-call-timeout edge case was missing from the table

- **Raised by:** `edge-case-explorer` (EC8).
- **Category:** Edge case completeness.
- **Original location:** [`feature-specification.md` Edge Cases](../feature-specification.md#edge-cases-and-failure-modes).
- **Concern:** D7 implicitly covered this (timeout is fatal, triggers abort, run-aborted fires) but the spec did not name the case in the table, leaving it to readers to infer.
- **Resolution:** added an explicit edge-case row: "The **completion** notification call times out. → The timeout is fatal; the orchestrator runs the abort path; run-aborted fires once."
- **Resolved by:** evidence (D7 + D10 + parent D15).
- **Affected decisions:** D7 (cross-reference added), D10 (cross-reference added).
- **Affected tech-notes:** —
- **Changed in spec:** Edge Cases table.

### F16: Open Items section claimed "none" while three behavioral edge cases were unresolved

- **Raised by:** `junior-developer` (Finding 8).
- **Category:** Spec content / honesty.
- **Original location:** [`feature-specification.md` Open Items](../feature-specification.md#open-items).
- **Concern:** Open Items declared completeness while the two-step quit timer-stop boundary, the in-flight call timeout treatment, and the state-bit/timer ordering were all unresolved.
- **Resolution:** the three behavioral edge cases are now resolved by D5 (first-keystroke), D16 (non-fatal), and D12 (ordering). The Open Items section now points to those decisions rather than claiming "none" preemptively.
- **Resolved by:** evidence + the new D12/D16 decisions captured in this review pass.
- **Affected decisions:** D5, D12, D16 (the resolutions).
- **Affected tech-notes:** —
- **Changed in spec:** Open Items (rewritten to point to the resolving decisions).

## Minor edits

- **F17:** Distinct-abort-sub-reason YAGNI reopen trigger relaxed from "operators report routinely needing" to "any operator reports needing" — `junior-developer` Finding 9. — Changed in spec: Deferred (YAGNI) — "Distinct notification text per abort sub-reason".
- **F18:** YAGNI deferral for notification severity/urgency split into two — operator-configurable urgency (kept) and implementation-fixed urgency differentiation between abort and completion (new deferral) — `user-experience-designer` Findings 6, 10. — Changed in spec: Out of Scope (configurable urgency clarification), Deferred (YAGNI) (new "Implementation-fixed urgency differentiation between abort and completion" entry).
- **F19:** Inflight-race edge-case row gained explicit "what the operator should expect next" — `user-experience-designer` Finding 9. — Changed in spec: Edge Cases ("operator answers during the brief interval" row).
- **F20:** Sanitizer fallback ("repo" placeholder) acknowledged in disambiguation claim — `edge-case-explorer` EC7. — Changed in spec: Outcome (last paragraph of disambiguation), Edge Cases (new sanitizer-fallback row), D2 evidence.
- **F21:** Empty/whitespace step name invariant noted in preconditions — `edge-case-explorer` EC9. — Changed in spec: Actors and Triggers — Preconditions; new trivial decision D18.

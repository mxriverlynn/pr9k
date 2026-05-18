# Team Findings: Phase 3 — Interactive Error Recovery

This file records every finding raised by the review team for Phase 3 of pr9k cmux mode, and how each was resolved. Behavioral outcomes live in [../feature-specification.md](../feature-specification.md); decisions the findings affected live in [decision-log.md](decision-log.md). No `feature-technical-notes.md` exists in this artifacts folder — no load-bearing mechanic qualified for Phase 3 (every mechanic the spec relies on is discoverable in the Phase 2 codebase and its test suite) — so no finding cites a T# ID.

Feature size: **Small** (single subsystem; builds entirely on the implemented and tested Phase 2 interaction channel, footer state machine, and orchestrator adapter; no cross-service integration; no auth/PII surface; no data migration; behavioral surface fits within the existing four-pane workspace). Review-team cap: 2 — `junior-developer` (mandatory generalist stress-test) plus `edge-case-explorer` (the feature's risk surface is overwhelmingly failure-mode and keystroke-absorption boundary behavior).

Both reviewers converged on one load-bearing concern: the draft asserted the Phase 2 error-mode path existed without verifying it. That concern was resolved by direct evidence from the shipped Phase 2 code and test suite (see F1, F2), not escalated to the user. Every finding was resolvable by evidence; nothing required user input.

## Major findings

### F1: The Phase 2/Phase 3 reuse seam was asserted, not verified (race-window guarantee included)

- **Agent:** junior-developer (JD-001), edge-case-explorer (FINDING-1)
- **Finding:** The draft framed Phase 3 as "thin reuse" of Phase 2's footer error-mode handling and orchestrator adapter (including the "a choice the instant the prompt appears is not lost" race-window guarantee) but never verified Phase 2 actually implemented and tested those paths — Phase 2's demo had no failing step, so the error path could have been stubbed.
- **Resolution:** Verified by direct evidence from the shipped Phase 2 code and tests, not assertion. `cmux_footer_machine.go` implements `ui.ModeError` (continue/retry/quit, `q`→`ModeQuitConfirm`, `n`/`esc`→restore); `cmux_key_adapter.go` forwards the recovery intents and pushes `StateFooter` on mode change; Phase 2 tests `TestAdapter_StateFooterPushedOnSetModeError`, `TestAdapter_ModeErrorRaceWindow_NoDroppedKeystroke`, and the footer-machine `q`-in-`ModeError` and `q`+`esc`-restore cases exercise the path for real. The shared `internal/ui/orchestrate.go` error path is the same code cmux mode's hidden orchestrator pane runs. The spec's race-window edge-case row and Primary Flow §6 were rewritten to state the behavioral invariant ("an error-recovery choice … is delivered and acted upon once the orchestrator is in error mode; no choice is dropped") without leaking file names; the file/test provenance was placed under D1's `Evidence:` field. The intro and Builds-on lines were updated to say Phase 2's paths "are covered by Phase 2's test suite."
- **Resolved by:** evidence
- **Affected decisions:** D1
- **Changed in spec:** Intro / Builds on, Primary Flow, Edge Cases and Failure Modes, Summary

### F2: The failed-step header state was assumed present, not confirmed

- **Agent:** junior-developer (JD-002)
- **Finding:** D2 relied on "the header pane's state-push message already carries … the failed state" without confirming the failed-step header path exists and is exercised, given Phase 2's happy-path demo never rendered a failed step.
- **Resolution:** Confirmed by evidence: the cmux header pane reuses the standard `internal/ui` step header renderer (Phase 2 plan D-20), driven by the *shared* `internal/ui/orchestrate.go` state machine that calls `header.SetStepState(idx, StepFailed)` on a non-zero exit — the identical code cmux mode runs in its hidden orchestrator pane. `internal/ui/header.go` renders `StepFailed` as the `✗` marker, and `interactionchannel.StateHeader.StepStates` carries the integer form of `StepFailed`. The failed-header path is therefore inherited shared code, not new cmux capability. D2's `Evidence:` was expanded with these citations.
- **Resolved by:** evidence
- **Affected decisions:** D2
- **Changed in spec:** Outcome, Primary Flow, User Interactions

### F3: Ambiguous whether silent absorption on non-control panes is new in Phase 3 or inherited from Phase 2

- **Agent:** junior-developer (JD-003)
- **Finding:** The spec presented silent absorption of control keys on non-control panes as a Phase 3 deliverable while the parent spec's "Operator focuses a non-control pane" flow is cited as Phase 2 source — leaving scope muddied (new behavior vs. confirmed inherited behavior).
- **Resolution:** Resolved by evidence: the header and log panes are display-only Phase 2 sub-processes that do not run the footer keyboard state machine and do not forward intents, so absorption already holds as of Phase 2. The spec's Alternate Flow and D4 were reworded to state Phase 3 *confirms* the Phase 2 absorption behavior is unchanged during error mode (an extension/confirmation, not a wholesale new behavior).
- **Resolved by:** evidence
- **Affected decisions:** D4
- **Changed in spec:** Alternate Flows and States, Edge Cases and Failure Modes

### F6: Spec was silent on the version bump and feature-doc updates the change requires

- **Agent:** junior-developer (JD-006)
- **Finding:** Phase 3 makes a previously stranded cmux failure interactively recoverable — a user-visible behavior change to `--cmux`. The versioning and documentation coding standards require a version bump and matching doc updates; the spec said nothing, while Phase 2 handled the equivalent obligation in its implementation plan.
- **Resolution:** Added Open Item OI-2 recording that the version bump and the cmux-mode feature-doc / setup how-to updates are an implementation-plan responsibility and must not be omitted from the shippable change. Kept out of the behavioral spec body (versioning/doc mechanics are implementation-plan scope, consistent with how Phase 1/2 handled it).
- **Resolved by:** evidence
- **Affected decisions:** —
- **Changed in spec:** Open Items, Summary

### F7: Parent spec's "Step fails" flow describes a Phase 5 notification

- **Agent:** junior-developer (JD-007)
- **Finding:** The parent spec's "Step fails and prompts for operator decision" alternate flow says a cmux notification fires; Phase 3 (which implements step-failure behavior) deliberately does not fire it until Phase 5. A reader of the parent spec alone would get a false picture of when the notification ships.
- **Resolution:** Resolved by evidence with no parent edit: the parent `feature-specification.md` is the whole-feature specification (the end-state across all phases) by design; phase decomposition and per-phase scoping live in `build-phase-outline.md` and the per-phase specs. The Phase 3 spec already scopes the notification out explicitly and points to Phase 5 (Outcome, Out of Scope, D4). The parent spec is internally correct as a whole-feature artifact; adding phase qualifiers to it would conflate the two document roles. Reasoning recorded here; no spec change.
- **Resolved by:** evidence
- **Affected decisions:** —
- **Changed in spec:** —

### F8: "No new message kinds" in Coordinations is implementation phrasing

- **Agent:** junior-developer (JD-008), edge-case-explorer (FINDING-6)
- **Finding:** The Coordinations table asserted "No new message kinds and no new cmux calls are introduced" — an implementation constraint masquerading as a behavioral commitment, and a mechanics leak into the spec.
- **Resolution:** Rewrote the Coordinations row behaviorally: "the orchestrator signals the failed-step state to the header, the error output to the log, and the error-mode state to the footer over the existing Phase 2 interaction channel … it does not introduce any new cmux calls (notifications and sidebar updates are deferred to Phases 5 and 4)." The message-shape claim was dropped from the spec; the verified-reuse provenance lives under D1's `Evidence:`.
- **Resolved by:** evidence
- **Affected decisions:** —
- **Changed in spec:** Coordinations

### F9: Duplicate alternate-flow and edge-case row for the quit-then-conflicting-intent case (YAGNI)

- **Agent:** junior-developer (JD-009)
- **Finding:** The standalone Alternate Flow "Operator initiates quit then sends a conflicting intent in rapid succession" duplicated an identical Edge Cases table row and added no behavioral content beyond inherited parent D20 — symmetry/completeness YAGNI anti-pattern.
- **Resolution:** Removed the standalone alternate-flow section; kept the single Edge Cases row, which cites parent D20 as inherited unchanged behavior. One surface is enough.
- **Resolved by:** evidence
- **Affected decisions:** D1
- **Changed in spec:** Alternate Flows and States, Edge Cases and Failure Modes

### F11: Behavior of non-confirmation keys during the quit confirmation (within error mode) was undefined

- **Agent:** edge-case-explorer (FINDING-2)
- **Finding:** The spec did not say what happens when, during an error prompt, the operator presses quit and then a non-confirmation key (e.g., retry) while the quit confirmation is showing — specifically whether that key is buffered and replayed on cancel, potentially triggering an unintended retry.
- **Resolution:** Resolved by evidence (the inherited footer state machine's quit-confirmation handler acts only on the confirm/cancel keys and lets any other key fall through ignored, with no buffering). Added an Edge Cases row and folded the rule into D1's decision text: non-confirmation keys are ignored and not buffered for replay; cancelling returns to the same error prompt without triggering continue/retry.
- **Resolved by:** evidence
- **Affected decisions:** D1
- **Changed in spec:** Alternate Flows and States, Edge Cases and Failure Modes

### F12: Cancel-quit footer-state restoration invariant was unstated

- **Agent:** edge-case-explorer (FINDING-3)
- **Finding:** The spec said cancelling a quit "returns to the error prompt" but did not state the invariant that makes this safe in an asynchronous multi-pane architecture (could a newer error state overwrite the restored one?).
- **Resolution:** Resolved by evidence: the orchestrator is paused in error mode for the entire quit-confirmation excursion, so no new error state can arrive to replace the one being restored. Added that invariant explicitly to the Alternate Flow, the Edge Cases row, and D1's rationale.
- **Resolved by:** evidence
- **Affected decisions:** D1
- **Changed in spec:** Alternate Flows and States, Edge Cases and Failure Modes

### F13: Transient cross-pane desync not acknowledged; "simultaneous" overclaimed

- **Agent:** edge-case-explorer (FINDING-4, FINDING-9)
- **Finding:** The spec's "three simultaneous signals" wording overclaimed atomicity in a multi-process renderer, and two real desync windows were unacknowledged: (a) an operator focusing the footer the instant after a failure may briefly see normal-mode shortcuts; (b) an operator who continues instantly may never visually see `[✗]`.
- **Resolution:** Replaced "simultaneous" with "near-simultaneous … subject to the small bounded cross-pane desync Phase 2 already accepts" (Outcome, User Interactions), citing parent T4. Added two Edge Cases rows for the stale-footer window and the continue-before-`[✗]` window, both classified as accepted bounded desync with the on-disk artifact as the authoritative failure record. Folded the reasoning into D2 and D4.
- **Resolved by:** evidence
- **Affected decisions:** D2, D4
- **Changed in spec:** Outcome, Alternate Flows and States, Edge Cases and Failure Modes, User Interactions

### F14: Retry-separator ordering dependency was an unstated correctness assumption

- **Agent:** edge-case-explorer (FINDING-5)
- **Finding:** D3's "in the same order" claim depended on the retry separator reaching the log pane before the retried step's output, but the spec did not state this as a behavioral requirement — an implementation could start the subprocess before emitting the separator.
- **Resolution:** Stated the sequencing requirement behaviorally in the spec (Primary Flow §7, Alternate Flow "Retry fails again", Coordinations) and in D3: the retry separator is emitted before any output from the retried step, and per-pane log delivery order is preserved. Evidence (shared orchestrate path writes the separator before the retried step runs; serial per-connection channel delivery) recorded under D3's `Evidence:`.
- **Resolved by:** evidence
- **Affected decisions:** D3
- **Changed in spec:** Primary Flow, Alternate Flows and States, Coordinations, User Interactions

## Minor edits

- F4: D1's rationale named Go files/methods in prose; rewritten behaviorally with the file/symbol provenance moved to D1's `Evidence:` field (the rationale archive's allowed home) — junior-developer — decision-log.md D1
- F5: Spec referenced "the standard failure indicator" and "retry separator" abstractly; made concrete as the user-observable `[✗]` marker and the `── <step name> (retry) ─────────────` separator line — junior-developer — feature-specification.md Outcome, User Interactions
- F10: OI-1's resolution guidance named specific implementation components and a test approach; trimmed to a behavioral pointer to the parent build outline's OQ-2 — junior-developer — feature-specification.md Open Items
- F15: Preparation-failure / soft-fail-on-timeout / skip edge-case rows did not say what the cmux header shows; clarified that each shows the same non-`[✗]` marker the standard display uses for that case (inherited unchanged) — edge-case-explorer — feature-specification.md Edge Cases and Failure Modes
- F16: Unbounded retry loop flagged as a potential edge case; confirmed as inherited standard-display behavior with no cmux-specific amplification and dropped as a YAGNI candidate (no new edge case created by Phase 3); the no-retry-cap inheritance is now stated explicitly in the spec — edge-case-explorer — feature-specification.md Alternate Flows and States, Out of Scope

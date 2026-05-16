# Implementation Iteration History: Phase 2 — First Real Workflow Runs End-to-End in Cmux

This file records how the Phase 2 implementation plan evolved across discussion rounds. Committed decisions live in [implementation-decision-log.md](implementation-decision-log.md); the primary plan lives in [../feature-implementation-plan.md](../feature-implementation-plan.md).

Source artifact: Phase 2 of `../../build-phase-outline.md` (lines 130–177), with the parent feature spec at `../../feature-specification.md` and parent technical notes at `../../artifacts/feature-technical-notes.md`. Phase 1's implementation plan at `../../phase-1-workspace-lifecycle/feature-implementation-plan.md`.

## R1: Parallel specialist review

- **Specialists engaged:** `behavioral-analyst`, `concurrency-analyst`, `test-engineer`, `junior-developer`. (Project-manager NOT engaged this round per skill design — aggregation is deterministic.)
- **New input provided:** Initial Phase 2 brief based on the build-phase-outline Phase 2 section, the parent feature specification, the parent feature technical notes (T1–T5), the Phase 1 implementation plan, and the in-repo Phase 1 code (`internal/cmuxctl`, `src/cmd/pr9k/main.go`). Specialists were also given the OQ-3 Option A (Unix domain socket) strong-default and told to treat T-notes as committed mechanics.

### Claim ledger

Findings consolidated from four parallel specialist reports. Findings that two or more specialists raised independently are consolidated into single rows naming every supporter.

| Claim ID | Specialists | Category | State | Summary |
|----------|-------------|----------|-------|---------|
| CL-1 | behavioral (B1), junior (OQ-3), concurrency (C1, C4) | ambiguity | Evidenced | OQ-3 (interaction-channel mechanism) is not yet a committed decision; every Phase 2 behavioral commitment depends on Option A's specific protocol shape. Cite: `build-phase-outline.md#oq-3`. |
| CL-2 | behavioral (B2), concurrency (C1, C2), test-engineer (T1, T2, T3) | ambiguity / deadlock-risk / race-condition | Evidenced | Readiness handshake protocol is undefined: synchronization primitive, deadline on a never-connecting pane, duplicate-ready-signal counting, framing on the wire. Cite: `feature-specification.md#primary-flow-step-7`, `artifacts/decision-log.md#d16`. |
| CL-3 | behavioral (B3) | mechanic-leak | Evidenced | The standard `main.go` pre-populates the first phase header state before `program.Run()`; the cmux orchestrator has no analogue, so the header pane shows nothing during the handshake window. Cite: `src/cmd/pr9k/main.go:236-250`. |
| CL-4 | behavioral (B4), concurrency (C5) | mechanic-leak | Evidenced | `workflow.Run` and `ui.Orchestrate` take a concrete `*ui.KeyHandler` by pointer; the orchestrator-to-footer split needs an explicit proxy contract (mode-state push direction, intent push direction, shutdown rendezvous). Cite: `src/internal/workflow/run.go:316`, `src/internal/ui/orchestrate.go:73`. |
| CL-5 | behavioral (B5), test-engineer (T6, T7) | ambiguity | Evidenced | The `q`/`y` quit-confirmation flow across the process boundary: footer must handle `q`/`y` locally and forward only fully resolved intents (`ActionQuit`, etc.) to the orchestrator; partial keystrokes must not leak. Cite: `feature-specification.md#alternate-flows-and-states`, `artifacts/decision-log.md#d20`. |
| CL-6 | behavioral (B6, B15), test-engineer (T13) | edge-case / mechanic-leak | Evidenced | When the orchestrator pushes `SetMode(ModeError)` and immediately blocks on `<-h.Actions`, a footer keystroke pressed before the mode update is applied is consumed by `ModeNormal` and dropped — a race window the T4 desync acceptance does not explicitly cover. Cite: `src/internal/ui/orchestrate.go:124-128`, `artifacts/feature-technical-notes.md#t4`. |
| CL-7 | behavioral (B7), test-engineer (T11) | mechanic-leak | Evidenced | "Skip current step" maps to `runner.Terminate`, which is the `cancel` function passed to `NewKeyHandler`; needs an explicit `ActionSkip` intent type (or equivalent) on the channel boundary. Cite: `src/internal/ui/ui.go:89-96`, `feature-specification.md#skip-current-step`. |
| CL-8 | behavioral (B8), test-engineer (T14) | ambiguity | Evidenced | Phase 1 explicitly skips logger creation; Phase 2 must reintroduce `logger.NewLogger` inside the orchestrator pane process. The `runCmuxMode` entry point must call something equivalent to `startup()` (not just `startupValidate`) so the per-run artifact directory and `RunStamp` are created. Cite: `src/cmd/pr9k/main.go:111-120`, `artifacts/decision-log.md#d17`. |
| CL-9 | behavioral (B9), concurrency (C10), test-engineer (T5), junior (OQ-1) | ambiguity / ordering-violation | Evidenced | "Byte-for-byte identical" log artifacts (D17) is loosely defined: timestamps, RunStamp, terminal-width-dependent phase-banner underline, and process-specific values inevitably diverge between runs. The acceptance criterion must be sharpened (e.g., "subprocess stdout lines + step markers + iteration records — not run stamps or terminal-width-dependent rendering"). Cite: `artifacts/decision-log.md#d17`. |
| CL-10 | behavioral (B10, B11), concurrency (C6), test-engineer (T12), junior (OQ-7) | mechanic-leak / ordering-violation | Evidenced | `statusline.Runner` (a) embeds `*logger.Logger` and (b) receives `PushState` from `workflow.Run`. In cmux mode the logger lives in the orchestrator while the runner lives in the footer — cross-process. Resolution must address both the logger coupling and the cross-boundary `PushState`/`Trigger` ordering. Cite: `src/internal/statusline/statusline.go:54-56`, `artifacts/decision-log.md#d26`. |
| CL-11 | behavioral (B12) | ambiguity | Evidenced | The `StatusLineActive` gate on the `?` help key is still present in `src/internal/ui/keys.go`. D20 says help is always available in cmux mode; the footer's `KeyHandler` must be configured to always allow help. Cite: `src/internal/ui/keys.go:83-89`. |
| CL-12 | behavioral (B13), concurrency (C7), test-engineer (S2) | mechanic-leak | Evidenced | The heartbeat indicator (`⋯ thinking (Ns)`) depends on a `HeartbeatReader` that reads `atomic.Int64` in the same process; `atomic.Int64` does not cross process boundaries. Either drop the indicator in cmux mode or forward `LastEventAt` over the channel via a 1Hz pusher goroutine. Cite: `src/internal/ui/header.go:80-123`, `src/cmd/pr9k/main.go:254`. |
| CL-13 | behavioral (B14), concurrency (C8), test-engineer (T9) | ambiguity / ordering-violation | Evidenced | "Workspace marked as done" lacks a committed observable: a footer `ModeDone` shortcut-hint change, header-checkbox final state, and a protocol-level "run complete" message must be defined so the footer can distinguish normal completion from display loss. The orchestrator must NOT close the interaction socket until panes have processed the done message. Cite: `feature-specification.md#primary-flow-step-13`, `artifacts/decision-log.md#d14`. |
| CL-14 | concurrency (C3) | T#-contradiction | Evidenced | A single-goroutine fan-out that writes sequentially to header, log, footer sockets violates T4's "single-digit-millisecond cross-pane drift": a slow log pane delays header/footer. Fan-out must be per-connection goroutines with independent buffered channels. Cite: `artifacts/feature-technical-notes.md#t4`. |
| CL-15 | concurrency (C11) | race-condition | Evidenced | `FakeClient.HangNext`/`HangRelease` are accessed without `f.mu` protection; existing Phase 1 code has a data race the race detector may not catch under normal scheduling. Likely to recur when extending `FakeClient` for Phase 2 testing. Cite: `src/internal/cmuxctl/fake.go`. |
| CL-16 | test-engineer (Required new test seams) | YAGNI-candidate | Evidenced | Three new test doubles needed: `FakeInteractionChannel`, `FakeDisplayPane`, `FakeFooterKeySource`. The 14 planned tests justify the listed hooks; do not add hooks speculatively. |
| CL-17 | junior (OQ-2) | ambiguity | Anecdotal | Is the orchestrator-in-separate-pane load-bearing or convention? Spec D13 chose it for ancestry preservation (T2), but the footer process has ancestry by construction — could orchestrator + footer be one process? Cite: `feature-specification.md#d13`, `artifacts/feature-technical-notes.md#t2`. |
| CL-18 | junior (OQ-4) | ambiguity | Anecdotal | Pane resize minimum-size behavior is undefined for the per-pane case: each pane has its own SIGWINCH; `MinTerminalWidth` constants in `internal/uichrome` are for the standard TUI. Cite: `feature-specification.md#operator-resizes-a-cmux-pane`. |
| CL-19 | junior (OQ-5) | ambiguity | Anecdotal | The help modal's visual form in a small (~2-row) footer pane is undefined; the standard TUI's modal is a full-screen overlay. Cite: `feature-specification.md#help-is-requested`. |
| CL-20 | junior (OQ-6) | ambiguity | Anecdotal | Display pane lifecycle after `workflow.Run` returns: does the log pane stay alive indefinitely (so scrollback works) or exit (which would trip the Phase 1 `DismissalObserver`)? Phase 2 must decide whether Phase 1's dismissal observer is reused, modified, or replaced. Cite: `artifacts/decision-log.md#d14`. |
| CL-21 | junior (OQ-8) | ambiguity | Anecdotal | The `pr9k v0.x.x` version label location in cmux mode is unspecified; standard mode renders it in the footer corner. Cite: standard mode `docs/features/tui-display.md` (referenced by CLAUDE.md). |
| CL-22 | junior (OQ-9), test-engineer (Required new test seams) | ambiguity | Evidenced | The interaction-channel interface must be injectable so the renderer panes can be unit-tested in isolation with a `FakeInteractionChannel`. Cite: `docs/coding-standards/testing.md`. |
| CL-23 | junior (OQ-10) | ambiguity | Anecdotal | `version.Version` reads `0.10.0` in this branch; Phase 1's plan committed to `0.10.0 → 0.11.0` but the bump did not land. Phase 2 may inherit the Phase 1 bump or get its own — the plan must coordinate. Cite: `src/internal/version/version.go`, Phase 1 implementation plan U10. |
| CL-24 | concurrency (C9) | overlap (non-finding) | Evidenced | Confirms `RealClient` queue and Phase 2 interaction sockets do not contend — they use different sockets with no shared mutex. No action required. |

### Open Questions raised

Plan-level Open Questions (resolvable in this skill via evidence, junior-developer reframing, or user input):

- **OQ-2-1 (commit OQ-3 Option A):** Formalize the Unix domain socket per workspace as Phase 2's interaction-channel mechanism. Strong-default exists; no specialist proposed an alternative. **Resolution path:** evidence (existing OQ-3 recommendation). [CL-1]
- **OQ-2-2 (readiness handshake protocol):** Name the wire shape: how a pane signals ready, how the orchestrator counts to three with role identity, the bounded deadline for the wait. **Resolution path:** plan decision + a behavioral-analyst handoff into the plan, or junior reframing. [CL-2]
- **OQ-2-3 (state push fan-out topology):** Commit per-connection goroutines with independent buffered channels to honor T4. Phase 1 has no fan-out; this is a Phase 2 design decision. **Resolution path:** plan decision driven by concurrency analyst finding C3. [CL-14]
- **OQ-2-4 (bidirectional socket I/O — separate read/write goroutines):** Commit to separate read and write goroutines per bidirectional connection to avoid the kernel-buffer deadlock. **Resolution path:** plan decision driven by concurrency C4. [CL-2, CL-4]
- **OQ-2-5 (KeyHandler proxy contract):** Define how the orchestrator's `workflow.Run` call receives a `KeyHandler`-shaped object whose `Actions` channel is fed by intents from the footer and whose `SetMode` calls are forwarded back. Adapter vs. real handler with custom cancel. **Resolution path:** plan decision. [CL-4]
- **OQ-2-6 (Skip step intent type):** Add `ActionSkip` (or equivalent) to `ui.StepAction`. **Resolution path:** plan decision. [CL-7]
- **OQ-2-7 (statusline.Runner cross-boundary):** Logger coupling resolution (silent diagnostics, separate file, forward via channel) and `PushState`/`Trigger` ordering resolution (snapshot-then-unlock survives or not). **Resolution path:** plan decision; possibly engage software-architect if the shape is non-obvious. [CL-10]
- **OQ-2-8 (heartbeat indicator):** Drop in cmux mode or forward via a 1Hz pusher goroutine. **Resolution path:** plan decision driven by simplicity + YAGNI. [CL-12]
- **OQ-2-9 (workspace-done observable):** Define the protocol-level "run complete" message that the orchestrator sends before closing the socket; how each pane transitions to ModeDone; how the Phase 1 `DismissalObserver` is reused/modified/replaced. **Resolution path:** plan decision; closely related to OQ-2-2 (handshake) and CL-20 (junior). [CL-13, CL-20]
- **OQ-2-10 (log artifact equivalence criterion):** Sharpen "byte-for-byte" to a testable comparison (per-step JSONL files + step-content lines, excluding RunStamp and terminal-width-dependent renders). **Resolution path:** plan decision + reframing; **user input** if the user wants to override D17's literal wording. [CL-9]
- **OQ-2-11 (logger creation in orchestrator pane):** Where in `runCmuxMode` is `logger.NewLogger` called; mirror the standard `startup()` shape. **Resolution path:** plan decision. [CL-8]
- **OQ-2-12 (help-modal `StatusLineActive` gate):** Remove the gate in cmux mode via constructor parameter or unconditional `SetStatusLineActive(true)`. **Resolution path:** plan decision. [CL-11]
- **OQ-2-13 (first-frame pre-population for header pane):** Mirror standard mode's pre-population so the header is not blank during the handshake window. **Resolution path:** plan decision. [CL-3]
- **OQ-2-14 (test seams: `FakeInteractionChannel`, `FakeDisplayPane`, `FakeFooterKeySource`):** Commit the three test doubles named by test-engineer. **Resolution path:** plan decision; YAGNI-bounded scope per CL-16. [CL-16, CL-22]
- **OQ-2-15 (pre-existing `FakeClient.HangNext` race):** Fix in Phase 2's prep work (mutex-protect the field) or document as out-of-scope. **Resolution path:** plan decision; possibly junior-reframable (fix-in-place is a one-line change). [CL-15]
- **OQ-2-16 (orchestrator-in-separate-pane evidence):** Is D13 load-bearing or convention? **Resolution path:** evidence from `feature-specification.md#d13` and `feature-technical-notes.md#t2`. [CL-17]
- **OQ-2-17 (display pane lifecycle after workflow completion):** Stay-alive vs. exit; impact on Phase 1's `DismissalObserver`. **Resolution path:** plan decision. [CL-20]
- **OQ-2-18 (version label in footer pane):** Preserve or drop. **Resolution path:** evidence (standard mode keeps it; default to keeping it for cmux). [CL-21]
- **OQ-2-19 (version bump coordination):** Does Phase 2 absorb 0.10.0 → 0.11.0 or land its own bump? **Resolution path:** **user input** (River must coordinate Phase 1's pending ship). [CL-23]

Spec-level Open Questions (would require returning to spec-stage work; small set, gate did not trip):

- **OQ-2-S1 (resize minimum-size contract):** Each pane's "too narrow" behavior is undefined. **Resolution path:** **user input** OR defer to first-implementation default (use existing `MinTerminalWidth`). [CL-18]
- **OQ-2-S2 (help modal visual form in small footer pane):** Undefined visual form. **Resolution path:** **user input** OR defer to first-implementation default (modal as inline overlay above the footer's status line, height-bounded). [CL-19]

T#-contradictions:

- **OQ-2-T1 (T4 fan-out contradiction):** Concurrency analyst C3 raises that a sequential single-goroutine fan-out would violate T4's "single-digit-ms drift" commitment. **Resolution path:** plan decision (per-connection goroutines) — the T-note is honored by choosing the correct topology, not by contradicting T4. [CL-14]

### Spec-maturity tags

| Tag | Count | Specialists | Gate-trip condition met? |
|-----|-------|-------------|--------------------------|
| plan-level | 17 (OQ-2-1 through OQ-2-19, minus the two S items) | 4 (behavioral, concurrency, test-engineer, junior) | n/a |
| spec-level | 2 (OQ-2-S1, OQ-2-S2) | 1 (junior) | Need ≥ 5 by ≥ 3 distinct specialists — **NOT MET** |
| T#-contradiction | 1 (OQ-2-T1; C3 only) | 1 (concurrency) | Need ≥ 2 by ≥ 2 distinct specialists — **NOT MET** |

**Spec-maturity gate trip: NO.** Both arms fall short. The plan-implementation skill continues normally without engaging PM for a gate-trip review.

### Resolution source

For each Open Question, the deterministic aggregation tags the resolution path. Items marked "user input" will be escalated in Round 2 (or before synthesis if Round 2 is skipped):

| OQ | Source | Notes |
|----|--------|-------|
| OQ-2-1 | evidence | Phase-2 brief committed Option A as strong-default; no alternative proposed. |
| OQ-2-2 | plan decision + behavioral handoff | Define wire protocol (ready message shape, role identity, deadline). |
| OQ-2-3 | plan decision | Per-connection goroutines, independent buffered channels. |
| OQ-2-4 | plan decision | Separate read/write goroutines per bidirectional connection. |
| OQ-2-5 | plan decision | Adapter type with mode-push-back and intent-push-in. |
| OQ-2-6 | plan decision | Add `ActionSkip` to `ui.StepAction`. |
| OQ-2-7 | plan decision | Logger coupling: silent diagnostics for the runner in cmux mode (smallest change). PushState/Trigger ordering: same snapshot semantics, accept the bounded staleness window per T4. |
| OQ-2-8 | plan decision + YAGNI | Drop heartbeat for Phase 2 unless evidence demands it. |
| OQ-2-9 | plan decision | Explicit "run complete" message; orchestrator waits for ack before closing socket. |
| OQ-2-10 | plan decision + reframing | "Equivalent log content modulo run-stamp / wall-clock / terminal-width-dependent renders." User confirms in synthesis or before. |
| OQ-2-11 | plan decision | Mirror standard `startup()`'s logger creation in the orchestrator's entry point. |
| OQ-2-12 | plan decision | One-line: footer calls `SetStatusLineActive(true)` unconditionally before running. |
| OQ-2-13 | plan decision | Mirror standard `main.go` pre-population in the orchestrator before unblocking the handshake. |
| OQ-2-14 | plan decision + YAGNI | Three test doubles per CL-16; no speculative hooks. |
| OQ-2-15 | plan decision | Fix `FakeClient.HangNext` race in Phase 2 (one-line mutex). |
| OQ-2-16 | evidence | D13 ancestry rationale (parent T2) applies to orchestrator only because the orchestrator otherwise could be a double-fork; footer co-location is technically possible but introduces orchestrator-failure-conflated-with-display-failure, defeating the diagnostic-pane purpose of D13. Keep D13 as committed. |
| OQ-2-17 | plan decision | Display panes stay alive after `workflow.Run` returns, hosting a "done" loop; Phase 1's `DismissalObserver` is reused unchanged (operator's workspace-close gesture or pane-close gesture both still fire dismissal). |
| OQ-2-18 | evidence | Keep version label in cmux footer (matches standard mode). |
| OQ-2-19 | user input | Phase 1 ship status + Phase 2 bump scope. **Escalated to user before synthesis.** |
| OQ-2-S1 | plan decision (defer) | Use existing `MinTerminalWidth`/`MinTerminalHeight` as per-pane defaults; resize behavior is "make the pane bigger" advisory if below threshold. |
| OQ-2-S2 | plan decision (defer) | Help renders as a one-line "press ? for help" prompt that expands inline above the footer pane's normal content when activated — bounded to the footer's height. |
| OQ-2-T1 | plan decision | Honor T4 by topology choice; no actual T-note contradiction. |

### Decisions produced

— (Decisions are committed during PM synthesis in Step 8.)

### Changed in plan

— (Plan is written during PM synthesis in Step 8.)

### Project-manager next-step recommendation

**Continue iterating — Round 2 focused on:**
1. **User escalation pass** for OQ-2-19 (version bump coordination), OQ-2-10 (sharpening "byte-for-byte" wording), OQ-2-S1 + OQ-2-S2 (resize / help-modal user preferences). Recommend defaults; ask user to confirm or override.
2. **No new specialist engagement** — Round 1 produced enough material for synthesis once user input is in. The deterministic aggregation has already settled all plan-level items by plan decision or evidence; no findings require fresh specialist analysis.

If user defers all four user-input items to recommended defaults, Round 2 may be skipped and the skill proceeds straight to YAGNI sweep + synthesis (Step 7.5 + Step 8).

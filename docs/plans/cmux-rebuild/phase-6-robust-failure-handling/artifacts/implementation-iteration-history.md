# Implementation Iteration History: Phase 6 — Robust Failure Handling

<!--
This file records how the implementation plan for Phase 6 — Robust Failure Handling
evolved across discussion rounds. Committed decisions live in
[implementation-decision-log.md](implementation-decision-log.md) and the primary plan
lives in [../feature-implementation-plan.md](../feature-implementation-plan.md).

Cross-referencing invariants:
- `Decisions produced:` — D# IDs from implementation-decision-log.md added or changed this round.
- `Changed in plan:` — sections of ../feature-implementation-plan.md this round updated.
-->

## R1: Parallel specialist review

- **Specialists engaged:** `concurrency-analyst`, `test-engineer`, `devops-engineer`, `junior-developer`. (`project-manager` synthesizes in Step 8; it does not facilitate per round.)
- **New input provided:** Initial feature specification (`feature-specification.md`), its `artifacts/decision-log.md` and `artifacts/team-findings.md`, and the Step-2 discovery notes (`artifacts/.discovery-notes.md`). Each specialist received a domain-scoped brief.

- **Claim ledger:**

  | # | Claim | Raised by | State |
  |---|-------|-----------|-------|
  | CL-1 | The abort gate (D1) should be a `sync.Once` with the failure cause captured inside the `Do` closure; a separate `atomic.Bool` `runCompleted` flag closes the post-run-window race (D9). | concurrency-analyst (C1, C6); test-engineer (F-C); junior-developer (P6-002, P6-009) | Evidenced — `docs/coding-standards/concurrency.md` "Mutually exclusive state flags: first-flag-wins"; decision-log D1/D9; Go 1.26.2 `sync.Once`/`atomic.Bool`. |
  | CL-2 | An abort detected on an `interactionchannel` connection goroutine must interrupt the synchronous `workflow.Run()` loop by calling `runner.Terminate()` (unblocks the step subprocess) then sending `ActionQuit`; a new `ExitReasonAborted` distinguishes it from `ExitReasonUserQuit`. | concurrency-analyst (C4); junior-developer (P6-003) | Evidenced — `workflow/run.go` (synchronous, single goroutine); existing `keyHandler.ForceQuit()` two-step; `workflow/exit_reason.go` enum has only completed/loop_broken/user_quit. |
  | CL-3 | The liveness emitter (D3) is a goroutine started in `runCmuxOrchestratorWith` after `AwaitReady` returns and before `runCmuxWorkflowAdapted`, on a context-derived cancel + `WaitGroup` join; cadence is a fixed 10s exported constant; D3's "running before the stall timer counts" is a correctness ordering, satisfied by this start point. | concurrency-analyst (C2); test-engineer (F-B, T2, G2); junior-developer (P6-001) | Evidenced — decision-log D3; `cmux_workflow.go:179–186` ctx/cancel/wg precedent; `channel.go` connection model. |
  | CL-4 | The 45s stall timer (D2) lives per-connection inside the `readLoop` goroutine on both sides, reset (Stop-drain-Reset) on every received message; threshold + an `onStall` callback are injected so tests use a short threshold. | concurrency-analyst (C3); test-engineer (F-A, T1, T13, T14); junior-developer (P6-001) | Evidenced — decision-log D2; `channel.go:269–286` `readLoop`; `docs/coding-standards/testing.md` no-`time.Sleep` rule. |
  | CL-5 | The liveness signal is a new zero-field `Liveness{}` message type with a typed wire-type constant `"liveness"`; reusing `StateHeader/Log/Footer` would violate D3's content-independence and cause render flicker. | concurrency-analyst (C7); test-engineer (F-B, T11); junior-developer (P6-008) | Evidenced — decision-log D3; `messages.go`/`framing.go`; `docs/coding-standards/go-patterns.md` typed-string-constant rule. |
  | CL-6 | D10's "finish the in-flight cmux call before aborting" is naturally satisfied by `RealClient`'s single-goroutine serial queue — abort-path calls queue behind any in-flight call; the only requirement is abort-path calls use `context.Background()` so `do()`'s `ctx.Done()` branch does not drop them. | concurrency-analyst (C5); junior-developer (P6-004) | Evidenced — `real.go` serial queue + `do()` `ctx.Done()` escape; `cmux_pane.go:200` `context.Background()` precedent. |
  | CL-7 | Each display pane renders a final line containing the exact token `run aborted` and **returns** (does not block on `<-ctx.Done()`); the clean-abort path needs an `Aborted bool` field on `WorkspaceDone` so panes render the same token on the survivable path; a shared exported constant pins the token. | concurrency-analyst (C8, C9); test-engineer (F-E, T7, T14, G5); devops-engineer (F-003); junior-developer (P6-005) | Evidenced — spec User Interactions F16; `cmux_pane.go:264–303` `runCmuxDisplayPaneWith`; `WorkspaceDone` currently `ExitCode int` only. |
  | CL-8 | The D7 help disclosure target is the cmux footer pane's inline `?` help (`buildFooterHelpLines()` in `cmux_footer_renderer.go`), **not** the `ui.go` `HelpModal*` constants — the cmux footer pane does not use `renderHelpModal()`. | devops-engineer (F-006); junior-developer (P6-006); discovery notes | Evidenced — code survey: `cmux_footer_machine.go`/`cmux_footer_renderer.go`; `ui/ui.go:48–70`. |
  | CL-9 | The classified mid-run cmux diagnostic (D5) is written via `logger.Log()` followed by an explicit `bufio` flush (or `log.Writer()` direct write) before any subsequent abort step; `os.Exit()` is prohibited on the abort path because it skips the deferred `log.Close()` flush. | devops-engineer (F-001, F-002, F-004) | Evidenced — `logger.go:49,95,116` `bufio.Writer` buffering; `cmux_pane.go:169` deferred `Close()`; `cmux_pane.go:127` cobra `RunE` error→exit-code chain. |
  | CL-10 | OQ-2 resolves to: mocked unit tests (injectable threshold/cadence) + the mandatory manual in-pane gate for any change to `internal/cmuxctl` or `internal/interactionchannel` + a manual test script; **no live macOS-runner cmux CI job** — cmux has no headless mode and standard runners lack a cmux server. | test-engineer (F-005, OQ-2 resolution); devops-engineer (F-005); junior-developer (P6-010) | Evidenced — `make ci` is hermetic today; cmux v0.64.7 has no `--headless`; both specialists independently reached the same conclusion (not disputed). |
  | CL-11 | The two abort-path cmux call groups (run-aborted notification, sidebar clears) are non-fatal on every failure including timeout (D6); the abort cannot recurse; `sidebar.ClearAll` must stay idempotent because the abort path's explicit clear and the existing deferred safety-net clear can both run. | concurrency-analyst (notes existing pattern); devops-engineer (F-004); junior-developer (P6-011) | Evidenced — decision-log D6; `docs/coding-standards/concurrency.md` two-layer-cleanup pattern; existing `FireRunAborted`/`ClearAll` log-and-continue shape. |
  | CL-12 | Spec opening-paragraph phrasing "introduces no new operator-facing feature" is loosely inaccurate (the "run aborted" pane line and help disclosure are operator-visible); the spec body describes both accurately. Cosmetic spec-wording note, not a behavioral gap. | junior-developer (P6-007) | Anecdotal — spec-wording observation; no implementation impact. Recorded, not actioned. |

- **Open Questions raised:** The junior-developer surfaced P6-001…P6-011 as open design questions. **All resolve by evidence from within Round 1** — the concurrency-analyst (C1–C9), test-engineer (F-A–F-E, OQ-2), and devops-engineer (F-001–F-006) outputs, produced in parallel, directly answer them (see Resolution source below). No question required user input, junior-developer reframing beyond what was supplied, or a re-engagement round. The two specialists the junior-developer named as possible handoffs — `behavioral-analyst` (P6-003 termination-window) and `structural-analyst` (P6-005 shared constant) — were **not** engaged: the termination-window concern is settled by CL-3 (the liveness goroutine runs independently of `Run()` and keeps ticking through `Terminate()`'s ≤13s grace window, so no false stall), and the shared-constant question is a one-line `const` decision settled by CL-7.

- **Spec-maturity tags:** `plan-level` — 11 (CL-1…CL-11); `spec-level` — 0; `T#-contradiction` — 0 (no `feature-technical-notes.md` exists for Phase 6; the classification does not apply). `Anecdotal` — 1 (CL-12, cosmetic). **Spec-maturity gate: did NOT trip** (requires ≥5 spec-level findings by ≥3 specialists, or ≥2 `T#`-contradictions; neither condition is near).

- **Resolution source:**
  - CL-1 / P6-002, P6-009 — evidence (concurrency-analyst C1/C6; coding standard). → D-1, D-2.
  - CL-2 / P6-003 — evidence (concurrency-analyst C4; `workflow` code survey). Termination-window sub-concern — evidence (CL-3: liveness goroutine independent of `Run()`). → D-4.
  - CL-3 / P6-001 — evidence (concurrency-analyst C2; test-engineer G2). → D-3.
  - CL-4 — evidence (concurrency-analyst C3; test-engineer F-A). → D-3.
  - CL-5 / P6-008 — evidence (concurrency-analyst C7; coding standard). → D-6.
  - CL-6 / P6-004 — evidence (concurrency-analyst C5; `real.go` survey). → D-7 (trivial).
  - CL-7 / P6-005 — evidence (concurrency-analyst C8/C9; test-engineer F-E; spec F16). → D-5.
  - CL-8 / P6-006 — evidence (devops-engineer F-006; code survey). → D-10.
  - CL-9 — evidence (devops-engineer F-001/F-002). → D-11.
  - CL-10 / P6-010 — evidence (test-engineer + devops-engineer agreed independently). → D-12 (trivial).
  - CL-11 / P6-011 — evidence (decision-log D6; coding standard). → D-8 (trivial).
  - CL-12 / P6-007 — recorded as a non-blocking cosmetic note; no resolution required. → plan Open Items OI-2.
  - Mid-run cmux error classification (spec D5) reusing the existing `cmuxctl/preflight.go` vocabulary — evidence (discovery-notes `preflight.go` survey). → D-9 (trivial).

- **Decisions produced:** D-1 (abort gate — `sync.Once` + cause capture), D-2 (`runCompleted` post-run-window guard), D-3 (liveness emitter + per-connection stall timer), D-4 (abort routing — `Terminate()` + `ActionQuit` + `ExitReasonAborted`), D-5 (pane-side `run aborted` token + `Aborted bool` on `WorkspaceDone`), D-6 (`Liveness{}` message type + `"liveness"` wire constant), D-7 (in-flight cmux call satisfied by the serial queue — trivial), D-8 (abort-path cmux calls non-fatal — trivial), D-9 (mid-run cmux error classification reuses preflight vocabulary — trivial), D-10 (D7 disclosure target = footer pane inline help), D-11 (abort-diagnostic `bufio` flush + `os.Exit()` prohibition), D-12 (OQ-2 test strategy — mocked + manual gate, no live CI — trivial). Full: D-1, D-2, D-3, D-4, D-5, D-6, D-10, D-11 (8). Trivial: D-7, D-8, D-9, D-12 (4).
- **Changed in plan:** All sections — initial plan authored this round: Source Specification, Outcome, Context, Team Composition and Participation, Implementation Approach (Architecture and Integration Points, Data Model and Persistence, Runtime Behavior, External Interfaces), Decomposition and Sequencing (W1–W6), RAID Log (Risks R1–R6, Assumptions A1–A4, Issues I1–I2, Dependencies Dep1–Dep6), Testing Strategy, Security Posture, Operational Readiness, Definition of Done, Specialist Handoffs for Implementation, Deferred (YAGNI), Open Items, Summary.
- **Project-manager next-step recommendation:** **Go to synthesis.** The spec-maturity gate did not trip; every Open Question resolved by evidence within the round; no specialist handoff remains genuinely unsatisfied; no disputed claims. Round cap for a Medium feature is 2 — one round was sufficient.

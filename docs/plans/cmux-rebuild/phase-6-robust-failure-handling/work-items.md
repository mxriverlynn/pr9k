# Work Items — Phase 6: Robust Failure Handling

These work items break the [Phase 6 implementation plan](feature-implementation-plan.md) into independently grabbable slices. Phase 6 hardens the cmux multi-process architecture so every failure mode — a dying display pane, an operator pane-close, a stalled interaction channel, a hung or erroring cmux API call, and orchestrator process death — converges on one predictable, inspectable abort. The whole phase ships as a single PR; W-4's liveness wiring is a go/no-go merge gate verified by the W-7 manual gate.

Work items are numbered `W-N` for cross-reference only. `Depends on` lines refer to other work items in this file.

## Shared reference artifacts

These artifacts apply to more than one work item. Each work item's own `**References.**` block lists the subset and the specific section the implementer should jump to.

- **Implementation plan** — [feature-implementation-plan.md](feature-implementation-plan.md). The Architecture and Integration Points, Runtime Behavior, and Decomposition (Work Units W1–W6) sections name every file and behavior. Plan-level decisions are cited inline in each work item as `See plan: D-N` breadcrumbs.
- **Feature specification** — [feature-specification.md](feature-specification.md). Defines the failure-handling behaviors (display loss aborts the run, channel-stall threshold, exactly one run-aborted notification, the `run aborted` token, the pane-close disclosure).
- **Concurrency standard** — [../../../coding-standards/concurrency.md](../../../coding-standards/concurrency.md). Stop-drain-Reset timer pattern, `context.WithCancel` + `WaitGroup` join, `sync.Once` / `atomic.Bool` usage, non-blocking channel sends. Applies to W-2, W-3, W-4.
- **Testing standard** — [../../../coding-standards/testing.md](../../../coding-standards/testing.md). Race detector required (`go test -race ./...`), no `time.Sleep` in tests (inject thresholds instead), `make ci` is the gate. Applies to W-2 through W-7.
- **Error-handling standard** — [../../../coding-standards/error-handling.md](../../../coding-standards/error-handling.md). Package-prefixed messages, file paths in I/O errors, unclassified fallback preserves the raw code verbatim. Applies to W-4, W-5.
- **Documentation standard** — [../../../coding-standards/documentation.md](../../../coding-standards/documentation.md). Feature docs ship with the feature; update `CLAUDE.md` when adding a doc file. Applies to W-6.
- **Versioning standard** — [../../../coding-standards/versioning.md](../../../coding-standards/versioning.md). No automatic `version.Version` bump; the `interactionchannel` IPC protocol is internal to one process tree and not part of pr9k's public API. Applies to W-1, W-6.

## W-1 — Interaction-channel message-layer additions

**Summary.** Add the wire-level message types every later work item depends on: a zero-field `Liveness{}` heartbeat message, an `Aborted bool` field on the existing `WorkspaceDone`, and a shared exported `RunAbortedToken` constant. All changes are mechanically additive — no existing message type or wire shape changes. See plan: D-5, D-6 (and Work Unit W1).

**Description.**
1. In `src/internal/interactionchannel/messages.go`, add a `Liveness{}` struct with zero fields, implementing the `Message` interface; its `WireType()` returns the new wire constant `"liveness"`.
2. In the same file, add an `Aborted bool` field to the existing `WorkspaceDone` struct, alongside the existing `ExitCode int`.
3. In `src/internal/interactionchannel/framing.go`, register a `"liveness"` case in `UnmarshalMessage`'s `"type"`-discriminator dispatch. The addition is additive — every existing case is unchanged.
4. Add an exported `RunAbortedToken = "run aborted"` constant (in `channel.go` or a shared cmux location — a one-line placement decision). This is the token panes render and tests assert against.

**Note on scope boundary with W-2.** This work item adds only the message *types and constant*. The stall timer that consumes `Liveness{}` arrivals, and the `FakeInteractionChannel` test-double additions, belong to W-2.

**References.**
- **Plan** — [feature-implementation-plan.md](feature-implementation-plan.md), Architecture and Integration Points (`messages.go` / `framing.go` bullets) and the Decomposition table row W1.
- **Spec section** — [feature-specification.md](feature-specification.md), the failure-handling behaviors for the liveness signal and the `run aborted` token.
- **Standard** — [../../../coding-standards/versioning.md](../../../coding-standards/versioning.md): the IPC additions are internal; no version bump.

**Tests.**
- Unit test: a `Liveness{}` value round-trips through `framing.go` serialize → `UnmarshalMessage` and decodes back to a `Liveness{}`.
- Unit test: `WorkspaceDone{Aborted: true}` round-trips and decodes with `Aborted == true`.
- Unit test: a zero-value `WorkspaceDone` decodes with `Aborted == false` (the field defaults safely on messages written before the field existed).
- Unit test: every existing `interactionchannel` message-type test still passes (no wire-shape regression).

**Acceptance criteria.**
- [ ] `Liveness{}` exists, implements `Message`, and `WireType()` returns `"liveness"`.
- [ ] `WorkspaceDone` has an `Aborted bool` field; zero value decodes `false`.
- [ ] `UnmarshalMessage` dispatches the `"liveness"` wire type; existing cases unchanged.
- [ ] `RunAbortedToken = "run aborted"` is exported.
- [ ] `cd src && go test -race ./internal/interactionchannel/...` passes.

**Depends on.** `None.`

## W-2 — Per-connection stall detection in `readLoop`

**Summary.** Add a per-connection stall timer to `readLoop` on both the orchestrator and display-pane sides: a `time.Timer` with an injected threshold (exported `StallThreshold = 45 * time.Second` default) and an injected `onStall` callback, reset on every received message. Add `HoldMessages()` / `ReleaseMessages()` to `FakeInteractionChannel` so tests can wedge a channel deterministically. This makes the channel declare a silent connection lost after the threshold. See plan: D-3 (and Work Unit W2).

**Description.**
1. In `src/internal/interactionchannel/channel.go`, give the per-connection `readLoop` a `time.Timer` created with an injected threshold value (default the exported `StallThreshold = 45 * time.Second`) and an injected `onStall` callback.
2. Reset the timer on every received message using the Stop-drain-Reset pattern (Stop, drain the channel if Stop returned false, then Reset) — see the concurrency standard.
3. On timer expiry, invoke `onStall`. The existing `ReadyHandshakeTimeout` and the clean-disconnect (EOF/broken-connection) path are unchanged — a clean disconnect must still be detected promptly without waiting out the stall threshold.
4. In `src/internal/interactionchannel/fake.go`, add `HoldMessages()` and `ReleaseMessages()` to `FakeInteractionChannel` so a test can wedge a channel (suppress message delivery) deterministically and then release it.

**Note on injectability.** The threshold and `onStall` callback are injectable *for tests only* — production always uses the fixed exported `StallThreshold` constant. This is not an operator-facing configuration knob. See plan: the Deferred (YAGNI) note distinguishing test injection from a deferred operator knob.

**References.**
- **Plan** — [feature-implementation-plan.md](feature-implementation-plan.md), Architecture and Integration Points (`channel.go` / `fake.go` bullets), Runtime Behavior (Launch and steady state), and Decomposition row W2; RAID R3 (goroutine/timer leak).
- **Spec section** — [feature-specification.md](feature-specification.md), the 45-second channel-stall threshold and the requirement that a healthy-but-quiet channel must not stall.
- **Standards** — [../../../coding-standards/concurrency.md](../../../coding-standards/concurrency.md) (Stop-drain-Reset, timer lifecycle); [../../../coding-standards/testing.md](../../../coding-standards/testing.md) (no `time.Sleep`; inject the threshold).

**Tests.**
- Unit test (short injected threshold, no `time.Sleep`): a held/wedged channel fires `onStall` after the threshold elapses.
- Unit test: a message arriving inside the stall window resets the timer and no stall fires.
- Unit test: a clean disconnect (EOF/broken connection) is detected promptly, without waiting out the threshold.
- Unit test: the stall timer does not leak — it stops when `readLoop` ends (race detector exercises the early-return path).
- Race detector clean: `cd src && go test -race ./internal/interactionchannel/...`.

**Acceptance criteria.**
- [ ] `StallThreshold = 45 * time.Second` is exported; threshold and `onStall` are injectable for tests.
- [ ] The `readLoop` timer is reset Stop-drain-Reset on every received message.
- [ ] `onStall` fires exactly once on expiry; a clean disconnect is still detected promptly.
- [ ] `FakeInteractionChannel` has `HoldMessages()` / `ReleaseMessages()`.
- [ ] No `time.Sleep` in the tests; `cd src && go test -race ./internal/interactionchannel/...` passes.

**Depends on.** `W-1.`

## W-3 — Abort gate and `ExitReasonAborted`

**Summary.** Build the one-way abort transition: an `abortGate` struct wrapping `sync.Once` with the failure cause captured inside the `Do` closure, a `runCompleted atomic.Bool` post-run-window guard, and a new `workflow.ExitReasonAborted` value distinct from `ExitReasonUserQuit`. This is the structural guarantee that concurrent failure detections produce exactly one abort sequence. See plan: D-1, D-2, D-4 (and Work Unit W3).

**Description.**
1. In `src/cmd/pr9k/`, add an `abortGate` struct wrapping a `sync.Once`. `gate.trigger(cause AbortCause)` runs the `Do` closure that captures `cause` into a field and sets an atomic flag; every later `trigger` is a no-op. Add `IsAborting() bool` (reads the atomic flag) and `Cause() AbortCause` (reads the captured cause) accessors. `AbortCause` is a typed value carrying the classified reason for the diagnostic.
2. Add a `runCompleted atomic.Bool` flag. It will be set the instant `workflow.Run()` returns, before any terminal notification or `WorkspaceDone` broadcast — the wiring of the *set site* and the *check sites* is W-4 work, but the flag type and contract are defined here.
3. In `src/internal/workflow/exit_reason.go`, add `ExitReasonAborted` as a typed-string constant alongside `ExitReasonCompleted` / `ExitReasonLoopBroken` / `ExitReasonUserQuit`.
4. In `src/internal/workflow/run.go`, return `RunResult{ExitReason: ExitReasonAborted}` when the `Run()` loop exits because of an `ActionQuit` raised by the abort gate, distinguished from an operator-raised `ActionQuit` by the gate state. `Runner.Terminate()` is reused unchanged.

**Note on scope boundary with W-4.** This work item delivers the gate, the flag *type*, and the exit-reason constant as standalone, unit-testable units. Threading the gate into the four detection paths and setting `runCompleted` at the `Run()` return site is W-4.

**References.**
- **Plan** — [feature-implementation-plan.md](feature-implementation-plan.md), Architecture and Integration Points (abort gate, `runCompleted`, `src/internal/workflow/` bullets), Runtime Behavior (Abort detection and routing), Decomposition row W3; RAID R1 (missed-path double-fire), R2 (post-run-window race), R4 (`ExitReason` switch exhaustiveness).
- **Spec section** — [feature-specification.md](feature-specification.md), the requirement that exactly one run-aborted notification fires per aborted run, and that the infrastructure-abort path stays distinguishable from operator-quit.
- **Standards** — [../../../coding-standards/concurrency.md](../../../coding-standards/concurrency.md) (`sync.Once`, `atomic.Bool`); [../../../coding-standards/testing.md](../../../coding-standards/testing.md) (`-race` stress tests).

**Tests.**
- Unit test (`-race`, stress): many concurrent `gate.trigger` calls produce exactly one `Do` execution; the first cause wins and is the value `Cause()` returns.
- Unit test: `IsAborting()` returns `false` before any trigger and `true` after.
- Unit test: `workflow.Run` returns `ExitReasonAborted` (distinct from `ExitReasonUserQuit`) when the loop exits on a gate-raised `ActionQuit`, and still returns `ExitReasonUserQuit` for an operator-raised quit.
- Unit test: `runCompleted` is observably settable and readable as an `atomic.Bool` (the set-before-notification ordering is asserted in W-4's integration test).

**Acceptance criteria.**
- [ ] `abortGate` is `sync.Once`-guarded; `trigger` is the single entry point; the first cause wins under `-race` stress.
- [ ] `IsAborting()` and `Cause()` accessors exist and reflect the transition.
- [ ] `runCompleted atomic.Bool` exists with a defined contract.
- [ ] `workflow.ExitReasonAborted` exists, distinct from `ExitReasonUserQuit`; `run.go` returns it on a gate-raised `ActionQuit`.
- [ ] `cd src && go test -race ./...` passes for the touched packages.

**Depends on.** `None.`

## W-4 — Orchestrator wiring: liveness emitter and abort routing

**Summary.** Wire the moving parts together: start the liveness emitter goroutine, thread the abort gate into every failure-detection path, set `runCompleted` at the `Run()` return site, and add the `ExitReasonAborted` branch that runs the five-step abort sequence. This is the work unit that makes every survivable failure converge on one clean abort, and it is the **go/no-go merge gate** — if the liveness emitter does not demonstrably keep a quiet channel healthy (verified in W-7), the PR does not merge. See plan: D-2, D-3, D-4, D-7, D-11 (and Work Unit W4).

**Description.**
1. In `src/cmd/pr9k/cmux_pane.go`, `runCmuxOrchestratorWith` constructs the `abortGate` (W-3) and the `runCompleted` flag, and starts the liveness emitter goroutine **after** `ch.AwaitReady` and **before** `runCmuxWorkflowAdapted`. The emitter broadcasts a `Liveness{}` message to every pane every `LivenessCadence = 10 * time.Second`, in every workflow phase (including between steps and while paused at a Phase 3 error-mode prompt). It runs on a `context.WithCancel`-derived context and is joined by a `WaitGroup` on every exit path (graceful, early-return, panic) — see RAID R3.
2. Pass the abort gate into the workflow adapter and into the channel's per-connection setup so the per-connection `onStall` callback and the display-loss handler can call `gate.trigger`.
3. Set `runCompleted` (`atomic.Bool`, W-3) the instant `workflow.Run()` returns, before any terminal notification or `WorkspaceDone` broadcast. Every detection path checks `runCompleted.Load()` before calling `gate.trigger`; a post-run failure is dropped.
4. Wire the three live detection paths to `gate.trigger`, each gated on `runCompleted`: (a) the orchestrator's cmux-call wrapper triggers on a steady-state `*cmuxctl.TimeoutError`, `*CmuxError`, or `*PlaintextError`; (b) the per-connection `onStall` callback triggers `causeChannelStall`; (c) the display-loss handler triggers `causeDisplayLoss`. The first `trigger` to win calls `runner.Terminate()` (SIGTERM→SIGKILL grace window) to unblock the synchronous subprocess wait inside `Run()`, then sends `ActionQuit`.
5. In `src/cmd/pr9k/cmux_workflow.go`, add an `ExitReasonAborted` branch to `runCmuxWorkflowAdapted` (alongside the existing `UserQuit` / `Completed` / `LoopBroken` branches) that runs the abort sequence in order: (1) write the classified diagnostic from `gate.Cause()` via `logger.Log()`, then an explicit `bufio` flush; (2) `notifier.FireRunAborted(ctx)` — fires exactly once, every failure including timeout non-fatal and logged; (3) `sidebar.ClearAll(ctx)` — non-fatal, a partial clear is an acceptable degraded state; (4) broadcast `WorkspaceDone{ExitCode: 1, Aborted: true}`; (5) return through the cobra `RunE` error chain with a non-zero code. **No `os.Exit()` on the abort path** — the deferred `log.Close()` flush must run.
6. Abort-path cmux calls (`FireRunAborted`, `ClearAll`) pass `context.Background()` so the serial-queue's `ctx.Done()` escape does not drop them.

**Note on the go/no-go gate.** If the liveness emitter does not keep a quiet channel under the 45-second stall threshold in the W-7 manual gate, the stall timer would false-abort real workflows. The merge is gated on that result — see RAID R6 and W-7.

**References.**
- **Plan** — [feature-implementation-plan.md](feature-implementation-plan.md), Architecture and Integration Points (`src/cmd/pr9k/cmux_*.go` bullets), Runtime Behavior (Launch and steady state, Abort detection and routing, Abort sequence, Post-run window), Decomposition row W4; RAID R1, R2, R3, R6 and Assumptions A1, A2, A3.
- **Spec section** — [feature-specification.md](feature-specification.md), the convergence requirement (every failure mode produces one clean abort), exactly-one run-aborted notification, abort-path calls non-fatal, the workspace stays open after an abort.
- **Standards** — [../../../coding-standards/concurrency.md](../../../coding-standards/concurrency.md) (`context.WithCancel` + `WaitGroup` join, non-blocking sends); [../../../coding-standards/error-handling.md](../../../coding-standards/error-handling.md) (abort-path call failures logged and continued); [../../../coding-standards/testing.md](../../../coding-standards/testing.md) (injectable cadence, no `time.Sleep`, `-race`).

**Tests.**
- Integration test (`FakeClient` + `FakeInteractionChannel`): a held/wedged channel triggers exactly one abort sequence — one `FireRunAborted`, one `ClearAll`, one `WorkspaceDone{Aborted: true}`.
- Integration test: a quiet workflow step with the liveness emitter running never aborts (the false-abort guard — also a Definition-of-Done item).
- Integration test (`-race`): near-simultaneous pane-death + cmux-timeout produce exactly one `FireRunAborted`.
- Integration test: a pane loss observed after `runCompleted` is set fires no run-aborted notification.
- Integration test: `os.Exit()` is not called on the abort path; the deferred `log.Close()` runs.
- Race detector clean: `cd src && go test -race ./...`.

**Acceptance criteria.**
- [ ] The liveness emitter starts after `AwaitReady`, broadcasts `Liveness{}` every 10s in every phase, and is ctx-cancelled + `WaitGroup`-joined on every exit path.
- [ ] `runCompleted` is set the instant `Run()` returns, before any terminal notification; every detection path checks it before `gate.trigger`.
- [ ] All three live detection paths route through `gate.trigger`; the winning trigger calls `Terminate()` then `ActionQuit`.
- [ ] `runCmuxWorkflowAdapted` routes `ExitReasonAborted` through the five-step abort sequence with the `bufio` flush and no `os.Exit()`.
- [ ] Abort-path cmux calls pass `context.Background()`.
- [ ] All W-4 integration tests pass under `-race`.

**Depends on.** `W-1, W-2, W-3.`

## W-5 — Pane-side abort rendering and mid-run cmux error classification

**Summary.** Make every display pane and the footer pane render a `run aborted` line and exit on both the orchestrator-death path (their own stall timer) and the clean-abort path (`WorkspaceDone{Aborted: true}`). Classify the mid-run cmux error by reusing the existing `preflight.go` vocabulary so the abort diagnostic names a real cause. See plan: D-5, D-9 (and Work Unit W5).

**Description.**
1. In `src/cmd/pr9k/cmux_pane.go`, give `runCmuxDisplayPaneWith` and `runCmuxFooterPaneWith` a `stalledCh` select arm (fed by the pane's own `readLoop` stall timer from W-2). On a `stalledCh` fire — or on receiving `WorkspaceDone{Aborted: true}` — the pane renders a final line containing `RunAbortedToken` (`"run aborted"`) on its own surface and **returns**. It must not block on `ctx.Done()`. No coordination between panes — each renders the same token independently.
2. In `src/cmd/pr9k/cmux_footer_machine.go`, add the matching `stalledCh` arm to the footer machine's select loop.
3. Wire the mid-run cmux error classifier: the orchestrator's cmux-call wrapper (W-4) reuses `src/internal/cmuxctl/preflight.go`'s existing `classifyDialError` / `classifyIdentifyError` vocabulary (access-denied, `auth_*`, `method_not_found` / `unknown_method`). A recognized code classifies to the named cause; an unrecognized code produces an unclassified diagnostic that preserves the raw code verbatim.

**Note on the orchestrator-death path.** When the orchestrator process dies, no in-process code runs — no gate, no broadcast. Each pane's `readLoop` detects the lost channel (a clean disconnect promptly, or the 45s stall timer) and this is what feeds `stalledCh`. The pane behaves identically to the clean-abort path from the operator's view: `run aborted` rendered, pane exits.

**References.**
- **Plan** — [feature-implementation-plan.md](feature-implementation-plan.md), Architecture and Integration Points (`cmux_pane.go` pane-side `stalledCh` bullet, `cmux_footer_machine.go` bullet, the `cmuxctl` no-modification note), Runtime Behavior (Pane-side detection and rendering, Orchestrator death), Decomposition row W5; RAID R5 (pane renders but does not return).
- **Spec section** — [feature-specification.md](feature-specification.md), the `run aborted` token rendered in each pane, mid-run cmux error classification, and the unknown-code fallback.
- **Standards** — [../../../coding-standards/error-handling.md](../../../coding-standards/error-handling.md) (unclassified fallback preserves the raw code verbatim); [../../../coding-standards/testing.md](../../../coding-standards/testing.md) (`-race`, pane function must return).

**Tests.**
- Unit test: `runCmuxDisplayPaneWith` renders a final line containing the exact token `run aborted` and the function **returns** on a `stalledCh` fire.
- Unit test: same pane returns on receiving `WorkspaceDone{Aborted: true}`.
- Unit test: `runCmuxFooterPaneWith` and the footer machine render the token and return on both paths.
- Unit test: a `*CmuxError` with a recognized code (`access-denied`, `auth_*`, `method_not_found`) classifies to the named cause.
- Unit test: a `*CmuxError` with an unrecognized code produces an unclassified diagnostic with the raw code verbatim.

**Acceptance criteria.**
- [ ] Each display pane and the footer pane render a final line containing `run aborted` and return on a `stalledCh` fire and on `WorkspaceDone{Aborted: true}` — they do not block on `ctx.Done()`.
- [ ] The footer machine's select loop has a `stalledCh` arm.
- [ ] A recognized mid-run cmux code classifies to a named cause; an unrecognized code is reported as an unclassified cmux error with the raw code verbatim.
- [ ] `cd src && go test -race ./...` passes for the touched packages.

**Depends on.** `W-1, W-2, W-4.`

## W-6 — Test-suite cascade and documentation

**Summary.** Carry the new signatures, the `ExitReasonAborted` constant, and the new firing assertions through the existing test suite; audit every `RunResult.ExitReason` switch in the codebase for the new case; ship the operator and maintainer documentation; and write the manual test script the W-7 gate runs. See plan: D-10 (and Work Unit W6).

**Description.**
1. Update `cmux_*_test.go`, the `interactionchannel` tests, and the `workflow` tests for the new signatures, the `ExitReasonAborted` constant, and the new firing assertions introduced by W-1 through W-5.
2. **`ExitReason` switch audit (RAID R4).** Go does not compiler-enforce exhaustiveness for string constants. Grep the codebase for every `switch` on `RunResult.ExitReason` (and any other consumer of the `ExitReason` enum) and add the `ExitReasonAborted` case wherever it is missing, so an abort never falls through to a default branch with the wrong behavior.
3. Add the "closing a pr9k pane closes the run" disclosure line to `buildFooterHelpLines()` in `src/cmd/pr9k/cmux_footer_renderer.go` and to `docs/how-to/setting-up-cmux.md`. The `ui.go` `HelpModal*` constants are **not** changed — the disclosure target is the cmux footer pane's inline `?` help. See plan: D-10.
4. Add a Phase 6 section to `docs/features/cmux-mode.md` describing the failure-handling behavior (the abort gate, the liveness signal and stall threshold, the four detection paths, the `run aborted` token, orchestrator-death behavior). Update the `docs/features/cmux-mode.md` entry in the root `CLAUDE.md` if its description line needs the Phase 6 addition.
5. Write the manual test script (a runnable script or a documented procedure, per plan D-12) that exercises the three W-7 scenarios: a deliberately quiet step that must **not** false-abort, a deliberate channel wedge that must abort within the stall window, and a near-simultaneous pane-death + cmux-timeout that must produce exactly one run-aborted notification.

**References.**
- **Plan** — [feature-implementation-plan.md](feature-implementation-plan.md), Decomposition row W6, RAID R4 (`ExitReason` switch audit), Testing Strategy, Definition of Done checklist.
- **Spec section** — [feature-specification.md](feature-specification.md), the pane-close disclosure requirement (footer pane `?` help and `setting-up-cmux.md`).
- **Standards** — [../../../coding-standards/documentation.md](../../../coding-standards/documentation.md) (feature docs ship with the feature; update `CLAUDE.md` when adding a doc file); [../../../coding-standards/versioning.md](../../../coding-standards/versioning.md) (no `version.Version` bump); [../../../coding-standards/testing.md](../../../coding-standards/testing.md) (`make ci` is the gate).

**Tests.**
- The full suite passes: `cd src && go test -race ./...`.
- `make ci` passes (test, lint, format, vet, vulncheck, mod-tidy, build).
- Verification that every `ExitReason` switch handles `ExitReasonAborted` (grep audit confirmed; no default-branch fall-through).
- Doc check: code blocks in `setting-up-cmux.md` and `cmux-mode.md` match production code; `CLAUDE.md` links resolve.

**Acceptance criteria.**
- [ ] The whole test suite passes under `-race` with the new signatures and firing assertions.
- [ ] Every `RunResult.ExitReason` switch in the codebase handles `ExitReasonAborted`.
- [ ] The pane-close disclosure is in `buildFooterHelpLines()` and `docs/how-to/setting-up-cmux.md`; the `ui.go` `HelpModal*` constants are unchanged.
- [ ] `docs/features/cmux-mode.md` has a Phase 6 section; `CLAUDE.md` is updated if needed.
- [ ] The manual test script exists and exercises the quiet-step non-abort, the channel wedge, and the near-simultaneous multi-failure case.
- [ ] `make ci` passes; no `version.Version` bump.

**Depends on.** `W-1, W-2, W-3, W-4, W-5.`

## W-7 — Mandatory manual in-pane gate

**Summary.** Run the W-6 manual test script against a real cmux installation as the go/no-go merge gate. The liveness quiet-step check is the gate: if a quiet workflow step false-aborts, the PR does not merge. This is the only HITL work item — it requires a human operating a real cmux server and observing pane behavior, which has no headless/CI substitute. See plan: the W4 go/no-go gate note and the Definition of Done.

**Description.**
1. With a real cmux installation (cmux v0.64.7 or compatible), run `pr9k --cmux --project-dir <repo>` against a real workflow and execute the three manual test script scenarios from W-6.
2. **Scenario 1 — quiet-step non-abort (the go/no-go gate).** Run a workflow with a deliberately quiet step (a long, output-silent Claude step) that stays silent past the 45-second stall threshold. The liveness emitter's 10-second cadence must keep every channel healthy and the run must complete normally with **no false abort**. If this fails, the PR does not merge.
3. **Scenario 2 — deliberate channel wedge.** Wedge a channel deliberately; confirm the run aborts within the stall window, the panes render `run aborted` and exit, exactly one run-aborted notification fires, the sidebar entries clear, the classified diagnostic is in the per-run log file, and the workspace stays open.
4. **Scenario 3 — near-simultaneous multi-failure.** Trigger a pane-death and a cmux-timeout near-simultaneously; confirm exactly one run-aborted notification fires (the abort gate absorbs the second detection).
5. Record the pass/fail result of each scenario — especially the Scenario 1 go/no-go outcome — as merge evidence. This run also confirms which `*CmuxError` codes appear in a live mid-run error, resolving the open question about the exact code strings (plan RAID A4 / I2).

**Note on why this is HITL.** cmux v0.64.7 has no headless mode and standard CI runners have no cmux server, so the timing-sensitive failure paths cannot be exercised in automated CI. A human running a real cmux server and observing real pane behavior is the only available gate — see plan Testing Strategy and the Deferred (YAGNI) live-cmux CI entry.

**References.**
- **Plan** — [feature-implementation-plan.md](feature-implementation-plan.md), the W4 go/no-go gate note under the Decomposition table, Testing Strategy (the manual in-pane gate paragraph), Operational Readiness (Rollout), Definition of Done (the final mandatory-manual-gate bullet); RAID R6 and Assumptions A1, A4.
- **Spec section** — [feature-specification.md](feature-specification.md), the 45-second stall threshold, the requirement a healthy-but-quiet channel must not false-abort, and exactly-one run-aborted notification.
- **Artifact** — the manual test script produced by W-6.
- **Setup** — [../../../how-to/setting-up-cmux.md](../../../how-to/setting-up-cmux.md) for installing and verifying cmux.

**Tests.**
- Manual: Scenario 1 (quiet-step non-abort) — pass means the run completes with no false abort. This is the go/no-go gate.
- Manual: Scenario 2 (channel wedge) — pass means a clean convergent abort within the stall window with exactly one notification and the diagnostic in the log file.
- Manual: Scenario 3 (near-simultaneous multi-failure) — pass means exactly one run-aborted notification.

**Acceptance criteria.**
- [ ] All three manual scenarios run against a real cmux and their pass/fail results are recorded as merge evidence.
- [ ] Scenario 1 passes — a quiet step does not false-abort (the go/no-go gate).
- [ ] Scenario 2 passes — a deliberate wedge produces a clean convergent abort within the stall window.
- [ ] Scenario 3 passes — near-simultaneous failures produce exactly one run-aborted notification.
- [ ] The live mid-run `*CmuxError` codes observed are noted against the classifier (RAID A4 / I2).

**Depends on.** `W-1, W-2, W-3, W-4, W-5, W-6.`

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

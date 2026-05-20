# Implementation Decision Log: Phase 2 — First Real Workflow Runs End-to-End in Cmux

This file records every implementation decision committed while planning Phase 2 of pr9k cmux mode. Behavioral and implementation statements live in [../feature-implementation-plan.md](../feature-implementation-plan.md) — this file captures the question, rationale, evidence, and rejected alternatives for each decision. Round-by-round history lives in [implementation-iteration-history.md](implementation-iteration-history.md).

Phase 2 inherits all Phase 1 implementation decisions D-1 through D-28 from [`../../phase-1-workspace-lifecycle/artifacts/implementation-decision-log.md`](../../phase-1-workspace-lifecycle/artifacts/implementation-decision-log.md) unchanged. The parent spec's decisions [D1, D4, D8, D13, D14, D16, D17, D20, D26](../../artifacts/decision-log.md) and the parent technical notes [T3, T4, T5](../../artifacts/feature-technical-notes.md) are the behavioral commitments Phase 2 honors; this log records the new implementation choices Phase 2 makes on top of those commitments.

## Trivial decisions

- D-6: Add `ActionSkip` to `ui.StepAction` — extend the existing intent enum in `src/internal/ui/ui.go` with a `ActionSkip` constant so the footer can forward "skip current step" across the interaction channel rather than reaching into a process-local cancel function. — Referenced in plan: Architecture and Integration Points, Decomposition and Sequencing (U3, U5).
- D-17: Phase 2 absorbs version bump 0.10.0 → 0.11.0 — bump `src/internal/version/version.go` in this PR per `docs/coding-standards/versioning.md` (Phase 1's planned bump did not land; Phase 2's new public surface — interaction-channel sub-commands, real cmux-mode behavior — makes a MINOR bump correct under `0.y.z` rules). — Referenced in plan: Decomposition and Sequencing (U12), Definition of Done.
- D-18: Keep `pr9k v0.x.x` version label in cmux footer pane — render the version label in the footer pane's right corner the same way standard mode does in `docs/features/tui-display.md`, no protocol message needed because `version.Version` is a compile-time constant. — Referenced in plan: External Interfaces, Runtime Behavior.
- D-19: Fix `FakeClient.HangNext`/`HangRelease` mutex bug in Phase 2 prep — add `f.mu.Lock()/Unlock()` around the existing field accesses in `src/internal/cmuxctl/fake.go` so the race detector stays clean when Phase 2 extends `FakeClient` for new tests. — Referenced in plan: Decomposition and Sequencing (U10).
- D-22: Footer KeyHandler calls `SetStatusLineActive(true)` unconditionally — remove the `StatusLineActive` gate's effect on the `?` help key in cmux mode by unconditionally activating the status line in the footer's KeyHandler initialization, satisfying parent D20's commitment that help is always available. — Referenced in plan: Runtime Behavior, Decomposition and Sequencing (U8).

## Full decisions

### D-1: Unix domain socket per workspace for the interaction channel

- **Question:** What mechanism does the orchestrator pane use to push state to the three display panes and to receive intents from the footer pane (parent OQ-3)?
- **Decision:** A single Unix domain socket per workspace, owned by the orchestrator pane process. Each display pane process (header, log, footer) connects to the socket on startup. The socket carries the readiness handshake, orchestrator-to-pane state pushes, and pane-to-orchestrator intents. Loss detection is the socket's broken-connection signal.
- **Rationale:** Mature, easy to test (Unix sockets are first-class `net.Listener`/`net.Conn` types in Go), works bidirectionally without a second channel, and produces deterministic loss-detection semantics — a closed connection delivers `io.EOF` on read and an error on write. Fits the multi-process topology cleanly: each display pane process is spawned by cmux but learns the socket path via an environment variable supplied by the orchestrator at `SurfaceSpawn` time. The pr9k codebase already has Unix-socket precedent in `internal/cmuxctl/real.go` (the cmux JSON-RPC connection) and a `.discovery-notes.md` "existing similar patterns" list that names this as the obvious shape.
- **Evidence:** parent feature spec OQ-3 strong-default ("a Unix domain socket per workspace owned by the orchestrator"); `.discovery-notes.md` "Existing similar patterns" section; `src/internal/cmuxctl/real.go` (Unix-socket precedent); `docs/coding-standards/concurrency.md` (channel-based dispatch translates naturally to a socket-backed channel). Specialists: behavioral B1, junior OQ-3, concurrency C1+C4 (R1 claim ledger CL-1).
- **Rejected alternatives:**
  - Stdin/stdout pipes (Option B from parent OQ-3) — rejected because the orchestrator does not spawn the display panes (cmux does, via `SurfaceSpawn`); there is no parent-child pipe to inherit. Forcing one would require the orchestrator to relay every pane's stdio through cmux, contradicting parent T3 (cmux surfaces are independent processes).
  - Filesystem polling on shared state files (Option C from parent OQ-3) — rejected because polling cadence interacts with parent OQ-4 (stall-threshold) in ways the spec deferred to Phase 6; a stateful socket gives loss detection for free. Also fails the simpler-version test in reverse — a polling design is *more* code, not less, once stall-threshold detection is added.
  - One socket per display pane (three separate sockets) — rejected; adds three lifecycle objects and three readiness handshakes for no benefit; a single socket multiplexes by per-connection role identity per [D-4](#d-4-readiness-handshake-protocol--ready-message-with-role-identity).
- **Specialist owner:** `behavioral-analyst`
- **Revisit criterion:** if Phase 4 sidebar mirroring requires the orchestrator to push state to consumers outside its own workspace (e.g., a sidebar process spawned by cmux but not part of the workspace), revisit the per-workspace ownership model.
- **Dissent (if any):** —
- **Driven by rounds:** R1
- **Dependent decisions:** D-2, D-3, D-4, D-5, D-15, D-20, D-21
- **Referenced in plan:** Architecture and Integration Points, Runtime Behavior, External Interfaces, Decomposition and Sequencing (U1)

### D-2: Per-connection goroutines with independent buffered channels for state-push fan-out

- **Question:** How does the orchestrator fan out state pushes (header updates, log lines, footer mode updates) across three display-pane connections so a slow consumer does not block the others?
- **Decision:** One goroutine per display-pane connection, each fed by its own buffered channel. The orchestrator's write path publishes state-push messages to each channel non-blockingly (drop-oldest on full buffer for log lines; latest-wins for header and footer mode-state — see [D-15](#d-15-workspace-done-explicit-protocol-message) for the workspace-done message which is delivered with at-least-once semantics). Each per-connection goroutine reads from its channel and writes to its socket; a slow socket back-pressures only its own channel.
- **Rationale:** Parent T4 commits to "single-digit-millisecond cross-pane drift" — a sequential single-goroutine fan-out (write to header socket, then log socket, then footer socket) would force every pane to wait for the slowest, blowing the drift budget under realistic burst conditions. Concurrency C3 (R1 claim ledger CL-14) raised this explicitly. Per-connection goroutines with independent buffered channels is the standard Go fan-out idiom and matches `docs/coding-standards/concurrency.md`'s "snapshot-then-unlock" and "non-blocking channel sends" patterns. The topology choice honors T4 rather than contradicting it (OQ-2-T1 resolved by correct topology, not by T-note revision).
- **Evidence:** parent T4 (single-digit-ms drift); concurrency C3 (R1 claim ledger CL-14); `docs/coding-standards/concurrency.md` (non-blocking channel sends; mutex-protected shared writers); `internal/claudestream` precedent for per-step parallel writers.
- **Rejected alternatives:**
  - Sequential single-goroutine fan-out (one write loop iterates header → log → footer) — rejected; violates T4 under burst conditions because a slow log socket delays header and footer indefinitely.
  - Shared channel with header-routing tag inside each message — rejected; adds routing complexity without giving each consumer its own back-pressure surface; one slow consumer still slows all readers of the shared channel.
  - Unbuffered channels (synchronous handoff per pane) — rejected; orchestrator's workflow thread would block on the slowest pane, defeating the per-connection isolation.
- **Specialist owner:** `concurrency-analyst`
- **Revisit criterion:** if Phase 4 sidebar mirroring adds a fourth or fifth consumer that pushes the per-connection-goroutine count high enough to matter on a 30-pane cmux session, revisit (the goroutine count grows linearly with consumer count; that is acceptable through Phase 4 but worth measuring).
- **Dissent (if any):** —
- **Driven by rounds:** R1
- **Dependent decisions:** D-3, D-15
- **Referenced in plan:** Architecture and Integration Points, Runtime Behavior, Decomposition and Sequencing (U4)

### D-3: Separate read goroutine and write goroutine per bidirectional connection

- **Question:** How are reads and writes coordinated on the footer pane's bidirectional connection (state pushes orchestrator → footer; intents footer → orchestrator)?
- **Decision:** Separate goroutines per direction. Each side of the footer connection runs a dedicated read goroutine (reading framed messages off the socket, dispatching by message type) and a dedicated write goroutine (consuming a per-direction channel, writing framed messages onto the socket). The header and log connections are write-only from the orchestrator's perspective (no intents flow back) and require only one write goroutine on the orchestrator side plus one read goroutine on the pane side.
- **Rationale:** Concurrency C4 (R1 claim ledger CL-2, CL-4) documented the kernel-buffer deadlock: a single bidirectional goroutine that does `socket.Read` then `socket.Write` in a loop deadlocks when both sides have full socket-buffers and are each blocked in `Write` waiting for the other to `Read`. Splitting read and write into separate goroutines per direction breaks the deadlock cycle. This is the standard Go pattern for full-duplex socket I/O. Coordination between the two goroutines uses Go channels per `docs/coding-standards/concurrency.md` (channel-based dispatch).
- **Evidence:** concurrency C4 (R1 claim ledger CL-2, CL-4); `docs/coding-standards/concurrency.md`; Go standard library precedent (`net/http`, `net/rpc` use this split internally).
- **Rejected alternatives:**
  - Single bidirectional goroutine per connection — rejected per C4; kernel-buffer deadlock under burst.
  - One read goroutine + write requests via `select { case ch <- msg: default: drop }` from the read loop — rejected; couples read and write timing in a way that breaks back-pressure semantics for state pushes.
- **Specialist owner:** `concurrency-analyst`
- **Revisit criterion:** if the message volume on the header or log connection grows enough that the unidirectional choice ceases to suffice (e.g., Phase 4 sidebar mirroring adds inbound focus events), revisit.
- **Dissent (if any):** —
- **Driven by rounds:** R1
- **Dependent decisions:** D-4, D-15
- **Referenced in plan:** Architecture and Integration Points, Runtime Behavior, Decomposition and Sequencing (U1, U4)

### D-4: Readiness handshake protocol — `ready` message with role identity

- **Question:** What is the wire shape of the readiness handshake the orchestrator uses to gate the first state push (parent feature spec Primary Flow step 7, parent D16)?
- **Decision:** Each display pane connects to the socket and sends one JSON-encoded `{"type":"ready","role":"header"|"log"|"footer"}` message. The orchestrator maintains a mutex-protected `map[string]bool` tracking per-role readiness; when all three roles have signaled, the orchestrator unblocks the workflow thread. The handshake has a 10-second wall-clock deadline (aligned with the upper bound of parent D27's cmux per-call timeout range); on timeout the orchestrator fires a fatal-teardown sentinel through the same dismissal path Phase 1 established.
- **Rationale:** Per-role booleans (rather than a count-to-three) defend against the race concurrency C2 raised: a single pane disconnecting and reconnecting would otherwise be counted twice and unblock the handshake prematurely. The 10s deadline keeps the operator from waiting indefinitely if a pane fails to start (e.g., an `exec` error inside cmux's pane process). Aligning the deadline with parent D27 means operators see the same upper bound across cmux operations. The mutex matches `docs/coding-standards/concurrency.md`'s mutex-protected-writes pattern.
- **Evidence:** parent feature spec Primary Flow step 7; parent D16 (readiness-handshake commitment); parent D27 (cmux per-call timeout 5–10s); concurrency C1+C2 (R1 claim ledger CL-2); behavioral B2 (R1 claim ledger CL-2); test-engineer T1+T2+T3 (R1).
- **Rejected alternatives:**
  - Count-only (track an integer "three ready signals received") — rejected per C2; a reconnect double-counts and unblocks prematurely.
  - Unbounded wait — rejected per C1; a never-connecting pane would hang the orchestrator forever.
  - Per-pane sequential handshakes (orchestrator polls each socket in turn) — rejected; converts a concurrent problem into a sequential one and doubles the worst-case handshake time.
  - Heartbeat-style keepalive instead of one-shot ready — rejected; YAGNI for Phase 2, no committed need for periodic re-affirmation while a workflow is running.
- **Specialist owner:** `behavioral-analyst`
- **Revisit criterion:** if Phase 6 introduces stall-threshold detection (parent OQ-4) that wants to reuse the handshake protocol for periodic re-affirmation, extend the message type set rather than replacing the one-shot shape.
- **Dissent (if any):** —
- **Driven by rounds:** R1
- **Dependent decisions:** D-8, D-15
- **Referenced in plan:** Architecture and Integration Points, Runtime Behavior, Decomposition and Sequencing (U2)

### D-5: `KeyHandler` proxy adapter inside the orchestrator process

- **Question:** `workflow.Run` and `ui.Orchestrate` take a concrete `*ui.KeyHandler` by pointer (`src/internal/workflow/run.go:316`, `src/internal/ui/orchestrate.go:73`). How does the orchestrator pane process supply a `KeyHandler`-shaped object whose intents come from the footer pane process across the interaction channel?
- **Decision:** Construct a real `*ui.KeyHandler` inside the orchestrator process with a customized `cancel` callback. A new adapter goroutine in the orchestrator reads intent messages off the footer-connection read goroutine and translates each intent into the corresponding `KeyHandler` action: `ActionRetry`/`ActionContinue`/`ActionQuit`/`ActionSkip` push onto the existing `Actions` channel; mode transitions (`SetMode`, `SetStatusLineActive`) are pushed *outbound* to the footer via the write goroutine so the footer's local renderer can update its shortcut bar. The footer process holds its own thin `keysModel`-equivalent that handles `q`/`y` locally (per parent D20) and forwards only fully resolved intents.
- **Rationale:** Refactoring `workflow.Run` to accept an interface would ripple through every existing caller (standard mode `main.go`, tests, future modes) and contradicts the narrow-reading ADR (`docs/adr/20260410170952-narrow-reading-principle.md`) — Phase 2 should not reshape generic pr9k internals to accommodate cmux mode. The adapter pattern matches `docs/coding-standards/api-design.md`'s "adapter types" guidance and isolates the cross-process bridge to one new file in the orchestrator's package. Behavioral B4 and concurrency C5 (R1 claim ledger CL-4) both surfaced the contract gap; the adapter is the minimal cross-process bridge.
- **Evidence:** `src/internal/workflow/run.go:316`; `src/internal/ui/orchestrate.go:73`; `docs/adr/20260410170952-narrow-reading-principle.md`; `docs/coding-standards/api-design.md` (adapter types); behavioral B4 + concurrency C5 (R1 claim ledger CL-4).
- **Rejected alternatives:**
  - Refactor `workflow.Run` to take a `KeyHandler`-shaped interface — rejected; narrow-reading violation; ripples through every caller; forces every test to redo the same adapter wiring.
  - Forward raw `tea.KeyMsg` over the channel to the orchestrator — rejected; defeats parent D20 (footer owns the keyboard state machine) and leaks Bubble Tea details through what should be a typed protocol surface.
  - Have the footer compose `KeyHandler` directly and ship it across the channel — rejected; `KeyHandler` holds in-memory mutex state that does not serialize.
- **Specialist owner:** `behavioral-analyst`
- **Revisit criterion:** if Phase 3 (interactive error recovery) introduces new intent types that map awkwardly through the adapter, revisit; the adapter's intent-table is the natural extension point.
- **Dissent (if any):** —
- **Driven by rounds:** R1
- **Dependent decisions:** D-6, D-15
- **Referenced in plan:** Architecture and Integration Points, Runtime Behavior, Decomposition and Sequencing (U5)

### D-7: Logger creation inside the orchestrator pane process

- **Question:** Phase 1 deliberately skips `logger.NewLogger` on the cmux path (`src/cmd/pr9k/main.go:111-120`). Phase 2 must produce the same log artifacts the standard display produces (parent D17). Where in the cmux-mode code path is the logger created?
- **Decision:** A new `runCmuxOrchestrator` entry point (the orchestrator pane's main function) calls `logger.NewLogger(projectDir)` immediately after process startup, mirroring the standard mode's `startup()` shape (`src/cmd/pr9k/main.go:236-250`). The resulting `*logger.Logger` powers the `RunStamp`-named directory under `<projectDir>/.pr9k/logs/<run-stamp>/` and feeds the standard `workflow.Run` machinery. The footer pane's `statusline.Runner` is constructed with `nil` logger per [D-9](#d-9-statuslinerunner-constructed-with-nil-logger-in-cmux-footer); the orchestrator owns the only logger in the cmux topology.
- **Rationale:** Parent D17 commits to log artifacts unchanged in cmux mode; behavioral B8 (R1 claim ledger CL-8) and test-engineer T14 surfaced the gap. The orchestrator is the only process with a coherent view of the run's lifecycle (start, iteration count, finalize), so it is the natural logger owner. Mirroring `startup()` keeps the logger's existing semantics (per-run file, `RunStamp` accessor for artifact-directory naming) without reimplementing them.
- **Evidence:** parent D17 (log artifacts unchanged); `src/cmd/pr9k/main.go:111-120` (Phase 1 skip); `src/cmd/pr9k/main.go:236-250` (standard `startup()` shape); behavioral B8 + test-engineer T14 (R1 claim ledger CL-8); `internal/logger` package contract in `docs/code-packages/logger.md`.
- **Rejected alternatives:**
  - Keep Phase 1's logger-skip behavior in cmux mode — rejected; violates D17 directly.
  - Create the logger in `main()` before forking and pass the file handle to the orchestrator subprocess — rejected; orchestrator is spawned by cmux's `SurfaceSpawn`, not by `os.StartProcess` from pr9k, so file-descriptor inheritance is not under pr9k's control.
  - Create one logger per pane and concatenate at workspace-close time — rejected; per-pane logs do not match the standard-mode artifact layout; reassembly is fragile and adds Phase 1-killing complexity for no spec-commitment benefit.
- **Specialist owner:** `behavioral-analyst`
- **Revisit criterion:** if Phase 4 sidebar mirroring needs the logger from a non-orchestrator process, revisit; until then the orchestrator-owned model is sufficient.
- **Dissent (if any):** —
- **Driven by rounds:** R1
- **Dependent decisions:** D-8, D-13
- **Referenced in plan:** Architecture and Integration Points, Runtime Behavior, Data Model and Persistence, Decomposition and Sequencing (U6)

### D-8: Pre-populate first phase header state in the orchestrator before unblocking the handshake

- **Question:** Standard mode's `main.go` pre-populates the first phase's header state immediately before `program.Run()` (`src/cmd/pr9k/main.go:236-250`) so the header is never blank when the user sees the TUI. The cmux orchestrator's analogous moment is "after readiness handshake completes, before pushing the first state to panes." Should the orchestrator pre-populate?
- **Decision:** Yes. Inside the orchestrator's `runCmuxOrchestrator` entry point, after `logger.NewLogger` and before the readiness-handshake await, compose the first phase's header state (step list, iteration `1 of N`, phase name) using the same `RunHeader` interface the standard mode uses. After the handshake completes, push that pre-populated state to the header pane as the first state-push message. The header pane renders it the moment its socket receives the first frame.
- **Rationale:** Behavioral B3 (R1 claim ledger CL-3) named the mechanic leak: without pre-population, the header pane shows nothing during the handshake window and for the first event after — a visibly-blank header is a regression against standard mode. Mirroring the standard `main.go` shape costs ~10 lines of code and produces the same first-frame UX. The pre-population happens inside the orchestrator process, before any cross-process traffic, so it does not interact with the handshake's race window.
- **Evidence:** `src/cmd/pr9k/main.go:236-250` (standard pre-population); behavioral B3 (R1 claim ledger CL-3); parent D14 (operator-initiated closure assumes panes render something useful from t=0).
- **Rejected alternatives:**
  - Blank-header-during-handshake — rejected; visibly worse than standard mode; T4-acceptable but not necessary.
  - Pre-populate inside each pane's startup code — rejected; the pane processes do not have access to the workflow's phase structure until the orchestrator pushes state; mirroring inside the orchestrator is strictly cheaper.
- **Specialist owner:** `behavioral-analyst`
- **Revisit criterion:** if Phase 3 (error recovery) needs the orchestrator to delay its first state push for a different reason, revisit the pre-population position.
- **Dissent (if any):** —
- **Driven by rounds:** R1
- **Dependent decisions:** —
- **Referenced in plan:** Runtime Behavior, Decomposition and Sequencing (U4)

### D-9: `statusline.Runner` constructed with `nil` logger in cmux footer

- **Question:** The `statusline.Runner` type in `src/internal/statusline/statusline.go` embeds a `*logger.Logger` and logs diagnostic messages (script start, exec failure, timeout) to it. In cmux mode the logger lives in the orchestrator while the runner lives in the footer — cross-process. What does the footer pass for the logger?
- **Decision:** Construct `statusline.Runner` with `nil` for the logger argument. The runner's `logLine` method becomes a no-op when the logger is nil (or alternatively, callers guard the call site — implementation detail for the runner). The runner's standard error output remains visible to the operator in the footer pane on script failure, satisfying the operator-visible feedback contract; the on-disk log loses the runner's internal diagnostics. This is the replacement for the larger "forward diagnostics over the channel" alternative (YAGNI-7 in the sweep).
- **Rationale:** YAGNI sweep YAGNI-7 demonstrated that forwarding runner diagnostics to the orchestrator's logger is speculative — no operator has named the absence of these diagnostics as a problem; the visible footer pane already surfaces script failures via the runner's error path. The `nil`-logger pattern is one of pr9k's existing "configurable observability" idioms and avoids a new protocol message type. The implementation will require either a `logger == nil` guard in `Runner.logLine` or a `noopLogger` adapter; both are trivial.
- **Evidence:** YAGNI-7 in `.yagni-sweep.md`; behavioral B10 + concurrency C6 (R1 claim ledger CL-10); `src/internal/statusline/statusline.go:54-56` (existing logger field).
- **Rejected alternatives:**
  - Forward runner diagnostics over the channel via a new `runner_log` protocol message — rejected per YAGNI-7; speculative observability with no named need.
  - Construct a footer-local logger in `<projectDir>/.pr9k/logs/<run-stamp>/footer.log` — rejected; introduces a second logger file with no spec-committed consumer; parent D17 already commits to "log artifacts unchanged," not "log artifacts plus per-pane diagnostics."
  - Refactor `statusline.Runner` to not depend on `*logger.Logger` — rejected as scope creep; the nil-logger pattern is the minimum change.
- **Specialist owner:** `behavioral-analyst`
- **Revisit criterion:** if an operator reports that they need runner diagnostics in the persisted log file (e.g., for a status-line script that fails only on long-running production runs), reopen and consider the channel-forwarding alternative.
- **Dissent (if any):** —
- **Driven by rounds:** R1
- **Dependent decisions:** —
- **Referenced in plan:** Architecture and Integration Points, Runtime Behavior, Decomposition and Sequencing (U7)

### D-10: Drop heartbeat indicator in cmux Phase 2

- **Question:** Standard mode's header renders a `⋯ thinking (Ns)` heartbeat suffix on silent claude turns, driven by `HeartbeatReader` reading an `atomic.Int64` field on the in-process `Pipeline`. `atomic.Int64` does not cross process boundaries. Does cmux Phase 2 forward the heartbeat or drop it?
- **Decision:** Drop the heartbeat indicator in cmux mode. The header pane renders without the `⋯ thinking` suffix; iteration counter and step state continue to update as state pushes arrive. Forwarding is deferred (YAGNI-1).
- **Rationale:** YAGNI sweep YAGNI-1 demonstrated that no spec commitment requires the indicator in cmux mode, no incident has named its absence, and the simpler version (drop it) satisfies every spec commitment Phase 2 ships. Forwarding would add a 1Hz pusher goroutine in the orchestrator, an atomic-or-mutex-protected timestamp field in the header pane, and a new protocol message type — speculative cost for no committed benefit. Behavioral B13 and concurrency C7 (R1 claim ledger CL-12) surfaced the gap; both also flagged the option as drop-able.
- **Evidence:** YAGNI-1 in `.yagni-sweep.md`; `src/internal/ui/header.go:80-123` (HeartbeatReader); `src/cmd/pr9k/main.go:254`; behavioral B13 + concurrency C7 + test-engineer S2 (R1 claim ledger CL-12).
- **Rejected alternatives:**
  - 1Hz pusher goroutine forwarding `LastEventAt` over the channel — rejected per YAGNI-1; speculative observability without a named operator need.
  - Forward only on first silent turn (lazy activation) — rejected; same speculative cost with worse first-occurrence latency.
- **Specialist owner:** `behavioral-analyst`
- **Revisit criterion:** an operator reports that long silent claude turns in cmux mode look indistinguishable from a hung run, OR Phase 4 (sidebar mirroring) adds infrastructure that makes the forwarding effectively free.
- **Dissent (if any):** —
- **Driven by rounds:** R1
- **Dependent decisions:** —
- **Referenced in plan:** Architecture and Integration Points, Runtime Behavior, Decomposition and Sequencing (U4)

### D-11: Reuse `internal/uichrome` minimum-size thresholds per pane

- **Question:** Each cmux pane has its own SIGWINCH (parent T3 — independent processes); when a pane becomes too narrow to render its content, what threshold and behavior applies?
- **Decision:** Each pane uses the standard TUI's `MinTerminalWidth=60` and `MinTerminalHeight=16` constants from `internal/uichrome`. Below threshold, the pane renders a one-line advisory: "make this pane wider" (or equivalent), preserving the pane's process but skipping the normal render path. Above threshold, the pane resumes normal rendering.
- **Rationale:** User-confirmed in Round 2 (OQ-2-S1). Reuses an existing codebase constant (`internal/uichrome.MinTerminalWidth`) rather than introducing per-pane bespoke thresholds. The advisory message keeps the operator informed without erroring out; the pane process stays alive so the orchestrator's interaction channel does not see a connection-loss event for what is a UI-resize concern. The pattern matches `docs/coding-standards/tui-rendering.md`'s "plain-text-first ANSI composition" — a width-bounded advisory is a single styled line.
- **Evidence:** User input in R2 (`implementation-iteration-history.md#r2`); `src/internal/uichrome` constants (referenced from CLAUDE.md `docs/code-packages/uichrome.md`); `docs/coding-standards/tui-rendering.md`.
- **Rejected alternatives:**
  - Bespoke per-pane minimum thresholds — rejected; new magic numbers with no spec basis.
  - No minimum (let the pane render whatever fits) — rejected; the standard `internal/uichrome` thresholds exist for a reason (sub-60-column renders truncate critical glyphs); cmux mode inherits the same constraint.
  - Resize the cmux pane programmatically — rejected; not in pr9k's authority (cmux owns pane sizing; operator owns layout).
- **Specialist owner:** `behavioral-analyst`
- **Revisit criterion:** if operator reports indicate that the 60-column threshold is wrong for the footer (which renders the shortest content), revisit; defer until reports arrive.
- **Dissent (if any):** —
- **Driven by rounds:** R1, R2
- **Dependent decisions:** —
- **Referenced in plan:** Runtime Behavior, External Interfaces

### D-12: Help modal renders as inline expansion above the footer

- **Question:** Standard mode's help modal is a full-screen overlay. In a small footer pane (~2 rows), what visual form does the help modal take?
- **Decision:** Inline expand above the footer's normal content. Pressing `?` (or the configured help key) in the footer pane expands the footer's render upward, drawing the help-modal content bounded to the footer pane's available height. Pressing `?` again or `esc` collapses back to the normal footer content. The footer process owns the modal state; no protocol message crosses to the orchestrator.
- **Rationale:** User-confirmed in Round 2 (OQ-2-S2). Inline expansion stays inside the footer pane's bounds (no pane-overlay machinery, no cmux focus interaction), is bounded to available height, and matches the parent D20 commitment that the footer owns the keyboard state machine (including the help mode). The standard TUI's full-screen overlay does not translate to a multi-process layout where each pane is its own process; the inline-expand pattern is the smallest implementation that satisfies the spec's "help is available" commitment.
- **Evidence:** User input in R2 (`implementation-iteration-history.md#r2`); parent D20 (footer owns keyboard state machine, including help); behavioral B11+B12 + junior OQ-5 (R1 claim ledger CL-11, CL-19); `docs/features/cmux-mode.md` (parent feature-doc commitment to per-pane independence).
- **Rejected alternatives:**
  - Full-footer-takeover (replace the entire footer with the help modal, hide shortcuts) — rejected; the help modal can grow beyond the footer's height for verbose workflows; the inline-expand pattern handles this gracefully via height-bounding.
  - Defer help entirely until Phase 3 — rejected; parent D20 commits to help available in cmux mode; the spec is settled.
  - Spawn a fourth visible pane for the help modal — rejected; contradicts the four-pane scaffold; introduces transient cmux RPCs in the hot path.
- **Specialist owner:** `behavioral-analyst`
- **Revisit criterion:** if the help-modal content grows beyond what the footer's bounded height can show (truncation becomes a real issue), revisit; until then the inline-expand pattern is sufficient.
- **Dissent (if any):** —
- **Driven by rounds:** R1, R2
- **Dependent decisions:** D-22
- **Referenced in plan:** Runtime Behavior, External Interfaces, Decomposition and Sequencing (U8)

### D-13: Log-artifact equivalence comparison is content-modulo-run-specifics, not byte-identity

- **Question:** Parent D17 commits to log artifacts "byte-for-byte same" between standard and cmux mode. RunStamp directory names, wall-clock timestamps, and `lipgloss.Width()`-dependent renders inevitably diverge per-run. What is the testable acceptance criterion?
- **Decision:** Equivalent content modulo run-specifics. The acceptance criterion compares per-step JSONL artifacts and step-content lines line-by-line; it excludes RunStamp directory names, wall-clock timestamps inside log lines, and terminal-width-dependent renders (phase-banner underlines, header checkbox layout). The same workflow, run twice in standard mode and once in cmux mode, must produce per-step JSONL files whose content (minus the named exclusions) matches.
- **Rationale:** User-confirmed in Round 2 (OQ-2-10). Literal byte-identity is unsatisfiable for any non-trivial run — wall-clock timestamps differ between runs of the same workflow even in identical environments. The user's chosen wording ("equivalent content, modulo run-specifics") names the testable shape: per-step JSONL artifact content is the load-bearing comparison (it is what consumers — humans, scripts, the next plan-implementation pass — actually read), and the named exclusions are mechanically detectable. Behavioral B9 + concurrency C10 + test-engineer T5 + junior OQ-1 (R1 claim ledger CL-9) all surfaced the gap.
- **Evidence:** User input in R2 (`implementation-iteration-history.md#r2`); behavioral B9 + concurrency C10 + test-engineer T5 + junior OQ-1 (R1 claim ledger CL-9); parent D17 (log artifacts unchanged).
- **Rejected alternatives:**
  - Literal byte-identity — rejected per user input; unsatisfiable.
  - Drop the equivalence claim entirely — rejected; parent D17 commits to it, and the artifact content (the load-bearing slice) is genuinely equivalent.
  - Hash-based comparison after a normalization pass — rejected as over-engineered; line-by-line comparison with named exclusions is more diagnostic when it fails.
- **Specialist owner:** `test-engineer`
- **Revisit criterion:** if a workflow produces artifacts whose content the comparison rule cannot mechanically normalize (e.g., embedded random IDs not in the exclusion list), extend the exclusion list rather than tightening the comparison.
- **Dissent (if any):** —
- **Driven by rounds:** R1, R2
- **Dependent decisions:** —
- **Referenced in plan:** Architecture and Integration Points, Testing Strategy, Definition of Done

### D-14: Three test doubles with hooks scoped to planned tests

- **Question:** Phase 2's new code (interaction channel + display panes + footer-side intent routing) needs test seams. What test doubles are required, and how is their scope bounded?
- **Decision:** Three new test doubles, each in `internal/<package>/fake.go`-style files alongside the production code:
  - `FakeInteractionChannel` — scripted message exchange for orchestrator-side unit tests; hooks: `EnqueueReady(role string)`, `ExpectStatePush()`, `InjectIntent(intent)`, `Close()`.
  - `FakeDisplayPane` — scripted state acceptance for pane-side unit tests; hooks: `Connect()`, `SendReady()`, `ExpectMessage()`, `Disconnect()`.
  - `FakeFooterKeySource` — scripted key-event injection for footer-side state-machine tests; hooks: `Press(key)`, `Mode()`, `LastForwardedIntent()`.
  All three doubles use the existing `sync.Mutex`-protected pattern from `internal/cmuxctl/fake.go`. No speculative hooks: the hook set is bounded to exactly what the 14 planned test items (T1–T14, mapped in `Testing Strategy`) require.
- **Rationale:** Test-engineer (R1, CL-16) named the three doubles explicitly and capped the surface at "what the planned tests need." YAGNI sweep CL-16 + YAGNI named anti-pattern check confirmed that speculative hook addition is the temptation to resist. The mutex-protected pattern matches `docs/coding-standards/testing.md` (closeable idempotency tests, input immutability tests) and `internal/cmuxctl/fake.go`'s precedent.
- **Evidence:** Test-engineer (R1 claim ledger CL-16, CL-22); `docs/coding-standards/testing.md`; `internal/cmuxctl/fake.go` (existing FakeClient pattern).
- **Rejected alternatives:**
  - Speculative test hooks (e.g., `FakeInteractionChannel.SimulateLatency`, `FakeDisplayPane.SimulateRender`) — rejected per YAGNI named anti-pattern; "hooks for tests that don't exist yet."
  - One mega-double covering all three roles — rejected; cohesion is lower than three focused doubles; mock setup becomes harder to read.
  - Reuse `FakeClient` from `internal/cmuxctl` — rejected; that double is for cmux JSON-RPC, not the new interaction-channel protocol.
- **Specialist owner:** `test-engineer`
- **Revisit criterion:** when a Phase 3 test plan names a new behavior to test, add only the hook that behavior requires.
- **Dissent (if any):** —
- **Driven by rounds:** R1
- **Dependent decisions:** —
- **Referenced in plan:** Testing Strategy, Decomposition and Sequencing (U10)

### D-15: Workspace-done explicit protocol message with orchestrator-waits-for-ack

- **Question:** When `workflow.Run` returns in the orchestrator (the workflow has completed), how do the display panes distinguish "normal completion" from "display loss" (orchestrator crash, channel stall) so the footer can transition to ModeDone rather than firing a false-positive abort? When does the orchestrator close the interaction socket?
- **Decision:** The orchestrator sends an explicit `{"type":"workspace_done","exitCode":N}` protocol message to all three panes. Each pane process: (a) updates its local mode to ModeDone, (b) renders the final state, (c) sends a `{"type":"done_ack","role":...}` reply on its own connection. The orchestrator waits for all three acks (or a 5-second timeout) before closing the interaction socket. After socket close the display panes stay alive (per D-16) hosting their final-state render until the operator dismisses the workspace; Phase 1's `DismissalObserver` then fires.
- **Rationale:** Behavioral B14 + concurrency C8 + test-engineer T9 (R1 claim ledger CL-13) surfaced the false-positive-abort risk: a footer that observes its socket close without context would correctly assume display loss and enter abort behavior, even when the orchestrator finished normally. An explicit message + ack handshake disambiguates the two cases and gives the operator a stable "final state" view before any cmux-level dismissal action. The 5-second ack timeout protects against a pane process that died silently — the orchestrator does not hang forever waiting for an ack from a dead pane.
- **Evidence:** Behavioral B14 + concurrency C8 + test-engineer T9 (R1 claim ledger CL-13); parent D14 (operator-initiated closure); parent D26 (status-line script keeps running until pane process exits, which means panes outlive workflow.Run).
- **Rejected alternatives:**
  - Socket-close-implies-done (no explicit message) — rejected per C8; indistinguishable from display loss; false-positive aborts.
  - Implicit done detection via "no state pushes for N seconds" — rejected; reintroduces stall-threshold concerns deferred to Phase 6 (parent OQ-4).
  - Best-effort send without ack (fire-and-close) — rejected; a pane that hasn't yet processed the last state push (still draining its buffered channel) would miss the done message and behave incorrectly.
- **Specialist owner:** `behavioral-analyst`
- **Revisit criterion:** if Phase 5 (completion notification) needs the orchestrator to fire its notification before pane acks return, decouple the notification path from the ack wait but preserve the ack.
- **Dissent (if any):** —
- **Driven by rounds:** R1
- **Dependent decisions:** D-16
- **Referenced in plan:** Architecture and Integration Points, Runtime Behavior, Decomposition and Sequencing (U9)

### D-16: Reuse Phase 1's `DismissalObserver` unchanged

- **Question:** After `workflow.Run` returns and the orchestrator sends workspace-done, the display panes stay alive showing their final state. The operator's dismissal gesture (workspace-close or close-each-pane) then fires. Does Phase 2 reuse Phase 1's `DismissalObserver`, modify it, or replace it?
- **Decision:** Reuse Phase 1's `DismissalObserver` unchanged. The display panes stay alive after `workflow.Run` returns (they hold their final-state render and the status-line script keeps running per parent D26 in the footer); the observer detects workspace-removal OR per-pane exit exactly as in Phase 1. The orchestrator's own pane process exits cleanly after the ack-wait timeout (or all acks received); the observer treats that as one of the per-pane-exit signals it already handles.
- **Rationale:** Junior OQ-6 (R1 claim ledger CL-20) raised the question explicitly. Phase 1's observer is a poll-based design with workspace-removal AND per-pane-exit arms; both arms naturally cover Phase 2's lifecycle. Reusing the observer without modification means Phase 2 inherits Phase 1's race-detector-clean implementation, single-flight semantics, and `shuttingDown` flag without recapitulating any of that work. The display-pane stay-alive choice is forced by parent D14 (operator-initiated closure) and parent D26 (status-line script keeps running) — they outlive the workflow on purpose.
- **Evidence:** Junior OQ-6 (R1 claim ledger CL-20); parent D14 (operator-initiated closure); parent D26 (status-line keeps running); Phase 1 `internal/cmuxctl/dismissal.go`; Phase 1 plan U5 verification.
- **Rejected alternatives:**
  - Replace `DismissalObserver` with channel-based dismissal driven by the interaction channel — rejected; refactor without evidence; introduces a second dismissal path the operator must reason about.
  - Modify the observer to skip per-pane-exit checks once the orchestrator pane has exited — rejected; per-pane exit of header/log/footer is exactly the close-each-pane dismissal gesture Phase 1 supports.
  - Have the orchestrator pane stay alive until dismissal — rejected; the orchestrator has nothing to render (it is hidden) and no work to do after workflow.Run + ack-wait; keeping it alive is busy-waiting.
- **Specialist owner:** `behavioral-analyst`
- **Revisit criterion:** if Phase 6 hardens against display-pane loss in a way that requires a new observer signal, revisit; until then unchanged reuse is correct.
- **Dissent (if any):** —
- **Driven by rounds:** R1
- **Dependent decisions:** —
- **Referenced in plan:** Runtime Behavior, Decomposition and Sequencing (U9)

### D-20: Per-pane processes via `pr9k` sub-commands rather than separate binaries

- **Question:** Each cmux pane needs a process to run inside it (parent T3). Phase 1 used `sh -c '...'` placeholders. For Phase 2's real renderers, do those processes live as separate binaries (e.g., `pr9k-cmux-header`), as sub-commands of the main `pr9k` binary (e.g., `pr9k cmux-pane --role=header`), or in some other form?
- **Decision:** Sub-commands of the main `pr9k` binary. Add one new cobra command, `pr9k cmux-pane`, with a `--role` flag taking values `orchestrator|header|log|footer`. The orchestrator's spawn calls (`SurfaceSpawn`) pass `pr9k cmux-pane --role=<role>` plus the interaction-channel socket path as an environment variable. The sub-command's `Run` function is the thin entry point that reuses the existing `internal/ui` renderers (`StatusHeader`, log-panel, `KeyHandler`) under the interaction-channel client wrapping (per YAGNI-8 simpler-version replacement).
- **Rationale:** YAGNI-8 in the sweep demonstrated that a bespoke `cmuxpane` package per pane would duplicate existing renderers; the sub-command-plus-thin-entry-point shape is the simpler version. A single binary (one `make build`) keeps distribution and operator-facing CLI surface coherent with the rest of pr9k (`pr9k workflow`, `pr9k sandbox` already exist). Separate binaries would double the release work (5 binaries instead of 1), complicate the `bin/` layout, and make `pr9k --version` divergence possible. Sub-command path resolution is straightforward (the cmux spawn uses the same `pr9k` binary the operator launched).
- **Evidence:** YAGNI-8 in `.yagni-sweep.md`; junior OQ-2 + behavioral B13 framing (R1 claim ledger CL-17); `internal/cli` cobra precedent (existing `pr9k workflow`, `pr9k sandbox`, `pr9k sandbox create` sub-commands).
- **Rejected alternatives:**
  - Separate binaries per role — rejected per YAGNI-8 + distribution complexity.
  - Single-flag `pr9k --cmux-pane-role=...` on the root command — rejected; mixes operator-facing flags with internal-process flags in the same surface; cobra sub-commands are the established pattern.
  - Use `os.Args[0]` symlinks (busybox-style) — rejected; non-portable across the cmux spawn shell layer.
- **Specialist owner:** `software-architect` (Phase 2 implementation)
- **Revisit criterion:** if Phase 3+ rendering needs to import packages that bloat the main `pr9k` binary, revisit the single-binary choice.
- **Dissent (if any):** —
- **Driven by rounds:** R1
- **Dependent decisions:** D-21
- **Referenced in plan:** Architecture and Integration Points, External Interfaces, Decomposition and Sequencing (U3)

### D-21: Interaction-channel socket at `<projectDir>/.pr9k/cmux-pane-<workspaceName>.sock` with 0700-mode parent

- **Question:** Where on the filesystem does the interaction-channel Unix socket live, with what permissions?
- **Decision:** `<projectDir>/.pr9k/cmux-pane-<workspaceName>.sock`. The parent directory `<projectDir>/.pr9k/` is created with mode 0700 (already the case from Phase 1's preflight). The socket file itself uses default Unix socket permissions (operator-readable, governed by parent directory mode). The orchestrator process binds the socket; on workspace teardown the socket file is unlinked via the orchestrator's exit path (or by `SurfaceList` polling cleanup if the orchestrator crashed). The socket path is passed to display-pane sub-commands via the `PR9K_CMUX_SOCKET` environment variable set on `SurfaceSpawn`.
- **Rationale:** The `.pr9k/` umbrella is already pr9k's runtime-state convention (ADR `20260418175134-pr9k-rename-and-pr9k-layout`). Mode 0700 on the parent directory provides filesystem-permission-based access control matching the Phase 1 `CMUX_SOCKET_PATH` model: only the operator who owns the directory can connect. No client-identity beyond filesystem permissions is needed because all four processes (orchestrator + three display panes) are spawned by the same cmux session and run as the same user. The workspace name in the socket filename disambiguates multiple concurrent pr9k cmux launches against the same project directory.
- **Evidence:** ADR `20260418175134-pr9k-rename-and-pr9k-layout`; Phase 1 `cmuxctl.Preflight`'s `CMUX_SOCKET_PATH` validation precedent; `docs/coding-standards/file-writes.md` (atomic patterns); `docs/coding-standards/error-handling.md` (file paths in I/O errors).
- **Rejected alternatives:**
  - `/tmp/pr9k-cmux-<workspaceName>.sock` — rejected; world-writable parent directory introduces cross-user risk that the `<projectDir>/.pr9k/` location avoids.
  - `$XDG_RUNTIME_DIR/pr9k/...` — rejected; Linux-only convention; pr9k must run on macOS (cmux's primary platform per parent D2) where `XDG_RUNTIME_DIR` is not standard.
  - One socket per pane connection (three socket files) — rejected per D-1; one socket multiplexes by role identity.
- **Specialist owner:** `devops-engineer`
- **Revisit criterion:** if multiple operators share a project directory (e.g., a CI runner with multiple sessions), revisit the workspace-name disambiguation strategy.
- **Dissent (if any):** —
- **Driven by rounds:** R1
- **Dependent decisions:** —
- **Referenced in plan:** Architecture and Integration Points, Security Posture, External Interfaces


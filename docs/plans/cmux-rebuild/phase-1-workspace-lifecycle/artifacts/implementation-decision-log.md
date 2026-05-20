# Implementation Decision Log: Phase 1 — Cmux Mode Launch and Workspace Lifecycle

This file records every implementation decision committed while planning Phase 1 of pr9k cmux mode. Behavioral and implementation statements live in [../feature-implementation-plan.md](../feature-implementation-plan.md) — this file captures the question, rationale, evidence, and rejected alternatives for each decision. Round-by-round history lives in [implementation-iteration-history.md](implementation-iteration-history.md).

Phase 1 decisions D1–D12 are inherited from the source spec's [decision-log.md](decision-log.md) and are not re-recorded here; the implementation respects them. Parent decisions D1, D2, D3, D4, D13, D18, D22, D25, D29 are inherited unchanged from the parent [decision-log](../../artifacts/decision-log.md).

## Trivial decisions

- D-25: CLI flag wiring — add `Cmux bool` field to `cli.Config` and register `--cmux` via `cmd.Flags().BoolVar(...)` in `internal/cli/args.go`, following the existing pattern used for `--workflow-dir` and `--project-dir`. — Referenced in plan: Decomposition and Sequencing (U1).
- D-26: Version bump — bump `src/internal/version/version.go` from `0.10.0` to `0.11.0` per `docs/coding-standards/versioning.md` (new public CLI surface during 0.y.z bumps MINOR). — Referenced in plan: Decomposition and Sequencing (U10), Definition of Done.
- D-27: Preserve existing `internal/ansi.StripAll` contract — do not change the existing function's behavior; existing callers (workflowio recovery view, sandbox-create smoke test) continue to use it unchanged. New stricter variant lands alongside per [D-14](#d-14-stripforterminaloutput-ansi-variant-for-the-cmux-diagnostic-path). — Referenced in plan: Security Posture, Decomposition and Sequencing (U8).

## Full decisions

### D-1: Cmux preflight integration shape

- **Question:** How does the cmux preflight slot into pr9k's existing `startup()` → `validator.Validate` → `preflight.Run` sequence?
- **Decision:** Keep `preflight.Run` unchanged. Add a separate `cmuxctl.Preflight(...)` function that lives in the new `internal/cmuxctl` package and is invoked from `main()` immediately after `startup()` returns successfully and only when `cfg.Cmux` is true. The cmux preflight runs the five distinguishable failure-condition checks (cmux not installed, cmux not running, not a cmux descendant, socket disabled, capability mismatch) plus the `system.identify` check, returns a slice of errors, and aborts via the existing fatal-error print-and-exit pattern.
- **Rationale:** This shape keeps the existing `preflight.Run` API stable, isolates cmux concerns to the new package, and satisfies the spec's D8 requirement that the standard preflight runs first. Extending `preflight.Run` with a `cmuxRequired bool` parameter and a separate cmux-prober interface would couple cmux logic into the preflight package and force every existing caller to pass new arguments. The discovery notes (`.discovery-notes.md` Preflight section) explicitly recommend this shape.
- **Evidence:** `.discovery-notes.md` Preflight section ("two viable shapes ... the second shape preserves the existing API and isolates cmux concerns"); `src/cmd/pr9k/main.go:62-108` (existing `startup()` shape); `docs/code-packages/preflight.md` (existing preflight contract); behavioral B8 finding (R1, claim ledger row 19); spec D8 (cmux preflight runs after standard preflight).
- **Rejected alternatives:**
  - Extend `preflight.Run` with a `cmuxRequired bool` parameter and a cmux-prober interface — rejected because it couples cmux logic into the existing preflight package and requires every existing caller to pass new arguments.
  - Run the cmux preflight inside `startup()` itself — rejected; `startup()` is the standard preflight surface and should not branch on `cfg.Cmux`.
  - Run the two preflights in parallel — rejected per D8 rationale (sequential is easier for the operator to read; both preflights are fast).
- **Specialist owner:** `devops-engineer`
- **Revisit criterion:** if a future phase needs cmux preflight to run conditionally inside `startup()` (e.g., to share a probe cache), revisit the package boundary.
- **Dissent (if any):** —
- **Driven by rounds:** R1
- **Dependent decisions:** D-3, D-9, D-13, D-15, D-16, D-18, D-19, D-20
- **Referenced in plan:** Architecture and Integration Points, Decomposition and Sequencing (U3)

### D-2: `--cmux` flag ships visible with experimental help text

- **Question:** Should `--cmux` ship `Hidden: true` until Phase 2 lands real workflow content, per the api-design standard's "hide incomplete commands" rule?
- **Decision:** Ship `--cmux` visible. Help text names Phase 1's scope explicitly: "experimental: launches a four-pane placeholder workspace; full workflow rendering ships in a later release." The flag is not marked `Hidden`.
- **Rationale:** `docs/coding-standards/api-design.md`'s "hide incomplete commands" rule defines incomplete as "wired to a stub or placeholder." Phase 1's `--cmux` does real work — the workspace lifecycle is fully functional end-to-end. The placeholder content lives in the panes, not in the flag. Hiding the flag would block the Phase 1 demo (operators cannot invoke a hidden flag without reading source) and contradicts the build-outline's commitment to a demoable Phase 1 deliverable.
- **Evidence:** `docs/coding-standards/api-design.md` ("Don't ship hidden/stub commands"); junior-developer Q3 (R1, claim ledger row 24); build-outline Phase 1 Outcome to demonstrate (5-step demo requires the flag to be invocable).
- **Rejected alternatives:**
  - `Hidden: true` until Phase 2 — rejected; the Phase 1 demo cannot be reproduced behind a hidden flag, and the placeholder content is in the panes (which operators see and may file bug reports against), not in the flag itself.
  - Ship as a separate `pr9k cmux` subcommand — rejected at parent decision level (parent D25 commits the launch surface to the `--cmux` flag).
- **Specialist owner:** `devops-engineer`
- **Revisit criterion:** if false bug reports against placeholder pane content arrive after Phase 1 ships, revisit the help-text wording or add a one-time "Phase 1 placeholder" message to the launching terminal.
- **Dissent (if any):** —
- **Driven by rounds:** R1, R2
- **Dependent decisions:** —
- **Referenced in plan:** Architecture and Integration Points, External Interfaces

### D-3: Pane spawn order is orchestrator-first

- **Question:** In what order does pr9k spawn the four panes (orchestrator, header, log, footer)?
- **Decision:** Orchestrator-first. The hidden orchestrator pane is created first (via the initial `workspace.create` if cmux's API attaches a pane to the workspace at create time, otherwise via the first `surface.split` immediately after); the visible panes (header, log, footer) split off subsequently. The orchestrator pane is hidden via the cmux pane-hide API after creation.
- **Rationale:** Two reasons converge on this order. (1) The hidden orchestrator pane preserves cmux ancestry for the launching pr9k process per parent T2 — creating it first makes ancestry explicit and lets the visible panes inherit it cleanly. (2) During teardown (partial-setup failure or cmux-side failure mid-spawn), the operator never sees a partial visible scaffold (no half-built header/log/footer arrangement); the most-load-bearing pane exists earliest. The behavioral spec leaves spawn order open; the plan commits.
- **Evidence:** behavioral B9 finding (R1, claim ledger row 16); parent T2 (cmux access model: descendants only); parent T3 (surfaces are processes); spec D1 (orchestrator pane runs a placeholder during Phase 1).
- **Rejected alternatives:**
  - Orchestrator-last (visible panes first, hidden orchestrator pane after) — rejected; partial-setup teardown would briefly show a complete visible scaffold without the orchestrator behind it, suggesting (incorrectly) the launch had succeeded.
  - Parallel spawn — rejected; cmux RPC is sequential per workspace lifecycle (parent T1, spec Coordinations table); attempting parallelism contradicts the single-flight queue committed in [D-5](#d-5-cmux-client-uses-a-single-goroutine-sequential-rpc-queue).
- **Specialist owner:** `behavioral-analyst`
- **Revisit criterion:** if cmux exposes a multi-pane atomic-create API in a future release, revisit (the order would become irrelevant).
- **Dissent (if any):** —
- **Driven by rounds:** R1, R2
- **Dependent decisions:** —
- **Referenced in plan:** Runtime Behavior, Decomposition and Sequencing (U4)

### D-4: Placeholder process is a shell one-liner

- **Question:** What process does each pane host — a shell one-liner, a dedicated `pr9k` subcommand, or a Go binary built specifically for placeholders?
- **Decision:** Shell one-liner. Visible panes (header, log, footer) get `sh -c 'printf "<role> — Phase 1 placeholder\n" && tail -f /dev/null'`. The orchestrator pane gets `sh -c 'tail -f /dev/null'` only (no visible output per spec D1). The argv passed to cmux's `surface.spawn` is composed in Go as a string slice `["sh","-c","printf '<role> — Phase 1 placeholder\\n' && tail -f /dev/null"]` (no shell quoting required by pr9k since cmux takes argv directly).
- **Rationale:** The shell one-liner avoids new code, new convention, new PATH dependency, and a new (potentially `Hidden`) cobra subcommand that would be deleted when Phase 2 ships real renderers. `tail -f /dev/null` is POSIX-portable; `sleep infinity` is a GNU extension that does not exist on macOS BSD `sleep`. Junior-developer Q6 (R1, claim ledger row 26) raised the choice; R2 evidence/YAGNI sweep confirms the one-liner.
- **Evidence:** junior-developer Q6 (R1); R2 OQ-R1-7 resolution; POSIX `tail` reference (`tail -f /dev/null` is portable across macOS BSD and GNU coreutils); spec D1 (orchestrator pane produces no visible output by default).
- **Rejected alternatives:**
  - `pr9k display placeholder <role>` subcommand — rejected per YAGNI; would add a new (hidden) cobra subcommand whose only consumer is Phase 1, with built-in deletion when Phase 2 ships real renderers. Documentation cost (CLAUDE.md, code-packages doc, feature doc) is not justified by Phase 1's needs.
  - `sleep infinity` instead of `tail -f /dev/null` — rejected; `sleep infinity` is a GNU coreutils extension and is not portable to macOS BSD `sleep` (`sleep: invalid time interval 'infinity'`). Phase 1 must run on macOS (cmux's primary platform per parent D2).
  - Build a Go placeholder binary that links to nothing else — rejected; over-engineered for "print and sleep."
- **Specialist owner:** `devops-engineer`
- **Revisit criterion:** if a later phase needs richer placeholder behavior (e.g., responsive to keystrokes, dynamic content), promote to a real subcommand or a Phase-2 renderer process.
- **Dissent (if any):** —
- **Driven by rounds:** R1, R2
- **Dependent decisions:** —
- **Referenced in plan:** Runtime Behavior, Decomposition and Sequencing (U4)

### D-5: Cmux client uses a single-goroutine sequential RPC queue

- **Question:** How does the cmux RPC client manage concurrent access to the single Unix-socket connection?
- **Decision:** Single-goroutine sequential RPC queue. All RPC requests funnel through one goroutine that owns the `net.Conn`; callers submit a request via a buffered request channel and receive the response on a per-request reply channel. Only one in-flight RPC at a time. Multiplex (concurrent in-flight RPCs with response demultiplexing by JSON-RPC `id`) is deferred to Phase 4 sidebar mirroring.
- **Rationale:** Two reasons. (1) Phase 1 has no concurrent-RPC pressure: preflight calls are sequential, workspace setup is sequential per parent T1, the dismissal poll is single-flight by [D-6](#d-6-dismissal-observation-via-500ms-polling-with-single-flight). The polling goroutine and the shutdown goroutine concurrently access one `net.Conn` (concurrency C2 finding, R1 claim ledger row 3); a single-goroutine queue is the simplest pattern that satisfies the requirement. (2) Multiplex adds protocol-level complexity (request-id allocation, response demultiplexing, error-routing on socket close) that Phase 4 will need but Phase 1 does not. Building it now is YAGNI; building it later when sidebar mirroring proves the need is correct.
- **Evidence:** concurrency C2 finding (R1, claim ledger row 3); concurrency-analyst recommendation in R1 ("serialize through a single goroutine for Phase 1; defer multiplex to Phase 4"); `docs/coding-standards/concurrency.md` (snapshot-then-unlock, channel-based action dispatch, mutex-protected shared writers); parent T1 (calls are sequential per workspace lifecycle).
- **Rejected alternatives:**
  - Full multiplex (concurrent in-flight RPCs with response demultiplexing by JSON-RPC `id`) — rejected for Phase 1 per YAGNI; would add request-id allocation, response demultiplexing, and error-routing complexity for which Phase 1 has no concrete trigger. Reopen for Phase 4 sidebar mirroring.
  - Mutex-protected `net.Conn` with multiple goroutines holding the lock for full RPC roundtrips — rejected; long-held locks block all other RPCs and provide no advantage over the channel-queue shape. The channel queue is more idiomatic Go.
- **Specialist owner:** `concurrency-analyst`
- **Revisit criterion:** Phase 4 sidebar mirroring (when concurrent RPCs become a measured need) or any phase where polling cadence × RPC count exceeds throughput.
- **Dissent (if any):** —
- **Driven by rounds:** R1
- **Dependent decisions:** D-6, D-8, D-21, D-22
- **Referenced in plan:** Architecture and Integration Points, Runtime Behavior, Decomposition and Sequencing (U2)

### D-6: Dismissal observation via 500ms polling with single-flight

- **Question:** How does pr9k observe the two events that constitute spec D9's dismissal contract — workspace removal from cmux's workspace list, and any of the four placeholder processes exiting?
- **Decision:** Polling at 500ms cadence, single-flight (one in-flight poll at a time), per-call timeout from parent D27 (5–10 second range). Each poll cycle issues `workspace.list` and `surface.list` (or equivalent per [D-19](#d-19-per-pane-exit-observation-via-surfacelist)) sequentially through the single-goroutine RPC queue from [D-5](#d-5-cmux-client-uses-a-single-goroutine-sequential-rpc-queue); first observation that satisfies either D9 arm fires the dismissal channel.
- **Rationale:** Cmux's JSON-RPC interface admits both polling and async subscription (parent T1); spec D9 leaves the mechanism to implementation. Polling is the simpler implementation: no subscription protocol to maintain, deterministic detection latency, no async-callback ordering risks. 500ms is the operator's perceptual threshold for "instant" workspace-close response (devops DOR-006 finding); shorter cadences flood the cmux socket with no operator benefit, longer cadences make workspace-close feel like a hang. Single-flight prevents poll pile-up if a poll RPC stalls under the per-call timeout.
- **Evidence:** devops DOR-006 finding (R1, claim ledger row 11); spec D9 (mechanism left to implementation); parent T1 (JSON-RPC admits polling); concurrency C2 finding (single-flight requirement); R2 OQ-R1-1 resolution (per-pane exit via `surface.list`).
- **Rejected alternatives:**
  - Async subscription / push notifications — rejected per YAGNI; cmux exposes the data via list calls, no concrete trigger for the subscription complexity, and async-callback ordering would interact with the shutdown sequence in fragile ways.
  - 100ms cadence — rejected; 10× the RPC volume for no operator-perceptible benefit.
  - 2s cadence — rejected; workspace-close gestures would feel laggy (operator perception threshold for "instant" is ~500ms).
  - No single-flight — rejected; concurrent polls plus the per-call timeout would let pollers pile up during a stall, defeating the timeout's purpose.
- **Specialist owner:** `devops-engineer`
- **Revisit criterion:** if cmux exposes a pane-exit subscription API and the polling cost becomes meaningful (e.g., measured CPU on the cmux process attributable to pr9k polling), revisit the polling decision.
- **Dissent (if any):** —
- **Driven by rounds:** R1, R2
- **Dependent decisions:** D-7, D-8, D-19, D-22
- **Referenced in plan:** Runtime Behavior, Decomposition and Sequencing (U5)

### D-7: D9 poll-timeout escalation tolerates N=3 consecutive fires before fatal teardown

- **Question:** Parent D15 says "every cmux API call timeout is fatal." Does the rule apply uniformly to D9 polling calls during the waiting phase, or are waiting-phase polls exempt?
- **Decision:** D15's fatal rule applies to launch-phase RPCs (preflight, workspace.create, surface.split, surface.spawn, pane-hide) and to teardown RPCs (workspace.close). For dismissal-observation polls during the waiting phase, a single transient poll timeout is **not** fatal; consecutive timeouts escalate. After N=3 consecutive `workspace.list` or `surface.list` poll timeouts, pr9k treats the poll path as broken, fires the dismissal channel with a fatal-teardown sentinel, runs best-effort teardown per [D-11](#d-11-best-effort-teardown-with-operator-visible-diagnostic), and exits non-zero. Successful polls reset the consecutive-timeout counter to zero.
- **Rationale:** Treating a single transient `workspace.list` poll timeout as fatal would tear down a healthy workspace mid-inspection (poor operator experience). Spec parent D15 was specified for the launch sequence and workflow-driving calls (behavioral B3 finding, R1 claim ledger row 12). Allowing infinite polls to time out silently would mask a real cmux failure indefinitely. Three consecutive failures is a small enough threshold that an operator notices the delay (≤ ~30 seconds at 500ms cadence + worst-case 10s timeout) but large enough to absorb realistic transient hiccups.
- **Evidence:** behavioral B3 finding (R1, claim ledger row 12); R2 OQ-R1-3 resolution; parent D15 specification scope; spec D9 (mechanism left to implementation but bounded by parent D15).
- **Rejected alternatives:**
  - Apply D15 fatally to every poll — rejected; tears down a healthy workspace on the first transient slowness.
  - Tolerate unbounded poll timeouts — rejected; a real cmux hang would never surface to the operator.
  - Use a rolling-window error rate (e.g., "5 of last 10") — rejected as over-engineered; consecutive-count is simpler and equivalent for the scenarios that matter.
- **Specialist owner:** `behavioral-analyst`
- **Revisit criterion:** if real-world telemetry from Phase 6+ shows the N=3 threshold causes false-positive teardowns (or fails to catch a real hang in time), tune the threshold.
- **Dissent (if any):** —
- **Driven by rounds:** R1, R2
- **Dependent decisions:** D-11
- **Referenced in plan:** Runtime Behavior, Operational Readiness

### D-8: Self-close double-fire mitigation via shuttingDown flag and channel priming

- **Question:** During shutdown, pr9k itself calls `workspace.close`, which causes the workspace to disappear from `workspace.list` — an event the dismissal-observation poller is also watching. How does pr9k avoid double-firing the dismissal channel and double-running teardown?
- **Decision:** A boolean `shuttingDown` flag (mutex-protected) is set true before `workspace.close` is issued during teardown. The dismissal-observation goroutine checks `shuttingDown` before forwarding any observation to the dismissal channel; if the flag is set, the observation is dropped silently. The dismissal channel is buffered to size 1 to absorb a race where the poller sees the workspace gone before `shuttingDown` is set. Teardown itself is guarded by `sync.Once`.
- **Rationale:** Concurrency C1+C6 and behavioral B1 (R1, claim ledger row 2) identified the self-close double-fire race plus a TOCTOU window when polling cadence puts a `workspace.list` call in flight during teardown. A `sync.Once` on teardown handles the case where two reads of the dismissal channel both reach teardown invocation (defense in depth). The `shuttingDown` flag plus channel priming covers the upstream case where a poll already in flight returns its result while teardown is starting.
- **Evidence:** concurrency C1+C6 finding (R1, claim ledger row 2); behavioral B1 finding; `docs/coding-standards/concurrency.md` (snapshot-then-unlock, mutex-protected shared writes, non-blocking channel sends).
- **Rejected alternatives:**
  - Cancel polls before issuing `workspace.close` — rejected; cancellation is racy (an in-flight RPC may already have returned its result), and the single-flight queue from [D-5](#d-5-cmux-client-uses-a-single-goroutine-sequential-rpc-queue) blocks `workspace.close` until the poll returns anyway. Just wait for the poll, set the flag, then call close.
  - Idempotent teardown without the flag — rejected; without the flag the dismissal channel still fires twice (once for poll observation, once for the operator's actual gesture if any), confusing log output. The flag plus `sync.Once` together produce a clean single execution.
- **Specialist owner:** `concurrency-analyst`
- **Revisit criterion:** if Phase 6's hardened signal handler test seams change the shutdown ordering, revisit.
- **Dissent (if any):** —
- **Driven by rounds:** R1
- **Dependent decisions:** D-9, D-22
- **Referenced in plan:** Runtime Behavior, Decomposition and Sequencing (U5, U7)

### D-9: Two-goroutine watchdog+cleanup signal handling pattern

- **Question:** Spec D6 commits a second SIGINT/SIGTERM/SIGHUP during graceful shutdown to immediate exit. But if the signal-handler goroutine is blocked inside a hangable cmux call (up to 10s per parent D27), the second-signal exit cannot be delivered. How is the immediacy guarantee delivered?
- **Decision:** Two-goroutine pattern. The cleanup goroutine receives the first signal, sets `shuttingDown` per [D-8](#d-8-self-close-double-fire-mitigation-via-shuttingdown-flag-and-channel-priming), and runs the teardown sequence (which may block on cmux calls). A separate watchdog goroutine, started at the same time as the cleanup goroutine, listens on the same signal channel; on its first receive (which is the operator's second signal, since the cleanup goroutine has already consumed one), the watchdog calls `os.Exit(1)` immediately regardless of cleanup state. The watchdog never touches the cmux client.
- **Rationale:** Concurrency C5, devops DOR-002, and behavioral B11 (R1, claim ledger row 6) jointly identified that a signal-handler that runs cleanup inline cannot deliver the second-signal-exit guarantee while a cmux RPC is in flight. Splitting into two goroutines lets cleanup proceed while the watchdog stays responsive. `os.Exit` (not `panic` or `runtime.Goexit`) is correct because cleanup may have left mutable state in an inconsistent place; running deferred functions or stack unwinding could cause further hangs.
- **Evidence:** concurrency C5 finding (R1); devops DOR-002; behavioral B11; `docs/features/signal-handling.md`; parent D27 (per-call timeout up to 10s); existing pattern at `src/cmd/pr9k/main.go:281-330` (signal handler with timeout fallback to `program.Kill()`).
- **Rejected alternatives:**
  - Single-goroutine signal handler with cleanup inline — rejected; cannot deliver second-signal-exit while a cmux call is in flight.
  - Run cleanup with a context that the watchdog cancels — rejected; cancellation is racy with the in-flight RPC's protocol-level state per [D-21](#d-21-per-call-timeout-protocol-close-socket-on-timeout-re-open-for-teardown), and `os.Exit` is the correct semantic for "operator demanded exit twice."
  - Block the watchdog on `signal.Notify` only — rejected; the first signal is already consumed by the cleanup goroutine, so the watchdog needs to share the channel (Go's `signal.Notify` to a single buffered channel handles this naturally).
- **Specialist owner:** `concurrency-analyst`
- **Revisit criterion:** if Phase 6 introduces test seams for the signal handler (junior-developer S1 deferred), revisit the goroutine boundary.
- **Dissent (if any):** —
- **Driven by rounds:** R1
- **Dependent decisions:** D-10, D-11
- **Referenced in plan:** Runtime Behavior, Decomposition and Sequencing (U6)

### D-10: SIGHUP registered alongside SIGINT and SIGTERM

- **Question:** Spec D6 commits SIGINT, SIGTERM, and SIGHUP to the same graceful-shutdown path. But `src/cmd/pr9k/main.go:284-285` registers only SIGINT and SIGTERM. How is SIGHUP picked up?
- **Decision:** Register SIGHUP in the existing `signal.Notify` call. The signal-handler entry point treats SIGHUP identically to SIGINT/SIGTERM: first delivery triggers the cleanup goroutine per [D-9](#d-9-two-goroutine-watchdogcleanup-signal-handling-pattern); second of any of the three kinds triggers the watchdog's `os.Exit`.
- **Rationale:** Devops DOR-001, behavioral B6 (R1, claim ledger row 7), and security SEC-005 (R2) jointly confirmed: without SIGHUP registration, Go's default disposition kills the process immediately on terminal close (when the shell forwards SIGHUP per its own configuration), guaranteeing an orphan workspace and contradicting spec D6. Registering SIGHUP is a one-line change to the existing code, and spec D6 already commits SIGHUP to the graceful-shutdown path.
- **Evidence:** devops DOR-001 finding (R1, claim ledger row 7); behavioral B6; security SEC-005 (R2); spec D6 (SIGHUP must trigger graceful shutdown); `src/cmd/pr9k/main.go:284-285` (current registration).
- **Rejected alternatives:**
  - Leave SIGHUP unregistered — rejected; contradicts spec D6 and guarantees orphans on terminal close.
  - Treat SIGHUP differently from SIGINT/SIGTERM (e.g., immediate exit) — rejected; spec D6 explicitly commits all three to graceful shutdown.
  - Apply SIGHUP only to the cmux Phase 1 path, not the standard run loop — rejected; the standard run loop already has the same orphan risk under SIGHUP, and a uniform signal-handler shape is simpler. (If the standard path's existing tests rely on SIGHUP killing immediately, those tests need updating; this is a feature, not a regression.)
- **Specialist owner:** `devops-engineer`
- **Revisit criterion:** if the standard (non-cmux) run loop tests fail under SIGHUP graceful-shutdown, revisit whether the registration should be cmux-mode-only.
- **Dissent (if any):** —
- **Driven by rounds:** R1, R2
- **Dependent decisions:** —
- **Referenced in plan:** Runtime Behavior, Security Posture, Decomposition and Sequencing (U6)

### D-11: Best-effort teardown with operator-visible diagnostic on workspace.close failure

- **Question:** The spec's "no orphan workspace from this failure mode" guarantee (Edge Cases, partial-setup row) is not actually guaranteed when `workspace.close` itself fails for a non-race reason (timeout, network error, cmux internal error). Either the guarantee needs an audit (carve out exceptions) or the operator needs a distinct error message when teardown itself fails.
- **Decision:** Best-effort teardown with operator-visible diagnostic. During teardown, pr9k attempts `workspace.close` once; on failure, pr9k logs to the launching terminal: `pr9k: orphan workspace "<name>" could not be closed; dismiss it manually via cmux's controls` (cmux diagnostic appended after sanitization per [D-14](#d-14-stripforterminaloutput-ansi-variant-for-the-cmux-diagnostic-path)). Pr9k continues to the focus-restore step regardless. Exit code is non-zero. The spec's "no orphan" claim is qualified by this diagnostic rather than contradicted.
- **Rationale:** Behavioral B2 finding (R1, claim ledger row 15) showed silent swallow defeats the orphan guarantee. The fix is operator visibility: the operator sees the workspace name, the operator can dismiss it manually. Retrying `workspace.close` would compound the timeout (parent D27) without evidence the second attempt would succeed. Skipping focus-restore on teardown failure is hostile (the prior workspace is unrelated to the cmux failure).
- **Evidence:** behavioral B2 finding (R1, claim ledger row 15); R2 OQ-R1-5 resolution; spec edge-cases partial-setup row; parent D27 (per-call timeout).
- **Rejected alternatives:**
  - Silent swallow (current spec implication) — rejected; defeats the orphan guarantee silently and the operator never learns the workspace was orphaned.
  - Retry `workspace.close` until success — rejected; compounds the timeout without evidence the retry would succeed; risks blocking shutdown indefinitely.
  - Skip focus-restore on teardown failure — rejected; the prior workspace restore is independent of cmux's teardown response and is cheap.
  - Emit a structured event instead of a terminal line — rejected; Phase 1 has no logger (per [D-16](#d-16-skip-standard-logger-and-bubble-tea-wiring-on-cmux-phase-1-path)) and the launching terminal is the only operator surface available.
- **Specialist owner:** `behavioral-analyst`
- **Revisit criterion:** if real operator reports show the diagnostic is ignored or misread, revisit (e.g., add a Phase-7 startup advisory entry for the orphan).
- **Dissent (if any):** —
- **Driven by rounds:** R1, R2
- **Dependent decisions:** —
- **Referenced in plan:** Runtime Behavior, Operational Readiness, Decomposition and Sequencing (U7)

### D-12: Drop the exit-code distinction on dismissal — every dismissal observation produces exit zero

- **Question:** The spec's User Interactions Dismissal-gestures bullet commits to "exit zero on workspace-close path; non-zero on close-pane path." This requires deterministic observation ordering (which event fires first deterministically tells pr9k which gesture the operator used). Spec D9 only commits "either fires first." Drop the distinction, or add a deterministic-ordering rule?
- **Decision:** **Drop the exit-code distinction.** Every dismissal observation produces exit zero. Non-zero exit is reserved for actual failures: preflight error, partial-setup teardown, signal-driven shutdown, observation-poll-timeout escalation per [D-7](#d-7-d9-poll-timeout-escalation-tolerates-n3-consecutive-fires-before-fatal-teardown), and `workspace.close` failure during shutdown per [D-11](#d-11-best-effort-teardown-with-operator-visible-diagnostic).
- **Rationale:** Behavioral B4 and devops DOR-008 (R1, claim ledger row 14) jointly demonstrated that for an operator's workspace-close gesture, both events typically fire near-simultaneously (cmux removes the workspace AND terminates the pane processes). Whichever event the implementation observes first determines the exit code, producing non-deterministic exit codes for the same operator action. Deterministic ordering would require either an unobserved cmux internal sequencing guarantee or a brittle sleep-and-recheck heuristic. Dropping the distinction is the user's explicit choice on R2 OQ-R1-4 and the simplest correct behavior.
- **Evidence:** behavioral B4 finding (R1, claim ledger row 14); devops DOR-008; R2 OQ-R1-4 user resolution ("**user input: drop the distinction**"); spec D9 (only commits "either fires first").
- **Rejected alternatives:**
  - Spec's original "exit zero on workspace-close path; non-zero on close-pane path" rule — rejected because it requires deterministic event ordering that is not technically achievable; the same operator gesture would produce non-deterministic exit codes.
  - Add a deterministic-ordering rule (e.g., "if both events fire within 200ms, use the workspace-close interpretation") — rejected as a brittle heuristic that adds complexity without resolving the underlying race; user explicitly preferred the simpler "always exit zero on dismissal" rule.
  - Reverse the rule (non-zero on workspace-close, zero on close-pane) — rejected; same race, opposite default; no operator benefit.
- **Specialist owner:** user
- **Revisit criterion:** if Phase 6 or later phases need exit-code distinction for scripting / CI, revisit with the new requirement context.
- **Dissent (if any):** This decision **contradicts the spec's `User Interactions` Dismissal-gestures bullet**. The spec must be amended in a follow-up commit to match the implementation contract; until then, the spec is authoritative for behavior except for this specific rule, where this decision is authoritative. Recorded as Open Item OI-A in the plan.
- **Driven by rounds:** R1, R2
- **Dependent decisions:** —
- **Referenced in plan:** Runtime Behavior, Open Items

### D-13: Method-presence capability check accepted; schema-shape validation deferred

- **Question:** Parent D18 commits to a method-presence capability check ("the contract is method presence, not a version match"). Method presence catches renamed or removed methods, but not schema-shape changes (a renamed JSON field, an added required field). Should `internal/cmuxctl` add schema-shape validation, or accept the risk explicitly?
- **Decision:** Accept the risk explicitly. Parent D18 binds the contract to method presence; standard `json.Unmarshal` with struct tags handles unknown/missing fields via zero values, which the implementation can defensively check. Schema-shape validation (e.g., a JSON Schema validator at the package boundary) is YAGNI — no concrete trigger, no incident, no named cmux release that broke shape. Reopen if a cmux release breaks a method's response shape and the resulting confusing error costs operator time.
- **Rationale:** Junior-developer Q2 (R1, claim ledger row 23) raised the question. Parent D18 is the canonical decision; this implementation decision affirms the parent and records the YAGNI rejection of the heavier alternative. Adding JSON schema validation at the package boundary requires schema sources for every cmux RPC method, which the cmux project does not publish, and would hand-codify a contract pr9k cannot independently enforce.
- **Evidence:** junior-developer Q2 finding (R1, claim ledger row 23); R2 OQ-R1-11 resolution; parent D18; `encoding/json` zero-value behavior; spec D18 reference.
- **Rejected alternatives:**
  - JSON Schema validation in `internal/cmuxctl` — rejected per YAGNI; no concrete trigger, no schema source published by cmux, no incident history. Reopen trigger: a cmux release breaks a method's response shape.
  - Pin to a specific cmux version string — rejected at parent decision level (parent D18); cmux's version surface is not stable enough.
- **Specialist owner:** `devops-engineer`
- **Revisit criterion:** a cmux release breaks a method's response shape and produces a confusing error in production.
- **Dissent (if any):** —
- **Driven by rounds:** R1, R2
- **Dependent decisions:** —
- **Referenced in plan:** External Interfaces, Deferred (YAGNI)

### D-14: `StripForTerminalOutput` ANSI variant for the cmux-diagnostic path

- **Question:** The existing `internal/ansi.StripAll` is documented (and pinned by tests) to pass through C0 cursor-movement bytes (CR `0x0d`, BS `0x08`, DEL `0x7f`, VT, FF, BEL, NUL) and 8-bit C1 controls (`0x80–0x9F`). Adversarial-security SEC-001 demonstrated terminal-overstrike injection; SEC-004 demonstrated title-set phishing via C1 OSC. The cmux-diagnostic-to-terminal path requires stricter sanitization. How is the gap closed?
- **Decision:** Add a `StripForTerminalOutput` (or `StripStrict`) function in `internal/ansi` that strips, in addition to the existing CSI/OSC/bare-ESC/two-byte-ESC handling: (a) all C0 cursor-movement bytes (CR `0x0d`, LF `0x0a` is preserved, BS `0x08`, DEL `0x7f`, VT `0x0b`, FF `0x0c`, BEL `0x07`, NUL `0x00`, plus all other `0x00-0x1F` non-printable bytes except LF and HT `0x09`); (b) all 8-bit C1 controls in `0x80-0x9F`, including consuming any C1 OSC (`0x9d`) or C1 DCS (`0x90`) payload to BEL (`0x07`) or 8-bit ST (`0x9c`) terminator. Apply only on the cmux-diagnostic-to-terminal path (preflight errors, partial-setup teardown diagnostics, `workspace.close`-failure diagnostic per [D-11](#d-11-best-effort-teardown-with-operator-visible-diagnostic)). Existing `StripAll` callers (workflowio recovery view, sandbox-create smoke test) are NOT migrated and preserve their existing contract per [D-27](#d-27-trivial).
- **Rationale:** Security SEC-001, SEC-004 (R2) demonstrated concrete exploits against the existing `StripAll`'s documented gap. The cmux-diagnostic path is the first external-data-to-terminal flow in the codebase that an attacker could influence (via a malicious cmux response, a forged response over `CMUX_SOCKET_PATH` per [D-15](#d-15-cmux_socket_path-validation-before-netdial)). Migrating existing callers to the strict variant is out of scope: the workflowio recovery view's contract is to preserve content for operator inspection; the cmux-diagnostic path's contract is to prevent terminal exploits. Two contracts, two functions.
- **Evidence:** security SEC-001 finding (R2; cited `src/internal/ansi/strip.go:14`, `strip_test.go:267-288`); security SEC-004 finding (R2; cited `strip_test.go:91-104` "document the gap" comment); R2 OQ-R2-1 recommendation; `docs/coding-standards/go-patterns.md` ("Sanitize external program output before reflecting to terminal"); existing `internal/ansi.StripAll` implementation.
- **Rejected alternatives:**
  - Modify `StripAll` to be strict — rejected; would break the workflowio recovery view's operator-inspection contract; SEC-002 finding noted the existing contract is documented even though imperfect.
  - Strip nothing additional and document the gap — rejected; the demonstrated exploits are real (terminal overstrike, title-set phishing), the cmux path is operator-visible, and the cost of the new function is small.
  - Use a third-party sanitizer (e.g., `charmbracelet/x/ansi`) — rejected; pr9k already has an in-repo ANSI package whose API is stable; adding a new variant is the consistent pattern.
- **Specialist owner:** `adversarial-security-analyst`
- **Revisit criterion:** if a future security review demonstrates additional terminal-injection vectors (e.g., SCO-style responses, DEC private-mode sequences), extend `StripForTerminalOutput`.
- **Dissent (if any):** —
- **Driven by rounds:** R2
- **Dependent decisions:** D-11
- **Referenced in plan:** Security Posture, Decomposition and Sequencing (U8)

### D-15: `CMUX_SOCKET_PATH` validation before `net.Dial`

- **Question:** Adversarial-security SEC-003 demonstrated `CMUX_SOCKET_PATH` env-var exploitation: the variable is fully trusted, allowing an attacker-controlled fake Unix socket to defeat the parent T2 ancestry-based access model (attacker mints fake JSON-RPC 2.0 responses; pr9k prints fake workspace-name confirmation, exits zero, no real cmux workspace ever appears). How is the env var hardened?
- **Decision:** Validate `CMUX_SOCKET_PATH` before `net.Dial`. The validation steps: (1) trim the env-var value per `docs/coding-standards/go-patterns.md`; (2) reject if empty (fall back to platform-default socket discovery); (3) `os.Stat` the path to verify it exists and is a Unix-socket-type file (`stat.Mode()&os.ModeSocket != 0`); (4) `filepath.EvalSymlinks` to resolve the canonical path; (5) verify the canonical path's parent directory is owned by the invoking user (`os.Stat` on the parent directory, check `Sys().(*syscall.Stat_t).Uid == os.Getuid()`) — defeats the "attacker writes a socket into a world-writable directory" attack. After connecting, the existing parent D18 capability check calls `system.identify`; require the response to identify as cmux (specific identity string TBD per OI-1). Any error printed to the launching terminal as part of the not-a-descendant precondition uses the resolved (symlink-expanded) canonical path so the operator sees redirection.
- **Rationale:** Security SEC-003 (R2) is a concrete attack chain that defeats T2's entire trust model. The existing code path `net.Dial`s `CMUX_SOCKET_PATH` unchecked. Validation is cheap, defeats the attack, and is consistent with `docs/coding-standards/go-patterns.md` (env-var trim, symlink-safe path resolution). The owner check is the standard defense against socket-redirection attacks in shared `/tmp`.
- **Evidence:** security SEC-003 finding (R2); `docs/coding-standards/go-patterns.md` (env-var trim, symlink-safe path resolution); parent T2 (cmux access model is ancestry-only); parent D18 (capability check via `system.identify`).
- **Rejected alternatives:**
  - Trust the env var as today — rejected; SEC-003 is a demonstrated concrete attack.
  - Disallow `CMUX_SOCKET_PATH` entirely (require platform-default discovery) — rejected; cmux operators do legitimately override the socket path (e.g., for non-default cmux installations); blocking the override is hostile.
  - Hash-pin the socket path — rejected; cmux does not provide a way to query the canonical socket-path hash.
  - Audit-log every `CMUX_SOCKET_PATH` usage — rejected; Phase 1 has no logger (per [D-16](#d-16-skip-standard-logger-and-bubble-tea-wiring-on-cmux-phase-1-path)) and the operator-visible terminal output already names the resolved path on error.
- **Specialist owner:** `adversarial-security-analyst`
- **Revisit criterion:** if cmux publishes a credentialed handshake (e.g., a token in the cmux user's home directory the client must present), revisit; the env-var validation may then be unnecessary.
- **Dissent (if any):** —
- **Driven by rounds:** R2
- **Dependent decisions:** —
- **Referenced in plan:** Security Posture, Decomposition and Sequencing (U8)

### D-16: Skip standard logger and Bubble Tea wiring on cmux Phase 1 path

- **Question:** `startup()` unconditionally creates the per-run logger and `.pr9k/logs/<run-stamp>` artifact directory; `main()` unconditionally starts the Bubble Tea program and status-line runner. The Phase 1 spec commits to "no `.pr9k/logs/` artifacts produced" and the cmux path must NOT enter `program.Run()` (alt-screen would corrupt the launching terminal) and must NOT start the status-line runner. How does the launch path branch?
- **Decision:** `main()` branches on `cfg.Cmux` after `startup()` returns successfully. On the cmux Phase 1 path, pr9k: (a) skips `logger.NewLogger(projectDir)` (no per-run logger); (b) skips the status-line runner; (c) skips Bubble Tea program creation and `program.Run()`; (d) invokes `cmuxctl.Preflight(...)` per [D-1](#d-1-cmux-preflight-integration-shape); (e) on preflight success, invokes the new `cmuxctl.RunPhase1(...)` (or equivalent name) workspace lifecycle entry point, which blocks until dismissal. The standard preflight's `.pr9k/` umbrella mkdir is preserved (it is already gated to only `mkdir`, not log creation, and is consistent with non-cmux launches).
- **Rationale:** Devops DOR-003 and DOR-004 (R1, claim ledger rows 8 and 9) jointly identified that the unconditional logger creation produces log artifacts the spec forbids, and the unconditional Bubble Tea program would corrupt the launching terminal (alt-screen). The branch must happen in `main()`, not inside `startup()`, to preserve the standard preflight's API and to keep `startup()`'s contract clean.
- **Evidence:** devops DOR-003 finding (R1, claim ledger row 8); devops DOR-004; behavioral B5; junior Q8; spec Coordinations table (`.pr9k/logs/` artifacts not produced row); `src/cmd/pr9k/main.go:96-108` (logger creation); `src/cmd/pr9k/main.go:191-214` (Bubble Tea wiring).
- **Rejected alternatives:**
  - Keep the logger but don't write to it — rejected; creates an empty log file that surprises operators.
  - Branch inside `startup()` — rejected; couples cmux-mode awareness into the standard preflight; `startup()` should be cmux-mode-agnostic.
  - Add a new `cmuxStartup()` function that mirrors `startup()` minus logger / TUI — rejected; duplicates the standard preflight code without benefit; the branch in `main()` is cheaper.
- **Specialist owner:** `devops-engineer`
- **Revisit criterion:** if Phase 2 introduces a logger contract that cmux mode shares (e.g., per-run log file inside cmux mode), revisit the branch shape.
- **Dissent (if any):** —
- **Driven by rounds:** R1
- **Dependent decisions:** —
- **Referenced in plan:** Architecture and Integration Points, Decomposition and Sequencing (U4)

### D-17: D11 sanitization character set keeps `.` for now; Phase 1 release-readiness check tied to OI-1

- **Question:** Spec D11 commits to the sanitization set `[a-zA-Z0-9._-]`. Devops DOR-010 (R1, claim ledger row 17) flagged the conjectural risk that future cmux versions may reserve dotted workspace names. Drop `.` from the set or keep it pending OI-1 resolution?
- **Decision:** Keep `.` in the sanitization set as committed by spec D11. Flag the question as a Phase 1 release-readiness check tied to OI-1 (pinned cmux version). When OI-1 is resolved (the team picks a cmux release to pin), the cmux setup how-to documents whether the pinned version reserves dotted names; if it does, drop `.` from the sanitization set in a follow-up commit before Phase 1 ships.
- **Rationale:** Devops DOR-010 (R1) is conjectural — no evidence cmux reserves dotted names in any release. Removing `.` proactively would break sanitization for repos whose basename contains version suffixes (`my-app.v2`, `pr9k.git`), producing less-readable workspace names with no demonstrated benefit. The Phase 1 release-readiness check is the right place to verify against the actual pinned version.
- **Evidence:** devops DOR-010 finding (R1, claim ledger row 17); R2 OQ-R1-10 deferral; spec D11; OI-1 (pinned cmux version).
- **Rejected alternatives:**
  - Drop `.` proactively — rejected per YAGNI; conjectural risk, demonstrated cost (less-readable workspace names).
  - Defer the question entirely (don't track it as a release check) — rejected; OI-1 resolution is the correct trigger to verify, and not tracking it risks shipping a sanitization set incompatible with the pinned cmux version.
- **Specialist owner:** `devops-engineer`
- **Revisit criterion:** OI-1 resolves and the pinned cmux version's documentation reveals dotted-name restrictions.
- **Dissent (if any):** —
- **Driven by rounds:** R1, R2
- **Dependent decisions:** —
- **Referenced in plan:** Open Items, Decomposition and Sequencing (U9)

### D-18: `internal/cmuxctl` package layout — interface, RealClient, FakeClient, Preflight, RunPhase1

- **Question:** What is the package shape for the new cmux-RPC code? Single package, multiple packages, what types are exported?
- **Decision:** A single new package `internal/cmuxctl` exports: (1) `CmuxClient` interface enumerating every RPC method Phase 1 needs (`SystemIdentify`, `WorkspaceCurrent`, `WorkspaceList`, `WorkspaceCreate`, `WorkspaceClose`, `WorkspaceSelect`, `SurfaceSplit`, `SurfaceSpawn`, `SurfaceHide`, `SurfaceList`); (2) `RealClient` struct implementing `CmuxClient` over a Unix-socket JSON-RPC 2.0 connection per [D-5](#d-5-cmux-client-uses-a-single-goroutine-sequential-rpc-queue); (3) `FakeClient` struct also implementing `CmuxClient`, scriptable per-method to drive tests; (4) `Preflight(ctx, client) []error` running the five distinguishable failure-condition checks per [D-1](#d-1-cmux-preflight-integration-shape); (5) `RunPhase1(ctx, client, projectDir) error` running the workspace lifecycle (current capture, name composition, workspace.create, pane spawn, dismissal-observation, teardown, focus restore) per the Decomposition table. A compile-time interface-satisfaction assertion (`var _ CmuxClient = (*RealClient)(nil)`) lives at `RealClient`'s declaration site per `docs/coding-standards/api-design.md`.
- **Rationale:** Devops DOR-007 and the test-engineer plan (R1, claim ledger row 13) confirmed the interface + fake test-double shape from day one. Single package keeps the cmux concerns colocated. The discovery notes' "no existing cmux client code" gap and the "no existing test-fake convention for cmux-shaped clients" gap both apply: this is greenfield, and the codebase's existing fake conventions (Prober, EditorRunner) are the canonical pattern.
- **Evidence:** devops DOR-007 finding (R1, claim ledger row 13); test-engineer T2 answer; `.discovery-notes.md` (Code Touch Points, Explicitly enumerated gaps); `docs/coding-standards/api-design.md` (compile-time interface assertions); existing fakes in `internal/preflight` (`Prober` interface) and `internal/workflowedit` (`EditorRunner` interface).
- **Rejected alternatives:**
  - Split `cmuxctl` into multiple packages (e.g., `internal/cmuxctl/client` + `internal/cmuxctl/preflight` + `internal/cmuxctl/lifecycle`) — rejected; over-decomposition for the size of Phase 1's surface; the package boundary should reflect a meaningful architectural seam, not file count.
  - Put the cmux RPC types in `internal/cmuxctl` but the lifecycle logic in a new `internal/cmuxlifecycle` — rejected; same over-decomposition rationale.
  - Skip the interface and use the concrete `RealClient` directly — rejected; tests require a fake (test-engineer plan); the interface is the seam.
- **Specialist owner:** `software-architect` (proposed package shape) / `test-engineer` (fake shape)
- **Revisit criterion:** if Phase 4 sidebar mirroring or Phase 6 hardening reveals the package surface is too coarse (e.g., separate `cmuxctl/sidebar` becomes warranted), revisit.
- **Dissent (if any):** —
- **Driven by rounds:** R1
- **Dependent decisions:** D-1, D-3, D-5, D-6
- **Referenced in plan:** Architecture and Integration Points, Testing Strategy, Decomposition and Sequencing (U2, U3, U4)

### D-19: Per-pane exit observation via `surface.list` (or equivalent)

- **Question:** Spec D9's placeholder-exit observation arm depends on cmux's API exposing per-pane process-exit notifications or a queryable pane-status endpoint. Behavioral B10 and junior Q5 (R1, claim ledger row 1) flagged that this capability is not confirmed in parent T1–T5 nor the cmux API table in the investigation doc. How is per-pane exit observed?
- **Decision:** Use cmux's `surface.list` (or equivalent — name TBD per OI-1's pinned cmux version documentation) to query per-pane state, including process exit status. The dismissal-observation poller per [D-6](#d-6-dismissal-observation-via-500ms-polling-with-single-flight) calls `surface.list` each cycle alongside `workspace.list`; any pane whose state indicates "exited" satisfies spec D9's placeholder-exit arm.

  If `surface.list` (or equivalent) does not exist in the pinned cmux version, document the degradation: D9's placeholder-exit arm collapses to workspace-list-only, the close-each-pane-individually gesture cannot be observed (the workspace persists per parent T5), and pr9k blocks until either the workspace itself is closed or a placeholder process termination is otherwise observable. Surface this as a Phase 1 implementation precondition in the cmux setup how-to (similar to D18 capability check).
- **Rationale:** Spec D7 (placeholder exit collapses to dismissal) and spec D9 (dismissal observation contract) both rest on per-pane exit observability. Parent T5 confirms cmux tracks per-pane exit status visibly to operators; the natural query surface is `surface.list` by parallel with `workspace.list`. R2 OQ-R1-1 resolution committed the plan to verifying this capability against the pinned cmux version and documenting degradation if absent.
- **Evidence:** behavioral B10 finding (R1, claim ledger row 1); junior Q5; R2 OQ-R1-1 resolution; spec D9; spec D7; parent T5 (workspace persists after pane exit, exit status visible).
- **Rejected alternatives:**
  - Workspace-list-only observation — rejected per spec D9 (close-each-pane gesture cannot be observed; pr9k blocks indefinitely).
  - Process-monitoring from the host (e.g., `os.FindProcess` on the placeholder PIDs) — rejected; placeholder PIDs are owned by cmux, not pr9k; host-level process monitoring is not portable across cmux configurations.
  - Document degradation as the only behavior — rejected; if `surface.list` exists, using it is correct and Phase 1's observability is complete.
- **Specialist owner:** `devops-engineer`
- **Revisit criterion:** OI-1 resolves and pinned cmux version's documentation confirms or denies `surface.list`-equivalent per-pane exit observability; or a future cmux release adds a pane-exit subscription API.
- **Dissent (if any):** —
- **Driven by rounds:** R1, R2
- **Dependent decisions:** D-6
- **Referenced in plan:** Runtime Behavior, Open Items

### D-20: Descendant probe via socket-open with allow-all-mode caveat documented

- **Question:** Junior Q1 (R1, claim ledger row 22) flagged that the named "pr9k is not a cmux descendant" precondition error cannot fire when cmux is configured in allow-all access mode (parent T2). Pure socket-open admits any local process under allow-all; the spec's named error condition is unreachable in that configuration. How is the descendant check actually performed?
- **Decision:** Use socket-open as the descendant probe. Under cmux's default access mode (descendants-only), a non-descendant pr9k process fails to connect and pr9k prints the named "not a descendant" error. Under cmux's allow-all access mode, the connection succeeds and the error never fires (the spec's named error is unreachable in that configuration); pr9k continues with the launch. Document the allow-all caveat in the cmux setup how-to: "under allow-all access mode the 'not a descendant' error never fires (this is operator policy, not a hard pr9k precondition)."
- **Rationale:** Junior Q1 (R1) raised the question; R2 OQ-R1-2 resolution settled it. Walking the process tree to detect cmux ancestry independently of cmux's access policy would duplicate cmux's own enforcement and produce inconsistent behavior across cmux versions. The simplest correct behavior is "ask cmux; if it admits us, proceed." Operators who use allow-all access mode opt out of the ancestry guarantee voluntarily; the documentation makes the trade-off visible.
- **Evidence:** junior Q1 finding (R1, claim ledger row 22); R2 OQ-R1-2 resolution; parent T2 (cmux access modes); spec edge-cases ("pr9k is not a descendant of a cmux process" row).
- **Rejected alternatives:**
  - Walk the process tree for a cmux ancestor — rejected; duplicates cmux's enforcement, would produce inconsistent results across cmux versions and platforms.
  - Probe cmux's access mode via a `system.config` RPC and conditionally skip the connection on allow-all — rejected; cmux does not expose access-mode introspection in the documented API surface; over-engineered.
  - Refuse to launch under allow-all access mode — rejected as hostile; operators who configure allow-all do so deliberately.
- **Specialist owner:** `adversarial-security-analyst` (per security framing) / `devops-engineer` (per setup-how-to ownership)
- **Revisit criterion:** if cmux exposes access-mode introspection in a future release, revisit whether the probe should branch.
- **Dissent (if any):** —
- **Driven by rounds:** R1, R2
- **Dependent decisions:** —
- **Referenced in plan:** Architecture and Integration Points, Security Posture

### D-21: Per-call timeout protocol — close socket on timeout, re-open for teardown

- **Question:** Concurrency C4 (R1, claim ledger row 5) flagged that the per-call timeout protocol-level behavior is unspecified — when a per-call timeout fires, does the client close the socket connection or attempt to abandon the request ID and continue using the connection?
- **Decision:** Close the socket connection on timeout. The single-goroutine RPC queue per [D-5](#d-5-cmux-client-uses-a-single-goroutine-sequential-rpc-queue) issues the timeout as the response (`ErrTimeout`), closes the underlying `net.Conn`, and marks the client as unhealthy. The cleanup path that runs `workspace.close` re-opens the connection for the teardown call (one fresh connection, one teardown attempt per [D-11](#d-11-best-effort-teardown-with-operator-visible-diagnostic)).
- **Rationale:** Abandoning the request ID and reusing the connection risks routing a late response to the wrong (subsequent) request — a correctness hazard. Closing the socket and re-opening for teardown is the standard pattern for one-shot RPC-over-Unix-socket clients with sequential semantics. The cost (one new socket dial during teardown, well under the per-call timeout budget) is small.
- **Evidence:** concurrency C4 finding (R1, claim ledger row 5); R2 (concurrency findings stand without re-engagement); standard JSON-RPC client patterns for timeout handling.
- **Rejected alternatives:**
  - Abandon the request ID, keep the connection — rejected; late responses route to the wrong request, producing silent data corruption.
  - Drain the connection synchronously after the timeout, hoping the late response arrives quickly — rejected; the late response may never arrive (the original cause of the timeout); blocks shutdown indefinitely.
  - Close the connection and refuse all further RPCs (no re-open) — rejected; the teardown call is essential and must succeed if at all possible.
- **Specialist owner:** `concurrency-analyst`
- **Revisit criterion:** if Phase 4's full multiplex (per [D-5](#d-5-cmux-client-uses-a-single-goroutine-sequential-rpc-queue) revisit) requires shared connection state across multiple in-flight RPCs, the timeout-and-close-all-IDs behavior may need a more nuanced shape.
- **Dissent (if any):** —
- **Driven by rounds:** R1
- **Dependent decisions:** D-11
- **Referenced in plan:** Runtime Behavior, Decomposition and Sequencing (U2)

### D-22: Goroutine cancellation discipline — context.Context, WaitGroup, buffered channel

- **Question:** Concurrency C3+C7 (R1, claim ledger row 4) flagged that the dismissal-observer goroutine has no committed cancellation discipline and no committed channel-priming guarantee. What is the discipline?
- **Decision:** Three patterns applied together. (1) **Context cancellation:** the dismissal-observer goroutine takes a `context.Context` and respects cancellation — between poll cycles, on timer/`select` waits, and on RPC cancellation per [D-21](#d-21-per-call-timeout-protocol-close-socket-on-timeout-re-open-for-teardown). The shutdown path cancels the context before issuing teardown RPCs. (2) **WaitGroup join:** the dismissal-observer is started under a `sync.WaitGroup`; the shutdown path waits for the goroutine to exit before declaring teardown complete. (3) **Buffered dismissal channel:** the dismissal channel is buffered to size 1, so a poll observation that fires concurrently with shutdown does not block on a full channel send (the `shuttingDown` flag per [D-8](#d-8-self-close-double-fire-mitigation-via-shuttingdown-flag-and-channel-priming) drops the observation, but the buffer protects against the race window).
- **Rationale:** `docs/coding-standards/concurrency.md` codifies all three patterns: context-cancellation for goroutine cooperation, WaitGroup drain for clean shutdown, non-blocking sends from non-deterministic producers. The dismissal observer is a long-lived goroutine that must shut down cleanly during teardown; without the discipline, goroutine leaks accumulate on every cmux launch.
- **Evidence:** concurrency C3+C7 finding (R1, claim ledger row 4); `docs/coding-standards/concurrency.md` (snapshot-then-unlock, WaitGroup drain, non-blocking sends); `docs/coding-standards/testing.md` (test seams for blocking receives).
- **Rejected alternatives:**
  - Use a `done` channel instead of `context.Context` — rejected; `context.Context` is the project's standard pattern (`exec.CommandContext`, etc.) and integrates with the per-call timeout naturally.
  - Skip the WaitGroup and rely on the dismissal channel as the synchronization point — rejected; a goroutine that has issued its dismissal channel send is not necessarily exited; the WaitGroup pins the lifecycle boundary.
  - Use an unbuffered dismissal channel — rejected; introduces a race where the poll observation send blocks forever if the receiver has already moved on.
- **Specialist owner:** `concurrency-analyst`
- **Revisit criterion:** Phase 6 hardening introduces test seams for the goroutine boundary; revisit.
- **Dissent (if any):** —
- **Driven by rounds:** R1
- **Dependent decisions:** —
- **Referenced in plan:** Runtime Behavior, Testing Strategy, Decomposition and Sequencing (U5)

### D-23: Pre-sanitized basename never appears in operator-visible terminal output (code-review enforcement)

- **Question:** Devops DOR-011 (R1, claim ledger row 18) flagged that the pre-sanitized basename must never appear in operator-visible terminal output (the operator must always see the sanitized name that cmux's workspace list shows). The implementation contract is "consistent use of the sanitized value." How is this enforced?
- **Decision:** Code-review enforcement only. The implementation declares a single `composeWorkspaceName(projectDir string) (sanitizedName string, err error)` function in `internal/cmuxctl` that takes the raw project-dir, sanitizes the basename per spec D11, returns the composed `pr9k-<sanitized>-<timestamp>` workspace name. Every operator-visible output (workspace-name confirmation per spec D2, error messages, teardown diagnostics) consumes the function's return value and never the raw basename. Code review enforces no other path exists. No runtime check, no static analysis rule.
- **Rationale:** Devops DOR-011 (R1) flagged the risk; the structural fix (single function as the canonical name source) makes the wrong path require new code, which code review catches. A runtime check (e.g., scan output strings for "looks unsanitized") is over-engineered and prone to false positives. A static analysis rule would require linting that does not exist in the project tooling.
- **Evidence:** devops DOR-011 finding (R1, claim ledger row 18); spec D11; spec D2 (workspace-name confirmation prints the sanitized name).
- **Rejected alternatives:**
  - Runtime check on every output line — rejected; over-engineered, false positives, runtime cost.
  - Static analysis rule — rejected; project tooling does not include the necessary linter, and adding one for a single concern is disproportionate.
- **Specialist owner:** `software-architect` (function-shape design) / project-manager (code-review enforcement during PR)
- **Revisit criterion:** if a regression occurs (operator sees a raw basename in terminal output), revisit and add a runtime guard at the print boundary.
- **Dissent (if any):** —
- **Driven by rounds:** R1
- **Dependent decisions:** —
- **Referenced in plan:** Runtime Behavior, Decomposition and Sequencing (U4)

### D-24: Phase 1 PR scope — code + docs + version bump in single PR

- **Question:** Devops DOR-009 and junior Q7 (R1, claim ledger row 20) jointly identified that the Phase 1 PR must include the MINOR version bump (0.10.0 → 0.11.0) AND the cmux setup how-to AND a feature doc AND CLAUDE.md updates — per `coding-standards/versioning.md`, `coding-standards/documentation.md`, and parent D25. Two coding standards are simultaneously violated if any of these is omitted. How is the PR shaped?
- **Decision:** Single PR for Phase 1 ships all of: (a) `--cmux` flag wiring (`cli.Config`, `args.go`, `main.go` branch); (b) `internal/cmuxctl` package (interface, RealClient, FakeClient, Preflight, RunPhase1) plus tests; (c) ANSI hardening (`StripForTerminalOutput` per [D-14](#d-14-stripforterminaloutput-ansi-variant-for-the-cmux-diagnostic-path)); (d) `CMUX_SOCKET_PATH` validation per [D-15](#d-15-cmux_socket_path-validation-before-netdial); (e) feature doc at `docs/features/cmux-mode.md`; (f) cmux setup how-to at `docs/how-to/setting-up-cmux.md` (with OI-1 + OI-3 resolutions baked in); (g) code-package doc at `docs/code-packages/cmuxctl.md`; (h) CLAUDE.md updates linking the new docs; (i) version bump to 0.11.0 in `src/internal/version/version.go`. The version bump may live in its own commit per `coding-standards/versioning.md` ("Bump is its own commit (or combined with docs-only changes)"); within a single PR is acceptable.
- **Rationale:** `coding-standards/versioning.md` requires a MINOR bump on new public CLI surface; `coding-standards/documentation.md` requires feature docs to ship with the feature; parent D25 reinforces both. Splitting Phase 1 across multiple PRs would either (a) ship code without docs (violates documentation standard) or (b) ship docs without the code that implements them (broken docs). A single PR is the only shape that satisfies both standards.
- **Evidence:** devops DOR-009 finding (R1, claim ledger row 20); junior Q7; `docs/coding-standards/versioning.md`; `docs/coding-standards/documentation.md`; parent D25.
- **Rejected alternatives:**
  - Split across multiple PRs (e.g., code first, docs follow-up) — rejected; violates `coding-standards/documentation.md` ("Feature docs ship with the feature, not as follow-ups").
  - Ship the code without the version bump — rejected; violates `coding-standards/versioning.md` (new public CLI surface requires MINOR bump).
  - Ship the version bump in a separate post-merge release commit — rejected; the version-bump commit is the release-readiness signal; bumping after merge separates the signal from the substance.
- **Specialist owner:** project-manager (PR-scope owner) / `devops-engineer` (release shape)
- **Revisit criterion:** if the PR grows unmanageably large (>2000 LOC of Go + docs), consider splitting U9 documentation into a tightly-coupled follow-up that lands within 24 hours of the Go PR.
- **Dissent (if any):** —
- **Driven by rounds:** R1
- **Dependent decisions:** D-26
- **Referenced in plan:** Decomposition and Sequencing (U9, U10), Definition of Done, Open Items

### D-28: Pin cmux v0.64.6 as the supported floor; update the pin every cmux release

- **Question:** OI-1 — what cmux version does pr9k pin as the named minimum-supported release in the setup how-to and the Phase 1 release notes? And how is the pin maintained as cmux ships new releases?
- **Decision:** Pin **cmux v0.64.6** (released 2026-05-14, the current latest release as of plan-implementation completion) as the named minimum-supported version. The pin is **rolling**: the team commits to updating the pinned version every time cmux ships a new release. The capability check from parent D18 is the runtime safety net for operators on older versions; the pinned version is the operator-facing recommendation in the setup how-to and the release notes.
- **Rationale:** Build-outline OQ-1 recommended "Option A — pin to the current latest cmux release as the floor and re-evaluate per release"; v0.64.6 is the current latest. cmux's release pace is high — four releases in 8 days at the time of pinning (v0.64.3 on 2026-05-06, v0.64.4 on 2026-05-11, v0.64.5 on 2026-05-13, v0.64.6 on 2026-05-14) — which makes a one-time pin stale within days. Committing to a rolling pin acknowledges this. cmux is in `0.y.z` semver; per the semver spec, MINOR bumps may carry breaking API changes — the capability check from parent D18 is the right tool for that risk, and the rolling pin keeps the documented "tested against" target accurate. Updates to the pinned version are doc-only changes (no code change unless the capability check fires for a method removal); they ship under the documentation-only commit exception in `docs/coding-standards/versioning.md`.
- **Evidence:** cmux GitHub repo at https://github.com/manaflow-ai/cmux; latest release v0.64.6 fetched via `gh release view --repo manaflow-ai/cmux` on 2026-05-15; cmux release cadence visible in `gh release list --repo manaflow-ai/cmux`; build-outline OQ-1 Option A recommendation; user direction to "pin the most recent cmux version with an explicit note that we will update cmux as often as possible."
- **Rejected alternatives:**
  - Pin a major version range (e.g., `>=0.64.0`) — rejected; cmux's `0.y.z` numbering means MINOR bumps may carry breaking API changes per the semver §4 rule for the `0.y.z` initial-development tier; the capability check is the right tool for that risk, not a documented version range.
  - Pin a specific older "stable" release — rejected; nothing about an older release is more stable than the latest, and pinning older defeats the purpose of operators tracking cmux's active development.
  - Defer the pin until Phase 1 ships — rejected; the cmux setup how-to (#226) cannot ship without the pin per [D-24](#d-24-phase-1-pr-scope--code--docs--version-bump-in-single-pr), and Phase 1 cannot complete without #226 per the same decision.
  - Don't pin; rely entirely on the runtime capability check — rejected; operators benefit from a "tested against version X" line in the docs even when the runtime check would catch incompatibilities. The capability check is the safety net, not the primary documentation contract.
  - One-time pin (no rolling-update commitment) — rejected; cmux's high release cadence makes a one-time pin stale within days.
- **Specialist owner:** project-manager (decision); `devops-engineer` (operationalization — runs `gh release view --repo manaflow-ai/cmux` before each pr9k release and updates the setup how-to + release notes if the pinned version has moved).
- **Revisit criterion:** **Whenever cmux ships a new release.** The doc-only update path is a single-commit change to `docs/how-to/setting-up-cmux.md` (and the Phase release notes if the bump coincides with a pr9k release) bumping the pinned version. The capability check needs no change unless a cmux release removes a method pr9k requires — in which case the runtime error from parent D18's check fires and prompts a separate fix.
- **Dissent (if any):** —
- **Driven by rounds:** R2 (deferred to user input until plan synthesis; user provided the version-pin direction post-synthesis with the rolling-update policy).
- **Dependent decisions:** D-17 (YAGNI-1 dotted-name re-verification trigger now fires against v0.64.6).
- **Referenced in plan:** Open Items (OI-1 resolution), Decomposition and Sequencing (U9), Definition of Done.

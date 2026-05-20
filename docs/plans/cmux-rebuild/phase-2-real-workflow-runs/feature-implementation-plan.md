# Feature Implementation Plan: Phase 2 — First Real Workflow Runs End-to-End in Cmux

Phase 2 puts real workflow content into the four-pane cmux scaffold Phase 1 established. The hidden orchestrator pane drives the workflow engine; the three visible panes (header, log, footer) render live state pushed across a Unix-domain-socket interaction channel ([D-1](artifacts/implementation-decision-log.md#d-1-unix-domain-socket-per-workspace-for-the-interaction-channel)); the readiness handshake gates the first step ([D-4](artifacts/implementation-decision-log.md#d-4-readiness-handshake-protocol--ready-message-with-role-identity)); the on-disk log artifacts are equivalent in content to a standard-mode run modulo run-specifics ([D-13](artifacts/implementation-decision-log.md#d-13-log-artifact-equivalence-comparison-is-content-modulo-run-specifics-not-byte-identity)). The implementation posture is **single-PR ship**: code, tests, feature-doc update, how-to update, and version bump 0.10.0 → 0.11.0 land together ([D-17](artifacts/implementation-decision-log.md#trivial-decisions)). No live-cmux CI integration test is required; the test surface is `FakeClient` plus three new test doubles ([D-14](artifacts/implementation-decision-log.md#d-14-three-test-doubles-with-hooks-scoped-to-planned-tests)).

## Source Specification

- **Build-phase source:** [../build-phase-outline.md, Phase 2 section](../build-phase-outline.md#phase-2) (lines 130–177)
- **Parent feature specification:** [../feature-specification.md](../feature-specification.md)
- **Parent specification decision log:** [../artifacts/decision-log.md](../artifacts/decision-log.md)
- **Parent specification technical notes:** [../artifacts/feature-technical-notes.md](../artifacts/feature-technical-notes.md)
- **Phase 1 implementation plan (precedent):** [../phase-1-workspace-lifecycle/feature-implementation-plan.md](../phase-1-workspace-lifecycle/feature-implementation-plan.md)
- **Phase 1 decision log (inherited unchanged):** [../phase-1-workspace-lifecycle/artifacts/implementation-decision-log.md](../phase-1-workspace-lifecycle/artifacts/implementation-decision-log.md)

No Phase 2-specific feature specification was authored; the source is the build-phase-outline Phase 2 section plus the parent feature specification. The build outline cites parent spec sections (Outcome, Primary Flow steps 7–10 + 12, Alternate Flows, User Interactions, Coordinations) as the behavioral source.

**Parent specification decisions Phase 2 inherits:** D1 (alternate presentation only — same workflow), D4 (four-pane layout), D8 (cmux preflight runs after standard preflight, inherited via Phase 1), D13 (orchestrator pane is hidden), D14 (operator-initiated closure — display panes outlive `workflow.Run`), D16 (readiness handshake gates first step), D17 (log artifacts unchanged — sharpened to equivalent content per [D-13](artifacts/implementation-decision-log.md#d-13-log-artifact-equivalence-comparison-is-content-modulo-run-specifics-not-byte-identity)), D20 (keyboard state machine lives in the footer), D26 (status-line script runs unchanged in the footer).

**Parent technical notes Phase 2 honors:** T3 (cmux surfaces are processes — each pane must be a real process; can't paint from outside), T4 (display processes update independently — per-connection goroutines for fan-out, eventual consistency with single-digit-ms drift), T5 (cmux workspace persists after pane exit — workspace stays open after `workflow.Run` returns; reuse Phase 1's `DismissalObserver`).

**Phase 1 implementation decisions Phase 2 inherits unchanged:** D-1 through D-28. The cmux preflight, `RealClient`, `FakeClient`, `DismissalObserver`, signal handling, ANSI sanitization, `CMUX_SOCKET_PATH` validation, and four-pane spawn lifecycle all remain as Phase 1 shipped them. Phase 2 adds new packages and entry points without modifying any Phase 1 surface.

**Phase 1 Open Items Phase 2 carries forward:** OI-2 (CI testing strategy for cmux mode — still deferred to Phase 6 per build-outline OQ-2 and YAGNI-5 below).

## Outcome

When this plan executes, the codebase contains:

- A new `internal/interactionchannel` package providing the Unix-socket protocol (message framing, role-tagged handshake, intent and state-push types) and per-connection goroutine fan-out ([D-1](artifacts/implementation-decision-log.md#d-1-unix-domain-socket-per-workspace-for-the-interaction-channel), [D-2](artifacts/implementation-decision-log.md#d-2-per-connection-goroutines-with-independent-buffered-channels-for-state-push-fan-out), [D-3](artifacts/implementation-decision-log.md#d-3-separate-read-goroutine-and-write-goroutine-per-bidirectional-connection)).
- A new `pr9k cmux-pane` cobra sub-command with `--role={orchestrator|header|log|footer}` flag, providing the four per-pane entry points that the orchestrator's `SurfaceSpawn` calls launch ([D-20](artifacts/implementation-decision-log.md#d-20-per-pane-processes-via-pr9k-sub-commands-rather-than-separate-binaries)).
- A new `runCmuxOrchestrator` entry point inside the `--role=orchestrator` sub-command that creates the logger, composes the first phase header, runs the readiness handshake, drives `workflow.Run` against a `KeyHandler` adapter ([D-5](artifacts/implementation-decision-log.md#d-5-keyhandler-proxy-adapter-inside-the-orchestrator-process)), and sends the explicit `workspace_done` protocol message at completion ([D-15](artifacts/implementation-decision-log.md#d-15-workspace-done-explicit-protocol-message-with-orchestrator-waits-for-ack)).
- A new `ActionSkip` constant on `ui.StepAction` ([D-6](artifacts/implementation-decision-log.md#trivial-decisions)) so the footer can forward "skip current step" across the interaction channel.
- A `nil`-logger construction path for `statusline.Runner` so it runs in the footer pane process without a cross-process logger ([D-9](artifacts/implementation-decision-log.md#d-9-statuslinerunner-constructed-with-nil-logger-in-cmux-footer)).
- The footer pane's KeyHandler initialized with `SetStatusLineActive(true)` unconditionally, restoring the `?` help key per parent D20 ([D-22](artifacts/implementation-decision-log.md#trivial-decisions)).
- Help modal rendered as an inline expansion above the footer pane's normal content ([D-12](artifacts/implementation-decision-log.md#d-12-help-modal-renders-as-inline-expansion-above-the-footer)).
- The pre-existing `FakeClient.HangNext`/`HangRelease` mutex bug fixed in `internal/cmuxctl/fake.go` ([D-19](artifacts/implementation-decision-log.md#trivial-decisions)).
- Three new test doubles (`FakeInteractionChannel`, `FakeDisplayPane`, `FakeFooterKeySource`) with scope bounded to the planned tests ([D-14](artifacts/implementation-decision-log.md#d-14-three-test-doubles-with-hooks-scoped-to-planned-tests)).
- Updates to `docs/features/cmux-mode.md`, `docs/how-to/setting-up-cmux.md`, a new `docs/code-packages/interactionchannel.md`, and CLAUDE.md links.
- Version bump `0.10.0 → 0.11.0` ([D-17](artifacts/implementation-decision-log.md#trivial-decisions)).

When operators run `pr9k --cmux --project-dir <repo>` from inside a cmux session against a project with a real workflow configured, they observe the build-outline Phase 2 demo: the workspace appears (as in Phase 1), the four panes connect and signal ready, the workflow's first step starts, the header pane's checkboxes tick over and the iteration counter advances, the log pane streams subprocess output, the footer pane shows the status-line script's output and the shortcut hints, the operator focuses the log pane and scrolls back, presses `?` for help, presses `q` then `y` to quit cleanly, observes the final state on each pane, dismisses the workspace, and confirms the per-step JSONL artifacts in `<projectDir>/.pr9k/logs/<run-stamp>/` match what a standard-mode run would produce.

## Context

- **Driving constraint:** Phase 1 built the workspace; without Phase 2, no real workflow runs inside cmux mode and every subsequent phase is decorating an empty box. Phase 2 is the smallest demoable slice that proves the multi-process architecture actually delivers the same pr9k experience an operator gets today. Once Phase 2 ships, operators can run a real workflow inside cmux and quit it cleanly even though some chrome (sidebar mirroring, completion notification, error-recovery prompts) is still missing.
- **Stakeholders:**
  - **Operators on macOS (cmux's primary platform)** — care that workflow inputs and outputs match the standard-mode experience, the readiness handshake is fast, and the quit path produces a clean final state on each pane.
  - **pr9k maintainers (River + collaborators)** — care that the interaction-channel protocol shape is stable enough for Phase 3 (error recovery) and Phase 4 (sidebar mirroring) to extend without rework, and that the `cmux-pane` sub-command shape keeps the binary count at one.
  - **Broader cmux user community** — care that pr9k respects cmux's per-pane process model (parent T3) and per-pane SIGWINCH (per [D-11](artifacts/implementation-decision-log.md#d-11-reuse-internaluichrome-minimum-size-thresholds-per-pane)).
- **Future-state concern:** Phase 4 sidebar mirroring will add more consumers behind the same orchestrator socket; the per-connection-goroutine count grows linearly with consumer count. Phase 6 stall-threshold detection (parent OQ-4) will likely extend the interaction-channel protocol with a heartbeat-style message type. The chosen socket-protocol shape (typed JSON messages with a `type` discriminator) is extensible without breaking changes through both phases. The `T4` drift budget (single-digit-ms) is honored by topology rather than measurement — if operator reports surface drift under slow-host conditions, that becomes a Phase 6 hardening question.
- **Out-of-scope boundary:** error-recovery prompts in the footer pane (Phase 3); sidebar mirroring (Phase 4); completion notification (Phase 5); hardening against accidental closure / process loss / channel stalls (Phase 6); orphan-startup advisory (Phase 7); heartbeat indicator forwarding (deferred per YAGNI-1); JSON-schema validation of channel messages (deferred per YAGNI-2); per-pane scrollback persistence to disk (deferred per YAGNI-3); configurable handshake timeout (deferred per YAGNI-4); live-cmux integration test (deferred to Phase 6 per YAGNI-5); cross-pane drift instrumentation (deferred per YAGNI-6); status-line runner diagnostic forwarding (replaced with nil-logger per YAGNI-7 / [D-9](artifacts/implementation-decision-log.md#d-9-statuslinerunner-constructed-with-nil-logger-in-cmux-footer)); bespoke per-pane renderer packages (replaced with sub-command + renderer reuse per YAGNI-8 / [D-20](artifacts/implementation-decision-log.md#d-20-per-pane-processes-via-pr9k-sub-commands-rather-than-separate-binaries)).

## Team Composition and Participation

| Specialist | Status | Key Input |
|------------|--------|-----------|
| `project-manager` | Coordinator | Facilitated R1 (parallel specialist review) and R2 (focused user-escalation pass), applied the YAGNI rule at Step 7.5, synthesized this plan. Spec-maturity gate did not trip; PM was NOT engaged for a gate-trip facilitation pass. |
| `behavioral-analyst` | Active (R1) | Fifteen findings (B1–B15) covering OQ-3 commitment, readiness-handshake protocol shape, header pre-population gap, KeyHandler proxy contract, quit-confirmation flow across boundary, ModeError race window, ActionSkip intent type, logger creation in orchestrator, log-artifact equivalence wording, statusline.Runner cross-boundary, `StatusLineActive` gate, heartbeat indicator dependency, workspace-done observable, ModeError keystroke timing. |
| `concurrency-analyst` | Active (R1) | Eleven findings (C1–C11) covering handshake deadlock, count-only race, fan-out topology, kernel-buffer deadlock on bidirectional sockets, KeyHandler/cancel ordering, statusline coupling, heartbeat atomic, workspace-done socket close ordering, RealClient/Phase 2 socket non-contention (non-finding), single-flight observation reuse, `FakeClient.HangNext` race. |
| `test-engineer` | Active (R1) | Fourteen-row test plan (T1–T14) plus three deferrals (S1, S2, S3): readiness handshake correctness, count-only race, role-identity protocol, quit confirmation across boundary, log-artifact equivalence comparator, ActionSkip wiring, statusline runner without logger, workspace-done message and ack, ModeError race window. Named the three required test doubles. |
| `junior-developer` | Active (R1) | Ten clarifying questions (OQ-1 through OQ-10) reframing assumptions about log-artifact equivalence, orchestrator-in-separate-pane evidence, interaction-channel injectability for tests, statusline runner placement, resize minimum-size, help-modal form, display-pane lifecycle, statusline.Runner cross-boundary cost, version-label location, version-bump coordination. |
| `adversarial-security-analyst` | Not engaged this round | Phase 2 does not change the external trust boundary established in Phase 1 (`CMUX_SOCKET_PATH` validation, `ansi.StripForTerminalOutput`). The new interaction-channel socket is filesystem-permission-based and lives under the existing `.pr9k/` 0700-mode parent. Re-engage in Phase 6 hardening or earlier if the security posture (see [Security Posture](#security-posture) below) needs adversarial review. |
| `user-experience-designer` | Not engaged this round | No new operator-facing surface beyond what parent feature spec settled and what user input confirmed in R2 (resize advisory, help-modal inline expansion). Re-engage in Phase 5 (completion notification) or Phase 3 (error-recovery prompts). |
| `devops-engineer` | Not engaged this round | No new SLO impact, no new infrastructure component. Operational posture inherits Phase 1's. Re-engage if Phase 6 introduces stall-threshold detection (parent OQ-4) requiring observability. |
| `software-architect` | Not engaged this round | The new `internal/interactionchannel` package shape and the `cmux-pane` sub-command are simple-mechanism choices the team-of-four was able to settle without architectural escalation. Re-engage if Phase 4 sidebar mirroring needs the interface to fan in additional consumer types. |
| `system-architect` | Not engaged this round | Phase 2 introduces sibling processes inside a single cmux session — no cross-service / bounded-context topology. The interaction-channel socket is a local IPC concern, not a service boundary. |
| `structural-analyst` | Not engaged this round | One new package + one new sub-command + targeted changes to existing files; no SOLID violations or coupling concerns surfaced by other specialists. |
| `information-architect` | Not engaged this round | Documentation updates are extensions of Phase 1's existing docs — no new information taxonomy needed. |
| `risk-analyst` | Not engaged this round | RAID risks are individually scoped to specific findings (handshake timeout, T4 drift, protocol skew); no portfolio-level risk prioritization needed. |
| `edge-case-explorer` | Not engaged this round | Test-engineer T1–T14 covered the edge cases that matter for the protocol surface (race window, handshake deadline, double-ready, socket close vs. workspace-done). |
| `gap-analyzer` | Not engaged this round | No spec-vs-implementation gap surfaced; spec is mature enough to plan against. |
| `adversarial-validator` | Not engaged this round | Reserved for Phase 6 final hardening review. |

## Implementation Approach

### Architecture and Integration Points

**New package: `internal/interactionchannel`.** Introduces the Phase 2 IPC surface. Exports:

- `Channel` — encapsulates a single Unix-socket listener (orchestrator side) or connection (display-pane side). Owns the per-connection read goroutine and write goroutine per [D-3](artifacts/implementation-decision-log.md#d-3-separate-read-goroutine-and-write-goroutine-per-bidirectional-connection); per-connection buffered channels for outbound messages per [D-2](artifacts/implementation-decision-log.md#d-2-per-connection-goroutines-with-independent-buffered-channels-for-state-push-fan-out).
- `Message` types (one per direction):
  - Pane → orchestrator: `Ready{Role string}`, `Intent{Type IntentType}` where `IntentType ∈ {ActionRetry, ActionContinue, ActionQuit, ActionSkip, ActionNext}`, `DoneAck{Role string}`.
  - Orchestrator → pane: `StateHeader{...}`, `StateLog{Lines [][]byte}`, `StateFooter{Mode, ShortcutLine, ...}`, `WorkspaceDone{ExitCode int}`.
- `Serve(ctx, socketPath) (*Channel, error)` — orchestrator-side listener; binds the socket at the path supplied by [D-21](artifacts/implementation-decision-log.md#d-21-interaction-channel-socket-at-projectdirpr9kcmux-pane-workspacenamesock-with-0700-mode-parent), accepts up to three role-tagged connections, exposes per-role outbound channels for state pushes and a single inbound channel for intents.
- `Dial(ctx, socketPath, role) (*Channel, error)` — display-pane-side connection; sends the `Ready{Role: role}` message on connect; exposes inbound channel for state messages and outbound channel for intents.
- `FakeInteractionChannel` ([D-14](artifacts/implementation-decision-log.md#d-14-three-test-doubles-with-hooks-scoped-to-planned-tests)) — test double with scripted message exchange.

The package follows `docs/coding-standards/api-design.md` (compile-time interface assertions, adapter types), `docs/coding-standards/concurrency.md` (channel-based dispatch, mutex-protected writes, snapshot-then-unlock), and `docs/coding-standards/error-handling.md` (package-prefixed `interactionchannel: ...` errors with the resolved socket path in I/O errors). Message framing uses length-prefixed JSON (`Length uint32 || JSON bytes`) — sized for the typical state-push payload (~1 KB) with headroom for log-line bursts.

**New CLI sub-command: `pr9k cmux-pane`.** Cobra sub-command living in `src/cmd/pr9k/cmux_pane.go` (or `src/internal/cli/cmux_pane.go` per existing pattern). Flag: `--role` (required) with values `orchestrator|header|log|footer`. Reads the interaction-channel socket path from the `PR9K_CMUX_SOCKET` environment variable. The sub-command's `Run` function dispatches to one of four thin entry points:

- `runCmuxOrchestrator(ctx)` — creates the logger ([D-7](artifacts/implementation-decision-log.md#d-7-logger-creation-inside-the-orchestrator-pane-process)), runs `Serve`, awaits readiness handshake ([D-4](artifacts/implementation-decision-log.md#d-4-readiness-handshake-protocol--ready-message-with-role-identity)), pre-populates first phase header state ([D-8](artifacts/implementation-decision-log.md#d-8-pre-populate-first-phase-header-state-in-the-orchestrator-before-unblocking-the-handshake)), constructs `RunHeader` and `KeyHandler` adapters ([D-5](artifacts/implementation-decision-log.md#d-5-keyhandler-proxy-adapter-inside-the-orchestrator-process)), drives `workflow.Run`, sends `WorkspaceDone` ([D-15](artifacts/implementation-decision-log.md#d-15-workspace-done-explicit-protocol-message-with-orchestrator-waits-for-ack)), waits for acks with 5s timeout, exits.
- `runCmuxHeaderPane(ctx)` — connects via `Dial`, reuses `internal/ui.StatusHeader` renderer, listens for `StateHeader` messages, applies minimum-size thresholds ([D-11](artifacts/implementation-decision-log.md#d-11-reuse-internaluichrome-minimum-size-thresholds-per-pane)), stays alive until `WorkspaceDone` then renders final state until pane exit ([D-16](artifacts/implementation-decision-log.md#d-16-reuse-phase-1s-dismissalobserver-unchanged)).
- `runCmuxLogPane(ctx)` — connects via `Dial`, reuses the standard log-panel renderer from `internal/ui`, listens for `StateLog` messages with drop-oldest semantics on burst, exposes pane-local scrollback via the existing `viewport.Model` ring buffer pattern, stays alive after `WorkspaceDone`.
- `runCmuxFooterPane(ctx)` — connects via `Dial`, constructs a `KeyHandler` with `SetStatusLineActive(true)` ([D-22](artifacts/implementation-decision-log.md#trivial-decisions)) and the `nil`-logger `statusline.Runner` ([D-9](artifacts/implementation-decision-log.md#d-9-statuslinerunner-constructed-with-nil-logger-in-cmux-footer)), handles `q`/`y` quit-confirmation locally, forwards fully resolved intents (`ActionRetry`, `ActionContinue`, `ActionQuit`, `ActionSkip`, `ActionNext`) to the orchestrator, renders inline help expansion on `?` ([D-12](artifacts/implementation-decision-log.md#d-12-help-modal-renders-as-inline-expansion-above-the-footer)), renders the version label `pr9k v0.11.0` ([D-18](artifacts/implementation-decision-log.md#trivial-decisions)), stays alive after `WorkspaceDone`.

**Existing-package modifications:**

- `src/internal/cmuxctl/runphase1.go`: change pane spawn argv from `sh -c '<placeholder-printf> && tail -f /dev/null'` to `<pr9k-binary> cmux-pane --role=<role>`; set `PR9K_CMUX_SOCKET` env via the `SurfaceSpawn` env parameter; pass the orchestrator-side socket path.
- `src/internal/cmuxctl/fake.go`: add `f.mu.Lock()/Unlock()` around `HangNext`/`HangRelease` field accesses ([D-19](artifacts/implementation-decision-log.md#trivial-decisions)).
- `src/internal/ui/ui.go`: add `ActionSkip` constant to `StepAction` ([D-6](artifacts/implementation-decision-log.md#trivial-decisions)).
- `src/internal/ui/keys.go`: footer's KeyHandler initialization gains the unconditional `SetStatusLineActive(true)` ([D-22](artifacts/implementation-decision-log.md#trivial-decisions)).
- `src/internal/statusline/statusline.go`: allow `nil` `*logger.Logger` (either guard `logLine` or accept a `noopLogger` adapter; implementation detail).
- `src/internal/version/version.go`: bump to `0.11.0` ([D-17](artifacts/implementation-decision-log.md#trivial-decisions)).
- `src/cmd/pr9k/main.go`: the existing `cfg.Cmux` branch (Phase 1) is extended to launch the `pr9k cmux-pane --role=orchestrator` sub-process implicitly via the new spawn argv in `RunPhase1`; the `--cmux` flag's behavior is unchanged from the operator's perspective.

### Data Model and Persistence

Phase 2 reintroduces the standard-mode log-artifact path: `<projectDir>/.pr9k/logs/<run-stamp>/` containing the per-run log file and per-step JSONL artifacts. The orchestrator pane process owns the `*logger.Logger` ([D-7](artifacts/implementation-decision-log.md#d-7-logger-creation-inside-the-orchestrator-pane-process)); the display panes never write to disk. Acceptance is equivalent content modulo run-specifics per [D-13](artifacts/implementation-decision-log.md#d-13-log-artifact-equivalence-comparison-is-content-modulo-run-specifics-not-byte-identity) — per-step JSONL files match line-by-line between a standard-mode reference run and a cmux-mode run, excluding the RunStamp directory name, wall-clock timestamps inside log lines, and any `lipgloss.Width`-dependent renders.

The interaction-channel socket file at `<projectDir>/.pr9k/cmux-pane-<workspaceName>.sock` ([D-21](artifacts/implementation-decision-log.md#d-21-interaction-channel-socket-at-projectdirpr9kcmux-pane-workspacenamesock-with-0700-mode-parent)) is ephemeral: created at orchestrator startup, unlinked at orchestrator exit. No schema changes, no migrations, no persistent state beyond what Phase 1 already creates.

### Runtime Behavior

**Launch sequence (extending Phase 1's lifecycle):**

1. `pr9k --cmux --project-dir <repo>` proceeds through Phase 1's preflight and workspace-creation steps unchanged.
2. Just before `RunPhase1`'s spawn step, the orchestrator-side socket path is computed: `<projectDir>/.pr9k/cmux-pane-<workspaceName>.sock`.
3. `RunPhase1` spawns the four panes orchestrator-first via the cmux `SurfaceSpawn` API ([Phase 1 D-3](../phase-1-workspace-lifecycle/artifacts/implementation-decision-log.md#d-3-pane-spawn-order-is-orchestrator-first)) — argv `<pr9k> cmux-pane --role=<role>`, env `PR9K_CMUX_SOCKET=<path>`.
4. The orchestrator process: starts up, calls `logger.NewLogger(projectDir)` ([D-7](artifacts/implementation-decision-log.md#d-7-logger-creation-inside-the-orchestrator-pane-process)), calls `interactionchannel.Serve(ctx, socketPath)` ([D-1](artifacts/implementation-decision-log.md#d-1-unix-domain-socket-per-workspace-for-the-interaction-channel)). The accept loop spawns per-connection read+write goroutine pairs per [D-3](artifacts/implementation-decision-log.md#d-3-separate-read-goroutine-and-write-goroutine-per-bidirectional-connection).
5. Each display pane process: starts up, calls `interactionchannel.Dial(ctx, socketPath, role)`, sends `Ready{Role: role}` per [D-4](artifacts/implementation-decision-log.md#d-4-readiness-handshake-protocol--ready-message-with-role-identity).
6. The orchestrator awaits the readiness handshake (mutex-protected per-role bool map; 10s deadline). On all-three-ready, pre-populates the first phase header state per [D-8](artifacts/implementation-decision-log.md#d-8-pre-populate-first-phase-header-state-in-the-orchestrator-before-unblocking-the-handshake) and pushes the first state messages.
7. The orchestrator constructs the `KeyHandler` adapter ([D-5](artifacts/implementation-decision-log.md#d-5-keyhandler-proxy-adapter-inside-the-orchestrator-process)) and calls `workflow.Run` with the standard `StepExecutor`, the `RunHeader`-compatible state-push adapter, and the `KeyHandler` adapter. `workflow.Run` produces logger artifacts under `<projectDir>/.pr9k/logs/<run-stamp>/` exactly as in standard mode.
8. Each state mutation in `workflow.Run` flows through the per-connection goroutines per [D-2](artifacts/implementation-decision-log.md#d-2-per-connection-goroutines-with-independent-buffered-channels-for-state-push-fan-out) to the three display panes.
9. Display panes render incoming state via existing `internal/ui` renderers (re-used per [D-20](artifacts/implementation-decision-log.md#d-20-per-pane-processes-via-pr9k-sub-commands-rather-than-separate-binaries)).
10. Operator interactions in the footer pane: `q`/`y` handled locally per parent D20, fully resolved intents forwarded to the orchestrator; `?` triggers inline help expansion per [D-12](artifacts/implementation-decision-log.md#d-12-help-modal-renders-as-inline-expansion-above-the-footer); the version label renders in the footer's corner per [D-18](artifacts/implementation-decision-log.md#trivial-decisions).

**Completion sequence:**

1. `workflow.Run` returns inside the orchestrator.
2. Orchestrator broadcasts `WorkspaceDone{ExitCode: N}` to all three panes per [D-15](artifacts/implementation-decision-log.md#d-15-workspace-done-explicit-protocol-message-with-orchestrator-waits-for-ack).
3. Each pane receives `WorkspaceDone`, transitions to `ModeDone` locally, renders the final state, sends `DoneAck{Role: role}`.
4. Orchestrator waits for all three acks (5s deadline). On ack-or-timeout, closes the interaction-channel socket and exits.
5. Phase 1's `DismissalObserver` ([D-16](artifacts/implementation-decision-log.md#d-16-reuse-phase-1s-dismissalobserver-unchanged)) sees the orchestrator pane exit (per-pane-exit arm) and fires dismissal; the existing Phase 1 teardown runs (`WorkspaceClose`, focus restore).
6. The three display panes stay alive after the socket closes, hosting their final-state render until the operator's workspace-close or close-each-pane gesture finishes the dismissal.

**Resize behavior.** Each pane has its own SIGWINCH (parent T3). Below the `MinTerminalWidth=60` / `MinTerminalHeight=16` thresholds from `internal/uichrome` per [D-11](artifacts/implementation-decision-log.md#d-11-reuse-internaluichrome-minimum-size-thresholds-per-pane), the pane renders a single-line "make this pane wider" advisory and waits for the next resize. Above threshold, normal rendering resumes.

**Failure paths.**

- Handshake timeout (10s, no `Ready` from one or more roles): orchestrator fires the fatal-teardown sentinel through the same dismissal path Phase 1 established; exits non-zero. Operator-visible error names which role(s) did not signal ready.
- Pane process exits before `WorkspaceDone`: orchestrator's connection-read goroutine receives `io.EOF`; the orchestrator's adapter treats this as the operator quitting (`ActionQuit` intent) and aborts the workflow cleanly. Display loss is detectable; Phase 6 will harden the response (display reconnect / orchestrator survival), but Phase 2 treats it as a quit.
- Orchestrator crashes mid-run: display panes receive `io.EOF` on their connection; each pane's read goroutine logs the disconnection and switches to a static "pr9k orchestrator unavailable — dismiss the workspace" render. Phase 1's `DismissalObserver` fires when cmux observes the orchestrator pane exited.

### External Interfaces

**CLI.** New `pr9k cmux-pane` sub-command with `--role={orchestrator|header|log|footer}` flag ([D-20](artifacts/implementation-decision-log.md#d-20-per-pane-processes-via-pr9k-sub-commands-rather-than-separate-binaries)). Reserved for internal use — pr9k's own `RunPhase1` invokes it via `SurfaceSpawn`; operators do not run it directly. The sub-command is not marked `Hidden` (consistent with Phase 1's `--cmux` decision); help text names it as an internal sub-command launched by cmux mode.

**Environment variables.** `PR9K_CMUX_SOCKET` (new, internal) — interaction-channel socket path; set by the orchestrator's spawn calls; consumed by the `cmux-pane` sub-command. Inherits the existing `CMUX_SOCKET_PATH` validation pattern Phase 1 established.

**Cmux JSON-RPC 2.0.** No new cmux RPCs are called beyond what Phase 1 already wires; the spawn argv changes from `sh -c '...'` to `pr9k cmux-pane --role=<role>` but the `SurfaceSpawn` shape is unchanged.

**Interaction-channel protocol (Unix domain socket).** Length-prefixed JSON messages. Message types per Architecture section. Filesystem-permission-based access control via the 0700-mode `.pr9k/` parent directory per [D-21](artifacts/implementation-decision-log.md#d-21-interaction-channel-socket-at-projectdirpr9kcmux-pane-workspacenamesock-with-0700-mode-parent). Internal to pr9k — not a public contract.

**Operator-visible terminal output.** Phase 1's surfaces (workspace-name confirmation, preflight errors, partial-setup teardown diagnostic, orphan-workspace diagnostic) remain unchanged. Phase 2 adds: per-pane rendering inside the cmux workspace (header checkboxes + iteration counter, log streaming, footer status line + shortcuts + version label + inline help expansion), the readiness-handshake-timeout error (if it fires), and the standard-mode log artifacts at `<projectDir>/.pr9k/logs/<run-stamp>/`.

## Decomposition and Sequencing

| # | Work Unit | Delivers | Depends On | Verification |
|---|-----------|----------|------------|--------------|
| U1 | `internal/interactionchannel` package skeleton: `Channel`, `Message` types, length-prefixed JSON framing, `Serve` / `Dial` functions, per-connection read+write goroutine pairs ([D-1](artifacts/implementation-decision-log.md#d-1-unix-domain-socket-per-workspace-for-the-interaction-channel), [D-3](artifacts/implementation-decision-log.md#d-3-separate-read-goroutine-and-write-goroutine-per-bidirectional-connection)) | Channel types and basic IPC plumbing exist; no protocol semantics yet | — | Unit tests: a `Serve`-listener accepts a `Dial`-client and round-trips a hand-built message; both read and write goroutines exit cleanly on context cancellation; race detector passes |
| U2 | Readiness handshake — `Ready` message type, mutex-protected per-role bool map in orchestrator, 10s deadline, fatal-teardown sentinel on timeout ([D-4](artifacts/implementation-decision-log.md#d-4-readiness-handshake-protocol--ready-message-with-role-identity)) | Orchestrator blocks until all three display panes signal ready or the deadline trips | U1 | Unit tests via `FakeInteractionChannel`: all-three-ready unblocks; two-ready-then-deadline trips the timeout with the correct missing-roles error; a duplicate-ready (reconnect) does not double-count |
| U3 | `pr9k cmux-pane` cobra sub-command with `--role` flag and `PR9K_CMUX_SOCKET` env reading; thin entry-point dispatch to four runners ([D-20](artifacts/implementation-decision-log.md#d-20-per-pane-processes-via-pr9k-sub-commands-rather-than-separate-binaries)); `ActionSkip` enum addition ([D-6](artifacts/implementation-decision-log.md#trivial-decisions)) | The sub-command parses, dispatches, and the four entry points exist as stubs | U1, U2 | Unit tests: `--role=...` parses each value; `--role=` missing produces an error; the `PR9K_CMUX_SOCKET` env var is read; each runner is invoked for the corresponding role |
| U4 | State-push fan-out — per-connection buffered channels, drop-oldest log-line semantics, latest-wins for header/footer mode-state ([D-2](artifacts/implementation-decision-log.md#d-2-per-connection-goroutines-with-independent-buffered-channels-for-state-push-fan-out)); first-phase header pre-population ([D-8](artifacts/implementation-decision-log.md#d-8-pre-populate-first-phase-header-state-in-the-orchestrator-before-unblocking-the-handshake)); heartbeat indicator dropped in cmux mode ([D-10](artifacts/implementation-decision-log.md#d-10-drop-heartbeat-indicator-in-cmux-phase-2)) | Orchestrator can publish state messages to header / log / footer at T4-compatible drift | U1, U2 | Unit tests via `FakeInteractionChannel` + `FakeDisplayPane`: a slow consumer back-pressures only its own channel; log-line burst with full buffer drops oldest lines; first-phase header state arrives as the first message after handshake; race detector passes |
| U5 | Footer keyboard state machine + KeyHandler adapter — local `q`/`y` handling, intent forwarding for `ActionRetry`/`ActionContinue`/`ActionQuit`/`ActionSkip`/`ActionNext`, mode-state push back from orchestrator to footer ([D-5](artifacts/implementation-decision-log.md#d-5-keyhandler-proxy-adapter-inside-the-orchestrator-process), [D-6](artifacts/implementation-decision-log.md#trivial-decisions)) | Footer process handles keystrokes locally, forwards only fully resolved intents | U3, U4 | Unit tests via `FakeFooterKeySource` + `FakeInteractionChannel`: `q` then `y` produces exactly one `ActionQuit` intent; `q` then `esc` produces no intent; `ActionSkip` intent maps to the orchestrator's KeyHandler `Actions` channel correctly |
| U6 | Logger creation in `runCmuxOrchestrator` mirroring standard `startup()` ([D-7](artifacts/implementation-decision-log.md#d-7-logger-creation-inside-the-orchestrator-pane-process)); log artifact equivalence-comparison test harness ([D-13](artifacts/implementation-decision-log.md#d-13-log-artifact-equivalence-comparison-is-content-modulo-run-specifics-not-byte-identity)) | Orchestrator produces `<projectDir>/.pr9k/logs/<run-stamp>/` directory with per-step JSONL artifacts | U3 | Unit tests: orchestrator startup creates the logger directory; per-step JSONL files exist; equivalence comparator excludes RunStamp / wall-clock / width-dependent renders; comparator returns "equivalent" for content-matching artifacts |
| U7 | Status-line script in footer pane — `statusline.Runner` constructed with `nil` logger, runs script on existing cadence, exposes `Sender` to the footer's local renderer ([D-9](artifacts/implementation-decision-log.md#d-9-statuslinerunner-constructed-with-nil-logger-in-cmux-footer)) | Footer pane runs the operator's configured status-line script per parent D26 | U5 | Unit tests: nil-logger construction does not panic; `logLine` is a no-op; the runner exec path is exercised; status-line output reaches the footer's render |
| U8 | Help-modal inline expand above footer ([D-12](artifacts/implementation-decision-log.md#d-12-help-modal-renders-as-inline-expansion-above-the-footer)); `SetStatusLineActive(true)` unconditional in footer's KeyHandler init ([D-22](artifacts/implementation-decision-log.md#trivial-decisions)); version label preserved in footer corner ([D-18](artifacts/implementation-decision-log.md#trivial-decisions)); minimum-size advisory ([D-11](artifacts/implementation-decision-log.md#d-11-reuse-internaluichrome-minimum-size-thresholds-per-pane)) | Operator can press `?` to expand help inline; below 60×16 the pane shows the "make wider" advisory; version label reads `pr9k v0.11.0` | U5 | Unit tests: `?` toggles help mode; `esc` collapses; below threshold the advisory is rendered; version label string matches `version.Version` |
| U9 | Workspace-done protocol — orchestrator broadcasts `WorkspaceDone` ([D-15](artifacts/implementation-decision-log.md#d-15-workspace-done-explicit-protocol-message-with-orchestrator-waits-for-ack)); panes transition to `ModeDone`, send `DoneAck`; orchestrator waits 5s for acks then closes socket; Phase 1's `DismissalObserver` reused ([D-16](artifacts/implementation-decision-log.md#d-16-reuse-phase-1s-dismissalobserver-unchanged)) | Workflow completion produces a clean operator-visible final state; orchestrator exits without false-positive abort | U4, U6, U8 | Unit tests via `FakeInteractionChannel` + `FakeDisplayPane`: `WorkspaceDone` produces one `DoneAck` per role; orchestrator closes socket after acks-or-timeout; a pane that fails to ack does not hang the orchestrator past 5s; orchestrator-pane exit is observed by `DismissalObserver` exactly as in Phase 1 |
| U10 | Test doubles — `FakeInteractionChannel`, `FakeDisplayPane`, `FakeFooterKeySource` ([D-14](artifacts/implementation-decision-log.md#d-14-three-test-doubles-with-hooks-scoped-to-planned-tests)); fix `FakeClient.HangNext`/`HangRelease` mutex bug ([D-19](artifacts/implementation-decision-log.md#trivial-decisions)) | Test doubles available with the hooks T1–T14 need; the pre-existing `FakeClient` race detector finding is resolved | parallel with U1–U9 | Unit tests on the doubles themselves; race detector clean for `FakeClient.HangNext`/`HangRelease` path |
| U11 | Documentation — update `docs/features/cmux-mode.md` for Phase 2 behavior; update `docs/how-to/setting-up-cmux.md` for the real-workflow run experience; create `docs/code-packages/interactionchannel.md`; update CLAUDE.md feature/code-package lists | Operators understand the Phase 2 demo; maintainers can navigate the new package | U1–U9 (docs reflect actual behavior) | `make build` passes; doc code blocks match production code; CLAUDE.md links resolve |
| U12 | Version bump `0.10.0` → `0.11.0` in `src/internal/version/version.go` ([D-17](artifacts/implementation-decision-log.md#trivial-decisions)) — combined with U11 docs in the same commit grouping per `docs/coding-standards/versioning.md` | `pr9k --version` prints `0.11.0`; release-readiness signal | U1–U11 (all surface changes complete) | `make build` passes; `pr9k --version` output matches |

## RAID Log

### Risks

| ID | Risk | Likelihood | Severity | Blast Radius | Reversibility | Owner | Mitigation |
|----|------|------------|----------|--------------|---------------|-------|------------|
| R1 | Per-connection goroutine count grows as Phase 4 adds sidebar mirroring; eventually the linear growth becomes meaningful on a 30-pane cmux session ([D-2](artifacts/implementation-decision-log.md#d-2-per-connection-goroutines-with-independent-buffered-channels-for-state-push-fan-out)) | Medium (Phase 4 will need more consumers) | Low (goroutine count remains in the 10s, not 1000s; Go's scheduler handles 10s of goroutines effortlessly) | `internal/interactionchannel` | Reversible (multiplex-with-shared-writer refactor possible) | `concurrency-analyst` (Phase 4) | Defer to Phase 4 measurement; Phase 2's topology is correct for the consumer count it has |
| R2 | T4 cross-pane drift budget (single-digit-ms) is observable on slow hosts (CPU-constrained CI runners, busy macOS systems); operator reports of visible desync | Low (parent T4 commits to the budget under typical conditions; no production traffic exists) | Medium (operator sees desync; not data-loss) | Cmux mode users | Reversible (tune buffer sizes, drop-oldest tuning) | `concurrency-analyst` | Measurement deferred per YAGNI-6; honor T4 by topology in Phase 2; reopen if operator reports surface |
| R3 | Interaction-channel protocol shape skews between Phase 2 and Phase 3 (Phase 3 may add new intent types); a Phase 2 orchestrator paired with a Phase 3 pane (or vice versa) produces confusing errors | Low (single binary per release; cross-version pairings require manual binary swap) | Medium (operator confusion until pairing identified) | Cmux mode users | Reversible (the JSON-message `type` discriminator naturally handles new types) | `behavioral-analyst` (Phase 3) | Document the protocol-type set in `docs/code-packages/interactionchannel.md`; Phase 3 adds types additively; YAGNI-2 reopens schema validation if shape skew becomes a real incident class |
| R4 | Per-pane Bubble Tea startup latency contributes to the 10s handshake window; on a slow macOS host with cold disk caches a pane may take >2s to reach `Dial`, leaving little headroom | Low (Bubble Tea startup on warm caches is ~50–200ms; cmux's pane-spawn latency is the larger constant) | Low (handshake-timeout produces a clean error; no silent corruption) | Cmux mode users | Reversible (extend the deadline, profile the startup) | `behavioral-analyst` | The handshake error message names the slow-to-ready role(s) so the operator can diagnose; if false-positives accumulate, raise the deadline or shrink Bubble Tea startup |
| R5 | SIGKILL of orchestrator (e.g., `kill -9` from outside, OOM kill) leaves display panes alive; each pane's read goroutine sees `io.EOF` and falls back to the "orchestrator unavailable — dismiss" render, but the operator sees the message three times (one per pane) | Low (SIGKILL is rare; cmux's per-pane render is independent per parent T3) | Low (cosmetic — three messages instead of one; functionally correct) | Cmux mode users | Reversible (Phase 6 hardens display-loss handling) | `behavioral-analyst` (Phase 6) | Phase 2 documents the behavior in the feature doc; Phase 6 may centralize the message via a fourth pane or a shared file |
| R6 | The 5-second `DoneAck` timeout is too short for a footer pane whose status-line script is mid-exec when `WorkspaceDone` arrives — the script's max-runtime budget plus the runner's snapshot lag could exceed 5s | Low (status-line scripts typically complete in <1s by design) | Low (the orchestrator closes the socket and exits anyway; the footer's final state still renders) | Cmux mode users | Reversible (tune the ack timeout if reports surface) | `behavioral-analyst` | If the runner is in mid-exec when `WorkspaceDone` arrives, the footer's ack is sent from the runner's reentrancy point; profile in Phase 6 |
| R7 | The pre-existing `FakeClient.HangNext`/`HangRelease` race fix ([D-19](artifacts/implementation-decision-log.md#trivial-decisions)) inadvertently changes test timing for Phase 1's existing tests | Low (mutex protection adds nanoseconds; Phase 1 tests are not timing-sensitive at that scale) | Low (a brittle test surfaces and needs the timing assumption fixed) | Phase 1 + Phase 2 test code | Reversible (revert the fix and re-evaluate) | `test-engineer` | `make test` runs the full Phase 1 test suite before Phase 2 lands; flaky-test triage is the standard path |

### Assumptions

| ID | Assumption | What Changes If Wrong | Verifier | Status |
|----|------------|-----------------------|----------|--------|
| A1 | `internal/ui.StatusHeader`, the log-panel renderer, and `KeyHandler` are usable from a sub-command entry point without standard-mode-specific dependencies (e.g., the Bubble Tea program loop) | The four runners need bespoke renderer wrappers; YAGNI-8's simpler-version premise weakens | Inspect the renderer types' constructors during U3/U4 implementation | Unverified — verifiable during U3+U4 |
| A2 | `statusline.Runner` tolerates `nil` `*logger.Logger` with minimal modification (one-line guard in `logLine`) | The footer's runner needs a `noopLogger` adapter or a constructor option; D-9 implementation is two lines instead of one | Inspect `statusline.go:54-56` during U7 | Unverified — verifiable during U7 |
| A3 | Cmux's `SurfaceSpawn` API supports passing an env-var map at spawn time (e.g., `PR9K_CMUX_SOCKET`) | The socket path must be passed via argv or via a process-tree-inheritance dance; D-21's env-var mechanism becomes argv | Inspect cmux v0.64.6 `surface.spawn` schema during U3 implementation; investigation doc `docs/plans/cmux-rebuild/investigation.md` should have the answer | Unverified — verifiable during U3 |
| A4 | Length-prefixed JSON framing is sufficient for the message volumes Phase 2 sends (no need for streaming framers like protobuf or message-pack) | Log-line bursts (large `StateLog` messages) need a separate streaming channel; protocol shape grows | Profile a real workflow run during U4 + U9 verification | Unverified — verifiable during integration testing |
| A5 | The `pr9k` binary path is resolvable at spawn time (cmux's `SurfaceSpawn` can invoke `pr9k cmux-pane ...` with the same binary that launched the orchestrator) | The orchestrator must derive its own binary path via `os.Executable()` and pass it absolutely to `SurfaceSpawn` | Inspect `RunPhase1` spawn-argv construction during U3 | Unverified — verifiable during U3 |

### Issues

| ID | Issue | Owner | Next Step |
|----|-------|-------|-----------|
| I1 | Phase 1 OI-2 (CI testing strategy for cmux mode) carries forward to Phase 2 unchanged | project-manager | Defer to Phase 6 per YAGNI-5 and build-outline OQ-2 |

### Dependencies

| ID | Dependency | Owner | Status |
|----|------------|-------|--------|
| Dep1 | Cmux v0.64.6's `surface.spawn` accepts an env-var map (or accepts an env-var convention pr9k controls) | `devops-engineer` | Verify against pinned cmux version; investigation doc sketches the API |
| Dep2 | The four `internal/ui` renderers (`StatusHeader`, log-panel, `KeyHandler`, footer renderer) are reusable from a sub-command entry point | `software-architect` (advisor only) | Verify during U3+U4 implementation; if not, plan tweaks per A1 |

## Testing Strategy

The test plan rests on test-engineer T1 through T14 from R1 plus the three deferrals (S1, S2, S3) recorded as YAGNI items (YAGNI-5, YAGNI-6, others as noted). The test seam is the new `interactionchannel.FakeInteractionChannel`, `FakeDisplayPane`, and `FakeFooterKeySource` ([D-14](artifacts/implementation-decision-log.md#d-14-three-test-doubles-with-hooks-scoped-to-planned-tests)), plus the inherited `FakeClient` for cmux JSON-RPC. Every behavioral commitment except live-cmux integration is exercised through these doubles.

- **Observable behaviors to test:**
  - Readiness handshake: all-three-ready unblocks the orchestrator; two-ready-then-deadline trips the timeout with the named missing-roles error; duplicate `Ready` (reconnect) does not double-count ([D-4](artifacts/implementation-decision-log.md#d-4-readiness-handshake-protocol--ready-message-with-role-identity)).
  - State-push fan-out: a slow consumer back-pressures only its own channel; log-line burst with full buffer drops oldest lines; first-phase header pre-population arrives as the first state message ([D-2](artifacts/implementation-decision-log.md#d-2-per-connection-goroutines-with-independent-buffered-channels-for-state-push-fan-out), [D-8](artifacts/implementation-decision-log.md#d-8-pre-populate-first-phase-header-state-in-the-orchestrator-before-unblocking-the-handshake)).
  - KeyHandler adapter: each intent type round-trips orchestrator ↔ footer with the correct `KeyHandler.Actions` channel mapping; `SetMode` calls forward outbound to the footer ([D-5](artifacts/implementation-decision-log.md#d-5-keyhandler-proxy-adapter-inside-the-orchestrator-process), [D-6](artifacts/implementation-decision-log.md#trivial-decisions)).
  - Quit-confirmation flow: `q` then `y` produces one `ActionQuit` intent; `q` then `esc` produces no intent; partial keystrokes do not leak across the boundary.
  - ModeError race window: a state-push for `SetMode(ModeError)` followed immediately by an `<-h.Actions` block does not drop a footer keystroke pressed in the race window.
  - Logger artifacts: orchestrator startup creates the per-run directory; per-step JSONL artifacts exist; equivalence comparator returns "equivalent" for content-matching artifacts and "diverged at line N" for genuinely different content; the comparator excludes RunStamp, wall-clock timestamps, and width-dependent renders ([D-7](artifacts/implementation-decision-log.md#d-7-logger-creation-inside-the-orchestrator-pane-process), [D-13](artifacts/implementation-decision-log.md#d-13-log-artifact-equivalence-comparison-is-content-modulo-run-specifics-not-byte-identity)).
  - Status-line in footer: `nil`-logger construction does not panic; the runner exec path produces output reaching the footer's render ([D-9](artifacts/implementation-decision-log.md#d-9-statuslinerunner-constructed-with-nil-logger-in-cmux-footer)).
  - Help inline expand: `?` toggles expansion; `esc` collapses; expansion is bounded to available pane height ([D-12](artifacts/implementation-decision-log.md#d-12-help-modal-renders-as-inline-expansion-above-the-footer)).
  - Minimum-size advisory: below 60×16 the pane renders the advisory; above threshold normal rendering resumes ([D-11](artifacts/implementation-decision-log.md#d-11-reuse-internaluichrome-minimum-size-thresholds-per-pane)).
  - Workspace-done: `WorkspaceDone` produces one `DoneAck` per role; orchestrator waits ≤5s for acks; a non-acking pane does not hang the orchestrator; orchestrator-pane exit is observed by Phase 1's `DismissalObserver` exactly as before ([D-15](artifacts/implementation-decision-log.md#d-15-workspace-done-explicit-protocol-message-with-orchestrator-waits-for-ack), [D-16](artifacts/implementation-decision-log.md#d-16-reuse-phase-1s-dismissalobserver-unchanged)).
  - Version label: footer renders `pr9k v0.11.0` ([D-18](artifacts/implementation-decision-log.md#trivial-decisions)).
  - `FakeClient.HangNext`/`HangRelease`: race detector remains clean under concurrent access ([D-19](artifacts/implementation-decision-log.md#trivial-decisions)).
- **Test doubles posture:** Three new doubles per [D-14](artifacts/implementation-decision-log.md#d-14-three-test-doubles-with-hooks-scoped-to-planned-tests), bounded to the planned tests' needs. All doubles `sync.Mutex`-protected per `docs/coding-standards/testing.md`.
- **Edge cases requiring coverage:**
  - Reconnecting display pane (Ready arriving twice from the same role) — handshake's per-role bool stays true; duplicate is idempotent.
  - Log-line burst exceeding buffer capacity — drop-oldest applied; no panic, no goroutine leak.
  - Pane process exit before `WorkspaceDone` — orchestrator treats as `ActionQuit` and aborts cleanly.
  - Orchestrator crash mid-run — display panes render the "orchestrator unavailable" message.
  - Footer keystroke during the `SetMode(ModeError)` race window (CL-6) — intent reaches the orchestrator after the mode applies.
- **Test levels:** unit (every behavioral commitment via the doubles); integration (live-cmux smoke test deferred per OI-2 / YAGNI-5; revisit at Phase 6).
- **Race detector:** `make test` runs `go test -race ./...` per `docs/coding-standards/testing.md`. Goroutines under scrutiny: per-connection read+write pairs (orchestrator side ×3 + pane side ×1 each), the per-role-bool mutex in the handshake, the `KeyHandler` adapter goroutine, the fan-out goroutines, the workspace-done ack-wait, plus all Phase 1 goroutines (single-flight RPC queue, dismissal observer, signal handler).

## Security Posture

Phase 2 introduces a new Unix domain socket inside the workspace's pr9k state directory (`<projectDir>/.pr9k/cmux-pane-<workspaceName>.sock`) carrying traffic between sibling processes under the same user account. The four processes (orchestrator + three display panes) are all spawned by the same cmux session and run with the same UID; no cross-user trust boundary is crossed.

- **Access control:** filesystem-permission-based via the existing `.pr9k/` 0700-mode parent directory ([D-21](artifacts/implementation-decision-log.md#d-21-interaction-channel-socket-at-projectdirpr9kcmux-pane-workspacenamesock-with-0700-mode-parent)). Only the operator who owns the directory can connect. No client-identity verification beyond filesystem permissions is needed.
- **Socket creation:** the orchestrator binds the socket. Implementation uses `net.Listen("unix", path)` semantics; the socket file is unlinked at orchestrator exit. If a stale socket exists at startup (e.g., from a previous crash), the orchestrator unlinks it before binding — workspace-name nanosecond timestamps ([Phase 1 D-3](../phase-1-workspace-lifecycle/artifacts/implementation-decision-log.md#d-3-pane-spawn-order-is-orchestrator-first)) make filename collisions vanishingly unlikely. The implementation should follow `docs/coding-standards/file-writes.md` `O_CREATE|O_EXCL`-style semantics for the socket file (or equivalent for `net.Listen`).
- **No new external trust boundaries.** The interaction channel is local-only; no network egress. The new `PR9K_CMUX_SOCKET` env var is set by the orchestrator and consumed only by the four panes — not an externally controllable input. Cmux's `surface.spawn` carries the env to the spawned process via cmux's existing access model (parent T2 descendants-only).
- **Inherited mitigations from Phase 1:** `CMUX_SOCKET_PATH` validation ([Phase 1 D-15](../phase-1-workspace-lifecycle/artifacts/implementation-decision-log.md#d-15-cmux_socket_path-validation-before-netdial)) still applies to the cmux JSON-RPC socket; `ansi.StripForTerminalOutput` ([Phase 1 D-14](../phase-1-workspace-lifecycle/artifacts/implementation-decision-log.md#d-14-stripforterminaloutput-ansi-variant-for-the-cmux-diagnostic-path)) still applies to operator-visible diagnostics from cmux.

`adversarial-security-analyst` is not engaged for Phase 2. Re-engage in Phase 6 hardening or earlier if a new external trust boundary appears (e.g., Phase 5 completion notifications crossing process boundaries with operator-supplied content).

## Operational Readiness

- **Observability:** Phase 2 produces the same `<projectDir>/.pr9k/logs/<run-stamp>/` artifacts standard mode produces (per-run log file, per-step JSONL artifacts) via the orchestrator's reused `*logger.Logger` ([D-7](artifacts/implementation-decision-log.md#d-7-logger-creation-inside-the-orchestrator-pane-process)); equivalence acceptance criterion per [D-13](artifacts/implementation-decision-log.md#d-13-log-artifact-equivalence-comparison-is-content-modulo-run-specifics-not-byte-identity). Operator-visible signals in cmux mode now include the live per-pane render (header, log, footer) as well as the Phase 1 launching-terminal surfaces (workspace-name confirmation, preflight errors, orphan diagnostic). The status-line script's diagnostics are NOT persisted to disk in cmux mode ([D-9](artifacts/implementation-decision-log.md#d-9-statuslinerunner-constructed-with-nil-logger-in-cmux-footer)); operators see them in the footer pane's runtime render.
- **SLO impact:** none. Phase 2 is operator-driven, not service-driven. The 10s handshake deadline + 5s `DoneAck` deadline are upper bounds on user-perceived latency at workflow start and end; both are well within "responsive" thresholds.
- **Feature flag:** `--cmux` flag is the operator-facing opt-in per parent D25 and Phase 1's [D-2](../phase-1-workspace-lifecycle/artifacts/implementation-decision-log.md#d-2--cmux-flag-ships-visible-with-experimental-help-text). Default `false`. Phase 2 widens the flag's behavior from Phase 1's placeholder demo to a real-workflow demo, but the flag surface is unchanged. The help-text wording is updated in the docs (U11) to reflect the broadened behavior; the experimental marker remains until Phase 6 ships.
- **Rollout:** Phase 2 ships in version 0.11.0 ([D-17](artifacts/implementation-decision-log.md#trivial-decisions)). Release notes name Phase 2's scope (real workflow runs end-to-end; no sidebar mirroring yet; no completion notification; error-recovery prompts deferred). Rollout is "operators read the release notes and the updated how-to."
- **Rollback:** revert the Phase 2 PR. The `--cmux` flag reverts to Phase 1's four-pane placeholder demo; standard mode is unaffected. Time-to-rollback: one `git revert` + one `make build`. The `internal/interactionchannel` package and the `pr9k cmux-pane` sub-command disappear; Phase 1's `RunPhase1` reverts to the `sh -c '...'` placeholder argv.
- **Cost and scale:** Per-workspace: 4 processes (orchestrator + 3 panes) + 1 Unix socket + ~5–10 goroutines per process. Negligible CPU and memory at the operator-driven scale Phase 2 targets. The per-connection-goroutine count grows linearly if Phase 4 adds consumers; risk R1 covers the future-state concern.
- **Compliance controls:** none added.

## Definition of Done

- [ ] `pr9k --cmux --project-dir <repo>` from inside a cmux session against a real-workflow repo produces the build-outline Phase 2 7-step demo:
  1. Operator launches pr9k in cmux mode against a target repository with a real workflow configured.
  2. Cmux workspace appears (per Phase 1) and the four panes start rendering. After the readiness handshake completes, the workflow begins.
  3. Header pane shows checkboxes ticking over and the iteration counter advancing as steps complete.
  4. Log pane streams subprocess output. Operator focuses the log pane and scrolls back through prior output.
  5. Footer pane shows status-line output on its existing cadence, shortcut hints, and the version label. Operator presses `?` and the help modal expands inline above the footer.
  6. Operator presses `q` then `y`; the orchestrator aborts cleanly, the workspace remains open showing the final state on each pane, and the operator dismisses it to return to the prior workspace.
  7. Operator inspects `<projectDir>/.pr9k/logs/<run-stamp>/` and confirms the per-step JSONL artifacts match the equivalence criterion per [D-13](artifacts/implementation-decision-log.md#d-13-log-artifact-equivalence-comparison-is-content-modulo-run-specifics-not-byte-identity) against a standard-mode reference run.
- [ ] All test-plan checkmarks pass (`make test` with race detector; `make ci`).
- [ ] No lint suppressions (`docs/coding-standards/lint-and-tooling.md`); no `//nolint`, no `.golangci.yml` exclusions added.
- [ ] Race detector passes for `make test ./...` including the new `internal/interactionchannel` package and the orchestrator-side adapter goroutines.
- [ ] `internal/interactionchannel` package's exported types compile-time-assert against their interfaces per `docs/coding-standards/api-design.md`.
- [ ] Feature doc `docs/features/cmux-mode.md` is updated to describe Phase 2 behavior (real workflow runs, handshake, per-pane rendering, completion, help inline expand, minimum-size advisory).
- [ ] How-to `docs/how-to/setting-up-cmux.md` is updated with Phase 2 walkthrough (verify workflow runs end-to-end, navigate the panes, dismiss cleanly).
- [ ] Code-package doc `docs/code-packages/interactionchannel.md` is created and accurately describes the package contract.
- [ ] CLAUDE.md is updated with links to the new docs and the new code-package doc.
- [ ] `pr9k --version` prints `0.11.0` ([D-17](artifacts/implementation-decision-log.md#trivial-decisions)).
- [ ] `pr9k cmux-pane --role=...` sub-command parses each role value and the `PR9K_CMUX_SOCKET` env var.
- [ ] The pre-existing `FakeClient.HangNext`/`HangRelease` race is fixed ([D-19](artifacts/implementation-decision-log.md#trivial-decisions)) and verified by race detector.
- [ ] PM next-step recommendation set in the Summary section.
- [ ] Post-ship owner: River (until cmux mode reaches Phase 6 stability).

## Specialist Handoffs for Implementation

- **`behavioral-analyst`** — dispatch for code review of `runCmuxOrchestrator`, the KeyHandler adapter, the workspace-done protocol, and the four pane runners' state-handling code before merge. Needs: the implementation diff, plus the spec D14 + D16 + D17 + D20 commitments mapped to specific code lines.
- **`concurrency-analyst`** — dispatch for race-detector clearance pass before merge. Needs: `make test` output (race detector active), the per-connection-goroutine count brief (per pane: 1 read + 1 write on orchestrator side; 1 read + 1 write on pane side; fan-out + handshake mutex on orchestrator), and the new ack-wait timeout's goroutine lifecycle.
- **`test-engineer`** — dispatch to verify the test plan against the implementation before merge. Needs: every U#-verification entry mapped to a real test, plus confirmation that the three new doubles cover each scripted case and the inherited `FakeClient` race fix is exercised.
- **`junior-developer`** — dispatch for a fresh-eyes pass on the updated feature doc + how-to + new code-package doc before merge. Needs: the docs, plus the build-outline Phase 2 7-step demo to verify the docs are reproducible.
- **`software-architect`** — dispatch (advisor only) if Assumption A1 turns out to be wrong (renderers cannot be reused unchanged). Needs: the failing import + the renderer's required adaptation cost.
- **`devops-engineer`** — dispatch to verify Assumption A3 against cmux v0.64.6's `surface.spawn` schema before U3 lands. Needs: pinned cmux version + the spawn-API documentation; if env-vars are not supported at spawn time, file an OI for argv-based passing.

## Deferred (YAGNI)

The YAGNI gate at Step 7.5 demoted the following items to this section. Each can be reopened on the named trigger.

### YAGNI-1: Heartbeat indicator forwarding across the process boundary

- **Why deferred:** Evidence test failed. The heartbeat indicator (`⋯ thinking (Ns)`) is a header suffix on silent claude turns; no spec commitment requires it in cmux mode; behavioral B13 surfaced the gap and concurrency C7 confirmed the cross-process cost. The simpler version (drop it) satisfies every spec commitment — header still shows step state, iteration counter still advances; only the per-step silence suffix is absent. Decision recorded at [D-10](artifacts/implementation-decision-log.md#d-10-drop-heartbeat-indicator-in-cmux-phase-2).
- **Reopen when:** An operator reports that long silent claude turns in cmux mode look indistinguishable from a hung run, OR Phase 4 (sidebar mirroring) adds infrastructure that makes forwarding effectively free.
- **Source:** R1, `behavioral-analyst` B13, `concurrency-analyst` C7, `test-engineer` S2.

### YAGNI-2: JSON-schema validation on the interaction channel

- **Why deferred:** Same anti-pattern Phase 1 named (Phase 1 YAGNI-2): parent D18 accepts method-presence checking; standard `json.Unmarshal` zero-values handle unknown/missing fields. No incident, no spec commitment to schema validation. Same author owns both ends — no external contract to validate against.
- **Reopen when:** Cross-version skew between orchestrator and display panes becomes possible (rolling release where panes from version N pair with an orchestrator from version N+1), OR a documented incident shows a confusing message-shape error.
- **Source:** R1, implicit YAGNI candidate following Phase 1 precedent.

### YAGNI-3: Per-pane scrollback persistence to disk after workflow completion

- **Why deferred:** Evidence test failed. Parent D17 commits to log file artifacts under `.pr9k/logs/` which already contain every log line; per-pane scrollback persistence would duplicate that content with no additional information. No operator has named the duplication as a need.
- **Reopen when:** Operators report that the `.pr9k/logs/` content does not match what they saw in the log pane during a run (i.e., the equivalence criterion in [D-13](artifacts/implementation-decision-log.md#d-13-log-artifact-equivalence-comparison-is-content-modulo-run-specifics-not-byte-identity) fails for a real workflow).
- **Source:** R1, `junior-developer` OQ-6.

### YAGNI-4: Configurable handshake-timeout value

- **Why deferred:** Named anti-pattern — "speculative configuration knob." No operator has described a need; the 10s default (aligned with parent D27's cmux per-call timeout upper bound) satisfies the spec without an operator-facing tuning surface.
- **Reopen when:** An operator reports the default trips false-positives on slow startups, OR Phase 6 introduces a broader stall-threshold configuration surface that absorbs this knob.
- **Source:** R1, implicit YAGNI candidate (concurrency C1 surfaced the timeout requirement; configurability not proposed).

### YAGNI-5: Live cmux integration test in CI for Phase 2

- **Why deferred:** Already deferred by build-outline OQ-2 to Phase 6; test-engineer S1 confirmed for Phase 2. The cmux JSON-RPC test surface is `FakeClient`; the interaction-channel test surface is the three new doubles. Live-cmux verification is the operator's responsibility via the Phase 2 demo until Phase 6 lands CI integration.
- **Reopen when:** Phase 6 starts and CI integration-test infrastructure decision lands (OI-2).
- **Source:** R1, `test-engineer` S1 (inherited from Phase 1 YAGNI-4).

### YAGNI-6: Cross-pane drift measurement / SLO

- **Why deferred:** Named anti-pattern — "SLOs and error budgets for traffic the system doesn't yet receive." T4 commits to the budget; no operator has reported observable desync; no production traffic exists.
- **Reopen when:** An operator reports visible cross-pane desync, OR Phase 4 (sidebar mirroring) creates a meaningful third measurement point.
- **Source:** R1, `test-engineer` S3 (cross-pane drift bound verification).

### YAGNI-7: Status-line script diagnostic forwarding to orchestrator log

- **Why deferred:** Simpler-version test fired. The replacement (`statusline.Runner` constructed with `nil` logger) makes `r.logLine()` a no-op while preserving the runner's visible failure mode (script stderr displayed in the footer pane). The on-disk log loses runner diagnostics, but no operator has named that as a need. Decision recorded at [D-9](artifacts/implementation-decision-log.md#d-9-statuslinerunner-constructed-with-nil-logger-in-cmux-footer).
- **Reopen when:** An operator reports they need to see runner diagnostics in the persisted log file (e.g., a status-line script that fails only on long-running production runs).
- **Source:** R1, `behavioral-analyst` B10, `concurrency-analyst` C6.

### YAGNI-8: A new `cmuxpane` package per pane renderer

- **Why deferred:** Simpler-version test fired. The standard run loop already has `ui.StatusHeader`, the log-panel renderer in `internal/ui`, and the footer's keyboard state machine in `internal/ui/keys.go`. Phase 2 reuses these renderers; the per-pane sub-commands wrap them with the interaction-channel client. A bespoke `cmuxpane` package per pane would duplicate the existing renderers. Decision recorded at [D-20](artifacts/implementation-decision-log.md#d-20-per-pane-processes-via-pr9k-sub-commands-rather-than-separate-binaries).
- **Reopen when:** A specific pane's rendering needs diverge materially from its standard-TUI counterpart (e.g., the footer pane's help-modal layout grows beyond what the inline-expand pattern can handle and warrants its own component).
- **Source:** R1, `junior-developer` OQ-2 framing + `behavioral-analyst` B13.

## Open Items

- **OI-1: CI integration testing strategy for cmux mode (inherited from Phase 1 OI-2; carries forward unchanged).**
  - **Resolves when:** Phase 6 starts and the team commits to a testing approach per build-outline OQ-2 (mock through Phase 5; revisit at Phase 6).
  - **Blocks implementation:** No — Phase 2 ships with mocked-only coverage; live-cmux integration testing is deferred to Phase 6 per YAGNI-5.

- **OI-2: Spec amendment recommended for parent D17 wording sharpening.**
  - **Resolves when:** the parent feature spec's D17 ("log artifacts byte-for-byte same") is amended to match [D-13](artifacts/implementation-decision-log.md#d-13-log-artifact-equivalence-comparison-is-content-modulo-run-specifics-not-byte-identity)'s "equivalent content modulo run-specifics" wording.
  - **Blocks implementation:** No — implementation follows [D-13](artifacts/implementation-decision-log.md#d-13-log-artifact-equivalence-comparison-is-content-modulo-run-specifics-not-byte-identity) regardless. Recommended that River updates the parent spec in a follow-up commit so the spec and implementation align.

## Summary

- **Outcome delivered:** Real workflow runs end-to-end inside cmux mode, with per-pane rendering (header / log / footer) driven by the orchestrator pane over a Unix-socket interaction channel, readiness-handshake-gated first step, and standard-mode-equivalent log artifacts on disk.
- **Team size:** 5 specialists active (behavioral, concurrency, test-engineer, junior, project-manager), 11 stood down or not engaged this round — see [artifacts/implementation-iteration-history.md](artifacts/implementation-iteration-history.md).
- **Rounds of facilitation:** 2 — see [artifacts/implementation-iteration-history.md](artifacts/implementation-iteration-history.md).
- **Decisions committed:** 21 (16 full + 5 trivial) — see [artifacts/implementation-decision-log.md](artifacts/implementation-decision-log.md).
- **Decisions settled by evidence:** 14 — see [artifacts/implementation-decision-log.md](artifacts/implementation-decision-log.md).
- **Decisions settled by junior-developer reframing:** 3 (D-16 display-pane lifecycle reused unchanged, D-18 version-label preserved, D-20 sub-command shape) — see [artifacts/implementation-decision-log.md](artifacts/implementation-decision-log.md).
- **Decisions settled by user input:** 4 (D-11 minimum-size thresholds, D-12 help inline expand, D-13 equivalence wording, D-17 version-bump scope) — see [artifacts/implementation-decision-log.md](artifacts/implementation-decision-log.md).
- **Rejected alternatives recorded:** 50+ — see [artifacts/implementation-decision-log.md](artifacts/implementation-decision-log.md).
- **Open items remaining:** 2 (OI-1 inherited from Phase 1; OI-2 spec-amendment recommendation).
- **Recommendation:** **Ship as planned.** Spec-maturity gate did not trip; PM was not engaged for a gate-trip facilitation pass. All work units are unblocked. Implementation can begin immediately on U1 + U10 (independent dependencies); U2–U9 ship as the package skeleton lands; U11 documentation runs in parallel with implementation work; U12 version-bump lands with U11 in the same PR.

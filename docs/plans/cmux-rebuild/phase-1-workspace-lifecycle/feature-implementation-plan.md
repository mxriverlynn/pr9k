# Feature Implementation Plan: Phase 1 — Cmux Mode Launch and Workspace Lifecycle

Phase 1 of the pr9k cmux-rebuild adds an opt-in `--cmux` launch mode that stands up a four-pane cmux workspace with placeholder content, holds it open until the operator dismisses it, and tears it down with the prior cmux workspace restored. The implementation posture is **single-PR ship**: code, tests, feature docs, setup how-to, and version bump (0.10.0 → 0.11.0) all land together ([D-24](artifacts/implementation-decision-log.md#d-24-phase-1-pr-scope--code--docs--version-bump-in-single-pr)) so neither the versioning standard nor the documentation standard is violated. No live-cmux CI test is required for Phase 1; the test surface is a `FakeClient` covering every behavioral commitment per the test plan.

## Source Specification

- **Feature specification:** [feature-specification.md](feature-specification.md)
- **Specification decision log:** [artifacts/decision-log.md](artifacts/decision-log.md)
- **Specification team findings:** [artifacts/team-findings.md](artifacts/team-findings.md)
- **Inherited parent technical notes:** [../artifacts/feature-technical-notes.md](../artifacts/feature-technical-notes.md) (T1–T5; Phase 1 has no phase-specific technical notes file)
- **Specification decisions this plan inherits:** D1, D2, D3, D4, D5, D6, D7, D8, D9, D10, D11, D12 (Phase 1 spec decisions); plus parent D1, D2, D3, D4, D13, D18, D22, D25, D29 (inherited unchanged per spec Summary).
- **Specification open items this plan must respect or resolve:** OI-1 (named minimum-supported cmux version), OI-2 (CI testing strategy for cmux mode), OI-3 (cmux setup how-to must document workspace-dismissal gestures).

## Outcome

When this plan executes, the codebase contains:

- A `--cmux` flag on `pr9k`'s existing run command, visible with experimental help text ([D-2](artifacts/implementation-decision-log.md#d-2--cmux-flag-ships-visible-with-experimental-help-text)).
- A new `internal/cmuxctl` package providing the cmux JSON-RPC 2.0 client, the cmux preflight, and the Phase 1 workspace lifecycle entry point ([D-18](artifacts/implementation-decision-log.md#d-18-internalcmuxctl-package-layout--interface-realclient-fakeclient-preflight-runphase1)).
- A new `internal/ansi.StripForTerminalOutput` function hardened against terminal-injection vectors that the existing `StripAll` deliberately permits ([D-14](artifacts/implementation-decision-log.md#d-14-stripforterminaloutput-ansi-variant-for-the-cmux-diagnostic-path)).
- `CMUX_SOCKET_PATH` env-var validation defending against socket-redirection attacks ([D-15](artifacts/implementation-decision-log.md#d-15-cmux_socket_path-validation-before-netdial)).
- SIGHUP registered alongside SIGINT and SIGTERM in the existing signal handler ([D-10](artifacts/implementation-decision-log.md#d-10-sighup-registered-alongside-sigint-and-sigterm)), under a two-goroutine watchdog+cleanup pattern that delivers spec D6's second-signal-immediate-exit guarantee ([D-9](artifacts/implementation-decision-log.md#d-9-two-goroutine-watchdogcleanup-signal-handling-pattern)).
- A new feature doc, a new how-to, a new code-package doc, CLAUDE.md updates, and a version bump from 0.10.0 to 0.11.0 in a single commit-grouping ([D-24](artifacts/implementation-decision-log.md#d-24-phase-1-pr-scope--code--docs--version-bump-in-single-pr), [D-26](artifacts/implementation-decision-log.md#trivial-decisions)).

When operators run `pr9k --cmux --project-dir <repo>` from inside a cmux session, they observe the build-outline Phase 1 demo: a freshly-named workspace appears with four panes (orchestrator hidden, header / log / footer visible with placeholder labels), persists across focus changes, and tears down cleanly with focus restored to the prior workspace.

## Context

- **Driving constraint:** Phase 1 is the foundation phase of the cmux-rebuild build outline; every later phase puts content inside this workspace. Nothing demoable can land until pr9k can stand up and dismantle a cmux workspace cleanly.
- **Stakeholders:**
  - **Operators on macOS (cmux's primary platform)** — care that the demo works first time, the workspace name is recognizable, and dismissal is one gesture.
  - **The pr9k maintainers (River + collaborators)** — care that the package layout supports the next six phases without rework, and that the test seam (`FakeClient`) lets phases 2–6 be developed without a live cmux dependency.
  - **The broader cmux user community** — care that pr9k respects cmux's access model (parent T2), does not pollute their workspace list with orphans, and is honest in its preflight diagnostics.
- **Future-state concern:** the dismissal-observation goroutine plus the single-flight RPC queue ([D-5](artifacts/implementation-decision-log.md#d-5-cmux-client-uses-a-single-goroutine-sequential-rpc-queue), [D-6](artifacts/implementation-decision-log.md#d-6-dismissal-observation-via-500ms-polling-with-single-flight)) are the foundation for Phase 4's sidebar mirroring (concurrent RPCs) and Phase 6's hardening (failure-mode generalization). The single-goroutine queue is intentionally simpler than what Phase 4 will need; the package shape (interface + RealClient + FakeClient) lets Phase 4 swap in a multiplexed implementation without breaking Phase 1's contract.
- **Out-of-scope boundary:** real workflow execution (Phase 2), streaming log output (Phase 2), header step-checkbox rendering (Phase 2), footer keyboard state machine (Phase 2), cmux sidebar entries (Phase 4), cmux notifications (Phase 5), generalized failure handling for orchestrator-pane loss / channel stalls / hung cmux during a running workflow (Phase 6), orphan startup advisory (Phase 7), `pr9k workflow` (the workflow builder; cmux mode does not apply per parent D9).

## Team Composition and Participation

| Specialist | Status | Key Input |
|------------|--------|-----------|
| `project-manager` | Coordinator | Facilitated R1 + R2, applied YAGNI at Step 7.5, synthesized this plan. |
| `devops-engineer` | Active | Twelve findings (DOR-001 through DOR-012) covering CLI flag, signal registration, logger branching, Bubble Tea conditional, preflight integration shape, polling cadence, sanitization set, version-bump and docs scope, and OI-1 release-readiness coupling. |
| `behavioral-analyst` | Active | Eleven findings (B1 through B11) covering self-close double-fire, no-orphan claim audit, D15 timeout scope on poll calls, exit-code-distinction infeasibility, preflight ordering, signal-handler watchdog requirement, pane spawn order. |
| `concurrency-analyst` | Active | Eight findings (C1 through C8) covering single-flight RPC queue, goroutine cancellation discipline, per-call timeout protocol-level behavior, two-goroutine signal pattern, dismissal-channel buffering, mutex protection. |
| `test-engineer` | Active | Fourteen-row test plan (T1 through T14) covering preflight, sanitization, lifecycle, dismissal observation, signal handling, ANSI sanitization, env-var validation; three deferrals (S1, S2, S3) recorded as YAGNI items. |
| `junior-developer` | Active | Ten clarifying questions (Q1 through Q10) reframing assumptions about descendant probes, capability-shape skew, Hidden flag, validator-runs-without-steps, placeholder process choice, security review, orphan operator visibility. |
| `adversarial-security-analyst` | Active (R2) | Five findings (SEC-001 through SEC-005): C0 cursor-byte injection, double-ESC bypass (cosmetic), `CMUX_SOCKET_PATH` SSRF / T2-bypass, C1 OSC title-set phishing, SIGHUP-orphan confirmation. |
| `software-architect` | Stood down | Package-shape recommendation absorbed into [D-18](artifacts/implementation-decision-log.md#d-18-internalcmuxctl-package-layout--interface-realclient-fakeclient-preflight-runphase1) without re-engagement; no separate findings document. |
| `system-architect` | Not needed on this plan | Phase 1 is a single-process boundary (launching pr9k + ephemeral placeholder shell processes inside cmux panes); no cross-service / bounded-context topology to evaluate. |
| `risk-analyst` | Not needed on this plan | RAID risks are low-count and individually scoped to specific findings (signal handling, env-var trust); no portfolio-level risk prioritization needed. |
| `structural-analyst` | Not needed on this plan | Phase 1 introduces one new package (`internal/cmuxctl`) and modifies one existing function entry point (`main()`); no SOLID violations or coupling concerns surfaced by other specialists. |
| `edge-case-explorer` | Not needed on this plan | Spec phase already engaged this specialist (F1 through F16 in [team-findings.md](artifacts/team-findings.md)); R1 specialists' edge-case coverage was sufficient at implementation tier. |
| `gap-analyzer` | Not needed on this plan | Spec is mature; only one implementation-level spec contradiction surfaced ([D-12](artifacts/implementation-decision-log.md#d-12-drop-the-exit-code-distinction-on-dismissal--every-dismissal-observation-produces-exit-zero)) and is recorded as user-resolved. |
| `adversarial-validator` | Not needed on this plan | Engaged at R2 via `adversarial-security-analyst` for the security frame; further adversarial validation is reserved for Phase 6 hardening and final pre-ship review. |

## Implementation Approach

### Architecture and Integration Points

**Entry-point branch.** `main()` is modified to branch on `cfg.Cmux` immediately after `startup()` returns successfully ([D-16](artifacts/implementation-decision-log.md#d-16-skip-standard-logger-and-bubble-tea-wiring-on-cmux-phase-1-path)). On the cmux path, `main()`: skips `logger.NewLogger(projectDir)`; skips the status-line runner; skips Bubble Tea program creation and `program.Run()`; invokes `cmuxctl.Preflight(ctx, client)` ([D-1](artifacts/implementation-decision-log.md#d-1-cmux-preflight-integration-shape)); on preflight success, invokes `cmuxctl.RunPhase1(ctx, client, projectDir)`. The standard preflight's `.pr9k/` umbrella mkdir is preserved (consistent with non-cmux launches). On the non-cmux path, `main()` behaves identically to today.

**New package: `internal/cmuxctl`.** Single package per [D-18](artifacts/implementation-decision-log.md#d-18-internalcmuxctl-package-layout--interface-realclient-fakeclient-preflight-runphase1), exporting:

- `CmuxClient` interface enumerating Phase 1's RPC surface: `SystemIdentify`, `WorkspaceCurrent`, `WorkspaceList`, `WorkspaceCreate`, `WorkspaceClose`, `WorkspaceSelect`, `SurfaceSplit`, `SurfaceSpawn`, `SurfaceHide`, `SurfaceList`. Compile-time interface assertion (`var _ CmuxClient = (*RealClient)(nil)`) per `docs/coding-standards/api-design.md`.
- `RealClient` over a Unix-socket JSON-RPC 2.0 connection. Single-goroutine sequential RPC queue ([D-5](artifacts/implementation-decision-log.md#d-5-cmux-client-uses-a-single-goroutine-sequential-rpc-queue)); per-call timeout closes the socket on fire ([D-21](artifacts/implementation-decision-log.md#d-21-per-call-timeout-protocol--close-socket-on-timeout-re-open-for-teardown)) and the teardown path re-opens for one final `WorkspaceClose` call.
- `FakeClient` for tests — scriptable per-method behavior, sync.Mutex-protected per `docs/coding-standards/testing.md`.
- `Preflight(ctx, client) []error` — runs the five distinguishable failure-condition checks (cmux not installed, cmux not running, not a cmux descendant via socket-open per [D-20](artifacts/implementation-decision-log.md#d-20-descendant-probe-via-socket-open-with-allow-all-mode-caveat-documented), socket disabled, capability mismatch via `system.identify` per parent D18) and the `CMUX_SOCKET_PATH` env-var validation per [D-15](artifacts/implementation-decision-log.md#d-15-cmux_socket_path-validation-before-netdial). Returns severity-ordered errors per `docs/coding-standards/error-handling.md` package-prefixed conventions.
- `RunPhase1(ctx, client, projectDir) error` — runs the workspace lifecycle: capture current workspace, compose+sanitize+collision-retry the workspace name ([D-23](artifacts/implementation-decision-log.md#d-23-pre-sanitized-basename-never-appears-in-operator-visible-terminal-output-code-review-enforcement)), `WorkspaceCreate`, spawn the four panes orchestrator-first ([D-3](artifacts/implementation-decision-log.md#d-3-pane-spawn-order-is-orchestrator-first)) using shell-one-liner placeholders ([D-4](artifacts/implementation-decision-log.md#d-4-placeholder-process-is-a-shell-one-liner)), print the workspace-name confirmation, run the dismissal-observation poller ([D-6](artifacts/implementation-decision-log.md#d-6-dismissal-observation-via-500ms-polling-with-single-flight), [D-7](artifacts/implementation-decision-log.md#d-7-d9-poll-timeout-escalation-tolerates-n3-consecutive-fires-before-fatal-teardown), [D-19](artifacts/implementation-decision-log.md#d-19-per-pane-exit-observation-via-surfacelist-or-equivalent)) until dismissal, run best-effort teardown ([D-11](artifacts/implementation-decision-log.md#d-11-best-effort-teardown-with-operator-visible-diagnostic-on-workspaceclose-failure)) with focus restore.

**Existing-package modifications:**

- `internal/cli/args.go`: add `Cmux bool` field to `Config`; register `--cmux` cobra `BoolVar` ([D-25](artifacts/implementation-decision-log.md#trivial-decisions)).
- `internal/ansi`: add `StripForTerminalOutput` per [D-14](artifacts/implementation-decision-log.md#d-14-stripforterminaloutput-ansi-variant-for-the-cmux-diagnostic-path); preserve existing `StripAll` contract ([D-27](artifacts/implementation-decision-log.md#trivial-decisions)).
- `src/cmd/pr9k/main.go`: branch on `cfg.Cmux` per [D-16](artifacts/implementation-decision-log.md#d-16-skip-standard-logger-and-bubble-tea-wiring-on-cmux-phase-1-path); register SIGHUP per [D-10](artifacts/implementation-decision-log.md#d-10-sighup-registered-alongside-sigint-and-sigterm); restructure signal handler into two-goroutine pattern per [D-9](artifacts/implementation-decision-log.md#d-9-two-goroutine-watchdogcleanup-signal-handling-pattern).
- `src/internal/version/version.go`: bump to `0.11.0` per [D-26](artifacts/implementation-decision-log.md#trivial-decisions).

### Data Model and Persistence

Phase 1 introduces no on-disk state beyond what the standard preflight already creates (`.pr9k/` umbrella directory). No log file is created on the cmux Phase 1 path per [D-16](artifacts/implementation-decision-log.md#d-16-skip-standard-logger-and-bubble-tea-wiring-on-cmux-phase-1-path); no per-step JSONL artifacts are produced per spec Coordinations table. No schema changes, no migrations, no data movement. The cmux workspace itself lives in cmux's process memory and is not persisted by pr9k.

### Runtime Behavior

**Launch sequence.**

1. `pr9k --cmux --project-dir <repo>` is invoked from a shell inside a cmux session.
2. Cobra parses flags into `cli.Config{Cmux: true, ...}`.
3. `startup()` runs unchanged: `steps.LoadSteps`, `validator.Validate`, `preflight.Run`. `.pr9k/` mkdir succeeds. Fatal errors abort here as today.
4. `main()` branches on `cfg.Cmux`. On cmux path: skip logger / status-line / Bubble Tea wiring ([D-16](artifacts/implementation-decision-log.md#d-16-skip-standard-logger-and-bubble-tea-wiring-on-cmux-phase-1-path)).
5. `cmuxctl.Preflight(ctx, client)` runs the five distinguishable checks. On any fatal failure, package-prefixed error printed to launching terminal, exit non-zero. Cmux-supplied diagnostic text is filtered through `ansi.StripForTerminalOutput` per [D-14](artifacts/implementation-decision-log.md#d-14-stripforterminaloutput-ansi-variant-for-the-cmux-diagnostic-path) before printing.
6. `cmuxctl.RunPhase1(...)`: capture current workspace via `WorkspaceCurrent` (record "no prior workspace" if appropriate per spec D10).
7. Compose workspace name: sanitize basename per spec D11 + [D-17](artifacts/implementation-decision-log.md#d-17-d11-sanitization-character-set-keeps--for-now-phase-1-release-readiness-check-tied-to-oi-1), append nanosecond UTC timestamp per parent D29.
8. `WorkspaceCreate` with composed name. On duplicate-name rejection, regenerate timestamp once and retry per spec D12; on second failure, route to partial-setup teardown.
9. Print workspace-name confirmation to launching terminal per spec D2.
10. Spawn the four panes orchestrator-first ([D-3](artifacts/implementation-decision-log.md#d-3-pane-spawn-order-is-orchestrator-first)): `SurfaceSpawn` orchestrator with `sh -c 'tail -f /dev/null'`, then `SurfaceHide` orchestrator; then split + spawn header / log / footer with `sh -c 'printf "<role> — Phase 1 placeholder\n" && tail -f /dev/null'` per [D-4](artifacts/implementation-decision-log.md#d-4-placeholder-process-is-a-shell-one-liner). Any cmux RPC failure during spawn routes to partial-setup teardown.
11. Start the dismissal-observation goroutine with `context.Context` + `sync.WaitGroup` discipline per [D-22](artifacts/implementation-decision-log.md#d-22-goroutine-cancellation-discipline--contextcontext-waitgroup-buffered-channel). Poll cycle: every 500ms ([D-6](artifacts/implementation-decision-log.md#d-6-dismissal-observation-via-500ms-polling-with-single-flight)), single-flight, issue `WorkspaceList` and `SurfaceList` sequentially through the RPC queue. Workspace gone OR any pane shows exited → fire dismissal channel. Three consecutive poll timeouts → fire dismissal channel with fatal-teardown sentinel ([D-7](artifacts/implementation-decision-log.md#d-7-d9-poll-timeout-escalation-tolerates-n3-consecutive-fires-before-fatal-teardown)).
12. Block on the dismissal channel.

**Dismissal sequence.**

1. Dismissal channel fires. Set `shuttingDown` flag (mutex-protected) per [D-8](artifacts/implementation-decision-log.md#d-8-self-close-double-fire-mitigation-via-shuttingdown-flag-and-channel-priming) and cancel the observer's context.
2. `sync.Once`-guarded teardown: attempt `WorkspaceClose` once. On failure, print operator-visible diagnostic per [D-11](artifacts/implementation-decision-log.md#d-11-best-effort-teardown-with-operator-visible-diagnostic-on-workspaceclose-failure) (`pr9k: orphan workspace "<name>" could not be closed; dismiss it manually via cmux's controls`).
3. If a prior workspace was captured, attempt `WorkspaceSelect` on it; ignore failures silently per spec D10.
4. Wait on `WaitGroup` for the dismissal-observer goroutine to exit cleanly.
5. Exit code: zero on every dismissal observation per [D-12](artifacts/implementation-decision-log.md#d-12-drop-the-exit-code-distinction-on-dismissal--every-dismissal-observation-produces-exit-zero); non-zero on partial-setup teardown, signal-driven shutdown, observation-poll-timeout escalation, or `WorkspaceClose` failure during shutdown.

> **Spec contradiction recorded.** [D-12](artifacts/implementation-decision-log.md#d-12-drop-the-exit-code-distinction-on-dismissal--every-dismissal-observation-produces-exit-zero) contradicts spec User Interactions Dismissal-gestures bullet ("zero for workspace-close path; non-zero for close-pane path"). User explicitly resolved during R2 to drop the distinction because deterministic event-ordering between `WorkspaceList` and `SurfaceList` observations is not technically achievable. Spec amendment recommended; tracked as Open Item OI-A below.

**Signal handling.**

1. `signal.Notify` registers SIGINT, SIGTERM, **and SIGHUP** ([D-10](artifacts/implementation-decision-log.md#d-10-sighup-registered-alongside-sigint-and-sigterm)) on a buffered channel.
2. Two-goroutine pattern per [D-9](artifacts/implementation-decision-log.md#d-9-two-goroutine-watchdogcleanup-signal-handling-pattern): cleanup goroutine receives the first signal, sets `shuttingDown`, runs the teardown sequence (which may block on cmux calls). Watchdog goroutine listens on the same channel; on its first receive (the operator's second signal), calls `os.Exit(1)` immediately regardless of cleanup state.
3. Cleanup goroutine routes through the same teardown as the dismissal channel (`sync.Once` ensures single execution).

### External Interfaces

**CLI.** New `--cmux` flag on `pr9k`'s root run command. Help text: `experimental: launches a four-pane placeholder workspace; full workflow rendering ships in a later release` ([D-2](artifacts/implementation-decision-log.md#d-2--cmux-flag-ships-visible-with-experimental-help-text)). Visible (not `Hidden`).

**Cmux JSON-RPC 2.0.** Outbound calls only, sequenced through the single-goroutine queue. Method-presence capability check accepts API skew at the method level; schema-shape validation is not added per [D-13](artifacts/implementation-decision-log.md#d-13-method-presence-capability-check-accepted-schema-shape-validation-deferred). The contract shape (CmuxClient interface) is internal to pr9k and not exposed; cmux's own API is the external contract this implementation calls.

**Environment variables.** `CMUX_SOCKET_PATH` (existing convention) is now validated per [D-15](artifacts/implementation-decision-log.md#d-15-cmux_socket_path-validation-before-netdial). No new pr9k-specific env vars.

**Operator-visible terminal output.** Workspace-name confirmation line per spec D2 + [D-23](artifacts/implementation-decision-log.md#d-23-pre-sanitized-basename-never-appears-in-operator-visible-terminal-output-code-review-enforcement); preflight failure messages naming the specific failure condition; partial-setup-teardown diagnostic; orphan-workspace diagnostic on `WorkspaceClose` failure ([D-11](artifacts/implementation-decision-log.md#d-11-best-effort-teardown-with-operator-visible-diagnostic-on-workspaceclose-failure)). All cmux-supplied diagnostic text passes through `ansi.StripForTerminalOutput` per [D-14](artifacts/implementation-decision-log.md#d-14-stripforterminaloutput-ansi-variant-for-the-cmux-diagnostic-path).

## Decomposition and Sequencing

| # | Work Unit | Delivers | Depends On | Verification |
|---|-----------|----------|------------|--------------|
| U1 | `--cmux` CLI flag + `Cmux bool` Config field ([D-25](artifacts/implementation-decision-log.md#trivial-decisions), [D-2](artifacts/implementation-decision-log.md#d-2--cmux-flag-ships-visible-with-experimental-help-text)) | `pr9k --cmux --help` shows the flag with experimental help text; `cli.Config` carries the value | — | Unit tests in `internal/cli`: flag parses, `--help` output contains the experimental wording |
| U2 | `internal/cmuxctl` package skeleton: `CmuxClient` interface, `RealClient` stub (single-goroutine queue), `FakeClient` ([D-18](artifacts/implementation-decision-log.md#d-18-internalcmuxctl-package-layout--interface-realclient-fakeclient-preflight-runphase1), [D-5](artifacts/implementation-decision-log.md#d-5-cmux-client-uses-a-single-goroutine-sequential-rpc-queue), [D-21](artifacts/implementation-decision-log.md#d-21-per-call-timeout-protocol--close-socket-on-timeout-re-open-for-teardown)) | The cmux client interface and test double exist; no behavior wired yet | U1 | Unit tests: `FakeClient` returns scripted responses; `RealClient` per-call timeout fires on stalled mock socket; compile-time interface assertion lints clean |
| U3 | `cmuxctl.Preflight(ctx, client)` — five distinguishable failure messages + `CMUX_SOCKET_PATH` validation + `system.identify` capability check ([D-1](artifacts/implementation-decision-log.md#d-1-cmux-preflight-integration-shape), [D-15](artifacts/implementation-decision-log.md#d-15-cmux_socket_path-validation-before-netdial), [D-20](artifacts/implementation-decision-log.md#d-20-descendant-probe-via-socket-open-with-allow-all-mode-caveat-documented), [D-13](artifacts/implementation-decision-log.md#d-13-method-presence-capability-check-accepted-schema-shape-validation-deferred)) | Preflight produces operator-actionable errors for each of the five conditions; `CMUX_SOCKET_PATH` redirection attacks are rejected | U2 | Unit tests via `FakeClient`: each of the five failure modes produces the named error with the correct package prefix; env-var validation rejects symlinked paths to non-socket files, world-writable parent directories, and non-cmux `system.identify` responses |
| U4 | `cmuxctl.RunPhase1(...)` — current capture, name composition + sanitization + collision retry, `WorkspaceCreate`, four-pane spawn orchestrator-first, `SurfaceHide` orchestrator, workspace-name confirmation print ([D-3](artifacts/implementation-decision-log.md#d-3-pane-spawn-order-is-orchestrator-first), [D-4](artifacts/implementation-decision-log.md#d-4-placeholder-process-is-a-shell-one-liner), [D-23](artifacts/implementation-decision-log.md#d-23-pre-sanitized-basename-never-appears-in-operator-visible-terminal-output-code-review-enforcement), [D-16](artifacts/implementation-decision-log.md#d-16-skip-standard-logger-and-bubble-tea-wiring-on-cmux-phase-1-path)) | Workspace stands up with four panes per spec; `main()` branches on `cfg.Cmux` correctly | U2, U3 | Unit tests via `FakeClient`: name composition handles each spec D11 sanitization case; collision-retry succeeds once and fails on second collision; spawn order is orchestrator → header → log → footer; partial-setup teardown invoked on any spawn failure |
| U5 | Dismissal observation — polling goroutine with single-flight + per-call timeout + N=3 consecutive-timeout escalation, context-cancellation discipline, channel priming + `shuttingDown` flag ([D-6](artifacts/implementation-decision-log.md#d-6-dismissal-observation-via-500ms-polling-with-single-flight), [D-7](artifacts/implementation-decision-log.md#d-7-d9-poll-timeout-escalation-tolerates-n3-consecutive-fires-before-fatal-teardown), [D-8](artifacts/implementation-decision-log.md#d-8-self-close-double-fire-mitigation-via-shuttingdown-flag-and-channel-priming), [D-19](artifacts/implementation-decision-log.md#d-19-per-pane-exit-observation-via-surfacelist-or-equivalent), [D-22](artifacts/implementation-decision-log.md#d-22-goroutine-cancellation-discipline--contextcontext-waitgroup-buffered-channel)) | Pr9k blocks until either dismissal observation fires; transient timeouts are absorbed; self-close does not double-fire | U4 | Unit tests via `FakeClient` with scripted poll responses: workspace-removed observation fires dismissal exactly once; per-pane exit observation fires dismissal exactly once; three consecutive timeouts escalate to fatal sentinel; `shuttingDown` flag suppresses post-shutdown observations; goroutine exits cleanly under context cancellation (race detector passes) |
| U6 | Signal handling — SIGHUP registration, two-goroutine watchdog+cleanup pattern, second-signal `os.Exit` ([D-9](artifacts/implementation-decision-log.md#d-9-two-goroutine-watchdogcleanup-signal-handling-pattern), [D-10](artifacts/implementation-decision-log.md#d-10-sighup-registered-alongside-sigint-and-sigterm)) | First signal triggers graceful teardown; second signal exits immediately even if cleanup is blocked in a cmux RPC | U5 | Unit tests for the two-goroutine pattern using injected signal channel: cleanup goroutine runs teardown; watchdog `os.Exit` callback fires on second signal regardless of cleanup state. SIGHUP signal-channel registration verified by inspecting the `signal.Notify` call site (test omitted per S1 deferral; integration verification deferred to Phase 6) |
| U7 | Teardown — `WorkspaceClose` (best-effort, swallow failure with operator diagnostic), focus restore (silent on stale prior) ([D-11](artifacts/implementation-decision-log.md#d-11-best-effort-teardown-with-operator-visible-diagnostic-on-workspaceclose-failure), [D-12](artifacts/implementation-decision-log.md#d-12-drop-the-exit-code-distinction-on-dismissal--every-dismissal-observation-produces-exit-zero)) | Dismissal produces exit zero; failures produce non-zero with named operator diagnostic | U4, U5 | Unit tests via `FakeClient`: successful `WorkspaceClose` produces exit zero; failed `WorkspaceClose` produces exit non-zero AND prints the operator-visible orphan diagnostic AND still attempts focus-restore; missing prior workspace at restore time is silent |
| U8 | Terminal output safety — `ansi.StripForTerminalOutput` function, `CMUX_SOCKET_PATH` validation ([D-14](artifacts/implementation-decision-log.md#d-14-stripforterminaloutput-ansi-variant-for-the-cmux-diagnostic-path), [D-15](artifacts/implementation-decision-log.md#d-15-cmux_socket_path-validation-before-netdial), [D-27](artifacts/implementation-decision-log.md#trivial-decisions)) | Cmux-supplied diagnostic text cannot inject CR/BS/DEL overstrike or C1 OSC title-set sequences into the launching terminal; `CMUX_SOCKET_PATH` symlink/world-writable redirection rejected | — (parallel with U3) | Unit tests in `internal/ansi`: every C0 cursor-movement byte stripped from input; C1 controls in `0x80-0x9F` stripped including OSC payload consumption; LF + HT preserved; existing `StripAll` tests unchanged. Env-var validation tests cover empty, non-existent, non-socket, symlink-to-non-socket, world-writable parent |
| U9 | Documentation — `docs/features/cmux-mode.md`, `docs/how-to/setting-up-cmux.md` (with OI-1 + OI-3 resolutions baked in), `docs/code-packages/cmuxctl.md`, CLAUDE.md updates linking the new docs ([D-24](artifacts/implementation-decision-log.md#d-24-phase-1-pr-scope--code--docs--version-bump-in-single-pr)) | Operators can install cmux + the pinned version, run the demo, and recognize orphan workspaces by their `pr9k-` prefix. Maintainers can navigate the new package | U2, U3, U4 (docs reflect actual behavior) | `make build` passes; doc code blocks match the production code; CLAUDE.md links resolve |
| U10 | Version bump 0.10.0 → 0.11.0 in `src/internal/version/version.go` ([D-26](artifacts/implementation-decision-log.md#trivial-decisions)) — combined with U9 docs commit per `coding-standards/versioning.md` | `pr9k --version` prints `0.11.0`; release-readiness signal | U1 through U9 (all surface changes complete) | `make build` passes; `pr9k --version` output matches |

## RAID Log

### Risks

| ID | Risk | Likelihood | Severity | Blast Radius | Reversibility | Owner | Mitigation |
|----|------|------------|----------|--------------|---------------|-------|------------|
| R1 | Cmux's `surface.list` (or equivalent) does not expose per-pane exit status in the pinned cmux version, degrading spec D9 placeholder-exit observation to workspace-list-only ([D-19](artifacts/implementation-decision-log.md#d-19-per-pane-exit-observation-via-surfacelist-or-equivalent)) | Low (parent T5 confirms cmux tracks per-pane exit visibly; an introspection API is the natural query) | Medium (close-each-pane gesture cannot be observed; pr9k blocks until workspace itself closed) | Limited to cmux mode | Reversible (degraded behavior documented; later phase can add a different observation path) | `devops-engineer` | Implementation must verify capability against pinned cmux version (OI-1 release-readiness check); document degradation in setup how-to if absent |
| R2 | SIGHUP graceful-shutdown change ([D-10](artifacts/implementation-decision-log.md#d-10-sighup-registered-alongside-sigint-and-sigterm)) breaks an existing standard-mode test that relied on default SIGHUP disposition (immediate kill) | Low (no test in the discovery notes was named as relying on this) | Low (test fix only) | Standard run loop | Reversible | `devops-engineer` | Run full `make test` before merge; if a test fails, evaluate whether to make SIGHUP registration cmux-mode-only |
| R3 | The `internal/cmuxctl` interface shape proves too coarse when Phase 4 sidebar mirroring requires concurrent RPCs; refactor required at Phase 4 ([D-5](artifacts/implementation-decision-log.md#d-5-cmux-client-uses-a-single-goroutine-sequential-rpc-queue)) | Medium (the YAGNI deferral of multiplex is conscious; Phase 4 will need it) | Low (interface is the seam; multiplex is an implementation swap behind the same interface) | `internal/cmuxctl` only | Reversible | `software-architect` (Phase 4) | The single-flight queue is the simplest correct implementation today; Phase 4 swaps `RealClient`'s internal queue without changing the `CmuxClient` interface |
| R4 | A future cmux release breaks a method's response shape (not its presence), producing a confusing JSON unmarshal error or a silent zero-value field in production ([D-13](artifacts/implementation-decision-log.md#d-13-method-presence-capability-check-accepted-schema-shape-validation-deferred)) | Low (cmux's API has been stable; no incidents recorded) | Medium (operator confusion until the cmux release is recognized) | Cmux mode users | Reversible (add schema-shape validation in a follow-up release) | `devops-engineer` | Defensive `json.Unmarshal` checks for required field presence; YAGNI rule reopens the schema-validation question if this fires |
| R5 | A cmux release with subtly different message semantics (e.g., `WorkspaceList` returns empty during a brief startup window) produces false-positive dismissal observations | Low (cmux is stable) | Medium (false-positive teardown of healthy workspace) | Cmux mode users | Reversible (tune polling logic) | `behavioral-analyst` | The workspace-name match (not just count) is the dismissal observation; a brief empty list does not cause a false-positive because the specific workspace name must disappear |

### Assumptions

| ID | Assumption | What Changes If Wrong | Verifier | Status |
|----|------------|-----------------------|----------|--------|
| A1 | Cmux exposes per-pane exit status via `surface.list` or an equivalent introspection method | [D-19](artifacts/implementation-decision-log.md#d-19-per-pane-exit-observation-via-surfacelist-or-equivalent) degradation path activates: D9 placeholder-exit arm collapses to workspace-list-only | OI-1 release-readiness check against pinned cmux version | Unverified — blocks Phase 1 release-readiness, not implementation start |
| A2 | Cmux's `system.identify` response contains a recognizable identity string (e.g., `"name": "cmux"` or `"product": "cmux"`) | [D-15](artifacts/implementation-decision-log.md#d-15-cmux_socket_path-validation-before-netdial) cannot reject non-cmux fake-socket responses; SEC-003 mitigation weakens | OI-1 release-readiness check against pinned cmux version | Unverified — blocks setup how-to |
| A3 | The shell one-liner `sh -c 'printf ... && tail -f /dev/null'` works inside cmux-spawned panes on macOS BSD `sh` and Linux `sh` (dash, bash) | [D-4](artifacts/implementation-decision-log.md#d-4-placeholder-process-is-a-shell-one-liner) reverts to a `pr9k display` subcommand | Manual demo on macOS during Phase 1 implementation; second demo on Linux if a Linux runner is available | Unverified — verifiable during U4 implementation |
| A4 | `os.Exit(1)` from the watchdog goroutine cleanly terminates pr9k even with the cleanup goroutine blocked in `net.Conn.Read` | Watchdog cannot deliver second-signal-immediate-exit guarantee; spec D6 violated | Unit test for the two-goroutine pattern with injected signal channel | Unverified — verifiable in U6 |
| A5 | Cmux's `WorkspaceClose` failure during teardown is the rare path, not the common path | The operator-visible orphan diagnostic per [D-11](artifacts/implementation-decision-log.md#d-11-best-effort-teardown-with-operator-visible-diagnostic-on-workspaceclose-failure) becomes operator-visible noise on every successful run | Phase 1 demo on a healthy host; subsequent ratio observed during dogfooding | Unverified — verifiable post-ship via real-use telemetry |

### Issues

| ID | Issue | Owner | Next Step |
|----|-------|-------|-----------|
| I1 | OI-1 (pinned cmux version) is unresolved and is now a release-readiness blocker (not just a documentation gap) per devops DOR-009/DOR-012 | project-manager | Pin a cmux version per build-outline OQ-1 recommendation (current latest cmux release); update setup how-to with the pinned version |
| I2 | OI-3 (cmux setup how-to documenting the dismissal gestures) is unresolved and depends on OI-1 | project-manager | After OI-1 resolves, document the workspace-close and close-pane gestures for the pinned cmux version |

### Dependencies

| ID | Dependency | Owner | Status |
|----|------------|-------|--------|
| Dep1 | Pinned cmux version (OI-1) — needed for setup how-to and for verifying assumptions A1, A2 | project-manager | Open |
| Dep2 | Cmux's documented JSON-RPC method names for the Phase 1 RPC surface (`workspace.create`, `surface.split`, `surface.spawn`, `surface.hide`, `surface.list`, `workspace.list`, `workspace.close`, `workspace.current`, `workspace.select`, `system.identify`) — needed to implement `RealClient` | `devops-engineer` | Investigation doc (`docs/plans/cmux-rebuild/investigation.md`) sketches the API; verify against pinned cmux version |

## Testing Strategy

The test plan rests on test-engineer T1 through T14 (R1) and the YAGNI deferrals S1, S2, S3 (R1). The test seam is the `CmuxClient` interface per [D-18](artifacts/implementation-decision-log.md#d-18-internalcmuxctl-package-layout--interface-realclient-fakeclient-preflight-runphase1); every behavioral commitment except live-cmux integration testing is exercised through `FakeClient`.

- **Observable behaviors to test** (per work-unit verification column above):
  - `--cmux` flag parses, help output is correct ([D-2](artifacts/implementation-decision-log.md#d-2--cmux-flag-ships-visible-with-experimental-help-text)).
  - Each of the five distinguishable preflight failures produces the correct named error.
  - `CMUX_SOCKET_PATH` validation rejects each attack class (non-existent, non-socket, symlink-to-non-socket, world-writable parent) and accepts trusted paths ([D-15](artifacts/implementation-decision-log.md#d-15-cmux_socket_path-validation-before-netdial)).
  - Workspace-name composition handles every spec D11 sanitization case (spaces, slashes, dots, control characters, unicode, empty result fallback to `repo`).
  - Workspace-name collision retried exactly once; second collision routes to partial-setup teardown ([spec D12](artifacts/decision-log.md#d12-workspace-name-collision-retried-once-then-fails)).
  - Spawn order is orchestrator → header → log → footer ([D-3](artifacts/implementation-decision-log.md#d-3-pane-spawn-order-is-orchestrator-first)).
  - Dismissal observation fires exactly once on workspace-removed and exactly once on per-pane exit; three consecutive poll timeouts escalate to fatal sentinel; `shuttingDown` flag suppresses post-shutdown observations ([D-6](artifacts/implementation-decision-log.md#d-6-dismissal-observation-via-500ms-polling-with-single-flight), [D-7](artifacts/implementation-decision-log.md#d-7-d9-poll-timeout-escalation-tolerates-n3-consecutive-fires-before-fatal-teardown), [D-8](artifacts/implementation-decision-log.md#d-8-self-close-double-fire-mitigation-via-shuttingdown-flag-and-channel-priming)).
  - Two-goroutine signal pattern: cleanup runs teardown, watchdog `os.Exit` callback fires on second signal regardless of cleanup state ([D-9](artifacts/implementation-decision-log.md#d-9-two-goroutine-watchdogcleanup-signal-handling-pattern)).
  - Best-effort teardown produces operator-visible orphan diagnostic on `WorkspaceClose` failure; focus-restore is silent on stale prior workspace ([D-11](artifacts/implementation-decision-log.md#d-11-best-effort-teardown-with-operator-visible-diagnostic-on-workspaceclose-failure)).
  - `ansi.StripForTerminalOutput` strips every C0 cursor-movement byte and every C1 control in `0x80–0x9F` while preserving LF and HT ([D-14](artifacts/implementation-decision-log.md#d-14-stripforterminaloutput-ansi-variant-for-the-cmux-diagnostic-path)); existing `StripAll` tests unchanged ([D-27](artifacts/implementation-decision-log.md#trivial-decisions)).
- **Test doubles posture:** `FakeClient` is a real test double with `sync.Mutex` protection per `docs/coding-standards/testing.md`. Scripted per-method behavior; sync-safe `Closed` idempotency check.
- **Edge cases requiring coverage:**
  - "No prior workspace" launching state ([spec D10](artifacts/decision-log.md#d10-no-prior-workspace-state-handled-gracefully)) — `WorkspaceCurrent` returns empty.
  - Captured prior workspace gone at restore time — `WorkspaceSelect` errors silently swallowed.
  - Same-nanosecond collision — `WorkspaceCreate` returns duplicate-name error on first attempt.
  - Each placeholder process exit observed via `SurfaceList` ([D-19](artifacts/implementation-decision-log.md#d-19-per-pane-exit-observation-via-surfacelist-or-equivalent)).
- **Test levels:** unit (every behavioral commitment via `FakeClient`); integration (live-cmux smoke test deferred per OI-2 / build-outline OQ-2 — Phase 1 ships with mocked-only coverage; revisit at Phase 6).
- **Race detector:** `make test` runs `go test -race ./...` per `docs/coding-standards/testing.md`. The dismissal-observer goroutine, the single-flight RPC queue, the two-goroutine signal pattern, and the `shuttingDown` flag are all under race-detector scrutiny.

## Security Posture

The adversarial-security-analyst engaged in R2 produced five findings (SEC-001 through SEC-005). Phase 1 commits to the following mitigations:

- **SEC-001 (C0 cursor-movement injection) and SEC-004 (C1 OSC title-set phishing):** new `ansi.StripForTerminalOutput` function applied to every cmux-supplied diagnostic before terminal printing ([D-14](artifacts/implementation-decision-log.md#d-14-stripforterminaloutput-ansi-variant-for-the-cmux-diagnostic-path)). Existing `StripAll` callers (workflowio recovery view, sandbox-create smoke test) are NOT migrated and preserve their existing operator-inspection contract ([D-27](artifacts/implementation-decision-log.md#trivial-decisions)).
- **SEC-002 (double-ESC bypass — cosmetic):** documented in a code comment on the existing `StripAll` function. Not exploitable; no code change.
- **SEC-003 (`CMUX_SOCKET_PATH` SSRF / T2 ancestry-model bypass):** env-var validation per [D-15](artifacts/implementation-decision-log.md#d-15-cmux_socket_path-validation-before-netdial) — empty-check + `os.Stat` socket-mode check + `EvalSymlinks` canonicalization + parent-directory ownership check. Plus the existing `system.identify` response check (parent D18) requires the response to identify as cmux. Operator-visible error messages print the resolved (symlink-expanded) canonical path so redirection is visible.
- **SEC-005 (SIGHUP-orphan):** SIGHUP registration per [D-10](artifacts/implementation-decision-log.md#d-10-sighup-registered-alongside-sigint-and-sigterm) closes the guaranteed-orphan-on-terminal-close path. Combined with [D-9](artifacts/implementation-decision-log.md#d-9-two-goroutine-watchdogcleanup-signal-handling-pattern)'s second-signal-exit guarantee, the signal-handling surface is fully covered for Phase 1.

The first external-data-to-terminal flow in the codebase is the cmux-diagnostic path; Phase 1 establishes the sanitization pattern that later phases inherit when more external data crosses the cmux boundary.

## Operational Readiness

- **Observability:** Phase 1 produces no `.pr9k/logs/` artifacts (per spec Coordinations table and [D-16](artifacts/implementation-decision-log.md#d-16-skip-standard-logger-and-bubble-tea-wiring-on-cmux-phase-1-path)). Operator-visible signals on the launching terminal: workspace-name confirmation (success), distinguishable preflight error message (failure), partial-setup teardown diagnostic (cmux-side mid-setup failure), orphan-workspace diagnostic on `WorkspaceClose` failure ([D-11](artifacts/implementation-decision-log.md#d-11-best-effort-teardown-with-operator-visible-diagnostic-on-workspaceclose-failure)). Every operator-visible cmux-supplied text is sanitized via `ansi.StripForTerminalOutput` per [D-14](artifacts/implementation-decision-log.md#d-14-stripforterminaloutput-ansi-variant-for-the-cmux-diagnostic-path).
- **SLO impact:** none. Phase 1 is operator-driven, not service-driven. The 500ms poll cadence ([D-6](artifacts/implementation-decision-log.md#d-6-dismissal-observation-via-500ms-polling-with-single-flight)) bounds dismissal-detection latency to ≤ ~510ms in the happy case and ≤ ~1.5 × per-call-timeout (parent D27 = 5–10s) in the worst case.
- **Feature flag:** `--cmux` flag is the operator-facing opt-in per parent D25 + [D-2](artifacts/implementation-decision-log.md#d-2--cmux-flag-ships-visible-with-experimental-help-text). Default `false`. No staged rollout — Phase 1 ships visible from day one with experimental help text.
- **Rollout:** Phase 1 ships in version 0.11.0 ([D-26](artifacts/implementation-decision-log.md#trivial-decisions)). The release notes name Phase 1's scope (workspace lifecycle only; no real workflow content yet). Rollout is "operators read the release notes and the new how-to."
- **Rollback:** revert the version-bump commit and the Phase 1 PR. The `--cmux` flag disappears; standard-mode behavior is unaffected (the `main()` branch is `if cfg.Cmux { ... } else { existing path }`). Time-to-rollback: one `git revert` + one `make build`.
- **Cost and scale:** 500ms poll cadence × two RPCs per cycle × workspace-lifetime average (assume 30s for the demo, 5min for steady-state inspection) yields ~120–1200 RPCs per launch — well under cmux's RPC budget (control-plane operations on a single Unix socket).
- **Compliance controls:** none added. `CMUX_SOCKET_PATH` validation is a security control, not a compliance one.

## Definition of Done

- [ ] `pr9k --cmux --help` shows the `--cmux` flag with the experimental help text from [D-2](artifacts/implementation-decision-log.md#d-2--cmux-flag-ships-visible-with-experimental-help-text).
- [ ] `pr9k --version` prints `0.11.0` ([D-26](artifacts/implementation-decision-log.md#trivial-decisions)).
- [ ] The build-outline Phase 1 5-step demo runs end-to-end on macOS:
  1. From inside a cmux session, the operator launches pr9k against a target repository in cmux mode.
  2. A fresh cmux workspace appears named `pr9k-<sanitized-basename>-<nanosecond-timestamp>`. Four panes (orchestrator hidden, header / log / footer visible with placeholder labels).
  3. The operator switches to another cmux workspace and back; the pr9k workspace persists with all panes intact.
  4. The operator closes the workspace via cmux's own controls; cmux returns them to the prior workspace.
  5. Repeat the test with cmux stopped: pr9k aborts with the named "cmux is installed but not running" message. Repeat with a cmux missing a required capability: distinct named error with the missing capability name.
- [ ] All test-plan checkmarks pass (`make test` with race detector; `make ci`).
- [ ] No lint suppressions (`docs/coding-standards/lint-and-tooling.md`); no `//nolint`, no `.golangci.yml` exclusions.
- [ ] Feature doc `docs/features/cmux-mode.md` exists and accurately describes Phase 1 behavior.
- [ ] How-to `docs/how-to/setting-up-cmux.md` exists, names the pinned cmux version (resolves OI-1), documents the dismissal gestures (resolves OI-3), and includes orphan-recognition instructions for the operator (`pr9k-` prefix in cmux's workspace list, per junior Q10).
- [ ] Code-package doc `docs/code-packages/cmuxctl.md` exists and accurately describes the package contract.
- [ ] CLAUDE.md is updated with links to the new docs.
- [ ] `internal/ansi.StripForTerminalOutput` exists and is unit-tested for every C0 and C1 byte covered by [D-14](artifacts/implementation-decision-log.md#d-14-stripforterminaloutput-ansi-variant-for-the-cmux-diagnostic-path).
- [ ] `CMUX_SOCKET_PATH` validation rejects every attack class covered by [D-15](artifacts/implementation-decision-log.md#d-15-cmux_socket_path-validation-before-netdial).
- [ ] The dismissal-observer goroutine exits cleanly under context cancellation (race detector passes).
- [ ] Post-ship owner: River (until cmux mode reaches Phase 6 stability).

## Specialist Handoffs for Implementation

- **`adversarial-security-analyst`** — dispatch for code review of `ansi.StripForTerminalOutput` and `CMUX_SOCKET_PATH` validation before merge. Needs: the implementation diff, plus the SEC-001/SEC-003/SEC-004 exploit cases as regression tests.
- **`test-engineer`** — dispatch to verify the test plan against the implementation before merge. Needs: every U#-verification entry from the Decomposition table mapped to a real test, plus confirmation that `FakeClient` covers each scripted case.
- **`devops-engineer`** — dispatch when OI-1 resolves to verify A1 and A2 (cmux's `surface.list` exists, `system.identify` returns a recognizable identity string) against the pinned cmux version. Needs: pinned cmux version + access to a host with cmux installed.
- **`behavioral-analyst`** — dispatch for code review of the dismissal-observation goroutine + signal handler + teardown sequence. Needs: the implementation diff, plus the spec D9 + D6 + D10 commitments mapped to specific code lines.
- **`concurrency-analyst`** — dispatch for race-detector clearance pass before merge. Needs: `make test` output (race detector active) and a brief on the goroutine count (single-flight RPC queue, dismissal observer, two-goroutine signal pattern).
- **`junior-developer`** — dispatch for a fresh-eyes pass on the new feature doc + how-to + code-package doc before merge. Needs: the docs, plus the build-outline Phase 1 5-step demo to verify the docs are reproducible.

## Deferred (YAGNI)

The YAGNI gate at Step 7.5 demoted the following items to this section. Each can be reopened on the named trigger.

### YAGNI-1: Drop `.` from the spec D11 sanitization character set

- **Why deferred:** Conjectural; no evidence cmux reserves dotted workspace names in any release. Removing `.` proactively would break sanitization for repos with version suffixes (`my-app.v2`, `pr9k.git`) for no demonstrated benefit. Retained as a Phase 1 release-readiness check tied to OI-1 per [D-17](artifacts/implementation-decision-log.md#d-17-d11-sanitization-character-set-keeps--for-now-phase-1-release-readiness-check-tied-to-oi-1).
- **Reopen when:** OI-1 resolves and the pinned cmux version's documentation reveals dotted-name restrictions.
- **Source:** R1, devops-engineer DOR-010.

### YAGNI-2: JSON Schema-shape validation in `internal/cmuxctl`

- **Why deferred:** Parent D18 explicitly accepts method-presence checking; standard `json.Unmarshal` zero-values handle unknown/missing fields. No incident, no named cmux release that broke shape. Cmux does not publish JSON schemas, so a validator would hand-codify a contract pr9k cannot independently enforce. Decision recorded at [D-13](artifacts/implementation-decision-log.md#d-13-method-presence-capability-check-accepted-schema-shape-validation-deferred).
- **Reopen when:** A cmux release breaks a method's response shape and produces a confusing error in production.
- **Source:** R1, junior-developer Q2.

### YAGNI-3: Double-signal orphan-path test

- **Why deferred:** Test is timing-dependent and brittle (relies on injected signal-channel behavior crossing the goroutine boundary precisely). The two-goroutine pattern itself is unit-tested via the injected signal channel ([D-9](artifacts/implementation-decision-log.md#d-9-two-goroutine-watchdogcleanup-signal-handling-pattern)); the orphan-path test adds little beyond what U6's injected-signal tests cover.
- **Reopen when:** Phase 6 hardens the signal-handler with proper test seams (e.g., a `signalDispatcher` interface).
- **Source:** R1, test-engineer S1.

### YAGNI-4: Live cmux wire-protocol test

- **Why deferred:** Requires a running cmux process; build-outline OQ-2 recommendation concurs that live-cmux integration testing is deferred to Phase 6.
- **Reopen when:** Phase 6 starts and CI integration-test infrastructure is decided per OI-2 / build-outline OQ-2.
- **Source:** R1, test-engineer S2.

### YAGNI-5: `FakeClient.Close` idempotency test

- **Why deferred:** Applies only if the `CmuxClient` interface commits to a `Close` method. `RealClient` has internal close semantics, but the interface itself does not require `Close` in Phase 1.
- **Reopen when:** A `Close` method is added to the `CmuxClient` interface (likely Phase 4 when sidebar mirroring may need explicit lifecycle management).
- **Source:** R1, test-engineer S3.

## Open Items

- **OI-1: Named minimum-supported cmux version (inherited from spec).**
  - **Resolves when:** the team picks a cmux release to pin (build-outline OQ-1 recommendation: current latest cmux release).
  - **Blocks implementation:** No for the code (capability check is method-presence-based per parent D18); **Yes for the setup how-to** (operators need a "tested against version X" line) per devops DOR-009/DOR-012. Now a release-readiness blocker.
- **OI-2: CI testing strategy for cmux mode (inherited from spec).**
  - **Resolves when:** the team commits to a testing approach (build-outline OQ-2 recommendation: mock through Phase 5; revisit at Phase 6).
  - **Blocks implementation:** No — Phase 1 ships with mocked-only coverage; live-cmux integration testing is deferred to Phase 6 per YAGNI-4.
- **OI-3: cmux setup how-to must document workspace-dismissal gestures (inherited from spec).**
  - **Resolves when:** the cmux setup how-to is written and lists the gestures the pinned cmux version exposes for workspace dismissal.
  - **Blocks implementation:** No for the code; **Yes for U9 documentation** (the Phase 1 demoable assumes the operator knows what gesture to use).
- **OI-A: Spec amendment to remove the exit-code distinction on dismissal.**
  - **Resolves when:** the spec's User Interactions Dismissal-gestures bullet is amended to match [D-12](artifacts/implementation-decision-log.md#d-12-drop-the-exit-code-distinction-on-dismissal--every-dismissal-observation-produces-exit-zero) (every dismissal observation produces exit zero; non-zero reserved for failures).
  - **Blocks implementation:** No — implementation follows [D-12](artifacts/implementation-decision-log.md#d-12-drop-the-exit-code-distinction-on-dismissal--every-dismissal-observation-produces-exit-zero) regardless. Recommended that River updates the spec in a follow-up commit so the spec and implementation align.

## Summary

- **Outcome delivered:** an opt-in `--cmux` launch mode that stands up a four-pane cmux workspace with placeholder content, holds it open until dismissal, tears down cleanly with focus restored, and ships with the docs + version bump that the project's coding standards require.
- **Team size:** 7 specialists active (devops, behavioral, concurrency, test-engineer, junior, security, project-manager), 6 stood down or not needed — see [artifacts/implementation-iteration-history.md](artifacts/implementation-iteration-history.md)
- **Rounds of facilitation:** 2 — see [artifacts/implementation-iteration-history.md](artifacts/implementation-iteration-history.md)
- **Decisions committed:** 27 (24 full + 3 trivial) — see [artifacts/implementation-decision-log.md](artifacts/implementation-decision-log.md)
- **Decisions settled by evidence:** 23 — see [artifacts/implementation-decision-log.md](artifacts/implementation-decision-log.md)
- **Decisions settled by junior-developer reframing:** 3 (D-2, D-13, D-20 — junior-developer findings reframed into evidence-backed accept-the-risk + YAGNI-defer outcomes) — see [artifacts/implementation-decision-log.md](artifacts/implementation-decision-log.md)
- **Decisions settled by user input:** 1 (D-12, exit-code distinction dropped) — see [artifacts/implementation-decision-log.md](artifacts/implementation-decision-log.md)
- **Rejected alternatives recorded:** 75+ — see [artifacts/implementation-decision-log.md](artifacts/implementation-decision-log.md)
- **Open items remaining:** 4 (OI-1 + OI-2 + OI-3 inherited from spec; OI-A new spec-amendment recommendation)
- **Recommendation:** **Ship as planned.** Implementation can begin immediately on U1 + U2 + U8 (independent of OI-1). U3 partial — env-var validation can ship without OI-1 — and U4 + U5 + U6 + U7 can ship as soon as the package skeleton is in place. U9 documentation must wait for OI-1 to resolve (one open dependency: pin a cmux version per build-outline OQ-1 recommendation). U10 version-bump lands with U9 in the same PR.

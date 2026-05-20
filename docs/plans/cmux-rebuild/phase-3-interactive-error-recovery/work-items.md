# Work Items — Phase 3: Interactive Error Recovery

These work items break down the [Phase 3 feature implementation plan](feature-implementation-plan.md) (which absorbs the never-wired Phase 2 happy-path integration as foundational work, then layers error recovery on top). The behavioral source is the [Phase 3 feature specification](feature-specification.md).

Work items are numbered `W-N` for cross-reference only. `Depends on` lines refer to other work items in this file. Work items appear in dependency order. Plan-level decisions are restated inline with a `See plan: D-N` breadcrumb (the decision log itself is a process artifact and is not linked).

The delivery is two ordered slices in a single combined release: slice 1 (W-1–W-4) wires the happy path and is demoable alone; slice 2 (W-5–W-8) verifies error recovery and ships the docs/version. The error path is **not** separable from the wiring — the same `runStepWithErrorHandling` in `src/internal/ui/orchestrate.go` both completes steps and enters `ModeError`, so wiring `workflow.Run` necessarily activates error mode.

## Shared reference artifacts

These artifacts apply to more than one work item. Each work item's own `**References.**` block links the ones it needs; the canonical description lives here.

- **Interaction-channel message set & test doubles** — [`../../../code-packages/interactionchannel.md`](../../../code-packages/interactionchannel.md). Defines the `StateHeader` / `StateLog` / `StateFooter` / `Intent` / `WorkspaceDone` message types and length-prefixed JSON framing, the per-connection goroutine model (read + write + watcher), and the three Phase 2 test doubles `FakeInteractionChannel`, `FakeDisplayPane`, `FakeFooterKeySource`. This is the event-contract equivalent for Phase 3 — no new message kinds are added (See plan: D-10). Shared across W-1, W-2, W-3, W-4, W-5.
- **Feature specification behaviors** — [`feature-specification.md`](feature-specification.md). The D1–D4 spec behaviors are realized across multiple work items: D1 (reuse the inherited error-mode loop unchanged), D2 (`[✗]` failure marker in the header), D3 (error output + retry separator as ordinary streamed content, separator-before-output ordering), D4 (control keys absorbed silently on non-control panes). Cited per work item by the spec section that defines the behavior.
- **Narrow-reading ADR** — [`../../../adr/20260410170952-narrow-reading-principle.md`](../../../adr/20260410170952-narrow-reading-principle.md). pr9k is a generic step runner; no cmux-specific or Ralph-specific recovery logic enters Go code. Phase 3 surfaces the existing `internal/ui` machinery through the panes, it does not reimplement it. Applies to W-2, W-3, W-4.
- **No lint suppressions** — [`../../../coding-standards/lint-and-tooling.md`](../../../coding-standards/lint-and-tooling.md). Applies to every code-bearing work item (W-1–W-5, W-8).

## W-1 — Expand `orchChannel` interface with the three state-push methods

**Summary.** Expand the existing `orchChannel` interface in place with `SendStateHeader` / `SendStateLog` / `SendStateFooter` so the orchestrator pane can push pane state, without introducing a new richer interface type. This is the interface surface the wired orchestrator (W-2) and the display panes (W-3, W-4) consume. See plan: D-3 (single concrete need, single production implementation, `FakeInteractionChannel` already implements the methods from Phase 2 U10 — expand in place rather than add a new type).

**Description.**
1. Add `SendStateHeader`, `SendStateLog`, and `SendStateFooter` method signatures to the `orchChannel` interface in `src/cmd/pr9k/cmux_pane.go` (currently lines 27–32, exposing only `AwaitReady`/`Send`/`Recv`/`Close`). Match the `StateHeader` / `StateLog` / `StateFooter` payload shapes from the interaction-channel message set.
2. Add a compile-time interface assertion that the production `*interactionchannel.Channel` satisfies the expanded `orchChannel`, per the adapter / compile-time-assertion convention.
3. Confirm `FakeInteractionChannel` already implements all three methods unchanged (Phase 2 U10). If any is missing, add the minimal mutex-protected stub so the double stays a faithful test surface.
4. No caller is wired in this work item — this is the interface surface only. It is not a stub: it produces a compile-clean, test-clean expansion that W-2/W-3/W-4 depend on.

**References.**
- **Event contract (shared)** — [`../../../code-packages/interactionchannel.md`](../../../code-packages/interactionchannel.md): `StateHeader`, `StateLog`, `StateFooter` message shapes; `FakeInteractionChannel` test double.
- **ADR / standard** — [`../../../coding-standards/api-design.md`](../../../coding-standards/api-design.md) (adapter types, compile-time interface assertions); [`../../../coding-standards/lint-and-tooling.md`](../../../coding-standards/lint-and-tooling.md) (no suppressions).

**Tests.**
- Compile-time assertion: `var _ orchChannel = (*interactionchannel.Channel)(nil)` (or equivalent) compiles.
- Unit: `FakeInteractionChannel` satisfies the expanded `orchChannel` (compile assertion in the test package); existing Phase 2 tests over the double still pass.
- `go test -race ./...` passes.

**Acceptance criteria.**
- [ ] `orchChannel` exposes `SendStateHeader`, `SendStateLog`, `SendStateFooter` plus the original `AwaitReady`/`Send`/`Recv`/`Close`; no new interface type was introduced.
- [ ] Compile-time assertions for both the production channel and `FakeInteractionChannel` compile.
- [ ] `make ci` passes with no lint suppressions.

**Depends on.** None.

## W-2 — Wire the orchestrator pane to `workflow.Run` with adapters, cancel, and concurrency ordering

**Summary.** Replace the orchestrator-pane stub with a real `workflow.Run` invocation driven by `RunHeader` / `StepExecutor` / `KeyHandler` adapters, the `KeyHandler` cancel set to `runner.Terminate`, the mode-change hook and key-adapter goroutine established before `workflow.Run` starts, and a synchronous `WriteToLog` → `SendStateLog` adapter. This closes the never-wired Phase 2 integration gap and, because the error path is not separable, simultaneously activates error mode. See plan: D-1 (absorb the Phase 2 end-to-end wiring), D-4 (`KeyHandler` cancel = `runner.Terminate`, mirroring `main.go:226`), D-5 (the `WriteToLog`→`SendStateLog` adapter is synchronous on the orchestrator goroutine so the retry separator is enqueued before any retried-step output), D-6 (establish `SetOnModeChange` + the key-adapter goroutine before `workflow.Run`; drain the footer-keystroke goroutine on context cancel via `sync.WaitGroup`), D-9 (reuse `internal/ui` unchanged).

**Description.**
1. Replace the stub at `src/cmd/pr9k/cmux_pane.go:113-114` (`// Workflow runs here — real implementation ships in a later work unit.` / `exitCode := 0`) with a real `workflow.Run(executor, header, keyHandler, cfg)` call, mirroring the standard path at `src/cmd/pr9k/main.go:374`.
2. Construct three adapters: a `RunHeader` whose `SetStepState` calls `ch.SendStateHeader`; a `StepExecutor`/`Runner` whose `SetSender`-drained log lines call `ch.SendStateLog`; a `KeyHandler` whose `SetOnModeChange` pushes `StateFooter` and whose `Actions` channel receives footer intents. Use compile-time interface assertions on each adapter.
3. Construct the `KeyHandler` with `runner.Terminate` as the cancel function (not `nil`), so quit during an active/error step actually terminates the running subprocess (See plan: D-4; mirrors `main.go:226`).
4. Implement the `WriteToLog` → `SendStateLog` adapter **synchronously on the orchestrator goroutine** so `runner.WriteToLog(RetryStepSeparator(step.Name))` (`orchestrate.go:135`) is enqueued to the log pane before any retried-step output (See plan: D-5; spec D3).
5. Establish `SetOnModeChange` and start the key-adapter goroutine **before** calling `workflow.Run`; drain the footer-keystroke goroutine on context cancel via `sync.WaitGroup`; rely on the buffered `h.Actions` (cap 10) + synchronous `onModeChange` "prime the channel before blocking receive" pattern (See plan: D-6).
6. Keep all recovery semantics in shared `internal/ui` — add no cmux-specific recovery logic to Go code (narrow-reading ADR).

**Note on Assumption A1 (Step 0 — before any wiring).** A1 — that the `internal/ui` renderers (`StatusHeader`, log panel, `KeyHandler`, footer state machine) are usable from the `cmux-pane` sub-command entry points without standard-mode Bubble Tea program-loop dependencies — is unverified. Inspect the renderer constructors first. If A1 holds (the expected case — the Phase 2 footer state machine + key adapter were authored Bubble-Tea-free), proceed with the wiring as ordinary code; this work item is AFK. If A1 proves false (renderers need bespoke wrappers), **stop and escalate** the plan's D-2 decomposition revisit criterion before continuing — do not silently work around it. This is an escalation gate within an AFK work item, not a standing human-in-the-loop requirement.

**References.**
- **Event contract (shared)** — [`../../../code-packages/interactionchannel.md`](../../../code-packages/interactionchannel.md): `StateHeader`/`StateLog`/`StateFooter` shapes; per-connection goroutine model.
- **Repo doc** — [`../../../code-packages/workflow.md`](../../../code-packages/workflow.md): `Runner`, `RunStep`/`RunStepFull`, `StepExecutor` interface, the `Run` loop, `SetSender`.
- **Spec section** — [`feature-specification.md#primary-flow`](feature-specification.md#primary-flow) (D1: reuse the inherited error-mode loop unchanged); [`feature-specification.md#coordinations`](feature-specification.md#coordinations) (D3 separator-before-output ordering on the way to the pane).
- **ADR / standard** — [`../../../adr/20260410170952-narrow-reading-principle.md`](../../../adr/20260410170952-narrow-reading-principle.md); [`../../../coding-standards/concurrency.md`](../../../coding-standards/concurrency.md) (establish-before-start, WaitGroup drain, prime-the-channel); [`../../../coding-standards/api-design.md`](../../../coding-standards/api-design.md) (adapter types, compile-time assertions); [`../../../coding-standards/lint-and-tooling.md`](../../../coding-standards/lint-and-tooling.md).

**Tests.**
- Unit via `FakeInteractionChannel`: orchestrator drives a scripted single-step workflow; `StateHeader`, `StateLog`, `StateFooter` are pushed in order.
- Unit: the constructed `KeyHandler` has a non-nil cancel function (assert it invokes `runner.Terminate`).
- Unit: the mode-change hook is set and the key-adapter goroutine is started **before** `workflow.Run` is invoked (assert via test sequencing / a recording double).
- Unit: the `WriteToLog`→`SendStateLog` adapter delivers a separator before subsequent output on the same goroutine (ordering assertion).
- Concurrency: footer-keystroke goroutine exits on context cancel; `go test -race ./...` clean.

**Acceptance criteria.**
- [ ] The stub at `cmux_pane.go:113-114` is gone; the orchestrator pane runs a real `workflow.Run`.
- [ ] `KeyHandler` is constructed with `runner.Terminate` as cancel (non-nil).
- [ ] `SetOnModeChange` + the key-adapter goroutine are established before `workflow.Run`; the footer-keystroke goroutine is WaitGroup-drained on cancel.
- [ ] The `WriteToLog`→`SendStateLog` adapter is synchronous on the orchestrator goroutine.
- [ ] Assumption A1 was inspected and confirmed (or escalated per the D-2 revisit criterion) before merge.
- [ ] `make ci` passes with the race detector and no lint suppressions.

**Depends on.** `W-1`.

## W-3 — Header and log panes consume and render `StateHeader` / `StateLog`

**Summary.** Extend `runCmuxDisplayPane` so the header and log panes consume the pushed `StateHeader` / `StateLog` messages and render them — the header showing the step grid including the `[✗]` failure marker, the log streaming output with the retry separator preserved before retried-step output. Both panes remain display-only consumers with no key path. See plan: D-2 (header/log panes consume pushed state; `runCmuxDisplayPane` no longer ignores them), D-5 (the synchronous log adapter preserves separator-before-output order on the way to the pane).

**Description.**
1. `runCmuxDisplayPane` (`src/cmd/pr9k/cmux_pane.go:164-193`) currently ignores every message except `WorkspaceDone`. Extend its message-dispatch loop to handle `StateHeader` and `StateLog`.
2. Header role: forward `StateHeader` to a renderer that produces the checkbox step grid and renders `[✗]` for a `StepFailed` step state, the same failure marker the standard display uses (spec D2). A successful retry advances the step to `[✓]`; a re-failure stays `[✗]`.
3. Log role: forward `StateLog` lines to a renderer that appends them to the pane output in arrival order, preserving the separator-before-output ordering W-2's synchronous adapter guarantees (spec D3). Error output renders as ordinary streamed content — no separate error panel, no special formatting.
4. Keep both panes display-only: no footer state machine, no intent forwarding, no key handling (this is what makes control-key absorption on non-control panes true — spec D4; verified end-to-end in W-7).

**References.**
- **Event contract (shared)** — [`../../../code-packages/interactionchannel.md`](../../../code-packages/interactionchannel.md): `StateHeader` (step grid, step-state enum incl. `StepFailed`), `StateLog` (line payload), `FakeDisplayPane`, display-side `Recv` loop.
- **Spec section** — [`feature-specification.md#user-interactions`](feature-specification.md#user-interactions) (D2 `[✗]` marker, retry-marking behavior; D3 error output + retry separator as ordinary streamed content).
- **ADR / standard** — [`../../../adr/20260410170952-narrow-reading-principle.md`](../../../adr/20260410170952-narrow-reading-principle.md); [`../../../coding-standards/lint-and-tooling.md`](../../../coding-standards/lint-and-tooling.md).

**Tests.**
- Unit via `FakeDisplayPane` / `FakeInteractionChannel`: `StateHeader` carrying a `StepFailed` index renders `[✗]` at that position; a subsequent success renders `[✓]`.
- Unit: `StateLog` lines render in delivery order; a separator-then-output sequence renders separator-first.
- Unit: `WorkspaceDone` after state messages still produces `DoneAck` and the pane holds until context cancel.
- `go test -race ./...` passes.

**Acceptance criteria.**
- [ ] `runCmuxDisplayPane` consumes and renders `StateHeader` and `StateLog`; it no longer ignores them.
- [ ] Header renders `[✗]` for `StepFailed`; log preserves separator-before-output order.
- [ ] Header and log panes have no key path (display-only).
- [ ] `make ci` passes with no lint suppressions.

**Depends on.** `W-1`.

## W-4 — Footer pane runs `footerStateMachine` + key adapter; isolated channel removed; goroutine drained

**Summary.** Wire the footer pane to the Phase 2 `footerStateMachine` + `newKeyHandlerAdapter` (currently zero non-test callers) so it consumes `StateFooter`, renders normal and error-mode hints, and forwards operator intents over the existing intent stream; remove the footer renderer's dead isolated `KeyHandler`/`actions` channel; drain the footer-keystroke goroutine on context cancel. See plan: D-7 (remove the renderer's isolated `KeyHandler`/`actions` channel — Issue I1 dead/contradictory code — and drive the footer from the wired path), D-6 (WaitGroup-drain the footer-keystroke goroutine on context cancel).

**Description.**
1. `runCmuxFooterPaneWith` (`src/cmd/pr9k/cmux_pane.go`) currently runs the status-line runner and handles `WorkspaceDone` but never runs `footerStateMachine` or `newKeyHandlerAdapter`. Wire both so the footer pane consumes `StateFooter` from the interaction channel and renders the appropriate hints: normal-mode shortcuts and the error-mode continue / retry / quit prompt, with the two-step quit confirmation inline.
2. Forward the operator's resolved choice back to the orchestrator as `Intent` messages over the existing Phase 2 intent stream — the same `actions` channel `workflow.Run` consumes, not an isolated one.
3. Remove the isolated private `KeyHandler` and unread `actions` channel in `src/cmd/pr9k/cmux_footer_renderer.go:22-25` (dead/contradictory code — Issue I1). The renderer keeps only display responsibilities: size and `?`/`esc` help-expand.
4. Drain the footer-keystroke goroutine on context cancel via `sync.WaitGroup` (See plan: D-6).
5. Add no new message kinds and no new cmux calls — the protocol is unchanged from Phase 2 (See plan: D-10).

**References.**
- **Event contract (shared)** — [`../../../code-packages/interactionchannel.md`](../../../code-packages/interactionchannel.md): `StateFooter`, `Intent`/`IntentType` enum, inbound intent channel, `FakeFooterKeySource`.
- **Spec section** — [`feature-specification.md#alternate-flows-and-states`](feature-specification.md#alternate-flows-and-states) (D1: two-step quit confirm; non-confirmation key ignored and not buffered; cancel restores the same error prompt).
- **ADR / standard** — [`../../../adr/20260410170952-narrow-reading-principle.md`](../../../adr/20260410170952-narrow-reading-principle.md); [`../../../coding-standards/concurrency.md`](../../../coding-standards/concurrency.md) (WaitGroup drain, establish-before-start); [`../../../coding-standards/lint-and-tooling.md`](../../../coding-standards/lint-and-tooling.md).

**Tests.**
- Unit via `FakeFooterKeySource` + `FakeInteractionChannel`: `StateFooter(ModeNormal)` renders normal hints; `StateFooter(ModeError)` renders continue/retry/quit.
- Unit: `q`→`y` produces exactly one quit intent; `q`→`esc` produces no intent and returns to the error-mode render; a non-confirmation key during quit-confirm is ignored and not buffered.
- Concurrency: the footer-keystroke goroutine exits when context is cancelled; race detector clean.
- Unit: the renderer no longer references an isolated `KeyHandler`/`actions` channel (the symbols are gone; no other caller depended on them).

**Acceptance criteria.**
- [ ] The footer pane runs `footerStateMachine` + `newKeyHandlerAdapter`, renders normal + error-mode hints, and forwards intents over the shared stream.
- [ ] The isolated `KeyHandler`/`actions` channel in `cmux_footer_renderer.go:22-25` is removed; no caller depends on it.
- [ ] The footer-keystroke goroutine is WaitGroup-drained on context cancel.
- [ ] No new message kinds or cmux calls were added.
- [ ] `make ci` passes with the race detector and no lint suppressions.

**Depends on.** `W-2`.

## W-5 — Round-trip integration test of the wired error-recovery composition

**Summary.** Author the single round-trip integration-style test over `FakeInteractionChannel` that exercises the wired error-recovery composition end-to-end: orchestrator `SetMode(ModeError)` → `StateFooter(ModeError)` pushed → footer produces an intent → `Orchestrate` acts on it. Phase 2's isolated unit tests are reused unchanged and must still pass against the wired path. See plan: D-8 (one round-trip integration test over `FakeInteractionChannel`; live-cmux deferred to Phase 6; do not author a new harness or a matrix duplicating Phase 2 coverage).

**Description.**
1. Write one round-trip test (a single well-structured test function with sub-cases) over `FakeInteractionChannel` driving the W-2/W-3/W-4 wired composition.
2. Cover the race-window guarantee: a footer intent produced the instant `SetMode(ModeError)` fires is not dropped — the orchestrator acts on it once in error mode (spec D1; the inherited Phase 2 `TestAdapter_ModeErrorRaceWindow_NoDroppedKeystroke` is reused unchanged for the component-level guarantee, not duplicated here).
3. Cover the separator-before-output invariant on retry, a re-failure re-entering error mode (header re-marks `[✗]`, footer re-shows the prompt), and the accepted quit-path transient `StateFooter(ModeNormal)` flash (plan RAID R4 — asserted as non-blocking, on-disk artifact authoritative).
4. Assert control-key delivery to the header/log pane paths produces no intent and no error/bell (spec D4).
5. Confirm the reused Phase 2 isolated tests still pass against the wired path. Add no new test double; all doubles stay `sync.Mutex`-protected per the testing standard.

**References.**
- **Event contract (shared)** — [`../../../code-packages/interactionchannel.md`](../../../code-packages/interactionchannel.md): `FakeInteractionChannel`, `FakeDisplayPane`, `FakeFooterKeySource` full API and mutex-protection contract.
- **Spec section** — [`feature-specification.md#edge-cases-and-failure-modes`](feature-specification.md#edge-cases-and-failure-modes) (D1 no-dropped-choice race window; D2 re-failure re-marks `[✗]`; D3 separator-before-output on retry; D4 control-key absorption).
- **ADR / standard** — [`../../../coding-standards/testing.md`](../../../coding-standards/testing.md) (race detector mandatory; mutex-protected doubles); [`../../../coding-standards/lint-and-tooling.md`](../../../coding-standards/lint-and-tooling.md).

**Tests.**
- The round-trip test itself: orchestrator `SetMode(ModeError)` → `StateFooter(ModeError)` → footer intent → `Orchestrate` action, including the race-window, separator-before-output, re-failure, and `StateFooter(ModeNormal)`-flash sub-cases.
- Regression: Phase 2's isolated unit tests pass unchanged against the wired path.
- `go test -race ./...` passes.

**Acceptance criteria.**
- [ ] One round-trip integration test exists and passes; it is not a duplicate of Phase 2's isolated tests.
- [ ] Phase 2's isolated unit tests pass unchanged against the wired path.
- [ ] No new test double was introduced; doubles remain mutex-protected.
- [ ] `make ci` passes with the race detector and no lint suppressions.

**Depends on.** `W-2`, `W-3`, `W-4`.

## W-6 — Manual happy-path demo: real workflow runs end-to-end in a live cmux workspace

**Summary.** Human-run verification that `pr9k --cmux --project-dir <repo>` from inside a live cmux session runs a real workflow end-to-end — the Phase 2 happy-path that was never observable from running code, now actually wired. This satisfies the first Definition-of-Done gate. No code changes. See plan: D-1 (the absorbed Phase 2 wiring), D-2 (slice 1 is demoable alone); plan Definition of Done, first bullet.

**Description.**
1. Against a real-workflow target repository and a live cmux session, run `pr9k --cmux --project-dir <repo>`.
2. Confirm the four panes connect (Phase 1/2 lifecycle holds, readiness handshake completes), the header checkboxes tick over and the iteration counter advances, the log pane streams subprocess output, and the footer pane shows the status line + shortcuts.
3. Confirm the on-disk artifacts at `<projectDir>/.pr9k/logs/<run-stamp>/` are produced and match the standard-mode artifact shape (per-run log file + per-step JSONL).
4. This is a human verification gate (cmux cannot be launched in CI — Open Item OI-1); it precedes the error-recovery demo (W-7).

**Note on scope boundary with W-7.** W-6 verifies only the happy path (no failing step). The failing-step error-recovery behaviors are W-7's gate and must not be conflated here.

**References.**
- **Repo doc** — [`../../../how-to/setting-up-cmux.md`](../../../how-to/setting-up-cmux.md) (install, launch, navigate); [`../../../features/cmux-mode.md`](../../../features/cmux-mode.md) (four-pane layout, Phase 2 lifecycle); [`../../../code-packages/cmuxctl.md`](../../../code-packages/cmuxctl.md) (Phase 1/2 lifecycle, preflight checks).
- **Spec section** — [`feature-specification.md#outcome`](feature-specification.md#outcome) (the now-working cmux run precondition).

**Tests.**
- Manual: a human observer confirms the four observable behaviors (panes connect, header ticks + counter advances, log streams, footer shows status line + shortcuts) and the on-disk artifact shape.
- Regression: `make ci` after the demo to confirm no code regressed during W-2–W-4.

**Acceptance criteria.**
- [ ] A human confirmed the workflow runs end-to-end across the four panes in a live cmux workspace.
- [ ] On-disk artifacts at `<projectDir>/.pr9k/logs/<run-stamp>/` are present and match the standard-mode shape.
- [ ] `make ci` passes.

**Depends on.** `W-2`, `W-3`, `W-4`.

## W-7 — Manual error-recovery demo: a failing step is interactively recoverable in a live cmux workspace

**Summary.** Human-run verification, against a real failing step in a live cmux session, that the failing step is interactively recoverable from the footer pane with standard-display semantics. This is the second Definition-of-Done gate. Live-cmux CI is deferred to Phase 6 (OI-1). See plan: D-1, D-4, D-5, D-8; plan Definition of Done, error-recovery bullets.

**Description.**
1. Against a workflow configured with a step that exits non-zero in a way that triggers the existing error mode, trigger the error path in a live cmux session.
2. Confirm: the header marks the step `[✗]` (spec D2); the error output appears in the log pane as ordinary streamed content (spec D3); the footer switches to the continue / retry / quit prompt (spec D1).
3. Confirm retry writes the `── <step name> (retry) ─────────────` separator to the log **before** any retried-step output (spec D3; See plan: D-5).
4. Confirm `q`→`y` runs the two-step confirmation and quits; `q`→`esc` cancels and restores the same error prompt; a non-confirmation key during quit-confirm is ignored and not buffered (spec D1). Confirm quit during an active/error step actually terminates the running subprocess (See plan: D-4).
5. Confirm control keys pressed on the header or log pane are absorbed silently — no error, no bell, no notification (spec D4).
6. Observe and note the accepted bounded-desync windows (stale-footer just after failure; continue-before-`[✗]`; quit-path transient `StateFooter(ModeNormal)` flash) — the on-disk artifact is the authoritative failure record.

**References.**
- **Repo doc** — [`../../../how-to/setting-up-cmux.md`](../../../how-to/setting-up-cmux.md) (workspace navigation, pane-focus instructions).
- **Spec section** — [`feature-specification.md#primary-flow`](feature-specification.md#primary-flow) and [`feature-specification.md#alternate-flows-and-states`](feature-specification.md#alternate-flows-and-states) (D1 continue/retry/quit + quit confirm + cancel-restore); [`feature-specification.md#user-interactions`](feature-specification.md#user-interactions) (D2 `[✗]`, D3 streamed error output, D4 control-key absorption); [`feature-specification.md#edge-cases-and-failure-modes`](feature-specification.md#edge-cases-and-failure-modes) (bounded-desync acceptances).

**Tests.**
- Manual: a human observer confirms every error-recovery Definition-of-Done bullet (header `[✗]`, log error output, footer prompt, separator-before-output on retry, two-step quit, cancel-restore, subprocess termination on quit, control-key absorption, accepted bounded-desync windows).
- Verification: the on-disk artifact at `<projectDir>/.pr9k/logs/<run-stamp>/` records the failure and is unchanged in shape from standard mode.

**Acceptance criteria.**
- [ ] A human confirmed all error-recovery behaviors end-to-end against a real failing step in a live cmux workspace.
- [ ] Retry separator appears before any retried-step output; quit terminates the subprocess; control keys on non-control panes are absorbed silently.
- [ ] Bounded-desync windows observed and accepted; on-disk artifact authoritative.

**Depends on.** `W-5`, `W-6`.

## W-8 — Docs and version bump: cmux-mode feature doc, setup how-to, CLAUDE.md links, `0.11.0` → `0.12.0`

**Summary.** Ship the documentation and version bump that the user-visible `--cmux` behavior change requires. Update the cmux-mode feature doc and the cmux setup how-to for the now-working run + error recovery, keep the CLAUDE.md links in sync, and bump `version.Version` `0.11.0` → `0.12.0`. See plan: D-11 (the version bump and feature-doc / how-to updates ship as part of this combined delivery; W-8 is a Definition-of-Done gate, not optional — resolves Open Item OI-2).

**Description.**
1. Bump `src/internal/version/version.go` from `0.11.0` to `0.12.0` (the single source of truth for the version; `pr9k --version` must print `0.12.0`).
2. Update [`docs/features/cmux-mode.md`](../../../features/cmux-mode.md) to reflect that the orchestrator now drives a real workflow end-to-end and that a failing step is interactively recoverable from the footer pane (header `[✗]`, log error output, footer continue/retry/quit, retry separator).
3. Update [`docs/how-to/setting-up-cmux.md`](../../../how-to/setting-up-cmux.md) with the error-recovery walkthrough: what `[✗]` means, how to continue/retry/quit from the footer pane, the two-step quit confirmation, and the separator appearance on retry.
4. Verify every CLAUDE.md link referencing these two doc files still resolves; add or correct links if the doc structure changed.
5. Verify doc code blocks match production code shapes (no doc references a flag, symbol, or output that does not exist). Run `make build` and confirm `pr9k --version` prints `0.12.0`.

**References.**
- **Standard** — [`../../../coding-standards/versioning.md`](../../../coding-standards/versioning.md) (`version.Version` single source of truth; what counts as a user-visible surface; `0.y.z` rules); [`../../../coding-standards/documentation.md`](../../../coding-standards/documentation.md) (feature docs ship with the feature; doc code blocks match production code; update CLAUDE.md when doc files change); [`../../../coding-standards/lint-and-tooling.md`](../../../coding-standards/lint-and-tooling.md).
- **Repo doc** — [`../../../features/cmux-mode.md`](../../../features/cmux-mode.md); [`../../../how-to/setting-up-cmux.md`](../../../how-to/setting-up-cmux.md).
- **Spec section** — [`feature-specification.md#summary`](feature-specification.md#summary) (the user-visible behavior change the docs must describe).

**Tests.**
- Verification: `pr9k --version` prints `0.12.0` after `make build`.
- Verification: all CLAUDE.md links referencing the two updated doc files resolve.
- Verification: no doc code block references a non-existent symbol/flag/output.
- `make ci` passes.

**Acceptance criteria.**
- [ ] `version.Version` is `0.12.0`; `pr9k --version` prints `0.12.0`.
- [ ] `docs/features/cmux-mode.md` and `docs/how-to/setting-up-cmux.md` describe the now-working run + error recovery.
- [ ] CLAUDE.md links in sync; doc code blocks match production code.
- [ ] `make ci` passes with no lint suppressions.

**Depends on.** `W-6`, `W-7`.

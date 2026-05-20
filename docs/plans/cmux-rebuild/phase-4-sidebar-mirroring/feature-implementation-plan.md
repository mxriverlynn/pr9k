# Feature Implementation Plan: Phase 4 — Sidebar Mirroring

Phase 4 mirrors the running workflow's current step name and iteration counter into cmux's persistent sidebar against the pr9k workspace's row, so an operator monitoring from any other cmux workspace sees live progress without switching back. The implementation is intra-codebase: a new `cmuxSidebar` adapter plus a thin `sidebarAwareHeader` wrapper compose alongside the Phase 2 `cmuxHeader`; four new RPC wrappers are added to `cmuxctl.CmuxClient`; the cmux version pin moves from 0.64.6 to 0.64.7; the existing `OnModeChange` closure is augmented in place to emit the error-mode marker; and the orchestrator composition root threads the cmux client and workspace handle that today are silently discarded ([D-5](artifacts/implementation-decision-log.md#d-5-cmuxctlcmuxclient-and-cmuxctlworkspace-are-threaded-through-the-orchestrator-composition-root-from-cmuxorchestratorhooks--runcmuxorchestratorwith--runcmuxworkflowadapted)). No new on-disk artifact, no new CLI flag, no schema change.

## Source Specification

- **Feature specification:** [feature-specification.md](feature-specification.md)
- **Spec-stage decision log:** [artifacts/decision-log.md](artifacts/decision-log.md)
- **Spec-stage technical notes:** [artifacts/feature-technical-notes.md](artifacts/feature-technical-notes.md) (T1 — cmux sidebar surface shape)
- **Spec-stage team-findings:** [artifacts/team-findings.md](artifacts/team-findings.md)
- **Parent feature specification:** [../feature-specification.md](../feature-specification.md)
- **Parent decision log:** [../artifacts/decision-log.md](../artifacts/decision-log.md)
- **Phase 2 implementation plan (format precedent + Phase 2 adapter contract Phase 4 extends):** [../phase-2-real-workflow-runs/feature-implementation-plan.md](../phase-2-real-workflow-runs/feature-implementation-plan.md)
- **Phase 3 implementation plan (`OnModeChange` extension Phase 4 augments in place):** [../phase-3-interactive-error-recovery/feature-implementation-plan.md](../phase-3-interactive-error-recovery/feature-implementation-plan.md)

**Parent specification decisions Phase 4 inherits unchanged:** [parent D5](../artifacts/decision-log.md#d5-mirror-key-state-into-cmux-sidebar) (mirror step + iteration only), [parent D15](../artifacts/decision-log.md#d15-cmux-api-per-call-timeout-is-fatal) (per-call timeout is fatal), [parent D17](../artifacts/decision-log.md#d17-log-file-artifacts-are-unchanged-in-cmux-mode) (log artifacts unchanged), [parent D-R1](../artifacts/decision-log.md#d-r1-orchestrator-is-the-in-pane-pr9k-process-no-hidden-orchestrator-pane-supersedes-d13d-3d-4-hidden-pane-parts), [parent D-R2](../artifacts/decision-log.md#d-r2-cmux-v2-protocol-is-the-contract-of-record).

**Phase-specific spec decisions Phase 4 implements:** spec [D1](artifacts/decision-log.md#d1-sidebar-entries-map-to-cmuxs-status-pill-and-progress-bar-surfaces), [D2](artifacts/decision-log.md#d2-pr9k-owns-its-sidebar-entries-under-a-stable-pr9k-prefixed-key), [D3](artifacts/decision-log.md#d3-progress-entry-uses-cmuxs-fraction-plus-label-progress-surface), [D4](artifacts/decision-log.md#d4-unbounded-iteration-count-suppresses-the-progress-entry-the-status-entry-continues-unchanged), [D5](artifacts/decision-log.md#d5-non-timeout-sidebar-errors-are-logged-and-the-run-continues-timeouts-remain-fatal-per-parent-d15), [D6](artifacts/decision-log.md#d6-sidebar-entries-are-cleared-on-every-graceful-run-end-path), [D7](artifacts/decision-log.md#d7-progress-entry-is-cleared-at-the-end-of-the-iteration-loop-status-entry-continues-through-finalization), [D13](artifacts/decision-log.md#d13-error-mode-suffixes-a-stable-marker-onto-the-status-entry-value); trivial D8–D17.

## Outcome

When this plan executes, the codebase contains:

- A new `cmuxSidebar` struct (in a new `src/cmd/pr9k/cmux_sidebar.go`) that owns a `cmuxctl.CmuxClient`, a `cmuxctl.Workspace` handle, a `*logger.Logger`, and caches `lastStepName` plus `progressPushed` ([D-1](artifacts/implementation-decision-log.md#d-1-sidebar-adapter-is-a-separate-cmuxsidebar-struct-plus-a-sidebarawareheader-wrapper-composed-alongside-cmuxheader)).
- A new `sidebarAwareHeader` wrapper in the same file that satisfies `workflow.RunHeader`, delegates to `*cmuxHeader` for header concerns, and observes the existing call sequence to drive `cmuxSidebar` ([D-1](artifacts/implementation-decision-log.md#d-1-sidebar-adapter-is-a-separate-cmuxsidebar-struct-plus-a-sidebarawareheader-wrapper-composed-alongside-cmuxheader)).
- A `nameAt(idx int) string` accessor added to `cmuxHeader` so the wrapper can resolve step name from index on `SetStepState` ([D-14](artifacts/implementation-decision-log.md#trivial-decisions)).
- Four new methods on `cmuxctl.CmuxClient`: `WorkspaceSetStatus`, `WorkspaceClearStatus`, `WorkspaceSetProgress`, `WorkspaceClearProgress` ([D-3](artifacts/implementation-decision-log.md#d-3-cmuxclient-gains-four-new-methods-directly-no-cmuxsidebar-sub-interface)); matching wrappers in `cmuxctl.RealClient`; matching `Func` fields + recorder slices in `cmuxctl.FakeClient` ([D-12](artifacts/implementation-decision-log.md#trivial-decisions)).
- `cmuxctl.CmuxClient` and `cmuxctl.Workspace` threaded through `cmuxOrchestratorHooks → runCmuxOrchestratorWith → runCmuxWorkflowAdapted`; the existing `_ Workspace` discard at `src/cmd/pr9k/cmux_pane.go:46` becomes a captured parameter ([D-5](artifacts/implementation-decision-log.md#d-5-cmuxctlcmuxclient-and-cmuxctlworkspace-are-threaded-through-the-orchestrator-composition-root-from-cmuxorchestratorhooks--runcmuxorchestratorwith--runcmuxworkflowadapted)).
- The existing `keyHandler.SetOnModeChange` closure in `runCmuxWorkflowAdapted` is augmented in place to call `sidebar.EnterErrorMode(ctx)` when `mode == ui.ModeError`, without a second `SetOnModeChange` call ([D-2](artifacts/implementation-decision-log.md#d-2-error-mode-marker-fires-by-augmenting-the-existing-onmodechange-closure-in-place-reading-from-a-cached-step-name)).
- `cmuxSidebar` construction hoisted to `runCmuxOrchestratorWith` with `defer sidebar.ClearAll(context.Background())` immediately after construction; the inner `runCmuxWorkflowAdapted` calls `sidebar.ClearAll(ctx)` on graceful exit before `keyCancel()` and before `WorkspaceDone` is broadcast ([D-6](artifacts/implementation-decision-log.md#d-6-cleanup-ordering--sidebarclearallctx-runs-in-runcmuxworkflowadapted-immediately-after-workflowrun-returns-using-the-parent-context), [D-7](artifacts/implementation-decision-log.md#d-7-cmuxsidebar-construction-is-hoisted-to-runcmuxorchestratorwith-a-defer-sidebarclearallcontextbackground-runs-on-every-exit-path)).
- Cmux version pin bumped from 0.64.6 to 0.64.7 in `src/internal/cmuxctl/client.go` package docstring, `src/internal/cmuxctl/preflight.go:65` (the unsupported-version error string), and `docs/how-to/setting-up-cmux.md:5` ([D-4](artifacts/implementation-decision-log.md#d-4-capability-check-stays-identify-only-the-cmux-version-pin-moves-0646--0647)).
- Documentation: a new sidebar-mirroring section in `docs/features/cmux-mode.md`; an extended subsection in `docs/how-to/setting-up-cmux.md` covering what an operator monitoring from another workspace sees and the error-mode marker behavior; four new RPC entries in `docs/code-packages/cmuxctl.md` ([D-11](artifacts/implementation-decision-log.md#trivial-decisions)).
- Version bump `0.12.0 → 0.13.0` ([D-10](artifacts/implementation-decision-log.md#trivial-decisions)).

When operators run `pr9k --cmux --project-dir <repo>` from inside a cmux session against a project with a real workflow configured, they observe the Phase 4 demo: the workspace appears (Phases 1–2); the first step starts and a status pill named after that step appears in cmux's sidebar against the pr9k workspace's row; if `-n M` was supplied, a progress entry shows `1 / M`; switching to a different cmux workspace, the operator continues to see the pr9k row's pill update with each step transition and the progress bar advance with each iteration; if a step fails into error mode, the pill text gains ` — awaiting input` until the operator resolves the continue/retry/quit prompt; when the iteration loop ends and finalization begins, the progress entry clears while the pill keeps tracking the finalization step; on graceful run end the pill and progress entry both clear from the pr9k workspace's row.

## Context

- **Driving constraint:** without Phase 4, the operator who switched away from the pr9k workspace mid-run has no in-cmux signal that pr9k is making progress. Phase 4 is the smallest demoable slice of the "monitor a workspace from elsewhere" benefit; Phase 5 then closes the unattended-error-prompt attention gap via cmux notifications, and Phase 6 generalizes failure-mode handling (sidebar-clear from abort paths, display-loss, channel stalls).
- **Stakeholders:**
  - **Operators on macOS (cmux's primary platform)** — care that the sidebar pill updates promptly, the progress entry advances honestly, the error-mode marker is a stable visual signal, and the sidebar clears cleanly when the run ends.
  - **pr9k maintainers** — care that the four new RPC methods integrate cleanly with the existing `cmuxctl` queue, that `cmuxHeader`'s responsibilities are not bloated with sidebar concerns, and that Phase 6's abort-path work can reuse the same `ClearAll` without modifying Phase 4 internals.
- **Future-state concern:** Phase 6 (failure-mode hardening) calls `sidebar.ClearAll` from the abort path per spec [D17](artifacts/decision-log.md#d17-phase-6-integrates-with-phase-4-by-calling-the-same-sidebar-clear-operations-no-new-interface-is-required). The hoisted construction in `runCmuxOrchestratorWith` plus the outer `defer` ([D-7](artifacts/implementation-decision-log.md#d-7-cmuxsidebar-construction-is-hoisted-to-runcmuxorchestratorwith-a-defer-sidebarclearallcontextbackground-runs-on-every-exit-path)) is the OCP extension point. Phase 5 (cmux notifications) adds an interruptive surface for the same lifecycle events the sidebar mirrors passively — the cached `lastStepName` on `cmuxSidebar` may be reused by Phase 5 if a third effect needs the active step name; if a fourth effect on mode change appears in Phase 5 that does not compose cleanly into the augmented closure, the closure may need to be refactored into a slice of hooks, but that is out of Phase 4's scope.
- **Out-of-scope boundary:** sidebar log surface ([spec Out of Scope](feature-specification.md#out-of-scope)); failure-specific sidebar *decoration* (icon/color/priority — spec D12 + Out of Scope); operator-configurable sidebar styling; sidebar mirroring of the status-line text (deferred at parent spec); mirroring sidebar state into on-disk log artifacts; non-graceful run-end cleanup (Phase 6); per-method `system.capabilities` enumeration (Deferred (YAGNI) — see below).

## Team Composition and Participation

| Specialist | Status | Key Input |
|------------|--------|-----------|
| `project-manager` | Coordinator | Aggregated R1 deterministically (gate did not trip; no PM facilitation pass was triggered); applied the YAGNI rule at Step 7.5; synthesized this plan and the decision log. |
| `software-architect` | Active (R1) | Eight findings (S1–S8) covering sidebar adapter placement, error-mode marker mechanism, iteration→finalize detection, CmuxClient interface extension, capability check resolution, workspace-handle/client threading, cleanup ordering, and Phase 6 forward compatibility. |
| `junior-developer` | Active (R1) | Twelve findings (F1–F12) covering the discarded workspace handle, the missing client thread, the single-field `OnModeChange` constraint, the three-location version pin conflict, the capability-check gap, the `pr9k.step` key choice, the standards-vs-precedent version-bump question, em-dash precedent, doc-surface location, FakeClient extension, signature-cascade impact, and the D7 simpler-version observation. |
| `user-experience-designer` | Not engaged (this plan) | UX contribution already landed at spec stage (D13 — the error-mode marker). No new operator-facing surface beyond what the spec settled. |
| `adversarial-security-analyst` | Not engaged (this plan) | No new auth / PII / untrusted-input / secrets path. Same socket transport, same trust model as Phases 1–3. |
| `devops-engineer` | Not engaged (this plan) | No new infrastructure component, no SLO surface, no rollout machinery beyond the version bump. |
| `data-engineer` | Not engaged (this plan) | No schema change, no migration, no persistent data — log artifacts unchanged per spec D11. |
| `structural-analyst` / `behavioral-analyst` / `concurrency-analyst` | Not engaged (this plan) | `software-architect` covered the SOLID, snapshot-then-unlock, and OnModeChange-composition analysis within scope; no upstream-finding swarm needed. |
| `test-engineer` / `edge-case-explorer` | Not engaged (this plan) | The test surface is "extend `FakeClient`, extend the existing adapter test pattern, verify call ordering via recorder slices" — the plan specifies this directly without dispatching a specialist. |
| `system-architect` | Not engaged (this plan) | Phase 4 is entirely intra-process; no bounded-context split, no cross-service integration. |
| `risk-analyst` | Not engaged (this plan) | RAID items are individually scoped; no portfolio-level prioritization needed for a small phase. |

## Implementation Approach

### Architecture and Integration Points

**New file: `src/cmd/pr9k/cmux_sidebar.go`.** Introduces two types ([D-1](artifacts/implementation-decision-log.md#d-1-sidebar-adapter-is-a-separate-cmuxsidebar-struct-plus-a-sidebarawareheader-wrapper-composed-alongside-cmuxheader)):

- `cmuxSidebar` — owns the cmux RPC client and workspace handle. Methods: `PushStep(ctx, name)`, `PushProgress(ctx, iter, maxIter)`, `EnterErrorMode(ctx)`, `ClearProgress(ctx)`, `ClearAll(ctx)`, plus an internal `pushStatus(ctx, value)`. Mutex-protects `lastStepName` and `progressPushed` per `docs/coding-standards/concurrency.md`. Non-timeout errors are logged via `*logger.Logger` and swallowed (spec D5); timeouts propagate fatal (parent D15). The literal status key is the typed `const sidebarStatusKey = "pr9k.step"` ([D-8](artifacts/implementation-decision-log.md#trivial-decisions)); the error-mode marker literal is `" — awaiting input"` using U+2014 ([D-9](artifacts/implementation-decision-log.md#trivial-decisions)).
- `sidebarAwareHeader` — wraps `*cmuxHeader`; satisfies `workflow.RunHeader` ([D-1](artifacts/implementation-decision-log.md#d-1-sidebar-adapter-is-a-separate-cmuxsidebar-struct-plus-a-sidebarawareheader-wrapper-composed-alongside-cmuxheader)); each method delegates to `cmuxHeader` first, then invokes the matching `cmuxSidebar` call. `RenderFinalizeLine` observes a one-shot `finalizeBegun` flag and issues `sidebar.ClearProgress` exactly once on the first call ([D-13](artifacts/implementation-decision-log.md#trivial-decisions)); `SetStepState(idx, StepActive)` issues `sidebar.PushStep(cmuxHeader.nameAt(idx))`.

**Modification: `src/cmd/pr9k/cmux_workflow.go`.** Three changes:

1. `cmuxHeader` gains a `nameAt(idx int) string` accessor that returns `h.names[idx]` under `h.mu`, or `""` if `idx` is out of bounds ([D-14](artifacts/implementation-decision-log.md#trivial-decisions)). No change to the existing methods.
2. `runCmuxWorkflowAdapted` gains two parameters — `client cmuxctl.CmuxClient` and `ws cmuxctl.Workspace` (signature now `(ctx, ch, log, projectDir, workflowDir, sf, sidebar)`, with `sidebar *cmuxSidebar` passed from the outer caller per [D-7](artifacts/implementation-decision-log.md#d-7-cmuxsidebar-construction-is-hoisted-to-runcmuxorchestratorwith-a-defer-sidebarclearallcontextbackground-runs-on-every-exit-path)) ([D-5](artifacts/implementation-decision-log.md#d-5-cmuxctlcmuxclient-and-cmuxctlworkspace-are-threaded-through-the-orchestrator-composition-root-from-cmuxorchestratorhooks--runcmuxorchestratorwith--runcmuxworkflowadapted)). It constructs the `sidebarAwareHeader` wrapper around `cmuxHeader` and the passed-in `*cmuxSidebar`, and passes the wrapper to `workflow.Run` as the `RunHeader`. The existing `keyHandler.SetOnModeChange` closure is augmented in place to call `sidebar.EnterErrorMode(ctx)` when `mode == ui.ModeError` ([D-2](artifacts/implementation-decision-log.md#d-2-error-mode-marker-fires-by-augmenting-the-existing-onmodechange-closure-in-place-reading-from-a-cached-step-name)). The graceful-path `sidebar.ClearAll(ctx)` call is added immediately after `workflow.Run` returns and after `keyHandler.SetMode(ui.ModeDone)`, before `keyCancel()` and `wg.Wait()` ([D-6](artifacts/implementation-decision-log.md#d-6-cleanup-ordering--sidebarclearallctx-runs-in-runcmuxworkflowadapted-immediately-after-workflowrun-returns-using-the-parent-context)).
3. `runCmuxOrchestratorWith` (in `src/cmd/pr9k/cmux_pane.go`) gains parameters `client cmuxctl.CmuxClient` and `ws cmuxctl.Workspace`. After it captures the client/workspace, it constructs `*cmuxSidebar` and registers `defer sidebar.ClearAll(context.Background())` ([D-7](artifacts/implementation-decision-log.md#d-7-cmuxsidebar-construction-is-hoisted-to-runcmuxorchestratorwith-a-defer-sidebarclearallcontextbackground-runs-on-every-exit-path)); the sidebar pointer is passed into `runCmuxWorkflowAdapted`.

**Modification: `src/cmd/pr9k/cmux_pane.go`.** Three changes:

1. The `cmuxOrchestratorHooks` factory gains a `client cmuxctl.CmuxClient` parameter (called from `main.go` at the `--cmux` branch where the production `*RealClient` exists).
2. The `Phase1Hooks.Run` closure at line 46 (`func(rctx, _ Workspace)`) replaces the `_` with the named `ws` parameter and threads it forward to `runCmuxOrchestratorWith` ([D-5](artifacts/implementation-decision-log.md#d-5-cmuxctlcmuxclient-and-cmuxctlworkspace-are-threaded-through-the-orchestrator-composition-root-from-cmuxorchestratorhooks--runcmuxorchestratorwith--runcmuxworkflowadapted)).
3. `runCmuxOrchestratorWith`'s signature change cascades into existing test files (`cmux_orchestrator_test.go`, `cmux_pane_test.go`, `cmux_u9_test.go`, `cmux_error_recovery_test.go`); these tests pass a `*cmuxctl.FakeClient` and a synthesized `cmuxctl.Workspace{ID: "ws:test"}` to the new parameters.

**Modification: `src/internal/cmuxctl/client.go`.** Add four method signatures to the `CmuxClient` interface ([D-3](artifacts/implementation-decision-log.md#d-3-cmuxclient-gains-four-new-methods-directly-no-cmuxsidebar-sub-interface)):

```go
WorkspaceSetStatus(ctx context.Context, ws Workspace, key, value string) error
WorkspaceClearStatus(ctx context.Context, ws Workspace, key string) error
WorkspaceSetProgress(ctx context.Context, ws Workspace, fraction float64, label string) error
WorkspaceClearProgress(ctx context.Context, ws Workspace) error
```

The package docstring's "cmux 0.64.6" reference moves to "cmux 0.64.7" ([D-4](artifacts/implementation-decision-log.md#d-4-capability-check-stays-identify-only-the-cmux-version-pin-moves-0646--0647)); the commit hash inside the docstring is bumped to the corresponding cmux 0.64.7 commit at implementation time.

**Modification: `src/internal/cmuxctl/real.go`.** Add four thin wrappers over `c.do(ctx, method, params)`. The JSON-RPC wire method names (e.g., `workspace.set_status` vs. `workspace.setStatus`) must be confirmed against the cmux 0.64.7 socket protocol at implementation time; the CLI verbs (`set-status`, `clear-status`, `set-progress`, `clear-progress`) are confirmed by the cmux CLI contract on `main`. Each wrapper passes the workspace ID via the existing `workspace_id` parameter convention used by `WorkspaceClose` and `WorkspaceSelect`.

**Modification: `src/internal/cmuxctl/fake.go`.** Add four `Func` fields and four recorder slices ([D-12](artifacts/implementation-decision-log.md#trivial-decisions)):

```go
WorkspaceSetStatusFunc      func(ctx context.Context, ws Workspace, key, value string) error
WorkspaceClearStatusFunc    func(ctx context.Context, ws Workspace, key string) error
WorkspaceSetProgressFunc    func(ctx context.Context, ws Workspace, fraction float64, label string) error
WorkspaceClearProgressFunc  func(ctx context.Context, ws Workspace) error
SetStatusCalls    []SetStatusCall    // ordered record for test ordering assertions
ClearStatusCalls  []ClearStatusCall
SetProgressCalls  []SetProgressCall
ClearProgressCalls []Workspace
```

The recorder slices are protected by the existing `FakeClient` mutex; the compile-time assertion `var _ CmuxClient = (*FakeClient)(nil)` (already present) becomes the compile gate that fails the build if any of the four implementations is missing.

**Modification: `src/internal/cmuxctl/preflight.go`.** Line 65 (the unsupported-version error string) bumps `v0.64.6` to `v0.64.7` ([D-4](artifacts/implementation-decision-log.md#d-4-capability-check-stays-identify-only-the-cmux-version-pin-moves-0646--0647)). Preflight otherwise unchanged — no per-method enumeration is added.

### Data Model and Persistence

Phase 4 introduces no new persistent state, no schema change, and no on-disk artifact. The sidebar entries are ephemeral state in cmux's process memory ([T1](artifacts/feature-technical-notes.md#t1-cmux-sidebar-surface-shape) — latest-wins per key, one-progress-per-workspace). The on-disk log artifacts at `<projectDir>/.pr9k/logs/<run-stamp>/` remain unchanged from Phase 2 (parent [D17](../artifacts/decision-log.md#d17-log-file-artifacts-are-unchanged-in-cmux-mode)).

`cmuxSidebar`'s in-memory cache (`lastStepName`, `progressPushed`) is per-run state, allocated at orchestrator startup and released when the orchestrator process exits. Concurrency: a single `sync.Mutex` protects both fields; reads are short and snapshot-then-unlock per `docs/coding-standards/concurrency.md`.

### Runtime Behavior

**Launch sequence (extending Phase 2/3 lifecycle):**

1. `pr9k --cmux --project-dir <repo>` proceeds through Phase 1 preflight (identify-only, with the bumped 0.64.7 error string per [D-4](artifacts/implementation-decision-log.md#d-4-capability-check-stays-identify-only-the-cmux-version-pin-moves-0646--0647)).
2. `RunPhase1` creates the workspace and spawns the four pane processes (unchanged from Phase 1/2).
3. The orchestrator pane's `cmuxOrchestratorHooks.Run` closure captures `ws cmuxctl.Workspace` (no longer discarded) and the production `CmuxClient` and calls `runCmuxOrchestratorWith(ctx, socketPath, projectDir, ackTimeout, ch, client, ws)`.
4. `runCmuxOrchestratorWith` constructs `*cmuxSidebar` and registers `defer sidebar.ClearAll(context.Background())` ([D-7](artifacts/implementation-decision-log.md#d-7-cmuxsidebar-construction-is-hoisted-to-runcmuxorchestratorwith-a-defer-sidebarclearallcontextbackground-runs-on-every-exit-path)) — between launch and the first step transition the pr9k workspace's sidebar row carries no pr9k entries ([spec D16](artifacts/decision-log.md#trivial-decisions)).
5. `runCmuxWorkflowAdapted` constructs `sidebarAwareHeader{inner: newCmuxHeader(ch), sidebar: sidebar, ctx: ctx}` and passes it to `workflow.Run` as `RunHeader`. The augmented `OnModeChange` closure is registered before `workflow.Run` starts ([D-2](artifacts/implementation-decision-log.md#d-2-error-mode-marker-fires-by-augmenting-the-existing-onmodechange-closure-in-place-reading-from-a-cached-step-name)).
6. The first `RenderInitializeLine` (or, for workflows without an initialize phase, the first `RenderIterationLine` followed by `SetStepState(0, StepActive)`) fires — the wrapper issues `sidebar.PushStep(stepName)` and `sidebar.PushProgress(1, M)` if `cfg.Iterations > 0` per [spec D4](artifacts/decision-log.md#d4-unbounded-iteration-count-suppresses-the-progress-entry-the-status-entry-continues-unchanged).

**Per-event sidebar push cadence (spec D10):**

- `RenderInitializeLine(stepNum, stepCount, stepName)` → `sidebar.PushStep(stepName)`.
- `RenderIterationLine(iter, maxIter, _)` → `sidebar.PushProgress(iter, maxIter)` (no-op when `maxIter <= 0` per [spec D4](artifacts/decision-log.md#d4-unbounded-iteration-count-suppresses-the-progress-entry-the-status-entry-continues-unchanged)).
- `SetStepState(idx, ui.StepActive)` (iteration phase) → `sidebar.PushStep(cmuxHeader.nameAt(idx))`.
- First `RenderFinalizeLine(stepNum, stepCount, stepName)` (first finalize call only) → `sidebar.ClearProgress(ctx)` (the natural per-iteration push cadence has already left the terminal value `K/M` in cmux's state — D7 is satisfied by clearing only, no additional terminal push per [D-13](artifacts/implementation-decision-log.md#trivial-decisions)); every subsequent `RenderFinalizeLine` → `sidebar.PushStep(stepName)`.
- `KeyHandler.SetMode(ui.ModeError)` → augmented closure → `sidebar.EnterErrorMode(ctx)` → pushes `lastStepName + " — awaiting input"` to the status pill ([D-2](artifacts/implementation-decision-log.md#d-2-error-mode-marker-fires-by-augmenting-the-existing-onmodechange-closure-in-place-reading-from-a-cached-step-name), [D-9](artifacts/implementation-decision-log.md#trivial-decisions)).
- `KeyHandler.SetMode(ui.ModeNormal)` (operator resolved error prompt) → no sidebar action; the next normal step push (continue → next step's `SetStepState`; retry → re-entered `SetStepState(idx, StepActive)`; quit → `ClearAll` from graceful teardown) overwrites the marker naturally.

**Completion sequence (graceful):**

1. `workflow.Run` returns inside `runCmuxWorkflowAdapted`.
2. `keyHandler.SetMode(ui.ModeDone)` fires (drops the error-mode marker if still set, via the next step push — but at this point there is no next push; the sidebar pill still carries whatever value was last pushed).
3. `sidebar.ClearAll(ctx)` issues two clears: `WorkspaceClearStatus(ctx, ws, sidebarStatusKey)` and (if `progressPushed`) `WorkspaceClearProgress(ctx, ws)` ([D-6](artifacts/implementation-decision-log.md#d-6-cleanup-ordering--sidebarclearallctx-runs-in-runcmuxworkflowadapted-immediately-after-workflowrun-returns-using-the-parent-context)).
4. `keyCancel()`, `wg.Wait()`, then `runCmuxWorkflowAdapted` returns the exit code.
5. `runCmuxOrchestratorWith` broadcasts `WorkspaceDone{ExitCode: ...}` and awaits acks (unchanged from Phase 2).
6. The outer `defer sidebar.ClearAll(context.Background())` runs again on function return — best-effort safety net for panic / early-return paths ([D-7](artifacts/implementation-decision-log.md#d-7-cmuxsidebar-construction-is-hoisted-to-runcmuxorchestratorwith-a-defer-sidebarclearallcontextbackground-runs-on-every-exit-path)). The second clear is a no-op against already-cleared cmux state (latest-wins-per-key semantics per [T1](artifacts/feature-technical-notes.md#t1-cmux-sidebar-surface-shape)); non-timeout errors from the no-op are logged-and-continued per [spec D5](artifacts/decision-log.md#d5-non-timeout-sidebar-errors-are-logged-and-the-run-continues-timeouts-remain-fatal-per-parent-d15).

**Error paths:**

- **Non-timeout sidebar error** (workspace handle rejected, malformed value, transient cmux error): logged to the per-run log file under `<projectDir>/.pr9k/logs/<stamp>/`; workflow continues. Per [spec D5](artifacts/decision-log.md#d5-non-timeout-sidebar-errors-are-logged-and-the-run-continues-timeouts-remain-fatal-per-parent-d15).
- **Sidebar call timeout:** treated as fatal per [parent D15](../artifacts/decision-log.md#d15-cmux-api-per-call-timeout-is-fatal); triggers the same abort path display-loss takes. The outer `defer sidebar.ClearAll(context.Background())` ([D-7](artifacts/implementation-decision-log.md#d-7-cmuxsidebar-construction-is-hoisted-to-runcmuxorchestratorwith-a-defer-sidebarclearallcontextbackground-runs-on-every-exit-path)) still runs on the abort path; if it also times out, the orphan-workspace mechanism (Phase 7) is the operator's fallback.
- **`cmuxctl.Workspace` is zero-value** (cmux returned an empty handle from `WorkspaceCreate`): `cmuxSidebar` skips all RPCs and logs a single warning at construction. The workflow runs without sidebar mirroring — this is a degenerate case not previously expected; Phase 6 hardens detection.

### External Interfaces

**CLI.** No new flag, no new sub-command, no change to existing flag behavior.

**Cmux JSON-RPC 2.0.** Four new methods called by pr9k against cmux 0.64.7's socket: `workspace.set_status` (status pill set), `workspace.clear_status` (status pill remove), `workspace.set_progress` (progress bar set), `workspace.clear_progress` (progress bar clear). The wire-level method names are confirmed at implementation time against the cmux 0.64.7 protocol; the Go method names on `CmuxClient` are fixed per [D-3](artifacts/implementation-decision-log.md#d-3-cmuxclient-gains-four-new-methods-directly-no-cmuxsidebar-sub-interface). All calls obey [parent D15](../artifacts/decision-log.md#d15-cmux-api-per-call-timeout-is-fatal)'s per-call wall-clock timeout.

**Cmux sidebar (operator-visible).** A status pill keyed `pr9k.step` ([D-8](artifacts/implementation-decision-log.md#trivial-decisions)) and a single progress bar (one-per-workspace per [T1](artifacts/feature-technical-notes.md#t1-cmux-sidebar-surface-shape)) appear in the pr9k workspace's sidebar row. The status pill's value transitions between the active step name (normal) and the active step name suffixed with `" — awaiting input"` (error mode per [D-9](artifacts/implementation-decision-log.md#trivial-decisions)). The progress bar's fraction is `N / M` and its label is the human-readable `"N / M"` (spec D3). Both clear on every graceful run-end path (spec D6).

**Operator-visible terminal output (inside the pr9k workspace).** Unchanged from Phase 3. The header / log / footer panes render the same content; the sidebar is a parallel projection.

## Decomposition and Sequencing

| # | Work Unit | Delivers | Depends On | Verification |
|---|-----------|----------|------------|--------------|
| W1 | Cmux version pin bump 0.64.6 → 0.64.7 in three locations (`src/internal/cmuxctl/client.go` package docstring, `src/internal/cmuxctl/preflight.go:65` error string, `docs/how-to/setting-up-cmux.md:5` Tested-against line); update the package-docstring commit hash to the matching cmux 0.64.7 commit ([D-4](artifacts/implementation-decision-log.md#d-4-capability-check-stays-identify-only-the-cmux-version-pin-moves-0646--0647)) | All three version-pin references match; preflight error message names 0.64.7 | — | `make build`, `make vet`, `grep -rn "0.64.6" src/ docs/` returns no production hits |
| W2 | `CmuxClient` interface extension: four new method signatures on `cmuxctl.CmuxClient`; matching `RealClient` wrappers (verifying wire method names against cmux 0.64.7 source); `FakeClient` extension with four `Func` fields and four recorder slices ([D-3](artifacts/implementation-decision-log.md#d-3-cmuxclient-gains-four-new-methods-directly-no-cmuxsidebar-sub-interface), [D-12](artifacts/implementation-decision-log.md#trivial-decisions)) | Production and test clients implement the four sidebar RPCs uniformly | W1 | Unit tests: each `RealClient` wrapper exercises a round-trip against a `httptest`-style cmux fake socket using the existing `runphase1_test.go` pattern; `FakeClient` recorder slices record call order for each method; `var _ CmuxClient = (*FakeClient)(nil)` and `var _ CmuxClient = (*RealClient)(nil)` both compile |
| W3 | `cmuxSidebar` + `sidebarAwareHeader` types in a new `src/cmd/pr9k/cmux_sidebar.go`; `nameAt(idx)` accessor on `cmuxHeader` in `cmux_workflow.go`; thread `CmuxClient` + `Workspace` through `cmuxOrchestratorHooks` → `runCmuxOrchestratorWith` → `runCmuxWorkflowAdapted` (signature changes); augment the existing `SetOnModeChange` closure to fire `sidebar.EnterErrorMode(ctx)` on `ModeError`; add `sidebar.ClearAll(ctx)` after `workflow.Run` returns; hoist `*cmuxSidebar` construction to `runCmuxOrchestratorWith` and register `defer sidebar.ClearAll(context.Background())` ([D-1](artifacts/implementation-decision-log.md#d-1-sidebar-adapter-is-a-separate-cmuxsidebar-struct-plus-a-sidebarawareheader-wrapper-composed-alongside-cmuxheader), [D-2](artifacts/implementation-decision-log.md#d-2-error-mode-marker-fires-by-augmenting-the-existing-onmodechange-closure-in-place-reading-from-a-cached-step-name), [D-5](artifacts/implementation-decision-log.md#d-5-cmuxctlcmuxclient-and-cmuxctlworkspace-are-threaded-through-the-orchestrator-composition-root-from-cmuxorchestratorhooks--runcmuxorchestratorwith--runcmuxworkflowadapted), [D-6](artifacts/implementation-decision-log.md#d-6-cleanup-ordering--sidebarclearallctx-runs-in-runcmuxworkflowadapted-immediately-after-workflowrun-returns-using-the-parent-context), [D-7](artifacts/implementation-decision-log.md#d-7-cmuxsidebar-construction-is-hoisted-to-runcmuxorchestratorwith-a-defer-sidebarclearallcontextbackground-runs-on-every-exit-path), [D-13](artifacts/implementation-decision-log.md#trivial-decisions), [D-14](artifacts/implementation-decision-log.md#trivial-decisions)) | The sidebar adapter compiles, integrates with the orchestrator, and pushes/clears at the right events | W2 | Unit tests using `FakeClient`: `sidebarAwareHeader.RenderInitializeLine` produces one `SetStatusCalls` entry; `RenderIterationLine(1, 3, "")` produces one `SetProgressCalls` entry with fraction `1/3` and label `"1 / 3"`; `RenderIterationLine(1, 0, "")` produces zero progress calls; `SetStepState(2, StepActive)` resolves `nameAt(2)` and pushes the corresponding step name; first `RenderFinalizeLine` produces one `ClearProgressCalls` entry and one `SetStatusCalls` entry; subsequent `RenderFinalizeLine` produces only a `SetStatusCalls` entry; `OnModeChange(ModeError, _)` produces a `SetStatusCalls` entry whose value matches the last pushed step name + `" — awaiting input"`; the graceful `ClearAll` produces one `ClearStatusCalls` and one `ClearProgressCalls` entry in that order |
| W4 | Test-suite cascade: update `src/cmd/pr9k/cmux_orchestrator_test.go`, `cmux_pane_test.go`, `cmux_u9_test.go`, `cmux_error_recovery_test.go` for the new `runCmuxOrchestratorWith` and `runCmuxWorkflowAdapted` signatures; tests pass a `*FakeClient` and a `Workspace{ID: "ws:test"}` ([F11](artifacts/implementation-iteration-history.md#claim-ledger)) | Phase 2 / Phase 3 test suite continues to pass under the new signatures; race detector clean | W3 | `cd src && go test -race ./cmd/pr9k/...` passes; the four `FakeClient.*Func` fields can be left nil (default behavior returns nil error) without breaking pre-Phase-4 tests |
| W5 | Documentation: extend `docs/features/cmux-mode.md` (sidebar mirroring + error-mode marker section); extend `docs/how-to/setting-up-cmux.md` (Tested-against → v0.64.7 + monitoring-from-elsewhere subsection); extend `docs/code-packages/cmuxctl.md` (four new RPC wrappers documented) ([D-11](artifacts/implementation-decision-log.md#trivial-decisions)); CLAUDE.md feature/code-package lists do not need new entries (existing files extended) | Operators understand the Phase 4 demo and the from-another-workspace use case; maintainers can navigate the four new RPC wrappers | W2, W3 | `make build` passes; doc code blocks match production code; CLAUDE.md links still resolve |
| W6 | Version bump `0.12.0 → 0.13.0` in `src/internal/version/version.go` ([D-10](artifacts/implementation-decision-log.md#trivial-decisions)) | `pr9k --version` prints `0.13.0`; release-readiness signal | W1–W5 (all surface changes complete) | `make ci` passes (rebuilds binary, runs `src/internal/cli/args_test.go` which reads `version.Version`); `pr9k --version` output matches |

## RAID Log

### Risks

| ID | Risk | Likelihood | Severity | Blast Radius | Reversibility | Owner | Mitigation |
|----|------|------------|----------|--------------|---------------|-------|------------|
| R1 | The JSON-RPC wire method names for `set-status`/`clear-status`/`set-progress`/`clear-progress` differ from the CLI verbs (e.g., `workspace.set_status` vs. `workspace.setStatus` vs. `sidebar.setStatus`); incorrect names produce `method_not_found` errors that spec D5 swallows silently | Medium (cmux's CLI contract documents verbs but not wire names) | Medium (sidebar just doesn't update — operator visibility lost; preflight does not catch it under [D-4](artifacts/implementation-decision-log.md#d-4-capability-check-stays-identify-only-the-cmux-version-pin-moves-0646--0647)) | Cmux mode operators | Reversible (one-line per RPC) | `software-architect` | Verify wire names against cmux 0.64.7 source or the cmux `--help` output at W2 implementation; each `RealClient` wrapper has a round-trip test in W2 that fails loudly if the wire name is wrong |
| R2 | The dot-prefixed key `pr9k.step` is rejected by cmux's key grammar (cmux's own examples use bare identifiers) | Low (no documented restriction; first push during W3 testing reveals it) | Low (one-line constant change to `pr9k_step` + doc update) | Cmux mode operators | Reversible (one constant + one doc string) | `software-architect` | First W3 round-trip test against a real cmux 0.64.7 binary catches it; fallback string `pr9k_step` documented in [D-8](artifacts/implementation-decision-log.md#trivial-decisions) |
| R3 | A Phase 5 third effect on `ModeError` does not compose cleanly into the augmented single closure (the closure becomes long, hard to follow, or hard to test) | Low (Phase 5 adds notifications — a single fire-on-entry call, easy to add) | Low (refactor closure into a slice-of-hooks pattern; cosmetic) | Cmux mode internals | Reversible (one refactor) | `software-architect` (Phase 5) | If Phase 5 needs the refactor, the change is localized to `runCmuxWorkflowAdapted`; the `KeyHandler` API does not need to change |
| R4 | The graceful-path `ClearAll` and the deferred `ClearAll` race against each other under panic-recovery edge cases (the defer fires while the inner clear is still in flight) | Low (the cmux `do()` queue is single-goroutine sequential; two pending calls serialize naturally) | Low (the second call is a no-op; latest-wins per T1) | Cmux mode operators | Reversible (drop the inner ClearAll, rely on defer only) | `software-architect` | Race detector exercises the path during W3 / W4 tests; cmux's serializing queue is the structural mitigation |
| R5 | The `lastStepName` cache is read by the augmented `OnModeChange` closure outside the `cmuxSidebar` mutex if the closure forgets to acquire it; data-race detected under `go test -race` | Low (the closure calls `sidebar.EnterErrorMode(ctx)` which is a `cmuxSidebar` method that acquires the mutex internally) | Low (detected at test time) | `cmuxSidebar` only | Reversible (mutex annotation) | `software-architect` | Code review of [D-1](artifacts/implementation-decision-log.md#d-1-sidebar-adapter-is-a-separate-cmuxsidebar-struct-plus-a-sidebarawareheader-wrapper-composed-alongside-cmuxheader) implementation; race detector in CI |
| R6 | Phase 6 hardening discovers the `defer sidebar.ClearAll(context.Background())` runs on more paths than expected, leading to noisy error logs from clears against not-yet-set state | Low (clearing already-cleared sidebar entries is a cmux no-op per T1) | Low (one log line per startup-without-workflow path) | Logs only | Reversible (gate the defer on `sidebar.everPushed`) | `software-architect` (Phase 6) | If reports surface, add a `everPushed` flag on `cmuxSidebar` so `ClearAll` is a true no-op when nothing was ever pushed |

### Assumptions

| ID | Assumption | What Changes If Wrong | Verifier | Status |
|----|------------|-----------------------|----------|--------|
| A1 | Version bump for Phase 4 should be MINOR (`0.13.0`) by Phase 2/3 precedent rather than PATCH (`0.12.1`) by the strict reading of `docs/coding-standards/versioning.md:37`. If the user prefers strict-standard alignment, the version constant is one line to change | Bump becomes `0.12.1`; doc / release-notes wording adjusts; the standard may also be amended to formalize the phase-per-MINOR convention | River at PR review | Unverified — user judgment |
| A2 | Cmux 0.64.7's wire-level JSON-RPC method names for the four sidebar surfaces are stable enough to hard-code in `RealClient` wrappers (i.e., cmux does not version the wire names independently of the version-pin) | If the names diverge across cmux 0.64.7 patch releases, the per-call timeout fatal path fires on every push; preflight does not catch it | `software-architect` at W2 implementation | Unverified — verifiable against the cmux source pin |
| A3 | The cmux 0.64.7 sidebar surface accepts the workspace handle in the same shape (`workspace_id` parameter) as existing `WorkspaceClose` / `WorkspaceSelect` RPCs | If the shape differs, the `RealClient` wrappers need a custom param struct | `software-architect` at W2 | Unverified — verifiable against cmux source |
| A4 | The four sidebar RPCs do not require an authentication token, capability claim, or scope beyond what the existing cmux v2 client already passes during `SystemIdentify` | If a new auth / capability is required, preflight must be extended to acquire / verify it; D-4's identify-only stance breaks | `software-architect` at W2 | Unverified — verifiable against cmux source |
| A5 | `RenderFinalizeLine`'s first-call detection is a sufficient signal for the iteration→finalize transition (i.e., `workflow.Run` always calls `RenderFinalizeLine` at least once when finalization begins) | If the orchestrator's finalization phase can have zero finalize steps (config has no finalize block), the progress bar is never cleared by the wrapper; the deferred `ClearAll` cleans it up at run end | `software-architect` at W3 (inspect `src/internal/workflow/run.go` finalize loop) | Unverified — verifiable during W3 implementation |

### Issues (Currently Active)

| ID | Issue | Owner | Next Step |
|----|-------|-------|-----------|
| I1 | The orchestrator composition root discards both the cmux client and the workspace handle at the call site where Phase 4 needs them (`src/cmd/pr9k/cmux_pane.go:46`) | `software-architect` (per D-5 decision) | Addressed in W3 — `cmuxOrchestratorHooks` is updated to capture `CmuxClient` from `main.go`, and the `Run` closure no longer discards `ws` |
| I2 | `KeyHandler.SetOnModeChange` is a single-field assignment and Phase 3 already occupies it; a second call would overwrite Phase 3's footer push | `software-architect` (per D-2 decision) | Addressed in W3 — the existing closure is augmented in place, not replaced |

### Dependencies (External, Phase-Boundary)

| ID | Dependency | Phase | Notes |
|----|------------|-------|-------|
| Dep1 | Phase 2's `cmuxHeader`, `runCmuxWorkflowAdapted`, `interactionchannel.Channel` are landed and stable | Phase 2 | Phase 4 extends `runCmuxWorkflowAdapted`'s signature and adds a `nameAt` accessor to `cmuxHeader` — both are additive changes |
| Dep2 | Phase 3's `KeyHandler.SetOnModeChange` callback infrastructure is landed and behaves as documented | Phase 3 | Phase 4 augments the existing closure registered by Phase 3; the registration point in `runCmuxWorkflowAdapted` is the integration site |
| Dep3 | Cmux v0.64.7 is the operator's installed version | Operator | Preflight error names 0.64.7; the how-to documents the minimum version |
| Dep4 | Phase 6 (failure-mode hardening) will call `sidebar.ClearAll` from its abort path | Phase 6 (future) | The hoisted construction + outer defer ([D-7](artifacts/implementation-decision-log.md#d-7-cmuxsidebar-construction-is-hoisted-to-runcmuxorchestratorwith-a-defer-sidebarclearallcontextbackground-runs-on-every-exit-path)) is the integration point; no new interface is required from Phase 4 |

## Testing Strategy

Test surface follows the Phase 2 / Phase 3 precedent: extend the existing `FakeClient` and adapter-test patterns; no new test harness, no live-cmux integration tests in this plan.

- **`cmuxctl.RealClient` round-trip tests (W2):** each of the four new RPC wrappers exercises a request / response cycle against a fake cmux socket using the existing `runphase1_test.go` pattern. Verifies the wire method names and the parameter shape against cmux 0.64.7.
- **`cmuxctl.FakeClient` recorder tests (W2):** each new `Func` field and recorder slice has a focused unit test verifying calls are recorded in order.
- **`cmuxSidebar` unit tests (W3):** with a `*FakeClient`, exercise each method (`PushStep`, `PushProgress`, `EnterErrorMode`, `ClearProgress`, `ClearAll`). Verify (a) `PushProgress(iter, 0)` is a no-op (spec D4), (b) non-timeout errors are logged-and-swallowed (spec D5), (c) timeouts propagate as fatal (parent D15), (d) `ClearAll` issues both `ClearStatus` and (if `progressPushed`) `ClearProgress` in order.
- **`sidebarAwareHeader` unit tests (W3):** drive the wrapper through a synthetic call sequence representative of the workflow (initialize step, iteration phase with `-n 3`, finalize phase, error excursion). Verify the recorder slices produce the expected call order: status pushes on every step transition, progress pushes on every iteration counter advance, exactly one `ClearProgress` at the first `RenderFinalizeLine`, exactly one `ClearStatus` + one `ClearProgress` at `ClearAll`.
- **Race detector (W3, W4):** `cd src && go test -race ./cmd/pr9k/... ./internal/cmuxctl/...` is the required gate per `docs/coding-standards/testing.md`.
- **Round-trip integration (W3):** one integration test in `src/cmd/pr9k/cmux_sidebar_test.go` runs a small synthetic workflow through `runCmuxWorkflowAdapted` with a `FakeClient` and `FakeInteractionChannel`, asserts the full call sequence (push step → push progress → first finalize → clear progress → final clear all) matches expectation. This is the single Phase-4 equivalent of Phase 3's W4 round-trip integration test.
- **Test cascade (W4):** existing tests in `src/cmd/pr9k/` updated for the new `runCmuxOrchestratorWith` and `runCmuxWorkflowAdapted` signatures pass without behavioral regression.

**No load-test, no fuzz, no concurrency-stress test required** — the cmux call surface is sequential per the existing `RealClient` queue, and `cmuxSidebar`'s state is short-lived per-run.

## Security Posture

No new attack surface. The new RPC methods use the same socket transport, the same filesystem-permission-based access control, the same `CMUX_SOCKET_PATH` validation, and the same per-call timeout policy as Phases 1–3. No new secret, no new auth path, no untrusted-input parsing. The `sidebarStatusKey` literal `"pr9k.step"` is operator-visible and pinned; no operator-supplied input ever flows into a key string. Step names are written by `config.json` (under the operator's control) — they flow through cmux's value field, which is a free-form short string with no parsing semantics on cmux's side. No further security review needed for this phase.

## Operational Readiness

- **No new SLO** — sidebar is best-effort per spec D5; non-timeout errors are logged-and-continued without operator-visible alarm.
- **No new infrastructure component** — uses the existing cmux socket connection.
- **No new feature flag** — Phase 4 ships as a behavioral extension of `--cmux` mode.
- **No new observability surface** — the existing per-run log file at `<projectDir>/.pr9k/logs/<run-stamp>/` is the diagnostic surface for sidebar failures, per spec D5.
- **Release notes** name the scope: cmux mode now mirrors step + iteration state into cmux's sidebar; error-mode pause is visible from other cmux workspaces via the `— awaiting input` marker; sidebar clears on graceful exit. Cmux version pin moves to 0.64.7. Operators on cmux 0.64.6 must upgrade.
- **Rollback path:** revert the Phase 4 commit; `--cmux` mode reverts to the Phase 3 behavior (no sidebar mirroring). No state migration; no data loss; no operator-visible change beyond the absence of sidebar entries.

## Definition of Done

- [ ] `cmuxctl.CmuxClient` exposes the four new RPC methods; `RealClient` and `FakeClient` implement them; compile-time interface assertions pass.
- [ ] `cmuxSidebar` and `sidebarAwareHeader` types exist in `src/cmd/pr9k/cmux_sidebar.go`; `sidebarAwareHeader` satisfies `workflow.RunHeader` (compile-time assertion).
- [ ] `cmuxctl.CmuxClient` and `cmuxctl.Workspace` are threaded from `main.go` → `cmuxOrchestratorHooks` → `runCmuxOrchestratorWith` → `runCmuxWorkflowAdapted`.
- [ ] Existing `keyHandler.SetOnModeChange` closure in `runCmuxWorkflowAdapted` is augmented in place to call `sidebar.EnterErrorMode(ctx)` on `ModeError`; no second `SetOnModeChange` call.
- [ ] `cmuxSidebar` construction is hoisted to `runCmuxOrchestratorWith`; `defer sidebar.ClearAll(context.Background())` is registered immediately after construction; inner `sidebar.ClearAll(ctx)` runs after `workflow.Run` returns and before `keyCancel()`.
- [ ] Cmux version pin reads `0.64.7` in `src/internal/cmuxctl/client.go`, `src/internal/cmuxctl/preflight.go`, and `docs/how-to/setting-up-cmux.md`.
- [ ] `pr9k.step` is defined as a typed constant in `src/cmd/pr9k/cmux_sidebar.go`.
- [ ] Error-mode marker literal is exactly `" — awaiting input"` (U+2014).
- [ ] `docs/features/cmux-mode.md`, `docs/how-to/setting-up-cmux.md`, `docs/code-packages/cmuxctl.md` updated.
- [ ] `version.Version` is `0.13.0` (or `0.12.1` if A1 is overruled).
- [ ] `cd src && go test -race ./...` passes.
- [ ] `make ci` passes.
- [ ] Manual smoke: launch `pr9k --cmux` against a small real workflow, switch to another cmux workspace, observe the pill update on step transition, observe the progress advance, kill the workflow mid-step to verify the error-mode marker appears, resolve the error to verify it clears, complete the run to verify both entries clear from the sidebar.

## Specialist Handoffs

- **`test-engineer`** — dispatch when W3 begins. Owns the round-trip integration test (`src/cmd/pr9k/cmux_sidebar_test.go`); does not author a new test harness — extends the existing `FakeClient` recorder pattern.
- **`software-architect`** — already engaged at the planning stage (R1); re-engage in Phase 5 if the augmented `OnModeChange` closure needs a slice-of-hooks refactor (R3 trigger), or in Phase 6 if the hoisted construction + defer pattern needs a centralized abort handler.
- **`adversarial-security-analyst`** — not engaged for Phase 4 (no new trust boundary). Reserved for Phase 6 hardening review.
- **`devops-engineer`** — not engaged for Phase 4. Reserved for Phase 6 if cmux SLO observability becomes relevant.

## Deferred (YAGNI)

### Per-method `system.capabilities` enumeration in preflight

- **Why deferred:** spec D15 commits to "the Phase 1 capability check is extended to cover Phase 4's sidebar methods," but the current `Preflight` is identify-only. Adding per-method enumeration over the four new methods plus the eight existing methods is a symmetry/completeness anti-pattern under the YAGNI rule — Phases 1–3 shipped without it, and the version pin alternative ([D-4](artifacts/implementation-decision-log.md#d-4-capability-check-stays-identify-only-the-cmux-version-pin-moves-0646--0647)) satisfies the same evidenced need with strictly less surface.
- **Reopen trigger:** a cmux minor release removes or renames a method pr9k calls and a runtime failure surfaces that the version pin did not prevent.
- **Source:** software-architect S5; junior-developer F5.

### `CmuxSidebar` sub-interface

- **Why deferred:** Rule of Three not met — `cmuxSidebar` is the only production caller and the FakeClient/RealClient are the only implementations. Single-implementation interface is a YAGNI rule named anti-pattern.
- **Reopen trigger:** a third concrete consumer of sidebar-specific RPCs appears in Phase 5 / Phase 6 / future work that is not the `cmuxSidebar` type, OR cmux exposes a new sidebar method whose signature does not fit `CmuxClient`'s shape.
- **Source:** software-architect S4.

### Pill icon / color / priority `opts` parameter on `WorkspaceSetStatus`

- **Why deferred:** spec D12 already defers operator-configurable styling; carrying the parameters in the wire wrapper now would invite premature use.
- **Reopen trigger:** spec D12 reopens (operators report they cannot identify the pr9k pill at a glance when other tools' pills are present).
- **Source:** spec D12 + software-architect S4.

### Two-step push-then-clear sequence at iteration→finalize transition

- **Why deferred:** spec D7 wording suggested a "push terminal value then clear" sequence; re-reading establishes the natural per-iteration push has already left the terminal value visible — only the clear is needed ([D-13](artifacts/implementation-decision-log.md#trivial-decisions)). Simpler-version test.
- **Reopen trigger:** an operator reports the progress bar does not reach `M/M` (or the early-break value) visibly before clearing — a bug in the natural push cadence rather than a missing terminal push.
- **Source:** junior-developer F12.

### New how-to file for "monitoring pr9k from another cmux workspace"

- **Why deferred:** the monitoring-from-elsewhere case is documented in the existing `docs/how-to/setting-up-cmux.md` as a subsection. A standalone how-to would duplicate the cmux-setup context.
- **Reopen trigger:** operators report they cannot find the monitoring documentation when searching, OR Phase 5/6 adds enough cross-workspace mechanics to warrant a unified guide.
- **Source:** junior-developer F9.

## Open Items

- **OI-1 (User input — A1):** Version bump `0.13.0` (MINOR by precedent) vs. `0.12.1` (PATCH by strict reading of `docs/coding-standards/versioning.md:37`). Recommended `0.13.0` per Phase 2/3 precedent; the plan ships at MINOR unless the user overrules at PR review. The constant is one line to change either way.

No other open items. Every Round 1 finding was resolved by evidence, reframing, or implementation choice within the plan's authority.

## Summary

Phase 4 mirrors the running workflow's current step name and iteration progress into cmux's persistent sidebar against the pr9k workspace's row, visible from any other cmux workspace, updated whenever the step changes or the iteration counter advances, with a stable `" — awaiting input"` marker on the status pill during error-mode pause, and cleared cleanly on every graceful run-end path. The implementation adds two new types in a new file (`cmuxSidebar` + `sidebarAwareHeader`), four new methods on the existing `CmuxClient` interface, a minimal `nameAt(idx)` accessor on `cmuxHeader`, an augmented (not replaced) `OnModeChange` closure, a hoisted `*cmuxSidebar` construction with an outer `defer ClearAll`, and a cmux version pin bump from 0.64.6 to 0.64.7. The structural prerequisite — threading the cmux client and workspace handle through `cmuxOrchestratorHooks → runCmuxOrchestratorWith → runCmuxWorkflowAdapted`, where today both are silently discarded — is part of the same work unit. Per-method capability enumeration is deferred under the YAGNI rule's simpler-version test; the version pin satisfies the same evidenced need at a fraction of the implementation surface. One open item (the standards-vs-precedent versioning question) is surfaced for user judgment at PR review.

- **Outcome delivered:** sidebar mirroring + error-mode marker + graceful-path cleanup, as committed by the spec.
- **Actor:** the human operator running pr9k in cmux mode.
- **Decisions settled:** 14 — 7 full ([D-1](artifacts/implementation-decision-log.md#d-1-sidebar-adapter-is-a-separate-cmuxsidebar-struct-plus-a-sidebarawareheader-wrapper-composed-alongside-cmuxheader)–[D-7](artifacts/implementation-decision-log.md#d-7-cmuxsidebar-construction-is-hoisted-to-runcmuxorchestratorwith-a-defer-sidebarclearallcontextbackground-runs-on-every-exit-path)) and 7 trivial ([D-8](artifacts/implementation-decision-log.md#trivial-decisions)–[D-14](artifacts/implementation-decision-log.md#trivial-decisions)) in [implementation-decision-log.md](artifacts/implementation-decision-log.md).
- **Specialists consulted:** `software-architect` (8 findings), `junior-developer` (12 findings). Single round; spec-maturity gate did not trip; PM facilitation pass not triggered.
- **YAGNI deferrals:** 5 — per-method capability enumeration, `CmuxSidebar` sub-interface, pill opts parameter, two-step push-then-clear sequence, standalone monitoring how-to.
- **Open items:** 1 (version bump magnitude — MINOR by precedent vs. PATCH by strict standard reading).
- **Recommendation:** ship as planned. The single open item is resolvable at PR review without blocking implementation.

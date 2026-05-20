# Work Items — Phase 4: Sidebar Mirroring

Vertical-slice work items for [Phase 4 — Sidebar Mirroring](feature-implementation-plan.md). Each work item is independently mergeable: every slice lands a narrow but complete path that is demoable or verifiable on its own (compile, tests pass, recorder-slice assertions hold). Work items are numbered `W-N` for cross-reference only. `Depends on` lines refer to other work items in this file.

The implementation plan organizes the same scope into work units W1–W6; this file refines plan W3 into four thinner slices (W-3 through W-6 below). The result is 9 work items, all AFK (none require an in-meeting decision before merging).

## Shared reference artifacts

These artifacts apply to more than one work item and are cited once here; each work item's `**References.**` block can point into them by anchor.

- **Feature specification** — [feature-specification.md](feature-specification.md) (behaviors every work item realizes; sections: Primary Flow, Alternate Flows, Edge Cases, User Interactions, Coordinations, Out of Scope).
- **Implementation plan** — [feature-implementation-plan.md](feature-implementation-plan.md) (decisions D-1 through D-14, work units W1–W6, RAID, Definition of Done).
- **Feature technical notes T1 — cmux sidebar surface shape** — [artifacts/feature-technical-notes.md](artifacts/feature-technical-notes.md) (latest-wins per key, one-progress-per-workspace, workspace-ID parameter convention).
- **Phase 2 implementation plan** — [../phase-2-real-workflow-runs/feature-implementation-plan.md](../phase-2-real-workflow-runs/feature-implementation-plan.md) (extension point for `cmuxHeader`, `runCmuxWorkflowAdapted`).
- **Phase 3 implementation plan** — [../phase-3-interactive-error-recovery/feature-implementation-plan.md](../phase-3-interactive-error-recovery/feature-implementation-plan.md) (extension point for the `OnModeChange` closure pattern).
- **Cmux CLI contract** — <https://raw.githubusercontent.com/manaflow-ai/cmux/main/docs/cli-contract.md> (CLI verb names `set-status`/`clear-status`/`set-progress`/`clear-progress`; wire-name verification source).
- **Concurrency standard** — [../../coding-standards/concurrency.md](../../coding-standards/concurrency.md) (snapshot-then-unlock; WaitGroup drain; channel-based dispatch; mutex-protected writes).
- **Testing standard** — [../../coding-standards/testing.md](../../coding-standards/testing.md) (`-race` mandatory; fake methods must capture calls; assert triggering conditions occurred; do not hardcode version strings).
- **API-design standard** — [../../coding-standards/api-design.md](../../coding-standards/api-design.md) (adapter types, compile-time interface assertions, precondition validation).
- **Versioning standard** — [../../coding-standards/versioning.md](../../coding-standards/versioning.md) (version constant is single source of truth; `0.y.z` MINOR-vs-PATCH question relevant to W-9).
- **Documentation standard** — [../../coding-standards/documentation.md](../../coding-standards/documentation.md) (feature docs ship with the feature; doc code blocks consistent with production code).
- **`cmuxctl` package reference** — [../../code-packages/cmuxctl.md](../../code-packages/cmuxctl.md) (`FakeClient` recorder pattern; existing `RealClient` queue mechanics).
- **`interactionchannel` package reference** — [../../code-packages/interactionchannel.md](../../code-packages/interactionchannel.md) (`FakeInteractionChannel` contract used by W-7).

## W-1 — Cmux version pin bump 0.64.6 to 0.64.7

**Summary.** Advance the cmux version pin from `0.64.6` to `0.64.7` in the three production locations where the old string appears. The change signals to operators and to the preflight error message that cmux 0.64.7 is the minimum required version for Phase 4's sidebar RPCs. This slice is standalone and verifiable by grep before any other Phase 4 code ships. See plan: W1 and [D-4](feature-implementation-plan.md#implementation-approach).

**Description.**

1. In `src/internal/cmuxctl/client.go`, update the package docstring reference from `cmux 0.64.6` to `cmux 0.64.7` and update the commit hash in the same docstring to the corresponding cmux 0.64.7 commit (look it up from the cmux repository at implementation time).
2. In `src/internal/cmuxctl/preflight.go` line 65, update the unsupported-version error string from `v0.64.6` to `v0.64.7`. Preflight logic is otherwise unchanged — no per-method capability enumeration is added here (deferred under YAGNI per the plan's Deferred section).
3. In `docs/how-to/setting-up-cmux.md` line 5, update the "Tested-against" line from `v0.64.6` to `v0.64.7`.
4. Run `grep -rn "0.64.6" src/ docs/` and verify zero production hits remain.

**References.**

- **Implementation plan** — [feature-implementation-plan.md#decomposition-and-sequencing](feature-implementation-plan.md#decomposition-and-sequencing) (W1) and the D-4 entry in the decision log.
- Files modified: `src/internal/cmuxctl/client.go`, `src/internal/cmuxctl/preflight.go`, `docs/how-to/setting-up-cmux.md`.

**Tests.**

- `make build` and `make vet` pass.
- `grep -rn "0.64.6" src/ docs/` returns zero hits on production files.
- No new unit tests required — the change is three string substitutions.

**Acceptance criteria.**

- [ ] `src/internal/cmuxctl/client.go` package docstring reads `cmux 0.64.7` with the matching commit hash.
- [ ] `src/internal/cmuxctl/preflight.go` unsupported-version error string reads `v0.64.7`.
- [ ] `docs/how-to/setting-up-cmux.md` "Tested-against" line reads `v0.64.7`.
- [ ] `grep -rn "0.64.6" src/ docs/` returns no production hits.
- [ ] `make build` and `make vet` pass.

**Depends on.** `None.`

## W-2 — `CmuxClient` interface extension: four new sidebar RPC methods

**Summary.** Add four new method signatures to the `cmuxctl.CmuxClient` interface, implement them in `RealClient` as thin JSON-RPC wrappers, and extend `FakeClient` with four `Func` fields and four recorder slices. The existing compile-time interface assertions (`var _ CmuxClient = (*FakeClient)(nil)` and `var _ CmuxClient = (*RealClient)(nil)`) become the build-time gate that fails if any implementation is missing. Each `RealClient` wrapper is covered by a round-trip test against a fake cmux socket so plan RAID R1 (wrong wire method names silently swallowed) is caught at test time rather than in production. See plan: W2 and D-3, D-12.

**Description.**

1. In `src/internal/cmuxctl/client.go`, add four method signatures to the `CmuxClient` interface:
   ```
   WorkspaceSetStatus(ctx context.Context, ws Workspace, key, value string) error
   WorkspaceClearStatus(ctx context.Context, ws Workspace, key string) error
   WorkspaceSetProgress(ctx context.Context, ws Workspace, fraction float64, label string) error
   WorkspaceClearProgress(ctx context.Context, ws Workspace) error
   ```
2. In `src/internal/cmuxctl/real.go`, add four thin wrappers over `c.do(ctx, method, params)`. Verify the exact JSON-RPC wire method names against the cmux 0.64.7 source — the CLI contract documents verbs (`set-status`, etc.) but not wire names. Each wrapper passes the workspace ID via the `workspace_id` parameter convention already used by `WorkspaceClose` and `WorkspaceSelect`.
3. In `src/internal/cmuxctl/fake.go`, add four `Func` fields and four recorder slices following the existing `FakeClient` pattern:
   - `WorkspaceSetStatusFunc`, `WorkspaceClearStatusFunc`, `WorkspaceSetProgressFunc`, `WorkspaceClearProgressFunc` — each calls the corresponding `Func` if non-nil, otherwise returns `nil`.
   - `SetStatusCalls []SetStatusCall`, `ClearStatusCalls []ClearStatusCall`, `SetProgressCalls []SetProgressCall`, `ClearProgressCalls []Workspace` — protected by the existing `FakeClient` mutex. Define `SetStatusCall`, `ClearStatusCall`, `SetProgressCall` struct types with the parameter fields needed for ordering assertions.
4. Verify both compile-time assertions still pass.
5. Write `RealClient` round-trip tests: for each of the four new methods, stand up a fake cmux socket (using the existing `runphase1_test.go` pattern), call the method, and assert the wire method name and parameter shape are what the fake socket received. These tests fail loudly if the wire name is wrong.
6. Write `FakeClient` recorder tests: for each new method, write a focused unit test that calls the method and asserts the recorder slice contains one entry with the expected arguments, in order.

**References.**

- **Implementation plan** — [feature-implementation-plan.md#decomposition-and-sequencing](feature-implementation-plan.md#decomposition-and-sequencing) (W2) and the D-3, D-12 entries in the decision log.
- **Cmux CLI contract** — see Shared reference artifacts above (verb-name source; wire names verified against cmux 0.64.7 source at implementation time).
- **Feature technical notes T1** — see Shared reference artifacts (workspace ID parameter convention, latest-wins-per-key semantics).
- **Testing standard** — see Shared reference artifacts (fake methods must capture calls; each fake method must have its own call counter).
- **Concurrency standard** — see Shared reference artifacts (test doubles with shared state need mutexes).
- **`cmuxctl` package reference** — see Shared reference artifacts (existing `FakeClient` and `RealClient` patterns to follow).

**Tests.**

- `RealClient` round-trip tests: one test per method in `src/internal/cmuxctl/`, exercising the complete request/response cycle against a fake socket, asserting the wire method name and workspace ID parameter shape.
- `FakeClient` recorder tests: one test per method asserting the recorder slice records the call with the correct arguments in order.
- Compile-time gate: `var _ CmuxClient = (*FakeClient)(nil)` and `var _ CmuxClient = (*RealClient)(nil)` both compile.
- `cd src && go test -race ./internal/cmuxctl/...` passes.

**Acceptance criteria.**

- [ ] `CmuxClient` interface has exactly four new method signatures matching the plan.
- [ ] `RealClient` implements all four methods; wire names verified by the round-trip tests passing.
- [ ] `FakeClient` has four `Func` fields and four recorder slices, mutex-protected; compile-time interface assertions pass.
- [ ] Round-trip tests for all four `RealClient` methods pass under `-race`.
- [ ] `FakeClient` recorder tests assert correct argument capture and ordering.
- [ ] `cd src && go test -race ./internal/cmuxctl/...` passes.

**Depends on.** `W-1.`

## W-3 — Composition-root threading: capture `Workspace` and thread `CmuxClient` + `Workspace` through the orchestrator call chain

**Summary.** Thread `cmuxctl.CmuxClient` and `cmuxctl.Workspace` from the `cmuxOrchestratorHooks` composition root through `runCmuxOrchestratorWith` and into `runCmuxWorkflowAdapted`. Today both values are available at the composition root but the `Workspace` is silently discarded at `src/cmd/pr9k/cmux_pane.go:46` (the `func(rctx, _ Workspace)` discard). This slice captures the value, updates the three function signatures, and updates existing tests to pass `*FakeClient` and `Workspace{ID: "ws:test"}` to the new parameters. No sidebar behavior is added here — this is purely a structural prerequisite for W-4 through W-6. See plan: W3 (partial) and D-5.

**Description.**

1. In `src/cmd/pr9k/cmux_pane.go`, update `cmuxOrchestratorHooks`: add a `client cmuxctl.CmuxClient` parameter to the factory (this value comes from `main.go` at the `--cmux` branch where the production `*RealClient` already exists).
2. In the same file, change the `Phase1Hooks.Run` closure from `func(rctx, _ Workspace)` to `func(rctx context.Context, ws cmuxctl.Workspace)`, capturing `ws` and threading it forward to `runCmuxOrchestratorWith`.
3. Update `runCmuxOrchestratorWith`'s signature to add `client cmuxctl.CmuxClient` and `ws cmuxctl.Workspace` parameters. The body is otherwise unchanged — the new parameters will be used in W-6 when `*cmuxSidebar` construction is added.
4. Update `runCmuxWorkflowAdapted`'s signature to add a `sidebar *cmuxSidebar` parameter (this will be passed in from W-6; for now the parameter is typed but unused — Go allows unused parameters, so this compiles cleanly).
5. Update `main.go` at the `--cmux` branch to pass the production `*RealClient` to the updated `cmuxOrchestratorHooks` factory.
6. Update all existing test files that call `runCmuxOrchestratorWith` or `runCmuxWorkflowAdapted` — specifically `src/cmd/pr9k/cmux_orchestrator_test.go`, `cmux_pane_test.go`, `cmux_u9_test.go`, and `cmux_error_recovery_test.go` — to pass `&cmuxctl.FakeClient{}` and `cmuxctl.Workspace{ID: "ws:test"}` (and `nil` for the sidebar pointer, which W-6 provides for the production path).

**Note on the `*cmuxSidebar` parameter.** This work item adds the parameter slot before the type exists in W-4 — strictly speaking the parameter type is unresolved until W-4 lands. Two acceptable orderings: (a) W-4 lands the `cmuxSidebar` type first; the W-3 PR rebases onto W-4's commit and adds the parameter; (b) merge W-4's type-definition commit and W-3's signature change in the same PR. Either is AFK and within the scope of a single PR. The work-items file orders W-3 before W-4 to emphasize the no-behavior-change refactor pattern; the implementer may swap the order if it simplifies their PR sequence.

**References.**

- **Implementation plan** — [feature-implementation-plan.md#decomposition-and-sequencing](feature-implementation-plan.md#decomposition-and-sequencing) (W3 — composition-root threading portion) and the D-5 entry in the decision log.
- **Phase 3 implementation plan** — see Shared reference artifacts (extension point for `runCmuxWorkflowAdapted` and the `OnModeChange` closure pattern).
- **API-design standard** — see Shared reference artifacts (adapter types, compile-time interface assertions).

**Tests.**

- No new tests in this slice.
- All existing tests in `src/cmd/pr9k/` continue to pass: `cd src && go test -race ./cmd/pr9k/...`.
- The four new `FakeClient.*Func` fields from W-2 remain nil in existing tests (default behavior returns nil error) without breaking any pre-Phase-4 test.

**Acceptance criteria.**

- [ ] `cmuxOrchestratorHooks` factory accepts `client cmuxctl.CmuxClient`; `main.go` passes the production `*RealClient`.
- [ ] The `_ Workspace` discard at `cmux_pane.go:46` is replaced with a named `ws` parameter threaded to `runCmuxOrchestratorWith`.
- [ ] `runCmuxOrchestratorWith` and `runCmuxWorkflowAdapted` signatures include the new parameters.
- [ ] All four existing test files compile and pass after the cascade update.
- [ ] `cd src && go test -race ./cmd/pr9k/...` passes with no behavioral regression.
- [ ] `make build` and `make vet` pass.

**Depends on.** `W-2.`

## W-4 — Sidebar adapter types: `cmuxSidebar` struct and `sidebarAwareHeader` wrapper

**Summary.** Create `src/cmd/pr9k/cmux_sidebar.go` containing the `cmuxSidebar` struct, the `sidebarAwareHeader` wrapper, and the `pr9k.step` / em-dash literals as typed constants. Add the `nameAt(idx int) string` accessor to `cmuxHeader` in `cmux_workflow.go`. These types are fully testable in isolation against `FakeClient`: each method maps to one or two recorder-slice entries that tests can assert on without any orchestrator wiring. See plan: W3 (partial) and D-1, D-8, D-9, D-13, D-14.

**Description.**

1. Create `src/cmd/pr9k/cmux_sidebar.go`. Define:
   - `const sidebarStatusKey = "pr9k.step"` per D-8.
   - `const errorModeSuffix = " — awaiting input"` (U+2014 em-dash) per D-9. Precedent: `IterationIssueSep` in `src/internal/ui/header.go`.
   - `cmuxSidebar` struct with fields: `client cmuxctl.CmuxClient`, `ws cmuxctl.Workspace`, `log *logger.Logger`, `mu sync.Mutex`, `lastStepName string`, `progressPushed bool`, `disabled bool`. Implement per the concurrency standard's snapshot-then-unlock.
   - Methods on `*cmuxSidebar`: `PushStep(ctx, name) error`, `PushProgress(ctx, iter, maxIter) error` (no-op when `maxIter <= 0` per spec D4), `EnterErrorMode(ctx) error`, `ClearProgress(ctx) error`, `ClearAll(ctx) error`, and an unexported `pushStatus(ctx, value) error`. Non-timeout errors are logged via `s.log` and returned as nil; `context.DeadlineExceeded` propagates as fatal per parent D15 and spec D5.
   - Construction-time nil-handle guard: if `ws.ID == ""` and `ws.Ref == ""`, log a warning and set `disabled = true`; all methods become no-ops when `disabled` is true.

2. Add `nameAt(idx int) string` to `cmuxHeader` in `src/cmd/pr9k/cmux_workflow.go`. The method acquires `h.mu`, reads `h.names[idx]` if `idx` is in bounds, and returns `""` otherwise. No change to any existing `cmuxHeader` method.

3. In the same `cmux_sidebar.go` file, define `sidebarAwareHeader` struct with fields: `inner *cmuxHeader`, `sidebar *cmuxSidebar`, `ctx context.Context`, `finalizeBegun bool`. Implement `workflow.RunHeader`:
   - `RenderInitializeLine(stepNum, stepCount int, stepName string)` — delegates to `inner`, then calls `sidebar.PushStep(ctx, stepName)`.
   - `RenderIterationLine(iter, maxIter int, label string)` — delegates to `inner`, then calls `sidebar.PushProgress(ctx, iter, maxIter)`.
   - `SetStepState(idx int, state ui.StepState)` — delegates to `inner`; when `state == ui.StepActive`, calls `sidebar.PushStep(ctx, s.inner.nameAt(idx))`.
   - `RenderFinalizeLine(stepNum, stepCount int, stepName string)` — delegates to `inner`; on the first call (when `!s.finalizeBegun`), sets `s.finalizeBegun = true` and calls `sidebar.ClearProgress(ctx)` per D-13; on every call (including the first), calls `sidebar.PushStep(ctx, stepName)`.
   - All other `RunHeader` methods (e.g., `SetPhaseSteps`) delegate to `inner` only.
   - Compile-time assertion: `var _ workflow.RunHeader = (*sidebarAwareHeader)(nil)`.

4. Write unit tests in `src/cmd/pr9k/cmux_sidebar_test.go`:
   - `TestCmuxSidebar_PushStep`: asserts one `SetStatusCalls` entry with the correct key and value.
   - `TestCmuxSidebar_PushProgress_Bounded`: `PushProgress(1, 3)` produces fraction `1.0/3.0` and label `"1 / 3"`.
   - `TestCmuxSidebar_PushProgress_Unbounded`: `PushProgress(1, 0)` produces zero `SetProgressCalls` entries.
   - `TestCmuxSidebar_EnterErrorMode`: after `PushStep("Feature work")` then `EnterErrorMode`, the second `SetStatusCalls` entry's value is `"Feature work — awaiting input"`.
   - `TestCmuxSidebar_ClearAll_ClearsProgressOnlyIfPushed`: with prior `PushProgress`, `ClearAll` produces one `ClearStatusCalls` and one `ClearProgressCalls`; without prior `PushProgress`, `ClearAll` produces one `ClearStatusCalls` and zero `ClearProgressCalls`.
   - `TestCmuxSidebar_NonTimeoutErrorIsSwallowed`: `WorkspaceSetStatusFunc` returns a non-deadline error; `PushStep` returns nil.
   - `TestCmuxSidebar_TimeoutErrorIsFatal`: `WorkspaceSetStatusFunc` returns `context.DeadlineExceeded`; `PushStep` propagates the error.
   - `TestCmuxSidebar_DisabledOnZeroWorkspace`: construct with `Workspace{}`; assert all methods are no-ops and emit a warning log line at construction.
   - `TestSidebarAwareHeader_CallSequence`: drive initialize → iteration ×3 → first finalize → second finalize; assert recorder slices match the expected ordered sequence (status push on each step, progress pushes on each iteration, exactly one `ClearProgress` at first finalize, status push on both finalize calls).
   - `TestNameAt_OutOfBounds`: `nameAt(-1)` and `nameAt(len(names))` return `""` without panic.

**References.**

- **Implementation plan** — [feature-implementation-plan.md#decomposition-and-sequencing](feature-implementation-plan.md#decomposition-and-sequencing) (W3 — adapter types portion) and D-1, D-8, D-9, D-13, D-14 in the decision log.
- **Feature specification** — see Shared reference artifacts: Primary Flow (push cadence), Alternate Flows (unbounded / finalize / error mode), Edge Cases.
- **Feature technical notes T1** — see Shared reference artifacts.
- **Concurrency standard** — see Shared reference artifacts (snapshot-then-unlock; test doubles with shared state need mutexes).
- **Testing standard** — see Shared reference artifacts (fake methods must capture calls; test array bounds guards explicitly).
- **`cmuxctl` package reference** — see Shared reference artifacts (`FakeClient` recorder pattern).

**Tests.** See Description step 4 — ten focused unit tests covering all `cmuxSidebar` methods, the `sidebarAwareHeader` call sequence, and the bounds guard on `nameAt`.

**Acceptance criteria.**

- [ ] `src/cmd/pr9k/cmux_sidebar.go` exists with `cmuxSidebar`, `sidebarAwareHeader`, `sidebarStatusKey`, and `errorModeSuffix` constants.
- [ ] `cmuxHeader.nameAt(idx)` returns the correct name and `""` out-of-bounds.
- [ ] `sidebarAwareHeader` satisfies `workflow.RunHeader` (compile-time assertion present).
- [ ] All unit tests pass with `-race`.
- [ ] `PushProgress(1, 0)` produces zero `SetProgressCalls` entries.
- [ ] First `RenderFinalizeLine` produces exactly one `ClearProgressCalls` entry; subsequent calls do not.
- [ ] `cd src && go test -race ./cmd/pr9k/...` passes.

**Depends on.** `W-3.`

## W-5 — Wire sidebar adapter into orchestrator: `sidebarAwareHeader` as `RunHeader` + augment `OnModeChange` + graceful `ClearAll`

**Summary.** In `runCmuxWorkflowAdapted`, construct `sidebarAwareHeader` by wrapping the existing `cmuxHeader` and the passed-in `*cmuxSidebar`, and pass the wrapper to `workflow.Run` as `RunHeader`. Augment the existing `keyHandler.SetOnModeChange` closure in place to call `sidebar.EnterErrorMode(ctx)` when `mode == ui.ModeError`. Add `sidebar.ClearAll(ctx)` immediately after `workflow.Run` returns and after `keyHandler.SetMode(ui.ModeDone)`, before `keyCancel()` and `wg.Wait()`. This is the graceful-path cleanup. See plan: W3 (partial) and D-2, D-6.

**Description.**

1. In `src/cmd/pr9k/cmux_workflow.go`, locate where `cmuxHeader` is constructed and `workflow.Run` is called.
2. After constructing `cmuxHeader`, construct `sidebarAwareHeader{inner: header, sidebar: sidebar, ctx: ctx}` and pass this as the `RunHeader` argument to `workflow.Run` (replacing the plain `cmuxHeader`).
3. Locate the existing `keyHandler.SetOnModeChange` closure registered by Phase 3 (`docs/plans/cmux-rebuild/phase-3-interactive-error-recovery/feature-implementation-plan.md` D-6). Augment it in place: after the existing Phase 3 footer-push logic, add `if mode == ui.ModeError { _ = sidebar.EnterErrorMode(ctx) }`. Do **not** register a second `SetOnModeChange` call — `KeyHandler.SetOnModeChange` writes a single field and a second call would overwrite Phase 3's footer push.
4. Locate the point in `runCmuxWorkflowAdapted` where `workflow.Run` returns. Immediately after `workflow.Run` returns and after `keyHandler.SetMode(ui.ModeDone)`, add `_ = sidebar.ClearAll(ctx)`. This must appear before `keyCancel()` and before `wg.Wait()`. Add an inline comment: `// graceful-path sidebar cleanup (D-6)`.

**Note on the sidebar pointer.** When W-5 lands, `sidebar` may still be nil if W-6 has not landed yet (the parameter exists from W-3 but the production construction site is in W-6). All `*cmuxSidebar` methods must be safe against a nil receiver — either by adding `if s == nil { return nil }` guards at the top of each method on `*cmuxSidebar` (preferred — defense in depth) or by ensuring W-5 and W-6 land in the same PR.

**References.**

- **Implementation plan** — [feature-implementation-plan.md#decomposition-and-sequencing](feature-implementation-plan.md#decomposition-and-sequencing) (W3 — wiring portion) and D-2, D-6 in the decision log.
- **Phase 3 implementation plan** — see Shared reference artifacts (the `OnModeChange` closure this work augments; D-6 of Phase 3).
- **Feature specification** — see Shared reference artifacts: Alternate Flows — error mode; Primary Flow — completion sequence.
- **Concurrency standard** — see Shared reference artifacts (channel/mutex pattern for the augmented closure; the closure calls `sidebar.EnterErrorMode` which acquires the `cmuxSidebar` mutex internally).

**Tests.**

- Existing Phase 3 tests in `cmux_error_recovery_test.go` continue to pass (`WorkspaceSetStatusFunc` is nil in those tests so the new branch is a no-op for them).
- New targeted integration test `TestRunCmuxWorkflowAdapted_ErrorModeMarker` in `src/cmd/pr9k/cmux_sidebar_wiring_test.go`: drives a small synthetic workflow through `runCmuxWorkflowAdapted` with a `FakeInteractionChannel` and `FakeClient`, transitions through `ModeError` and back to `ModeNormal`, then graceful completion; asserts `SetStatusCalls` includes the error-mode marker value and that a subsequent step push overwrites it.
- `cd src && go test -race ./cmd/pr9k/...` passes.

**Acceptance criteria.**

- [ ] `runCmuxWorkflowAdapted` passes `sidebarAwareHeader` (not bare `cmuxHeader`) to `workflow.Run`.
- [ ] The existing `SetOnModeChange` closure is augmented in place with the `ModeError` branch; no second `SetOnModeChange` call exists in the file.
- [ ] `sidebar.ClearAll(ctx)` appears after `workflow.Run` returns and after `SetMode(ui.ModeDone)`, before `keyCancel()`.
- [ ] `*cmuxSidebar` methods are nil-receiver safe (or W-5 and W-6 land together).
- [ ] Existing Phase 3 error-recovery tests continue to pass.
- [ ] The new wiring integration test passes under `-race`.

**Depends on.** `W-4.`

## W-6 — Hoist sidebar construction and register outer defer

**Summary.** In `runCmuxOrchestratorWith`, construct `*cmuxSidebar` immediately after acquiring the `client` and `ws` parameters from W-3, and register `defer sidebar.ClearAll(context.Background())` immediately after construction. This outer defer is the safety net for panic, early-return, and abort paths that bypass the inner graceful-path `ClearAll` added in W-5. Pass the `*cmuxSidebar` pointer into `runCmuxWorkflowAdapted`. With this slice, the complete Phase 4 sidebar mirroring path is live in production. See plan: W3 (partial) and D-7.

**Description.**

1. In `src/cmd/pr9k/cmux_sidebar.go` (created in W-4), add a `newCmuxSidebar(client cmuxctl.CmuxClient, ws cmuxctl.Workspace, log *logger.Logger) *cmuxSidebar` constructor.
2. In `src/cmd/pr9k/cmux_pane.go`'s `runCmuxOrchestratorWith`, after the function has access to `client` and `ws` (both threaded in via W-3), construct the sidebar adapter: `sidebar := newCmuxSidebar(client, ws, log)` where `log` is the per-run logger already present in scope.
3. Immediately after construction, register the outer defer: `defer func() { _ = sidebar.ClearAll(context.Background()) }()`. Add an inline comment: `// D-7: safety-net clear for panic/abort paths; inner ClearAll in runCmuxWorkflowAdapted covers the graceful path`.
4. Pass `sidebar` into `runCmuxWorkflowAdapted` via the `sidebar *cmuxSidebar` parameter threaded in W-3.
5. The `context.Background()` in the defer is intentional: the parent context `ctx` may already be cancelled on the abort path, so teardown needs its own fresh context. Pattern precedent: `src/internal/cmuxctl/runphase1.go`.
6. The second `ClearAll` on the deferred path is a no-op against already-cleared cmux state (latest-wins-per-key per T1); non-timeout errors are logged-and-swallowed per spec D5.

**References.**

- **Implementation plan** — [feature-implementation-plan.md#decomposition-and-sequencing](feature-implementation-plan.md#decomposition-and-sequencing) (W3 — hoisted construction portion) and D-7 in the decision log.
- **Feature technical notes T1** — see Shared reference artifacts (latest-wins semantics; idempotent double-clear).
- **Concurrency standard** — see Shared reference artifacts (`context.Background()` in teardown closure pattern from `src/internal/cmuxctl/runphase1.go`).

**Tests.**

- No new unit tests in this slice; the construction and defer are covered by the round-trip integration test added in W-7.
- Verify the existing test cascade from W-3 still passes: `cd src && go test -race ./cmd/pr9k/...`.
- `make build` passes (the complete wiring compiles end-to-end).

**Acceptance criteria.**

- [ ] `newCmuxSidebar` constructor exists in `src/cmd/pr9k/cmux_sidebar.go`.
- [ ] `runCmuxOrchestratorWith` constructs `*cmuxSidebar` and registers `defer ClearAll(context.Background())` immediately after construction.
- [ ] `sidebar` is passed into `runCmuxWorkflowAdapted`.
- [ ] `make build` and `make vet` pass.
- [ ] `cd src && go test -race ./cmd/pr9k/...` passes.

**Depends on.** `W-5.`

## W-7 — Round-trip integration test for the complete sidebar call sequence

**Summary.** Write a single integration test that drives `runCmuxWorkflowAdapted` with `FakeClient` and `FakeInteractionChannel` through a small synthetic workflow and asserts the complete sidebar call sequence: step push on first step → progress push → additional step pushes → first finalize triggers `ClearProgress` → graceful `ClearAll` at the end. This is the Phase 4 equivalent of Phase 3's round-trip integration test. See plan: Testing Strategy — Round-trip integration.

**Description.**

1. In `src/cmd/pr9k/`, create `cmux_sidebar_integration_test.go` (or extend `cmux_sidebar_test.go` from W-4).
2. Write `TestSidebarCallSequence_FullWorkflow` that:
   - Constructs a `*FakeClient` with recorder slices.
   - Constructs a `FakeInteractionChannel` with a scripted handshake.
   - Builds a minimal synthetic workflow with two iteration steps and one finalization step, with `Iterations = 2` so the progress entry is active.
   - Calls `runCmuxWorkflowAdapted` (via the composed path from W-6) with the fake client and `cmuxctl.Workspace{ID: "ws:test"}`.
   - Asserts the ordered recorder-slice entries:
     - First `SetStatusCalls` entry: step name of the first iteration step.
     - First `SetProgressCalls` entry: fraction `0.5` (`1.0/2.0`), label `"1 / 2"`.
     - Subsequent `SetStatusCalls` entries advancing through both iterations.
     - Second `SetProgressCalls` entry for iteration 2.
     - One `ClearProgressCalls` entry at the first `RenderFinalizeLine`.
     - Final `ClearStatusCalls` entry from the graceful `ClearAll`.
3. Use `-race` as the required gate.
4. Per the testing standard: use channel-based hang injection if any blocking call needs to be verified rather than `time.Sleep`; protect `FakeClient` recorder slices with the existing mutex; assert the triggering conditions occurred (e.g., assert at least one iteration completed before asserting on the clear entries).

**References.**

- **Implementation plan** — [feature-implementation-plan.md#testing-strategy](feature-implementation-plan.md#testing-strategy).
- **Phase 3 implementation plan** — see Shared reference artifacts (the round-trip integration test pattern this work follows; Phase 3 D-8).
- **Testing standard** — see Shared reference artifacts (assert triggering conditions occurred; channel-based hang injection; no `time.Sleep` in tests).
- **Concurrency standard** — see Shared reference artifacts (test doubles with shared state need mutexes).
- **`interactionchannel` package reference** — see Shared reference artifacts (`FakeInteractionChannel` test double contract).
- **`cmuxctl` package reference** — see Shared reference artifacts (`FakeClient` recorder pattern).

**Tests.** The entire work item is a test. Required assertions enumerated in Description step 2.

**Acceptance criteria.**

- [ ] `TestSidebarCallSequence_FullWorkflow` passes with `-race`.
- [ ] The test asserts ordered recorder-slice entries across the full lifecycle: step push → progress push → clear-progress at first finalize → clear-all at end.
- [ ] The `ClearProgressCalls` entry appears before the final `ClearStatusCalls` entry in the ordered sequence.
- [ ] No `time.Sleep` calls in the test body.
- [ ] `cd src && go test -race ./cmd/pr9k/...` passes with no existing test regression.

**Depends on.** `W-6.`

## W-8 — Documentation updates

**Summary.** Extend three existing documentation files to cover Phase 4's new surface. `docs/features/cmux-mode.md` gains a sidebar-mirroring section; `docs/how-to/setting-up-cmux.md` gains a "Monitoring from another workspace" subsection (the "Tested-against" line itself moves to v0.64.7 in W-1); `docs/code-packages/cmuxctl.md` gains entries for the four new RPC wrappers. CLAUDE.md does not require new entries because only existing files are extended. See plan: W5 and D-11.

**Description.**

1. In `docs/features/cmux-mode.md`, append a "Sidebar mirroring (Phase 4)" section covering: what the status pill shows (step name; error-mode marker), when the progress entry is shown (bounded iterations only); the status-key constant `pr9k.step`; the error-mode marker literal `" — awaiting input"` (U+2014); the graceful-path clear behavior; the explicit out-of-scope items (no log surface, no failure-specific decoration) so operators do not expect more.

2. In `docs/how-to/setting-up-cmux.md`, add a subsection under the Phase 2 walkthrough titled "Monitoring from another workspace". Describe: switch to a different cmux workspace during a run; the pr9k workspace row shows the status pill and (if `-n M` was supplied) the progress bar; the pill updates on each step transition; error-mode shows `<step name> — awaiting input`; graceful run end clears both entries.

3. In `docs/code-packages/cmuxctl.md`, add documentation for the four new `CmuxClient` methods: `WorkspaceSetStatus`, `WorkspaceClearStatus`, `WorkspaceSetProgress`, `WorkspaceClearProgress`. For each, document the signature, the cmux surface it targets (status pill vs. progress bar), the parameter shape (workspace ID convention), and the error semantics (non-timeout logged-and-continued; timeouts fatal).

4. Verify that code blocks in the updated docs remain consistent with the production code — in particular, the `sidebarStatusKey` constant value `"pr9k.step"` and the error-mode marker `" — awaiting input"` must match `cmux_sidebar.go`.

**References.**

- **Implementation plan** — [feature-implementation-plan.md#decomposition-and-sequencing](feature-implementation-plan.md#decomposition-and-sequencing) (W5) and D-11 in the decision log.
- **Feature specification** — see Shared reference artifacts (User Interactions, Out of Scope).
- **Feature technical notes T1** — see Shared reference artifacts (status key naming; progress bar one-per-workspace).
- **Documentation standard** — see Shared reference artifacts (feature docs ship with the feature; code blocks consistent with production code).
- Files modified: `docs/features/cmux-mode.md`, `docs/how-to/setting-up-cmux.md`, `docs/code-packages/cmuxctl.md`.

**Tests.**

- `make build` passes (doc changes do not break compilation).
- Doc code blocks reference the exact constant values from `cmux_sidebar.go` — verified by reading both files in PR review.
- CLAUDE.md links to the three extended files continue to resolve (no new entries needed; the links already exist).

**Acceptance criteria.**

- [ ] `docs/features/cmux-mode.md` has a "Sidebar mirroring (Phase 4)" section covering the status pill, progress entry, error-mode marker, and graceful-path clear.
- [ ] `docs/how-to/setting-up-cmux.md` has a "Monitoring from another workspace" subsection.
- [ ] `docs/code-packages/cmuxctl.md` documents all four new `CmuxClient` methods.
- [ ] Doc code blocks use `pr9k.step` and ` — awaiting input` (U+2014) consistent with `cmux_sidebar.go`.
- [ ] `make build` passes.

**Depends on.** `W-2, W-4.`

## W-9 — Version bump 0.12.0 to 0.13.0

**Summary.** Bump `version.Version` from `0.12.0` to `0.13.0` in `src/internal/version/version.go`. This is the Phase 4 release marker. See plan: W6 and D-10. **Plan Open Item OI-1 applies:** the written standard at `docs/coding-standards/versioning.md` section "`0.y.z` — initial development" states that backwards-compatible additions bump PATCH during `0.y.z` (strict reading: `0.12.1`). Phase 2 and Phase 3 each bumped MINOR by convention. The plan recommends `0.13.0` to maintain that convention; the user may override to `0.12.1` at PR review. Either way the constant is one line.

**Description.**

1. In `src/internal/version/version.go`, update `const Version` from `"0.12.0"` to `"0.13.0"` (or `"0.12.1"` if the user overrides OI-1 at PR review).
2. Run `make ci` — rebuilds the binary and runs `src/internal/cli/args_test.go`, which reads from `version.Version` and passes automatically after the bump.
3. Verify `./bin/pr9k --version` prints the updated version string.
4. Do not hardcode the version string in any test. All tests read from `version.Version` per the testing standard.
5. Per the versioning standard, a version bump is its own commit (or combined with doc-only changes). Phase 4 is a feature PR; this bump is the final commit before merge.

**References.**

- **Implementation plan** — [feature-implementation-plan.md#decomposition-and-sequencing](feature-implementation-plan.md#decomposition-and-sequencing) (W6) and D-10 and Open Item OI-1.
- **Versioning standard** — see Shared reference artifacts (version constant is single source of truth; version bump is its own commit; `0.y.z` MINOR-vs-PATCH question).
- **Testing standard** — see Shared reference artifacts (never hardcode version strings in tests).
- File modified: `src/internal/version/version.go`.

**Tests.**

- `make ci` passes end-to-end (test, lint, format, vet, vulncheck, mod-tidy, build).
- `./bin/pr9k --version` output matches the bumped version string.

**Acceptance criteria.**

- [ ] `src/internal/version/version.go` `Version` constant reads `0.13.0` (or `0.12.1` per OI-1 user judgment at PR review).
- [ ] `make ci` passes end-to-end.
- [ ] `./bin/pr9k --version` prints the updated version.
- [ ] No test file hardcodes the version string as a literal.
- [ ] This is the final commit in the Phase 4 PR, after W-1 through W-8 are complete.

**Depends on.** `W-1, W-2, W-3, W-4, W-5, W-6, W-7, W-8.`

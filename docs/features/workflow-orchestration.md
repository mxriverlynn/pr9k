# Workflow Orchestration

Drives the entire ralph-tui workflow: running initialize steps, iterating over GitHub issues, sequencing steps with error recovery, and running finalization tasks.

- **Last Updated:** 2026-04-10 (issue #54)
- **Authors:**
  - River Bailey

## Overview

- `Run()` is the top-level orchestration goroutine that manages the full lifecycle in three config-defined phases: initialize, iteration loop, and finalize
- The initialize phase runs once before the loop, binding captured values (e.g. GitHub username, first issue ID) into the persistent VarTable so all subsequent phases can use them
- The iteration loop runs for exactly N iterations when `Iterations > 0`, or until `BreakLoopIfEmpty` fires (e.g. no issue found) when `Iterations == 0` (unbounded / run-until-done mode)
- `Orchestrate()` sequences resolved steps, manages step state transitions (pending → active → done/failed), and handles interactive error recovery
- All steps — initialize, iteration, and finalize — are resolved per-phase via the `VarTable` using `{{VAR}}` substitution; there are no hardcoded script calls in `Run()`
- The orchestration communicates with the keyboard handler via a `StepAction` channel for quit, continue, and retry decisions

Key files:
- `ralph-tui/internal/workflow/run.go` — `Run` function, `RunConfig`, `buildStep`, `ResolveCommand`, header adapters
- `ralph-tui/internal/ui/orchestrate.go` — `Orchestrate` function, `ResolvedStep`, error handling loop
- `ralph-tui/internal/workflow/run_test.go` — Unit tests for the `Run` orchestration loop
- `ralph-tui/internal/ui/orchestrate_test.go` — Unit tests for step sequencing and error recovery

## Architecture

```
                         Run()
                           │
              ┌────────────┼─────────────┐
              │            │             │
              ▼            ▼             ▼
       Initialize     Iteration     Finalize
         Phase         Loop          Phase
       (once, pre-    (1..N or      (always,
        loop)          BreakLoop)    post-loop)
              │            │             │
              └────────────┼─────────────┘
                           │
                    ┌──────▼──────────────────┐
                    │      buildStep()         │
                    │  {{VAR}} substitution    │
                    │  via VarTable + phase    │
                    └──────┬──────────────────┘
                           │
                    ┌──────▼──────────────────┐
                    │     Orchestrate()        │
                    │                          │
                    │  for each step:          │
                    │   drain Actions          │
                    │   set Active             │
                    │   RunStep()              │
                    │   handle result          │
                    └────────┬─────────────────┘
                             │
         ┌───────────┬───────┼──────────┐
         │           │       │          │
         ▼           ▼       ▼          ▼
      success    terminated failure    quit
      → Done     → Done    → Failed  → return
                 (skip)    → ModeError
                           → wait on
                             Actions:
                             c/r/q
```

## Key Files

| File | Purpose |
|------|---------|
| `ralph-tui/internal/workflow/run.go` | Run loop, RunConfig, buildStep, ResolveCommand, header adapters |
| `ralph-tui/internal/ui/orchestrate.go` | Step sequencing, error recovery state machine |
| `ralph-tui/internal/workflow/run_test.go` | Tests for Run lifecycle, initialize phase, BreakLoopIfEmpty |
| `ralph-tui/internal/ui/orchestrate_test.go` | Tests for Orchestrate behavior |

## Core Types

```go
// RunConfig holds all parameters needed by Run.
type RunConfig struct {
    ProjectDir      string
    Iterations      int
    InitializeSteps []steps.Step  // run once before the iteration loop
    Steps           []steps.Step  // run each iteration
    FinalizeSteps   []steps.Step  // run once after the loop
}

// RunResult holds the outcome of a completed Run call.
type RunResult struct {
    // IterationsRun is the index of the last iteration that began (1-based).
    // It includes the iteration that triggered a breakLoopIfEmpty exit.
    // Zero if the iteration loop never started.
    IterationsRun int
}

// StepExecutor wraps StepRunner + LastCapture + Close.
// *Runner satisfies this interface.
type StepExecutor interface {
    ui.StepRunner
    LastCapture() string  // last non-empty stdout line from the most recent RunStep
    Close() error
}

// RunHeader updates the TUI status header during workflow execution.
// *ui.StatusHeader satisfies this interface.
type RunHeader interface {
    RenderInitializeLine(stepNum, stepCount int, stepName string)
    RenderIterationLine(iter, maxIter int, issueID string)
    RenderFinalizeLine(stepNum, stepCount int, stepName string)
    SetPhaseSteps(names []string)
    SetStepState(idx int, state ui.StepState)
}

// ResolvedStep holds a step's name and its fully-resolved command argv.
type ResolvedStep struct {
    Name    string
    Command []string
}
```

## Implementation Details

### The Run Loop

`Run()` executes the full workflow lifecycle in three phases, all driven by the `VarTable`:

```go
func Run(executor StepExecutor, header RunHeader, keyHandler *ui.KeyHandler, cfg RunConfig) RunResult {
    vt := vars.New(cfg.ProjectDir, cfg.Iterations)

    // Phase 1: Initialize
    vt.SetPhase(vars.Initialize)
    for j, s := range cfg.InitializeSteps { ... }

    // Phase 2: Iteration loop
    iterationsRun := 0
    for i := 1; cfg.Iterations == 0 || i <= cfg.Iterations; i++ {
        iterationsRun = i
        vt.ResetIteration()
        vt.SetIteration(i)
        vt.SetPhase(vars.Iteration)
        ...
    }

    // Phase 3: Finalization
    vt.SetPhase(vars.Finalize)
    for j, s := range cfg.FinalizeSteps { ... }

    _ = executor.Close()
    return RunResult{IterationsRun: iterationsRun}
}
```

**Phase 1 — Initialize:** runs each `InitializeSteps` step once before the loop:
- Sets VarTable phase to `vars.Initialize`
- Calls `header.RenderInitializeLine(j+1, len(InitializeSteps), s.Name)` to update `IterationLine` before each step runs — so the header shows `"Initializing N/M: <step name>"` while that step executes
- Calls `buildStep` and `ui.Orchestrate` with a `noopHeader{}` — step checkboxes are not updated during the initialize phase (no `SetPhaseSteps` call, no checkbox rendering); only `IterationLine` is updated via `RenderInitializeLine`
- After each step, if `s.CaptureAs != ""`, calls `executor.LastCapture()` and binds the value into the persistent VarTable scope via `vt.Bind(vars.Initialize, s.CaptureAs, ...)`
- Bound values (e.g. `GITHUB_USER`, `ISSUE_ID`) are available in all subsequent phases via VarTable resolution
- If `Orchestrate` returns `ActionQuit`, closes executor and returns immediately
- If `buildStep` fails (e.g. missing prompt file), logs `"Error preparing initialize step: ..."` and skips `RenderInitializeLine` for that step, then continues to the next init step

**Phase 2 — Iteration loop:** runs `Steps` repeatedly:
- Runs from `i=1` upward; exits when `i > Iterations` (bounded) or when `BreakLoopIfEmpty` fires (unbounded when `Iterations == 0`)
- Resets the iteration table (`ResetIteration`), sets `ITER`, switches phase to `Iteration`
- Updates the status header for each iteration
- Builds resolved steps via `buildStep` (uses VarTable for `{{VAR}}` substitution)
- After each step, if `s.CaptureAs != ""`, binds captured output into the iteration-scoped VarTable, then re-calls `header.RenderIterationLine(i, cfg.Iterations, issueID)` — looking up `ISSUE_ID` from the iteration VarTable to update the header with the newly bound issue ID (empty string if `ISSUE_ID` was not the captured variable)
- If `s.BreakLoopIfEmpty` is set, `executor.LastCapture()` is empty, **and the step completed as `StepDone`**, exits the loop: remaining iteration steps are marked `StepSkipped` in the header before the loop exits, then finalization still runs; if the step failed (non-zero exit), the check is skipped so normal error-mode handling takes effect instead
- If `buildStep` fails, logs `"Error preparing steps: ..."` and breaks the inner loop (finalization still runs)
- If `Orchestrate` returns `ActionQuit`, closes executor and returns without finalization

**Phase 3 — Finalization:** runs even after an early loop exit:
- Calls `header.SetPhaseSteps(finalizeNames)` to switch the header to finalization step names
- Switches the VarTable phase to `Finalize`
- For each finalize step: calls `header.RenderFinalizeLine(j+1, len(FinalizeSteps), s.Name)` to update `IterationLine` before the step runs — so the header shows `"Finalizing N/M: <step name>"` while that step executes
- Builds resolved steps via `buildStep`; if `buildStep` fails, logs `"Error preparing finalize step: ..."` and skips `RenderFinalizeLine` for that step, then continues to the next
- Runs through `Orchestrate()` with a `trackingOffsetIterHeader` adapter (same adapter as the iteration phase, reused since both phases use `SetStepState`)

### Step Resolution

`buildStep` converts a single `Step` into a `ResolvedStep` by either building a Claude CLI command or resolving a shell command. Both paths use the `VarTable` for `{{VAR}}` substitution:

```go
func buildStep(projectDir string, s steps.Step, vt *vars.VarTable, phase vars.Phase) (ui.ResolvedStep, error) {
    if s.IsClaude {
        prompt, err := steps.BuildPrompt(projectDir, s, vt, phase)
        if err != nil {
            return ui.ResolvedStep{}, fmt.Errorf("step %q: %w", s.Name, err)
        }
        return ui.ResolvedStep{
            Name:    s.Name,
            Command: []string{"claude", "--permission-mode", "acceptEdits", "--model", s.Model, "-p", prompt},
        }, nil
    }
    return ui.ResolvedStep{
        Name:    s.Name,
        Command: ResolveCommand(projectDir, s.Command, vt, phase),
    }, nil
}
```

The `VarTable` is created once at the start of `Run` and carries iteration-scoped variables (`ISSUE_ID`, `STARTING_SHA`) alongside persistent built-ins (`PROJECT_DIR`, `MAX_ITER`, `ITER`, `STEP_NUM`, `STEP_COUNT`, `STEP_NAME`) and any values bound by initialize-phase `captureAs` steps. At the start of each iteration, the table is reset and the new iteration's values are bound before step resolution runs.

### The Orchestrate State Machine

`Orchestrate()` runs resolved steps in sequence with error recovery:

```go
func Orchestrate(steps []ResolvedStep, runner StepRunner, header StepHeader, h *KeyHandler) StepAction {
    for i, step := range steps {
        // Non-blocking drain: catch ActionQuit from ForceQuit before starting
        select {
        case action := <-h.Actions:
            if action == ActionQuit { return ActionQuit }
        default:
        }

        header.SetStepState(i, StepActive)
        action := runStepWithErrorHandling(i, step, runner, header, h)
        if action == ActionQuit { return ActionQuit }
    }
    return ActionContinue
}
```

On step failure (non-zero exit, not user-terminated), `runStepWithErrorHandling` enters error mode and blocks on user input:

```go
func runStepWithErrorHandling(...) StepAction {
    for {
        err := runner.RunStep(step.Name, step.Command)

        if err == nil || runner.WasTerminated() {
            header.SetStepState(idx, StepDone)
            return ActionContinue
        }

        header.SetStepState(idx, StepFailed)
        h.SetMode(ModeError)

        action := <-h.Actions  // blocks until user decides
        h.SetMode(ModeNormal)

        switch action {
        case ActionContinue: return ActionContinue  // step stays [✗], advance
        case ActionRetry:    // loop back to re-run
        case ActionQuit:     return ActionQuit
        }
    }
}
```

### Header Adapters

Two adapter types route `SetStepState` calls to the correct TUI checkbox position depending on the workflow phase:

```go
// noopHeader satisfies ui.StepHeader with no-op methods. Passed to
// Orchestrate during the initialize phase to suppress step-checkbox updates
// — the initialize phase has no checkbox grid. Note: IterationLine IS still
// updated during initialize, but via header.RenderInitializeLine called
// directly from Run, not through Orchestrate.
type noopHeader struct{}
func (noopHeader) SetStepState(int, ui.StepState) {}

// trackingOffsetIterHeader adapts RunHeader to ui.StepHeader for a single
// step at absolute index idx. It also records the last StepState set so Run
// can check whether the step ended as StepDone before consulting
// BreakLoopIfEmpty.
//
// Used for both iteration and finalization phases — both phases call
// SetPhaseSteps to swap the header's step name list, so the same
// SetStepState call routes correctly for either phase.
type trackingOffsetIterHeader struct {
    h         RunHeader
    idx       int
    lastState ui.StepState
}
func (a *trackingOffsetIterHeader) SetStepState(_ int, state ui.StepState) {
    a.lastState = state
    a.h.SetStepState(a.idx, state)
}
```

The `trackingOffsetIterHeader` adapter is needed because `Orchestrate` always calls `header.SetStepState(i, ...)` using the local step index `i`, but each step is dispatched individually from `Run()` — so the absolute TUI checkbox position must be pinned at construction time via `idx`. The tracking variant also records `lastState` so `Run` can distinguish a successful `StepDone` completion from a failed step before evaluating `BreakLoopIfEmpty`.

## Testing

- `ralph-tui/internal/workflow/run_test.go` — Tests `Run` lifecycle with `fakeExecutor` and `fakeRunHeader` test doubles:
  - `TestRun_InitializeStepsRunBeforeIterationSteps` — verifies ordering: init steps run before iteration steps
  - `TestRun_InitializeCaptureAvailableInIteration` — verifies that `CaptureAs` values bound in the initialize phase are substituted as `{{VAR}}` tokens in iteration step commands
  - `TestRun_InitializeBuildErrorContinuesToNextInitStep` — verifies that a bad init step (missing prompt file) logs `"Error preparing initialize step"`, skips that step, and continues to the next
  - `TestRun_QuitFromInitializeOrchestrateClosesEarly` — verifies `ActionQuit` during init closes the executor and skips iteration and finalization
  - `TestRun_BreakLoopIfEmptyCapture` / `TestRun_BreakLoopIfEmptyNonEmptyCapture` — verify early-exit loop semantics
  - `TestRun_BreakLoopIfEmpty_MarksRemainingStepsSkipped` — verifies trigger at index 0 marks all subsequent step indices as `StepSkipped`
  - `TestRun_BreakLoopIfEmpty_NoSkipWhenNotTriggered` — verifies no `StepSkipped` calls when captured value is non-empty (break does not fire)
  - `TestRun_BreakLoopIfEmpty_LastStepNoRemainingSkips` — boundary: single-step iteration, break fires on the only step → no remaining steps to mark
  - `TestRun_BreakLoopIfEmpty_MultiIterBreakOnSecond` — multi-iteration: break fires on iteration 2 only; `StepSkipped` appears exactly once, confirming iteration 1 steps are unaffected
  - `TestRun_BreakLoopIfEmpty_FailedStepNoSkips` — failed step guard: step returns error → no `StepSkipped` calls (break check is skipped)
  - `TestRun_StepBuildErrorSkipsIterationAndContinuesToFinalization` — verifies a missing prompt file for an iteration step logs `"Error preparing steps"` and skips to finalization
  - `TestRun_IterationsRunOnNormalCompletion` — verifies `RunResult.IterationsRun` equals the configured `Iterations` count after a normal bounded run
  - `TestRun_IterationsRunZeroOnInitializeQuit` — verifies `RunResult.IterationsRun` is zero when `ActionQuit` fires during the initialize phase
  - `TestRun_IterationsRunOnIterationQuit` — verifies `RunResult.IterationsRun` reflects the in-progress iteration index when `ActionQuit` fires mid-loop
  - `TestRun_IterationHeaderUpdatesAfterCaptureAs` — verifies that `RenderIterationLine` is re-called with the issue ID after a `captureAs` step binds `ISSUE_ID`
  - `TestRun_SecondIterationStartsWithEmptyIssueID` — verifies that at the start of each iteration, `RenderIterationLine` is called with an empty issue ID (cleared by `ResetIteration`)
  - `TestRun_NonCapturingIterStepDoesNotRerenderHeader` — verifies that iteration steps without `captureAs` do not trigger a second `RenderIterationLine` call
  - `TestRun_InitializeRenderCalledPerStep` — verifies `RenderInitializeLine` is called once per init step with correct `stepNum`, `stepCount`, and `stepName` values
  - `TestRun_FinalizeRenderCalledPerStep` — verifies `RenderFinalizeLine` is called once per finalize step with correct `stepNum`, `stepCount`, and `stepName` values
  - `TestRun_InitializeBuildErrorSkipsRenderInitializeLine` — verifies that `RenderInitializeLine` is not called for an init step whose `buildStep` call fails
  - `TestRun_FinalizeBuildErrorSkipsRenderFinalizeLine` — verifies that `RenderFinalizeLine` is not called for a finalize step whose `buildStep` call fails
  - `TestRun_CaptureAsNonIssueIDProducesEmptyIssueIDInHeader` — verifies that a `captureAs` binding for a variable other than `ISSUE_ID` still re-renders the header, but with an empty issue ID
  - `TestRun_QuitFromInitializeProducesZeroIterationAndFinalizeHeaderCalls` — verifies that quitting during the initialize phase produces zero `RenderIterationLine` and `RenderFinalizeLine` calls
  - `TestRun_QuitDuringFinalizeRecordsOnlyTheQuittingStepRender` — verifies that when quit fires during the finalize phase, only render calls up to and including the quitting step are recorded
  - `TestRun_FinalizeRenderCalledAfterBreakLoopIfEmpty` — verifies that `RenderFinalizeLine` is still called for finalize steps after an early loop exit via `BreakLoopIfEmpty`
- `ralph-tui/internal/ui/orchestrate_test.go` — Tests step sequencing, error recovery (continue/retry/quit), terminated step handling, pre-step quit drain

## Additional Information

- [Architecture Overview](../architecture.md) — System-level view of the orchestration flow with block diagrams
- [Building Custom Workflows](../how-to/building-custom-workflows.md) — How to create and modify workflow step sequences
- [Variable Output & Injection](../how-to/variable-output-and-injection.md) — How iteration variables are captured and injected into steps
- [Step Definitions & Prompt Building](step-definitions.md) — How steps are loaded and prompts are built
- [Subprocess Execution & Streaming](subprocess-execution.md) — How RunStep executes subprocesses; how LastCapture returns stdout output
- [CLI & Configuration](cli-configuration.md) — How ProjectDir and Iterations are parsed and passed to RunConfig
- [Keyboard Input & Error Recovery](keyboard-input.md) — How user decisions flow through the Actions channel
- [Signal Handling & Shutdown](signal-handling.md) — How ForceQuit injects ActionQuit for clean shutdown
- [TUI Status Header](tui-display.md) — How step state updates are rendered
- [File Logging](file-logging.md) — How step separator lines are written to the log file
- [Variable State Management](variable-state.md) — VarTable scopes, phase transitions, and CaptureAs binding
- [ralph-tui Plan](../plans/ralph-tui.md) — Original specification including orchestration design
- [Concurrency](../coding-standards/concurrency.md) — Coding standards for channel-based dispatch and non-blocking drain
- [API Design](../coding-standards/api-design.md) — Coding standards for adapter types used in header adapters

# Workflow Orchestration

Drives the entire ralph-tui workflow: running initialize steps, iterating over GitHub issues, sequencing steps with error recovery, running finalization tasks, and writing structured chrome into the log body (phase banners, step banners, capture logs, completion summary).

- **Last Updated:** 2026-04-11
- **Authors:**
  - River Bailey

## Overview

- `Run()` is the top-level orchestration goroutine that manages the full lifecycle in three config-defined phases: initialize, iteration loop, and finalize
- The initialize phase runs once before the loop, binding captured values (e.g. GitHub username, first issue ID) into the persistent VarTable so all subsequent phases can use them
- The iteration loop runs for exactly N iterations when `Iterations > 0`, or until `BreakLoopIfEmpty` fires (e.g. no issue found) when `Iterations == 0` (unbounded / run-until-done mode)
- `Orchestrate()` sequences resolved steps, manages step state transitions (pending → active → done/failed), and handles interactive error recovery. It also writes a `Starting step: <name>` banner to the log body before every started step
- `Run` writes full-width `PhaseBanner` headings (`Initializing`, `Iterations`, `Finalizing`) on entering each phase, and a `Captured VAR = "value"` line after any step with `captureAs` set
- All steps — initialize, iteration, and finalize — are resolved per-phase via the `VarTable` using `{{VAR}}` substitution; there are no hardcoded script calls in `Run()`
- The orchestration communicates with the keyboard handler via a `StepAction` channel for quit, continue, and retry decisions
- After the finalize phase, `Run` writes a `CompletionSummary` line to the log body (not the header) and returns on its own — the workflow goroutine in `main.go` tears down the TUI and exits the process

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
    // LogWidth is the column width used for full-width phase banner
    // underlines. 0 or negative falls back to ui.DefaultTerminalWidth.
    // main.go computes ui.TerminalWidth() - 2 (for rounded border glyphs)
    // and passes it here.
    LogWidth        int
}

// RunResult holds the outcome of a completed Run call.
type RunResult struct {
    // IterationsRun is the index of the last iteration that began (1-based).
    // It includes the iteration that triggered a breakLoopIfEmpty exit.
    // Zero if the iteration loop never started.
    IterationsRun int
}

// StepExecutor wraps StepRunner + LastCapture.
// *Runner satisfies this interface.
type StepExecutor interface {
    ui.StepRunner
    LastCapture() string  // last non-empty stdout line from the most recent RunStep
}

// RunHeader updates the TUI status header during workflow execution.
// *ui.StatusHeader satisfies this interface.
//
// The header no longer carries a completion-summary method — the final
// summary is written to the log body via ui.CompletionSummary instead.
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

### Pre-Run Validation

Before `Run()` is called, `main.go` invokes `validator.Validate(projectDir)` against `ralph-steps.json`. This covers all eight D13 validation categories — JSON parseability, schema shape per step, phase size, referenced file existence, and variable scope resolution — collecting every error in a single pass. If any errors are found, the process exits 1 and writes all structured errors to stderr before the TUI starts. This ensures every step's config is sound before any subprocess runs.

See [Config Validation](config-validation.md) for the full list of validation rules.

### The Run Loop

`Run()` executes the full workflow lifecycle in three phases, all driven by the `VarTable`. A local `emitBlank` closure writes a single blank separator line before every content block (iteration separator, Orchestrate call, phase banner, capture log, completion summary) — no-op on the very first call so the log does not begin with a blank line. A `writePhaseBanner` closure calls `emitBlank` then writes `PhaseBanner(name, logWidth)` to the log. A `writeCaptureLog` closure calls `emitBlank` then writes `CaptureLog(varName, value)`.

```go
func Run(executor StepExecutor, header RunHeader, keyHandler *ui.KeyHandler, cfg RunConfig) RunResult {
    vt := vars.New(cfg.ProjectDir, cfg.Iterations)

    logWidth := cfg.LogWidth
    if logWidth <= 0 {
        logWidth = ui.DefaultTerminalWidth
    }

    needBlank := false
    emitBlank := func() {
        if needBlank { executor.WriteToLog("") }
        needBlank = true
    }
    writePhaseBanner := func(phaseName string) {
        emitBlank()
        heading, underline := ui.PhaseBanner(phaseName, logWidth)
        executor.WriteToLog(heading)
        executor.WriteToLog(underline)
    }
    writeCaptureLog := func(varName, value string) {
        emitBlank()
        executor.WriteToLog(ui.CaptureLog(varName, value))
    }

    // Phase 1: Initialize (banner only if there are init steps)
    vt.SetPhase(vars.Initialize)
    if len(cfg.InitializeSteps) > 0 {
        writePhaseBanner("Initializing")
    }
    for j, s := range cfg.InitializeSteps { ... }

    // Phase 2: Iteration loop (banner is unconditional)
    writePhaseBanner("Iterations")
    iterationsRun := 0
    for i := 1; cfg.Iterations == 0 || i <= cfg.Iterations; i++ {
        iterationsRun = i
        vt.ResetIteration()
        vt.SetIteration(i)
        vt.SetPhase(vars.Iteration)
        ...
        emitBlank()
        executor.WriteToLog(ui.StepSeparator(fmt.Sprintf("Iteration %d", i)))
        ...
    }

    // Phase 3: Finalization (banner only if there are finalize steps)
    if len(cfg.FinalizeSteps) > 0 {
        writePhaseBanner("Finalizing")
    }
    for j, s := range cfg.FinalizeSteps { ... }

    // Phase 4: Completion — write summary to log body and return
    emitBlank()
    executor.WriteToLog(ui.CompletionSummary(iterationsRun, len(cfg.FinalizeSteps)))

    return RunResult{IterationsRun: iterationsRun}
}
```

**Phase 1 — Initialize:** runs each `InitializeSteps` step once before the loop:
- Sets VarTable phase to `vars.Initialize`
- Writes `PhaseBanner("Initializing", logWidth)` if and only if `len(cfg.InitializeSteps) > 0` (no banner for an empty phase)
- Calls `header.RenderInitializeLine(j+1, len(InitializeSteps), s.Name)` to update `IterationLine` before each step runs — so the header shows `"Initializing N/M: <step name>"` while that step executes
- Calls `emitBlank` then `buildStep` and `ui.Orchestrate` with a `noopHeader{}` — step checkboxes are not updated during the initialize phase (no `SetPhaseSteps` call, no checkbox rendering); only `IterationLine` is updated via `RenderInitializeLine`. `Orchestrate` itself writes the `Starting step: <name>` banner + underline + trailing blank line to the log before running the step
- After each step, if `s.CaptureAs != ""`, calls `executor.LastCapture()`, binds the value into the persistent VarTable scope via `vt.Bind(vars.Initialize, s.CaptureAs, ...)`, and calls `writeCaptureLog(s.CaptureAs, captured)` to append a `Captured VAR = "value"` line to the log body
- Bound values (e.g. `GITHUB_USER`, `ISSUE_ID`) are available in all subsequent phases via VarTable resolution
- If `Orchestrate` returns `ActionQuit`, returns immediately
- If `buildStep` fails (e.g. missing prompt file), logs `"Error preparing initialize step: ..."` and skips `RenderInitializeLine` for that step, then continues to the next init step

**Phase 2 — Iteration loop:** runs `Steps` repeatedly:
- Writes `PhaseBanner("Iterations", logWidth)` unconditionally at the top of the phase
- Runs from `i=1` upward; exits when `i > Iterations` (bounded) or when `BreakLoopIfEmpty` fires (unbounded when `Iterations == 0`)
- Resets the iteration table (`ResetIteration`), sets `ITER`, switches phase to `Iteration`
- Updates the status header for each iteration
- Writes `ui.StepSeparator("Iteration N")` (with an `emitBlank` before) to mark each iteration in the log body
- Builds resolved steps via `buildStep` (uses VarTable for `{{VAR}}` substitution)
- Calls `emitBlank` before each `Orchestrate` call so consecutive steps are separated by one blank line
- After each step, if `s.CaptureAs != ""`, binds captured output into the iteration-scoped VarTable, re-calls `header.RenderIterationLine(i, cfg.Iterations, issueID)` — looking up `ISSUE_ID` from the iteration VarTable to update the header with the newly bound issue ID (empty string if `ISSUE_ID` was not the captured variable) — and calls `writeCaptureLog(s.CaptureAs, captured)` to append the bound value to the log body
- If `s.BreakLoopIfEmpty` is set, `executor.LastCapture()` is empty, **and the step completed as `StepDone`**, exits the loop: remaining iteration steps are marked `StepSkipped` in the header before the loop exits, then finalization still runs; if the step failed (non-zero exit), the check is skipped so normal error-mode handling takes effect instead
- If `buildStep` fails, logs `"Error preparing steps: ..."` and breaks the inner loop (finalization still runs)
- If `Orchestrate` returns `ActionQuit`, returns without finalization

**Phase 3 — Finalization:** runs even after an early loop exit:
- Calls `header.SetPhaseSteps(finalizeNames)` to switch the header to finalization step names
- Switches the VarTable phase to `Finalize`
- Writes `PhaseBanner("Finalizing", logWidth)` if and only if `len(cfg.FinalizeSteps) > 0`
- For each finalize step: calls `header.RenderFinalizeLine(j+1, len(FinalizeSteps), s.Name)` to update `IterationLine` before the step runs — so the header shows `"Finalizing N/M: <step name>"` while that step executes
- Builds resolved steps via `buildStep`; if `buildStep` fails, logs `"Error preparing finalize step: ..."` and skips `RenderFinalizeLine` for that step, then continues to the next
- Runs through `Orchestrate()` with a `trackingOffsetIterHeader` adapter (same adapter as the iteration phase, reused since both phases use `SetStepState`)

**Phase 4 — Completion:** after finalize completes normally:
- Calls `emitBlank` then writes `ui.CompletionSummary(iterationsRun, len(cfg.FinalizeSteps))` — `"Ralph completed after N iteration(s) and M finalizing tasks."` — as the **last non-blank line of the log body**. The header's `IterationLine` retains the final `"Finalizing N/M: <step name>"` value from the last finalize step; there is no header-level completion line
- Returns `RunResult{IterationsRun: iterationsRun}` — the caller (the workflow goroutine in `main.go`) then restores the terminal and exits the process

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
            Command: []string{"claude", "--permission-mode", "bypassPermissions", "--model", s.Model, "-p", prompt},
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

`Orchestrate()` runs resolved steps in sequence with error recovery, and writes a `Starting step: <name>` banner to the log body before each started step:

```go
func Orchestrate(steps []ResolvedStep, runner StepRunner, header StepHeader, h *KeyHandler) StepAction {
    for i, step := range steps {
        // Non-blocking drain: catch ActionQuit from ForceQuit before starting
        select {
        case action := <-h.Actions:
            if action == ActionQuit { return ActionQuit }
        default:
        }

        // Write the "Starting step: <name>" banner — heading, underline,
        // and a trailing blank line separating the banner from step output.
        heading, underline := StepStartBanner(step.Name)
        runner.WriteToLog(heading)
        runner.WriteToLog(underline)
        runner.WriteToLog("")

        header.SetStepState(i, StepActive)
        action := runStepWithErrorHandling(i, step, runner, header, h)
        if action == ActionQuit { return ActionQuit }
    }
    return ActionContinue
}
```

The banner is written **after** the pre-step quit drain so pending quits don't write a banner for a step that will not run. Retries re-enter `runStepWithErrorHandling` and write `RetryStepSeparator(step.Name)` via `WriteToLog` before the retry; the `Starting step` banner is written once per step (not per attempt).

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
  - `TestRun_QuitFromInitializeSkipsRemainingPhases` — verifies `ActionQuit` during the initialize phase skips all iteration and finalization steps
  - `TestRun_QuitFromIterationSkipsFinalization` — verifies `ActionQuit` during an iteration skips the finalization phase
  - `TestRun_QuitFromFinalizationReturnsWithoutSummary` — verifies `ActionQuit` during finalization returns without writing the completion summary
  - `TestRun_BreakLoopIfEmptyCapture` / `TestRun_BreakLoopIfEmptyNonEmptyCapture` — verify early-exit loop semantics
  - `TestRun_BreakLoopIfEmpty_MarksRemainingStepsSkipped` — verifies trigger at index 0 marks all subsequent step indices as `StepSkipped`
  - `TestRun_BreakLoopIfEmpty_NoSkipWhenNotTriggered` — verifies no `StepSkipped` calls when captured value is non-empty (break does not fire)
  - `TestRun_BreakLoopIfEmpty_LastStepNoRemainingSkips` — boundary: single-step iteration, break fires on the only step → no remaining steps to mark
  - `TestRun_BreakLoopIfEmpty_MultiIterBreakOnSecond` — multi-iteration: break fires on iteration 2 only; `StepSkipped` appears exactly once, confirming iteration 1 steps are unaffected
  - `TestRun_BreakLoopIfEmpty_FailedStepNoSkips` — failed step guard: step returns error → no `StepSkipped` calls (break check is skipped)
  - `TestRun_UnlimitedIterations` — verifies `Iterations==0` runs until `BreakLoopIfEmpty` exits the loop
  - `TestRun_NegativeIterationsRunsZeroIterations` — verifies negative `Iterations` values run zero iterations and proceed directly to finalization
  - `TestRun_StepBuildErrorSkipsIterationAndContinuesToFinalization` — verifies a missing prompt file for an iteration step logs `"Error preparing steps"` and skips to finalization
  - `TestRun_StepBuildErrorAbortsAllRemainingIterations` — verifies a build error on iteration 1 of a 3-iteration config exits the entire loop (not just the current iteration)
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
  - `TestRun_CompletionSummaryAndBlockForKeypress` — verifies the `ui.CompletionSummary` string is written to the log body via `executor.WriteToLog`, that it is the last non-blank line, that `Run` blocks before `ActionQuit` is sent, and that `Run` returns after it is sent (uses `fakeExecutor.onLog` as a synchronization point for the completion line)
  - `TestRun_CompletionSummaryWithEmptyFinalize` — verifies the completion summary is written with `finalizeCount=0` when `FinalizeSteps` is empty
  - `TestRun_CompletionSummary_AfterBreakLoopIfEmpty` — verifies `iterationsRun=1` after early loop exit with a 3-iteration config; confirms the log-body completion summary fires with the correct partial iteration count
  - `TestRun_LogsPhaseBanners` — verifies each phase writes a banner heading exactly matching the phase name followed by a `═`-filled underline whose rune count equals `cfg.LogWidth`
  - `TestRun_PhaseBannerOrderingAcrossPhases` — verifies the Initializing → Iterations → Finalizing ordering in the log body
  - `TestRun_InitializingPhaseSkippedWhenNoInitSteps` — verifies the Initializing banner is omitted when `InitializeSteps` is empty, while the Iterations banner is still present
  - `TestRun_FinalizingPhaseSkippedWhenNoFinalizeSteps` — verifies the Finalizing banner is omitted when `FinalizeSteps` is empty
  - `TestRun_PhaseBannerUsesDefaultWidthWhenZero` — verifies `cfg.LogWidth == 0` falls back to `ui.DefaultTerminalWidth` (80) runes
  - `TestRun_CaptureLogWrittenAfterCaptureStep` — verifies an iteration step with `captureAs: "ISSUE_ID"` produces a `Captured ISSUE_ID = "42"` log line after the step
  - `TestRun_CaptureLogWrittenForInitializePhase` — verifies the same behavior for an initialize-phase capture
  - `TestRun_CaptureLogNotWrittenForNonCaptureStep` — negative test: no `Captured ` line appears when the step has no `captureAs`
- `ralph-tui/internal/ui/orchestrate_test.go` — Tests step sequencing, error recovery (continue/retry/quit), terminated step handling, pre-step quit drain, retry separator:
  - `TestOrchestrate_WritesStepStartBannerBeforeEachStep` — verifies heading, underline, and blank line are written to the log before each step runs
  - `TestOrchestrate_SetsStepActiveBeforeRunning` — verifies `SetStepState(Active)` is called before `RunStep` via a `callbackStubRunner`
  - `TestOrchestrate_Retry_StateTransitionSequence` — verifies the `Active→Failed→Done` state transition sequence on retry (note: `StepActive` is not re-set on retry — this is documented in the test)

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

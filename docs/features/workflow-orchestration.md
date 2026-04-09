# Workflow Orchestration

Drives the entire ralph-tui workflow: executing a three-phase config-driven loop, sequencing steps with error recovery, and managing the VariablePool for just-in-time variable substitution.

- **Last Updated:** 2026-04-09 (updated for three-phase config-driven Run() refactor)
- **Authors:**
  - River Bailey

## Overview

- `Run()` is the top-level orchestration goroutine that drives three phases — pre-loop, loop, post-loop — entirely from a `*steps.WorkflowConfig`; no paths or step lists are hardcoded
- `executeStep()` is the core step executor: it resolves the command (via `BuildPrompt` or `ResolveCommand`), runs it (via `CaptureOutput` or `RunStep`), stores captured output in the `VariablePool`, and drives error recovery via `HandleStepError`
- `runPhase()` sequences a slice of steps by calling `executeStep()` for each, draining the quit channel before each step
- `HandleStepError()` (in `ui/orchestrate.go`) handles the error case after a step fails — it blocks on the Actions channel and returns the user's chosen action (continue, retry, quit)
- Variables captured by `outputVariable` steps are stored in the `VariablePool` and substituted just-in-time into subsequent steps via `pool.All()`
- Loop-scoped variables are cleared between iterations via `LoopVariableNames` + `pool.Clear`

Key files:
- `ralph-tui/internal/workflow/run.go` — Run, runPhase, executeStep, phaseStepNames, RunConfig, RunHeader
- `ralph-tui/internal/workflow/variables.go` — VariablePool: in-memory key-value store for workflow variables
- `ralph-tui/internal/ui/orchestrate.go` — HandleStepError, StepHeader, ResolvedStep
- `ralph-tui/internal/workflow/run_test.go` — Unit tests for the Run orchestration loop
- `ralph-tui/internal/ui/orchestrate_test.go` — Unit tests for HandleStepError behavior

## Architecture

```
                        Run()
                          │
              ┌───────────┼────────────────┐
              │           │                │
              ▼           ▼                ▼
          Display      Phase 1:         Phase 2:
          Banner       pre-loop         loop (1..N)
                       runPhase()          │
                           │     ┌─────────┴──────────┐
                           │     │                    │
                           │     ▼                    ▼
                           │  pool.Clear(        executeStep()
                           │  loopVarNames)      for each step
                           │  SetPhaseSteps(          │
                           │  "Iter i/N", ...)         │
                           │                     exitLoopIfEmpty?
                           │                     → break
                           │
                           ▼
                        Phase 3:
                        post-loop
                        runPhase()
                           │
                           ▼
                    Completion Summary
                    → Close executor


              executeStep()
                    │
          ┌─────────┴──────────────┐
          │                        │
     IsClaudeStep?           IsCommandStep?
     BuildPrompt()           ResolveCommand()
     claude CLI cmd          shell cmd argv
          │                        │
          └──────────┬─────────────┘
                     │
              ┌──────▼──────┐
              │ retry loop  │
              │             │
              │ SetActive   │
              │             │
        ┌─────┴─────┐       │
        │outputVar? │       │
        │           │       │
      yes           no      │
        │           │       │
  CaptureOutput  RunStep    │
  pool.Set()     (stream)   │
        │           │       │
        └────┬──────┘       │
             │              │
           success        failure
           SetDone     HandleStepError
           return         │
                    ┌─────┴──────┐
                    │            │
                  Quit        Continue
                  return      return
                           ActionRetry
                           → retry loop
```

## Key Files

| File | Purpose |
|------|---------|
| `ralph-tui/internal/workflow/run.go` | Run, runPhase, executeStep, phaseStepNames, RunConfig, RunHeader |
| `ralph-tui/internal/workflow/variables.go` | VariablePool: in-memory key-value store for workflow variables |
| `ralph-tui/internal/ui/orchestrate.go` | HandleStepError, error recovery state machine |
| `ralph-tui/internal/workflow/run_test.go` | Tests for Run lifecycle |
| `ralph-tui/internal/ui/orchestrate_test.go` | Tests for HandleStepError behavior |

## Core Types

```go
// RunConfig holds all parameters needed by Run.
type RunConfig struct {
    ProjectDir string
    Iterations int
    Config     *steps.WorkflowConfig  // three-phase step config (pre-loop/loop/post-loop)
}

// StepExecutor wraps StepRunner + CaptureOutput + Close.
// *Runner satisfies this interface.
type StepExecutor interface {
    ui.StepRunner
    CaptureOutput(command []string) (string, error)
    Close() error
}

// RunHeader updates the TUI status header during workflow execution.
// *ui.StatusHeader satisfies this interface.
type RunHeader interface {
    SetPhaseSteps(label string, names []string)  // switch to a new phase's step names
    SetStepState(idx int, state ui.StepState)
}

// VariablePool is a simple in-memory key-value store for workflow variables.
// Accessed only from Run()'s step loop (single goroutine) — no mutex needed.
type VariablePool struct { /* unexported */ }

func NewVariablePool() *VariablePool
func (vp *VariablePool) Set(name, value string)
func (vp *VariablePool) Get(name string) (string, bool)
func (vp *VariablePool) All() map[string]string       // shallow copy; mutations don't affect pool
func (vp *VariablePool) Clear(names []string)         // silently ignores absent keys

// LoopVariableNames returns the outputVariable names declared by loop-phase steps.
// Used by Run() to clear loop-scoped variables between iterations.
func LoopVariableNames(cfg *steps.WorkflowConfig) []string
```

## Implementation Details

### The Run Loop

`Run()` executes the full three-phase workflow lifecycle:

1. **Banner** — reads and displays `ralph-bash/ralph-art.txt`
2. **Phase 1: pre-loop** — calls `runPhase` for `cfg.Config.PreLoop`; these steps run once before any iteration
3. **Phase 2: loop** — for each iteration (1..N):
   - Clears loop-scoped variables from the pool via `LoopVariableNames` + `Clear`
   - Updates the header via `SetPhaseSteps("Iteration i/N", loopNames)`
   - Calls `executeStep` for each step in `cfg.Config.Loop`
   - If any step sets `exitLoopIfEmpty` and captures an empty string, the loop exits early
   - A completed iteration (no early exit) increments `iterationsRun`
4. **Phase 3: post-loop** — calls `runPhase` for `cfg.Config.PostLoop`; runs regardless of how the loop exited
5. **Completion summary** — logs the iteration count
6. **Close** — sends EOF to the log pipe

At every phase boundary and before every step, Run drains the Actions channel for a pending quit (injected by OS signals or keyboard) before proceeding.

### executeStep

`executeStep` is the single-step executor shared by all three phases. It resolves the command once before the retry loop, then executes it repeatedly until success or quit:

```go
func executeStep(executor StepExecutor, header RunHeader, keyHandler *ui.KeyHandler,
    idx int, step steps.Step, projectDir string, pool *VariablePool) (exitLoop bool, quit bool) {

    vars := pool.All()
    // Command resolution (once, before retry loop):
    if step.IsClaudeStep() {
        prompt, _ := steps.BuildPrompt(projectDir, step, vars)
        cmd = []string{"claude", "--permission-mode", step.DefaultPermissionMode(),
            "--model", step.DefaultModel(), "-p", prompt}
    } else {
        cmd = ResolveCommand(projectDir, step.Command, vars)
    }

    for {
        header.SetStepState(idx, ui.StepActive)

        if step.OutputVariable != "" {
            captured, err = executor.CaptureOutput(cmd)  // silent capture → pool
        } else {
            err = executor.RunStep(step.Name, cmd)        // streaming to TUI
        }

        if err == nil {
            header.SetStepState(idx, ui.StepDone)
            if step.OutputVariable != "" {
                pool.Set(step.OutputVariable, captured)
                if step.ExitLoopIfEmpty && strings.TrimSpace(captured) == "" {
                    return true, false  // exit the iteration loop
                }
            }
            return false, false
        }

        // Step failed — enter error recovery.
        action := ui.HandleStepError(executor, header, keyHandler, idx)
        switch action {
        case ui.ActionQuit:    return false, true
        case ui.ActionRetry:   executor.WriteToLog(ui.RetryStepSeparator(step.Name))
        case ui.ActionContinue: return false, false
        }
    }
}
```

### runPhase

`runPhase` sequences all steps in a phase slice, draining the quit channel before each step:

```go
func runPhase(executor StepExecutor, header RunHeader, keyHandler *ui.KeyHandler,
    phase []steps.Step, projectDir string, pool *VariablePool) (quit bool) {

    for i, step := range phase {
        select {
        case action := <-keyHandler.Actions:
            if action == ui.ActionQuit { return true }
        default:
        }
        _, q := executeStep(executor, header, keyHandler, i, step, projectDir, pool)
        if q { return true }
    }
    return false
}
```

### HandleStepError

`HandleStepError` (in `ui/orchestrate.go`) handles the error case after a step fails. If the step was user-terminated (WasTerminated), it marks the step done and returns `ActionContinue` immediately. Otherwise it enters error mode, blocks on the Actions channel, restores normal mode, and returns the action:

```go
func HandleStepError(runner StepRunner, header StepHeader, h *KeyHandler, stepIdx int) StepAction {
    if runner.WasTerminated() {
        header.SetStepState(stepIdx, StepDone)
        return ActionContinue
    }

    header.SetStepState(stepIdx, StepFailed)
    h.SetMode(ModeError)

    action := <-h.Actions  // blocks until user decides: c / r / q
    h.SetMode(ModeNormal)
    return action
}
```

The retry separator (`── step-name (retry) ─────────────`) is written by the caller (`executeStep`) before looping back, keeping `HandleStepError` focused on the error state machine.

## Testing

- `ralph-tui/internal/workflow/run_test.go` — Tests Run lifecycle with mock executor and header: phase sequencing, exitLoopIfEmpty, variable pool scoping, quit propagation
- `ralph-tui/internal/ui/orchestrate_test.go` — Tests HandleStepError: terminated step, error mode with continue/retry/quit actions

## Additional Information

- [Architecture Overview](../architecture.md) — System-level view of the orchestration flow with block diagrams
- [Step Definitions & Prompt Building](step-definitions.md) — How steps are loaded and prompts are built
- [Subprocess Execution & Streaming](subprocess-execution.md) — How RunStep executes subprocesses
- [CLI & Configuration](cli-configuration.md) — How ProjectDir and Iterations are parsed and passed to RunConfig
- [Keyboard Input & Error Recovery](keyboard-input.md) — How user decisions flow through the Actions channel
- [Signal Handling & Shutdown](signal-handling.md) — How ForceQuit injects ActionQuit for clean shutdown
- [TUI Status Header](tui-display.md) — How SetPhaseSteps and SetStepState update the header
- [File Logging](file-logging.md) — How step separator lines are written to the log file
- [ralph-tui Plan](../plans/ralph-tui.md) — Original specification including orchestration design
- [Concurrency](../coding-standards/concurrency.md) — Coding standards for channel-based dispatch and non-blocking drain
- [API Design](../coding-standards/api-design.md) — Coding standards for interface design and adapter types

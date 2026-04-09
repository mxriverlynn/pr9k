# Workflow Orchestration

Drives the entire ralph-tui workflow: iterating over GitHub issues, sequencing steps with error recovery, and running finalization tasks.

- **Last Updated:** 2026-04-09
- **Authors:**
  - River Bailey

## Overview

- `Run()` is the top-level orchestration goroutine that manages the full lifecycle: banner display, GitHub user lookup, iteration loop, finalization phase, and completion summary
- The iteration loop runs for exactly N iterations when `Iterations > 0`, or until no issue is found when `Iterations == 0` (unbounded / run-until-done mode)
- `Orchestrate()` sequences resolved steps, manages step state transitions (pending → active → done/failed), and handles interactive error recovery
- Iteration steps are resolved per-iteration with the current issue ID and commit SHA; finalization steps are resolved once
- The orchestration communicates with the keyboard handler via a `StepAction` channel for quit, continue, and retry decisions

Key files:
- `ralph-tui/internal/workflow/run.go` — Run function, RunConfig, buildIterationSteps, buildFinalizeSteps
- `ralph-tui/internal/ui/orchestrate.go` — Orchestrate function, ResolvedStep, error handling loop
- `ralph-tui/internal/workflow/run_test.go` — Unit tests for the Run orchestration loop
- `ralph-tui/internal/ui/orchestrate_test.go` — Unit tests for step sequencing and error recovery

## Architecture

```
                         Run()
                           │
              ┌────────────┼────────────────┐
              │            │                │
              ▼            ▼                ▼
         Display      Get GitHub      Iteration Loop
         Banner       Username         (1..N)
                                          │
                           ┌──────────────┼──────────────┐
                           │              │              │
                           ▼              ▼              ▼
                     get_next_issue  git rev-parse   buildIteration
                                       HEAD           Steps()
                                                        │
                                                        ▼
                                              ┌──────────────────┐
                                              │   Orchestrate()  │
                                              │                  │
                                              │  for each step:  │
                                              │   drain Actions  │
                                              │   set Active     │
                                              │   RunStep()      │
                                              │   handle result  │
                                              └────────┬─────────┘
                                                       │
                              ┌─────────────┬──────────┼──────────┐
                              │             │          │          │
                              ▼             ▼          ▼          ▼
                           success     terminated    failure     quit
                           → Done      → Done       → Failed    → return
                                       (skip)       → ModeError
                                                    → wait on
                                                      Actions:
                                                      c/r/q
                           │
                           ▼
                    Finalization Phase
                    (Orchestrate again)
                           │
                           ▼
                    Completion Summary
                    → Close executor
```

## Key Files

| File | Purpose |
|------|---------|
| `ralph-tui/internal/workflow/run.go` | Run loop, RunConfig, step resolution, header adapters |
| `ralph-tui/internal/ui/orchestrate.go` | Step sequencing, error recovery state machine |
| `ralph-tui/internal/workflow/run_test.go` | Tests for Run lifecycle |
| `ralph-tui/internal/ui/orchestrate_test.go` | Tests for Orchestrate behavior |

## Core Types

```go
// RunConfig holds all parameters needed by Run.
type RunConfig struct {
    ProjectDir    string
    Iterations    int
    Steps         []steps.Step
    FinalizeSteps []steps.Step
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
    SetIteration(current, total int, issueID, issueTitle string)
    SetStepState(idx int, state ui.StepState)
    SetFinalization(current, total int, steps []string)
    SetFinalizeStepState(idx int, state ui.StepState)
}

// ResolvedStep holds a step's name and its fully-resolved command argv.
type ResolvedStep struct {
    Name    string
    Command []string
}
```

## Implementation Details

### The Run Loop

`Run()` executes the full workflow lifecycle:

1. **Banner** — displays the embedded `ralph-art.txt` banner (compiled into the binary via `//go:embed`)
2. **GitHub username** — calls `scripts/get_gh_user` via `CaptureOutput`
3. **Iteration loop** — runs from i=1 upward, stopping when `i > Iterations` (bounded) or when no issue is found (unbounded, `Iterations == 0`):
   - Fetches the next issue via `scripts/get_next_issue`
   - If no issue found, exits the loop early (the only exit condition in unbounded mode)
   - Captures the current HEAD SHA
   - Updates the status header
   - Builds resolved steps via `buildIterationSteps`
   - Runs steps through `Orchestrate()`
   - If `Orchestrate` returns `ActionQuit`, closes and returns immediately
4. **Finalization** — runs even after early loop exit:
   - Switches the header to finalization mode
   - Builds resolved steps via `buildFinalizeSteps`
   - Runs through `Orchestrate()` with a `finalHeader` adapter
5. **Completion summary** — logs iteration count and finalization task count
6. **Close** — sends EOF to the log pipe

### Step Resolution

`buildIterationSteps` converts `[]Step` into `[]ResolvedStep` by either building a Claude CLI command or resolving a shell command:

```go
// Claude step → claude --permission-mode acceptEdits --model <model> -p <prompt>
result[i] = ui.ResolvedStep{
    Name:    s.Name,
    Command: []string{"claude", "--permission-mode", "acceptEdits", "--model", s.Model, "-p", prompt},
}

// Shell step → resolve template vars and script paths
result[i] = ui.ResolvedStep{
    Name:    s.Name,
    Command: ResolveCommand(projectDir, s.Command, issueID),
}
```

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

### Iteration Label Formatting

`iterationLabel` centralizes the progress display for both bounded and unbounded modes so the format is consistent across log messages and step separators:

```go
func iterationLabel(i, total int) string {
    if total > 0 {
        return fmt.Sprintf("Iteration %d/%d", i, total)
    }
    return fmt.Sprintf("Iteration %d", i)
}
```

It is called wherever an iteration identifier appears in log output: the "no issue found" early-exit message and the `StepSeparator` line written before each iteration's steps.

### Header Adapters

`iterHeader` and `finalHeader` adapt `RunHeader` to `StepHeader` so `Orchestrate` can update the correct row (iteration steps vs. finalization steps) without knowing which phase it's in:

```go
type iterHeader struct{ h RunHeader }
func (a *iterHeader) SetStepState(idx int, state ui.StepState) { a.h.SetStepState(idx, state) }

type finalHeader struct{ h RunHeader }
func (a *finalHeader) SetStepState(idx int, state ui.StepState) { a.h.SetFinalizeStepState(idx, state) }
```

## Testing

- `ralph-tui/internal/workflow/run_test.go` — Tests Run lifecycle with mock executor and header
- `ralph-tui/internal/ui/orchestrate_test.go` — Tests step sequencing, error recovery (continue/retry/quit), terminated step handling, pre-step quit drain

## Additional Information

- [Architecture Overview](../architecture.md) — System-level view of the orchestration flow with block diagrams
- [Building Custom Workflows](../how-to/building-custom-workflows.md) — How to create and modify workflow step sequences
- [Variable Output & Injection](../how-to/variable-output-and-injection.md) — How iteration variables are captured and injected into steps
- [Step Definitions & Prompt Building](step-definitions.md) — How steps are loaded and prompts are built
- [Subprocess Execution & Streaming](subprocess-execution.md) — How RunStep executes subprocesses
- [CLI & Configuration](cli-configuration.md) — How ProjectDir and Iterations are parsed and passed to RunConfig
- [Keyboard Input & Error Recovery](keyboard-input.md) — How user decisions flow through the Actions channel
- [Signal Handling & Shutdown](signal-handling.md) — How ForceQuit injects ActionQuit for clean shutdown
- [TUI Status Header](tui-display.md) — How step state updates are rendered
- [File Logging](file-logging.md) — How step separator lines are written to the log file
- [ralph-tui Plan](../plans/ralph-tui.md) — Original specification including orchestration design
- [Concurrency](../coding-standards/concurrency.md) — Coding standards for channel-based dispatch and non-blocking drain
- [API Design](../coding-standards/api-design.md) — Coding standards for adapter types used in header adapters

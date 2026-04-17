# Workflow Orchestration

Drives the entire ralph-tui workflow: running initialize steps, iterating over GitHub issues, sequencing steps with error recovery, running finalization tasks, and writing structured chrome into the log body (phase banners, step banners, capture logs, completion summary).

- **Last Updated:** 2026-04-17
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
- After the finalize phase, `Run` emits the run-level cumulative claude spend summary via `WriteRunSummary` (D13 2c), then writes a `CompletionSummary` line to the log body (not the header) and returns on its own — the workflow goroutine in `main.go` tears down the TUI and exits the process

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
// CaptureMode selects how a step's output is bound to LastCapture after it succeeds.
type CaptureMode int

const (
    CaptureLastLine CaptureMode = iota  // default: last non-empty stdout line (non-claude steps)
    CaptureResult                        // Aggregator.Result() from the claudestream pipeline (claude steps)
)

// StatusRunner is the interface for driving status-line refreshes from the
// workflow goroutine. *statusline.Runner satisfies this interface.
// A nil StatusRunner is safe: all push/trigger calls check for nil first.
type StatusRunner interface {
    PushState(statusline.State)
    Trigger()
}

// RunConfig holds all parameters needed by Run.
type RunConfig struct {
    WorkflowDir     string
    Iterations      int
    // Env is the per-workflow env allowlist loaded from the "env" field of
    // ralph-steps.json (StepFile.Env). Combined with sandbox.BuiltinEnvAllowlist
    // when building docker run args for claude steps.
    Env             []string
    InitializeSteps []steps.Step  // run once before the iteration loop
    Steps           []steps.Step  // run each iteration
    FinalizeSteps   []steps.Step  // run once after the loop
    // LogWidth is the column width used for full-width phase banner
    // underlines. 0 or negative falls back to ui.DefaultTerminalWidth.
    // main.go computes ui.TerminalWidth() - 2 (for rounded border glyphs)
    // and passes it here.
    LogWidth        int
    // RunStamp is the per-run identifier used to name the artifact directory
    // (e.g. "ralph-2026-04-14-173022.123"). Populated from Logger.RunStamp() in
    // main.go. When empty, JSONL artifact paths are not populated for claude
    // steps (persistence is skipped).
    RunStamp        string
    // Runner is the optional status-line runner. When nil, all PushState and
    // Trigger calls are skipped.
    Runner          StatusRunner
}

// RunResult holds the outcome of a completed Run call.
type RunResult struct {
    // IterationsRun is the index of the last iteration that began (1-based).
    // It includes the iteration that triggered a breakLoopIfEmpty exit.
    // Zero if the iteration loop never started.
    IterationsRun int
}

// StepExecutor wraps StepRunner + LastCapture + LastStats + ProjectDir + RunSandboxedStep + WriteRunSummary.
// *Runner satisfies this interface.
type StepExecutor interface {
    ui.StepRunner
    LastCapture() string                  // last non-empty stdout line (or Aggregator.Result() for claude steps)
    LastStats() claudestream.StepStats    // StepStats from the most recent RunSandboxedStep pipeline call
    ProjectDir() string                   // target repository directory used as cmd.Dir for every subprocess
    RunSandboxedStep(stepName string, command []string, opts SandboxOptions) error
    // WriteRunSummary writes line to both the TUI (via sendLine) and the file
    // logger. Used by Run() for the run-level cumulative summary (D13 2c) so
    // the total spend line is persisted to disk, unlike WriteToLog which is
    // TUI-only.
    WriteRunSummary(line string)
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
    Name        string
    Command     []string
    IsClaude    bool   // true for claude steps; routes to RunSandboxedStep
    CidfilePath string // docker cidfile path; passed to SandboxOptions.CidfilePath
    // ArtifactPath is the per-step .jsonl file path for claude steps (D14).
    // Empty for non-claude steps; the runner skips JSONL persistence when empty.
    ArtifactPath string
    // CaptureMode selects how LastCapture is populated after the step succeeds.
    // Zero value (CaptureLastLine) preserves current non-claude behaviour.
    CaptureMode CaptureMode
}
```

## Implementation Details

### Pre-Run Validation

Before `Run()` is called, `main.go` invokes `validator.Validate(workflowDir)` against `ralph-steps.json`. This covers all ten D13 validation categories — JSON parseability, schema shape per step, phase size, referenced file existence, variable scope resolution, env passthrough names, and sandbox isolation rules B and C — collecting every error in a single pass. If any errors are found, the process exits 1 and writes all structured errors to stderr before the TUI starts. This ensures every step's config is sound before any subprocess runs.

See [Config Validation](config-validation.md) for the full list of validation rules.

### Status-Line State Pushing

`buildState(vt, phase, sessionID, ver)` snapshots the current workflow state into a `statusline.State` value. It reads all seven built-in variables from `vt` using `GetInPhase` (phase-pure: does not consult `vt`'s internal phase field) and copies non-built-in user-defined captures via `vt.AllCaptures(phase)`. The returned `State` is a value type — the caller owns it and there are no shared references.

Inside `Run`, a local `push(phase)` closure wraps the two-step PushState+Trigger pattern:

```go
push := func(phase vars.Phase) {
    if cfg.Runner == nil { return }
    cfg.Runner.PushState(buildState(vt, phase, cfg.RunStamp, version.Version))
    cfg.Runner.Trigger()
}
```

Before the first `vt.SetPhase` call, one initial `PushState` (without a `Trigger`) is emitted so the timer goroutine never fires against a zero-value `State`:

```go
if cfg.Runner != nil {
    cfg.Runner.PushState(buildState(vt, vars.Initialize, cfg.RunStamp, version.Version))
}
```

`push` is then called after every `vt` mutation:

| Call site | Phase passed |
|-----------|-------------|
| After `vt.SetPhase(vars.Initialize)` | `vars.Initialize` |
| After `vt.SetPhase(vars.Iteration)` | `vars.Iteration` |
| After `vt.ResetIteration()` | `vars.Iteration` |
| After `vt.SetIteration(i)` | `vars.Iteration` |
| After `vt.SetStep(num, count, name)` | current phase |
| After `vt.Bind(phase, name, value)` | current phase |
| After `vt.SetPhase(vars.Finalize)` | `vars.Finalize` |

The invariant is: `triggers == len(pushes) − 1` (the initial seed push has no trigger). `cfg.Runner == nil` is safe on all paths.

A second trigger source exists outside the workflow goroutine: `ui.Model.Update` detects UI mode transitions and calls `cfg.Runner.Trigger()` once per mode change via `WithModeTrigger`. See [TUI Display: Mode-Change Status-Line Trigger](tui-display.md) for the implementation.

### The Run Loop

`Run()` executes the full workflow lifecycle in three phases, all driven by the `VarTable`. A local `emitBlank` closure writes a single blank separator line before every content block (iteration separator, Orchestrate call, phase banner, capture log, completion summary) — no-op on the very first call so the log does not begin with a blank line. A `writePhaseBanner` closure calls `emitBlank` then writes `PhaseBanner(name, logWidth)` to the log. A `writeCaptureLog` closure calls `emitBlank` then writes `CaptureLog(varName, value)`.

```go
func Run(executor StepExecutor, header RunHeader, keyHandler *ui.KeyHandler, cfg RunConfig) RunResult {
    vt := vars.New(cfg.WorkflowDir, executor.ProjectDir(), cfg.Iterations)

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

    // Phase 4: Completion — run-level summary, then completion line, then return
    for _, line := range claudestream.Renderer{}.FinalizeRun(rs.invocations, rs.retries, rs.total) {
        executor.WriteRunSummary(line)
    }
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
- Calls `Renderer{}.FinalizeRun(rs.invocations, rs.retries, rs.total)` and writes each returned line via `executor.WriteRunSummary` (D13 2c) — the run-level cumulative summary (e.g. `total claude spend across 7 step invocations: …`) is written to both the TUI and the file logger. Skipped when `rs.invocations == 0` (no claude steps ran)
- Calls `emitBlank` then writes `ui.CompletionSummary(iterationsRun, len(cfg.FinalizeSteps))` — `"Ralph completed after N iteration(s) and M finalizing tasks."` — as the **last non-blank line of the log body**. The header's `IterationLine` retains the final `"Finalizing N/M: <step name>"` value from the last finalize step; there is no header-level completion line
- Returns `RunResult{IterationsRun: iterationsRun}` — the caller (the workflow goroutine in `main.go`) then restores the terminal and exits the process

### Step Resolution

`buildStep` converts a single `Step` into a `ResolvedStep` by either building a Claude CLI command or resolving a shell command. Both paths use the `VarTable` for `{{VAR}}` substitution:

```go
func buildStep(workflowDir string, s steps.Step, vt *vars.VarTable, phase vars.Phase, env []string, executor StepExecutor) (ui.ResolvedStep, error) {
    if s.IsClaude {
        prompt, err := steps.BuildPrompt(workflowDir, s, vt, phase)
        if err != nil {
            return ui.ResolvedStep{}, fmt.Errorf("step %q: %w", s.Name, err)
        }
        uid, gid := sandbox.HostUIDGID()
        cidfile, err := sandbox.Path()
        if err != nil {
            return ui.ResolvedStep{}, fmt.Errorf("step %q: cidfile: %w", s.Name, err)
        }
        profileDir := preflight.ResolveProfileDir()
        projectDir := executor.ProjectDir()
        envAllowlist := append([]string{}, sandbox.BuiltinEnvAllowlist...)
        envAllowlist = append(envAllowlist, env...)
        argv := sandbox.BuildRunArgs(projectDir, profileDir, uid, gid, cidfile, envAllowlist, s.Model, prompt)
        return ui.ResolvedStep{
            Name:        s.Name,
            Command:     argv,
            IsClaude:    true,
            CidfilePath: cidfile,
        }, nil
    }
    return ui.ResolvedStep{
        Name:    s.Name,
        Command: ResolveCommand(workflowDir, s.Command, vt, phase),
    }, nil
}
```

After `buildStep` returns, `Run` populates `ArtifactPath` and `CaptureMode` for every `IsClaude == true` step before passing it to `Orchestrate`:

```go
if resolved.IsClaude {
    resolved.ArtifactPath = artifactPath(&resolved, phasePrefix, stepIdx)
    resolved.CaptureMode  = ui.CaptureResult
}
```

`artifactPath` is a local helper closure in `Run` that computes the per-step `.jsonl` path:

```
<projectDir>/logs/<runStamp>/<phasePrefix><stepIdx02d>-<slug>.jsonl
```

Phase prefixes: `"initialize-"`, `"iter<NN>-"` (1-indexed iteration), `"finalize-"`. Returns `""` when `cfg.RunStamp == ""` (persistence disabled) or the step is not a claude step.

The `VarTable` is created once at the start of `Run` and carries iteration-scoped variables (`ISSUE_ID`, `STARTING_SHA`) alongside persistent built-ins (`WORKFLOW_DIR`, `PROJECT_DIR`, `MAX_ITER`, `ITER`, `STEP_NUM`, `STEP_COUNT`, `STEP_NAME`) and any values bound by initialize-phase `captureAs` steps. At the start of each iteration, the table is reset and the new iteration's values are bound before step resolution runs.

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

### runStats

`runStats` accumulates `claudestream.StepStats` across all claude step invocations in a run (D21). It lives exclusively on `Run`'s stack frame — no mutex required:

```go
type runStats struct {
    invocations int
    retries     int
    total       claudestream.StepStats
}

func (rs *runStats) add(s claudestream.StepStats, isRetry bool) {
    rs.invocations++
    if isRetry { rs.retries++ }
    rs.total.InputTokens += s.InputTokens
    // ... all numeric StepStats fields accumulated ...
}
```

After the finalize phase completes, `Run` calls `claudestream.Renderer{}.FinalizeRun(rs.invocations, rs.retries, rs.total)` and writes each returned line via `executor.WriteRunSummary` (D13 2c). `FinalizeRun` returns nil when `rs.invocations == 0` (no claude steps ran), so no summary line appears for non-claude-only runs.

### stepDispatcher

`stepDispatcher` is an adapter that wraps `StepExecutor` and implements `ui.StepRunner` so that `Orchestrate` can call `runner.RunStep` uniformly across all phases. For a step marked `IsClaude=true`, `RunStep` transparently delegates to the wrapped executor's `RunSandboxedStep` with `SandboxOptions` populated from the current step's `CidfilePath`, `ArtifactPath`, and `CaptureMode`. Non-claude steps pass through to the executor's `RunStep` unchanged.

A new `stepDispatcher` is created for each step so that `current` always reflects the step about to be executed, and `prevFailed` is intentionally reset between steps (retries only count re-executions of the same step):

```go
type stepDispatcher struct {
    exec       StepExecutor
    current    ui.ResolvedStep
    stats      *runStats
    prevFailed bool  // true if the previous RunSandboxedStep returned an error
}

func (d *stepDispatcher) RunStep(name string, command []string) error {
    if d.current.IsClaude {
        err := d.exec.RunSandboxedStep(name, command, SandboxOptions{
            CidfilePath:  d.current.CidfilePath,
            ArtifactPath: d.current.ArtifactPath,
            CaptureMode:  d.current.CaptureMode,
        })
        // Fold stats regardless of outcome — D21: the spend was real.
        if d.stats != nil {
            d.stats.add(d.exec.LastStats(), d.prevFailed)
        }
        d.prevFailed = err != nil
        return err
    }
    d.prevFailed = false
    return d.exec.RunStep(name, command)
}
```

Each phase in `Run()` creates a fresh dispatcher per step and passes it as the `runner` to `Orchestrate`:

```go
// initialize phase
action := ui.Orchestrate([]ui.ResolvedStep{resolved}, &stepDispatcher{exec: executor, current: resolved, stats: rs}, noopHeader{}, keyHandler)

// iteration phase
action := ui.Orchestrate([]ui.ResolvedStep{resolved}, &stepDispatcher{exec: executor, current: resolved, stats: rs}, th, keyHandler)

// finalize phase
action := ui.Orchestrate([]ui.ResolvedStep{resolved}, &stepDispatcher{exec: executor, current: resolved, stats: rs}, &trackingOffsetIterHeader{h: header, idx: j}, keyHandler)
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
  - `TestRun_CompletionSummaryAndReturnsImmediately` — verifies the `ui.CompletionSummary` string is written to the log body via `executor.WriteToLog`, that it is the last non-blank line, and that `Run` returns on its own immediately without requiring any user input
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
  - `TestRun_ProjectDirFlowsIntoVarTable` — verifies `executor.ProjectDir()` flows into the VarTable as `PROJECT_DIR`, not `WorkflowDir`; a step with `command: ["echo", "{{PROJECT_DIR}}"]` receives the target repo path
  - `TestRun_WorkflowDirFlowsIntoVarTable` — verifies `cfg.WorkflowDir` flows into the VarTable as `WORKFLOW_DIR`, not `executor.ProjectDir()`; a step with `command: ["echo", "{{WORKFLOW_DIR}}"]` receives the install directory
  - `TestStepDispatcher_ClaudeStep_RoutesToRunSandboxedStep` (TP-001) — verifies `stepDispatcher.RunStep` dispatches a step with `IsClaude=true` to `RunSandboxedStep` with the correct `SandboxOptions.CidfilePath`; the underlying `RunStep` is not called
  - `TestStepDispatcher_NonClaudeStep_RoutesToRunStep` (TP-002) — verifies `stepDispatcher.RunStep` delegates a non-claude step to the underlying executor's `RunStep`; `RunSandboxedStep` is not called
  - `TestStepDispatcher_ClaudeStep_ForwardsCidfilePathToSandboxOptions` (TP-006) — verifies `ResolvedStep.CidfilePath` flows through `stepDispatcher.RunStep` into `SandboxOptions.CidfilePath` passed to `RunSandboxedStep`
  - `TestStepDispatcher_DelegatesWasTerminatedAndWriteToLog` (TP-007) — verifies `WasTerminated()` and `WriteToLog()` delegate to the wrapped executor unchanged
  - `TestStepDispatcher_ClaudeStep_PropagatesRunSandboxedStepError` (TP-011) — verifies errors from `RunSandboxedStep` flow through `stepDispatcher.RunStep` to the caller; prevents silent error swallowing that would bypass error-mode recovery in `Orchestrate`
  - `TestRun_InitializePhase_PassesEnvThroughBuildStep` (TP-008) — verifies `RunConfig.Env` is threaded through the initialize phase into `buildStep`, producing `-e` flags for the custom env var in the sandboxed step command
  - `TestRun_FinalizePhase_ClaudeStep_DispatchesToRunSandboxedStep` (TP-012) — verifies claude steps in the finalize phase route to `RunSandboxedStep`, confirming `stepDispatcher` wiring is consistent across initialize, iteration, and finalize phases
  - `TestRun_FinalizeCaptureAsIgnored` (WARN-004) — documents that the finalize phase intentionally does not call `vt.Bind()` after `Orchestrate`; a `captureAs` binding in a finalize step does not propagate to subsequent finalize steps (asymmetry with initialize/iteration is by design)
  - `TestRunStats_ZeroValue` — verifies all `runStats` fields start at zero for a zero-value struct
  - `TestRunStats_Add_AccumulatesAllFields` — verifies `runStats.add` correctly sums all seven numeric `StepStats` fields (InputTokens, OutputTokens, CacheCreationTokens, CacheReadTokens, NumTurns, TotalCostUSD, DurationMS) across two invocations, and increments `invocations` for each call
  - `TestRunStats_Add_RetryIncrement` — verifies `retries` increments only when `isRetry=true` and `invocations` always increments regardless
  - `TestStepDispatcher_ClaudeStep_FoldsStatsIntoRunStats` — verifies `LastStats()` is called once after `RunSandboxedStep` succeeds and all returned fields are folded into `runStats`
  - `TestStepDispatcher_ClaudeStep_FoldsStatsOnError` — verifies stats are folded into `runStats` even when `RunSandboxedStep` returns an error (D21: "the spend was real")
  - `TestStepDispatcher_ClaudeStep_RetryCountsOnSecondCallAfterError` — exercises the first-error → second-success retry path: asserts `invocations=2`, `retries=1`, and `prevFailed` cleared after success
  - `TestStepDispatcher_NonClaudeStep_ResetsRetryTracking` — verifies a non-claude step between two claude steps clears `prevFailed`, preventing spurious retry counts on the second claude step
  - `TestStepDispatcher_ClaudeStep_ForwardsArtifactPathAndCaptureMode` — verifies `ResolvedStep.ArtifactPath` and `ResolvedStep.CaptureMode` flow through `stepDispatcher.RunStep` into `SandboxOptions` passed to `RunSandboxedStep`
  - `TestRun_ClaudeStep_ArtifactPathInSandboxOptions` — verifies the full artifact path format `<projectDir>/logs/<runStamp>/iter01-01-<slug>.jsonl` is set in `SandboxOptions` for an iteration-phase claude step
  - `TestRun_ClaudeStep_EmptyRunStamp_NoArtifactPath` — verifies `ArtifactPath` is empty (persistence disabled) when `RunConfig.RunStamp == ""`
  - `TestRun_InitializePhase_ArtifactPathPrefix` — verifies the `"initialize-"` phase prefix appears in the artifact path for initialize-phase claude steps
  - `TestRun_FinalizePhase_ArtifactPathPrefix` — verifies the `"finalize-"` phase prefix appears in the artifact path for finalize-phase claude steps
  - `TestRun_ClaudeStep_CaptureModeIsResult` — verifies `CaptureMode=CaptureResult` is set in `SandboxOptions` for claude steps
  - `TestRun_NonClaudeStep_CaptureModeDefaultsToLastLine` — verifies non-claude steps never reach `RunSandboxedStep`; `CaptureLastLine` (zero value) is preserved by default
  - `TestBuildState_PopulatesAllFields` — all `State` fields correct from a seeded VarTable
  - `TestBuildState_PhaseStrings` — `Initialize`/`Iteration`/`Finalize` map to `"initialize"`/`"iteration"`/`"finalize"` strings
  - `TestBuildState_CapturesIsDefensiveCopy` — mutating the returned `Captures` map does not affect the VarTable
  - `TestBuildState_IterationPhaseIncludesIterCaptures` — iteration-scoped captures shadow persistent ones; finalize excludes them
  - `TestRun_StatusRunner_InitialPushNoTrigger` — initial `PushState` fires with no `Trigger`; all subsequent pushes include a trigger
  - `TestRun_StatusRunner_PushAtEveryMutationSite` — 11 `PushState` calls, 10 `Trigger` calls; correct phase sequence across a full bounded run
  - `TestRun_StatusRunner_NilRunnerNoPanic` — `cfg.Runner == nil` does not panic
  - `TestRun_StatusRunner_CaptureReflectedInNextPush` — a `Bind` value is visible in the next push's `Captures`
  - `TestRun_RunSummary_EmittedForClaudeSteps` — verifies the run-level cumulative summary line appears before `CompletionSummary`, contains expected token/cost fragments, and that `writeRunSummaryCalls == 1`
  - `TestRun_RunSummary_NotEmittedForNonClaudeSteps` — verifies no summary line is written when no claude steps ran (FinalizeRun returns nil for zero invocations)
  - `TestRun_RunSummary_MultipleClaudeStepsAccumulate` — verifies stats from two claude step invocations are accumulated (total cost and invocation count both reflected in the summary line)
- `ralph-tui/internal/ui/orchestrate_test.go` — Tests step sequencing, error recovery (continue/retry/quit), terminated step handling, pre-step quit drain, retry separator:
  - `TestOrchestrate_WritesStepStartBannerBeforeEachStep` — verifies heading, underline, and blank line are written to the log before each step runs
  - `TestOrchestrate_SetsStepActiveBeforeRunning` — verifies `SetStepState(Active)` is called before `RunStep` via a `callbackStubRunner`
  - `TestOrchestrate_Retry_StateTransitionSequence` — verifies the `Active→Failed→Done` state transition sequence on retry (note: `StepActive` is not re-set on retry — this is documented in the test)
  - `TestCaptureMode_ZeroValueIsCaptureLastLine` — documents the iota contract: `CaptureMode(0) == CaptureLastLine` and `CaptureLastLine != CaptureResult`; protects against silent breakage if iota ordering changes

## Additional Information

- [Status Line Feature](status-line.md) — Configuration, script contract, refresh trigger matrix, and lifecycle
- [Status Line Package](statusline.md) — Runner API, State, BuildPayload, Sanitize, and concurrency model
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

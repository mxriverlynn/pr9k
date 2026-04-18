# `internal/workflow` package

The `workflow` package contains `Runner` — the subprocess executor that drives non-claude and sandboxed Claude steps — and the `Run` loop that sequences initialize, iteration, and finalize phases.

## Runner

`Runner` executes steps and streams output to the TUI via a caller-supplied `sendLine` callback. It is created by `NewRunner` and satisfies the `StepExecutor` interface.

### RunStep / RunStepFull

```go
func (r *Runner) RunStep(stepName string, command []string) error
func (r *Runner) RunStepFull(stepName string, command []string, captureMode ui.CaptureMode) error
```

`RunStep` is the default entry point for non-claude steps; it delegates to `RunStepFull` with `ui.CaptureLastLine`. `RunStepFull` accepts an explicit `captureMode`:

| `captureMode` value | `LastCapture()` result |
|---------------------|------------------------|
| `ui.CaptureLastLine` (zero) | Last non-empty stdout line, whitespace-trimmed |
| `ui.CaptureFullStdout` | All stdout lines joined with `"\n"`, capped at 32 KiB |

The 32 KiB cap: content longer than 32 KiB is truncated to 30 KiB and the following marker is appended: `[...truncated, full content exceeds 32 KiB]`. The cut point is snapped backward with `utf8.RuneStart` to the nearest rune boundary so that multi-byte sequences are never split.

If the step exits non-zero, `LastCapture()` is always `""` regardless of mode.

### captureMode mapping

`buildStep` (in `run.go`) maps the JSON string field `steps.Step.CaptureMode` to `ui.CaptureMode`:

| JSON string | `ui.CaptureMode` |
|-------------|-----------------|
| `""` or absent | `ui.CaptureLastLine` |
| `"lastLine"` | `ui.CaptureLastLine` |
| `"fullStdout"` | `ui.CaptureFullStdout` |

Any other value causes `buildStep` to return an error (defense-in-depth; the validator is the primary gate). The validator also rejects `captureMode` on claude steps.

### LastCapture

```go
func (r *Runner) LastCapture() string
```

Returns the value bound during the most recent successful `RunStep`/`RunStepFull` or `RunSandboxedStep` call. The workflow `Run` loop calls this immediately after each captureAs step and stores the result in the VarTable.

### RunSandboxedStep

```go
func (r *Runner) RunSandboxedStep(stepName string, command []string, opts SandboxOptions) error
```

Executes a step inside Docker. `captureMode` is not applicable here — claude steps always bind `claudestream.Aggregator.Result()` via `SandboxOptions.CaptureMode == CaptureResult`.

## StepExecutor interface

`StepExecutor` (in `run.go`) is the full executor interface implemented by `*Runner`:

```go
type StepExecutor interface {
    ui.StepRunner
    LastCapture() string
    LastStats() claudestream.StepStats
    ProjectDir() string
    RunSandboxedStep(stepName string, command []string, opts SandboxOptions) error
    RunStepFull(stepName string, command []string, captureMode ui.CaptureMode) error
    WriteRunSummary(line string)
}
```

`ui.StepRunner` contributes `RunStep`, `WasTerminated`, and `WriteToLog`.

## stepDispatcher

`stepDispatcher` wraps `StepExecutor` and implements `ui.StepRunner` so that `Orchestrate` can call `runner.RunStep(name, command)` uniformly. For claude steps it transparently delegates to `RunSandboxedStep`; for non-claude steps it calls `RunStepFull(name, command, d.current.CaptureMode)`, forwarding the `CaptureMode` from the resolved step.

## Run loop

`Run` (in `run.go`) drives three phases — initialize, iteration, finalize — calling `buildStep` for each step to produce a `ui.ResolvedStep`, then wrapping it in a `stepDispatcher` and handing it to `ui.Orchestrate`. Captured values are bound to the VarTable after each `captureAs` step via `executor.LastCapture()`.

# `internal/workflow` package

The `workflow` package contains `Runner` — the subprocess executor that drives non-claude and sandboxed Claude steps — and the `Run` loop that sequences initialize, iteration, and finalize phases.

## Runner

`Runner` executes steps and streams output to the TUI via a caller-supplied `sendLine` callback. It is created by `NewRunner` and satisfies the `StepExecutor` interface.

### RunStep / RunStepFull

```go
func (r *Runner) RunStep(stepName string, command []string) error
func (r *Runner) RunStepFull(stepName string, command []string, captureMode ui.CaptureMode, timeoutSeconds int) error
```

`RunStep` is the default entry point for non-claude steps; it delegates to `RunStepFull` with `ui.CaptureLastLine` and `timeoutSeconds=0`. `RunStepFull` accepts an explicit `captureMode` and `timeoutSeconds`:

| `captureMode` value | `LastCapture()` result |
|---------------------|------------------------|
| `ui.CaptureLastLine` (zero) | Last non-empty stdout line, whitespace-trimmed |
| `ui.CaptureFullStdout` | All stdout lines joined with `"\n"`, capped at 32 KiB |

The 32 KiB cap: content longer than 32 KiB is truncated to 30 KiB and the following marker is appended: `[...truncated, full content exceeds 32 KiB]`. The cut point is snapped backward with `utf8.RuneStart` to the nearest rune boundary so that multi-byte sequences are never split.

When `timeoutSeconds > 0`, a goroutine is spawned that fires when the deadline expires:

- **Sandboxed (claude) steps** — invokes the cidfile-driven Terminator (`docker kill --signal=SIGTERM`), then `docker kill --signal=SIGKILL` after 10 seconds.
- **Host (non-claude) steps** — the child process is started with `Setpgid: true` so its process-group ID equals its PID. The goroutine uses `syscall.Kill(-proc.Pid, SIGTERM)` to signal the entire group, reaching any grandchildren that called `setsid`. After 10 seconds, `syscall.Kill(-proc.Pid, SIGKILL)` is sent.

After a timeout, `WasTimedOut()` returns `true` and `Run` sets `IterationRecord.Notes` to `"timed out after Ns"`.

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
    RunStepFull(stepName string, command []string, captureMode ui.CaptureMode, timeoutSeconds int) error
    SessionBlacklisted(id string) bool
    WasTimedOut() bool
    WriteRunSummary(line string)
}
```

`ui.StepRunner` contributes `RunStep`, `WasTerminated`, and `WriteToLog`.

`WasTimedOut()` returns `true` if the most recent step was ended by the per-step timeout goroutine. The flag is reset at the start of each `RunStepFull` or `RunSandboxedStep` call.

### Session blacklist

When a claude step times out and the claudestream pipeline has captured a `session_id`, that ID is added to an unexported map protected by `processMu`. Callers must not access this map directly; use the thread-safe accessors:

```go
func (r *Runner) SessionBlacklisted(id string) bool
func (r *Runner) BlacklistedSessions() []string
```

Both acquire `processMu` internally. Writing to the blacklist is only done inside `RunSandboxedStep` while holding `processMu`, so there is no data race.

## stepDispatcher

`stepDispatcher` wraps `StepExecutor` and implements `ui.StepRunner` so that `Orchestrate` can call `runner.RunStep(name, command)` uniformly. For claude steps it transparently delegates to `RunSandboxedStep` (passing `TimeoutSeconds` via `SandboxOptions`); for non-claude steps it calls `RunStepFull(name, command, d.current.CaptureMode, d.current.TimeoutSeconds)`, forwarding both fields from the resolved step.

When a step times out and the user chooses to retry, `stepDispatcher.RunStep` detects that `WasTimedOut()` is still `true` (the flag is reset only at the start of the next inner call) and invokes `onTimeoutRetry` to emit an `IterationRecord` for the timed-out attempt before the retry begins. This ensures the iteration log always has a record of timeout events even when the retry ultimately succeeds.

## Run loop

`Run` (in `run.go`) drives three phases — initialize, iteration, finalize — calling `buildStep` for each step to produce a `ui.ResolvedStep`, then wrapping it in a `stepDispatcher` and handing it to `ui.Orchestrate`. Captured values are bound to the VarTable after each `captureAs` step via `executor.LastCapture()`.

After every step (including prep failures), `Run` appends one `IterationRecord` to `.ralph-cache/iteration.jsonl`. See [Iteration log](#iteration-log) below.

## Iteration log

### IterationRecord

```go
type IterationRecord struct {
    SchemaVersion int     `json:"schema_version"` // always 1; bump on incompatible changes
    IssueID       string  `json:"issue_id"`
    IterationNum  int     `json:"iteration_num"`  // 0 for initialize/finalize phases
    StepName      string  `json:"step_name"`
    Model         string  `json:"model,omitempty"`
    Status        string  `json:"status"`         // "done" | "skipped" | "failed" | "unknown"
    DurationS     float64 `json:"duration_s"`
    InputTokens   int     `json:"input_tokens,omitempty"`
    OutputTokens  int     `json:"output_tokens,omitempty"`
    SessionID     string  `json:"session_id,omitempty"`
    Notes         string  `json:"notes,omitempty"` // prep error message when Status=="failed"
}
```

**Status values:**
| Value | Meaning |
|-------|---------|
| `"done"` | Step completed successfully (`StepDone` or `StepActive`) |
| `"failed"` | Step exited non-zero, or `buildStep` returned a prep error |
| `"skipped"` | Step was skipped (`StepSkipped`) |
| `"unknown"` | `SetStepState` was never called — step never started |

**Notes field:** populated in two cases: (1) `buildStep` prep error — value is the error string; (2) step timeout — value is `"timed out after Ns"`. Normal successful steps leave `notes` absent (`omitempty`).

### AppendIterationRecord

```go
func AppendIterationRecord(projectDir string, rec IterationRecord) error
```

Appends one JSON line to `<projectDir>/.ralph-cache/iteration.jsonl`. Safe for concurrent callers: O_APPEND writes under PIPE_BUF are atomic on POSIX. The caller is responsible for ensuring `.ralph-cache/` exists (preflight.Run guarantees this at startup). Write failures are non-fatal — `Run` logs a `warning:` line and continues.

**File location:** `<projectDir>/.ralph-cache/iteration.jsonl`
**Schema version:** `1` (in the `schema_version` field of every record)
**Lifecycle:** the file accumulates records for the entire run. The finalize step `lessons-learned.md` truncates it at the end of each run.

### captureStates and skipIfCaptureEmpty

`Run` maintains a `captureStates map[string]ui.StepState` for the iteration phase. After each step in an iteration, the map records the `StepState` of every step that has a `captureAs` name. The map is reset at the start of each iteration so captures from one iteration do not influence skip decisions in the next.

Before executing an iteration step that has `SkipIfCaptureEmpty` set, `Run` checks two conditions:

1. The named capture's current value in the VarTable is the empty string.
2. The source step's state in `captureStates` is `StepDone` (success).

If both conditions hold, the step is marked `StepSkipped`, a log line `"Step skipped (capture %q is empty)"` is emitted, an `IterationRecord` with `status: "skipped"` is appended to `iteration.jsonl`, and the iteration loop continues to the next step. If the source step failed (`StepFailed`) the skip logic does not apply — the step runs normally, guarding against silently ignoring a crash.

See [Skipping Steps Conditionally](../how-to/skipping-steps-conditionally.md) for usage examples and the fail-safe semantics.

### stateTracker / stepStatus helpers

`stateTracker` is a `ui.StepHeader` that records the last `StepState` without TUI output. It is used in the initialize phase (which uses `noopHeader` for display) so that `Run` can determine step success or failure for `IterationRecord.Status` after `Orchestrate` returns.

```go
func stepStatus(state ui.StepState) string
```

Maps a `ui.StepState` to the `IterationRecord.Status` string using explicit cases for all known states. `StepPending` (zero value — step never started) maps to `"unknown"` rather than `"done"` so that records from short-circuited call paths are distinguishable.

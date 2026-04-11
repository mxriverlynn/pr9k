# Subprocess Execution & Streaming

Executes workflow steps as subprocesses with real-time stdout/stderr streaming to both the TUI and a file logger, with support for graceful termination and per-step stdout capture.

- **Last Updated:** 2026-04-10
- **Authors:**
  - River Bailey

## Overview

- The `Runner` struct manages subprocess lifecycle: starting, streaming, terminating, and capturing output
- Subprocess output is forwarded line-by-line via a `sendLine` callback (installed via `SetSender`); the callback writes to a buffered channel consumed by a drain goroutine in `main.go` that batches lines into `LogLinesMsg` messages for the Bubble Tea TUI
- Two scanner goroutines (one for stdout, one for stderr) forward lines to both the pipe and the file logger, coordinated by a `sync.WaitGroup`; only the stdout goroutine captures lines for `LastCapture`
- After each successful `RunStep`, the last non-empty stdout line is stored and retrievable via `LastCapture()`; the orchestrator calls this to bind `CaptureAs` values into the `VarTable`
- `Terminate()` sends SIGTERM with a 3-second SIGKILL fallback; `WasTerminated()` lets the orchestrator distinguish user-initiated skips from genuine failures

Key files:
- `ralph-tui/internal/workflow/workflow.go` — `Runner` struct, `RunStep`, `Terminate`, `WriteToLog`, `LastCapture`, `CaptureOutput`
- `ralph-tui/internal/workflow/run.go` — `ResolveCommand` ({{VAR}} substitution + script path resolution)
- `ralph-tui/internal/workflow/workflow_test.go` — Unit tests for subprocess execution
- `ralph-tui/internal/workflow/run_test.go` — Integration tests for `LastCapture`, `CaptureOutput`, and `ResolveCommand`

## Architecture

```
                    ┌──────────────────────────────────┐
                    │             Runner                │
                    │                                   │
  RunStep()         │  ┌─────────────────────────────┐ │
  ──────────────────┼─▶│       exec.Command()        │ │
                    │  │       cmd.Dir = workDir      │ │
                    │  └──────────────┬──────────────┘ │
                    │                 │                 │
                    │         ┌───────┴───────┐         │
                    │         │               │         │
                    │      ┌──▼──┐        ┌──▼──┐      │
                    │      │stdout│       │stderr│      │
                    │      │ pipe │       │ pipe │      │
                    │      └──┬──┘        └──┬──┘      │
                    │         │               │         │
                    │  ┌──────▼───────────────▼──────┐ │
                    │  │      scanner goroutines      │ │
                    │  │  capture=true (stdout)       │ │
                    │  │  capture=false (stderr)      │ │
                    │  │  + sync.WaitGroup            │ │
                    │  └──────┬──────────┬────────────┘ │
                    │         │          │               │
                    │         ▼          ▼               │
                    │  sendLine(line)  Logger            │
  Terminate()       │  (mu snapshot-   (file)            │
  ──────────────────┼─▶ then-unlock)                    │
  SIGTERM→SIGKILL   │                                   │
                    │   lastCapture                      │
                    │   (stdout only,                    │
                    │    on success)                     │
                    └──────┼─────────────────────────────┘
                           │
                           ▼
                     buffered channel → drain goroutine
                           │
                           ▼
                     program.Send(LogLinesMsg)
                           │
                           ▼
                     Bubble Tea TUI (SetSender)
```

## Key Files

| File | Purpose |
|------|---------|
| `ralph-tui/internal/workflow/workflow.go` | `Runner` struct, `RunStep`, `Terminate`, `WriteToLog`, `LastCapture`, `CaptureOutput` |
| `ralph-tui/internal/workflow/run.go` | `ResolveCommand` — `{{VAR}}` substitution and script path resolution |
| `ralph-tui/internal/workflow/workflow_test.go` | Tests for `RunStep`, `Terminate`, `WasTerminated`, `WriteToLog`, `Close`, and `SetSender` |
| `ralph-tui/internal/workflow/run_test.go` | Integration tests for `LastCapture`, `CaptureOutput`, `ResolveCommand` |

## Core Types

```go
// Runner executes workflow steps and forwards subprocess output via a sendLine callback.
type Runner struct {
    mu         sync.Mutex     // protects sendLine
    log        *logger.Logger // file logger
    workingDir string         // cmd.Dir for every subprocess
    sendLine   func(string)   // callback invoked for every forwarded line; never nil

    // processMu guards currentProc, procDone, and terminated.
    processMu   sync.Mutex
    currentProc *os.Process
    procDone    chan struct{} // closed when subprocess exits
    terminated  bool         // set by Terminate(), reset at start of RunStep

    // lastCapture holds the last non-empty stdout line from the most recent
    // successful RunStep. Empty string if the last step failed or produced no output.
    lastCapture string
}
```

`SetSender` installs a callback that is invoked for every line forwarded through `forwardPipe` and `WriteToLog`. If `send` is nil, a no-op is installed. The callback must not panic and must not block — it is called synchronously inside scanner goroutines, so a blocking callback stalls subprocess output and a panicking callback crashes the process:

```go
func (r *Runner) SetSender(send func(string)) {
    if send == nil {
        send = func(string) {}
    }
    r.mu.Lock()
    r.sendLine = send
    r.mu.Unlock()
}
```

## Implementation Details

### Subprocess Streaming (RunStep)

`RunStep` starts a subprocess, creates two scanner goroutines to forward stdout and stderr, and waits for both to drain before calling `cmd.Wait()`. Only the stdout goroutine captures lines for `LastCapture`:

```go
func (r *Runner) RunStep(stepName string, command []string) error {
    r.processMu.Lock()
    r.terminated = false  // reset for this step
    r.processMu.Unlock()

    cmd := exec.Command(command[0], command[1:]...)
    cmd.Dir = r.workingDir

    stdout, _ := cmd.StdoutPipe()
    stderr, _ := cmd.StderrPipe()
    cmd.Start()

    // Track process for Terminate()
    r.processMu.Lock()
    r.currentProc = cmd.Process
    r.procDone = done
    r.processMu.Unlock()

    var capturedLines []string
    var wg sync.WaitGroup
    wg.Add(2)
    go forwardPipe(stdout, true)   // capture=true: accumulates capturedLines
    go forwardPipe(stderr, false)  // capture=false: forwards only, no capture
    wg.Wait()                      // drain before Wait

    waitErr := cmd.Wait()
    if waitErr == nil {
        r.lastCapture = lastNonEmptyLine(capturedLines)
    } else {
        r.lastCapture = ""
    }
    return waitErr
}
```

Each scanner goroutine uses a 256KB buffer to handle long lines:

```go
scanner := bufio.NewScanner(pipe)
buf := make([]byte, 256*1024)
scanner.Buffer(buf, 256*1024)
```

The `sendLine` callback is snapshotted under `r.mu` and invoked after the lock is released (snapshot-then-unlock) to prevent TOCTOU races while keeping the critical section short. The file logger is written to under its own internal mutex.

### Per-Step Stdout Capture (LastCapture)

After each successful `RunStep`, `Runner` stores the last non-empty stdout line. The orchestrator calls `LastCapture()` to retrieve it and bind it into the VarTable when a step has `CaptureAs` set:

```go
// LastCapture returns the last non-empty stdout line from the most recent
// successful RunStep call, stripped of trailing carriage returns and whitespace.
// Returns "" if the last step failed or produced no non-empty stdout output.
func (r *Runner) LastCapture() string {
    return r.lastCapture
}
```

`lastNonEmptyLine` walks the captured slice in reverse, trims trailing `\r` and whitespace, and returns the first non-empty line found. Stderr lines are never captured — `forwardPipe` is called with `capture=false` for stderr, so only stdout contributes to `LastCapture`.

`LastCapture()` is part of the `StepExecutor` interface. `CaptureOutput` is not — it exists only as a concrete method on `*Runner` (see below).

### Graceful Termination

`Terminate()` sends SIGTERM to the active subprocess. If the process hasn't exited within 3 seconds, SIGKILL is sent:

```go
func (r *Runner) Terminate() {
    r.processMu.Lock()
    proc := r.currentProc
    done := r.procDone
    r.terminated = true
    r.processMu.Unlock()

    if proc == nil { return }  // no-op when idle

    _ = proc.Signal(syscall.SIGTERM)

    select {
    case <-done:                        // process exited
    case <-time.After(3 * time.Second): // timeout → force kill
        _ = proc.Kill()
    }
}
```

`WasTerminated()` reports whether the most recent `RunStep` was ended by a `Terminate()` call. The flag is reset at the start of each `RunStep`. The orchestrator uses this to distinguish user-initiated skips (step marked done) from genuine failures (enters error mode).

### Direct Log Injection (WriteToLog)

`WriteToLog` writes a single line directly via the `sendLine` callback, without running a subprocess. Used for step separator lines between subprocess outputs. Note: lines written via `WriteToLog` appear in the TUI log panel but are **not** written to the file logger:

```go
func (r *Runner) WriteToLog(line string) {
    r.mu.Lock()
    send := r.sendLine
    r.mu.Unlock()
    send(line)
}
```

### Single-Value Output Capture (CaptureOutput)

`CaptureOutput` runs a command and returns trimmed stdout as a string. Stderr is discarded. It is a concrete method on `*Runner` but is **not part of the `StepExecutor` interface** — it is not called from the workflow run loop. Single-value lookups that previously used `CaptureOutput` directly (GitHub username, issue ID, HEAD SHA) are now configured as initialize-phase steps with `CaptureAs`, handled via `RunStep` + `LastCapture`:

```go
func (r *Runner) CaptureOutput(command []string) (string, error) {
    cmd := exec.Command(command[0], command[1:]...)
    cmd.Dir = r.workingDir
    out, err := cmd.Output()
    return strings.TrimSpace(string(out)), err
}
```

### Command Resolution (ResolveCommand)

`ResolveCommand` lives in `run.go` and prepares command arrays for execution by substituting `{{VAR}}` tokens via the `VarTable` and resolving relative script paths:

```go
func ResolveCommand(projectDir string, command []string, vt *vars.VarTable, phase vars.Phase) []string {
    result := make([]string, len(command))
    for i, arg := range command {
        substituted, _ := vars.Substitute(arg, vt, phase)
        result[i] = substituted
    }
    // Resolve the executable if it looks like a relative script path.
    exe := result[0]
    if !filepath.IsAbs(exe) && strings.ContainsRune(exe, '/') {
        result[0] = filepath.Join(projectDir, exe)
    }
    return result
}
```

Bare commands like `git` are not resolved — only relative paths containing a `/` separator are joined with `projectDir`.

## Concurrency

| Resource | Protection | Why |
|----------|-----------|-----|
| `sendLine` callback | `mu sync.Mutex` (snapshot-then-unlock) | Snapshotted under `mu`, called after unlock; prevents TOCTOU race when `SetSender` swaps the callback concurrently with scanner goroutines reading it |
| `currentProc`, `procDone`, `terminated` | `processMu sync.Mutex` | Accessed by RunStep (main goroutine) and Terminate (keyboard/signal goroutine) |
| `WaitGroup` drain before `cmd.Wait()` | `sync.WaitGroup` | Ensures all pipe output is forwarded before the process exit status is collected |

## Error Handling

| Scenario | Error Message | Behavior |
|----------|---------------|----------|
| StdoutPipe creation fails | `"workflow: stdout pipe: ..."` | Returned to caller |
| StderrPipe creation fails | `"workflow: stderr pipe: ..."` | Returned to caller |
| Command start fails | `"workflow: start %q: ..."` | Returned to caller |
| Scanner error during streaming | logged as `"scanner error: ..."` | Warning only; does not fail the step |
| Logger write error during streaming | logged as `"logger error: ..."` | First error logged, subsequent writes skipped |

## Testing

- `ralph-tui/internal/workflow/workflow_test.go` — Tests for `RunStep`, `Terminate`, `WasTerminated`, `WriteToLog`, `Close`, `ResolveCommand`, and `SetSender`:
  - `TestResolveCommand_*` — 10 tests covering `{{VAR}}` substitution, script path resolution, immutability, empty slice, bare command passthrough
  - `TestRunStep_SendLineReceivesStdout`, `TestRunStep_SendLineReceivesStderr` — sendLine receives stdout and stderr lines
  - `TestRunStep_SendLineBurstOrdering` — lines arrive in order under burst load
  - `TestRunStep_SetSenderNilInstallsNoOp` — nil sender installs a no-op
  - `TestRunStep_SetSenderReplacementTakesEffect` — replacement is reflected immediately
  - `TestWriteToLog_SendLineInvoked` — WriteToLog path invokes sendLine
  - `TestRunStep_ConcurrentSetSenderNoRace`, `TestRunStep_ConcurrentStdoutStderrSenderNoRace` — race-detector tests for concurrent sender swaps and concurrent stdout/stderr goroutines
  - `TestRunStep_SendLineAfterTerminateNoPanic` — sendLine calls survive Terminate without panic
  - `TestRunStep_SendLineDefaultNoOp`, `TestWriteToLog_DefaultNoOpSendLineNoPanic` — default no-op installed by NewRunner does not panic
  - `TestWriteToLog_AfterCloseSendLineStillInvoked` — sendLine fires even after Close
  - `TestSetSender_AtomicReplacementViaWriteToLog` — atomic replacement via WriteToLog
  - `TestWriteToLog_DoesNotWriteToFileLogger` — verifies WriteToLog forwards to sendLine but does not write to the file logger
- `ralph-tui/internal/workflow/run_test.go` — Integration tests for:
  - `TestLastCapture_LastNonEmptyStdoutLine` — verifies last non-empty stdout line is returned
  - `TestLastCapture_EmptyOnFailure` — verifies `""` is returned after a failed step
  - `TestLastCapture_StripsTrailingCarriageReturn` — verifies trailing `\r` is stripped
  - `TestLastCapture_StderrNotCaptured` — verifies stderr output does not appear in `LastCapture`
  - `TestCaptureOutput_UsesWorkingDir` — verifies `CaptureOutput` sets `cmd.Dir`
  - `TestBuildStep_*` — tests for `buildStep` including Claude and shell step variants

## Additional Information

- [Architecture Overview](../architecture.md) — System-level view showing how streaming fits into the data flow
- [Variable Output & Injection](../how-to/variable-output-and-injection.md) — How `ResolveCommand` and `LastCapture` fit into the variable injection system
- [Building Custom Workflows](../how-to/building-custom-workflows.md) — How shell command steps use `ResolveCommand` for path resolution
- [Workflow Orchestration](workflow-orchestration.md) — How `RunStep` and `LastCapture` are called by the orchestration loop
- [Variable State Management](variable-state.md) — How `CaptureAs` bindings use `LastCapture` to populate the `VarTable`
- [Step Definitions & Prompt Building](step-definitions.md) — How steps are loaded and prompts are built before execution
- [Keyboard Input & Error Recovery](keyboard-input.md) — How `Terminate` is triggered by keyboard input
- [Signal Handling & Shutdown](signal-handling.md) — How `Terminate` is triggered by OS signals
- [File Logging](file-logging.md) — The logger that receives forwarded subprocess output
- [CLI & Configuration](cli-configuration.md) — How `ProjectDir` sets the working directory for all subprocesses
- [Concurrency](../coding-standards/concurrency.md) — Coding standards for mutex-protected writes, WaitGroup drain, and sendLine callback patterns
- [Error Handling](../coding-standards/error-handling.md) — Coding standards for scanner error checking and goroutine write error tracking
- [Go Patterns](../coding-standards/go-patterns.md) — Coding standard for 256KB scanner buffer sizing

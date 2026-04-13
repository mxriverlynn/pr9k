# Subprocess Execution & Streaming

Executes workflow steps as subprocesses with real-time stdout/stderr streaming to both the TUI and a file logger, with support for graceful termination and per-step stdout capture.

- **Last Updated:** 2026-04-12
- **Authors:**
  - River Bailey

## Overview

- The `Runner` struct manages subprocess lifecycle: starting, streaming, terminating, and capturing output
- Subprocess output is forwarded line-by-line via a `sendLine` callback (installed via `SetSender`); the callback writes to a buffered channel consumed by a drain goroutine in `main.go` that batches lines into `LogLinesMsg` messages for the Bubble Tea TUI
- Two scanner goroutines (one for stdout, one for stderr) forward lines to both the pipe and the file logger, coordinated by a `sync.WaitGroup`; only the stdout goroutine captures lines for `LastCapture`
- After each successful `RunStep`, the last non-empty stdout line is stored and retrievable via `LastCapture()`; the orchestrator calls this to bind `CaptureAs` values into the `VarTable`
- `Terminate()` sends SIGTERM with a 3-second SIGKILL fallback (via `terminateGracePeriod`); for sandboxed steps it dispatches through an installed terminator closure rather than signaling the host process directly; `WasTerminated()` lets the orchestrator distinguish user-initiated skips from genuine failures
- `RunSandboxedStep` executes a command inside a Docker sandbox, installing a terminator closure and using explicit empty stdin to prevent raw-mode keyboard inheritance; the cidfile is cleaned up (ENOENT-tolerant) after the step exits

Key files:
- `ralph-tui/internal/workflow/workflow.go` — `Runner` struct, `RunStep`, `Terminate`, `WriteToLog`, `LastCapture`, `CaptureOutput`
- `ralph-tui/internal/workflow/run.go` — `ResolveCommand` ({{VAR}} substitution + script path resolution)
- `ralph-tui/internal/workflow/workflow_test.go` — Unit tests for subprocess execution
- `ralph-tui/internal/workflow/run_test.go` — Integration tests for `LastCapture` and `CaptureOutput`

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
| `ralph-tui/internal/workflow/workflow_test.go` | Tests for `RunStep`, `Terminate`, `WasTerminated`, `WriteToLog`, and `SetSender` |
| `ralph-tui/internal/workflow/run_test.go` | Integration tests for `LastCapture`, `CaptureOutput`, `ResolveCommand` |

## Core Types

```go
// Runner executes workflow steps and forwards subprocess output via a sendLine callback.
type Runner struct {
    mu         sync.Mutex     // protects sendLine and lastCapture
    log        *logger.Logger // file logger
    projectDir string         // cmd.Dir for every subprocess (target repository)
    sendLine   func(string)   // callback invoked for every forwarded line; never nil

    // processMu guards currentProc, currentTerminator, procDone, and terminated.
    processMu         sync.Mutex
    currentProc       *os.Process
    currentTerminator func(syscall.Signal) error // nil for plain RunStep; set by RunSandboxedStep
    procDone          chan struct{}               // closed when subprocess exits
    terminated        bool                       // set by Terminate(), reset at start of RunStep/RunSandboxedStep

    // terminateGraceOverride, when non-zero, replaces terminateGracePeriod in
    // Terminate(). Used in tests to avoid waiting the full 3 seconds.
    terminateGraceOverride time.Duration

    // lastCapture holds the last non-empty stdout line from the most recent
    // successful RunStep call. Empty string if the last step failed or produced no output.
    // Protected by mu; written by runCommand, read by LastCapture.
    lastCapture string
}

// SandboxOptions carries the sandbox-specific parameters for RunSandboxedStep.
type SandboxOptions struct {
    // Terminator, when non-nil, is called by Runner.Terminate() instead of
    // signaling the host process directly. It receives SIGTERM first; if the
    // process does not exit within the grace period, it receives SIGKILL.
    Terminator func(syscall.Signal) error
    // CidfilePath is the path of the Docker --cidfile to clean up after the
    // step exits. May be empty. Cleanup is ENOENT-tolerant.
    CidfilePath string
}
```

`terminateGracePeriod` is the package-level constant (3 seconds) controlling the SIGTERM→SIGKILL escalation window:

```go
const terminateGracePeriod = 3 * time.Second
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
    if len(command) == 0 {
        return fmt.Errorf("workflow: RunStep %q: empty command", stepName)
    }

    r.processMu.Lock()
    r.terminated = false  // reset for this step
    r.processMu.Unlock()

    cmd := exec.Command(command[0], command[1:]...)
    cmd.Dir = r.projectDir

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
    r.mu.Lock()
    defer r.mu.Unlock()
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
    term := r.currentTerminator
    done := r.procDone
    r.terminated = true
    r.processMu.Unlock()

    if proc == nil { return }  // no-op when idle

    grace := terminateGracePeriod
    if r.terminateGraceOverride > 0 {
        grace = r.terminateGraceOverride
    }

    if term != nil {
        // Sandboxed step: dispatch through the terminator closure (e.g. docker kill)
        // rather than signaling the host docker CLI process directly.
        _ = term(syscall.SIGTERM)
        select {
        case <-done:
        case <-time.After(grace):
            _ = term(syscall.SIGKILL)
        }
    } else {
        _ = proc.Signal(syscall.SIGTERM)
        select {
        case <-done:                    // process exited
        case <-time.After(grace):       // timeout → force kill
            _ = proc.Kill()
        }
    }
}
```

`WasTerminated()` reports whether the most recent `RunStep` or `RunSandboxedStep` was ended by a `Terminate()` call. The flag is reset at the start of each `RunStep`/`RunSandboxedStep`. The orchestrator uses this to distinguish user-initiated skips (step marked done) from genuine failures (enters error mode).

### Sandboxed Step Execution (RunSandboxedStep)

`RunSandboxedStep` runs a command inside a Docker sandbox. It differs from `RunStep` in three ways: it installs `opts.Terminator` so that `Terminate()` dispatches signals through the container (not the host `docker` CLI process), it provides explicit empty stdin (`bytes.NewReader(nil)`) to prevent raw-mode keyboard inheritance, and it cleans up the cidfile at `opts.CidfilePath` after the step exits (ENOENT-tolerant via `sandbox.Cleanup`):

```go
func (r *Runner) RunSandboxedStep(stepName string, command []string, opts SandboxOptions) error {
    if len(command) == 0 {
        return fmt.Errorf("workflow: RunSandboxedStep %q: empty command", stepName)
    }

    r.processMu.Lock()
    r.terminated = false
    r.currentTerminator = opts.Terminator
    r.processMu.Unlock()

    defer func() {
        _ = sandbox.Cleanup(opts.CidfilePath)
    }()

    return r.runCommand(stepName, command, bytes.NewReader(nil))
}
```

### Shared Subprocess Core (runCommand)

`runCommand` is the private shared core used by both `RunStep` and `RunSandboxedStep`. It starts the subprocess, creates two scanner goroutines, waits for both pipes to drain, and then calls `cmd.Wait()`. A key correctness detail: the `currentTerminator` is cleared **before** the `procDone` channel is closed, so any `Terminate()` racing with natural step completion observes a nil terminator and short-circuits instead of dispatching a stale signal:

```go
defer func() {
    r.processMu.Lock()
    r.currentTerminator = nil
    r.processMu.Unlock()
    close(done)
    r.processMu.Lock()
    r.currentProc = nil
    r.processMu.Unlock()
}()
```

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
    if len(command) == 0 {
        return "", fmt.Errorf("workflow: CaptureOutput: empty command")
    }
    cmd := exec.Command(command[0], command[1:]...)
    cmd.Dir = r.projectDir
    out, err := cmd.Output()
    return strings.TrimSpace(string(out)), err
}
```

### Command Resolution (ResolveCommand)

`ResolveCommand` lives in `run.go` and prepares command arrays for execution by substituting `{{VAR}}` tokens via the `VarTable` and resolving relative script paths:

```go
func ResolveCommand(workflowDir string, command []string, vt *vars.VarTable, phase vars.Phase) []string {
    if len(command) == 0 {
        return command
    }

    result := make([]string, len(command))
    for i, arg := range command {
        substituted, _ := vars.Substitute(arg, vt, phase)
        result[i] = substituted
    }
    // Resolve the executable if it looks like a relative script path.
    exe := result[0]
    if !filepath.IsAbs(exe) && strings.ContainsRune(exe, '/') {
        result[0] = filepath.Join(workflowDir, exe)
    }
    return result
}
```

Bare commands like `git` are not resolved — only relative paths containing a `/` separator are joined with `workflowDir`.

## Concurrency

| Resource | Protection | Why |
|----------|-----------|-----|
| `sendLine` callback | `mu sync.Mutex` (snapshot-then-unlock) | Snapshotted under `mu`, called after unlock; prevents TOCTOU race when `SetSender` swaps the callback concurrently with scanner goroutines reading it |
| `lastCapture` | `mu sync.Mutex` | Written by `RunStep` after `wg.Wait()`, read by `LastCapture()`; both hold `mu` to prevent data races between concurrent callers |
| `currentProc`, `currentTerminator`, `procDone`, `terminated` | `processMu sync.Mutex` | Accessed by RunStep/RunSandboxedStep (main goroutine) and Terminate (keyboard/signal goroutine); `currentTerminator` is cleared before `close(done)` to prevent stale signal dispatch when Terminate races with natural step completion |
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

- `ralph-tui/internal/workflow/workflow_test.go` — Tests for `RunStep`, `RunSandboxedStep`, `Terminate`, `WasTerminated`, `WriteToLog`, `ResolveCommand`, `SetSender`, `NewRunner`, and empty-command guards:
  - `TestResolveCommand_*` — 10 tests covering `{{VAR}}` substitution, script path resolution, immutability, empty slice, bare command passthrough
  - `TestSetSender_ForwardsEveryStdoutLine`, `TestSetSender_ForwardsEveryStderrLine` — sendLine receives stdout and stderr lines
  - `TestSetSender_BurstDoesNotDropOrReorder` — lines arrive in order under burst load
  - `TestSetSender_NilIsTreatedAsNoop` — nil sender installs a no-op
  - `TestSetSender_CalledBeforeAndAfterRunStep` — replacement is reflected immediately
  - `TestWriteToLog_ForwardsToSender` — WriteToLog path invokes sendLine
  - `TestRunStep_ConcurrentSetSenderNoRace`, `TestRunStep_ConcurrentStdoutStderrSenderNoRace` — race-detector tests for concurrent sender swaps and concurrent stdout/stderr goroutines
  - `TestRunStep_SendLineAfterTerminateNoPanic` — sendLine calls survive Terminate without panic
  - `TestRunStep_DefaultNoOpSendLineNoPanic`, `TestWriteToLog_DefaultNoOpSendLineNoPanic` — after `SetSender(nil)` the no-op sender does not panic
  - `TestSetSender_AtomicReplacementViaWriteToLog` — atomic replacement via WriteToLog
  - `TestWriteToLog_DoesNotWriteToFileLogger` — verifies WriteToLog forwards to sendLine but does not write to the file logger
  - `TestNewRunner_WriteToLogWithoutSetSenderPanics*` — verifies that calling WriteToLog before SetSender panics with a descriptive message
  - `TestRunStep_ReturnsErrorForEmpty*` — verifies that RunStep returns an error for empty and nil command slices
  - `TestCaptureOutput_ReturnsErrorForEmptyCommandSlice`, `TestCaptureOutput_ReturnsErrorForNilCommand` — verifies CaptureOutput returns an error for empty and nil command slices
  - `TestRunSandboxedStep_ReturnsErrorForEmptyCommandSlice`, `TestRunSandboxedStep_ReturnsErrorForNilCommand` — verifies RunSandboxedStep returns an error for empty and nil command slices
  - `TestRunSandboxedStep_OutputForwarding` — verifies stdout/stderr are forwarded via sendLine
  - `TestRunSandboxedStep_LastCapturePopulation` — verifies last non-empty stdout line is stored after a successful sandboxed step
  - `TestRunSandboxedStep_CleansCidfileOnError` — verifies cidfile is removed even when the command exits non-zero
  - `TestRunSandboxedStep_ResetTerminatedFlag` — verifies `terminated` is reset at the start of `RunSandboxedStep`
  - `TestRunSandboxedStep_TerminatorClearedBeforeWaitReturn` — verifies `currentTerminator` is cleared before `procDone` is closed (prevents stale signal dispatch on natural exit)
  - `TestRunSandboxedStep_ReturnsErrorForNonExistentCommand` — verifies error is returned when the command does not exist
  - `TestRunSandboxedStep_ReturnsErrorOnNonZeroExit` — verifies error is returned for a non-zero exit code
  - `TestRunSandboxedStep_UsesProjectDir` — verifies `cmd.Dir` is set to `projectDir`
  - `TestTerminate_UsesTerminatorWhenInstalled` — verifies Terminate dispatches SIGTERM+SIGKILL through the terminator closure when one is installed
  - `TestTerminate_UsesProcessWhenNoTerminator` — verifies Terminate signals the host process directly when no terminator is installed
  - `TestTerminate_TerminatorSIGTERMOnlyWhenProcessExitsPromptly` — verifies SIGKILL is not sent when the process exits within the grace period
  - `TestTerminate_IntegrationOrchestrationCanProceed` — integration test verifying that after Terminate() the runner is in a consistent state and can process the next step
- `ralph-tui/internal/workflow/run_test.go` — Integration tests for:
  - `TestLastCapture_LastNonEmptyStdoutLine` — verifies last non-empty stdout line is returned
  - `TestLastCapture_EmptyOnFailure` — verifies `""` is returned after a failed step
  - `TestLastCapture_StripsTrailingCarriageReturn` — verifies trailing `\r` is stripped
  - `TestLastCapture_StderrNotCaptured` — verifies stderr output does not appear in `LastCapture`
  - `TestCaptureOutput_UsesWorkingDir` — verifies `CaptureOutput` sets `cmd.Dir`
  - `TestBuildStep_*` — tests for `buildStep` including Claude step variants (iteration, var substitution, missing prompt file, finalize phase)

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

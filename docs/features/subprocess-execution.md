# Subprocess Execution & Streaming

Executes workflow steps as subprocesses with real-time stdout/stderr streaming to both the TUI and a file logger, with support for graceful termination and per-step stdout capture.

- **Last Updated:** 2026-04-13
- **Authors:**
  - River Bailey

## Overview

- The `Runner` struct manages subprocess lifecycle: starting, streaming, terminating, and capturing output
- Subprocess output is forwarded line-by-line via a `sendLine` callback (installed via `SetSender`); the callback writes to a buffered channel consumed by a drain goroutine in `main.go` that batches lines into `LogLinesMsg` messages for the Bubble Tea TUI
- Two scanner goroutines (one for stdout, one for stderr) forward lines to both the pipe and the file logger, coordinated by a `sync.WaitGroup`; only the stdout goroutine captures lines for `LastCapture`
- After each successful `RunStep`, the last non-empty stdout line is stored and retrievable via `LastCapture()`; the orchestrator calls this to bind `CaptureAs` values into the `VarTable`
- `Terminate()` sends SIGTERM with a 3-second SIGKILL fallback (via `terminateGracePeriod`); for sandboxed steps it dispatches through an installed terminator closure rather than signaling the host process directly; `WasTerminated()` lets the orchestrator distinguish user-initiated skips from genuine failures
- `RunSandboxedStep` executes a command inside a Docker sandbox, installing a terminator closure and using explicit empty stdin to prevent raw-mode keyboard inheritance; the cidfile is cleaned up (ENOENT-tolerant) after the step exits. When `opts.CaptureMode == CaptureResult`, a `claudestream.Pipeline` is constructed and the claude-aware stdout path is activated (`bufio.Reader.ReadString('\n')` with a 64MB hard safety cap — lines exceeding the cap emit a `{"type":"ralph_truncation_marker","reason":"line_too_long","bytes":<n>}` sentinel instead of being parsed; each normal line is fed to `pipeline.Observe`); after the subprocess exits, `lastStats` is populated from the pipeline's aggregator and the aggregator error is checked before the process exit code
- `LastStats()` returns the `claudestream.StepStats` from the most recent `RunSandboxedStep` call that used the pipeline; the `stepDispatcher` in `workflow/run.go` calls this after each sandboxed step to fold stats into the run-level accumulator

Key files:
- `src/internal/workflow/workflow.go` — `Runner` struct, `RunStep`, `Terminate`, `WriteToLog`, `LastCapture`, `CaptureOutput`
- `src/internal/workflow/run.go` — `ResolveCommand` ({{VAR}} substitution + script path resolution)
- `src/internal/workflow/workflow_test.go` — Unit tests for subprocess execution
- `src/internal/workflow/run_test.go` — Integration tests for `LastCapture` and `CaptureOutput`

## Architecture

```
                    ┌──────────────────────────────────┐
                    │             Runner                │
                    │                                   │
  RunStep() /       │  ┌─────────────────────────────┐ │
  RunSandboxedStep()│  │       exec.Command()        │ │
  ──────────────────┼─▶│       cmd.Dir = projectDir  │ │
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
| `src/internal/workflow/workflow.go` | `Runner` struct, `RunStep`, `Terminate`, `WriteToLog`, `LastCapture`, `CaptureOutput` |
| `src/internal/workflow/run.go` | `ResolveCommand` — `{{VAR}}` substitution and script path resolution |
| `src/internal/workflow/workflow_test.go` | Tests for `RunStep`, `Terminate`, `WasTerminated`, `WriteToLog`, and `SetSender` |
| `src/internal/workflow/run_test.go` | Integration tests for `LastCapture`, `CaptureOutput`, `ResolveCommand` |

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

    // lastStats holds the StepStats from the most recent RunSandboxedStep call
    // that used the claudestream pipeline. Reset on each RunSandboxedStep entry.
    // Protected by mu; read by LastStats.
    lastStats claudestream.StepStats

    // activePipeline and activePipelineStartedAt support the heartbeat indicator.
    // Set when a claude step starts; cleared in a LIFO defer before pipeline.Close().
    // Protected by processMu; read by HeartbeatSilence.
    activePipeline          *claudestream.Pipeline
    activePipelineStartedAt time.Time
}

// Compile-time assertion that *Runner satisfies ui.HeartbeatReader.
var _ ui.HeartbeatReader = (*Runner)(nil)

// SandboxOptions carries the sandbox-specific parameters for RunSandboxedStep.
type SandboxOptions struct {
    // Terminator, when non-nil, is called by Runner.Terminate() instead of
    // signaling the host process directly. It receives SIGTERM first; if the
    // process does not exit within the grace period, it receives SIGKILL.
    Terminator func(syscall.Signal) error
    // CidfilePath is the path of the Docker --cidfile to clean up after the
    // step exits. May be empty. Cleanup is ENOENT-tolerant.
    CidfilePath string
    // ArtifactPath is the path for the per-step .jsonl file (D14). When
    // non-empty and CaptureMode == CaptureResult, a RawWriter is opened here.
    ArtifactPath string
    // CaptureMode selects the capture semantics for the step. CaptureResult
    // activates the claudestream pipeline. Zero value (CaptureLastLine)
    // preserves current non-pipeline behaviour.
    CaptureMode ui.CaptureMode
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

`LastCapture()` and `LastStats()` are both part of the `StepExecutor` interface. `CaptureOutput` is not — it exists only as a concrete method on `*Runner` (see below).

`LastStats()` returns the `claudestream.StepStats` from the most recent `RunSandboxedStep` call that used the pipeline:

```go
func (r *Runner) LastStats() claudestream.StepStats {
    r.mu.Lock()
    defer r.mu.Unlock()
    return r.lastStats
}
```

`lastStats` is reset to a zero value at the start of every `RunSandboxedStep` call, so the result is always scoped to the most recent step.

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

`RunSandboxedStep` runs a command inside a Docker sandbox. It differs from `RunStep` in: terminator construction is deferred to `runCommand`, explicit empty stdin (`bytes.NewReader(nil)`) prevents raw-mode keyboard inheritance, cidfile cleanup runs after the step exits, and when `opts.CaptureMode == CaptureResult` the claudestream pipeline is activated.

**Non-pipeline path** (`CaptureMode != CaptureResult`): delegates directly to `runCommand` with a plain 256 KB dual-scanner, identical to `RunStep` except for empty stdin and terminator installation.

**Pipeline path** (`CaptureMode == CaptureResult`): constructs a `claudestream.Pipeline` (optionally backed by a `RawWriter` at `opts.ArtifactPath`), passes it to `runCommand`, then post-processes the result:

```go
func (r *Runner) RunSandboxedStep(stepName string, command []string, opts SandboxOptions) error {
    // ... reset terminated, defer cidfile cleanup ...

    r.lastStats = claudestream.StepStats{} // reset before each call

    if opts.CaptureMode != ui.CaptureResult {
        return r.runCommand(stepName, command, bytes.NewReader(nil), &opts, nil)
    }

    // Construct pipeline (RawWriter may be nil if ArtifactPath is empty or open failed).
    pipeline := claudestream.NewPipeline(rw)
    defer func() {
        _ = pipeline.Close()
        if wErr := pipeline.WriteErr(); wErr != nil {
            _ = r.log.Log(stepName, fmt.Sprintf("[artifact] write error: %v", wErr))
        }
    }()

    cmdErr := r.runCommand(stepName, command, bytes.NewReader(nil), &opts, pipeline)

    r.lastStats = pipeline.Aggregator().Stats() // always fold, even on error (D21)

    // D15: aggregator error takes precedence over process exit code.
    if aggErr := pipeline.Aggregator().Err(); aggErr != nil {
        r.lastCapture = ""
        return aggErr
    }
    if cmdErr != nil {
        r.lastCapture = ""
        return cmdErr
    }

    // D13 2a: emit per-step summary line.
    for _, line := range pipeline.Renderer().Finalize(pipeline.Aggregator().Stats()) {
        send(line); r.log.Log(stepName, line)
    }

    // D6: bind result.result to lastCapture for captureAs.
    r.lastCapture = pipeline.Aggregator().Result()
    return nil
}
```

### Shared Subprocess Core (runCommand)

`runCommand` is the private shared core used by both `RunStep` and `RunSandboxedStep`. It takes a `*SandboxOptions` parameter (nil for `RunStep`) and an optional `*claudestream.Pipeline` (nil for non-claude steps). Terminator construction happens **after** the `*exec.Cmd` is created but **before** `cmd.Start()` — this resolves the construction-ordering constraint where `sandbox.NewTerminator` needs the cmd pointer:

- If `opts.Terminator != nil`: use it directly (test-injection path)
- If `opts.Terminator == nil` and `opts.CidfilePath != ""`: auto-construct via `sandbox.NewTerminator(cmd, opts.CidfilePath)`
- If `opts == nil` (RunStep): no terminator installed; `currentTerminator` stays nil throughout

**Stdout handling depends on whether a pipeline is provided:**

- **Pipeline nil (non-claude path):** 256 KB bufio.Scanner, same as stderr. Last non-empty line stored for `LastCapture`.
- **Pipeline non-nil (claude path):** `bufio.Reader.ReadString('\n')` loop with a 64MB hard safety cap (`maxLineBytes`). Lines within the cap are passed verbatim to `pipeline.Observe`; returned display lines are forwarded via `sendLine` and logged. Lines exceeding the cap emit a `{"type":"ralph_truncation_marker","reason":"line_too_long","bytes":<n>}` sentinel to the pipeline and log a `[truncated line: N bytes]` warning — the oversized payload is discarded. `lastCapture` is not set here — `RunSandboxedStep` sets it from `pipeline.Aggregator().Result()` after the pipeline is closed.

Stderr always uses the 256 KB scanner path, prefixing each line with `[stderr] ` for visibility in the TUI log panel.

A key correctness detail: `currentTerminator` is cleared **before** the `procDone` channel is closed, so any `Terminate()` racing with natural step completion observes a nil terminator and short-circuits instead of dispatching a stale signal:

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

- `src/internal/workflow/workflow_test.go` — Tests for `RunStep`, `RunSandboxedStep`, `Terminate`, `WasTerminated`, `WriteToLog`, `ResolveCommand`, `SetSender`, `NewRunner`, and empty-command guards:
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
  - `TestRunSandboxedStep_CleansCidfile` — verifies cidfile is removed after a successful step
  - `TestRunSandboxedStep_CleansCidfile_NonexistentPath` — verifies that a nonexistent cidfile path is tolerated (ENOENT-tolerant cleanup)
  - `TestRunSandboxedStep_CleansCidfileOnError` — verifies cidfile is removed even when the command exits non-zero
  - `TestRunSandboxedStep_CleansCidfile_OnStartError` — verifies the ENOENT-tolerant `defer sandbox.Cleanup` runs even when `cmd.Start()` fails before the container starts (cidfile never written, cleanup must not panic)
  - `TestRunSandboxedStep_ResetTerminatedFlag` — verifies `terminated` is reset at the start of `RunSandboxedStep`
  - `TestRunSandboxedStep_InstallsAndClearsTerminator` — verifies terminator is installed in `currentTerminator` during execution and cleared to nil after the step returns
  - `TestRunSandboxedStep_UsesEmptyStdin` — verifies RunSandboxedStep provides explicit empty stdin so Docker does not inherit the parent's raw-mode keyboard reader
  - `TestRunSandboxedStep_TerminatorClearedBeforeWaitReturn` — verifies `currentTerminator` is cleared before `procDone` is closed (prevents stale signal dispatch on natural exit)
  - `TestRunSandboxedStep_ReturnsErrorForNonExistentCommand` — verifies error is returned when the command does not exist
  - `TestRunSandboxedStep_ReturnsErrorOnNonZeroExit` — verifies error is returned for a non-zero exit code
  - `TestRunSandboxedStep_UsesProjectDir` — verifies `cmd.Dir` is set to `projectDir`
  - `TestTerminate_UsesTerminatorWhenInstalled` — verifies Terminate dispatches SIGTERM+SIGKILL through the terminator closure when one is installed
  - `TestTerminate_UsesProcessWhenNoTerminator` — verifies Terminate signals the host process directly when no terminator is installed
  - `TestTerminate_TerminatorSIGTERMOnlyWhenProcessExitsPromptly` — verifies SIGKILL is not sent when the process exits within the grace period
  - `TestTerminate_IntegrationOrchestrationCanProceed` — integration test verifying that after Terminate() the runner is in a consistent state and can process the next step
- `src/internal/workflow/run_test.go` — Integration tests for:
  - `TestLastCapture_LastNonEmptyStdoutLine` — verifies last non-empty stdout line is returned
  - `TestLastCapture_EmptyOnFailure` — verifies `""` is returned after a failed step
  - `TestLastCapture_StripsTrailingCarriageReturn` — verifies trailing `\r` is stripped
  - `TestLastCapture_StderrNotCaptured` — verifies stderr output does not appear in `LastCapture`
  - `TestCaptureOutput_UsesWorkingDir` — verifies `CaptureOutput` sets `cmd.Dir`
  - `TestBuildStep_ClaudeStepIteration` — verifies a claude iteration step resolves to a docker run command with the correct model and prompt
  - `TestBuildStep_ClaudeStepWithVarSubstitution` — verifies `{{VAR}}` tokens in a claude step's prompt file path are substituted before the prompt is read
  - `TestBuildStep_ClaudeStepMissingPromptFile` — verifies a missing prompt file returns an error wrapped with the step name
  - `TestBuildStep_ClaudeStepFinalize` — verifies a finalize-phase claude step resolves with the correct argv
  - `TestBuildStep_ClaudeStep_SandboxBindMount` — verifies the resolved command contains the project dir as a docker bind mount
  - `TestBuildStep_ClaudeStep_SandboxOptionsCidfile` — verifies `ResolvedStep.CidfilePath` is populated from the sandbox cidfile path
  - `TestBuildStep_ClaudeStep_EnvPassthrough` — verifies env vars listed in the step config produce `-e` flags in the docker run command
  - `TestBuildStep_ClaudeStep_DispatchesToSandboxedRunner` — integration: verifies `Run` dispatches a claude iteration step to `RunSandboxedStep` (not `RunStep`)
  - `TestBuildStep_ClaudeStep_EnvAllowlistMergesBuiltinAndUser` (TP-004) — verifies both builtin env vars (e.g. `ANTHROPIC_API_KEY`) and user-supplied env vars appear as `-e` flags in the resolved command
  - `TestBuildStep_NonClaudeStep_ZeroValuesCidfileAndIsClaude` (TP-005) — verifies non-claude steps resolve with `IsClaude=false` and empty `CidfilePath`
  - `TestBuildStep_ClaudeStep_EnvAllowlistDefensiveCopy` (TP-009) — verifies `buildStep` does not mutate `sandbox.BuiltinEnvAllowlist` when appending user-supplied env vars
  - `TestBuildStep_ClaudeStep_NilUserEnv_OnlyBuiltinsInCommand` (SUGG-004) — verifies that a claude step with a nil user-env slice (no top-level `env` field in `ralph-steps.json`) produces only builtin `-e` flags and no user-supplied vars
  - `TestRunSandboxedStep_AutoConstructsTerminatorFromCidfilePath` (TP-003) — verifies a non-nil terminator is auto-constructed via `sandbox.NewTerminator` when `opts.CidfilePath` is set and `opts.Terminator` is nil; cleared to nil after the step exits
  - `TestRunStep_CurrentTerminatorStaysNilDuringExecution` (TP-010) — verifies `currentTerminator` remains nil throughout a `RunStep` call, confirming the `opts==nil` guard in `runCommand` skips terminator installation for non-sandboxed steps

## Additional Information

- [Architecture Overview](../architecture.md) — System-level view showing how streaming fits into the data flow
- [Variable Output & Injection](../how-to/variable-output-and-injection.md) — How `ResolveCommand` and `LastCapture` fit into the variable injection system
- [Building Custom Workflows](../how-to/building-custom-workflows.md) — How shell command steps use `ResolveCommand` for path resolution
- [Workflow Orchestration](workflow-orchestration.md) — How `RunStep` and `LastCapture` are called by the orchestration loop
- [Variable State Management](../code-packages/vars.md) — How `CaptureAs` bindings use `LastCapture` to populate the `VarTable`
- [Step Definitions & Prompt Building](../code-packages/steps.md) — How steps are loaded and prompts are built before execution
- [Keyboard Input & Error Recovery](keyboard-input.md) — How `Terminate` is triggered by keyboard input
- [Signal Handling & Shutdown](signal-handling.md) — How `Terminate` is triggered by OS signals
- [File Logging](../code-packages/logger.md) — The logger that receives forwarded subprocess output
- [CLI & Configuration](cli-configuration.md) — How `ProjectDir` sets the working directory for all subprocesses
- [Concurrency](../coding-standards/concurrency.md) — Coding standards for mutex-protected writes, WaitGroup drain, and sendLine callback patterns
- [Error Handling](../coding-standards/error-handling.md) — Coding standards for scanner error checking and goroutine write error tracking
- [Go Patterns](../coding-standards/go-patterns.md) — Coding standard for 256KB scanner buffer sizing

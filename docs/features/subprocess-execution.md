# Subprocess Execution & Streaming

Executes workflow steps as subprocesses with real-time stdout/stderr streaming to both the TUI and a file logger, with support for graceful termination.

- **Last Updated:** 2026-04-08 12:00
- **Authors:**
  - River Bailey

## Overview

- The `Runner` struct manages subprocess lifecycle: starting, streaming, terminating, and capturing output
- Subprocess output streams through an `io.Pipe` — the write end receives forwarded stdout/stderr, the read end is passed to the Glyph TUI for real-time display
- Two scanner goroutines (one for stdout, one for stderr) forward lines to both the pipe and the file logger, coordinated by a `sync.WaitGroup`
- `Terminate()` sends SIGTERM with a 3-second SIGKILL fallback; `WasTerminated()` lets the orchestrator distinguish user-initiated skips from genuine failures

Key files:
- `ralph-tui/internal/workflow/workflow.go` — Runner struct, RunStep, Terminate, CaptureOutput, ResolveCommand
- `ralph-tui/internal/workflow/workflow_test.go` — Unit tests for subprocess execution and command resolution

## Architecture

```
                    ┌──────────────────────────┐
                    │        Runner             │
                    │                           │
  RunStep()         │  ┌─────────────────────┐ │
  ──────────────────┼─▶│   exec.Command()    │ │
                    │  │   cmd.Dir = workDir  │ │
                    │  └──────────┬──────────┘ │
                    │             │             │
                    │     ┌───────┴───────┐     │
                    │     │               │     │
                    │  ┌──▼──┐        ┌──▼──┐  │
                    │  │stdout│       │stderr│  │
                    │  │ pipe │       │ pipe │  │
                    │  └──┬──┘        └──┬──┘  │
                    │     │               │     │
                    │  ┌──▼──────────────▼──┐  │
                    │  │  scanner goroutines │  │
                    │  │  (256KB buffer)     │  │
                    │  │  + sync.WaitGroup   │  │
                    │  └──┬──────────────┬──┘  │
                    │     │              │      │
                    │     ▼              ▼      │
                    │  ┌───────┐   ┌────────┐  │
                    │  │io.Pipe│   │ Logger │  │
  Terminate()       │  │(mutex)│   │(file)  │  │
  ──────────────────┼─▶│       │   │        │  │
  SIGTERM→SIGKILL   │  └───┬───┘   └────────┘  │
                    │      │                    │
                    └──────┼────────────────────┘
                           │
                           ▼
                     Glyph TUI
                     (LogReader)
```

## Key Files

| File | Purpose |
|------|---------|
| `ralph-tui/internal/workflow/workflow.go` | Runner struct, RunStep, Terminate, WriteToLog, CaptureOutput, ResolveCommand |
| `ralph-tui/internal/workflow/workflow_test.go` | Unit tests for all Runner methods and ResolveCommand |

## Core Types

```go
// Runner executes workflow steps and streams subprocess output through an io.Pipe.
type Runner struct {
    logReader  *io.PipeReader  // read end → TUI
    logWriter  *io.PipeWriter  // write end ← scanner goroutines
    mu         sync.Mutex      // protects logWriter writes
    log        *logger.Logger  // file logger
    workingDir string          // cmd.Dir for every subprocess

    processMu   sync.Mutex     // guards process state below
    currentProc *os.Process    // active subprocess (nil when idle)
    procDone    chan struct{}   // closed when subprocess exits
    terminated  bool           // set by Terminate(), reset at start of RunStep
}
```

## Implementation Details

### Subprocess Streaming (RunStep)

`RunStep` starts a subprocess, creates two scanner goroutines to forward stdout and stderr, and waits for both to drain before calling `cmd.Wait()`:

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

    var wg sync.WaitGroup
    wg.Add(2)
    go forward(stdout)  // scanner goroutine
    go forward(stderr)  // scanner goroutine
    wg.Wait()           // drain before Wait
    return cmd.Wait()
}
```

Each scanner goroutine uses a 256KB buffer to handle long lines:

```go
scanner := bufio.NewScanner(pipe)
buf := make([]byte, 256*1024)
scanner.Buffer(buf, 256*1024)
```

Writes to the shared `io.PipeWriter` are mutex-protected because `io.PipeWriter` is not safe for concurrent use. The file logger is also written to under its own internal mutex.

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

`WriteToLog` writes a single line directly to the log pipe without running a subprocess. Used for step separator lines between subprocess outputs:

```go
func (r *Runner) WriteToLog(line string) {
    r.mu.Lock()
    _, _ = fmt.Fprintln(r.logWriter, line)
    r.mu.Unlock()
}
```

### Output Capture (CaptureOutput)

`CaptureOutput` runs a command and returns trimmed stdout as a string. Stderr is discarded. Used for single-value queries that don't need streaming:

```go
func (r *Runner) CaptureOutput(command []string) (string, error) {
    cmd := exec.Command(command[0], command[1:]...)
    cmd.Dir = r.workingDir
    out, err := cmd.Output()
    return strings.TrimSpace(string(out)), err
}
```

Used by the workflow loop for:
- `scripts/get_next_issue` — fetches the next GitHub issue ID
- `scripts/get_gh_user` — fetches the GitHub username
- `git rev-parse HEAD` — captures the current commit SHA

### Command Resolution (ResolveCommand)

`ResolveCommand` prepares command arrays for execution by replacing template variables and resolving relative script paths:

```go
func ResolveCommand(projectDir string, command []string, issueID string) []string {
    result := make([]string, len(command))
    for i, arg := range command {
        result[i] = strings.ReplaceAll(arg, "{{ISSUE_ID}}", issueID)
    }
    // Resolve relative script paths (e.g., "scripts/close_gh_issue")
    exe := result[0]
    if !filepath.IsAbs(exe) && strings.ContainsRune(exe, '/') {
        result[0] = filepath.Join(projectDir, exe)
    }
    return result
}
```

Bare commands like `git` are not resolved — only relative paths containing a `/` separator.

## Concurrency

| Resource | Protection | Why |
|----------|-----------|-----|
| `logWriter` (io.PipeWriter) | `mu sync.Mutex` | Two scanner goroutines write concurrently; PipeWriter is not thread-safe |
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

- `ralph-tui/internal/workflow/workflow_test.go` — Tests for RunStep, Terminate, WasTerminated, WriteToLog, CaptureOutput, ResolveCommand, and Close

## Additional Information

- [Architecture Overview](../architecture.md) — System-level view showing how streaming fits into the data flow
- [Variable Output & Injection](../how-to/variable-output-and-injection.md) — How `ResolveCommand` and `CaptureOutput` fit into the variable injection system
- [Building Custom Workflows](../how-to/building-custom-workflows.md) — How shell command steps use ResolveCommand for path resolution
- [Workflow Orchestration](workflow-orchestration.md) — How RunStep is called by the orchestration loop
- [Step Definitions & Prompt Building](step-definitions.md) — How steps are loaded and prompts are built before execution
- [Keyboard Input & Error Recovery](keyboard-input.md) — How Terminate is triggered by keyboard input
- [Signal Handling & Shutdown](signal-handling.md) — How Terminate is triggered by OS signals
- [File Logging](file-logging.md) — The logger that receives forwarded subprocess output
- [CLI & Configuration](cli-configuration.md) — How ProjectDir sets the working directory for all subprocesses
- [Concurrency](../coding-standards/concurrency.md) — Coding standards for mutex-protected writes, WaitGroup drain, and io.Pipe streaming
- [Error Handling](../coding-standards/error-handling.md) — Coding standards for scanner error checking and goroutine write error tracking
- [Go Patterns](../coding-standards/go-patterns.md) — Coding standard for 256KB scanner buffer sizing

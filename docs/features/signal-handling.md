# Signal Handling & Shutdown

Handles OS signals (SIGINT/SIGTERM) to trigger clean shutdown of the pr9k workflow, terminating the active subprocess and exiting with the appropriate status code.

- **Last Updated:** 2026-04-11
- **Authors:**
  - River Bailey

## Overview

- Listens for SIGINT and SIGTERM via `os/signal.Notify` on a buffered channel
- On signal receipt, calls `KeyHandler.ForceQuit()` which terminates the active subprocess and injects `ActionQuit` into the orchestration channel
- The orchestration loop picks up the quit action before the next step starts via a non-blocking channel drain
- The main goroutine tracks whether a signal was received to select the exit code: 0 for normal completion, 1 for signal-initiated shutdown

Key files:
- `src/cmd/src/main.go` — Signal setup, signal handler goroutine, exit code selection

## Architecture

```
  OS Signal
  (SIGINT / SIGTERM)
       │
       ▼
  ┌──────────┐
  │ sigChan  │  buffered channel (cap 1)
  │ (os.Signal)│
  └────┬─────┘
       │
       ▼
  signal handler goroutine:
  ┌──────────────────────────────────────────────┐
  │  <-sigChan               ← blocks unconditionally│
  │  close(signaled)         ← one-shot flag     │
  │  keyHandler.ForceQuit()  ← terminate sub +   │
  │                             inject ActionQuit│
  │  wait on <-workflowDone or 2s timeout        │
  │  program.Kill()          ← always kill       │
  └──────────────────────────────────────────────┘
       │
       ├───▶ Runner.Terminate()     → SIGTERM subprocess, SIGKILL after 3s
       │
       └───▶ Actions <- ActionQuit  → non-blocking send to orchestration
                │
                ▼
          Orchestrate() drains Actions
          before each step → sees ActionQuit
          → returns ActionQuit
          → Run() closes and returns

  workflow goroutine (normal completion path):
  ┌──────────────────────────┐
  │  defer close(workflowDone)│
  │  workflow.Run(...)       │
  │  log.Close()             │  ← flush and close log file
  │  close(lineCh)           │  ← signal drain goroutine to exit
  │  keyHandler.SetMode(     │
  │    ui.ModeDone)          │  ← TUI stays alive for user review
  └──────────────────────────┘

  main goroutine (after program.Run() returns):
  ┌─────────────────────────────────────────┐
  │  signal.Stop(sigChan)                   │  ← deregister after TUI exits
  │  wait on <-workflowDone or 4s timeout   │  ← flush logs, close channels
  │  select {                               │
  │  case <-signaled:                       │
  │    os.Exit(1)   ← signal-initiated      │
  │                    shutdown             │
  │  default:                               │
  │    os.Exit(0)   ← normal completion    │
  │  }                                      │
  └─────────────────────────────────────────┘
```

## Implementation Details

### Signal Registration

In `main.go`, signals are registered before the workflow goroutine starts:

```go
sigChan := make(chan os.Signal, 1)
signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
signaled := make(chan struct{})
go func() {
    <-sigChan
    close(signaled)
    keyHandler.ForceQuit()
    select {
    case <-workflowDone:
    case <-time.After(2 * time.Second):
    }
    program.Kill()
}()
```

The signal handler blocks unconditionally on `<-sigChan` — there is no `case <-workflowDone` escape hatch. This ensures the handler remains active during `ModeDone` (post-workflow), so a SIGINT on the done screen still triggers `ForceQuit` + `program.Kill()` and restores the terminal cleanly. If no signal ever arrives, the goroutine stays blocked forever — this is fine because the process exits when `program.Run()` returns (via `q` → `y` → `tea.QuitMsg`), and the goroutine is cleaned up by process exit.

After `ForceQuit`, the handler waits up to 2 seconds for the workflow goroutine to finish, then always calls `program.Kill()`. If the signal arrives during `ModeDone` (workflow already finished), `<-workflowDone` resolves immediately and `program.Kill()` force-stops the TUI.

### ForceQuit Integration

`KeyHandler.ForceQuit()` does three things:
1. Flips mode to `ModeQuitting` and updates the shortcut bar (so the footer shows `"Quitting..."` immediately)
2. Calls the `cancel` function (which is `Runner.Terminate()`) to stop the active subprocess
3. Non-blocking sends `ActionQuit` to the `Actions` channel so the orchestration loop exits cleanly

The non-blocking send (`select`/`default`) ensures `ForceQuit` never blocks, which is critical since it runs in a signal handler goroutine.

`ForceQuit` is called from **two** places and both produce identical shutdown semantics:
- The OS signal handler goroutine (this file)
- The QuitConfirm `y` path in `KeyHandler.handleQuitConfirm` (see [Keyboard Input](keyboard-input.md))

Unifying these paths means the signal-initiated and user-confirmed quit flows go through the same subprocess-termination + ActionQuit-injection sequence. A test harness that drives `y` from an error-mode step gets the same behavior as sending SIGINT at the terminal.

### Pre-Step Drain

Before each step, `Orchestrate()` performs a non-blocking drain of the `Actions` channel:

```go
select {
case action := <-h.Actions:
    if action == ActionQuit { return ActionQuit }
default:
}
```

This catches the `ActionQuit` injected by `ForceQuit` even if the signal arrives between steps (when no goroutine is blocking on the channel).

### Workflow Goroutine Cleanup

On normal workflow completion, the workflow goroutine flushes logs, closes channels, and enters `ModeDone` — the TUI stays alive so the user can review output:

```go
go func() {
    defer close(workflowDone)
    _ = workflow.Run(runner, proxy, keyHandler, runCfg)
    _ = log.Close()                    // flush and close the log file
    close(lineCh)                      // signal drain goroutine to exit
    keyHandler.SetMode(ui.ModeDone)    // TUI stays alive for user review
}()
```

`signal.Stop(sigChan)` is no longer called here — signals stay registered until the process exits, so the signal handler goroutine remains active during `ModeDone`. The user exits via `q` → `y` → `tea.QuitMsg`, which causes `program.Run()` to return. `signal.Stop` is called after `program.Run()` returns in the main goroutine.

### Exit Code Selection

After `program.Run()` returns, the main goroutine deregisters signals, waits for the workflow goroutine to finish cleanup, then selects the exit code:

```go
_, runErr := program.Run()
signal.Stop(sigChan)

// Wait for the workflow goroutine to finish cleanup (log flush, channel close).
select {
case <-workflowDone:
case <-time.After(4 * time.Second):
}

select {
case <-signaled:
    os.Exit(1)  // signal-initiated shutdown
default:
    os.Exit(0)  // normal completion
}
```

`signal.Stop(sigChan)` deregisters signal notifications after the TUI exits cleanly. The `workflowDone` wait ensures the workflow goroutine's cleanup completes before `os.Exit`. The 4-second timeout exceeds the 3-second `terminateGracePeriod` in `runner.Terminate()` plus buffer for `log.Close()` and `close(lineCh)`.

`program.Run()` returns when `tea.QuitMsg` is received (user quit via `q` → `y`) or `program.Kill()` is called (signal path). Before exit code selection, any non-nil error from `program.Run()` is inspected:

```go
if runErr != nil && !errors.Is(runErr, tea.ErrProgramKilled) {
    fmt.Fprintln(os.Stderr, "bubbletea:", runErr)
    os.Exit(1)
}
```

`tea.ErrProgramKilled` is explicitly tolerated — it is a normal forced-exit return from the signal path, not a crash. Any other non-nil error is an unexpected Bubble Tea failure: it is printed to stderr and the process exits immediately with code 1. The signal handler does not call `os.Exit` itself; exit code selection always happens here in the main goroutine.

## Testing

Signal handling is tested indirectly through:
- `src/internal/ui/ui_test.go` — Tests for `ForceQuit` behavior (cancel called, ActionQuit sent)
- `src/internal/ui/orchestrate_test.go` — Tests for pre-step quit drain (ActionQuit injected before step starts)

## Additional Information

- [Architecture Overview](../architecture.md) — System-level signal flow diagram
- [Keyboard Input & Error Recovery](keyboard-input.md) — ForceQuit method and Actions channel
- [Subprocess Execution & Streaming](subprocess-execution.md) — Terminate method (SIGTERM/SIGKILL)
- [Workflow Orchestration](workflow-orchestration.md) — Pre-step drain in Orchestrate
- [Concurrency](../coding-standards/concurrency.md) — Coding standards for non-blocking sends (critical for signal-safe ForceQuit)
